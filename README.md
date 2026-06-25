<div align="center">

<img src="https://raw.githubusercontent.com/bouncei/mcpgate/main/site/og.png" alt="mcpgate — a lock for your MCP server" width="720" />

# mcpgate

**A drop-in auth gateway for self-hosted MCP servers.**

Add API-key authentication, per-tool authorization, rate limiting, and an audit log
in front of any [Model Context Protocol](https://modelcontextprotocol.io) server —
**without changing a line of the upstream.**

[![CI](https://github.com/bouncei/mcpgate/actions/workflows/ci.yml/badge.svg)](https://github.com/bouncei/mcpgate/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bouncei/mcpgate)](https://goreportcard.com/report/github.com/bouncei/mcpgate)
![Go version](https://img.shields.io/github/go-mod/go-version/bouncei/mcpgate)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Website](https://img.shields.io/badge/site-mcpgate.bouncei.dev-E0A23B)](https://mcpgate.bouncei.dev)

[Website](https://mcpgate.bouncei.dev) · [Quickstart](#quickstart) · [How it works](#how-it-works) · [Configuration](#configuration) · [Contributing](#contributing)

</div>

---

## Why

MCP servers are landing on the network faster than they're being secured. Internet scans in 2026 found **tens of thousands of MCP servers exposed with no authentication at all** — answering tool calls that read files, query databases, and run shell commands for anyone who connects. The protocol doesn't require auth by default, and most servers ship without it.

You shouldn't have to fork or rewrite a server to lock it down. `mcpgate` is a small reverse proxy you drop **in front of** an existing MCP server: it authenticates every request, enforces which tools each caller may use, rate-limits abuse, and writes an audit trail — while the upstream server stays exactly as it is.

## Features

- 🔑 **API-key authentication** — keys are SHA-256 hashed at rest; unauthenticated requests get `401` before they ever reach your server.
- 🔒 **Per-tool allowlist (deny by default)** — each key may call only the tools you name. Disallowed tools are also stripped from `tools/list`, so clients never even see them.
- 🚦 **Per-key rate limiting** — token-bucket limits per API key; a noisy or runaway client gets `429`, not a melted upstream.
- 📋 **Structured audit log** — one JSON line per request: who, which tool, the decision, status, and latency. Pipe it anywhere.
- 🌊 **Streamable HTTP + SSE** — tool calls and Server-Sent Events stream through untouched; `tools/list` is filtered whether the server answers in JSON or SSE.
- 📦 **Single static binary** — run it as a sidecar, one instance per upstream. No runtime, no database.

## How it works

```
                     ┌──────────────── mcpgate ────────────────┐
 MCP client  ──────► │  auth → rate-limit → policy → proxy      │ ──────►  upstream
 (Bearer key)        │                       │  ▲ tools/list     │          MCP server
                     └───────────────────────┼──┴────────────────┘          (unchanged)
                                              ▼
                                       audit log (JSON)
```

Point your MCP client at mcpgate instead of the server directly. Every request runs the chain; allowed requests are proxied through. `mcpgate` parses the JSON-RPC layer to enforce **per-tool** policy — it blocks disallowed `tools/call` with a JSON-RPC error and filters disallowed tools out of `tools/list` responses. It targets the MCP spec revision `2025-06-18`.

## Install

```bash
# Go 1.25+
go install github.com/bouncei/mcpgate/cmd/mcpgate@latest
```

Or build from source:

```bash
git clone https://github.com/bouncei/mcpgate.git
cd mcpgate
go build -o mcpgate ./cmd/mcpgate
```

Or with Docker:

```bash
docker build -t mcpgate .
docker run -p 8080:8080 -v "$PWD/config.yaml:/etc/mcpgate/config.yaml" mcpgate
```

> Prebuilt cross-platform binaries and a published container image ship with the first tagged release.

## Quickstart

Protect an MCP server running at `http://localhost:9000/mcp` in about 30 seconds:

```bash
# 1. Mint an API key (prints the key once + a ready-to-paste config entry)
mcpgate keygen --label claude-desktop

# 2. Drop the printed hash into config.yaml
cat > config.yaml <<'EOF'
upstream:
  url: "http://localhost:9000/mcp"
listen: ":8080"
keys:
  - label: "claude-desktop"
    hash: "<paste the hash from keygen>"
    allow: ["read_file", "list_dir"]   # deny by default
EOF

# 3. Validate and run
mcpgate validate -c config.yaml
mcpgate serve -c config.yaml
```

Now point your MCP client at `http://localhost:8080/mcp` and send the key as an
`Authorization: Bearer <key>` header. Requests without a valid key get `401`; calls to
tools outside the allowlist are rejected before they reach your server.

## Configuration

A single `config.yaml`, reviewable in a PR, with secrets hashed at rest. See [`examples/config.yaml`](examples/config.yaml).

| Key | Description |
|-----|-------------|
| `upstream.url` | The MCP endpoint mcpgate forwards to. **Required.** |
| `upstream.timeout` | Upstream response timeout (e.g. `30s`). |
| `listen` | Address mcpgate listens on. Default `:8080`. |
| `keys[].label` | Human-readable name for the key (appears in the audit log). |
| `keys[].hash` | SHA-256 hex of the API key, from `mcpgate keygen`. |
| `keys[].allow` | Permitted tool names. `["*"]` for all; empty/omitted denies everything. |
| `keys[].rate_limit` | Optional per-key `{ rps, burst }`; falls back to `defaults.rate_limit`. |
| `defaults.rate_limit` | Default token-bucket `{ rps, burst }` for keys without their own. |
| `audit.output` | `stdout` (default) or a file path. |

## CLI

| Command | Description |
|---------|-------------|
| `mcpgate keygen --label <name>` | Generate a new API key and a config entry. |
| `mcpgate validate -c config.yaml` | Validate a config file. |
| `mcpgate serve -c config.yaml` | Run the gateway. |
| `mcpgate version` | Print the version. |

## Security model

- **Deny by default.** A tool runs only if it's in the caller's allowlist (or `*`).
- **No leakage.** Disallowed tools are removed from `tools/list` — in JSON *and* SSE responses — so clients can't discover them.
- **Fail closed.** If a `tools/list` response can't be filtered, mcpgate refuses it rather than risk leaking the full tool list.
- **Hashed keys.** Plaintext keys are never stored or logged; only SHA-256 hashes live in config.
- **Bounded input.** Request bodies are size-limited; every request emits exactly one audit event.

Found a security issue? Please open a private advisory rather than a public issue.

## Roadmap

Planned, not yet shipped: stdio transport wrapping · OAuth2/OIDC · mTLS · multiple upstreams per instance · configurable body limits.

## Contributing

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). The repo is plain Go with no external services:

```bash
go test ./...     # full suite
go vet ./...       # static checks
go build ./...     # build
```

## License

[Apache-2.0](LICENSE).
