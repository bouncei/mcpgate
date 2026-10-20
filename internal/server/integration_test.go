package server

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// sseUpstream streams an SSE response for tools/call to verify pass-through.
func sseUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"streamed\"}]}}\n\n"))
		if fl != nil {
			fl.Flush()
		}
	}))
}

func TestSSEResponseStreamsThrough(t *testing.T) {
	up := sseUpstream(t)
	defer up.Close()
	h := newTestServer(t, up.URL+"/mcp")

	rec := post(t, h, "k-full", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"anything"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q, want SSE", ct)
	}
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	var sawData bool
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data:") && strings.Contains(sc.Text(), "streamed") {
			sawData = true
		}
	}
	if !sawData {
		t.Errorf("expected streamed SSE data, got %s", rec.Body.String())
	}
}

// TestClientKeyNeverReachesUpstream verifies the client's mcpgate key is
// stripped before proxying, on every path that reaches the upstream, and that
// configured upstream.headers are sent instead.
func TestClientKeyNeverReachesUpstream(t *testing.T) {
	cases := []struct {
		name     string
		headers  map[string]string
		wantAuth string // Authorization the upstream should see; "" = none
	}{
		{name: "no upstream headers", wantAuth: ""},
		{name: "upstream authorization configured", headers: map[string]string{"Authorization": "Bearer upstream-secret"}, wantAuth: "Bearer upstream-secret"},
	}
	requests := []struct {
		method, body string
	}{
		{http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`},
		{http.MethodPost, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`},
		{http.MethodPost, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file"}}`},
		{http.MethodGet, ""},
		{http.MethodDelete, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, r.Header.Values("Authorization")...)
				if len(r.Header.Values("Authorization")) == 0 {
					seen = append(seen, "")
				}
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
			}))
			defer up.Close()
			h := newTestServerWithHeaders(t, up.URL+"/mcp", tc.headers)

			for _, rq := range requests {
				req := httptest.NewRequest(rq.method, "http://gateway/mcp", strings.NewReader(rq.body))
				req.Header.Set("Authorization", "Bearer k-full")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("%s %s: status = %d", rq.method, rq.body, rec.Code)
				}
			}

			mu.Lock()
			defer mu.Unlock()
			if len(seen) != len(requests) {
				t.Fatalf("upstream saw %d Authorization values (%q), want one per request (%d)", len(seen), seen, len(requests))
			}
			for _, v := range seen {
				if strings.Contains(v, "k-full") {
					t.Errorf("upstream received the client's mcpgate key: %q", v)
				}
				if v != tc.wantAuth {
					t.Errorf("upstream Authorization = %q, want %q", v, tc.wantAuth)
				}
			}
		})
	}
}
