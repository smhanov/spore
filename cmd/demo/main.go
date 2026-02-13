package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	blog "github.com/smhanov/spore"
)

type memoryStore struct {
	mu       sync.RWMutex
	entities map[string]*blog.Entity
}

func (m *memoryStore) Migrate(ctx context.Context) error {
	return nil
}

func newMemoryStore() *memoryStore {
	return &memoryStore{entities: map[string]*blog.Entity{}}
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
	_ = m.Save(context.Background(), postToEntity(p))
}

func (m *memoryStore) Save(ctx context.Context, e *blog.Entity) error {
	if e == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ID == "" {
		e.ID = fmt.Sprintf("e%d", len(m.entities)+1)
	}
	copy := *e
	if copy.Attrs == nil {
		copy.Attrs = blog.Attributes{}
	}
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = time.Now().UTC()
	}
	if copy.UpdatedAt == nil {
		now := time.Now().UTC()
		copy.UpdatedAt = &now
	}
	copy.Attrs = cloneAttrs(copy.Attrs)
	m.entities[copy.ID] = &copy
	return nil
}

func (m *memoryStore) Get(ctx context.Context, id string) (*blog.Entity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entity, ok := m.entities[id]
	if !ok {
		return nil, nil
	}
	copy := *entity
	copy.Attrs = cloneAttrs(copy.Attrs)
	return &copy, nil
}

func (m *memoryStore) Find(ctx context.Context, q blog.Query) ([]*blog.Entity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*blog.Entity
	for _, entity := range m.entities {
		if q.Kind != "" && entity.Kind != q.Kind {
			continue
		}
		if !matchesFilters(entity, q.Filter) {
			continue
		}
		copy := *entity
		copy.Attrs = cloneAttrs(copy.Attrs)
		out = append(out, &copy)
	}
	applyEntityOrder(out, q.OrderBy)
	return sliceEntities(out, q.Limit, q.Offset), nil
}

func (m *memoryStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entities, id)
	return nil
}

func postToEntity(post blog.Post) *blog.Entity {
	status := "draft"
	if post.PublishedAt != nil {
		status = "published"
	}
	return &blog.Entity{
		ID:          post.ID,
		Kind:        "post",
		Slug:        post.Slug,
		Status:      status,
		PublishedAt: post.PublishedAt,
		Attrs: blog.Attributes{
			"title":            post.Title,
			"subtitle":         post.Subtitle,
			"content_markdown": post.ContentMarkdown,
			"content_html":     post.ContentHTML,
			"meta_description": post.MetaDescription,
			"author_id":        post.AuthorID,
			"tags":             post.Tags,
		},
	}
}

func cloneAttrs(attrs blog.Attributes) blog.Attributes {
	if attrs == nil {
		return blog.Attributes{}
	}
	payload, err := json.Marshal(attrs)
	if err != nil {
		return blog.Attributes{}
	}
	var out blog.Attributes
	if err := json.Unmarshal(payload, &out); err != nil {
		return blog.Attributes{}
	}
	return out
}

func matchesFilters(entity *blog.Entity, filter map[string]interface{}) bool {
	for key, value := range filter {
		if !matchesFilter(entity, key, value) {
			return false
		}
	}
	return true
}

func matchesFilter(entity *blog.Entity, key string, value interface{}) bool {
	if entity == nil {
		return false
	}
	stringValue := fmt.Sprint(value)
	switch key {
	case "id":
		return entity.ID == stringValue
	case "kind":
		return entity.Kind == stringValue
	case "slug":
		return entity.Slug == stringValue
	case "status":
		return entity.Status == stringValue
	case "owner_id":
		return entity.OwnerID == stringValue
	case "parent_id":
		return entity.ParentID == stringValue
	default:
		return fmt.Sprint(entity.Attrs[key]) == stringValue
	}
}

func applyEntityOrder(entities []*blog.Entity, orderBy string) {
	orderBy = strings.TrimSpace(strings.ToLower(orderBy))
	field := "created_at"
	direction := "desc"
	if orderBy != "" {
		parts := strings.Fields(orderBy)
		if len(parts) >= 1 {
			field = parts[0]
		}
		if len(parts) == 2 {
			direction = strings.ToLower(parts[1])
		}
	}
	ascending := direction == "asc"
	sort.Slice(entities, func(i, j int) bool {
		left := entityOrderValue(entities[i], field)
		right := entityOrderValue(entities[j], field)
		if ascending {
			return left.Before(right)
		}
		return right.Before(left)
	})
}

func entityOrderValue(entity *blog.Entity, field string) time.Time {
	if entity == nil {
		return time.Time{}
	}
	switch field {
	case "published_at":
		if entity.PublishedAt != nil {
			return entity.PublishedAt.UTC()
		}
		return time.Time{}
	case "created_at":
		return entity.CreatedAt.UTC()
	default:
		return entity.CreatedAt.UTC()
	}
}

func sliceEntities(entities []*blog.Entity, limit, offset int) []*blog.Entity {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(entities) {
		return []*blog.Entity{}
	}
	end := len(entities)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return entities[offset:end]
}

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

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

	imageStore, err := blog.NewFileImageStore("images", "/blog/images")
	if err != nil {
		log.Fatal(err)
	}

	handler, err := blog.NewHandler(blog.Config{
		Store:               store,
		ImageStore:          imageStore,
		RoutePrefix:         "/blog",
		AdminAuthMiddleware: auth,
		CustomCSSURLs:       []string{},
		SiteURL:             fmt.Sprintf("http://localhost:%d", *port),
		SiteTitle:           "Spore Demo",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Top-level mux: serve /sitemap.xml at the root, delegate everything else to the blog handler.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Spore Demo</title>
	<style>
		body { font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; margin: 0; background: #f8fafc; color: #0f172a; }
		.wrap { min-height: 100vh; display: grid; place-items: center; padding: 24px; }
		.card { max-width: 680px; background: #fff; border: 1px solid #e2e8f0; border-radius: 16px; padding: 28px; box-shadow: 0 1px 2px rgba(15,23,42,.04); }
		h1 { margin: 0 0 12px; font-size: 28px; }
		p { margin: 0 0 18px; line-height: 1.6; color: #334155; }
		a.btn { display: inline-block; background: #111827; color: #fff; text-decoration: none; padding: 10px 16px; border-radius: 10px; font-weight: 600; }
		a.btn:hover { background: #1f2937; }
	</style>
</head>
<body>
	<main class="wrap">
		<section class="card">
			<h1>Welcome to the Spore demo</h1>
			<p>This demo blog is mounted at <strong>/blog</strong>. The admin panel is at <strong>/blog/admin</strong>.</p>
			<a class="btn" href="/blog">Go to Blog</a>
		</section>
	</main>
</body>
</html>`))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		serveSitemap(w, r, handler)
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Serving blog at http://localhost:%d/blog\n", *port)
	fmt.Printf("Sitemap at http://localhost:%d/sitemap.xml\n", *port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// sitemapURLSet is the top-level <urlset> element.
type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// sitemapURL is a single <url> entry.
type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func serveSitemap(w http.ResponseWriter, r *http.Request, h *blog.Handler) {
	entries, err := h.SitemapEntries(r.Context())
	if err != nil {
		http.Error(w, "failed to build sitemap", http.StatusInternalServerError)
		return
	}

	var urls []sitemapURL
	for _, e := range entries {
		u := sitemapURL{Loc: e.Loc}
		if e.LastMod != nil {
			u.LastMod = e.LastMod.UTC().Format(time.RFC3339)
		}
		urls = append(urls, u)
	}

	doc := sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		http.Error(w, "failed to encode sitemap", http.StatusInternalServerError)
	}
}
