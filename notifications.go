package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	envVAPIDPublicKey      = "SPORE_VAPID_PUBLIC_KEY"
	envVAPIDPrivateKey     = "SPORE_VAPID_PRIVATE_KEY"
	envVAPIDSubscriber     = "SPORE_VAPID_SUBSCRIBER"
	defaultVAPIDSubscriber = "mailto:admin@example.com"
)

func (s *service) configurePushFromEnv() {
	s.pushPublicKey = strings.TrimSpace(os.Getenv(envVAPIDPublicKey))
	s.pushPrivateKey = strings.TrimSpace(os.Getenv(envVAPIDPrivateKey))
	s.pushSubscriber = strings.TrimSpace(os.Getenv(envVAPIDSubscriber))
}

func parsePushSubscription(raw []byte) (endpoint string, normalizedJSON string, err error) {
	var payload pushSubscriptionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", err
	}
	payload.Endpoint = strings.TrimSpace(payload.Endpoint)
	if payload.Endpoint == "" {
		return "", "", fmt.Errorf("endpoint is required")
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", "", err
	}
	normalized, err := json.Marshal(generic)
	if err != nil {
		return "", "", err
	}
	return payload.Endpoint, string(normalized), nil
}

func (s *service) notifyAdminsOfNewComment(comment Comment, post Post) {
	ctx := context.Background()
	publicKey, privateKey, subscriber, err := s.ensurePushSettings(ctx)
	if err != nil || publicKey == "" || privateKey == "" {
		return
	}
	enabled, err := s.store.GetNotificationsEnabled(ctx)
	if err != nil || !enabled {
		return
	}
	subscriptions, err := s.store.ListAdminPushSubscriptions(ctx)
	if err != nil || len(subscriptions) == 0 {
		return
	}

	title := "New comment posted"
	if comment.Status == "pending" {
		title = "New comment awaiting moderation"
	}
	body := fmt.Sprintf("%s commented on \"%s\"", comment.AuthorName, post.Title)
	url := s.routePrefix + "/admin?view=comments"
	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
		"url":   url,
	})

	for _, sub := range subscriptions {
		if err := s.sendPushToSubscription(payload, sub.SubscriptionJSON, publicKey, privateKey, subscriber); err != nil {
			log.Printf("spore push failed for endpoint %s: %v", sub.Endpoint, err)
		}
	}
}

func (s *service) sendPushToSubscription(payload []byte, subscriptionJSON, publicKey, privateKey, subscriber string) error {
	var subscription webpush.Subscription
	if err := json.Unmarshal([]byte(subscriptionJSON), &subscription); err != nil {
		return err
	}
	resp, err := webpush.SendNotification(payload, &subscription, &webpush.Options{
		Subscriber:      subscriber,
		VAPIDPublicKey:  publicKey,
		VAPIDPrivateKey: privateKey,
		TTL:             60,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		_ = s.store.DeleteAdminPushSubscriptionByEndpoint(context.Background(), subscription.Endpoint)
		return nil
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected push response: %d", resp.StatusCode)
	}
	return nil
}

func (s *service) ensurePushSettings(ctx context.Context) (publicKey, privateKey, subscriber string, err error) {
	publicKey, privateKey, subscriber, err = s.store.GetVAPIDSettings(ctx)
	if err != nil {
		return "", "", "", err
	}

	if strings.TrimSpace(publicKey) != "" && strings.TrimSpace(privateKey) != "" {
		originalSubscriber := strings.TrimSpace(subscriber)
		subscriber = normalizedSubscriber(subscriber)
		if originalSubscriber == "" {
			_ = s.store.UpdateVAPIDSettings(ctx, publicKey, privateKey, subscriber)
		}
		return publicKey, privateKey, subscriber, nil
	}

	seedPublic := strings.TrimSpace(s.pushPublicKey)
	seedPrivate := strings.TrimSpace(s.pushPrivateKey)
	if seedPublic == "" || seedPrivate == "" {
		seedPrivate, seedPublic, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			return "", "", "", err
		}
	}

	subscriber = normalizedSubscriber(subscriber)
	if strings.TrimSpace(subscriber) == "" {
		subscriber = normalizedSubscriber(s.pushSubscriber)
	}

	if err := s.store.UpdateVAPIDSettings(ctx, seedPublic, seedPrivate, subscriber); err != nil {
		return "", "", "", err
	}
	return seedPublic, seedPrivate, subscriber, nil
}

func normalizedSubscriber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultVAPIDSubscriber
	}
	return value
}
