package blog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const commentOwnerCookie = "blog_commenter_token"

type createCommentRequest struct {
	AuthorName string  `json:"author_name"`
	Content    string  `json:"content"`
	ParentID   *string `json:"parent_id"`
}

type commentResponse struct {
	ID         string            `json:"id"`
	ParentID   *string           `json:"parent_id,omitempty"`
	AuthorName string            `json:"author_name"`
	Content    string            `json:"content"`
	Status     string            `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  *time.Time        `json:"updated_at,omitempty"`
	Owned      bool              `json:"owned"`
	Replies    []commentResponse `json:"replies,omitempty"`
}

func (s *service) mountCommentRoutes(r chi.Router) {
	r.Get("/{slug}/comments", s.handleListComments)
	r.Post("/{slug}/comments", s.handleCreateComment)
	r.Put("/comments/{id}", s.handleUpdateComment)
	r.Delete("/comments/{id}", s.handleDeleteComment)
}

func (s *service) handleListComments(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.commentsEnabled(r)
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	if !enabled {
		http.Error(w, "comments are disabled", http.StatusForbidden)
		return
	}

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

	ownerHash := s.ownerTokenHash(r)
	comments, err := s.cfg.Store.ListCommentsByPost(r.Context(), post.ID)
	if err != nil {
		http.Error(w, "failed to list comments", http.StatusInternalServerError)
		return
	}

	response := buildCommentThread(comments, ownerHash)
	writeJSON(w, response)
}

func (s *service) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.commentsEnabled(r)
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	if !enabled {
		http.Error(w, "comments are disabled", http.StatusForbidden)
		return
	}

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

	var payload createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	payload.AuthorName = strings.TrimSpace(payload.AuthorName)
	payload.Content = strings.TrimSpace(payload.Content)
	if len(payload.AuthorName) < 2 || len(payload.AuthorName) > 60 {
		http.Error(w, "name must be 2-60 characters", http.StatusBadRequest)
		return
	}
	if len(payload.Content) < 1 || len(payload.Content) > 2000 {
		http.Error(w, "comment must be 1-2000 characters", http.StatusBadRequest)
		return
	}

	if payload.ParentID != nil {
		parent, err := s.cfg.Store.GetCommentByID(r.Context(), *payload.ParentID)
		if err != nil {
			http.Error(w, "failed to load parent", http.StatusInternalServerError)
			return
		}
		if parent == nil || parent.PostID != post.ID || parent.ParentID != nil || parent.Status != "approved" {
			http.Error(w, "invalid parent comment", http.StatusBadRequest)
			return
		}
	}

	ownerToken := s.ensureOwnerToken(w, r)
	ownerHash := hashToken(ownerToken)

	comment := Comment{
		PostID:         post.ID,
		ParentID:       payload.ParentID,
		AuthorName:     payload.AuthorName,
		Content:        payload.Content,
		OwnerTokenHash: ownerHash,
		CreatedAt:      time.Now().UTC(),
	}

	settings, err := s.cfg.Store.GetAISettings(r.Context())
	if err == nil && settings != nil && aiProviderConfigured(settings.Dumb) {
		comment.Status = "pending"
	}
	if comment.Status == "" {
		comment.Status = "approved"
	}

	if err := s.cfg.Store.CreateComment(r.Context(), &comment); err != nil {
		http.Error(w, "failed to save comment", http.StatusInternalServerError)
		return
	}

	if comment.Status == "pending" {
		go s.runCommentSpamCheck(comment, *post)
	}

	resp := commentResponse{
		ID:         comment.ID,
		ParentID:   comment.ParentID,
		AuthorName: comment.AuthorName,
		Content:    comment.Content,
		Status:     comment.Status,
		CreatedAt:  comment.CreatedAt,
		UpdatedAt:  comment.UpdatedAt,
		Owned:      true,
	}
	writeJSON(w, resp)
}

func (s *service) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ownerHash := s.ownerTokenHash(r)
	if ownerHash == "" {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}

	var payload struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	payload.Content = strings.TrimSpace(payload.Content)
	if len(payload.Content) < 1 || len(payload.Content) > 2000 {
		http.Error(w, "comment must be 1-2000 characters", http.StatusBadRequest)
		return
	}

	updated, err := s.cfg.Store.UpdateCommentContentByOwner(r.Context(), id, ownerHash, payload.Content)
	if err != nil {
		http.Error(w, "failed to update comment", http.StatusInternalServerError)
		return
	}
	if !updated {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *service) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ownerHash := s.ownerTokenHash(r)
	if ownerHash == "" {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}

	deleted, err := s.cfg.Store.DeleteCommentByOwner(r.Context(), id, ownerHash)
	if err != nil {
		http.Error(w, "failed to delete comment", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func buildCommentThread(comments []Comment, ownerHash string) []commentResponse {
	replies := map[string][]commentResponse{}
	roots := []commentResponse{}

	for _, c := range comments {
		owned := ownerHash != "" && c.OwnerTokenHash == ownerHash
		visible := c.Status == "approved" || (c.Status == "pending" && owned)
		if !visible {
			continue
		}

		status := "approved"
		if owned {
			status = c.Status
		}

		resp := commentResponse{
			ID:         c.ID,
			ParentID:   c.ParentID,
			AuthorName: c.AuthorName,
			Content:    c.Content,
			Status:     status,
			CreatedAt:  c.CreatedAt,
			UpdatedAt:  c.UpdatedAt,
			Owned:      owned,
		}

		if c.ParentID == nil {
			roots = append(roots, resp)
			continue
		}

		replies[*c.ParentID] = append(replies[*c.ParentID], resp)
	}

	for i := range roots {
		root := &roots[i]
		root.Replies = replies[root.ID]
	}

	return roots
}

func (s *service) commentsEnabled(r *http.Request) (bool, error) {
	settings, err := s.cfg.Store.GetBlogSettings(r.Context())
	if err != nil {
		return false, err
	}
	if settings == nil {
		return true, nil
	}
	return settings.CommentsEnabled, nil
}

func (s *service) ownerTokenHash(r *http.Request) string {
	cookie, err := r.Cookie(commentOwnerCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return ""
	}
	return hashToken(cookie.Value)
}

func (s *service) ensureOwnerToken(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(commentOwnerCookie)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		return cookie.Value
	}

	token := generateToken()
	http.SetCookie(w, &http.Cookie{
		Name:     commentOwnerCookie,
		Value:    token,
		Path:     s.routePrefix,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   60 * 60 * 24 * 365,
	})
	return token
}

func (s *service) runCommentSpamCheck(comment Comment, post Post) {
	ctx := context.Background()
	spam, reason, err := s.checkCommentSpam(ctx, comment, post)
	if err != nil {
		_ = s.cfg.Store.UpdateCommentStatus(ctx, comment.ID, "approved", nil)
		return
	}
	if spam {
		if strings.TrimSpace(reason) == "" {
			reason = "flagged as spam"
		}
		_ = s.cfg.Store.UpdateCommentStatus(ctx, comment.ID, "rejected", &reason)
		return
	}
	_ = s.cfg.Store.UpdateCommentStatus(ctx, comment.ID, "approved", nil)
}
