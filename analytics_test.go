package blog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// memEntityStore is a minimal in-memory BlogStore used to exercise the
// analytics flow end-to-end (view → increment → admin read).
type memEntityStore struct {
	mu       sync.Mutex
	entities map[string]*Entity
}

func newMemEntityStore() *memEntityStore {
	return &memEntityStore{entities: map[string]*Entity{}}
}

func (m *memEntityStore) Migrate(ctx context.Context) error { return nil }

func (m *memEntityStore) Save(ctx context.Context, e *Entity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *e
	if copy.Attrs != nil {
		clone := make(Attributes, len(copy.Attrs))
		for k, v := range copy.Attrs {
			clone[k] = v
		}
		copy.Attrs = clone
	}
	m.entities[e.ID] = &copy
	return nil
}

func (m *memEntityStore) Get(ctx context.Context, id string) (*Entity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entities[id]
	if !ok {
		return nil, nil
	}
	copy := *e
	return &copy, nil
}

func (m *memEntityStore) Find(ctx context.Context, q Query) ([]*Entity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matched []*Entity
	for _, e := range m.entities {
		if q.Kind != "" && e.Kind != q.Kind {
			continue
		}
		match := true
		for k, v := range q.Filter {
			switch k {
			case "status":
				if e.Status != v {
					match = false
				}
			case "owner_id":
				if e.OwnerID != v {
					match = false
				}
			case "slug":
				if e.Slug != v {
					match = false
				}
			}
			if !match {
				break
			}
		}
		if !match {
			continue
		}
		copy := *e
		matched = append(matched, &copy)
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return []*Entity{}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = len(matched) - offset
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], nil
}

func (m *memEntityStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entities, id)
	return nil
}

func TestViewDedupeCacheBlocksRepeatViews(t *testing.T) {
	cache := newViewDedupeCache(30 * time.Minute)
	if !cache.shouldCount("token-a", "post-1") {
		t.Fatalf("first view should count")
	}
	if cache.shouldCount("token-a", "post-1") {
		t.Fatalf("repeat view within window should not count")
	}
	if !cache.shouldCount("token-a", "post-2") {
		t.Fatalf("different post for same visitor should still count")
	}
	if !cache.shouldCount("token-b", "post-1") {
		t.Fatalf("different visitor for same post should still count")
	}
}

func TestIsLikelyBot(t *testing.T) {
	cases := map[string]bool{
		"":                             true,
		"Mozilla/5.0 Googlebot/2.1":    true,
		"facebookexternalhit/1.1":      true,
		"curl/7.86.0":                  true,
		"Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit/605.1 Safari/605.1.15": false,
	}
	for ua, want := range cases {
		if got := isLikelyBot(ua); got != want {
			t.Fatalf("isLikelyBot(%q) = %v want %v", ua, got, want)
		}
	}
}

func TestPostViewIncrementsAnalytics(t *testing.T) {
	store := newMemEntityStore()
	now := time.Now().UTC()
	post := &Post{ID: "post-1", Slug: "hello", Title: "Hello", PublishedAt: &now}
	if err := store.Save(context.Background(), entityFromPost(post)); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/hello", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120 Safari/537.36")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view status = %d", rr.Code)
	}

	// Increment runs asynchronously; wait briefly for the entity to land.
	var analytics *Entity
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		analytics, _ = store.Get(context.Background(), analyticsEntityID("post-1"))
		if analytics != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if analytics == nil {
		t.Fatalf("expected analytics entity to be created after view")
	}
	if got := readInt64(analytics.Attrs["total_views"]); got != 1 {
		t.Fatalf("total_views = %d want 1", got)
	}
}

func TestAdminListAnalyticsReturnsRows(t *testing.T) {
	store := newMemEntityStore()
	now := time.Now().UTC()
	post := &Post{ID: "post-1", Slug: "hello", Title: "Hello", PublishedAt: &now}
	if err := store.Save(context.Background(), entityFromPost(post)); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	// Pre-seed an analytics blob to avoid relying on the async path.
	adapter := newStoreAdapter(store)
	if err := adapter.IncrementPostViews(context.Background(), "post-1"); err != nil {
		t.Fatalf("increment: %v", err)
	}
	if err := adapter.IncrementPostViews(context.Background(), "post-1"); err != nil {
		t.Fatalf("increment: %v", err)
	}

	// Add an approved comment so the comment_count field has a value.
	approved := &Comment{ID: "c1", PostID: "post-1", AuthorName: "Ana", Content: "Great", Status: "approved", CreatedAt: time.Now().UTC()}
	if err := store.Save(context.Background(), entityFromComment(approved)); err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	// And a rejected comment that should be excluded.
	rejected := &Comment{ID: "c2", PostID: "post-1", AuthorName: "Spam", Content: "Buy now", Status: "rejected", CreatedAt: time.Now().UTC()}
	if err := store.Save(context.Background(), entityFromComment(rejected)); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/admin/api/analytics", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("analytics status = %d body=%s", rr.Code, rr.Body.String())
	}

	var rows []AdminPostAnalytics
	if err := json.NewDecoder(rr.Body).Decode(&rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d want 1", len(rows))
	}
	row := rows[0]
	if row.PostID != "post-1" {
		t.Fatalf("post_id = %s", row.PostID)
	}
	if row.TotalViews != 2 {
		t.Fatalf("total_views = %d want 2", row.TotalViews)
	}
	if row.CommentCount != 1 {
		t.Fatalf("comment_count = %d want 1 (rejected should be skipped)", row.CommentCount)
	}
	if !strings.EqualFold(row.Status, "published") {
		t.Fatalf("status = %s want published", row.Status)
	}
}

func TestBotViewDoesNotIncrement(t *testing.T) {
	store := newMemEntityStore()
	now := time.Now().UTC()
	post := &Post{ID: "post-1", Slug: "hello", Title: "Hello", PublishedAt: &now}
	if err := store.Save(context.Background(), entityFromPost(post)); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blog/hello", nil)
	req.Header.Set("User-Agent", "Googlebot/2.1 (+http://www.google.com/bot.html)")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view status = %d", rr.Code)
	}

	time.Sleep(100 * time.Millisecond)
	analytics, _ := store.Get(context.Background(), analyticsEntityID("post-1"))
	if analytics != nil {
		t.Fatalf("bot view should not create analytics entity, got %+v", analytics)
	}
}
