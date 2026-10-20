package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyForwardsToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("upstream path = %q, want /mcp", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	p, err := New(upstream.URL+"/mcp", 5*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Client hits the gateway at a different path; proxy must route to /mcp.
	req := httptest.NewRequest(http.MethodPost, "http://gateway/anything", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestProxyUpstreamDownReturns502(t *testing.T) {
	// Point at a closed port.
	p, err := New("http://127.0.0.1:0/mcp", 1*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://gateway/", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestProxyUpstreamHeaders(t *testing.T) {
	const clientAuth = "Bearer mcpg_clientkey"
	cases := []struct {
		name    string
		headers map[string]string
		conn    string      // client-sent Connection header, if any
		want    http.Header // headers the upstream must see (exact values)
		absent  []string    // headers the upstream must not see
	}{
		{
			name:   "client authorization is stripped",
			absent: []string{"Authorization"},
		},
		{
			name:    "configured authorization replaces the client's",
			headers: map[string]string{"Authorization": "Bearer upstream-secret"},
			want:    http.Header{"Authorization": {"Bearer upstream-secret"}},
		},
		{
			name:    "other configured headers are set and client auth still stripped",
			headers: map[string]string{"X-Api-Key": "upstream-key"},
			want:    http.Header{"X-Api-Key": {"upstream-key"}},
			absent:  []string{"Authorization"},
		},
		{
			name:    "configured header overrides a client-sent value",
			headers: map[string]string{"X-Tenant": "configured"},
			want:    http.Header{"X-Tenant": {"configured"}},
		},
		{
			// Hop-by-hop stripping of Connection-named headers must not let a
			// client remove a configured header.
			name:    "client cannot strip configured headers via Connection",
			headers: map[string]string{"Authorization": "Bearer upstream-secret", "X-Api-Key": "upstream-key"},
			conn:    "Authorization, X-Api-Key",
			want:    http.Header{"Authorization": {"Bearer upstream-secret"}, "X-Api-Key": {"upstream-key"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got http.Header
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
			}))
			defer upstream.Close()

			p, err := New(upstream.URL+"/mcp", 5*time.Second, tc.headers)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "http://gateway/mcp", nil)
			req.Header.Set("Authorization", clientAuth)
			req.Header.Set("X-Tenant", "client")
			if tc.conn != "" {
				req.Header.Set("Connection", tc.conn)
			}
			rec := httptest.NewRecorder()
			p.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}

			for _, v := range got.Values("Authorization") {
				if v == clientAuth {
					t.Errorf("upstream received the client's mcpgate key")
				}
			}
			for k, vs := range tc.want {
				if g := got.Values(k); len(g) != 1 || g[0] != vs[0] {
					t.Errorf("upstream %s = %q, want %q", k, g, vs)
				}
			}
			for _, k := range tc.absent {
				if v := got.Values(k); len(v) != 0 {
					t.Errorf("upstream %s = %q, want absent", k, v)
				}
			}
		})
	}
}
