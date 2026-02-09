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
		saw = true
		if q.Limit != 5 || q.Offset != 2 {
			t.Fatalf("unexpected limit/offset got %d/%d", q.Limit, q.Offset)
		}
		if q.Kind != entityKindPost {
			t.Fatalf("unexpected kind: %s", q.Kind)
		}
		if q.Filter["status"] != "published" {
			t.Fatalf("expected published filter")
		}
		post := &Post{ID: "1", Slug: "hello", Title: "Hello", PublishedAt: &now}
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
