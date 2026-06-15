package server

import (
	"net/http"
	"net/http/httptest"
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
