package blog

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *service) mountAdminRoutes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Get("/posts", s.handleAdminListPosts)
		r.Get("/posts/{id}", s.handleAdminGetPost)
		r.Post("/posts", s.handleAdminCreatePost)
		r.Put("/posts/{id}", s.handleAdminUpdatePost)
		r.Delete("/posts/{id}", s.handleAdminDeletePost)

		r.Get("/settings", s.handleAdminGetBlogSettings)
		r.Put("/settings", s.handleAdminUpdateBlogSettings)

		r.Get("/comments", s.handleAdminListComments)
		r.Put("/comments/{id}/status", s.handleAdminUpdateCommentStatus)
		r.Delete("/comments/{id}", s.handleAdminDeleteComment)

		r.Get("/notifications/vapid-key", s.handleAdminGetNotificationPublicKey)
		r.Post("/notifications/subscribe", s.handleAdminSubscribeNotifications)
		r.Delete("/notifications/subscribe", s.handleAdminUnsubscribeNotifications)

		r.Get("/ai/settings", s.handleAdminGetAISettings)
		r.Put("/ai/settings", s.handleAdminUpdateAISettings)
		r.Post("/ai/chat", s.handleAdminAIChat)

		r.Get("/wxr/export", s.handleAdminExportWXR)
		r.Post("/wxr/import", s.handleAdminImportWXR)

		r.Get("/tasks", s.handleAdminListTasks)

		// Image endpoints (only available if ImageStore is configured)
		r.Get("/images/enabled", s.handleImagesEnabled)
		r.Post("/images", s.handleUploadImage)
		r.Delete("/images/{id}", s.handleDeleteImage)
	})

	distFS, err := fs.Sub(s.adminFS, "frontend/dist")
	if err != nil {
		distFS = s.adminFS
	}
	r.Get("/*", s.serveAdminSPA(distFS))
	// Root fallback
	r.Get("/", s.serveAdminSPA(distFS))
}

func (s *service) handleAdminListPosts(w http.ResponseWriter, r *http.Request) {
	limit := 0
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	posts, err := s.store.ListAllPosts(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}
	writeJSON(w, posts)
}

func (s *service) handleAdminGetPost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	post, err := s.store.GetPostByID(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load post", http.StatusInternalServerError)
		return
	}
	if post == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, post)
}

func (s *service) handleAdminCreatePost(w http.ResponseWriter, r *http.Request) {
	var p Post
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if p.ID == "" {
		p.ID = generateID()
	}
	// Convert markdown to HTML
	if p.ContentMarkdown != "" {
		html, err := markdownToHTMLUnsafe(p.ContentMarkdown)
		if err != nil {
			http.Error(w, "failed to convert markdown", http.StatusInternalServerError)
			return
		}
		p.ContentHTML = html
	}
	if err := s.store.CreatePost(r.Context(), &p); err != nil {
		http.Error(w, "failed to create post", http.StatusInternalServerError)
		return
	}
	s.queuePostProcessing("post saved")
	writeJSON(w, p)
}

func (s *service) handleAdminUpdatePost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p Post
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if p.ID == "" {
		p.ID = id
	}
	if p.ID != id {
		http.Error(w, "id mismatch", http.StatusBadRequest)
		return
	}

	// Convert markdown to HTML
	if p.ContentMarkdown != "" {
		html, err := markdownToHTMLUnsafe(p.ContentMarkdown)
		if err != nil {
			http.Error(w, "failed to convert markdown", http.StatusInternalServerError)
			return
		}
		p.ContentHTML = html
	}
	if err := s.store.UpdatePost(r.Context(), &p); err != nil {
		http.Error(w, "failed to update post", http.StatusInternalServerError)
		return
	}
	s.queuePostProcessing("post saved")

	writeJSON(w, p)
}

func (s *service) handleAdminDeletePost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeletePost(r.Context(), id); err != nil {
		http.Error(w, "failed to delete post", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *service) handleImagesEnabled(w http.ResponseWriter, r *http.Request) {
	enabled := s.cfg.ImageStore != nil
	writeJSON(w, map[string]bool{"enabled": enabled})
}

func (s *service) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ImageStore == nil {
		http.Error(w, "image storage not configured", http.StatusNotImplemented)
		return
	}

	// Parse multipart form with 32MB max memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "no image file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id := generateID()
	storeURL, err := s.cfg.ImageStore.SaveImage(r.Context(), id, header.Filename, contentType, file)
	if err != nil {
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	// Extract the filename from the store URL to build the public-facing URL.
	savedFilename := path.Base(storeURL)
	savedID := savedFilename
	if ext := path.Ext(savedFilename); ext != "" {
		savedID = strings.TrimSuffix(savedFilename, ext)
	}
	publicURL := s.routePrefix + "/images/" + savedFilename

	writeJSON(w, map[string]string{
		"id":  savedID,
		"url": publicURL,
	})
}

func (s *service) handleGetImage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ImageStore == nil {
		http.Error(w, "image storage not configured", http.StatusNotImplemented)
		return
	}

	id := chi.URLParam(r, "id")
	contentType, reader, err := s.cfg.ImageStore.GetImage(r.Context(), id)
	if err != nil {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	io.Copy(w, reader)
}

func (s *service) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ImageStore == nil {
		http.Error(w, "image storage not configured", http.StatusNotImplemented)
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.cfg.ImageStore.DeleteImage(r.Context(), id); err != nil {
		http.Error(w, "failed to delete image", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *service) serveAdminSPA(dist fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, s.routePrefix+"/admin")
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			p = "index.html"
		}

		if file, err := dist.Open(p); err == nil {
			defer file.Close()
			if info, err := file.Stat(); err == nil && !info.IsDir() {
				http.ServeContent(w, r, p, info.ModTime(), file.(io.ReadSeeker))
				return
			}
		}

		fallback, err := dist.Open("index.html")
		if err != nil {
			http.Error(w, "admin ui not built", http.StatusInternalServerError)
			return
		}
		defer fallback.Close()
		info, _ := fallback.Stat()
		http.ServeContent(w, r, path.Base("index.html"), info.ModTime(), fallback.(io.ReadSeeker))
	}
}

func (s *service) handleAdminListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListRecentTasks(r.Context(), 50)
	if err != nil {
		http.Error(w, "failed to list tasks", http.StatusInternalServerError)
		return
	}
	writeJSON(w, tasks)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "json encode error", http.StatusInternalServerError)
	}
}
