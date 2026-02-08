package blog

import (
	"hash/fnv"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// firstImageRe matches the first <img> tag and extracts the src.
var firstImageRe = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

func (s *service) mountPublicRoutes(r chi.Router) {
	r.Get("/", s.handleListPosts)
	r.Get("/tag/{tagSlug}", s.handleListPostsByTag)
	r.Get("/{slug}", s.handleViewPost)
	s.mountCommentRoutes(r)
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

	posts, err := s.cfg.Store.ListPublishedPosts(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.cfg.Store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	data := map[string]any{
		"Posts":       posts,
		"RoutePrefix": s.routePrefix,
		"CustomCSS":   s.cfg.CustomCSSURLs,
		"DateDisplay": settings.DateDisplay,
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

	posts, err := s.cfg.Store.ListPostsByTag(r.Context(), tagSlug, limit, offset)
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.cfg.Store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	data := map[string]any{
		"Posts":       posts,
		"RoutePrefix": s.routePrefix,
		"CustomCSS":   s.cfg.CustomCSSURLs,
		"TagSlug":     tagSlug,
		"DateDisplay": settings.DateDisplay,
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
	slug := chi.URLParam(r, "slug")
	post, err := s.cfg.Store.GetPublishedPostBySlug(r.Context(), slug)
	if err != nil {
		http.Error(w, "failed to load post", http.StatusInternalServerError)
		return
	}
	if post == nil {
		http.NotFound(w, r)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.cfg.Store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	// Load related posts
	var relatedPosts []RelatedPost
	rawRelated, err := s.cfg.Store.GetRelatedPosts(r.Context(), post.ID, 4)
	if err == nil && len(rawRelated) > 0 {
		if err := s.cfg.Store.LoadPostsTags(r.Context(), rawRelated); err == nil {
			for _, rp := range rawRelated {
				relatedPosts = append(relatedPosts, RelatedPost{
					Post:       rp,
					FirstImage: extractFirstImage(rp.ContentHTML),
					Excerpt:    trimToLength(markdownToPlainText(rp.ContentMarkdown), 150),
				})
			}
		}
	} else {
		fallback, err := s.cfg.Store.ListPublishedPosts(r.Context(), 50, 0)
		if err == nil && len(fallback) > 0 {
			picks := pickDeterministicPosts(fallback, post.ID, 4, seedForPost(post))
			if len(picks) > 0 {
				if err := s.cfg.Store.LoadPostsTags(r.Context(), picks); err == nil {
					for _, rp := range picks {
						relatedPosts = append(relatedPosts, RelatedPost{
							Post:       rp,
							FirstImage: extractFirstImage(rp.ContentHTML),
							Excerpt:    trimToLength(markdownToPlainText(rp.ContentMarkdown), 150),
						})
					}
				}
			}
		}
	}

	data := map[string]any{
		"Post":            post,
		"RoutePrefix":     s.routePrefix,
		"CustomCSS":       s.cfg.CustomCSSURLs,
		"CommentsEnabled": settings.CommentsEnabled,
		"RelatedPosts":    relatedPosts,
		"DateDisplay":     settings.DateDisplay,
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
