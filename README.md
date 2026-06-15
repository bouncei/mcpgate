# mcpgate

**A drop-in auth gateway for self-hosted MCP servers.**

MCP servers are landing on the network faster than they're being secured — recent scans found tens of thousands of internet-exposed MCP servers with *no authentication at all*, exposing tools that can read files, query databases, and run commands to anyone who connects. The MCP protocol doesn't require auth by default.

`mcpgate` is a small reverse proxy you put in front of an existing MCP server to add authentication, per-tool authorization, rate limiting, and an audit log — **without changing the upstream server's code**.

## Features

- **API-key authentication** — keys are SHA-256 hashed at rest; unauthenticated requests get `401`.
- **Per-tool allowlist (deny by default)** — each key may call only the tools you list. Blocked tools are also hidden from `tools/list`, so clients never see them.
- **Per-key rate limiting** — token-bucket limits per API key; `429` when exceeded.
- **Structured audit log** — one JSON line per request (who, tool, decision, status, latency).
- **SSE-aware** — streams Server-Sent Events through untouched.
- **Single static binary** — run it as a sidecar, one instance per upstream.

## Quickstart

Protect an MCP server running at `http://localhost:9000/mcp` in 30 seconds:

```bash
# 1. Mint an API key (prints the key once + a config snippet)
mcpgate keygen --label claude-desktop

# 2. Create config.yaml with the printed hash
cat > config.yaml <<'EOF'
upstream:
  url: "http://localhost:9000/mcp"
listen: ":8080"
keys:
  - label: "claude-desktop"
    hash: "<paste the hash from keygen>"
    allow: ["read_file", "list_dir"]
EOF

# 3. Validate and run
mcpgate validate -c config.yaml
mcpgate serve -c config.yaml

# Or via Docker:
# docker run -p 8080:8080 -v $PWD/config.yaml:/etc/mcpgate/config.yaml ghcr.io/bouncei/mcpgate
```

Now point your MCP client at `http://localhost:8080/mcp` and send the key as `Authorization: Bearer <key>`.

## Configuration

See [`examples/config.yaml`](examples/config.yaml). Keys:

- `upstream.url` — the MCP endpoint mcpgate forwards to (required).
- `listen` — address mcpgate listens on (default `:8080`).
- `keys[].hash` — SHA-256 hex of the API key (from `mcpgate keygen`).
- `keys[].allow` — list of permitted tool names; `["*"]` for all; empty/omitted denies everything.
- `keys[].rate_limit` — optional per-key `{ rps, burst }`; falls back to `defaults.rate_limit`.
- `audit.output` — `stdout` (default) or a file path.

## CLI

- `mcpgate keygen --label <name>` — generate a key + config entry.
- `mcpgate validate -c config.yaml` — check a config file.
- `mcpgate serve -c config.yaml` — run the gateway.
- `mcpgate version` — print the version.

## Limitations (v1)

- One upstream per instance (run multiple instances for multiple servers).
- Streamable HTTP + SSE transports only — stdio wrapping, OAuth2/OIDC, and mTLS are planned.

## License

Apache-2.0.
