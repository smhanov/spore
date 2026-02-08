package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/smhanov/llmhub"
)

type aiSettingsResponse struct {
	Settings     AISettings `json:"settings"`
	SmartEnabled bool       `json:"smart_enabled"`
	DumbEnabled  bool       `json:"dumb_enabled"`
}

type aiChatRequest struct {
	Mode            string `json:"mode"`
	ContentMarkdown string `json:"content_markdown"`
	Query           string `json:"query"`
	WebSearch       bool   `json:"web_search"`
}

type aiChatResponse struct {
	ContentMarkdown string `json:"content_markdown"`
	Notes           string `json:"notes,omitempty"`
}

func (s *service) handleAdminGetAISettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.cfg.Store.GetAISettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load ai settings", http.StatusInternalServerError)
		return
	}
	if settings == nil {
		settings = &AISettings{}
	}
	writeJSON(w, aiSettingsResponse{
		Settings:     *settings,
		SmartEnabled: aiProviderConfigured(settings.Smart),
		DumbEnabled:  aiProviderConfigured(settings.Dumb),
	})
}

func (s *service) handleAdminUpdateAISettings(w http.ResponseWriter, r *http.Request) {
	var payload AISettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Store.UpdateAISettings(r.Context(), &payload); err != nil {
		http.Error(w, "failed to update ai settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, aiSettingsResponse{
		Settings:     payload,
		SmartEnabled: aiProviderConfigured(payload.Smart),
		DumbEnabled:  aiProviderConfigured(payload.Dumb),
	})
}

func (s *service) handleAdminAIChat(w http.ResponseWriter, r *http.Request) {
	var req aiChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "smart"
	}

	settings, err := s.cfg.Store.GetAISettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load ai settings", http.StatusInternalServerError)
		return
	}
	if settings == nil {
		http.Error(w, "ai not configured", http.StatusConflict)
		return
	}

	var providerSettings AIProviderSettings
	if mode == "dumb" {
		providerSettings = settings.Dumb
	} else {
		providerSettings = settings.Smart
	}

	if !aiProviderConfigured(providerSettings) {
		http.Error(w, "ai not configured", http.StatusConflict)
		return
	}

	client, err := newLLMClient(providerSettings, req.WebSearch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	prompt := buildAIPrompt(req.ContentMarkdown, req.Query)
	resp, err := client.Generate(r.Context(), prompt)
	if err != nil {
		http.Error(w, fmt.Sprintf("ai request failed: %v", err), http.StatusBadRequest)
		return
	}

	content, notes := parseAIResponse(resp.Text())
	if strings.TrimSpace(content) == "" {
		content = req.ContentMarkdown
	}

	writeJSON(w, aiChatResponse{
		ContentMarkdown: content,
		Notes:           notes,
	})
}

func aiProviderConfigured(settings AIProviderSettings) bool {
	if strings.TrimSpace(settings.Provider) == "" || strings.TrimSpace(settings.Model) == "" {
		return false
	}
	if needsAPIKey(settings.Provider) && strings.TrimSpace(settings.APIKey) == "" {
		return false
	}
	return true
}

func needsAPIKey(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "anthropic", "gemini":
		return true
	default:
		return false
	}
}

func supportsWebSearch(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gemini":
		return true
	default:
		return false
	}
}

func newLLMClient(settings AIProviderSettings, webSearch bool) (*llmhub.Client, error) {
	if strings.TrimSpace(settings.Provider) == "" {
		return nil, fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(settings.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	if needsAPIKey(settings.Provider) && strings.TrimSpace(settings.APIKey) == "" {
		return nil, fmt.Errorf("api key is required for %s", settings.Provider)
	}

	opts := []llmhub.Option{
		llmhub.WithModel(settings.Model),
	}
	if strings.TrimSpace(settings.BaseURL) != "" {
		opts = append(opts, llmhub.WithBaseURL(settings.BaseURL))
	}
	if settings.Temperature != nil {
		opts = append(opts, llmhub.WithTemperature(*settings.Temperature))
	}
	if settings.MaxTokens != nil {
		opts = append(opts, llmhub.WithMaxTokens(*settings.MaxTokens))
	}
	if webSearch && supportsWebSearch(settings.Provider) {
		opts = append(opts, llmhub.WithWebSearch(true))
	}
	if supportsWebSearch(settings.Provider) {
		opts = append(opts, llmhub.WithWebSearch(true))
	}

	return llmhub.New(settings.Provider, settings.APIKey, opts...)
}

func buildAIPrompt(content, query string) []*llmhub.Message {
	system := llmhub.NewSystemMessage(llmhub.Text(
		"You are a meticulous blog editor. Rewrite the provided markdown according to the user request. " +
			"Return only JSON with keys content_markdown and notes. Do not wrap in code fences.",
	))
	user := llmhub.NewUserMessage(llmhub.Text(
		"Current markdown:\n" + content + "\n\nUser request:\n" + query,
	))
	return []*llmhub.Message{system, user}
}

func parseAIResponse(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	payload := struct {
		ContentMarkdown string `json:"content_markdown"`
		Notes           string `json:"notes"`
	}{}

	if json.Unmarshal([]byte(trimmed), &payload) == nil {
		return unwrapNestedJSON(payload.ContentMarkdown, payload.Notes)
	}

	if obj, ok := extractJSONObject(trimmed); ok {
		if json.Unmarshal([]byte(obj), &payload) == nil {
			return unwrapNestedJSON(payload.ContentMarkdown, payload.Notes)
		}
	}

	return trimmed, ""
}

// unwrapNestedJSON detects when the AI model double-nests its JSON response
// (i.e. content_markdown itself is a JSON string with a content_markdown key)
// and recursively unwraps it.
func unwrapNestedJSON(content, notes string) (string, string) {
	inner := struct {
		ContentMarkdown string `json:"content_markdown"`
		Notes           string `json:"notes"`
	}{}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &inner) == nil && inner.ContentMarkdown != "" {
		if notes == "" {
			notes = inner.Notes
		}
		return unwrapNestedJSON(inner.ContentMarkdown, notes)
	}
	return content, notes
}

func extractJSONObject(text string) (string, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return "", false
	}
	return text[start : end+1], true
}

func (s *service) aiPreviewConfigured(ctx context.Context) (bool, bool, error) {
	settings, err := s.cfg.Store.GetAISettings(ctx)
	if err != nil {
		return false, false, err
	}
	if settings == nil {
		return false, false, nil
	}
	return aiProviderConfigured(settings.Smart), aiProviderConfigured(settings.Dumb), nil
}

type commentSpamResult struct {
	Spam   bool   `json:"spam"`
	Reason string `json:"reason"`
}

var (
	markdownCodeBlockRe  = regexp.MustCompile("(?s)```.*?```")
	markdownInlineCodeRe = regexp.MustCompile("`[^`]*`")
	markdownImageRe      = regexp.MustCompile(`!\[([^\]]*)\]\([^\)]*\)`)
	markdownLinkRe       = regexp.MustCompile(`\[(.*?)\]\([^\)]*\)`)
	markdownHeaderRe     = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	markdownQuoteRe      = regexp.MustCompile(`(?m)^>\s*`)
	markdownBulletRe     = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	markdownOrderedRe    = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	htmlTagRe            = regexp.MustCompile(`<[^>]+>`)
)

func (s *service) checkCommentSpam(ctx context.Context, comment Comment, post Post) (bool, string, error) {
	settings, err := s.cfg.Store.GetAISettings(ctx)
	if err != nil {
		return false, "", err
	}
	if settings == nil || !aiProviderConfigured(settings.Dumb) {
		return false, "", nil
	}

	client, err := newLLMClient(settings.Dumb, false)
	if err != nil {
		return false, "", err
	}

	prompt := buildCommentSpamPrompt(comment, post)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.Generate(ctx, prompt)
	if err != nil {
		return false, "", err
	}

	spam, reason := parseCommentSpamResponse(resp.Text())
	return spam, reason, nil
}

func buildCommentSpamPrompt(comment Comment, post Post) []*llmhub.Message {
	excerpt := postExcerptForSpam(post)
	system := llmhub.NewSystemMessage(llmhub.Text(
		"You are an AI assistant who specializes in identifying spam comments on blog posts. " +
			"Analyze the blog post and comment to determine if the comment is spam. " +
			"Classify as either \"spam\" or \"not-spam\" using these characteristics: " +
			"1) relevance to the post content, " +
			"2) promotional links or content related to cryptocurrency or financial products, " +
			"3) generic or templated phrases that could apply to any post, " +
			"4) nonsensical or machine-generated text, " +
			"5) excessive flattery that does not engage with the content, " +
			"6) comments addressing the author by name might not be spam, but consider context. " +
			"If unsure, reply \"not-spam\". Reply with only \"spam\" or \"not-spam\".",
	))
	user := llmhub.NewUserMessage(llmhub.Text(
		"BLOG POST TITLE: " + post.Title + "\n" +
			"BLOG POST EXCERPT: " + excerpt + "\n" +
			"COMMENT TEXT: " + comment.Content,
	))
	return []*llmhub.Message{system, user}
}

func postExcerptForSpam(post Post) string {
	excerpt := strings.TrimSpace(post.MetaDescription)
	if excerpt == "" {
		excerpt = markdownToPlainText(post.ContentMarkdown)
	}
	return trimToLength(excerpt, 500)
}

func trimToLength(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func markdownToPlainText(markdown string) string {
	text := markdown
	text = markdownCodeBlockRe.ReplaceAllString(text, " ")
	text = markdownInlineCodeRe.ReplaceAllString(text, " ")
	text = markdownImageRe.ReplaceAllString(text, "$1")
	text = markdownLinkRe.ReplaceAllString(text, "$1")
	text = markdownHeaderRe.ReplaceAllString(text, "")
	text = markdownQuoteRe.ReplaceAllString(text, "")
	text = markdownBulletRe.ReplaceAllString(text, "")
	text = markdownOrderedRe.ReplaceAllString(text, "")
	text = htmlTagRe.ReplaceAllString(text, " ")
	text = strings.NewReplacer("*", " ", "_", " ", "~", " ", "|", " ").Replace(text)
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func parseCommentSpamResponse(text string) (bool, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, ""
	}

	normalized := strings.ToLower(strings.TrimSpace(trimmed))
	if normalized == "spam" {
		return true, ""
	}
	if normalized == "not-spam" || normalized == "not spam" {
		return false, ""
	}

	var payload commentSpamResult
	if json.Unmarshal([]byte(trimmed), &payload) == nil {
		return payload.Spam, strings.TrimSpace(payload.Reason)
	}

	if obj, ok := extractJSONObject(trimmed); ok {
		if json.Unmarshal([]byte(obj), &payload) == nil {
			return payload.Spam, strings.TrimSpace(payload.Reason)
		}
	}

	return false, ""
}

// generatePostTags asynchronously generates tags for a post using the dumb AI.
func (s *service) generatePostTags(postID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		post, err := s.cfg.Store.GetPostByID(ctx, postID)
		if err != nil || post == nil {
			return
		}

		settings, err := s.cfg.Store.GetAISettings(ctx)
		if err != nil || settings == nil || !aiProviderConfigured(settings.Dumb) {
			return
		}

		client, err := newLLMClient(settings.Dumb, false)
		if err != nil {
			return
		}

		prompt := buildTaggingPrompt(post.Title, post.ContentMarkdown)
		resp, err := client.Generate(ctx, prompt)
		if err != nil {
			return
		}

		tags := parseTaggingResponse(resp.Text())
		if len(tags) == 0 {
			return
		}

		_ = s.cfg.Store.SetPostTags(ctx, postID, tags)
	}()
}

func buildTaggingPrompt(title, content string) []*llmhub.Message {
	plainText := markdownToPlainText(content)
	excerpt := trimToLength(plainText, 3000)

	system := llmhub.NewSystemMessage(llmhub.Text(
		`You are an expert content taxonomy system. Your goal is to analyze blog posts and generate a list of relevant, specific tags that will be used to calculate content similarity and recommend related reading.

Tagging Guidelines:

Specificity: Avoid generic tags (e.g., avoid "Update", "General", "News", "Thoughts"). Prefer specific entities, technologies, or distinct concepts (e.g., use "Go", "Distributed Systems", "Options Trading", "Vue3").

Granularity: Aim for a mix of broad categories (1-2 tags) and specific niches (3-4 tags).

Quantity: Generate exactly 5 to 8 tags per post.

Format: Output strictly a JSON array of strings. Lowercase all tags. Remove punctuation/hashtags.

Relevance: A tag must be central to the post, not just a keyword mentioned once in passing.

Example 1: Input Title: "Understanding Goroutines and Channels" Input Content: [Discussion about concurrency patterns in Go...] Output: ["go", "golang", "concurrency", "goroutines", "channels", "backend development"]

Example 2: Input Title: "My travels to Japan and the best Ramen I ate" Input Content: [Travel log about Tokyo and food...] Output: ["travel", "japan", "tokyo", "food", "ramen", "culinary tourism"]`,
	))
	user := llmhub.NewUserMessage(llmhub.Text(
		"Analyze the following post and return the JSON array of tags.\n\nTitle: " + title + "\nContent: " + excerpt,
	))
	return []*llmhub.Message{system, user}
}

func parseTaggingResponse(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	// Try to parse as JSON array directly
	var tags []string
	if json.Unmarshal([]byte(trimmed), &tags) == nil {
		return cleanTags(tags)
	}

	// Try to extract JSON array from the response
	if arr, ok := extractJSONArray(trimmed); ok {
		if json.Unmarshal([]byte(arr), &tags) == nil {
			return cleanTags(tags)
		}
	}

	return nil
}

func extractJSONArray(text string) (string, bool) {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start == -1 || end == -1 || end <= start {
		return "", false
	}
	return text[start : end+1], true
}

func cleanTags(tags []string) []string {
	var result []string
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		t = strings.Trim(t, "#")
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		result = append(result, t)
	}
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

// contentSignificantlyChanged checks if the markdown content has changed enough to re-tag.
func contentSignificantlyChanged(oldContent, newContent string) bool {
	old := strings.TrimSpace(oldContent)
	new_ := strings.TrimSpace(newContent)
	if old == new_ {
		return false
	}
	if old == "" && new_ != "" {
		return true
	}
	// Simple heuristic: if at least 10% of the content changed by length
	oldLen := len([]rune(old))
	newLen := len([]rune(new_))
	diff := newLen - oldLen
	if diff < 0 {
		diff = -diff
	}
	threshold := oldLen / 10
	if threshold < 50 {
		threshold = 50
	}
	return diff >= threshold
}
