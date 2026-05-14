package blog

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	entityKindAnalytics = "post_analytics"
	analyticsIDPrefix   = "analytics-"

	viewerCookieName = "blog_viewer_token"
	viewDedupeWindow = 30 * time.Minute
)

// PostAnalytics stores the aggregated view stats for a single post. The blob
// is persisted as a sibling entity keyed by post ID, so existing databases
// upgrade without any schema change — analytics rows appear on first view.
type PostAnalytics struct {
	PostID       string     `json:"post_id"`
	TotalViews   int64      `json:"total_views"`
	LastViewedAt *time.Time `json:"last_viewed_at,omitempty"`
}

// AdminPostAnalytics is the per-post view returned to the admin UI: post
// metadata joined with the analytics counters and comment count.
type AdminPostAnalytics struct {
	PostID       string     `json:"post_id"`
	PostTitle    string     `json:"post_title"`
	PostSlug     string     `json:"post_slug"`
	Status       string     `json:"status"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	TotalViews   int64      `json:"total_views"`
	CommentCount int        `json:"comment_count"`
	LastViewedAt *time.Time `json:"last_viewed_at,omitempty"`
}

func analyticsEntityID(postID string) string {
	return analyticsIDPrefix + postID
}

// botUserAgentRe matches common bot/crawler User-Agent fragments.
var botUserAgentRe = regexp.MustCompile(`(?i)(bot|crawl|spider|slurp|fetcher|preview|monitor|lighthouse|headless|prerender|pingdom|http-client|wget|curl|python-requests|go-http-client|java/|okhttp|httrack|scrapy|facebookexternalhit|embedly|quora link preview|outbrain|pinterest|skypeuripreview|nuzzel|discordbot|twitterbot|whatsapp|telegrambot|linkedinbot)`)

func isLikelyBot(userAgent string) bool {
	if strings.TrimSpace(userAgent) == "" {
		return true
	}
	return botUserAgentRe.MatchString(userAgent)
}

// viewDedupeCache prevents counting the same visitor viewing the same post
// repeatedly within a short window. It is purely in-memory and best-effort;
// on restart it forgets, which is fine for view-count analytics.
type viewDedupeCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
	window  time.Duration
	maxSize int
}

func newViewDedupeCache(window time.Duration) *viewDedupeCache {
	return &viewDedupeCache{
		entries: make(map[string]time.Time),
		window:  window,
		maxSize: 5000,
	}
}

func (c *viewDedupeCache) shouldCount(token, postID string) bool {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(postID) == "" {
		return false
	}
	key := token + ":" + postID
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if last, ok := c.entries[key]; ok {
		if now.Sub(last) < c.window {
			c.entries[key] = now
			return false
		}
	}
	c.entries[key] = now
	if len(c.entries) > c.maxSize {
		c.evictLocked()
	}
	return true
}

func (c *viewDedupeCache) evictLocked() {
	threshold := time.Now().Add(-c.window)
	for k, t := range c.entries {
		if t.Before(threshold) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) <= c.maxSize {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	for k, t := range c.entries {
		if oldestKey == "" || t.Before(oldestTime) {
			oldestKey = k
			oldestTime = t
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (s *service) ensureViewerToken(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(viewerCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return cookie.Value
	}
	token := generateToken()
	http.SetCookie(w, &http.Cookie{
		Name:     viewerCookieName,
		Value:    token,
		Path:     s.routePrefix,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   60 * 60 * 24 * 365,
	})
	return token
}

// recordPostView increments a post's view counter unless the request looks
// like a bot or the same visitor recently viewed the same post.
func (s *service) recordPostView(w http.ResponseWriter, r *http.Request, postID string) {
	if strings.TrimSpace(postID) == "" {
		return
	}
	if isLikelyBot(r.Header.Get("User-Agent")) {
		return
	}
	token := s.ensureViewerToken(w, r)
	if s.viewDedupe == nil || !s.viewDedupe.shouldCount(token, postID) {
		return
	}
	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.store.IncrementPostViews(ctx, id)
	}(postID)
}

// IncrementPostViews creates or updates the analytics entity for a post.
// New posts and freshly-upgraded databases start at zero — the entity is
// only materialised on the first view, so no migration is required.
func (a *storeAdapter) IncrementPostViews(ctx context.Context, postID string) error {
	if strings.TrimSpace(postID) == "" {
		return fmt.Errorf("post id required")
	}
	id := analyticsEntityID(postID)
	entity, err := a.store.Get(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if entity == nil {
		entity = &Entity{
			ID:        id,
			Kind:      entityKindAnalytics,
			OwnerID:   postID,
			CreatedAt: now,
			Attrs:     Attributes{},
		}
	}
	if entity.Attrs == nil {
		entity.Attrs = Attributes{}
	}
	current := readInt64(entity.Attrs["total_views"])
	entity.Attrs["total_views"] = current + 1
	entity.Attrs["post_id"] = postID
	entity.Attrs["last_viewed_at"] = now.Format(time.RFC3339Nano)
	entity.OwnerID = postID
	entity.Kind = entityKindAnalytics
	entity.UpdatedAt = &now
	return a.store.Save(ctx, entity)
}

// GetPostAnalytics returns the stored analytics for a single post (nil if absent).
func (a *storeAdapter) GetPostAnalytics(ctx context.Context, postID string) (*PostAnalytics, error) {
	entity, err := a.store.Get(ctx, analyticsEntityID(postID))
	if err != nil || entity == nil {
		return nil, err
	}
	return entityToAnalytics(entity), nil
}

// ListAnalytics fetches analytics for every post, keyed by post ID.
func (a *storeAdapter) ListAnalytics(ctx context.Context) (map[string]*PostAnalytics, error) {
	entities, err := a.fetchAllEntities(ctx, entityKindAnalytics)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*PostAnalytics, len(entities))
	for _, e := range entities {
		analytics := entityToAnalytics(e)
		if analytics == nil || analytics.PostID == "" {
			continue
		}
		out[analytics.PostID] = analytics
	}
	return out, nil
}

// CountCommentsByPost returns the number of visible comments per post.
// Hidden and rejected comments are excluded so the admin view reflects what
// readers actually see on the public site.
func (a *storeAdapter) CountCommentsByPost(ctx context.Context) (map[string]int, error) {
	entities, err := a.fetchAllEntities(ctx, entityKindComment)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, e := range entities {
		if e == nil || e.OwnerID == "" {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(e.Status))
		if status == "rejected" || status == "hidden" {
			continue
		}
		counts[e.OwnerID]++
	}
	return counts, nil
}

func entityToAnalytics(e *Entity) *PostAnalytics {
	if e == nil {
		return nil
	}
	postID := e.OwnerID
	if postID == "" {
		postID = strings.TrimPrefix(e.ID, analyticsIDPrefix)
	}
	var totalViews int64
	var lastViewedAt *time.Time
	if e.Attrs != nil {
		if v, ok := e.Attrs["post_id"]; ok {
			if s, ok := v.(string); ok && s != "" {
				postID = s
			}
		}
		totalViews = readInt64(e.Attrs["total_views"])
		if v, ok := e.Attrs["last_viewed_at"]; ok {
			if s, ok := v.(string); ok && s != "" {
				if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
					lastViewedAt = &t
				} else if t, err := time.Parse(time.RFC3339, s); err == nil {
					lastViewedAt = &t
				}
			}
		}
	}
	return &PostAnalytics{
		PostID:       postID,
		TotalViews:   totalViews,
		LastViewedAt: lastViewedAt,
	}
}

func readInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case string:
		var out int64
		_, _ = fmt.Sscanf(n, "%d", &out)
		return out
	}
	return 0
}
