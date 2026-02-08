package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockStore struct {
	migrateFn                func(ctx context.Context) error
	listFn                   func(ctx context.Context, limit, offset int) ([]Post, error)
	listAllFn                func(ctx context.Context, limit, offset int) ([]Post, error)
	getPubFn                 func(ctx context.Context, slug string) (*Post, error)
	listTagFn                func(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error)
	createFn                 func(ctx context.Context, p *Post) error
	updateFn                 func(ctx context.Context, p *Post) error
	getByIDFn                func(ctx context.Context, id string) (*Post, error)
	deleteFn                 func(ctx context.Context, id string) error
	setPostTagsFn            func(ctx context.Context, postID string, tagNames []string) error
	getPostTagsFn            func(ctx context.Context, postID string) ([]Tag, error)
	loadPostsTagsFn          func(ctx context.Context, posts []Post) error
	getRelatedPostsFn        func(ctx context.Context, postID string, limit int) ([]Post, error)
	getAIFn                  func(ctx context.Context) (*AISettings, error)
	updateAIFn               func(ctx context.Context, settings *AISettings) error
	getSettingsFn            func(ctx context.Context) (*BlogSettings, error)
	updateSettingsFn         func(ctx context.Context, settings *BlogSettings) error
	createCommentFn          func(ctx context.Context, c *Comment) error
	getCommentFn             func(ctx context.Context, id string) (*Comment, error)
	listCommentsFn           func(ctx context.Context, postID string) ([]Comment, error)
	updateCommentByOwnerFn   func(ctx context.Context, id, ownerTokenHash, content string) (bool, error)
	deleteCommentByOwnerFn   func(ctx context.Context, id, ownerTokenHash string) (bool, error)
	updateCommentStatusFn    func(ctx context.Context, id, status string, spamReason *string) error
	listCommentsModerationFn func(ctx context.Context, status string, limit, offset int) ([]AdminComment, error)
	deleteCommentFn          func(ctx context.Context, id string) error
}

func (m *mockStore) Migrate(ctx context.Context) error {
	if m.migrateFn != nil {
		return m.migrateFn(ctx)
	}
	return nil
}

func (m *mockStore) GetPublishedPostBySlug(ctx context.Context, slug string) (*Post, error) {
	if m.getPubFn != nil {
		return m.getPubFn(ctx, slug)
	}
	return nil, nil
}

func (m *mockStore) ListPublishedPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	if m.listFn != nil {
		return m.listFn(ctx, limit, offset)
	}
	return []Post{}, nil
}

func (m *mockStore) ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error) {
	if m.listTagFn != nil {
		return m.listTagFn(ctx, tagSlug, limit, offset)
	}
	return []Post{}, nil
}

func (m *mockStore) CreatePost(ctx context.Context, p *Post) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	return nil
}

func (m *mockStore) UpdatePost(ctx context.Context, p *Post) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, p)
	}
	return nil
}

func (m *mockStore) GetPostByID(ctx context.Context, id string) (*Post, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) DeletePost(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockStore) SetPostTags(ctx context.Context, postID string, tagNames []string) error {
	if m.setPostTagsFn != nil {
		return m.setPostTagsFn(ctx, postID, tagNames)
	}
	return nil
}

func (m *mockStore) GetPostTags(ctx context.Context, postID string) ([]Tag, error) {
	if m.getPostTagsFn != nil {
		return m.getPostTagsFn(ctx, postID)
	}
	return []Tag{}, nil
}

func (m *mockStore) LoadPostsTags(ctx context.Context, posts []Post) error {
	if m.loadPostsTagsFn != nil {
		return m.loadPostsTagsFn(ctx, posts)
	}
	return nil
}

func (m *mockStore) GetRelatedPosts(ctx context.Context, postID string, limit int) ([]Post, error) {
	if m.getRelatedPostsFn != nil {
		return m.getRelatedPostsFn(ctx, postID, limit)
	}
	return []Post{}, nil
}

func (m *mockStore) ListAllPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, limit, offset)
	}
	// Default to ListPublishedPosts behavior for backwards compatibility
	return m.ListPublishedPosts(ctx, limit, offset)
}

func (m *mockStore) GetAISettings(ctx context.Context) (*AISettings, error) {
	if m.getAIFn != nil {
		return m.getAIFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) UpdateAISettings(ctx context.Context, settings *AISettings) error {
	if m.updateAIFn != nil {
		return m.updateAIFn(ctx, settings)
	}
	return nil
}

func (m *mockStore) GetBlogSettings(ctx context.Context) (*BlogSettings, error) {
	if m.getSettingsFn != nil {
		return m.getSettingsFn(ctx)
	}
	return &BlogSettings{CommentsEnabled: true}, nil
}

func (m *mockStore) UpdateBlogSettings(ctx context.Context, settings *BlogSettings) error {
	if m.updateSettingsFn != nil {
		return m.updateSettingsFn(ctx, settings)
	}
	return nil
}

func (m *mockStore) CreateComment(ctx context.Context, c *Comment) error {
	if m.createCommentFn != nil {
		return m.createCommentFn(ctx, c)
	}
	return nil
}

func (m *mockStore) GetCommentByID(ctx context.Context, id string) (*Comment, error) {
	if m.getCommentFn != nil {
		return m.getCommentFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) ListCommentsByPost(ctx context.Context, postID string) ([]Comment, error) {
	if m.listCommentsFn != nil {
		return m.listCommentsFn(ctx, postID)
	}
	return []Comment{}, nil
}

func (m *mockStore) UpdateCommentContentByOwner(ctx context.Context, id, ownerTokenHash, content string) (bool, error) {
	if m.updateCommentByOwnerFn != nil {
		return m.updateCommentByOwnerFn(ctx, id, ownerTokenHash, content)
	}
	return false, nil
}

func (m *mockStore) DeleteCommentByOwner(ctx context.Context, id, ownerTokenHash string) (bool, error) {
	if m.deleteCommentByOwnerFn != nil {
		return m.deleteCommentByOwnerFn(ctx, id, ownerTokenHash)
	}
	return false, nil
}

func (m *mockStore) UpdateCommentStatus(ctx context.Context, id, status string, spamReason *string) error {
	if m.updateCommentStatusFn != nil {
		return m.updateCommentStatusFn(ctx, id, status, spamReason)
	}
	return nil
}

func (m *mockStore) ListCommentsForModeration(ctx context.Context, status string, limit, offset int) ([]AdminComment, error) {
	if m.listCommentsModerationFn != nil {
		return m.listCommentsModerationFn(ctx, status, limit, offset)
	}
	return []AdminComment{}, nil
}

func (m *mockStore) DeleteCommentByID(ctx context.Context, id string) error {
	if m.deleteCommentFn != nil {
		return m.deleteCommentFn(ctx, id)
	}
	return nil
}

func (m *mockStore) CreateTask(ctx context.Context, task *Task) error    { return nil }
func (m *mockStore) GetTask(ctx context.Context, id string) (*Task, error) { return nil, nil }
func (m *mockStore) ListPendingTasks(ctx context.Context) ([]Task, error) { return nil, nil }
func (m *mockStore) ListRecentTasks(ctx context.Context, limit int) ([]Task, error) {
	return nil, nil
}
func (m *mockStore) UpdateTask(ctx context.Context, task *Task) error  { return nil }
func (m *mockStore) ResetRunningTasks(ctx context.Context) error       { return nil }

func TestNewHandlerRequiresStore(t *testing.T) {
	if _, err := NewHandler(Config{}); err == nil {
		t.Fatalf("expected error when store is missing")
	}
}

func TestPublicListUsesQueryParams(t *testing.T) {
	saw := false
	ms := &mockStore{listFn: func(ctx context.Context, limit, offset int) ([]Post, error) {
		saw = true
		if limit != 5 || offset != 2 {
			t.Fatalf("unexpected limit/offset got %d/%d", limit, offset)
		}
		return []Post{{ID: "1", Slug: "hello", Title: "Hello"}}, nil
	}}
	h, err := NewHandler(Config{Store: ms})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/?limit=5&offset=2", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !saw {
		t.Fatalf("list call not observed")
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Hello") {
		t.Fatalf("expected body to include post title; got body: %s", body)
	}
}

func TestPublicViewNotFound(t *testing.T) {
	ms := &mockStore{getPubFn: func(ctx context.Context, slug string) (*Post, error) {
		return nil, nil
	}}
	h, err := NewHandler(Config{Store: ms})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/missing", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d want 404", rr.Code)
	}
}

func TestAdminCreateGeneratesID(t *testing.T) {
	var saved Post
	ms := &mockStore{createFn: func(ctx context.Context, p *Post) error {
		saved = *p
		return nil
	}}
	h, err := NewHandler(Config{Store: ms})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	payload := `{"slug":"new","title":"New","content_markdown":"md","content_html":"<p>md</p>","author_id":1}`
	req := httptest.NewRequest(http.MethodPost, "/blog/admin/api/posts", bytes.NewBufferString(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}

	var resp Post
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" || saved.ID == "" {
		t.Fatalf("expected generated id, got resp '%s' saved '%s'", resp.ID, saved.ID)
	}
}

func TestAdminUpdateIDMismatch(t *testing.T) {
	called := false
	ms := &mockStore{updateFn: func(ctx context.Context, p *Post) error {
		called = true
		return nil
	}}
	h, err := NewHandler(Config{Store: ms})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	payload := `{"id":"different","slug":"post"}`
	req := httptest.NewRequest(http.MethodPut, "/blog/admin/api/posts/expected", bytes.NewBufferString(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400", rr.Code)
	}
	if called {
		t.Fatalf("update should not be called on id mismatch")
	}
}

func TestAdminMiddlewareApplied(t *testing.T) {
	ms := &mockStore{listFn: func(ctx context.Context, limit, offset int) ([]Post, error) {
		return []Post{}, nil
	}}
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-MW", "on")
			next.ServeHTTP(w, r)
		})
	}

	h, err := NewHandler(Config{Store: ms, AdminAuthMiddleware: mw})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/admin/api/posts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get("X-MW") != "on" {
		t.Fatalf("middleware header missing")
	}
}

func TestAdminSPAFallbackServesIndex(t *testing.T) {
	ms := &mockStore{}
	h, err := NewHandler(Config{Store: ms})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/admin/does-not-exist", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Blog Admin") {
		t.Fatalf("expected admin placeholder content")
	}
}
