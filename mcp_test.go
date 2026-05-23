package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mcpRoundtrip sends a JSON-RPC request and returns the parsed response.
func mcpRoundtrip(t *testing.T, h http.Handler, key string, payload map[string]any) *mcpJSONRPCResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/blog/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Code == http.StatusAccepted {
		return nil
	}
	var resp mcpJSONRPCResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	return &resp
}

func mcpToolText(t *testing.T, resp *mcpJSONRPCResponse) string {
	t.Helper()
	if resp == nil {
		t.Fatalf("nil response")
	}
	if resp.Error != nil {
		t.Fatalf("rpc error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %#v", resp.Result)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("tool returned error: %#v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("empty content")
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	return text
}

func seedMCPHandler(t *testing.T) (*Handler, *memEntityStore, string) {
	t.Helper()
	store := newMemEntityStore()
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	// Provision a key via the admin endpoint.
	req := httptest.NewRequest(http.MethodPost, "/blog/admin/api/mcp/key", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("generate key status=%d body=%s", rr.Code, rr.Body.String())
	}
	var info map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	key, _ := info["api_key"].(string)
	if key == "" {
		t.Fatalf("no key returned")
	}
	return h, store, key
}

func TestMCPRequiresKey(t *testing.T) {
	store := newMemEntityStore()
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"})
	req := httptest.NewRequest(http.MethodPost, "/blog/mcp", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rr.Code)
	}
}

func TestMCPWrongKeyRejected(t *testing.T) {
	h, _, _ := seedMCPHandler(t)
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"})
	req := httptest.NewRequest(http.MethodPost, "/blog/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer not-the-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", rr.Code)
	}
}

func TestMCPInitializeAndListTools(t *testing.T) {
	h, _, key := seedMCPHandler(t)
	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": mcpProtocolVersion},
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	if got, _ := result["protocolVersion"].(string); got != mcpProtocolVersion {
		t.Fatalf("protocolVersion = %s", got)
	}

	resp = mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	result, _ = resp.Result.(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) < 9 {
		t.Fatalf("tools count = %d want at least 9", len(tools))
	}
	names := map[string]bool{}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _ := tool["name"].(string)
		names[name] = true
	}
	for _, expected := range []string{"list_posts", "create_post", "update_post", "delete_post", "list_comments", "set_comment_status", "get_analytics", "list_tags", "upload_image"} {
		if !names[expected] {
			t.Fatalf("missing tool: %s", expected)
		}
	}
}

func TestMCPUploadImage(t *testing.T) {
	store := newMemEntityStore()
	imageStore, err := NewFileImageStore(t.TempDir(), "/blog/images")
	if err != nil {
		t.Fatalf("image store: %v", err)
	}
	h, err := NewHandler(Config{Store: store, ImageStore: imageStore})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/blog/admin/api/mcp/key", nil))
	var info map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info)
	key, _ := info["api_key"].(string)

	const pixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "upload_image",
			"arguments": map[string]any{
				"filename":    "pixel.png",
				"data_base64": "data:image/png;base64," + pixelPNG,
				"alt":         "Tiny pixel",
			},
		},
	})
	text := mcpToolText(t, resp)
	var uploaded map[string]any
	if err := json.Unmarshal([]byte(text), &uploaded); err != nil {
		t.Fatalf("decode upload result: %v", err)
	}
	url, _ := uploaded["url"].(string)
	if !strings.HasPrefix(url, "/blog/images/") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("url = %q", url)
	}
	if got, _ := uploaded["content_type"].(string); got != "image/png" {
		t.Fatalf("content_type = %q", got)
	}
	if got, _ := uploaded["markdown"].(string); got != "![Tiny pixel]("+url+")" {
		t.Fatalf("markdown = %q", got)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, url, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("image get status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("served content-type = %q", got)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("served image body is empty")
	}
}

func TestMCPUploadImageRequiresImageStore(t *testing.T) {
	h, _, key := seedMCPHandler(t)
	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "upload_image",
			"arguments": map[string]any{
				"filename":    "pixel.png",
				"data_base64": "iVBORw0KGgo=",
			},
		},
	})
	result, _ := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("expected tool error, got %#v", result)
	}
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	if text, _ := block["text"].(string); !strings.Contains(text, "image storage not configured") {
		t.Fatalf("unexpected error text: %q", text)
	}
}

func TestMCPCreatePostAndList(t *testing.T) {
	h, store, key := seedMCPHandler(t)

	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "create_post",
			"arguments": map[string]any{
				"title":            "Hello from MCP",
				"content_markdown": "# Hi\n\nBody here.",
				"published":        true,
				"tags":             []string{"intro", "mcp"},
			},
		},
	})
	text := mcpToolText(t, resp)
	var created map[string]any
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if got, _ := created["title"].(string); got != "Hello from MCP" {
		t.Fatalf("title = %q", got)
	}
	if got, _ := created["status"].(string); got != "published" {
		t.Fatalf("status = %s", got)
	}
	postID, _ := created["id"].(string)
	if postID == "" {
		t.Fatalf("missing id")
	}

	// Verify it shows up in list_posts.
	resp = mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "list_posts"},
	})
	text = mcpToolText(t, resp)
	if !strings.Contains(text, "Hello from MCP") {
		t.Fatalf("list_posts did not include new post: %s", text)
	}

	// And in the underlying store.
	entity, err := store.Get(context.Background(), postID)
	if err != nil {
		t.Fatalf("store get: %v", err)
	}
	if entity == nil {
		t.Fatalf("post not persisted")
	}
}

func TestMCPUpdatePostPartial(t *testing.T) {
	h, _, key := seedMCPHandler(t)

	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "create_post",
			"arguments": map[string]any{
				"title":            "Original",
				"content_markdown": "Body",
				"published":        true,
			},
		},
	})
	text := mcpToolText(t, resp)
	var created map[string]any
	_ = json.Unmarshal([]byte(text), &created)
	id, _ := created["id"].(string)

	// Unpublish via published_at=null while leaving everything else intact.
	resp = mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name": "update_post",
			"arguments": map[string]any{
				"id":           id,
				"published_at": nil,
				"subtitle":     "draft now",
			},
		},
	})
	text = mcpToolText(t, resp)
	var updated map[string]any
	if err := json.Unmarshal([]byte(text), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated["published_at"] != nil {
		t.Fatalf("expected published_at=null got %v", updated["published_at"])
	}
	if got, _ := updated["subtitle"].(string); got != "draft now" {
		t.Fatalf("subtitle = %s", got)
	}
	if got, _ := updated["title"].(string); got != "Original" {
		t.Fatalf("title should be unchanged, got %s", got)
	}
}

func TestMCPCommentModeration(t *testing.T) {
	store := newMemEntityStore()
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	// Provision key.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/blog/admin/api/mcp/key", nil))
	var info map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info)
	key, _ := info["api_key"].(string)

	// Seed a post and a pending comment.
	now := time.Now().UTC()
	post := &Post{ID: "p1", Slug: "p1", Title: "Post", PublishedAt: &now}
	_ = store.Save(context.Background(), entityFromPost(post))
	comment := &Comment{ID: "c1", PostID: "p1", AuthorName: "Anon", Content: "spam", Status: "pending", CreatedAt: now}
	_ = store.Save(context.Background(), entityFromComment(comment))

	reason := "obvious spam"
	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "set_comment_status",
			"arguments": map[string]any{
				"id":     "c1",
				"status": "rejected",
				"reason": reason,
			},
		},
	})
	text := mcpToolText(t, resp)
	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s, _ := got["status"].(string); s != "rejected" {
		t.Fatalf("status = %s", s)
	}
	if s, _ := got["spam_reason"].(string); s != reason {
		t.Fatalf("spam_reason = %s", s)
	}

	// Delete the comment.
	resp = mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name":      "delete_comment",
			"arguments": map[string]any{"id": "c1"},
		},
	})
	_ = mcpToolText(t, resp)
	if e, _ := store.Get(context.Background(), "c1"); e != nil {
		t.Fatalf("expected comment deleted")
	}
}

func TestMCPAnalyticsTool(t *testing.T) {
	store := newMemEntityStore()
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/blog/admin/api/mcp/key", nil))
	var info map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info)
	key, _ := info["api_key"].(string)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("p%d", i)
		post := &Post{ID: id, Slug: id, Title: id, PublishedAt: &now}
		_ = store.Save(context.Background(), entityFromPost(post))
	}
	adapter := newStoreAdapter(store)
	_ = adapter.IncrementPostViews(context.Background(), "p2")
	_ = adapter.IncrementPostViews(context.Background(), "p2")
	_ = adapter.IncrementPostViews(context.Background(), "p0")

	resp := mcpRoundtrip(t, h, key, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "get_analytics",
			"arguments": map[string]any{},
		},
	})
	text := mcpToolText(t, resp)
	var payload struct {
		Rows []AdminPostAnalytics `json:"rows"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Rows) != 3 {
		t.Fatalf("rows = %d want 3", len(payload.Rows))
	}
	if payload.Rows[0].PostID != "p2" {
		t.Fatalf("top row by views should be p2, got %s", payload.Rows[0].PostID)
	}
	if payload.Rows[0].TotalViews != 2 {
		t.Fatalf("p2 views = %d want 2", payload.Rows[0].TotalViews)
	}
}

func TestMCPAdminKeyLifecycle(t *testing.T) {
	store := newMemEntityStore()
	h, err := NewHandler(Config{Store: store})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	// Initial state: no key.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/blog/admin/api/mcp", nil))
	var info1 map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info1)
	if has, _ := info1["has_key"].(bool); has {
		t.Fatalf("expected no key initially")
	}
	if url, _ := info1["url"].(string); !strings.HasSuffix(url, "/blog/mcp") {
		t.Fatalf("url = %s", url)
	}

	// Generate.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/blog/admin/api/mcp/key", nil))
	var info2 map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info2)
	if has, _ := info2["has_key"].(bool); !has {
		t.Fatalf("expected key after generate")
	}
	key1, _ := info2["api_key"].(string)
	if key1 == "" || !strings.HasPrefix(key1, "spore_") {
		t.Fatalf("unexpected key shape: %s", key1)
	}
	if snippet, _ := info2["config_snippet"].(string); !strings.Contains(snippet, key1) {
		t.Fatalf("config_snippet missing key")
	}

	// Regenerate -> different key.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/blog/admin/api/mcp/key", nil))
	var info3 map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info3)
	key2, _ := info3["api_key"].(string)
	if key2 == "" || key2 == key1 {
		t.Fatalf("expected regenerated key to differ")
	}

	// Revoke.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/blog/admin/api/mcp/key", nil))
	var info4 map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &info4)
	if has, _ := info4["has_key"].(bool); has {
		t.Fatalf("expected no key after revoke")
	}
}
