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

// Security: if a non-compliant upstream returns tools/list as a non-JSON
// (e.g. SSE) response that cannot be filtered, the gateway must fail closed —
// it must NOT stream the unfiltered tool list to the caller.
func TestToolsListNonJSONFailsClosed(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"run_command\"}]}}\n\n")
	}))
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")

	// k-limited may only call read_file; run_command must never be exposed.
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
