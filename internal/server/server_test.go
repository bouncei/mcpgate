package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bouncei/mcpgate/internal/auth"
	"github.com/bouncei/mcpgate/internal/config"
)

// stubUpstream returns canned responses for tools/list and tools/call.
func stubUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(string(body), `"tools/list"`):
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[
				{"name":"read_file"},{"name":"run_command"}]}}`)
		case strings.Contains(string(body), `"tools/call"`):
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok"}]}}`)
		default:
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":0,"result":{}}`)
		}
	}))
}

func newTestServer(t *testing.T, upstreamURL string) http.Handler {
	t.Helper()
	rl := config.RateLimit{RPS: 100, Burst: 100}
	cfg := &config.Config{
		Upstream: config.Upstream{URL: upstreamURL, Timeout: config.Duration(5_000_000_000)},
		Audit:    config.Audit{Output: "stdout"},
		Keys: []config.Key{
			{Label: "limited", Hash: auth.HashKey("k-limited"), Allow: []string{"read_file"}, RateLimit: &rl},
			{Label: "full", Hash: auth.HashKey("k-full"), Allow: []string{"*"}, RateLimit: &rl},
		},
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler()
}

func post(t *testing.T, h http.Handler, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://gateway/mcp", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUnauthenticatedRejected(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")
	rec := post(t, h, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAllowedToolCallProxied(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")
	rec := post(t, h, "k-limited", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("expected upstream result, got %s", rec.Body.String())
	}
}

func TestDeniedToolCallBlocked(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")
	rec := post(t, h, "k-limited", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"run_command"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (policy denial uses JSON-RPC error over 200)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not permitted") {
		t.Errorf("expected policy error, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"result"`) {
		t.Error("denied call must not reach upstream")
	}
}

func TestToolsListFiltered(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")
	rec := post(t, h, "k-limited", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "read_file") {
		t.Error("read_file should be visible")
	}
	if strings.Contains(body, "run_command") {
		t.Error("run_command should have been filtered from tools/list")
	}
}

func TestRateLimited(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	rl := config.RateLimit{RPS: 1, Burst: 1}
	cfg := &config.Config{
		Upstream: config.Upstream{URL: up.URL + "/mcp", Timeout: config.Duration(5_000_000_000)},
		Audit:    config.Audit{Output: "stdout"},
		Keys:     []config.Key{{Label: "slow", Hash: auth.HashKey("k-slow"), Allow: []string{"*"}, RateLimit: &rl}},
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file"}}`
	if got := post(t, h, "k-slow", body); got.Code != http.StatusOK {
		t.Fatalf("first call status = %d", got.Code)
	}
	if got := post(t, h, "k-slow", body); got.Code != http.StatusTooManyRequests {
		t.Fatalf("second call status = %d, want 429", got.Code)
	}
}
