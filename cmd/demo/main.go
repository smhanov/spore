package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	blog "github.com/smhanov/go-blog"
)

type memoryStore struct {
	mu       sync.RWMutex
	posts    map[string]blog.Post // keyed by ID
	ai       *blog.AISettings
	settings *blog.BlogSettings
	comments map[string]blog.Comment
}

func (m *memoryStore) Migrate(ctx context.Context) error {
	return nil
}

func newMemoryStore() *memoryStore {
	return &memoryStore{posts: map[string]blog.Post{}, comments: map[string]blog.Comment{}}
}

func (m *memoryStore) seed() {
	now := time.Now()
	p := blog.Post{
		ID:              "1",
		Slug:            "hello-world",
		Title:           "Hello, GoBlogPlug",
		ContentMarkdown: "# Hello\nThis is a demo post served from memory.",
		ContentHTML:     "<h1>Hello</h1><p>This is a demo post served from memory.</p>",
		PublishedAt:     &now,
		MetaDescription: "Demo post rendered by GoBlogPlug",
		AuthorID:        1,
	}
	m.posts[p.ID] = p
}

func (m *memoryStore) GetPublishedPostBySlug(ctx context.Context, slug string) (*blog.Post, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.posts {
		if p.Slug == slug && p.PublishedAt != nil {
			copy := p
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *memoryStore) ListPublishedPosts(ctx context.Context, limit, offset int) ([]blog.Post, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var posts []blog.Post
	for _, p := range m.posts {
		if p.PublishedAt != nil {
			posts = append(posts, p)
		}
	}
	// naive ordering by published date desc
	for i := 0; i < len(posts); i++ {
		for j := i + 1; j < len(posts); j++ {
			if posts[j].PublishedAt != nil && posts[i].PublishedAt != nil && posts[j].PublishedAt.After(*posts[i].PublishedAt) {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}
	if offset >= len(posts) {
		return []blog.Post{}, nil
	}
	end := offset + limit
	if end > len(posts) {
		end = len(posts)
	}
	return posts[offset:end], nil
}

func (m *memoryStore) ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]blog.Post, error) {
	return m.ListPublishedPosts(ctx, limit, offset)
}

func (m *memoryStore) CreatePost(ctx context.Context, p *blog.Post) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == "" {
		p.ID = fmt.Sprintf("%d", len(m.posts)+1)
	}
	m.posts[p.ID] = *p
	return nil
}

func (m *memoryStore) UpdatePost(ctx context.Context, p *blog.Post) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.posts[p.ID] = *p
	return nil
}

func (m *memoryStore) GetPostByID(ctx context.Context, id string) (*blog.Post, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.posts[id]
	if !ok {
		return nil, nil
	}
	copy := p
	return &copy, nil
}

func (m *memoryStore) DeletePost(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.posts, id)
	return nil
}

func (m *memoryStore) ListAllPosts(ctx context.Context, limit, offset int) ([]blog.Post, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var posts []blog.Post
	for _, p := range m.posts {
		posts = append(posts, p)
	}
	// Sort by published date (unpublished at end)
	for i := 0; i < len(posts); i++ {
		for j := i + 1; j < len(posts); j++ {
			swap := false
			if posts[i].PublishedAt == nil && posts[j].PublishedAt != nil {
				swap = true
			} else if posts[i].PublishedAt != nil && posts[j].PublishedAt != nil && posts[j].PublishedAt.After(*posts[i].PublishedAt) {
				swap = true
			}
			if swap {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}
	if offset >= len(posts) {
		return []blog.Post{}, nil
	}
	end := offset + limit
	if end > len(posts) {
		end = len(posts)
	}
	return posts[offset:end], nil
}

func (m *memoryStore) GetAISettings(ctx context.Context) (*blog.AISettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ai == nil {
		return nil, nil
	}
	copy := *m.ai
	return &copy, nil
}

func (m *memoryStore) UpdateAISettings(ctx context.Context, settings *blog.AISettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if settings == nil {
		m.ai = nil
		return nil
	}
	copy := *settings
	m.ai = &copy
	return nil
}

func (m *memoryStore) GetBlogSettings(ctx context.Context) (*blog.BlogSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.settings == nil {
		return &blog.BlogSettings{CommentsEnabled: true}, nil
	}
	copy := *m.settings
	return &copy, nil
}

func (m *memoryStore) UpdateBlogSettings(ctx context.Context, settings *blog.BlogSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if settings == nil {
		m.settings = nil
		return nil
	}
	copy := *settings
	m.settings = &copy
	return nil
}

func (m *memoryStore) CreateComment(ctx context.Context, c *blog.Comment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.ID == "" {
		c.ID = fmt.Sprintf("c%d", len(m.comments)+1)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.Status == "" {
		c.Status = "approved"
	}
	m.comments[c.ID] = *c
	return nil
}

func (m *memoryStore) GetCommentByID(ctx context.Context, id string) (*blog.Comment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.comments[id]
	if !ok {
		return nil, nil
	}
	copy := c
	return &copy, nil
}

func (m *memoryStore) ListCommentsByPost(ctx context.Context, postID string) ([]blog.Comment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []blog.Comment
	for _, c := range m.comments {
		if c.PostID == postID {
			out = append(out, c)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.Before(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (m *memoryStore) UpdateCommentContentByOwner(ctx context.Context, id, ownerTokenHash, content string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.comments[id]
	if !ok || c.OwnerTokenHash != ownerTokenHash || c.Status == "rejected" {
		return false, nil
	}
	c.Content = content
	now := time.Now().UTC()
	c.UpdatedAt = &now
	m.comments[id] = c
	return true, nil
}

func (m *memoryStore) DeleteCommentByOwner(ctx context.Context, id, ownerTokenHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.comments[id]
	if !ok || c.OwnerTokenHash != ownerTokenHash {
		return false, nil
	}
	delete(m.comments, id)
	return true, nil
}

func (m *memoryStore) UpdateCommentStatus(ctx context.Context, id, status string, spamReason *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.comments[id]
	if !ok {
		return nil
	}
	c.Status = status
	if spamReason != nil {
		reason := *spamReason
		c.SpamReason = &reason
	}
	now := time.Now().UTC()
	c.UpdatedAt = &now
	if status == "approved" || status == "rejected" {
		c.SpamCheckedAt = &now
	}
	m.comments[id] = c
	return nil
}

func (m *memoryStore) ListCommentsForModeration(ctx context.Context, status string, limit, offset int) ([]blog.AdminComment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []blog.AdminComment
	for _, c := range m.comments {
		if status != "" && c.Status != status {
			continue
		}
		post := m.posts[c.PostID]
		out = append(out, blog.AdminComment{
			Comment:   c,
			PostTitle: post.Title,
			PostSlug:  post.Slug,
		})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if offset >= len(out) {
		return []blog.AdminComment{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (m *memoryStore) DeleteCommentByID(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.comments, id)
	return nil
}

func main() {
	var store blog.BlogStore

	if _, err := os.Stat("blog.db"); err == nil {
		fmt.Println("Found blog.db, using SQLite store")
		db, err := sqlx.Open("sqlite3", "blog.db")
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		store = blog.NewSQLXStore(db)
	} else {
		fmt.Println("blog.db not found, using in-memory store")
		memStore := newMemoryStore()
		memStore.seed()
		store = memStore
	}

	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/blog/admin") {
				// demo: allow everything; plug your auth here
			}
			next.ServeHTTP(w, r)
		})
	}

	imageStore, err := blog.NewFileImageStore("images", "/blog/admin/api/images")
	if err != nil {
		log.Fatal(err)
	}

	handler, err := blog.NewHandler(blog.Config{
		Store:               store,
		ImageStore:          imageStore,
		RoutePrefix:         "/blog",
		AdminAuthMiddleware: auth,
		CustomCSSURLs:       []string{},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Serving blog at http://localhost:8080/blog")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
