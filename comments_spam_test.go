package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func stringPtr(value string) *string {
	return &value
}

// spamStubServer is an OpenAI-compatible chat-completions stub used as the
// blog's LLM provider in tests. Verdicts are derived from the prompt text:
// a user message containing "SPAMMY" is classified as spam, anything else as
// not-spam. The stub can be told to answer the next N requests with HTTP 500
// to exercise failure handling.
type spamStubServer struct {
	mu       sync.Mutex
	failNext int
	calls    int
	srv      *httptest.Server
}

func newSpamStubServer(t *testing.T) *spamStubServer {
	t.Helper()
	stub := &spamStubServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.calls++
		fail := stub.failNext > 0
		if fail {
			stub.failNext--
		}
		stub.mu.Unlock()

		if fail {
			http.Error(w, "stub unavailable", http.StatusInternalServerError)
			return
		}

		var payload struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)

		text := ""
		for _, msg := range payload.Messages {
			if msg.Role != "user" {
				continue
			}
			var plain string
			if json.Unmarshal(msg.Content, &plain) == nil {
				text += plain
				continue
			}
			var parts []map[string]any
			if json.Unmarshal(msg.Content, &parts) == nil {
				for _, part := range parts {
					if fragment, ok := part["text"].(string); ok {
						text += fragment
					}
				}
			}
		}

		verdict := "not-spam"
		if strings.Contains(text, "SPAMMY") {
			verdict = "spam"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "stub-completion",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": verdict}},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	})
	stub.srv = httptest.NewServer(mux)
	t.Cleanup(stub.srv.Close)
	return stub
}

func (s *spamStubServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *spamStubServer) failNextRequests(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = n
}

// stubProviderSettings returns openai provider settings pointed at the stub.
func stubProviderSettings(baseURL string) AIProviderSettings {
	return AIProviderSettings{
		Provider: "openai",
		Model:    "stub-model",
		APIKey:   "stub-key",
		BaseURL:  baseURL,
	}
}

func seedPublishedPost(t *testing.T, store *memEntityStore, id, slug string) *Post {
	t.Helper()
	now := time.Now().UTC()
	post := &Post{ID: id, Slug: slug, Title: "Hello", ContentMarkdown: "Body text", PublishedAt: &now}
	if err := store.Save(context.Background(), entityFromPost(post)); err != nil {
		t.Fatalf("seed post: %v", err)
	}
	return post
}

func seedComment(t *testing.T, store *memEntityStore, comment *Comment) {
	t.Helper()
	if err := store.Save(context.Background(), entityFromComment(comment)); err != nil {
		t.Fatalf("seed comment %s: %v", comment.ID, err)
	}
}

func loadComment(t *testing.T, store *memEntityStore, id string) *Comment {
	t.Helper()
	comment, err := newStoreAdapter(store).GetCommentByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load comment %s: %v", id, err)
	}
	if comment == nil {
		t.Fatalf("comment %s not found", id)
	}
	return comment
}

// waitForComment polls the store until the comment satisfies predicate.
func waitForComment(t *testing.T, store *memEntityStore, id string, predicate func(*Comment) bool) *Comment {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last *Comment
	for time.Now().Before(deadline) {
		comment, err := newStoreAdapter(store).GetCommentByID(context.Background(), id)
		if err == nil && comment != nil {
			last = comment
			if predicate(comment) {
				return comment
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == nil {
		t.Fatalf("comment %s not found before expected state was reached", id)
	}
	t.Fatalf("comment %s did not reach expected state in time (status=%q spam_reason=%v)", id, last.Status, last.SpamReason)
	return nil
}

func postComment(t *testing.T, h *Handler, slug, author, content string) commentResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"author_name": author, "content": content})
	req := httptest.NewRequest(http.MethodPost, "/blog/"+slug+"/comments", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create comment status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp commentResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode comment response: %v", err)
	}
	return resp
}

func TestCommentNeedsSpamCheck(t *testing.T) {
	stamp := time.Now().UTC().Add(-time.Hour)
	cases := []struct {
		name    string
		comment Comment
		want    bool
	}{
		{"never checked", Comment{Status: "approved"}, true},
		{"stamped clean", Comment{Status: "approved", SpamCheckedAt: &stamp}, false},
		{"stamped with failure marker", Comment{Status: "approved", SpamCheckedAt: &stamp, SpamReason: stringPtr("ai check failed: boom")}, true},
		{"stamped with whitespace marker", Comment{Status: "approved", SpamCheckedAt: &stamp, SpamReason: stringPtr("   ")}, false},
		{"pending never checked", Comment{Status: "pending"}, true},
	}
	for _, tc := range cases {
		if got := commentNeedsSpamCheck(tc.comment); got != tc.want {
			t.Fatalf("%s: commentNeedsSpamCheck = %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestCreateCommentWithoutAIApprovesImmediately(t *testing.T) {
	store := newMemEntityStore()
	seedPublishedPost(t, store, "p1", "hello")
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	resp := postComment(t, h, "hello", "Anon", "Nice post!")
	if resp.Status != "approved" {
		t.Fatalf("status = %s want approved (no AI configured)", resp.Status)
	}

	comment := loadComment(t, store, resp.ID)
	if comment.SpamCheckedAt != nil {
		t.Fatalf("expected no spam stamp without AI, got %v", comment.SpamCheckedAt)
	}
	if comment.SpamReason != nil {
		t.Fatalf("expected no spam reason without AI, got %v", *comment.SpamReason)
	}
}

func TestCreateCommentWithSmartOnlyProviderIsPendingAndChecked(t *testing.T) {
	stub := newSpamStubServer(t)
	store := newMemEntityStore()
	seedPublishedPost(t, store, "p1", "hello")
	adapter := newStoreAdapter(store)
	settings := &AISettings{Smart: stubProviderSettings(stub.srv.URL)}
	if err := adapter.UpdateAISettings(context.Background(), settings); err != nil {
		t.Fatalf("save ai settings: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	resp := postComment(t, h, "hello", "Anon", "Smart provider should hold this for review")
	if resp.Status != "pending" {
		t.Fatalf("status = %s want pending (smart-only provider configured)", resp.Status)
	}

	comment := waitForComment(t, store, resp.ID, func(c *Comment) bool {
		return c.Status == "approved" && c.SpamCheckedAt != nil
	})
	if comment.SpamReason != nil {
		t.Fatalf("expected clean verdict, got reason %q", *comment.SpamReason)
	}
	if stub.requestCount() == 0 {
		t.Fatalf("expected stub LLM to be called via smart fallback")
	}
}

func TestCreateCommentWithDumbProviderRejectsSpam(t *testing.T) {
	stub := newSpamStubServer(t)
	store := newMemEntityStore()
	seedPublishedPost(t, store, "p1", "hello")
	adapter := newStoreAdapter(store)
	settings := &AISettings{Dumb: stubProviderSettings(stub.srv.URL)}
	if err := adapter.UpdateAISettings(context.Background(), settings); err != nil {
		t.Fatalf("save ai settings: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	resp := postComment(t, h, "hello", "Spammer", "SPAMMY cheapest crypto coins, buy now!")
	if resp.Status != "pending" {
		t.Fatalf("status = %s want pending", resp.Status)
	}

	comment := waitForComment(t, store, resp.ID, func(c *Comment) bool {
		return c.Status == "rejected"
	})
	if comment.SpamReason == nil || *comment.SpamReason != "flagged as spam" {
		t.Fatalf("spam reason = %v want %q", comment.SpamReason, "flagged as spam")
	}
	if comment.SpamCheckedAt == nil {
		t.Fatalf("expected spam stamp on rejected comment")
	}
}

func TestSpamRecheckProcessesOnlyEligibleComments(t *testing.T) {
	stub := newSpamStubServer(t)
	store := newMemEntityStore()
	seedPublishedPost(t, store, "p1", "hello")

	older := time.Now().UTC().Add(-time.Hour)
	seedComment(t, store, &Comment{ID: "c-unchecked", PostID: "p1", AuthorName: "Anon", Content: "SPAMMY unrelated promotion", Status: "approved", CreatedAt: older})
	seedComment(t, store, &Comment{ID: "c-human", PostID: "p1", AuthorName: "Anon", Content: "genuinely thoughtful", Status: "approved", CreatedAt: older, SpamCheckedAt: &older})
	seedComment(t, store, &Comment{ID: "c-failed", PostID: "p1", AuthorName: "Anon", Content: "also thoughtful", Status: "approved", CreatedAt: older, SpamCheckedAt: &older, SpamReason: stringPtr("ai check failed: old failure")})
	seedComment(t, store, &Comment{ID: "c-pending", PostID: "p1", AuthorName: "Anon", Content: "first!", Status: "pending", CreatedAt: older})
	seedComment(t, store, &Comment{ID: "c-orphan", PostID: "missing-post", AuthorName: "Anon", Content: "SPAMMY orphaned spam", Status: "approved", CreatedAt: older})

	adapter := newStoreAdapter(store)
	if err := adapter.UpdateAISettings(context.Background(), &AISettings{Dumb: stubProviderSettings(stub.srv.URL)}); err != nil {
		t.Fatalf("save ai settings: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	task := &Task{ID: "t1", TaskType: TaskTypeSpamRecheck, Status: TaskStatusRunning, Payload: `{"reason":"unit test"}`}
	if err := h.svc.processSpamRecheck(context.Background(), task); err != nil {
		t.Fatalf("processSpamRecheck: %v", err)
	}

	// Never-checked approved comment is spam-checked and rejected with reason.
	rejected := loadComment(t, store, "c-unchecked")
	if rejected.Status != "rejected" {
		t.Fatalf("c-unchecked status = %s want rejected", rejected.Status)
	}
	if rejected.SpamReason == nil || *rejected.SpamReason != "flagged as spam" {
		t.Fatalf("c-unchecked reason = %v want %q", rejected.SpamReason, "flagged as spam")
	}

	// Human-approved comment is untouched.
	human := loadComment(t, store, "c-human")
	if human.Status != "approved" {
		t.Fatalf("c-human status = %s want approved", human.Status)
	}
	if human.SpamReason != nil {
		t.Fatalf("c-human reason = %v want nil", human.SpamReason)
	}
	if !human.SpamCheckedAt.Equal(older) {
		t.Fatalf("c-human stamp changed: %v want %v", human.SpamCheckedAt, older)
	}

	// Failure-marked comment is re-checked and cleared.
	failed := loadComment(t, store, "c-failed")
	if failed.Status != "approved" {
		t.Fatalf("c-failed status = %s want approved", failed.Status)
	}
	if failed.SpamReason != nil {
		t.Fatalf("c-failed reason = %v want nil (marker should be cleared)", failed.SpamReason)
	}
	if failed.SpamCheckedAt == nil || !failed.SpamCheckedAt.After(older) {
		t.Fatalf("c-failed stamp = %v want newer than %v", failed.SpamCheckedAt, older)
	}

	// Pending comment is checked and promoted.
	pending := loadComment(t, store, "c-pending")
	if pending.Status != "approved" || pending.SpamCheckedAt == nil {
		t.Fatalf("c-pending status=%s stamp=%v want approved with stamp", pending.Status, pending.SpamCheckedAt)
	}

	// Comment whose post vanished is skipped without failing the task.
	orphan := loadComment(t, store, "c-orphan")
	if orphan.Status != "approved" || orphan.SpamCheckedAt != nil {
		t.Fatalf("c-orphan status=%s stamp=%v want untouched", orphan.Status, orphan.SpamCheckedAt)
	}
}

func TestFailedCheckIsRetriedBySpamRecheck(t *testing.T) {
	stub := newSpamStubServer(t)
	stub.failNextRequests(1)
	store := newMemEntityStore()
	seedPublishedPost(t, store, "p1", "hello")
	adapter := newStoreAdapter(store)
	settings := &AISettings{Dumb: stubProviderSettings(stub.srv.URL)}
	if err := adapter.UpdateAISettings(context.Background(), settings); err != nil {
		t.Fatalf("save ai settings: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	resp := postComment(t, h, "hello", "Anon", "held for a check that will fail")
	if resp.Status != "pending" {
		t.Fatalf("status = %s want pending", resp.Status)
	}

	// The failing check approves the comment but leaves a failure marker.
	comment := waitForComment(t, store, resp.ID, func(c *Comment) bool {
		return c.SpamReason != nil && strings.HasPrefix(*c.SpamReason, "ai check failed:")
	})
	if comment.Status != "approved" {
		t.Fatalf("status = %s want approved after failed check", comment.Status)
	}
	if comment.SpamCheckedAt == nil {
		t.Fatalf("expected stamp even on failed check")
	}

	// The re-check pass retries the comment and clears the marker.
	task := &Task{ID: "t1", TaskType: TaskTypeSpamRecheck, Status: TaskStatusRunning, Payload: `{"reason":"retry"}`}
	if err := h.svc.processSpamRecheck(context.Background(), task); err != nil {
		t.Fatalf("processSpamRecheck: %v", err)
	}
	comment = loadComment(t, store, resp.ID)
	if comment.Status != "approved" {
		t.Fatalf("status = %s want approved after retry", comment.Status)
	}
	if comment.SpamReason != nil {
		t.Fatalf("reason = %q want nil after successful retry", *comment.SpamReason)
	}
	if stub.requestCount() < 2 {
		t.Fatalf("expected failed check plus retry, stub saw %d calls", stub.requestCount())
	}
}

func TestAdminAISettingsSaveQueuesSpamRecheck(t *testing.T) {
	store := newMemEntityStore()
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/blog/admin/api/ai/settings", bytes.NewBufferString(`{"smart":{},"dumb":{}}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save ai settings status = %d body=%s", rr.Code, rr.Body.String())
	}

	tasks, err := newStoreAdapter(store).ListRecentTasks(context.Background(), 50)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	found := map[string]bool{}
	for _, task := range tasks {
		found[task.TaskType] = true
	}
	if !found[TaskTypeSpamRecheck] {
		t.Fatalf("expected a queued %s task, got types %v", TaskTypeSpamRecheck, found)
	}
	if !found[TaskTypePostProcessing] {
		t.Fatalf("expected a queued %s task, got types %v", TaskTypePostProcessing, found)
	}
}
