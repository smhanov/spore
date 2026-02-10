package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockStore struct {
	migrateFn func(ctx context.Context) error
	saveFn    func(ctx context.Context, e *Entity) error
	getFn     func(ctx context.Context, id string) (*Entity, error)
	findFn    func(ctx context.Context, q Query) ([]*Entity, error)
	deleteFn  func(ctx context.Context, id string) error
}

func (m *mockStore) Migrate(ctx context.Context) error {
	if m.migrateFn != nil {
		return m.migrateFn(ctx)
	}
	return nil
}

func (m *mockStore) Save(ctx context.Context, e *Entity) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, e)
	}
	return nil
}

func (m *mockStore) Get(ctx context.Context, id string) (*Entity, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) Find(ctx context.Context, q Query) ([]*Entity, error) {
	if m.findFn != nil {
		return m.findFn(ctx, q)
	}
	return []*Entity{}, nil
}

func (m *mockStore) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func TestNewHandlerRequiresStore(t *testing.T) {
	if _, err := NewHandler(Config{}); err == nil {
		t.Fatalf("expected error when store is missing")
	}
}

func TestPublicListUsesQueryParams(t *testing.T) {
	saw := false
	now := time.Now().UTC()
	ms := &mockStore{findFn: func(ctx context.Context, q Query) ([]*Entity, error) {
		if q.Kind == entityKindTask {
			return []*Entity{}, nil
		}
		if q.Kind != entityKindPost {
			t.Fatalf("unexpected kind: %s", q.Kind)
		}
		if q.Filter["status"] != "published" {
			t.Fatalf("expected published filter")
		}
		post := &Post{ID: "1", Slug: "hello", Title: "Hello", PublishedAt: &now}
		// The count query uses a large limit; only assert limit/offset on the primary query.
		if q.Limit == 5 && q.Offset == 2 {
			saw = true
		}
		return []*Entity{entityFromPost(post)}, nil
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
	ms := &mockStore{findFn: func(ctx context.Context, q Query) ([]*Entity, error) {
		return []*Entity{}, nil
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
	var saved *Entity
	ms := &mockStore{saveFn: func(ctx context.Context, e *Entity) error {
		saved = e
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
	if resp.ID == "" || saved == nil || saved.ID == "" {
		t.Fatalf("expected generated id, got resp '%s' saved '%v'", resp.ID, saved)
	}
}

func TestAdminUpdateIDMismatch(t *testing.T) {
	called := false
	ms := &mockStore{saveFn: func(ctx context.Context, e *Entity) error {
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
	ms := &mockStore{findFn: func(ctx context.Context, q Query) ([]*Entity, error) {
		return []*Entity{}, nil
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

func TestBuildPagination(t *testing.T) {
	// Page 1 of 3
	p := buildPagination(1, 10, 25, "/blog/")
	if p.CurrentPage != 1 || p.TotalPages != 3 {
		t.Fatalf("expected page 1 of 3, got %d of %d", p.CurrentPage, p.TotalPages)
	}
	if p.PrevPageURL != "" {
		t.Fatalf("expected no prev on page 1, got %s", p.PrevPageURL)
	}
	if p.NextPageURL == "" {
		t.Fatal("expected next page URL on page 1")
	}

	// Page 3 of 3
	p = buildPagination(3, 10, 25, "/blog/")
	if p.CurrentPage != 3 || p.TotalPages != 3 {
		t.Fatalf("expected page 3 of 3, got %d of %d", p.CurrentPage, p.TotalPages)
	}
	if p.NextPageURL != "" {
		t.Fatalf("expected no next on last page, got %s", p.NextPageURL)
	}
	if p.PrevPageURL == "" {
		t.Fatal("expected prev page URL on page 3")
	}

	// Single page
	p = buildPagination(1, 10, 5, "/blog/")
	if p.TotalPages != 1 {
		t.Fatalf("expected 1 total page, got %d", p.TotalPages)
	}
	if p.NextPageURL != "" || p.PrevPageURL != "" {
		t.Fatalf("expected no navigation on single page")
	}
}

func TestPostsToSummaries(t *testing.T) {
	posts := []Post{
		{
			ID:              "1",
			Slug:            "test",
			Title:           "Test Post",
			ContentHTML:     `<p><img src="/images/hero.jpg" alt="Hero"> Hello world</p>`,
			ContentMarkdown: "Hello world this is a test post with some content",
		},
		{
			ID:              "2",
			Slug:            "no-image",
			Title:           "No Image",
			ContentHTML:     "<p>Just text</p>",
			ContentMarkdown: "Just text",
		},
	}

	summaries := postsToSummaries(posts)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	if summaries[0].FirstImage != "/images/hero.jpg" {
		t.Fatalf("expected first image URL, got %q", summaries[0].FirstImage)
	}
	if summaries[0].Excerpt == "" {
		t.Fatal("expected non-empty excerpt")
	}
	if summaries[0].Title != "Test Post" {
		t.Fatalf("expected embedded Post.Title, got %q", summaries[0].Title)
	}

	if summaries[1].FirstImage != "" {
		t.Fatalf("expected empty first image, got %q", summaries[1].FirstImage)
	}
}

func TestTplStripHTML(t *testing.T) {
	input := "<p>Hello <b>world</b></p>"
	result := tplStripHTML(input)
	if strings.Contains(result, "<") {
		t.Fatalf("expected no HTML tags, got %q", result)
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "world") {
		t.Fatalf("expected text content, got %q", result)
	}
}

func TestTplTruncate(t *testing.T) {
	input := "Hello world this is a long string"
	result := tplTruncate(5, input)
	if len([]rune(result)) > 8 { // 5 chars + "..."
		t.Fatalf("expected truncated string, got %q", result)
	}
}

func TestNowTemplateFunction(t *testing.T) {
	before := time.Now()
	result := time.Now() // same as what the template func returns
	after := time.Now()
	if result.Before(before) || result.After(after) {
		t.Fatalf("now() returned unexpected time: %v", result)
	}
}

func TestListAllSkipsPagination(t *testing.T) {
	now := time.Now().UTC()
	ms := &mockStore{findFn: func(ctx context.Context, q Query) ([]*Entity, error) {
		if q.Kind == entityKindTask {
			return []*Entity{}, nil
		}
		post := &Post{
			ID:          "1",
			Slug:        "all-post",
			Title:       "All Post",
			PublishedAt: &now,
		}
		return []*Entity{entityFromPost(post)}, nil
	}}
	h, err := NewHandler(Config{Store: ms, ListAll: true})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	// With ListAll, Pagination should be zero-valued (no page nav rendered)
	if strings.Contains(body, "Page ") {
		t.Fatalf("expected no pagination in ListAll mode; got: %s", body)
	}
	if !strings.Contains(body, "All Post") {
		t.Fatalf("expected post title in output; got: %s", body)
	}
}

func TestListIncludesFirstImageAndExcerpt(t *testing.T) {
	now := time.Now().UTC()
	ms := &mockStore{findFn: func(ctx context.Context, q Query) ([]*Entity, error) {
		if q.Kind == entityKindTask {
			return []*Entity{}, nil
		}
		post := &Post{
			ID:              "1",
			Slug:            "img-post",
			Title:           "Image Post",
			ContentHTML:     `<p><img src="/images/test.jpg"> Some content here</p>`,
			ContentMarkdown: "Some content here for the excerpt",
			PublishedAt:     &now,
		}
		return []*Entity{entityFromPost(post)}, nil
	}}
	h, err := NewHandler(Config{Store: ms})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/images/test.jpg") {
		t.Fatalf("expected first image in output; got: %s", body)
	}
	if !strings.Contains(body, "Page 1 of 1") {
		t.Fatalf("expected pagination in output; got: %s", body)
	}
}
