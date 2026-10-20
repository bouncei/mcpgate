package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// New builds a reverse proxy that routes every incoming request to the single
// upstream MCP endpoint. SSE is supported via immediate flushing (FlushInterval = -1).
//
// The client's Authorization header carries its mcpgate API key, so it is
// always stripped before forwarding. headers are then set on every upstream
// request, letting operators give the upstream its own credential.
func New(upstreamURL string, timeout time.Duration, headers map[string]string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream url %q: %w", upstreamURL, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("upstream url must be absolute, got %q", upstreamURL)
	}

	upstreamHeaders := make(http.Header, len(headers))
	for k, v := range headers {
		upstreamHeaders.Set(k, v)
	}

	rp := &httputil.ReverseProxy{
		// Rewrite (not Director) runs after hop-by-hop headers are removed, so
		// a client can't strip the configured headers by naming them in its
		// Connection header.
		Rewrite: func(pr *httputil.ProxyRequest) {
			out := pr.Out
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.URL.Path = target.Path
			out.URL.RawPath = target.RawPath
			out.URL.RawQuery = target.RawQuery
			out.Host = target.Host
			pr.SetXForwarded()
			out.Header.Del("Authorization")
			for k, v := range upstreamHeaders {
				out.Header[k] = v
			}
		},
		FlushInterval: -1, // flush immediately for SSE streaming
		Transport: &http.Transport{
			ResponseHeaderTimeout: timeout,
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	return rp, nil
}
