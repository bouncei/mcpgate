# Contributing to mcpgate

Thank you for your interest in contributing. This document covers how to run tests, the package layout, and the invariants you must preserve.

## Running tests

```bash
# Standard test run
go test ./...

# With race detector (required before submitting a PR)
go test ./... -race

# With verbose output
go test ./... -v -race -count=1
```

All tests must pass with `-race` before a pull request will be merged.

## Package layout

```
cmd/mcpgate/          Entry point — wires up the CLI root command and calls os.Exit.
internal/
  config/             YAML config loading and validation (Config, Key, Upstream, …).
  auth/               API-key extraction and SHA-256 hash verification.
  jsonrpc/            JSON-RPC 2.0 types and helpers (Request, Response, Error).
  policy/             Per-tool allow/deny enforcement and tools/list filtering.
  ratelimit/          Per-key token-bucket rate limiter (wraps golang.org/x/time/rate).
  audit/              Structured JSON audit event emission (stdout or file).
  proxy/              Reverse-proxy logic — forwards requests to the upstream MCP server.
  server/             HTTP server wiring: middleware chain, route registration, SSE handling.
  cli/                Cobra commands: serve, validate, keygen, version.
```

## Invariants contributors MUST preserve

**1. Deny-by-default authorization.**
A tool call is permitted only when the authenticated key's `allow` list contains the exact tool name or the wildcard `"*"`. An empty or absent `allow` list means the key may call *no* tools. This invariant must hold for every code path — do not add opt-in permissive modes or fallback allows.

**2. MCP spec 2025-06-18 — single-message JSON-RPC, no batching.**
mcpgate targets the Streamable HTTP + SSE transport defined in the MCP specification dated 2025-06-18. JSON-RPC batch requests (arrays at the top level) are explicitly out of scope and must be rejected with a `-32600 Invalid Request` error. Do not add batch support without first updating this document and the spec reference in the README.

**3. Every request emits exactly one audit event.**
Each inbound MCP request must produce exactly one structured audit log entry — whether the request is allowed, denied (auth failure, policy denial, rate-limit), or results in an upstream error. Do not skip audit emission in error paths, and do not emit more than one event per request (e.g., do not emit both a "denied" and an "error" event for the same request).

## Submitting changes

1. Fork the repo and create a feature branch off `main`.
2. Make your changes, add or update tests.
3. Run `go test ./... -race` and `go vet ./...` — both must be clean.
4. Open a pull request against `main` with a clear description of what changed and why.

## Code style

- `gofmt`/`goimports` formatting is expected.
- Keep exported symbols documented with Go doc comments.
- Prefer small, focused commits over large omnibus changes.
