# Spore

Spore is a drop-in blogging handler for Go web apps. It renders public pages with `html/template`, exposes a JSON admin API, and serves an embedded Vue-admin shell. Features include:

- 📝 Markdown editing with automatic HTML conversion (powered by Goldmark)
- 🖼️ Optional image upload support
- 🎨 Customizable templates and CSS
- 🔒 Pluggable admin authentication middleware
- 🗄️ Flexible storage backend (implement your own or use the included SQLX reference)
- 🏷️ Tag support for organizing posts
- 📊 SEO-friendly with meta descriptions and structured data
- 🤖 Programmatic SEO capabilities for LLM-optimized content generation

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Programmatic SEO](#programmatic-seo)
- [Configuration](#configuration)
- [Implementing the BlogStore Interface](#implementing-the-blogstore-interface)
- [Image Storage](#image-storage)
- [Templates](#templates)
- [Admin UI](#admin-ui)
- [API Reference](#api-reference)
- [Data Models](#data-models)
- [Complete Example](#complete-example)

## Installation

```bash
go get github.com/smhanov/spore
```

## Quick Start

### 1. Build the admin frontend

The admin UI is a Vue app that must be compiled before the Go code can embed it:

```bash
cd frontend
npm install
npm run build
cd ..
```

### 2. Seed the database (Optional)

This project includes a seed script that creates a SQLite database `blog.db` populated with sample content:

```bash
go run ./cmd/seed/main.go
```

### 3. Run the demo

The demo server will automatically use `blog.db` if it exists. Otherwise, it defaults to a transient in-memory store:

```bash
go run ./cmd/demo
```

Then visit:
- **Blog**: [http://localhost:8080/blog](http://localhost:8080/blog)
- **Admin UI**: [http://localhost:8080/blog/admin](http://localhost:8080/blog/admin)

## Programmatic SEO

Spore is designed with programmatic SEO in mind, enabling automatic generation of content optimized for Large Language Models (LLMs) and modern search engines. This feature allows you to:

- Generate structured, semantic content at scale
- Optimize content for LLM comprehension and retrieval
- Create topic clusters and internal linking strategies automatically
- Build content that ranks well in both traditional search engines and AI-powered search

*Note: Programmatic SEO features are under active development and will be available in future releases.*

## Configuration

The `blog.Config` struct controls how Spore integrates with your application:

```go
type Config struct {
    // Store is required - implements the BlogStore interface for persistence
    Store BlogStore

    // ImageStore is optional - enables image upload functionality
    ImageStore ImageStore

    // RoutePrefix sets the base path for all blog routes (default: "/blog")
    RoutePrefix string

    // AdminAuthMiddleware wraps admin routes with authentication
    AdminAuthMiddleware func(http.Handler) http.Handler

    // LayoutTemplatePath provides a custom base template (optional)
    LayoutTemplatePath string

    // CustomCSSURLs injects additional CSS files into rendered pages
    CustomCSSURLs []string
}
```

### Basic Setup

```go
package main

import (
    "log"
    "net/http"

    "github.com/jmoiron/sqlx"
    _ "github.com/mattn/go-sqlite3"
    blog "github.com/smhanov/spore"
)

func main() {
    // Open database connection
    db, err := sqlx.Open("sqlite3", "blog.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Create the blog handler
    handler, err := blog.NewHandler(blog.Config{
        Store:       blog.NewSQLXStore(db),
        RoutePrefix: "/blog",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Starting server on :8080")
    http.ListenAndServe(":8080", handler)
}
```

### With Authentication Middleware

```go
func main() {
    db, _ := sqlx.Open("sqlite3", "blog.db")
    defer db.Close()

    // Define authentication middleware
    authMiddleware := func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Check for valid session/token
            token := r.Header.Get("Authorization")
            if !isValidToken(token) {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }

    handler, err := blog.NewHandler(blog.Config{
        Store:               blog.NewSQLXStore(db),
        RoutePrefix:         "/blog",
        AdminAuthMiddleware: authMiddleware,
    })
    if err != nil {
        log.Fatal(err)
    }

    http.ListenAndServe(":8080", handler)
}
```

## Implementing the BlogStore Interface

The `BlogStore` interface defines the persistence contract your application must satisfy:

```go
type BlogStore interface {
    // Migrate applies any pending schema changes for the store.
    Migrate(ctx context.Context) error

    // Public methods - used for rendering the blog
    GetPublishedPostBySlug(ctx context.Context, slug string) (*Post, error)
    ListPublishedPosts(ctx context.Context, limit, offset int) ([]Post, error)
    ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error)

    // Admin methods - used by the admin API
    CreatePost(ctx context.Context, p *Post) error
    UpdatePost(ctx context.Context, p *Post) error
    GetPostByID(ctx context.Context, id string) (*Post, error)
    DeletePost(ctx context.Context, id string) error
    ListAllPosts(ctx context.Context, limit, offset int) ([]Post, error)
}
```

### Using the Built-in SQLX Store

The package includes a ready-to-use SQLX implementation:

Migrations are applied automatically when `blog.NewHandler` is called, so you
do not need to run them manually.

```go
import (
    "github.com/jmoiron/sqlx"
    _ "github.com/mattn/go-sqlite3" // or your preferred driver
    blog "github.com/smhanov/spore"
)

func main() {
    db, err := sqlx.Open("sqlite3", "blog.db")
    if err != nil {
        log.Fatal(err)
    }

    store := blog.NewSQLXStore(db)
    // Use store in your Config...
}
```

### Database Schema

The package exports schema constants used by the built-in migrations. You can also
reuse them in your own tooling if needed:

```go
// SchemaBlogPosts creates the main posts table
const SchemaBlogPosts = `
CREATE TABLE IF NOT EXISTS blog_posts (
    id TEXT PRIMARY KEY,
    slug TEXT UNIQUE NOT NULL,
    title TEXT NOT NULL,
    content_markdown TEXT NOT NULL,
    content_html TEXT NOT NULL,
    published_at TIMESTAMP NULL,
    meta_description TEXT,
    author_id INTEGER NOT NULL
);`

// SchemaBlogTags creates the tags table
const SchemaBlogTags = `
CREATE TABLE IF NOT EXISTS blog_tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL
);`

// SchemaBlogPostTags creates the many-to-many relationship
const SchemaBlogPostTags = `
CREATE TABLE IF NOT EXISTS blog_post_tags (
    post_id TEXT NOT NULL REFERENCES blog_posts(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES blog_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, tag_id)
);`
```

### Custom Store Implementation

Here's an example in-memory store implementation:

```go
type memoryStore struct {
    mu    sync.RWMutex
    posts map[string]blog.Post
}

func newMemoryStore() *memoryStore {
    return &memoryStore{posts: map[string]blog.Post{}}
}

func (m *memoryStore) Migrate(ctx context.Context) error {
    return nil
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
    // Sort by published date descending
    sort.Slice(posts, func(i, j int) bool {
        return posts[i].PublishedAt.After(*posts[j].PublishedAt)
    })
    // Apply pagination
    if offset >= len(posts) {
        return []blog.Post{}, nil
    }
    end := offset + limit
    if end > len(posts) {
        end = len(posts)
    }
    return posts[offset:end], nil
}

func (m *memoryStore) ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error) {
    // Filter posts by tag, then paginate
    // Implementation depends on how you store tags
}

func (m *memoryStore) CreatePost(ctx context.Context, p *blog.Post) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if p.ID == "" {
        p.ID = uuid.New().String()
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
    if p, ok := m.posts[id]; ok {
        copy := p
        return &copy, nil
    }
    return nil, nil
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
    // Apply pagination...
    return posts[offset:end], nil
}
```

## Image Storage

Spore supports optional image uploads through the `ImageStore` interface:

```go
type ImageStore interface {
    // SaveImage stores an image and returns its URL/path for retrieval
    SaveImage(ctx context.Context, id, filename, contentType string, reader io.Reader) (url string, err error)

    // GetImage retrieves an image by its ID
    GetImage(ctx context.Context, id string) (contentType string, reader io.ReadCloser, err error)

    // DeleteImage removes an image by its ID
    DeleteImage(ctx context.Context, id string) error
}
```

### Using the Built-in File Image Store

```go
// Create a file-based image store
imageStore, err := blog.NewFileImageStore(
    "uploads",                    // Directory to store images
    "/blog/admin/api/images",     // URL prefix for serving images
)
if err != nil {
    log.Fatal(err)
}

handler, err := blog.NewHandler(blog.Config{
    Store:      store,
    ImageStore: imageStore,  // Enable image uploads
    // ...
})
```

### Custom Image Store (e.g., S3)

```go
type s3ImageStore struct {
    client *s3.Client
    bucket string
    cdnURL string
}

func (s *s3ImageStore) SaveImage(ctx context.Context, id, filename, contentType string, reader io.Reader) (string, error) {
    key := fmt.Sprintf("blog-images/%s-%s", id, filename)
    _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
        Bucket:      &s.bucket,
        Key:         &key,
        Body:        reader,
        ContentType: &contentType,
    })
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s/%s", s.cdnURL, key), nil
}

func (s *s3ImageStore) GetImage(ctx context.Context, id string) (string, io.ReadCloser, error) {
    // Retrieve from S3...
}

func (s *s3ImageStore) DeleteImage(ctx context.Context, id string) error {
    // Delete from S3...
}
```

## Templates

Default templates are embedded in the package. You can customize the appearance by:

### 1. Adding Custom CSS

```go
handler, err := blog.NewHandler(blog.Config{
    Store: store,
    CustomCSSURLs: []string{
        "/static/blog-custom.css",
        "https://fonts.googleapis.com/css2?family=Inter:wght@400;600&display=swap",
    },
})
```

### 2. Providing a Custom Base Layout

Create a custom layout template that defines a `base.html` template:

```html
<!-- templates/my-layout.html -->
{{define "base.html"}}
<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <title>{{if .Post}}{{.Post.Title}} | My Site{{else}}Blog | My Site{{end}}</title>
    {{if .Post}}
        <meta name="description" content="{{.Post.MetaDescription}}">
    {{end}}
    {{range .CustomCSS}}
        <link rel="stylesheet" href="{{.}}">
    {{end}}
    <link rel="stylesheet" href="/static/styles.css">
</head>
<body>
    <nav>
        <a href="/">Home</a>
        <a href="{{.RoutePrefix}}/">Blog</a>
    </nav>
    <main>
        {{block "content" .}}{{end}}
    </main>
    <footer>© 2026 My Company</footer>
</body>
</html>
{{end}}
```

Then configure it:

```go
handler, err := blog.NewHandler(blog.Config{
    Store:              store,
    LayoutTemplatePath: "templates/my-layout.html",
})
```

### Template Data

Templates receive the following data:

**List Page (list.html):**
```go
map[string]any{
    "Posts":       []Post,      // List of published posts
    "RoutePrefix": string,      // e.g., "/blog"
    "CustomCSS":   []string,    // Custom CSS URLs
}
```

**Post Page (post.html):**
```go
map[string]any{
    "Post":        *Post,       // The full post object
    "RoutePrefix": string,      // e.g., "/blog"
    "CustomCSS":   []string,    // Custom CSS URLs
}
```

### Available Template Functions

- `safeHTML`: Renders HTML content without escaping (use for `Post.ContentHTML`)

## Admin UI

The admin interface is embedded from `frontend/dist` and accessible at `<RoutePrefix>/admin`.

### Features

- Create, edit, and delete posts
- Markdown editor with live preview
- Image upload (when ImageStore is configured)
- Publish/unpublish posts
- SEO metadata editing

### Building the Admin UI

```bash
cd frontend
npm install
npm run build
```

The build output in `frontend/dist` is automatically embedded when you build your Go application.

## API Reference

### Public Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `<prefix>/` | List published posts (supports `?limit=N&offset=N`) |
| GET | `<prefix>/{slug}` | View a single published post |

### Admin API Routes

All admin routes are prefixed with `<prefix>/admin/api` and protected by your `AdminAuthMiddleware`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/posts` | List all posts (supports `?limit=N&offset=N`) |
| GET | `/posts/{id}` | Get a post by ID |
| POST | `/posts` | Create a new post |
| PUT | `/posts/{id}` | Update a post |
| DELETE | `/posts/{id}` | Delete a post |
| GET | `/images/enabled` | Check if image upload is enabled |
| POST | `/images` | Upload an image (multipart form, field: `image`) |
| GET | `/images/{id}` | Retrieve an image |
| DELETE | `/images/{id}` | Delete an image |

### Example API Requests

**Create a Post:**
```bash
curl -X POST http://localhost:8080/blog/admin/api/posts \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "my-first-post",
    "title": "My First Post",
    "content_markdown": "# Hello World\n\nThis is my first blog post!",
    "meta_description": "An introduction to my blog",
    "author_id": 1
  }'
```

**Publish a Post:**
```bash
curl -X PUT http://localhost:8080/blog/admin/api/posts/{id} \
  -H "Content-Type: application/json" \
  -d '{
    "id": "...",
    "slug": "my-first-post",
    "title": "My First Post",
    "content_markdown": "# Hello World\n\nThis is my first blog post!",
    "published_at": "2026-01-30T12:00:00Z",
    "meta_description": "An introduction to my blog",
    "author_id": 1
  }'
```

**Upload an Image:**
```bash
curl -X POST http://localhost:8080/blog/admin/api/images \
  -F "image=@photo.jpg"
```

## Data Models

### Post

```go
type Post struct {
    ID              string     `json:"id"`
    Slug            string     `json:"slug"`              // URL-friendly identifier
    Title           string     `json:"title"`
    ContentMarkdown string     `json:"content_markdown"`  // Source markdown
    ContentHTML     string     `json:"content_html"`      // Rendered HTML (auto-generated)
    PublishedAt     *time.Time `json:"published_at"`      // nil = draft
    MetaDescription string     `json:"meta_description"`  // SEO description
    AuthorID        int        `json:"author_id"`
    Tags            []Tag      `json:"tags"`
}
```

### Tag

```go
type Tag struct {
    ID   string `json:"id"`
    Name string `json:"name"`   // Display name
    Slug string `json:"slug"`   // URL-friendly identifier
}
```

## Complete Example

Here's a full example integrating Spore into an existing application:

```go
package main

import (
    "log"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/jmoiron/sqlx"
    _ "github.com/mattn/go-sqlite3"
    blog "github.com/smhanov/spore"
)

func main() {
    // Initialize database
    db, err := sqlx.Open("sqlite3", "app.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Run blog migrations
    db.MustExec(blog.SchemaBlogPosts)
    db.MustExec(blog.SchemaBlogTags)
    db.MustExec(blog.SchemaBlogPostTags)

    // Create stores
    blogStore := blog.NewSQLXStore(db)
    imageStore, err := blog.NewFileImageStore("uploads", "/blog/admin/api/images")
    if err != nil {
        log.Fatal(err)
    }

    // Authentication middleware
    adminAuth := func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            session, err := getSession(r)
            if err != nil || !session.IsAdmin {
                http.Redirect(w, r, "/login", http.StatusSeeOther)
                return
            }
            next.ServeHTTP(w, r)
        })
    }

    // Create blog handler
    blogHandler, err := blog.NewHandler(blog.Config{
        Store:               blogStore,
        ImageStore:          imageStore,
        RoutePrefix:         "/blog",
        AdminAuthMiddleware: adminAuth,
        LayoutTemplatePath:  "templates/layout.html",
        CustomCSSURLs:       []string{"/static/blog.css"},
    })
    if err != nil {
        log.Fatal(err)
    }

    // Set up main router
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // Mount static files
    r.Handle("/static/*", http.StripPrefix("/static/", 
        http.FileServer(http.Dir("static"))))
    
    // Serve uploaded images
    r.Handle("/uploads/*", http.StripPrefix("/uploads/", 
        http.FileServer(http.Dir("uploads"))))

    // Mount your application routes
    r.Get("/", handleHome)
    r.Get("/login", handleLogin)
    r.Post("/login", handleLoginPost)

    // Mount the blog
    r.Mount("/", blogHandler)

    log.Println("Server running on http://localhost:8080")
    log.Println("Blog at http://localhost:8080/blog")
    log.Println("Admin at http://localhost:8080/blog/admin")
    http.ListenAndServe(":8080", r)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Welcome to my site!"))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
    // Render login form...
}

func handleLoginPost(w http.ResponseWriter, r *http.Request) {
    // Process login...
}

func getSession(r *http.Request) (*Session, error) {
    // Retrieve session from cookie...
    return nil, nil
}

type Session struct {
    UserID  int
    IsAdmin bool
}
```

## LLM Integration Prompt

Use this prompt with an LLM to integrate Spore into your existing Go project:

```
I want to add the Spore blogging system to my existing Go web application. Here's what I need:

PROJECT CONTEXT:
- My project uses [describe your web framework: chi/mux/gin/standard http.ServeMux/etc.]
- Database: [SQLite/PostgreSQL/MySQL/other]
- Current project structure: [briefly describe]
- Authentication: [describe your auth system if any, or "none yet"]

REQUIREMENTS:
1. Install the Spore package (github.com/smhanov/spore)
2. Set up the database schema with the three required tables:
   - blog_posts (id, slug, title, content_markdown, content_html, published_at, meta_description, author_id)
   - blog_tags (id, name, slug)
   - blog_post_tags (post_id, tag_id) - junction table
3. Create a blog handler with these configurations:
   - Store: Use the built-in SQLX store with my database
   - ImageStore: Set up file-based image storage in "./uploads/images" directory
   - RoutePrefix: Mount the blog at "/blog"
   - AdminAuthMiddleware: [If you have auth: "integrate with my existing auth", otherwise: "create basic auth check"]
4. Mount the blog handler to my existing router
5. Ensure image uploads are accessible via the web server
6. Add a seed command to populate initial blog posts (optional but recommended)

ADDITIONAL CONSIDERATIONS:
- Ensure the uploads directory exists and is writable
- Configure static file serving for the uploads directory
- The blog admin UI will be at /blog/admin
- Public blog will be at /blog

Please provide:
1. Step-by-step implementation instructions
2. All code changes needed
3. Database migration commands
4. Any new dependencies to add to go.mod
5. Instructions for running and testing the blog

EXAMPLE INTEGRATION PATTERNS:

For chi router:
```go
blogHandler, err := blog.NewHandler(blog.Config{
    Store:       blog.NewSQLXStore(db),
    ImageStore:  imageStore,
    RoutePrefix: "/blog",
})
r.Mount("/", blogHandler)
```

For standard http.ServeMux:
```go
blogHandler, err := blog.NewHandler(blog.Config{...})
mux.Handle("/blog/", blogHandler)
```

Adapt the integration to match my project's patterns and conventions.
```

This prompt provides an LLM with all the context needed to successfully integrate Spore into an existing Go project. Customize the PROJECT CONTEXT section with your specific details before using it.

## License

MIT License - see LICENSE file for details.
