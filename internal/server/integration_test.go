package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
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
