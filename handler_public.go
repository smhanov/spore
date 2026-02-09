package blog

import (
	"hash/fnv"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// firstImageRe matches the first <img> tag and extracts the src.
var firstImageRe = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

func (s *service) mountPublicRoutes(r chi.Router) {
	r.Get("/", s.handleListPosts)
	r.Get("/feed", s.handleRSSFeed)
	r.Get("/tag/{tagSlug}", s.handleListPostsByTag)
	r.Get("/api/images/{id}", s.handleGetImage)
	s.mountCommentRoutes(r)
	r.Get("/*", s.handleViewPost)
}

func (s *service) handleListPosts(w http.ResponseWriter, r *http.Request) {
	limit := 10
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	posts, err := s.store.ListPublishedPosts(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	data := map[string]any{
		"Posts":           posts,
		"RoutePrefix":     s.routePrefix,
		"CustomCSS":       s.cfg.CustomCSSURLs,
		"DateDisplay":     settings.DateDisplay,
		"Limit":           limit,
		"NextOffset":      offset + len(posts),
		"SiteTitle":       s.effectiveTitle(settings),
		"SiteURL":         s.cfg.SiteURL,
		"SiteDescription": s.effectiveDescription(settings),
		"CanonicalURL":    s.canonicalURL("/"),
		"FeedURL":         s.canonicalURL("/feed"),
	}

	s.executeTemplate(w, "list.html", data)
}

func (s *service) handleListPostsByTag(w http.ResponseWriter, r *http.Request) {
	tagSlug := chi.URLParam(r, "tagSlug")
	limit := 10
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	posts, err := s.store.ListPostsByTag(r.Context(), tagSlug, limit, offset)
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	data := map[string]any{
		"Posts":           posts,
		"RoutePrefix":     s.routePrefix,
		"CustomCSS":       s.cfg.CustomCSSURLs,
		"TagSlug":         tagSlug,
		"DateDisplay":     settings.DateDisplay,
		"Limit":           limit,
		"NextOffset":      offset + len(posts),
		"SiteTitle":       s.effectiveTitle(settings),
		"SiteURL":         s.cfg.SiteURL,
		"SiteDescription": s.effectiveDescription(settings),
		"CanonicalURL":    s.canonicalURL("/tag/" + tagSlug),
		"FeedURL":         s.canonicalURL("/feed"),
	}

	s.executeTemplate(w, "list.html", data)
}

// RelatedPost holds a post with its first image and excerpt for the related posts section.
type RelatedPost struct {
	Post
	FirstImage string
	Excerpt    string
}

func (s *service) handleViewPost(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "*")
	post, err := s.store.GetPublishedPostBySlug(r.Context(), slug)
	if err != nil {
		http.Error(w, "failed to load post", http.StatusInternalServerError)
		return
	}
	if post == nil {
		if s.cfg.StaticFilePath != "" {
			fullPath := filepath.Join(s.cfg.StaticFilePath, slug)
			// Minimal security check to ensure we stay within StaticFilePath
			cleaned := filepath.Clean(fullPath)
			absStatic, _ := filepath.Abs(s.cfg.StaticFilePath)
			absRequested, _ := filepath.Abs(cleaned)

			if strings.HasPrefix(absRequested, absStatic) {
				if info, err := os.Stat(absRequested); err == nil && !info.IsDir() {
					http.ServeFile(w, r, absRequested)
					return
				}
			}
		}

		http.NotFound(w, r)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	// Load related posts
	var finalPosts []Post
	targetCount := 5

	// 1. Try to get distinct related posts
	rawRelated, err := s.store.GetRelatedPosts(r.Context(), post.ID, targetCount)
	if err == nil {
		finalPosts = append(finalPosts, rawRelated...)
	}

	// 2. If we need more, fill with random recent posts
	if len(finalPosts) < targetCount {
		needed := targetCount - len(finalPosts)
		fallback, err := s.store.ListPublishedPosts(r.Context(), 50, 0)
		if err == nil && len(fallback) > 0 {
			// Build set of exclusion IDs (current post + already picked related)
			exclude := make(map[string]bool)
			exclude[post.ID] = true
			for _, p := range finalPosts {
				exclude[p.ID] = true
			}

			// Filter fallback candidates
			var candidates []Post
			for _, p := range fallback {
				if !exclude[p.ID] {
					candidates = append(candidates, p)
				}
			}

			// Pick deterministic random ones from candidates
			// We pass "" as excludeID because we already filtered
			picks := pickDeterministicPosts(candidates, "", needed, seedForPost(post))
			finalPosts = append(finalPosts, picks...)
		}
	}

	// 3. Convert to RelatedPost View Models
	var relatedPosts []RelatedPost
	if len(finalPosts) > 0 {
		if err := s.store.LoadPostsTags(r.Context(), finalPosts); err == nil {
			for _, rp := range finalPosts {
				relatedPosts = append(relatedPosts, RelatedPost{
					Post:       rp,
					FirstImage: extractFirstImage(rp.ContentHTML),
					Excerpt:    trimToLength(markdownToPlainText(rp.ContentMarkdown), 150),
				})
			}
		}
	}

	firstImage := extractFirstImage(post.ContentHTML)

	data := map[string]any{
		"Post":            post,
		"RoutePrefix":     s.routePrefix,
		"CustomCSS":       s.cfg.CustomCSSURLs,
		"CommentsEnabled": settings.CommentsEnabled,
		"RelatedPosts":    relatedPosts,
		"DateDisplay":     settings.DateDisplay,
		"SiteTitle":       s.effectiveTitle(settings),
		"SiteURL":         s.cfg.SiteURL,
		"SiteDescription": s.effectiveDescription(settings),
		"CanonicalURL":    s.canonicalURL("/" + post.Slug),
		"FirstImage":      s.resolveImageURL(firstImage),
		"FeedURL":         s.canonicalURL("/feed"),
	}

	s.executeTemplate(w, "post.html", data)
}

// extractFirstImage pulls the first image URL from HTML content.
func extractFirstImage(html string) string {
	matches := firstImageRe.FindStringSubmatch(html)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func seedForPost(post *Post) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(post.ID))
	if post.Slug != "" {
		_, _ = h.Write([]byte(post.Slug))
	}
	return int64(h.Sum64())
}

func pickDeterministicPosts(posts []Post, excludeID string, limit int, seed int64) []Post {
	if limit <= 0 {
		return []Post{}
	}
	pool := make([]Post, 0, len(posts))
	for _, p := range posts {
		if p.ID == excludeID {
			continue
		}
		pool = append(pool, p)
	}
	if len(pool) <= limit {
		return pool
	}

	rng := rand.New(rand.NewSource(seed))
	for i := len(pool) - 1; i > 0; i -= 1 {
		j := rng.Intn(i + 1)
		pool[i], pool[j] = pool[j], pool[i]
	}
	return pool[:limit]
}

// effectiveTitle returns the stored blog title if set, falling back to config.
func (s *service) effectiveTitle(settings BlogSettings) string {
	if settings.Title != "" {
		return settings.Title
	}
	return s.cfg.SiteTitle
}

// effectiveDescription returns the stored blog description if set, falling back to config.
func (s *service) effectiveDescription(settings BlogSettings) string {
	if settings.Description != "" {
		return settings.Description
	}
	return s.cfg.SiteDescription
}

func (s *service) executeTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tpl, ok := s.templates[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := tpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
}

// canonicalURL builds a full canonical URL by joining SiteURL + routePrefix + path.
func (s *service) canonicalURL(path string) string {
	if s.cfg.SiteURL == "" {
		return ""
	}
	base := strings.TrimSuffix(s.cfg.SiteURL, "/")
	return base + s.routePrefix + path
}

// resolveImageURL converts a relative image URL to an absolute URL using SiteURL.
func (s *service) resolveImageURL(img string) string {
	if img == "" {
		return ""
	}
	if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
		return img
	}
	if s.cfg.SiteURL == "" {
		return img
	}
	base := strings.TrimSuffix(s.cfg.SiteURL, "/")
	if !strings.HasPrefix(img, "/") {
		img = "/" + img
	}
	return base + img
}
