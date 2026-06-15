package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"github.com/bouncei/mcpgate/internal/audit"
	"github.com/bouncei/mcpgate/internal/auth"
	"github.com/bouncei/mcpgate/internal/config"
	"github.com/bouncei/mcpgate/internal/jsonrpc"
	"github.com/bouncei/mcpgate/internal/policy"
	"github.com/bouncei/mcpgate/internal/proxy"
	"github.com/bouncei/mcpgate/internal/ratelimit"
)

// filterKey carries the caller's allowlist into the proxy's ModifyResponse,
// signalling that this response is a tools/list that must be filtered.
type filterKey struct{}

type Server struct {
	auth    *auth.Authenticator
	limiter *ratelimit.Limiter
	audit   *audit.Logger
	proxy   *httputil.ReverseProxy
}

func New(cfg *config.Config) (*Server, error) {
	rp, err := proxy.New(cfg.Upstream.URL, cfg.Upstream.Timeout.Std())
	if err != nil {
		return nil, err
	}
	aud, err := audit.New(cfg.Audit.Output)
	if err != nil {
		return nil, err
	}
	s := &Server{
		auth:    auth.New(cfg),
		limiter: ratelimit.New(),
		audit:   aud,
		proxy:   rp,
	}
	// Filter tools/list responses to the caller's allowlist.
	rp.ModifyResponse = func(resp *http.Response) error {
		allow, ok := resp.Request.Context().Value(filterKey{}).([]string)
		if !ok {
			return nil // not a tools/list request
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		filtered, err := jsonrpc.FilterToolsList(body, func(name string) bool {
			return policy.Allowed(allow, name)
		})
		if err != nil {
			filtered = body // unparseable: pass through rather than break the client
		}
		resp.Body = io.NopCloser(bytes.NewReader(filtered))
		resp.ContentLength = int64(len(filtered))
		resp.Header.Set("Content-Length", strconv.Itoa(len(filtered)))
		return nil
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.recoverMW(http.HandlerFunc(s.serve))
}

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	id, ok := s.auth.Authenticate(bearer(r))
	if !ok {
		s.audit.Decision(audit.Event{Decision: "deny:auth", Status: 401, Method: r.Method, Latency: time.Since(start)})
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !s.limiter.Allow(id.Label, id.RateLimit.RPS, id.RateLimit.Burst) {
		w.Header().Set("Retry-After", "1")
		s.audit.Decision(audit.Event{Label: id.Label, Decision: "deny:ratelimit", Status: 429, Method: r.Method, Latency: time.Since(start)})
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Non-POST (GET SSE listen / DELETE session terminate) carries no JSON-RPC
	// request body to inspect — authenticate and proxy through.
	if r.Method != http.MethodPost {
		s.audit.Decision(audit.Event{Label: id.Label, Decision: "allow", Status: 200, Method: r.Method, Latency: time.Since(start)})
		s.proxy.ServeHTTP(w, r)
		return
	}

	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		s.audit.Decision(audit.Event{Label: id.Label, Decision: "deny:read", Status: 400, Method: r.Method, Latency: time.Since(start)})
		return
	}
	msg, err := jsonrpc.Parse(body)
	if err != nil {
		writeJSONRPCError(w, http.StatusBadRequest, nil, jsonrpc.CodeInvalidRequest, err.Error())
		s.audit.Decision(audit.Event{Label: id.Label, Decision: "deny:parse", Status: 400, Latency: time.Since(start)})
		return
	}
	// Restore the body for the proxy.
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	switch msg.Method {
	case "tools/call":
		tool, ok := msg.ToolName()
		if !ok {
			writeJSONRPCError(w, http.StatusOK, msg.ID, jsonrpc.CodeInvalidParams, "missing tool name")
			s.audit.Decision(audit.Event{Label: id.Label, Method: "tools/call", Decision: "deny:params", Status: 200, Latency: time.Since(start)})
			return
		}
		if !policy.Allowed(id.Allow, tool) {
			writeJSONRPCError(w, http.StatusOK, msg.ID, jsonrpc.CodeToolNotPermitted, "tool not permitted: "+tool)
			s.audit.Decision(audit.Event{Label: id.Label, Method: "tools/call", Tool: tool, Decision: "deny:policy", Status: 200, Latency: time.Since(start)})
			return
		}
		s.audit.Decision(audit.Event{Label: id.Label, Method: "tools/call", Tool: tool, Decision: "allow", Status: 200, Latency: time.Since(start)})
		s.proxy.ServeHTTP(w, r)

	case "tools/list":
		// Force a single JSON response from upstream so we can filter it.
		r.Header.Set("Accept", "application/json")
		ctx := context.WithValue(r.Context(), filterKey{}, id.Allow)
		s.audit.Decision(audit.Event{Label: id.Label, Method: "tools/list", Decision: "allow", Status: 200, Latency: time.Since(start)})
		s.proxy.ServeHTTP(w, r.WithContext(ctx))

	default:
		// initialize, notifications, ping, etc. — proxy through unchanged.
		s.audit.Decision(audit.Event{Label: id.Label, Method: msg.Method, Decision: "allow", Status: 200, Latency: time.Since(start)})
		s.proxy.ServeHTTP(w, r)
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func writeJSONRPCError(w http.ResponseWriter, status int, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(jsonrpc.ErrorResponse(id, code, msg))
}
