package blog

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	attrMCPAPIKey          = "mcp_api_key"
	attrMCPAPIKeyCreatedAt = "mcp_api_key_created_at"

	mcpProtocolVersion = "2024-11-05"
	mcpServerName      = "spore-blog"
	mcpServerVersion   = "0.1.0"
)

// ---------- API key persistence (stored alongside other blog settings) ----------

// GetMCPAPIKey reads the configured MCP API key and the time it was issued.
// Returns empty strings (no error) when no key is set.
func (a *storeAdapter) GetMCPAPIKey(ctx context.Context) (string, *time.Time, error) {
	entity, err := a.store.Get(ctx, entityIDBlogSettings)
	if err != nil || entity == nil || entity.Attrs == nil {
		return "", nil, err
	}
	key := strings.TrimSpace(fmt.Sprint(entity.Attrs[attrMCPAPIKey]))
	if key == "<nil>" {
		key = ""
	}
	var createdAt *time.Time
	if raw, ok := entity.Attrs[attrMCPAPIKeyCreatedAt]; ok {
		if s, ok := raw.(string); ok && s != "" {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				createdAt = &t
			} else if t, err := time.Parse(time.RFC3339, s); err == nil {
				createdAt = &t
			}
		}
	}
	if key == "" {
		return "", nil, nil
	}
	return key, createdAt, nil
}

// SetMCPAPIKey stores the key (or clears it when empty) on the blog settings entity.
func (a *storeAdapter) SetMCPAPIKey(ctx context.Context, key string) (string, *time.Time, error) {
	entity, err := a.getOrCreateBlogSettingsEntity(ctx)
	if err != nil {
		return "", nil, err
	}
	attrs := cloneAttributes(entity.Attrs)
	if attrs == nil {
		attrs = Attributes{}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		delete(attrs, attrMCPAPIKey)
		delete(attrs, attrMCPAPIKeyCreatedAt)
		entity.Attrs = attrs
		return "", nil, a.store.Save(ctx, entity)
	}
	now := time.Now().UTC()
	attrs[attrMCPAPIKey] = key
	attrs[attrMCPAPIKeyCreatedAt] = now.Format(time.RFC3339Nano)
	entity.Attrs = attrs
	return key, &now, a.store.Save(ctx, entity)
}

// generateMCPAPIKey returns a high-entropy URL-safe token prefixed for
// easy recognition in logs and config files.
func generateMCPAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "spore_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// ---------- HTTP transport ----------

func (s *service) mountMCPRoutes(r chi.Router) {
	r.Options("/mcp", s.handleMCPCORS)
	r.Post("/mcp", s.handleMCPRequest)
}

func (s *service) handleMCPCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Session-Id")
	w.WriteHeader(http.StatusNoContent)
}

func (s *service) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	storedKey, _, err := s.store.GetMCPAPIKey(r.Context())
	if err != nil {
		http.Error(w, "failed to load mcp configuration", http.StatusInternalServerError)
		return
	}
	if storedKey == "" {
		http.Error(w, "mcp api key not configured", http.StatusUnauthorized)
		return
	}

	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	provided := strings.TrimSpace(auth[len("bearer "):])
	if subtle.ConstantTimeCompare([]byte(provided), []byte(storedKey)) != 1 {
		http.Error(w, "invalid api key", http.StatusUnauthorized)
		return
	}

	var req mcpJSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMCPError(w, nil, mcpErrParse, "Parse error", err.Error())
		return
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		writeMCPError(w, req.ID, mcpErrInvalidRequest, "Invalid JSON-RPC version", req.JSONRPC)
		return
	}
	s.dispatchMCP(w, r, &req)
}

const (
	mcpErrParse          = -32700
	mcpErrInvalidRequest = -32600
	mcpErrMethodNotFound = -32601
	mcpErrInvalidParams  = -32602
	mcpErrInternal       = -32603
)

type mcpJSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpJSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (s *service) dispatchMCP(w http.ResponseWriter, r *http.Request, req *mcpJSONRPCRequest) {
	switch req.Method {
	case "initialize":
		writeMCPResult(w, req.ID, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    mcpServerName,
				"version": mcpServerVersion,
			},
			"instructions": "Manage a Spore blog: create and edit posts, moderate comments, and read analytics. Drafts have published_at=null. Use tools/list to discover the full schema.",
		})
	case "notifications/initialized", "initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted)
	case "ping":
		writeMCPResult(w, req.ID, map[string]any{})
	case "tools/list":
		writeMCPResult(w, req.ID, map[string]any{"tools": mcpToolDefinitions()})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeMCPError(w, req.ID, mcpErrInvalidParams, "Invalid params", err.Error())
				return
			}
		}
		result, err := s.callMCPTool(r.Context(), params.Name, params.Arguments)
		if err != nil {
			writeMCPResult(w, req.ID, mcpToolError(err))
			return
		}
		writeMCPResult(w, req.ID, mcpToolResult(result))
	default:
		writeMCPError(w, req.ID, mcpErrMethodNotFound, "Method not found", req.Method)
	}
}

func writeMCPResult(w http.ResponseWriter, id json.RawMessage, result any) {
	if isMCPNotification(id) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp := mcpJSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeMCPError(w http.ResponseWriter, id json.RawMessage, code int, message string, data any) {
	resp := mcpJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpRPCError{Code: code, Message: message, Data: data},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func isMCPNotification(id json.RawMessage) bool {
	s := strings.TrimSpace(string(id))
	return s == "" || s == "null"
}

// ---------- Tool registry ----------

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func mcpToolDefinitions() []mcpToolDef {
	stringField := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	intField := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}
	return []mcpToolDef{
		{
			Name:        "list_posts",
			Description: "List blog posts (both drafts and published), newest first. Returns id, slug, title, status, published_at, updated_at, tags, and a short excerpt.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":  intField("Maximum number of posts to return (default 50, max 500)."),
					"offset": intField("Pagination offset (default 0)."),
					"status": map[string]any{"type": "string", "enum": []string{"any", "published", "draft"}, "description": "Filter by status. Default 'any'."},
				},
			},
		},
		{
			Name:        "get_post",
			Description: "Fetch one post by ID or slug, including full markdown body and tags.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   stringField("Post UUID."),
					"slug": stringField("Post slug (alternative to id)."),
				},
			},
		},
		{
			Name:        "create_post",
			Description: "Create a new post. Provide markdown; HTML is rendered automatically. Set published=true to publish immediately, or supply published_at as an RFC3339 timestamp.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"title"},
				"properties": map[string]any{
					"title":            stringField("Post title."),
					"subtitle":         stringField("Optional subtitle."),
					"slug":             stringField("URL slug. Auto-generated from the title if omitted."),
					"content_markdown": stringField("Body content in markdown."),
					"meta_description": stringField("SEO meta description."),
					"published":        map[string]any{"type": "boolean", "description": "Publish immediately at the current time."},
					"published_at":     stringField("Explicit publish timestamp (RFC3339). Overrides 'published' when set."),
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tag names."},
					"author_id":        intField("Author identifier (defaults to 1)."),
				},
			},
		},
		{
			Name:        "update_post",
			Description: "Partial update of an existing post. Only fields you supply are modified. To unpublish, pass published_at=null.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":               stringField("Post UUID."),
					"title":            stringField("New title."),
					"subtitle":         stringField("New subtitle."),
					"slug":             stringField("New URL slug."),
					"content_markdown": stringField("Replacement markdown body."),
					"meta_description": stringField("Replacement SEO description."),
					"published_at": map[string]any{
						"type":        []string{"string", "null"},
						"description": "RFC3339 timestamp, or null to unpublish.",
					},
					"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Replacement tag list."},
				},
			},
		},
		{
			Name:        "delete_post",
			Description: "Delete a post permanently.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": stringField("Post UUID."),
				},
			},
		},
		{
			Name:        "list_comments",
			Description: "List comments across all posts for moderation. Optional status filter (pending, approved, rejected, hidden).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":  map[string]any{"type": "string", "enum": []string{"any", "pending", "approved", "rejected", "hidden"}},
					"limit":   intField("Default 50, max 200."),
					"offset":  intField("Pagination offset (default 0)."),
					"post_id": stringField("Restrict to one post (optional)."),
				},
			},
		},
		{
			Name:        "get_comment",
			Description: "Fetch a single comment by ID.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": stringField("Comment UUID."),
				},
			},
		},
		{
			Name:        "set_comment_status",
			Description: "Approve, hide, or reject a comment. 'rejected' is the typical action for spam.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id", "status"},
				"properties": map[string]any{
					"id":     stringField("Comment UUID."),
					"status": map[string]any{"type": "string", "enum": []string{"approved", "hidden", "rejected"}},
					"reason": stringField("Optional note (stored as spam_reason for rejected comments)."),
				},
			},
		},
		{
			Name:        "delete_comment",
			Description: "Delete a comment permanently.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": stringField("Comment UUID."),
				},
			},
		},
		{
			Name:        "get_analytics",
			Description: "Return per-post analytics (total views, comment counts, last viewed) sorted by views by default.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sort":  map[string]any{"type": "string", "enum": []string{"views", "comments", "recent"}, "description": "Sort by views (default), comments, or recent activity."},
					"limit": intField("Maximum rows to return (default 20)."),
				},
			},
		},
		{
			Name:        "list_tags",
			Description: "Return every distinct tag across published posts with usage counts.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (s *service) callMCPTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	case "list_posts":
		return s.mcpListPosts(ctx, args)
	case "get_post":
		return s.mcpGetPost(ctx, args)
	case "create_post":
		return s.mcpCreatePost(ctx, args)
	case "update_post":
		return s.mcpUpdatePost(ctx, args)
	case "delete_post":
		return s.mcpDeletePost(ctx, args)
	case "list_comments":
		return s.mcpListComments(ctx, args)
	case "get_comment":
		return s.mcpGetComment(ctx, args)
	case "set_comment_status":
		return s.mcpSetCommentStatus(ctx, args)
	case "delete_comment":
		return s.mcpDeleteComment(ctx, args)
	case "get_analytics":
		return s.mcpGetAnalytics(ctx, args)
	case "list_tags":
		return s.mcpListTags(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func mcpToolResult(data any) map[string]any {
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcpToolError(err)
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(payload)}},
		"isError": false,
	}
}

func mcpToolError(err error) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": "Error: " + err.Error()}},
		"isError": true,
	}
}

// ---------- Tool implementations ----------

type mcpPostSummary struct {
	ID              string     `json:"id"`
	Slug            string     `json:"slug"`
	Title           string     `json:"title"`
	Subtitle        string     `json:"subtitle,omitempty"`
	Status          string     `json:"status"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
	MetaDescription string     `json:"meta_description,omitempty"`
	Tags            []string   `json:"tags"`
	Excerpt         string     `json:"excerpt"`
}

func summarisePost(p Post) mcpPostSummary {
	tags := make([]string, 0, len(p.Tags))
	for _, t := range p.Tags {
		tags = append(tags, t.Name)
	}
	return mcpPostSummary{
		ID:              p.ID,
		Slug:            p.Slug,
		Title:           p.Title,
		Subtitle:        p.Subtitle,
		Status:          postStatus(&p),
		PublishedAt:     p.PublishedAt,
		UpdatedAt:       p.UpdatedAt,
		MetaDescription: p.MetaDescription,
		Tags:            tags,
		Excerpt:         trimToLength(markdownToPlainText(p.ContentMarkdown), 200),
	}
}

func detailedPost(p Post) map[string]any {
	tags := make([]string, 0, len(p.Tags))
	for _, t := range p.Tags {
		tags = append(tags, t.Name)
	}
	return map[string]any{
		"id":               p.ID,
		"slug":             p.Slug,
		"title":            p.Title,
		"subtitle":         p.Subtitle,
		"status":           postStatus(&p),
		"published_at":     p.PublishedAt,
		"updated_at":       p.UpdatedAt,
		"meta_description": p.MetaDescription,
		"author_id":        p.AuthorID,
		"tags":             tags,
		"content_markdown": p.ContentMarkdown,
	}
}

func (s *service) mcpListPosts(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Status string `json:"status"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if input.Limit <= 0 {
		input.Limit = 50
	}
	if input.Limit > 500 {
		input.Limit = 500
	}
	posts, err := s.store.ListAllPosts(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	filtered := make([]Post, 0, len(posts))
	for _, p := range posts {
		st := postStatus(&p)
		if input.Status == "" || input.Status == "any" || input.Status == st {
			filtered = append(filtered, p)
		}
	}
	end := input.Offset + input.Limit
	if input.Offset > len(filtered) {
		input.Offset = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	pageSlice := filtered[input.Offset:end]
	summaries := make([]mcpPostSummary, 0, len(pageSlice))
	for _, p := range pageSlice {
		summaries = append(summaries, summarisePost(p))
	}
	return map[string]any{
		"total":  len(filtered),
		"limit":  input.Limit,
		"offset": input.Offset,
		"posts":  summaries,
	}, nil
}

func (s *service) mcpGetPost(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	input.ID = strings.TrimSpace(input.ID)
	input.Slug = strings.TrimSpace(input.Slug)
	if input.ID == "" && input.Slug == "" {
		return nil, fmt.Errorf("provide either 'id' or 'slug'")
	}
	var post *Post
	var err error
	if input.ID != "" {
		post, err = s.store.GetPostByID(ctx, input.ID)
	} else {
		post, err = s.findPostBySlug(ctx, input.Slug)
	}
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, fmt.Errorf("post not found")
	}
	return detailedPost(*post), nil
}

func (s *service) findPostBySlug(ctx context.Context, slug string) (*Post, error) {
	posts, err := s.store.ListAllPosts(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	for i := range posts {
		if strings.EqualFold(posts[i].Slug, slug) {
			p := posts[i]
			return &p, nil
		}
	}
	return nil, nil
}

type mcpPostInput struct {
	Title           *string         `json:"title"`
	Subtitle        *string         `json:"subtitle"`
	Slug            *string         `json:"slug"`
	ContentMarkdown *string         `json:"content_markdown"`
	MetaDescription *string         `json:"meta_description"`
	Published       *bool           `json:"published"`
	PublishedAt     json.RawMessage `json:"published_at"`
	Tags            *[]string       `json:"tags"`
	AuthorID        *int            `json:"author_id"`
}

func parsePublishedAt(raw json.RawMessage, fallbackPublished *bool) (*time.Time, bool, error) {
	// Returns (timestamp, explicitlySet, error).
	if len(raw) == 0 {
		if fallbackPublished != nil && *fallbackPublished {
			now := time.Now().UTC()
			return &now, true, nil
		}
		return nil, false, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" {
		return nil, true, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false, fmt.Errorf("published_at must be a string or null")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, false, fmt.Errorf("published_at must be RFC3339: %w", err)
	}
	t = t.UTC()
	return &t, true, nil
}

func (s *service) mcpCreatePost(ctx context.Context, args json.RawMessage) (any, error) {
	var input mcpPostInput
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if input.Title == nil || strings.TrimSpace(*input.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	title := strings.TrimSpace(*input.Title)
	slug := ""
	if input.Slug != nil {
		slug = strings.TrimSpace(*input.Slug)
	}
	if slug == "" {
		slug = tagSlug(title)
	}
	if slug == "" {
		return nil, fmt.Errorf("could not derive a slug from the title")
	}

	publishedAt, _, err := parsePublishedAt(input.PublishedAt, input.Published)
	if err != nil {
		return nil, err
	}

	post := &Post{
		ID:          generateID(),
		Slug:        slug,
		Title:       title,
		PublishedAt: publishedAt,
	}
	if input.Subtitle != nil {
		post.Subtitle = *input.Subtitle
	}
	if input.ContentMarkdown != nil {
		post.ContentMarkdown = *input.ContentMarkdown
	}
	if input.MetaDescription != nil {
		post.MetaDescription = *input.MetaDescription
	}
	if input.AuthorID != nil {
		post.AuthorID = *input.AuthorID
	} else {
		post.AuthorID = 1
	}
	if input.Tags != nil {
		post.Tags = tagsFromNames(*input.Tags)
	}

	if post.ContentMarkdown != "" {
		html, err := markdownToHTMLUnsafe(post.ContentMarkdown)
		if err != nil {
			return nil, fmt.Errorf("render markdown: %w", err)
		}
		post.ContentHTML = html
	}

	if err := s.store.CreatePost(ctx, post); err != nil {
		return nil, err
	}
	s.queuePostProcessing("mcp create_post")
	return detailedPost(*post), nil
}

func (s *service) mcpUpdatePost(ctx context.Context, args json.RawMessage) (any, error) {
	var input mcpPostInput
	var id struct {
		ID string `json:"id"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if err := json.Unmarshal(args, &id); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if strings.TrimSpace(id.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	post, err := s.store.GetPostByID(ctx, id.ID)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, fmt.Errorf("post not found")
	}

	if input.Title != nil {
		post.Title = strings.TrimSpace(*input.Title)
	}
	if input.Subtitle != nil {
		post.Subtitle = *input.Subtitle
	}
	if input.Slug != nil {
		v := strings.TrimSpace(*input.Slug)
		if v != "" {
			post.Slug = v
		}
	}
	if input.ContentMarkdown != nil {
		post.ContentMarkdown = *input.ContentMarkdown
		html, err := markdownToHTMLUnsafe(post.ContentMarkdown)
		if err != nil {
			return nil, fmt.Errorf("render markdown: %w", err)
		}
		post.ContentHTML = html
	}
	if input.MetaDescription != nil {
		post.MetaDescription = *input.MetaDescription
	}
	if input.Tags != nil {
		post.Tags = tagsFromNames(*input.Tags)
	}
	if len(input.PublishedAt) > 0 {
		t, explicit, err := parsePublishedAt(input.PublishedAt, nil)
		if err != nil {
			return nil, err
		}
		if explicit {
			post.PublishedAt = t
		}
	} else if input.Published != nil {
		if *input.Published {
			now := time.Now().UTC()
			post.PublishedAt = &now
		} else {
			post.PublishedAt = nil
		}
	}

	if err := s.store.UpdatePost(ctx, post); err != nil {
		return nil, err
	}
	s.queuePostProcessing("mcp update_post")
	return detailedPost(*post), nil
}

func (s *service) mcpDeletePost(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		ID string `json:"id"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if strings.TrimSpace(input.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := s.store.DeletePost(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": input.ID, "deleted": true}, nil
}

func (s *service) mcpListComments(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		PostID string `json:"post_id"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if input.Limit <= 0 {
		input.Limit = 50
	}
	if input.Limit > 200 {
		input.Limit = 200
	}
	status := input.Status
	if status == "any" {
		status = ""
	}
	comments, err := s.store.ListCommentsForModeration(ctx, status, input.Limit, input.Offset)
	if err != nil {
		return nil, err
	}
	if input.PostID != "" {
		filtered := comments[:0]
		for _, c := range comments {
			if c.PostID == input.PostID {
				filtered = append(filtered, c)
			}
		}
		comments = filtered
	}
	return map[string]any{
		"limit":    input.Limit,
		"offset":   input.Offset,
		"comments": comments,
	}, nil
}

func (s *service) mcpGetComment(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		ID string `json:"id"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if strings.TrimSpace(input.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	comment, err := s.store.GetCommentByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if comment == nil {
		return nil, fmt.Errorf("comment not found")
	}
	return comment, nil
}

func (s *service) mcpSetCommentStatus(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		ID     string  `json:"id"`
		Status string  `json:"status"`
		Reason *string `json:"reason"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if strings.TrimSpace(input.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	switch input.Status {
	case "approved", "hidden", "rejected":
	default:
		return nil, fmt.Errorf("status must be one of approved, hidden, rejected")
	}
	if err := s.store.UpdateCommentStatus(ctx, input.ID, input.Status, input.Reason); err != nil {
		return nil, err
	}
	comment, err := s.store.GetCommentByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *service) mcpDeleteComment(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		ID string `json:"id"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if strings.TrimSpace(input.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := s.store.DeleteCommentByID(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": input.ID, "deleted": true}, nil
}

func (s *service) mcpGetAnalytics(ctx context.Context, args json.RawMessage) (any, error) {
	var input struct {
		Sort  string `json:"sort"`
		Limit int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}
	analytics, err := s.store.ListAnalytics(ctx)
	if err != nil {
		return nil, err
	}
	commentCounts, err := s.store.CountCommentsByPost(ctx)
	if err != nil {
		return nil, err
	}
	posts, err := s.store.ListAllPosts(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	rows := make([]AdminPostAnalytics, 0, len(posts))
	for i := range posts {
		p := posts[i]
		row := AdminPostAnalytics{
			PostID:       p.ID,
			PostTitle:    p.Title,
			PostSlug:     p.Slug,
			Status:       postStatus(&p),
			PublishedAt:  p.PublishedAt,
			CommentCount: commentCounts[p.ID],
		}
		if a, ok := analytics[p.ID]; ok && a != nil {
			row.TotalViews = a.TotalViews
			row.LastViewedAt = a.LastViewedAt
		}
		rows = append(rows, row)
	}
	switch input.Sort {
	case "comments":
		sortAnalytics(rows, func(a, b AdminPostAnalytics) bool {
			if a.CommentCount != b.CommentCount {
				return a.CommentCount > b.CommentCount
			}
			return a.TotalViews > b.TotalViews
		})
	case "recent":
		sortAnalytics(rows, func(a, b AdminPostAnalytics) bool {
			at := time.Time{}
			bt := time.Time{}
			if a.LastViewedAt != nil {
				at = *a.LastViewedAt
			}
			if b.LastViewedAt != nil {
				bt = *b.LastViewedAt
			}
			return at.After(bt)
		})
	default:
		sortAnalytics(rows, func(a, b AdminPostAnalytics) bool {
			if a.TotalViews != b.TotalViews {
				return a.TotalViews > b.TotalViews
			}
			return a.CommentCount > b.CommentCount
		})
	}
	if input.Limit < len(rows) {
		rows = rows[:input.Limit]
	}
	return map[string]any{
		"rows":  rows,
		"total": len(rows),
	}, nil
}

func (s *service) mcpListTags(ctx context.Context, _ json.RawMessage) (any, error) {
	posts, err := s.store.ListAllPosts(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	display := map[string]string{}
	for _, p := range posts {
		if p.PublishedAt == nil {
			continue
		}
		for _, t := range p.Tags {
			slug := strings.TrimSpace(t.Slug)
			if slug == "" {
				slug = tagSlug(t.Name)
			}
			if slug == "" {
				continue
			}
			counts[slug]++
			if _, ok := display[slug]; !ok {
				display[slug] = t.Name
			}
		}
	}
	type tagCount struct {
		Slug  string `json:"slug"`
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	out := make([]tagCount, 0, len(counts))
	for slug, count := range counts {
		out = append(out, tagCount{Slug: slug, Name: display[slug], Count: count})
	}
	// Stable sort by count desc, then slug asc.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Count > out[i].Count || (out[j].Count == out[i].Count && out[j].Slug < out[i].Slug) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return map[string]any{"tags": out}, nil
}

func sortAnalytics(rows []AdminPostAnalytics, less func(a, b AdminPostAnalytics) bool) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if less(rows[j], rows[i]) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func tagsFromNames(names []string) []Tag {
	tags := make([]Tag, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		slug := tagSlug(name)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		tags = append(tags, Tag{ID: slug, Name: name, Slug: slug})
	}
	return tags
}

// ---------- Admin REST surface for managing the API key ----------

func (s *service) handleAdminGetMCPInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.mcpInfo(r))
}

func (s *service) handleAdminGenerateMCPKey(w http.ResponseWriter, r *http.Request) {
	key, err := generateMCPAPIKey()
	if err != nil {
		http.Error(w, "failed to generate key", http.StatusInternalServerError)
		return
	}
	if _, _, err := s.store.SetMCPAPIKey(r.Context(), key); err != nil {
		http.Error(w, "failed to persist key", http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.mcpInfo(r))
}

func (s *service) handleAdminRevokeMCPKey(w http.ResponseWriter, r *http.Request) {
	if _, _, err := s.store.SetMCPAPIKey(r.Context(), ""); err != nil {
		http.Error(w, "failed to revoke key", http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.mcpInfo(r))
}

func (s *service) mcpInfo(r *http.Request) map[string]any {
	key, createdAt, _ := s.store.GetMCPAPIKey(r.Context())
	url := s.mcpServerURL(r)
	info := map[string]any{
		"url":         url,
		"api_key":     key,
		"created_at":  createdAt,
		"has_key":     key != "",
		"server_name": mcpServerName,
	}
	if key != "" {
		info["config_snippet"] = mcpConfigSnippet(url, key)
	}
	return info
}

func (s *service) mcpServerURL(r *http.Request) string {
	prefix := s.routePrefix
	if s.cfg.SiteURL != "" {
		base := strings.TrimSuffix(s.cfg.SiteURL, "/")
		return base + prefix + "/mcp"
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s://%s%s/mcp", scheme, host, prefix)
}

func mcpConfigSnippet(url, key string) string {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			mcpServerName: map[string]any{
				"type": "http",
				"url":  url,
				"headers": map[string]any{
					"Authorization": "Bearer " + key,
				},
			},
		},
	}
	buf, _ := json.MarshalIndent(cfg, "", "  ")
	return string(buf)
}
