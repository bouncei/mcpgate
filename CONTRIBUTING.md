# Contributing to mcpgate

Thanks for your interest in improving mcpgate.

## Development

```bash
go test ./...        # run the full suite
go vet ./...         # static checks
go build ./...       # build
go run ./cmd/mcpgate keygen --label dev   # mint a key
```

## Layout

- `cmd/mcpgate` — CLI entrypoint
- `internal/config` — YAML config: schema, loading, validation, defaults
- `internal/auth` — API-key authentication (SHA-256 hashes)
- `internal/jsonrpc` — JSON-RPC parsing, error envelopes, `tools/list` filtering
- `internal/policy` — per-tool allowlist (deny by default)
- `internal/ratelimit` — per-key token-bucket limiting
- `internal/audit` — structured JSON audit logging
- `internal/proxy` — single-upstream reverse proxy (SSE-aware)
- `internal/server` — the middleware chain tying it all together

## Invariants to preserve

- **Deny by default:** a tool runs only if it is in the caller's allowlist (or `*`).
- **Targets MCP spec 2025-06-18:** request bodies are single JSON-RPC messages; batched (array) bodies are rejected.
- **One audit event per request**, on every path.
- New behavior comes with a test (TDD).
