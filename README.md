# mcpgate

Drop-in auth gateway for self-hosted MCP servers.

---

## The problem

MCP servers are increasingly deployed inside organizations to give AI assistants access to internal tools — file systems, databases, shell commands, APIs. But the MCP protocol requires no authentication by default, and security scans conducted in 2026 found more than 12,000 internet-exposed MCP servers with no auth at all. Any client that can reach the port can call any tool. Running one of these servers on a machine with broad filesystem or database access means a single reachable port is enough to compromise it.

## What mcpgate does

mcpgate sits as a reverse proxy in front of one MCP server (Streamable HTTP and SSE transport) and adds the following without any changes to the upstream server:

- **API-key authentication** — requests without a valid `Authorization: Bearer <key>` header are rejected before they reach the upstream.
- **Per-tool allowlist authorization (deny by default)** — each key declares exactly which tools it may call. A key with an empty `allow` list can call nothing. Disallowed tools are filtered out of `tools/list` responses so clients cannot even see them.
- **Per-key rate limiting** — token-bucket rate limiting (configurable RPS and burst) enforced per API key.
- **Structured JSON audit log** — every request produces exactly one audit event (allowed or denied) written to stdout or a file.

## Quickstart

**1. Install**

```bash
# From source
go install github.com/bouncei/mcpgate/cmd/mcpgate@latest

# Docker
docker pull ghcr.io/bouncei/mcpgate

# Or download a release binary from the GitHub Releases page
```

**2. Generate a key**

```bash
mcpgate keygen --label my-client
```

This prints a raw API key (give this to the client) and a ready-to-paste config block containing the SHA-256 hash. Never store the raw key in config — only the hash goes in `config.yaml`.

**3. Create `config.yaml`**

See [`examples/config.yaml`](examples/config.yaml) for a fully annotated example. Minimal working config:

```yaml
upstream:
  url: "http://localhost:9000/mcp"
  timeout: 30s

listen: ":8080"

keys:
  - label: "my-client"
    hash: "<paste hash from keygen output>"
    allow: ["read_file", "list_dir"]
```

**4. Validate and run**

```bash
mcpgate validate -c config.yaml
# OK: 1 key(s), upstream http://localhost:9000/mcp

mcpgate serve -c config.yaml
```

**5. Point your MCP client at mcpgate**

Configure your MCP client to connect to `http://localhost:8080/mcp` and include the header:

```
Authorization: Bearer <raw key from keygen>
```

The client will see only the tools its key is allowed to call.

**6. Docker one-liner**

```bash
docker run \
  -v $PWD/config.yaml:/etc/mcpgate/config.yaml \
  -p 8080:8080 \
  ghcr.io/bouncei/mcpgate
```

---

## Config reference

```yaml
upstream:
  url: "http://localhost:9000/mcp"   # Required. URL of the upstream MCP server.
  timeout: 30s                       # Optional. HTTP timeout for upstream requests. Default: 30s.

listen: ":8080"                      # Optional. Address mcpgate listens on. Default: :8080.

keys:
  - label: "claude-desktop"          # Required. Human-readable name; appears in audit logs.
    hash: "<sha256-hex>"             # Required. SHA-256 hex of the raw API key.
    allow:                           # Required. Tools this key may call.
      - "read_file"                  #   List tool names explicitly, or use ["*"] for all tools.
      - "list_dir"                   #   An empty list means the key can call no tools.
    rate_limit:                      # Optional. Overrides defaults.rate_limit for this key.
      rps: 5                         #   Sustained requests per second.
      burst: 10                      #   Maximum burst size.

defaults:
  rate_limit:                        # Applied to keys that do not specify their own rate_limit.
    rps: 2
    burst: 5

audit:
  output: stdout                     # Where to write audit events. "stdout" or a file path.
```

---

## How it works

Every inbound HTTP request passes through a middleware chain before reaching the upstream:

```
client request
  → auth middleware        (verify Bearer token against key hashes)
  → rate-limit middleware  (token-bucket check for the matched key)
  → policy middleware      (check tool name against key's allow list)
  → proxy                  (forward to upstream MCP server)
  → tools/list filter      (strip disallowed tools from list responses)
  → audit                  (emit one structured JSON event)
```

Tool-level enforcement works by inspecting the JSON-RPC method and `params.name` field on `tools/call` requests. For `tools/list` responses, mcpgate rewrites the upstream's response body to remove any tool the key is not permitted to call — so restricted tools are invisible to the client, not just blocked.

mcpgate targets the MCP specification dated 2025-06-18 (single-message JSON-RPC bodies over Streamable HTTP and SSE). JSON-RPC batch requests are rejected.

---

## Limitations (v1)

- **Single upstream per instance.** Each mcpgate process protects exactly one MCP server. Run one instance per server.
- **Streamable HTTP + SSE only.** stdio-wrapped MCP servers are not supported yet.
- **API-key auth only.** OAuth2/OIDC and mTLS are planned but not implemented.
- **No capability rewriting.** mcpgate does not modify the upstream's declared capabilities beyond filtering `tools/list`.

---

## License

Apache-2.0. See [LICENSE](LICENSE).
