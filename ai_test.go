package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smhanov/llmhub"
)

// validateStubServer is an OpenAI-compatible chat-completions stub that always
// replies with the assistant message "ok". It can be told to answer the next N
// requests with HTTP 500 to exercise failure handling.
type validateStubServer struct {
	mu       sync.Mutex
	failNext int
	calls    int
	srv      *httptest.Server
}

func newValidateStubServer(t *testing.T) *validateStubServer {
	t.Helper()
	stub := &validateStubServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.calls++
		fail := stub.failNext > 0
		if fail {
			stub.failNext--
		}
		stub.mu.Unlock()

		if fail {
			http.Error(w, "stub unavailable", http.StatusInternalServerError)
			return
		}

		// Brief pause so a successful round trip measures a nonzero duration.
		time.Sleep(5 * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "stub-completion",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	})
	stub.srv = httptest.NewServer(mux)
	t.Cleanup(stub.srv.Close)
	return stub
}

func (s *validateStubServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *validateStubServer) failNextRequests(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = n
}

// postValidateAI POSTs the body to the admin validate endpoint and returns the
// HTTP status plus the decoded response (zero value for non-200 statuses).
func postValidateAI(t *testing.T, h *Handler, body string) (int, aiValidateResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/blog/admin/api/ai/validate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp aiValidateResponse
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode validate response: %v", err)
		}
	}
	return rr.Code, resp
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := NewHandler(Config{Store: newMemEntityStore()})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return h
}

func TestAdminValidateAIMalformedJSON(t *testing.T) {
	h := newTestHandler(t)

	code, _ := postValidateAI(t, h, `{not json`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d want %d", code, http.StatusBadRequest)
	}
}

func TestAdminValidateAIBadSlot(t *testing.T) {
	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]any{
		"slot":     "clever",
		"settings": AISettings{Smart: stubProviderSettings("http://localhost:1")},
	})
	code, _ := postValidateAI(t, h, string(body))
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d want %d", code, http.StatusBadRequest)
	}
}

func TestAdminValidateAIUnconfiguredProvider(t *testing.T) {
	stub := newValidateStubServer(t)
	h := newTestHandler(t)

	// Model missing: the pre-check must short-circuit without any network call.
	body, _ := json.Marshal(map[string]any{
		"slot":     "smart",
		"settings": AISettings{Smart: AIProviderSettings{Provider: "openai", APIKey: "stub-key", BaseURL: stub.srv.URL}},
	})
	code, resp := postValidateAI(t, h, string(body))
	if code != http.StatusOK {
		t.Fatalf("status = %d want %d body=%s", code, http.StatusOK, resp.Error)
	}
	if resp.OK {
		t.Fatalf("ok = true want false for unconfigured provider")
	}
	if resp.Error == "" {
		t.Fatalf("expected an error message for unconfigured provider")
	}
	if stub.requestCount() != 0 {
		t.Fatalf("expected no outbound calls for unconfigured provider, stub saw %d", stub.requestCount())
	}
}

func TestAdminValidateAISuccess(t *testing.T) {
	stub := newValidateStubServer(t)
	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]any{
		"slot":     "smart",
		"settings": AISettings{Smart: stubProviderSettings(stub.srv.URL)},
	})
	code, resp := postValidateAI(t, h, string(body))
	if code != http.StatusOK {
		t.Fatalf("status = %d want %d body=%s", code, http.StatusOK, resp.Error)
	}
	if !resp.OK {
		t.Fatalf("ok = false want true (error=%q)", resp.Error)
	}
	if resp.Response != "ok" {
		t.Fatalf("response = %q want %q", resp.Response, "ok")
	}
	if resp.DurationMS <= 0 {
		t.Fatalf("duration_ms = %d want > 0", resp.DurationMS)
	}
	if stub.requestCount() != 1 {
		t.Fatalf("expected exactly one stub call, saw %d", stub.requestCount())
	}
}

func TestAdminValidateAIProviderError(t *testing.T) {
	stub := newValidateStubServer(t)
	stub.failNextRequests(1)
	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]any{
		"slot":     "dumb",
		"settings": AISettings{Dumb: stubProviderSettings(stub.srv.URL)},
	})
	code, resp := postValidateAI(t, h, string(body))
	if code != http.StatusOK {
		t.Fatalf("status = %d want %d (provider errors are reported inline)", code, http.StatusOK)
	}
	if resp.OK {
		t.Fatalf("ok = true want false for provider error")
	}
	if resp.Error == "" {
		t.Fatalf("expected provider error text in response")
	}
	if !bytes.Contains([]byte(resp.Error), []byte("stub unavailable")) && !bytes.Contains([]byte(resp.Error), []byte("500")) {
		t.Fatalf("error = %q want it to mention the stub failure", resp.Error)
	}
}

func TestAdminValidateAIUsesPostedSettings(t *testing.T) {
	stub := newValidateStubServer(t)
	// The store is completely unconfigured; the handler must test the posted
	// settings rather than the stored ones.
	h := newTestHandler(t)

	body, _ := json.Marshal(map[string]any{
		"slot":     "dumb",
		"settings": AISettings{Dumb: stubProviderSettings(stub.srv.URL)},
	})
	code, resp := postValidateAI(t, h, string(body))
	if code != http.StatusOK {
		t.Fatalf("status = %d want %d", code, http.StatusOK)
	}
	if !resp.OK {
		t.Fatalf("ok = false want true with posted settings (error=%q)", resp.Error)
	}
	if stub.requestCount() != 1 {
		t.Fatalf("expected exactly one stub call, saw %d", stub.requestCount())
	}
}

// geminiStubServer captures the JSON bodies POSTed to the Gemini
// generateContent endpoint and always replies with a minimal valid response.
type geminiStubServer struct {
	mu     sync.Mutex
	bodies []map[string]any
	srv    *httptest.Server
}

func newGeminiStubServer(t *testing.T) *geminiStubServer {
	t.Helper()
	stub := &geminiStubServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ":generateContent") {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		stub.mu.Lock()
		stub.bodies = append(stub.bodies, body)
		stub.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "ok"}}}},
			},
		})
	})
	stub.srv = httptest.NewServer(mux)
	t.Cleanup(stub.srv.Close)
	return stub
}

func (s *geminiStubServer) requestBodies() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.bodies...)
}

// TestNewLLMClientGeminiWebSearchFlag drives newLLMClient against a Gemini stub
// and inspects the outgoing request body: the google_search grounding tool must
// appear only when the webSearch flag is true.
func TestNewLLMClientGeminiWebSearchFlag(t *testing.T) {
	stub := newGeminiStubServer(t)
	prompt := []*llmhub.Message{
		llmhub.NewSystemMessage(llmhub.Text("You are a connectivity test.")),
		llmhub.NewUserMessage(llmhub.Text("Reply with exactly: ok")),
	}
	settings := AIProviderSettings{
		Provider: "gemini",
		Model:    "gemini-2.0-flash",
		APIKey:   "stub-key",
		BaseURL:  stub.srv.URL,
	}

	t.Run("web search off", func(t *testing.T) {
		client, err := newLLMClient(settings, false)
		if err != nil {
			t.Fatalf("newLLMClient: %v", err)
		}
		if _, err := client.Generate(context.Background(), prompt); err != nil {
			t.Fatalf("generate: %v", err)
		}

		bodies := stub.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("expected 1 stub call, saw %d", len(bodies))
		}
		if _, ok := bodies[0]["contents"]; !ok {
			t.Fatalf("expected contents in gemini request body: %v", bodies[0])
		}
		if _, ok := bodies[0]["tools"]; ok {
			t.Fatalf("webSearch=false must not send a grounding tool, body: %v", bodies[0])
		}
	})

	t.Run("web search on", func(t *testing.T) {
		client, err := newLLMClient(settings, true)
		if err != nil {
			t.Fatalf("newLLMClient: %v", err)
		}
		if _, err := client.Generate(context.Background(), prompt); err != nil {
			t.Fatalf("generate: %v", err)
		}

		bodies := stub.requestBodies()
		if len(bodies) != 2 {
			t.Fatalf("expected 2 stub calls, saw %d", len(bodies))
		}
		tools, ok := bodies[1]["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Fatalf("webSearch=true must send a tools option, body: %v", bodies[1])
		}
		tool, ok := tools[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected tools shape: %v", bodies[1]["tools"])
		}
		if _, ok := tool["google_search"]; !ok {
			t.Fatalf("expected google_search grounding tool, got: %v", bodies[1]["tools"])
		}
	})
}
