package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
		return payload.ContentMarkdown, payload.Notes
	}

	if obj, ok := extractJSONObject(trimmed); ok {
		if json.Unmarshal([]byte(obj), &payload) == nil {
			return payload.ContentMarkdown, payload.Notes
		}
	}

	return trimmed, ""
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
