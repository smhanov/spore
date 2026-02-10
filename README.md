# Spore

Spore is a drop-in blogging handler for Go web apps. It renders public pages with `html/template`, exposes a JSON admin API, and serves an embedded Vue-admin shell. Features include:

- Markdown editing with automatic HTML conversion (powered by Goldmark)
- Optional image upload support
- Customizable templates and CSS
- Pluggable admin authentication middleware
- Flexible storage backend (implement your own or use the included SQLX reference)
- AI-powered auto-tagging, auto-descriptions, interactive AI chat, and spam detection
- Related posts section based on shared tags
- SEO-friendly with meta descriptions and structured data
- Public comments with one-level replies, @mentions, and owner-only edit/delete
- Admin moderation tools with instant hide/delete
- WXR (WordPress eXtended RSS) import and export
- RSS 2.0 feed with autodiscovery
- Sitemap integration for SEO
- Configurable date display (absolute or approximate)
- Pagination with `?page=N` support on list pages
- Custom template directory for full template overriding
- Template helper functions (`truncate`, `stripHTML`) for card layouts
- `PostSummary` data with `FirstImage` and `Excerpt` for rich card UIs

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [AI Features](#ai-features)
- [Related Posts](#related-posts)
- [Comments](#comments)
- [Date Display](#date-display)
- [RSS Feed](#rss-feed)
- [Sitemap](#sitemap)
- [WXR Import / Export](#wxr-import--export)
- [Implementing the BlogStore Interface](#implementing-the-blogstore-interface)
- [Image Storage](#image-storage)
- [Templates](#templates)
  - [Custom Template Directory](#3-custom-template-directory)
  - [Template Data](#template-data)
  - [Available Template Functions](#available-template-functions)
- [Pagination](#pagination)
- [Admin UI](#admin-ui)
- [API Reference](#api-reference)
- [Data Models](#data-models)
- [Complete Example](#complete-example)

## Installation

```bash
go get github.com/smhanov/spore
```

## Quick Start

The admin UI is pre-built and embedded in the Go module — no Node.js or npm required.

### 1. Seed the database (Optional)

This project includes a seed script that creates a SQLite database `blog.db` populated with sample content:

```bash
go run ./cmd/seed/main.go
```

### 2. Run the demo

The demo server will automatically use `blog.db` if it exists. Otherwise, it defaults to a transient in-memory store:

```bash
go run ./cmd/demo
```

Then visit:

- **Blog**: [http://localhost:8080/blog](http://localhost:8080/blog)
- **Admin UI**: [http://localhost:8080/blog/admin](http://localhost:8080/blog/admin)

## Configuration

The `Config` struct controls how Spore integrates with your application:

```go
type Config struct {
    // Store is required — implements the BlogStore interface for persistence.
    Store BlogStore

    // ImageStore is optional — enables image upload functionality.
    ImageStore ImageStore

    // RoutePrefix sets the base path for all blog routes (default: "/blog").
    RoutePrefix string

    // AdminAuthMiddleware wraps admin routes with authentication.
    AdminAuthMiddleware func(http.Handler) http.Handler

    // LayoutTemplatePath provides a custom base template (optional).
    LayoutTemplatePath string

    // CustomCSSURLs injects additional CSS files into rendered pages.
    CustomCSSURLs []string

    // StaticFilePath is the optional directory from which to serve files
    // not found as posts (e.g., legacy paths or downloadable assets).
    StaticFilePath string

    // TemplatesDir is an optional directory containing custom templates
    // (list.html, post.html). If set, templates found here override the
    // embedded defaults. A base.html here also overrides the embedded
    // base layout (but LayoutTemplatePath takes priority when both are set).
    TemplatesDir string

    // ListAll disables pagination and displays every published post on a
    // single page. When true, query params ?page, ?limit, and ?offset are
    // ignored on list pages.
    ListAll bool

    // Optional metadata used for WXR export/import and SEO.
    SiteTitle                string
    SiteDescription          string
    SiteURL                  string
    SiteLanguage             string
    DefaultAuthorLogin       string
    DefaultAuthorDisplayName string
    ImportAuthorID           int
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
    db, err := sqlx.Open("sqlite3", "blog.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

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
handler, err := blog.NewHandler(blog.Config{
    Store:       blog.NewSQLXStore(db),
    RoutePrefix: "/blog",
    AdminAuthMiddleware: func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.Header.Get("Authorization")
            if !isValidToken(token) {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    },
})
```

## AI Features

Spore supports two AI tiers — **Smart** and **Dumb** — configured in the admin AI Settings page. The dumb tier is used for background automation (auto-tagging, auto-descriptions, spam checks) and is intended for fast, inexpensive models like GPT-4o-mini or Gemini Flash. The smart tier powers the interactive AI chat editor.

Supported providers: **OpenAI**, **Anthropic**, **Gemini**, and **Ollama**. If only one tier is configured, the dumb tier falls back to the smart tier.

### Auto-Tagging

Tags are generated asynchronously whenever a post is created or substantially updated (≥10% content change or 50+ character difference).

1. **Post saved** — the system fires an async background task.
2. **AI analyzes content** — the dumb AI receives the title and a plain-text excerpt (up to 3,000 characters) and returns 5–8 lowercase tags.
3. **Tags stored** — tags are saved in the post's `attrs.tags`. Existing tags are replaced.
4. **Tags displayed** — tags appear as clickable pills on both the listing and detail pages.

If no dumb AI is configured, posts are saved without tags.

#### Tag Filtering

Public visitors can filter by tag:

```
GET <prefix>/tag/{tagSlug}
```

For example, `/blog/tag/golang` shows all posts tagged "golang".

### Auto-Descriptions

When a post is saved without a `meta_description`, or after a WXR import, the dumb AI is asked to generate a concise SEO meta description from the post content. The description is stored on the post and used for `<meta>` tags, OpenGraph, and JSON-LD.

### AI Chat

The admin editor exposes an interactive AI chat endpoint (`POST /admin/api/ai/chat`). Authors can send a query along with the current post content, and the AI returns rewritten markdown plus optional notes. The Gemini provider also supports web search grounding.

### AI Spam Checks

If a dumb AI provider is configured, new comments are created in a **pending** state and asynchronously classified. Comments flagged as spam are automatically rejected and hidden from the public view. Rejected comments remain visible in the admin moderation queue for manual review.

## Related Posts

Each blog post page includes a "Related Posts" section at the bottom (above comments). Related posts are determined by counting shared tags — posts with the most tags in common appear first.

- **Automatic** — no manual curation needed.
- **Visual cards** — each card shows the first image in the post content (or a placeholder icon), the title, a plain-text excerpt (up to 150 characters), and tag pills.
- **Up to 4 posts** displayed.
- **Fallback** — if a post has no tags, the system picks 4 deterministic random posts as a fallback instead of showing nothing.
- **Responsive grid** — collapses to a single column on mobile.

### How Similarity Is Calculated

```text
1) Load all published posts and their tag lists
2) Score each candidate by number of shared tags
3) Sort by score DESC, then published_at DESC
4) Keep top 4
```

## Comments

Spore includes a built-in commenting system. Visitors can leave comments without logging in, reply one level deep, and @mention other commenters. Users can edit or delete their own comments later as long as they are using the same browser (identity is tracked via a `blog_commenter_token` cookie with a 1-year expiry).

Validation rules: author name 2–60 characters, content 1–2,000 characters. Replies to replies are rejected (only one level of nesting is supported).

The admin UI provides a moderation queue where you can approve, hide, reject, or delete comments. Comments can be globally enabled or disabled from the Settings page.

## Date Display

Posts can show either absolute dates ("Published Jan 2, 2006") or approximate dates ("Published 3 days ago"). This is configurable in the admin Settings page via the `date_display` field. The default is `"absolute"`.

## RSS Feed

Spore automatically serves an RSS 2.0 feed at `<prefix>/feed` (e.g., `/blog/feed`). The feed includes the 20 most recent published posts with titles, permalinks, descriptions, publication dates, and tags as categories.

A `<link rel="alternate">` autodiscovery tag is automatically injected into every public page's `<head>`, so RSS readers can find the feed by visiting any blog page.

The feed uses `SiteURL`, `SiteTitle`, `SiteDescription`, and `SiteLanguage` from your `Config` for metadata. If `SiteURL` is not set, it derives the base URL from the incoming request.

## Sitemap

Spore provides a `SitemapEntries` method on the `*Handler` returned by `NewHandler`. This lets you merge blog URLs into your application's own `sitemap.xml` without serving a separate blog-specific sitemap.

```go
blogHandler, err := blog.NewHandler(blog.Config{
    Store:   store,
    SiteURL: "https://example.com",
    // ...
})

// In your sitemap.xml handler:
entries, err := blogHandler.SitemapEntries(r.Context())
for _, e := range entries {
    // e.Loc     = "https://example.com/blog/my-post"
    // e.LastMod = &time.Time{...} (or nil)
}
```

Each `SitemapEntry` has:

| Field     | Type         | Description                                          |
| --------- | ------------ | ---------------------------------------------------- |
| `Loc`     | `string`     | Absolute URL of the page                             |
| `LastMod` | `*time.Time` | Last modification time (`UpdatedAt` or `PublishedAt`)|

The method returns an entry for the blog index page plus one entry per published post.

See the demo in `cmd/demo` for a complete working example that serves `/sitemap.xml` at the site root.

## WXR Import / Export

Spore supports WordPress eXtended RSS (WXR) for data portability:

- **Export** (`GET /admin/api/wxr/export`) — generates a WXR 1.2 XML file with all posts, tags, comments, and author information. The `Config` fields `SiteTitle`, `SiteDescription`, `SiteURL`, `SiteLanguage`, `DefaultAuthorLogin`, and `DefaultAuthorDisplayName` populate the export metadata.
- **Import** (`POST /admin/api/wxr/import`) — accepts a WXR XML file (multipart form field `file` or raw XML body). Posts are deduplicated by slug, HTML content is converted to Markdown, and comments (including one-level replies) are imported. After import, the system automatically queues background tasks to generate tags, descriptions, and download/re-host external images.

## Implementing the BlogStore Interface

Spore uses a minimal, entity-based store interface. All domain objects — posts, comments, tasks, and settings — are stored as `Entity` values with flexible JSON attributes.

```go
type BlogStore interface {
    // Migrate applies any pending schema changes for the store.
    Migrate(ctx context.Context) error

    // Save creates or updates an entity (upsert by ID).
    Save(ctx context.Context, e *Entity) error

    // Get retrieves a single entity by its ID.
    Get(ctx context.Context, id string) (*Entity, error)

    // Find retrieves entities matching a query.
    Find(ctx context.Context, q Query) ([]*Entity, error)

    // Delete removes an entity by ID.
    Delete(ctx context.Context, id string) error
}
```

### Using the Built-in SQLX Store

The package includes a ready-to-use SQLX implementation. Migrations are applied automatically when `blog.NewHandler` is called, so you do not need to run them manually.

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

The package exports the schema constant used by the built-in migrations:

```sql
-- blog.SchemaBlogEntities
CREATE TABLE IF NOT EXISTS blog_entities (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    slug TEXT NULL,
    status TEXT NULL,
    owner_id TEXT NULL,
    parent_id TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL,
    published_at TIMESTAMP NULL,
    attributes JSON NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind ON blog_entities(kind);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_slug ON blog_entities(kind, slug);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_status ON blog_entities(kind, status);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_owner ON blog_entities(kind, owner_id);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_parent ON blog_entities(kind, parent_id);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_created ON blog_entities(kind, created_at);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_published ON blog_entities(kind, published_at);
```

### Custom Store Implementation

Here's a minimal in-memory store (entity-based):

```go
type memoryStore struct {
    mu       sync.RWMutex
    entities map[string]*blog.Entity
}

func (m *memoryStore) Migrate(ctx context.Context) error { return nil }

func (m *memoryStore) Save(ctx context.Context, e *blog.Entity) error {
    // Upsert into map, set IDs/timestamps as needed
}

func (m *memoryStore) Get(ctx context.Context, id string) (*blog.Entity, error) {
    // Return entity by ID
}

func (m *memoryStore) Find(ctx context.Context, q blog.Query) ([]*blog.Entity, error) {
    // Filter in-memory by Kind + Filter, then order/limit
}

func (m *memoryStore) Delete(ctx context.Context, id string) error {
    // Delete by ID
}
```

## Image Storage

Spore supports optional image uploads through the `ImageStore` interface:

```go
type ImageStore interface {
    // SaveImage stores an image and returns its URL/path for retrieval.
    SaveImage(ctx context.Context, id, filename, contentType string, reader io.Reader) (url string, err error)

    // GetImage retrieves an image by its ID.
    GetImage(ctx context.Context, id string) (contentType string, reader io.ReadCloser, err error)

    // DeleteImage removes an image by its ID.
    DeleteImage(ctx context.Context, id string) error
}
```

### Using the Built-in File Image Store

```go
imageStore, err := blog.NewFileImageStore(
    "uploads",                    // Directory to store images
    "/blog/images",               // URL prefix for serving images
)
if err != nil {
    log.Fatal(err)
}

handler, err := blog.NewHandler(blog.Config{
    Store:      store,
    ImageStore: imageStore,
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
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <title>
      {{if .Post}}{{.Post.Title}} | My Site{{else}}Blog | My Site{{end}}
    </title>
    {{if .Post}}
    <meta name="description" content="{{.Post.MetaDescription}}" />
    {{if .CanonicalURL}}<link rel="canonical" href="{{.CanonicalURL}}" />{{end}}
    <meta property="og:type" content="article" />
    <meta property="og:title" content="{{.Post.Title}}" />
    <meta property="og:description" content="{{.Post.MetaDescription}}" />
    {{if .CanonicalURL}}<meta property="og:url" content="{{.CanonicalURL}}" />{{end}}
    {{if .FirstImage}}<meta property="og:image" content="{{.FirstImage}}" />{{end}}
    {{else}}
    {{if .SiteDescription}}<meta name="description" content="{{.SiteDescription}}" />{{end}}
    {{end}} {{range .CustomCSS}}
    <link rel="stylesheet" href="{{.}}" />
    {{end}}
    <link rel="stylesheet" href="/static/styles.css" />
  </head>
  <body>
    <nav>
      <a href="/">Home</a>
      <a href="{{.RoutePrefix}}/">Blog</a>
    </nav>
    <main>{{block "content" .}}{{end}}</main>
    <footer>&copy; 2026 My Company</footer>
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

### 3. Custom Template Directory

For full control over `list.html` or `post.html`, set `TemplatesDir` to a directory containing your replacement templates:

```go
handler, err := blog.NewHandler(blog.Config{
    Store:        store,
    TemplatesDir: "templates/blog",
})
```

Spore looks for templates in this order:

1. **`TemplatesDir`** — if the file exists there, use it.
2. **Embedded defaults** — fall back to the built-in template.

For the base layout, priority is:

1. `LayoutTemplatePath` (explicit file path)
2. `TemplatesDir/base.html`
3. Embedded `base.html`

This means you can override just `list.html` without touching the base layout, or override everything.

### Template Data

Templates receive the following data:

**List Page (`list.html`):**

```go
map[string]any{
    "Posts":           []PostSummary, // Posts with FirstImage and Excerpt populated
    "AllPosts":        []Post,        // Raw Post slice (no FirstImage/Excerpt)
    "Pagination":      *Pagination,   // Page navigation (nil when ListAll is true)
    "RoutePrefix":     string,        // e.g., "/blog"
    "CustomCSS":       []string,      // Custom CSS URLs
    "TagSlug":         string,        // Set when filtering by tag (e.g., "golang")
    "DateDisplay":     string,        // "absolute" or "approximate"
    "Limit":           int,           // Current page size
    "NextOffset":      int,           // Offset for infinite-scroll continuation
    "SiteTitle":       string,        // From Config.SiteTitle
    "SiteURL":         string,        // From Config.SiteURL
    "SiteDescription": string,        // From Config.SiteDescription
    "CanonicalURL":    string,        // Full canonical URL for the page
    "FeedURL":         string,        // Absolute URL of the RSS feed
}
```

**Post Page (`post.html`):**

```go
map[string]any{
    "Post":            *Post,         // The full post object (with Tags populated)
    "RoutePrefix":     string,        // e.g., "/blog"
    "CustomCSS":       []string,      // Custom CSS URLs
    "CommentsEnabled": bool,          // Whether comments are enabled
    "RelatedPosts":    []RelatedPost, // Up to 4 related posts with images/excerpts
    "DateDisplay":     string,        // "absolute" or "approximate"
    "SiteTitle":       string,        // From Config.SiteTitle
    "SiteURL":         string,        // From Config.SiteURL
    "SiteDescription": string,        // From Config.SiteDescription
    "CanonicalURL":    string,        // Full canonical URL for the post
    "FirstImage":      string,        // Absolute URL of first image in post (for og:image)
    "FeedURL":        string,        // Absolute URL of the RSS feed
}
```

Each `PostSummary` contains all `Post` fields plus:

| Field        | Type     | Description                                              |
| ------------ | -------- | -------------------------------------------------------- |
| `FirstImage` | `string` | URL of the first `<img>` found in the rendered HTML      |
| `Excerpt`    | `string` | Plain-text excerpt (up to 300 characters) from the body  |

The `Pagination` object:

| Field          | Type     | Description                                      |
| -------------- | -------- | ------------------------------------------------ |
| `CurrentPage`  | `int`    | The 1-based current page number                  |
| `TotalPages`   | `int`    | Total number of pages                            |
| `PrevPageURL`  | `string` | URL to the previous page (empty on page 1)       |
| `NextPageURL`  | `string` | URL to the next page (empty on the last page)    |

### Available Template Functions

- `safeHTML` — renders HTML content without escaping (use for `Post.ContentHTML`)
- `formatPublishedDate` — formats a `*time.Time` according to the current `DateDisplay` setting
- `rfc3339` — formats a `*time.Time` as an RFC 3339 string (used in SEO meta tags)
- `jsonEscape` — escapes a string for safe use inside JSON literals
- `truncate` — truncates a string to a maximum number of characters, appending `…` if shortened: `{{truncate .Excerpt 100}}`
- `stripHTML` — removes all HTML tags from a string, returning plain text: `{{stripHTML .Post.ContentHTML}}`
- `now` — returns the current `time.Time`, useful for copyright years or "last updated" displays: `{{now.Year}}`

### Example: Custom Card Layout

Here is a minimal `list.html` that uses `PostSummary` data and pagination to build a card grid:

```html
{{define "content"}}
<div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 24px">
  {{range .Posts}}
  <article style="border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden">
    {{if .FirstImage}}
    <a href="{{$.RoutePrefix}}/{{.Slug}}">
      <img src="{{.FirstImage}}" alt="{{.Title}}"
           style="width: 100%; height: 180px; object-fit: cover">
    </a>
    {{end}}
    <div style="padding: 16px">
      <h3><a href="{{$.RoutePrefix}}/{{.Slug}}">{{.Title}}</a></h3>
      <p>{{truncate .Excerpt 120}}</p>
    </div>
  </article>
  {{end}}
</div>

{{if .Pagination}}
<nav style="display: flex; justify-content: space-between; padding: 24px 0">
  {{if .Pagination.PrevPageURL}}<a href="{{.Pagination.PrevPageURL}}">&larr; Previous</a>{{else}}<span></span>{{end}}
  <span>Page {{.Pagination.CurrentPage}} of {{.Pagination.TotalPages}}</span>
  {{if .Pagination.NextPageURL}}<a href="{{.Pagination.NextPageURL}}">Next &rarr;</a>{{else}}<span></span>{{end}}
</nav>
{{end}}
{{end}}
```

Save this as `templates/blog/list.html` and set `TemplatesDir: "templates/blog"` in your config.

### Example: Custom Post Template with Related Posts and Comments

The related posts and comment system are available as template data on every post page. When writing a custom `post.html`, iterate over `.RelatedPosts` to render related post cards, and include `{{template "comments" .}}` for the comment section:

```html
{{define "content"}}
<article style="max-width: 700px; margin: 0 auto; padding: 40px 20px">
  <h1>{{.Post.Title}}</h1>
  {{if .Post.PublishedAt}}
  <p style="color: #6b7280">{{formatPublishedDate .Post.PublishedAt $.DateDisplay}}</p>
  {{end}}
  <div>{{safeHTML .Post.ContentHTML}}</div>
</article>

{{/* Related posts — each item is a RelatedPost with FirstImage and Excerpt */}}
{{if .RelatedPosts}}
<section style="max-width: 700px; margin: 0 auto; padding: 0 20px 40px">
  <h3>Related Posts</h3>
  <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 16px">
    {{range .RelatedPosts}}
    <a href="{{$.RoutePrefix}}/{{.Slug}}" style="text-decoration: none; color: inherit; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden">
      {{if .FirstImage}}
      <img src="{{.FirstImage}}" alt="{{.Title}}" style="width: 100%; height: 120px; object-fit: cover">
      {{end}}
      <div style="padding: 12px">
        <h4 style="margin: 0 0 4px">{{.Title}}</h4>
        {{if .Excerpt}}<p style="margin: 0; color: #6b7280; font-size: 0.875rem">{{.Excerpt}}</p>{{end}}
      </div>
    </a>
    {{end}}
  </div>
</section>
{{end}}

{{/* Include the built-in comment section — form, styles, and JS */}}
{{template "comments" .}}
{{end}} {{define "post.html"}} {{template "base.html" .}} {{end}}
```

Each `RelatedPost` in `.RelatedPosts` has all the fields of a `Post` plus `FirstImage` (URL of the first image in the post) and `Excerpt` (plain-text excerpt up to 150 characters). See the [RelatedPost](#relatedpost) data model for details.

The `comments` template requires `.Post.Slug`, `.RoutePrefix`, and `.CommentsEnabled` in the template data, all of which are provided automatically on post pages.

You can also override `comments.html` itself by placing your own version in `TemplatesDir`. The template receives the same data as `post.html`.

## Pagination

List pages support a `?page=N` query parameter for server-rendered pagination. The default page size is 10 posts. You can also use `?limit=N&offset=N` for programmatic access or infinite scroll.

The `Pagination` object is included in all list template data and contains:

- `CurrentPage` — the current page (1-based)
- `TotalPages` — total pages based on published post count and limit
- `PrevPageURL` / `NextPageURL` — ready-to-use URLs (empty strings when at the boundary)

The default `list.html` template includes both infinite scroll (via JavaScript) and a `<nav>` with previous/next links that work without JavaScript.

To disable pagination entirely and list every post on a single page, set `ListAll: true` in your config:

```go
handler, err := blog.NewHandler(blog.Config{
    Store:   store,
    ListAll: true,
})
```

When `ListAll` is enabled, `.Pagination` is `nil` in the template data, the `?page`/`?limit`/`?offset` query parameters are ignored, and all published posts are fetched in a single request.

## Admin UI

The admin interface is embedded from `frontend/dist` and accessible at `<RoutePrefix>/admin`.

### Features

- Create, edit, and delete posts
- Markdown editor with live preview
- Image upload (when ImageStore is configured)
- Publish/unpublish posts
- SEO metadata editing (meta descriptions, slugs)
- AI chat for content editing (when Smart AI is configured)
- Auto-generated tags and descriptions
- Comment moderation queue
- Blog settings (comments toggle, date display mode)
- AI settings (Smart and Dumb provider configuration)
- WXR import and export
- Background task monitoring

### Building the Admin UI

The pre-built admin UI is included in the repository and embedded at compile time — most users never need to rebuild it. If you want to modify the admin frontend:

```bash
cd frontend
npm install
npm run build
```

Or use `go generate`:

```bash
go generate ./...
```

The build output in `frontend/dist` is automatically embedded when you build your Go application.

## API Reference

### Public Routes

| Method | Path                       | Description                                           |
| ------ | -------------------------- | ----------------------------------------------------- |
| GET    | `<prefix>/`                | List published posts (`?limit=N&offset=N&page=N`)     |
| GET    | `<prefix>/feed`            | RSS 2.0 feed of recent posts                          |
| GET    | `<prefix>/tag/{tagSlug}`   | List published posts filtered by tag (`?page=N`)      |
| GET    | `<prefix>/images/{id}`     | Retrieve an image by ID                               |
| GET    | `<prefix>/{slug}`          | View a single published post (includes related posts) |
| GET    | `<prefix>/{slug}/comments` | List comments for a post                              |
| POST   | `<prefix>/{slug}/comments` | Create a comment                                      |
| PUT    | `<prefix>/comments/{id}`   | Edit own comment (requires matching owner cookie)     |
| DELETE | `<prefix>/comments/{id}`   | Delete own comment (requires matching owner cookie)   |

### Admin API Routes

All admin routes are prefixed with `<prefix>/admin/api` and protected by your `AdminAuthMiddleware`.

| Method | Path                    | Description                                                |
| ------ | ----------------------- | ---------------------------------------------------------- |
| GET    | `/posts`                | List all posts (`?limit=N&offset=N`)                       |
| GET    | `/posts/{id}`           | Get a post by ID                                           |
| POST   | `/posts`                | Create a new post                                          |
| PUT    | `/posts/{id}`           | Update a post                                              |
| DELETE | `/posts/{id}`           | Delete a post                                              |
| GET    | `/settings`             | Get blog settings                                          |
| PUT    | `/settings`             | Update blog settings                                       |
| GET    | `/comments`             | List comments for moderation (`?status=&limit=N&offset=N`) |
| PUT    | `/comments/{id}/status` | Set comment status (approved/hidden/rejected)              |
| DELETE | `/comments/{id}`        | Delete a comment                                           |
| GET    | `/ai/settings`          | Get AI provider configuration                              |
| PUT    | `/ai/settings`          | Update AI provider configuration                           |
| POST   | `/ai/chat`              | Interactive AI chat for editing                            |
| GET    | `/wxr/export`           | Export all data as WXR XML                                 |
| POST   | `/wxr/import`           | Import a WXR XML file                                      |
| GET    | `/tasks`                | List background tasks                                      |
| GET    | `/images/enabled`       | Check if image upload is enabled                           |
| POST   | `/images`               | Upload an image (multipart form, field: `image`)           |
| DELETE | `/images/{id}`          | Delete an image                                            |

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
    Slug            string     `json:"slug"`
    Title           string     `json:"title"`
    ContentMarkdown string     `json:"content_markdown"`
    ContentHTML     string     `json:"content_html"`       // Auto-generated from markdown
    PublishedAt     *time.Time `json:"published_at"`       // nil = draft
    MetaDescription string     `json:"meta_description"`
    AuthorID        int        `json:"author_id"`
    Tags            []Tag      `json:"tags"`
}
```

### Tag

```go
type Tag struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Slug string `json:"slug"`
}
```

### Comment

```go
type Comment struct {
    ID             string     `json:"id"`
    PostID         string     `json:"post_id"`
    ParentID       *string    `json:"parent_id,omitempty"`
    AuthorName     string     `json:"author_name"`
    Content        string     `json:"content"`
    Status         string     `json:"status"`              // approved, pending, hidden, rejected
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      *time.Time `json:"updated_at,omitempty"`
    SpamCheckedAt  *time.Time `json:"spam_checked_at,omitempty"`
    SpamReason     *string    `json:"spam_reason,omitempty"`
}
```

### Entity

All domain objects are stored as entities with flexible JSON attributes:

```go
type Entity struct {
    ID          string     `json:"id"`
    Kind        string     `json:"kind"`           // "post", "comment", "task", "setting"
    Slug        string     `json:"slug,omitempty"`
    Status      string     `json:"status,omitempty"`
    OwnerID     string     `json:"owner_id,omitempty"`
    ParentID    string     `json:"parent_id,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   *time.Time `json:"updated_at,omitempty"`
    PublishedAt *time.Time `json:"published_at,omitempty"`
    Attrs       Attributes `json:"attrs"`           // Flexible JSON map for domain-specific fields
}
```

### Query

```go
type Query struct {
    Kind    string                 // Filter by entity kind
    Filter  map[string]interface{} // Equality filters on promoted columns or attrs
    Limit   int
    Offset  int
    OrderBy string                 // e.g., "created_at DESC"
}
```

### PostSummary

Used in the list template `.Posts` for card layouts:

```go
type PostSummary struct {
    Post
    FirstImage string  // URL of the first <img> in the post HTML
    Excerpt    string  // Plain-text excerpt (up to 300 characters)
}
```

### Pagination

Included in list template data as `.Pagination`:

```go
type Pagination struct {
    CurrentPage int    // 1-based current page
    TotalPages  int    // Total number of pages
    NextPageURL string // URL to next page (empty on last page)
    PrevPageURL string // URL to previous page (empty on page 1)
}
```

### RelatedPost

Used internally in the post detail template:

```go
type RelatedPost struct {
    Post
    FirstImage string  // URL of the first <img> found in the post HTML
    Excerpt    string  // Plain-text excerpt (up to 150 characters)
}
```

### AISettings

```go
type AISettings struct {
    Smart AIProviderSettings `json:"smart"`
    Dumb  AIProviderSettings `json:"dumb"`
}

type AIProviderSettings struct {
    Provider    string   `json:"provider"`     // "openai", "anthropic", "gemini", "ollama"
    Model       string   `json:"model"`
    APIKey      string   `json:"api_key"`
    BaseURL     string   `json:"base_url"`
    Temperature *float64 `json:"temperature"`
    MaxTokens   *int     `json:"max_tokens"`
}
```

### BlogSettings

```go
type BlogSettings struct {
    CommentsEnabled bool   `json:"comments_enabled"` // Default: true
    DateDisplay     string `json:"date_display"`     // "absolute" (default) or "approximate"
}
```

### Task

```go
type Task struct {
    ID           string     `json:"id"`
    TaskType     string     `json:"task_type"`      // "generate_tags", "generate_description", "import_images"
    Status       string     `json:"status"`         // "pending", "running", "completed", "failed"
    Payload      string     `json:"payload"`
    Result       string     `json:"result"`
    ErrorMessage *string    `json:"error_message,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
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
    db, err := sqlx.Open("sqlite3", "app.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Create stores
    blogStore := blog.NewSQLXStore(db)
    imageStore, err := blog.NewFileImageStore("uploads", "/blog/images")
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

    // Create blog handler (migrations run automatically)
    blogHandler, err := blog.NewHandler(blog.Config{
        Store:               blogStore,
        ImageStore:          imageStore,
        RoutePrefix:         "/blog",
        AdminAuthMiddleware: adminAuth,
        LayoutTemplatePath:  "templates/layout.html",
        CustomCSSURLs:       []string{"/static/blog.css"},
        SiteTitle:           "My Company Blog",
        SiteDescription:     "Insights and updates from My Company",
        SiteURL:             "https://example.com",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Set up main router
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    r.Handle("/static/*", http.StripPrefix("/static/",
        http.FileServer(http.Dir("static"))))
    r.Handle("/uploads/*", http.StripPrefix("/uploads/",
        http.FileServer(http.Dir("uploads"))))

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

## License

MIT License - see LICENSE file for details.
