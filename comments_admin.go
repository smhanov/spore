package blog

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type blogSettingsPayload struct {
	CommentsEnabled      bool   `json:"comments_enabled"`
	NotificationsEnabled bool   `json:"notifications_enabled"`
	VAPIDPublicKey       string `json:"vapid_public_key"`
	VAPIDPrivateKey      string `json:"vapid_private_key"`
	VAPIDSubscriber      string `json:"vapid_subscriber"`
	DateDisplay          string `json:"date_display"`
	Title                string `json:"title"`
	Description          string `json:"description"`
}

func (s *service) handleAdminGetBlogSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetBlogSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	if settings == nil {
		resolved := resolveBlogSettings(nil)
		settings = &resolved
	} else {
		resolved := resolveBlogSettings(settings)
		settings = &resolved
	}
	notificationsEnabled, err := s.store.GetNotificationsEnabled(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	publicKey, privateKey, subscriber, err := s.ensurePushSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"comments_enabled":      settings.CommentsEnabled,
		"notifications_enabled": notificationsEnabled,
		"vapid_public_key":      publicKey,
		"vapid_private_key":     privateKey,
		"vapid_subscriber":      subscriber,
		"date_display":          settings.DateDisplay,
		"title":                 settings.Title,
		"description":           settings.Description,
	})
}

func (s *service) handleAdminUpdateBlogSettings(w http.ResponseWriter, r *http.Request) {
	var payload blogSettingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	settings := &BlogSettings{
		CommentsEnabled: payload.CommentsEnabled,
		DateDisplay:     normalizeDateDisplay(payload.DateDisplay),
		Title:           payload.Title,
		Description:     payload.Description,
	}
	if err := s.store.UpdateBlogSettings(r.Context(), settings); err != nil {
		http.Error(w, "failed to update settings", http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdateNotificationsEnabled(r.Context(), payload.NotificationsEnabled); err != nil {
		http.Error(w, "failed to update settings", http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdateVAPIDSettings(r.Context(), payload.VAPIDPublicKey, payload.VAPIDPrivateKey, payload.VAPIDSubscriber); err != nil {
		http.Error(w, "failed to update settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"comments_enabled":      settings.CommentsEnabled,
		"notifications_enabled": payload.NotificationsEnabled,
		"vapid_public_key":      strings.TrimSpace(payload.VAPIDPublicKey),
		"vapid_private_key":     strings.TrimSpace(payload.VAPIDPrivateKey),
		"vapid_subscriber":      strings.TrimSpace(payload.VAPIDSubscriber),
		"date_display":          settings.DateDisplay,
		"title":                 settings.Title,
		"description":           settings.Description,
	})
}

func (s *service) handleAdminListComments(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	comments, err := s.store.ListCommentsForModeration(r.Context(), status, limit, offset)
	if err != nil {
		http.Error(w, "failed to list comments", http.StatusInternalServerError)
		return
	}
	writeJSON(w, comments)
}

func (s *service) handleAdminUpdateCommentStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	status := strings.TrimSpace(strings.ToLower(payload.Status))
	switch status {
	case "approved", "hidden", "rejected":
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateCommentStatus(r.Context(), id, status, nil); err != nil {
		http.Error(w, "failed to update status", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *service) handleAdminDeleteComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteCommentByID(r.Context(), id); err != nil {
		http.Error(w, "failed to delete comment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
