package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Requirement 4: non-POST requests (GET SSE listen / DELETE) are authenticated
// and proxied through without body inspection.
func TestNonPostProxied(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")

	req := httptest.NewRequest(http.MethodGet, "http://gateway/mcp", nil)
	req.Header.Set("Authorization", "Bearer k-full")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
}

// Requirement 5: batched (JSON array) bodies are rejected with 400.
func TestBatchRejected(t *testing.T) {
	up := stubUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")

	rec := post(t, h, "k-full", `[{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("batch status = %d, want 400", rec.Code)
	}
}

// tools/list returned as SSE (text/event-stream) must still be filtered to the
// caller's allowlist — this is the form real MCP servers (e.g. the everything
// server) use.
func TestToolsListSSEFiltered(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"read_file\"},{\"name\":\"run_command\"}]}}\n\n")
	}))
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")

	rec := post(t, h, "k-limited", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "read_file") {
		t.Error("read_file should be visible")
	}
	if strings.Contains(rec.Body.String(), "run_command") {
		t.Errorf("run_command should have been filtered from SSE tools/list: %s", rec.Body.String())
	}
}

// Security: if the upstream returns tools/list in a form we cannot filter
// (neither JSON nor SSE), the gateway must fail closed — never stream the
// unfiltered tool list to the caller.
func TestToolsListUnfilterableFailsClosed(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `tools: read_file, run_command`)
	}))
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")

	rec := post(t, h, "k-limited", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected fail-closed (non-200), got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "run_command") {
		t.Errorf("disallowed tool leaked in unfilterable response: %s", rec.Body.String())
	}
}

// Requirement 1: a panic in the handler chain is recovered as a 500.
func TestRecoverMiddleware(t *testing.T) {
	s := &Server{}
	h := s.recoverMW(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "http://gateway/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", rec.Code)
	}
}
