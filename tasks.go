package blog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/smhanov/llmhub"
)

const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"

	TaskTypeGenerateDescription = "generate_description"
	TaskTypeGenerateTags        = "generate_tags"
	TaskTypeImportImages        = "import_images"
)

// ---------------------------------------------------------------------------
// Task runner
// ---------------------------------------------------------------------------

// taskRunner manages background processing of persisted async tasks.
type taskRunner struct {
	svc    *service
	notify chan struct{}
}

func newTaskRunner(svc *service) *taskRunner {
	return &taskRunner{
		svc:    svc,
		notify: make(chan struct{}, 1),
	}
}

// start resets any interrupted tasks and begins the processing loop.
func (tr *taskRunner) start() {
	ctx := context.Background()
	if err := tr.svc.cfg.Store.ResetRunningTasks(ctx); err != nil {
		log.Printf("tasks: failed to reset running tasks: %v", err)
	}

	go tr.run()
}

// nudge signals the runner that new work is available.
func (tr *taskRunner) nudge() {
	select {
	case tr.notify <- struct{}{}:
	default:
	}
}

func (tr *taskRunner) run() {
	// Process anything already queued from a previous run.
	tr.processPending()

	for range tr.notify {
		tr.processPending()
	}
}

func (tr *taskRunner) processPending() {
	ctx := context.Background()
	for {
		tasks, err := tr.svc.cfg.Store.ListPendingTasks(ctx)
		if err != nil {
			log.Printf("tasks: list pending: %v", err)
			return
		}
		if len(tasks) == 0 {
			return
		}
		for _, task := range tasks {
			tr.processTask(ctx, task)
		}
	}
}

func (tr *taskRunner) processTask(ctx context.Context, task Task) {
	task.Status = TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()
	if err := tr.svc.cfg.Store.UpdateTask(ctx, &task); err != nil {
		log.Printf("tasks: mark running id=%s: %v", task.ID, err)
		return
	}

	log.Printf("tasks: start id=%s type=%s", task.ID, task.TaskType)
	start := time.Now()

	var err error
	switch task.TaskType {
	case TaskTypeGenerateDescription:
		err = tr.svc.processGenerateDescription(ctx, &task)
	case TaskTypeGenerateTags:
		err = tr.svc.processGenerateTags(ctx, &task)
	case TaskTypeImportImages:
		err = tr.svc.processImportImages(ctx, &task)
	default:
		err = fmt.Errorf("unknown task type: %s", task.TaskType)
	}

	if err != nil {
		log.Printf("tasks: failed id=%s type=%s dt=%s err=%v", task.ID, task.TaskType, time.Since(start), err)
		task.Status = TaskStatusFailed
		errMsg := err.Error()
		task.ErrorMessage = &errMsg
	} else {
		log.Printf("tasks: done id=%s type=%s dt=%s", task.ID, task.TaskType, time.Since(start))
		task.Status = TaskStatusCompleted
	}

	task.UpdatedAt = time.Now().UTC()
	if updateErr := tr.svc.cfg.Store.UpdateTask(ctx, &task); updateErr != nil {
		log.Printf("tasks: update id=%s: %v", task.ID, updateErr)
	}
}

// ---------------------------------------------------------------------------
// Task queueing helpers
// ---------------------------------------------------------------------------

func (s *service) queueDescriptionGeneration(postID string) {
	payload, _ := json.Marshal(map[string]string{"post_id": postID})
	task := Task{
		ID:       generateID(),
		TaskType: TaskTypeGenerateDescription,
		Status:   TaskStatusPending,
		Payload:  string(payload),
		Result:   "{}",
	}
	if err := s.cfg.Store.CreateTask(context.Background(), &task); err != nil {
		log.Printf("tasks: queue description post=%s: %v", postID, err)
		return
	}
	s.tasks.nudge()
}

func (s *service) queueTagGeneration(postID string) {
	payload, _ := json.Marshal(map[string]string{"post_id": postID})
	task := Task{
		ID:       generateID(),
		TaskType: TaskTypeGenerateTags,
		Status:   TaskStatusPending,
		Payload:  string(payload),
		Result:   "{}",
	}
	if err := s.cfg.Store.CreateTask(context.Background(), &task); err != nil {
		log.Printf("tasks: queue tags post=%s: %v", postID, err)
		return
	}
	s.tasks.nudge()
}

func (s *service) queueImageImport(baseSiteURL string, postIDs []string) {
	payload, _ := json.Marshal(importImagesPayload{
		BaseSiteURL: baseSiteURL,
		PostIDs:     postIDs,
	})
	task := Task{
		ID:       generateID(),
		TaskType: TaskTypeImportImages,
		Status:   TaskStatusPending,
		Payload:  string(payload),
		Result:   "{}",
	}
	if err := s.cfg.Store.CreateTask(context.Background(), &task); err != nil {
		log.Printf("tasks: queue image import: %v", err)
		return
	}
	s.tasks.nudge()
}

// ---------------------------------------------------------------------------
// Generate meta description
// ---------------------------------------------------------------------------

func (s *service) processGenerateDescription(ctx context.Context, task *Task) error {
	var payload struct {
		PostID string `json:"post_id"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	post, err := s.cfg.Store.GetPostByID(ctx, payload.PostID)
	if err != nil {
		return fmt.Errorf("load post: %w", err)
	}
	if post == nil {
		return nil // post deleted, nothing to do
	}

	// Skip if description was set between queueing and processing.
	if strings.TrimSpace(post.MetaDescription) != "" {
		return nil
	}

	settings, err := s.cfg.Store.GetAISettings(ctx)
	if err != nil {
		return fmt.Errorf("load ai settings: %w", err)
	}
	provider := dumbAISettings(settings)
	if provider == nil {
		return nil // AI not configured, skip silently
	}

	client, err := newLLMClient(*provider, false)
	if err != nil {
		return fmt.Errorf("create ai client: %w", err)
	}

	prompt := buildDescriptionPrompt(post.Title, post.ContentMarkdown)
	aiCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Printf("ai description start post_id=%s provider=%s model=%s",
		post.ID,
		strings.ToLower(strings.TrimSpace(provider.Provider)),
		strings.TrimSpace(provider.Model),
	)
	start := time.Now()
	resp, err := client.Generate(aiCtx, prompt)
	if err != nil {
		log.Printf("ai description failed post_id=%s dt=%s err=%v", post.ID, time.Since(start), err)
		return fmt.Errorf("ai generation: %w", err)
	}
	log.Printf("ai description done post_id=%s dt=%s", post.ID, time.Since(start))

	description := parseDescriptionResponse(resp.Text())
	if description == "" {
		return fmt.Errorf("ai returned empty description")
	}

	post.MetaDescription = description
	if err := s.cfg.Store.UpdatePost(ctx, post); err != nil {
		return fmt.Errorf("update post: %w", err)
	}
	return nil
}

func buildDescriptionPrompt(title, content string) []*llmhub.Message {
	excerpt := markdownToPlainText(content)
	excerpt = trimToLength(excerpt, 3000)

	system := llmhub.NewSystemMessage(llmhub.Text(
		`You are an expert SEO copywriter who creates irresistible meta descriptions that maximize click-through rates from search results.

Create a meta description for this blog post following these rules:
- 140-160 characters maximum
- Open with a bold claim, surprising fact, provocative question, or counterintuitive insight
- Make the reader feel they'll miss out if they don't click
- Include a clear benefit or takeaway
- Use power words that trigger emotion (discover, proven, secret, essential, mistake, etc.)
- Write in second person ("you") when appropriate
- Avoid weak openings like "This post discusses...", "In this article...", "Learn about..."
- Do NOT repeat the title verbatim
- Return ONLY the description text, nothing else — no quotes, no JSON, no labels`,
	))
	user := llmhub.NewUserMessage(llmhub.Text(
		"Title: " + title + "\n\nContent:\n" + excerpt,
	))
	return []*llmhub.Message{system, user}
}

func parseDescriptionResponse(text string) string {
	trimmed := stripThinkTags(text)
	if trimmed == "" {
		return ""
	}

	// Try to parse as JSON in case the model wraps it.
	var obj map[string]string
	if json.Unmarshal([]byte(trimmed), &obj) == nil {
		for _, key := range []string{"meta_description", "description", "text"} {
			if v, ok := obj[key]; ok && strings.TrimSpace(v) != "" {
				trimmed = strings.TrimSpace(v)
				break
			}
		}
	}

	// Strip surrounding quotes.
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
			(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
			trimmed = trimmed[1 : len(trimmed)-1]
		}
	}

	// Truncate to 160 chars if needed.
	runes := []rune(trimmed)
	if len(runes) > 160 {
		trimmed = string(runes[:157]) + "..."
	}

	return trimmed
}

// ---------------------------------------------------------------------------
// Generate tags (async task)
// ---------------------------------------------------------------------------

func (s *service) processGenerateTags(ctx context.Context, task *Task) error {
	var payload struct {
		PostID string `json:"post_id"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	post, err := s.cfg.Store.GetPostByID(ctx, payload.PostID)
	if err != nil {
		return fmt.Errorf("load post: %w", err)
	}
	if post == nil {
		return nil
	}

	// Skip if tags were already set.
	tags, err := s.cfg.Store.GetPostTags(ctx, post.ID)
	if err != nil {
		return fmt.Errorf("load tags: %w", err)
	}
	if len(tags) > 0 {
		return nil
	}

	settings, err := s.cfg.Store.GetAISettings(ctx)
	if err != nil {
		return fmt.Errorf("load ai settings: %w", err)
	}
	provider := dumbAISettings(settings)
	if provider == nil {
		return nil
	}

	client, err := newLLMClient(*provider, false)
	if err != nil {
		return fmt.Errorf("create ai client: %w", err)
	}

	prompt := buildTaggingPrompt(post.Title, post.ContentMarkdown)
	aiCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Printf("ai tagger-task start post_id=%s provider=%s model=%s",
		post.ID,
		strings.ToLower(strings.TrimSpace(provider.Provider)),
		strings.TrimSpace(provider.Model),
	)
	start := time.Now()
	resp, err := client.Generate(aiCtx, prompt)
	if err != nil {
		log.Printf("ai tagger-task failed post_id=%s dt=%s err=%v", post.ID, time.Since(start), err)
		return fmt.Errorf("ai generation: %w", err)
	}
	log.Printf("ai tagger-task done post_id=%s dt=%s", post.ID, time.Since(start))

	resultTags := parseTaggingResponse(resp.Text())
	if len(resultTags) == 0 {
		return fmt.Errorf("ai returned no tags")
	}

	return s.cfg.Store.SetPostTags(ctx, post.ID, resultTags)
}

// ---------------------------------------------------------------------------
// Import images
// ---------------------------------------------------------------------------

type importImagesPayload struct {
	BaseSiteURL string   `json:"base_site_url"`
	PostIDs     []string `json:"post_ids"`
}

type importImagesResult struct {
	URLMap         map[string]string `json:"url_map"`
	ProcessedCount int              `json:"processed_count"`
	TotalCount     int              `json:"total_count"`
	Errors         []string         `json:"errors,omitempty"`
	ReplacedCount  int              `json:"replaced_count"`
}

func (s *service) processImportImages(ctx context.Context, task *Task) error {
	if s.cfg.ImageStore == nil {
		return fmt.Errorf("image store not configured")
	}

	var payload importImagesPayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	if payload.BaseSiteURL == "" {
		return fmt.Errorf("base_site_url is required")
	}

	// Restore progress from previous run (for resumability).
	var result importImagesResult
	if task.Result != "" && task.Result != "{}" {
		_ = json.Unmarshal([]byte(task.Result), &result)
	}
	if result.URLMap == nil {
		result.URLMap = map[string]string{}
	}

	// Gather unique image URLs from all imported posts.
	imageURLs := map[string]bool{}
	for _, postID := range payload.PostIDs {
		post, err := s.cfg.Store.GetPostByID(ctx, postID)
		if err != nil || post == nil {
			continue
		}
		for _, u := range extractImageURLs(post.ContentHTML, post.ContentMarkdown, payload.BaseSiteURL) {
			imageURLs[u] = true
		}
	}

	result.TotalCount = len(imageURLs)
	log.Printf("tasks: image import found %d unique images from %d posts", result.TotalCount, len(payload.PostIDs))

	// Download each image, skipping already-processed ones.
	for imgURL := range imageURLs {
		if _, ok := result.URLMap[imgURL]; ok {
			continue // already downloaded in a previous run
		}

		newURL, err := s.downloadAndStoreImage(ctx, imgURL)
		if err != nil {
			log.Printf("tasks: image download failed url=%s err=%v", imgURL, err)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", imgURL, err))
			result.ProcessedCount++
			s.saveTaskResult(ctx, task, result)
			continue
		}

		log.Printf("tasks: image downloaded url=%s -> %s", imgURL, newURL)
		result.URLMap[imgURL] = newURL
		result.ProcessedCount++
		s.saveTaskResult(ctx, task, result)
	}

	// Replace old URLs with new URLs in all imported posts.
	for _, postID := range payload.PostIDs {
		post, err := s.cfg.Store.GetPostByID(ctx, postID)
		if err != nil || post == nil {
			continue
		}

		changed := false
		for oldURL, newURL := range result.URLMap {
			if strings.Contains(post.ContentMarkdown, oldURL) {
				post.ContentMarkdown = strings.ReplaceAll(post.ContentMarkdown, oldURL, newURL)
				changed = true
			}
			if strings.Contains(post.ContentHTML, oldURL) {
				post.ContentHTML = strings.ReplaceAll(post.ContentHTML, oldURL, newURL)
				changed = true
			}
		}

		if changed {
			if err := s.cfg.Store.UpdatePost(ctx, post); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update post %s: %v", postID, err))
			} else {
				result.ReplacedCount++
			}
		}
	}

	s.saveTaskResult(ctx, task, result)
	log.Printf("tasks: image import complete downloaded=%d replaced=%d errors=%d",
		len(result.URLMap), result.ReplacedCount, len(result.Errors))
	return nil
}

func (s *service) downloadAndStoreImage(ctx context.Context, imageURL string) (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || !strings.HasPrefix(contentType, "image/") {
		// Guess from the URL extension.
		contentType = contentTypeFromExtension(path.Ext(imageURL))
	}

	// Extract filename from URL path.
	parsedURL, _ := url.Parse(imageURL)
	filename := path.Base(parsedURL.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "image" + extensionFromContentType(contentType)
	}

	// Deterministic ID from URL so the same image is not duplicated.
	id := imageURLHash(imageURL)

	// Limit to 50 MB.
	limited := io.LimitReader(resp.Body, 50<<20)

	newURL, err := s.cfg.ImageStore.SaveImage(ctx, id, filename, contentType, limited)
	if err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	return newURL, nil
}

// imageURLHash returns a deterministic hex ID for a given URL.
func imageURLHash(imageURL string) string {
	sum := sha256.Sum256([]byte(imageURL))
	return hex.EncodeToString(sum[:16])
}

// extractImageURLs finds image URLs in HTML/Markdown content from the given base site.
func extractImageURLs(html, markdown, baseSiteURL string) []string {
	parsedBase, err := url.Parse(strings.TrimSuffix(baseSiteURL, "/"))
	if err != nil || parsedBase.Host == "" {
		return nil
	}
	baseHost := parsedBase.Host
	fullText := html + "\n" + markdown

	matches := imageURLRe.FindAllString(fullText, -1)

	seen := map[string]bool{}
	var result []string
	for _, m := range matches {
		// Clean trailing punctuation sometimes left by regex.
		m = strings.TrimRight(m, ".,;:!?\"')")
		parsed, err := url.Parse(m)
		if err != nil {
			continue
		}
		if parsed.Host != baseHost {
			continue
		}
		if seen[m] {
			continue
		}
		seen[m] = true
		result = append(result, m)
	}
	return result
}

var imageURLRe = regexp.MustCompile(`https?://[^\s"'<>\)]+\.(?:jpg|jpeg|png|gif|webp|svg|bmp|ico)(?:\?[^\s"'<>\)]*)?`)

// saveTaskResult persists intermediate progress for resumability.
func (s *service) saveTaskResult(ctx context.Context, task *Task, result any) {
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	task.Result = string(data)
	task.UpdatedAt = time.Now().UTC()
	_ = s.cfg.Store.UpdateTask(ctx, task)
}
