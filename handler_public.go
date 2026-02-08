package blog

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *service) mountPublicRoutes(r chi.Router) {
	r.Get("/", s.handleListPosts)
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

	data := map[string]any{
		"Posts":       posts,
		"RoutePrefix": s.routePrefix,
		"CustomCSS":   s.cfg.CustomCSSURLs,
	}

	s.executeTemplate(w, "list.html", data)
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

	commentsEnabled := true
	if settings, err := s.cfg.Store.GetBlogSettings(r.Context()); err == nil && settings != nil {
		commentsEnabled = settings.CommentsEnabled
	}

	data := map[string]any{
		"Post":            post,
		"RoutePrefix":     s.routePrefix,
		"CustomCSS":       s.cfg.CustomCSSURLs,
		"CommentsEnabled": commentsEnabled,
	}

	s.executeTemplate(w, "post.html", data)
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
