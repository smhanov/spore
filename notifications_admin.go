package blog

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type pushSubscriptionPayload struct {
	Endpoint string `json:"endpoint"`
}

func (s *service) handleAdminGetNotificationPublicKey(w http.ResponseWriter, r *http.Request) {
	publicKey, privateKey, subscriber, err := s.ensurePushSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	notificationsEnabled, err := s.store.GetNotificationsEnabled(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"supported":             publicKey != "" && privateKey != "",
		"notifications_enabled": notificationsEnabled,
		"public_key":            publicKey,
		"private_key":           privateKey,
		"subscriber":            subscriber,
	})
}

func (s *service) handleAdminSubscribeNotifications(w http.ResponseWriter, r *http.Request) {
	publicKey, privateKey, _, err := s.ensurePushSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	if publicKey == "" || privateKey == "" {
		http.Error(w, "push notifications are not configured", http.StatusNotImplemented)
		return
	}
	notificationsEnabled, err := s.store.GetNotificationsEnabled(r.Context())
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	if !notificationsEnabled {
		http.Error(w, "notifications are disabled", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	endpoint, normalized, err := parsePushSubscription(body)
	if err != nil {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if err := s.store.UpsertAdminPushSubscription(r.Context(), endpoint, normalized); err != nil {
		http.Error(w, "failed to save subscription", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *service) handleAdminUnsubscribeNotifications(w http.ResponseWriter, r *http.Request) {
	var payload pushSubscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	endpoint := strings.TrimSpace(payload.Endpoint)
	if endpoint == "" {
		http.Error(w, "endpoint is required", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteAdminPushSubscriptionByEndpoint(r.Context(), endpoint); err != nil {
		http.Error(w, "failed to remove subscription", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
