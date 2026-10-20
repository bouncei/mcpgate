---
name: mcpgate
description: Put an auth gateway in front of a self-hosted MCP server with mcpgate — per-client API keys, per-tool allowlists (deny by default), rate limits, and a JSON audit log of every tool call — then connect Claude Code, Cursor or any MCP client through it. Use this whenever the user wants to secure, lock down, gate, or add authentication/API keys to an MCP server, restrict which MCP tools an agent or teammate can call, audit or log MCP tool calls, rate-limit MCP clients, rotate a leaked MCP key, debug "tool not permitted" / 401 / 429 errors from a gateway, or mentions mcpgate — even if they don't name mcpgate explicitly.
---

# mcpgate

mcpgate is a single Go binary that sits between MCP clients and **one** self-hosted MCP server:

```
client ──Bearer key──▶ mcpgate :8080 ──▶ upstream MCP server (Streamable HTTP / SSE)
                         │ auth → rate limit → tool allowlist → proxy
                         └▶ one JSON audit line per request
```

- **Auth:** each client gets its own API key (`mcpg_…`); only the SHA-256 hash goes in the config.
- **Policy:** each key has an `allow` list of exact tool names (or `"*"`). Everything else is denied, *and hidden* from `tools/list`.
- **Audit:** every request → one JSON line with `label`, `method`, `tool`, `decision`, `status`, `latency_ms`.

Repo: https://github.com/bouncei/mcpgate (Apache-2.0).

## Check fit before you start

mcpgate fits when the upstream speaks **Streamable HTTP or SSE** (it has a URL like `http://host:port/mcp`). Tell the user up front if any of these apply, because they change the plan:

| Situation | What to do |
|---|---|
| Upstream is a **stdio** server (`command` + `args` in client config) | Not supported directly. Run it behind a stdio→Streamable-HTTP bridge first, then point mcpgate at the bridge's URL. |
| Needs OAuth/OIDC or mTLS for clients | Not supported (roadmap). mcpgate does static Bearer API keys only. |
| Several upstream servers | One mcpgate instance per upstream, each on its own port. |
| Must gate MCP **resources/prompts** | Only `tools/call` and `tools/list` are policed; resources and prompts pass through for any valid key. Say so. |
| Must restrict tool **arguments** (read-only SQL, certain paths) | Policy is per tool *name*. A generic tool like `query` or `run_command` is all-or-nothing. Enforce argument-level limits in the upstream itself (e.g. a read-only DB role) and use mcpgate for who-can-call-what. |
| Exposed beyond localhost | mcpgate serves plain HTTP. Put TLS in front (Caddy, nginx, a cloud LB) — never send Bearer keys over plain HTTP across a network. |

## Setup workflow

Work through these in order. Each step builds on the previous one.

### 1. Install

```bash
bash scripts/install.sh            # prebuilt binary for this OS/arch → ~/.local/bin (or $MCPGATE_INSTALL_DIR)
mcpgate version
```

The script verifies the release checksum. If it can't download, it tells you the fallbacks: `go install github.com/bouncei/mcpgate/cmd/mcpgate@latest` (Go 1.25+) or Docker (`docker build -t mcpgate https://github.com/bouncei/mcpgate.git`).

### 2. Discover the upstream's exact tool names

Allowlists are **exact string matches** — no globs, no prefixes. A typo silently denies the tool. So get the real names from the upstream instead of guessing:

```bash
python3 scripts/discover_tools.py http://localhost:9000/mcp
```

It runs the MCP handshake against the upstream, prints every tool with its description and annotations (read-only / destructive hints), and prints suggested `allow:` lines. If the upstream itself needs a header, pass `-H "Authorization: Bearer …"`.

### 3. Decide who gets what

Ask (or infer from the request) which clients exist and what each needs. Default to least privilege:
- One key **per client/person/bot**, with a descriptive `label` (`claude-code-alice`, `ci-bot`). Labels show up in the audit log and rate limits are tracked **per label**, so never reuse a label for two keys.
- Read-only clients get only read-ish tools (use the `readOnlyHint` annotations from step 2).
- Use `"*"` only when the user explicitly wants a client to have everything — and remember it also admits tools the upstream adds later.

### 4. Mint keys

```bash
mcpgate keygen --label claude-code-alice
```

This prints the plaintext key **once** and a ready-to-paste config entry (`label`, `hash`, `allow: []`). The plaintext key is never recoverable from the config. Handle it like a password:
- Give it to the user (or put it in the client's env var / secret store). Don't write it into config.yaml, committed files, or logs.
- If you lose it before handing it over, just run keygen again.

### 5. Write config.yaml

```yaml
upstream:
  url: "http://localhost:9000/mcp"
  timeout: 30s
listen: "127.0.0.1:8080"          # localhost only; use ":8080" only behind TLS / on a trusted network
keys:
  - label: "claude-code-alice"
    hash: "<sha256 from keygen>"
    allow: ["read_file", "list_directory", "search_files"]
    rate_limit: { rps: 5, burst: 20 }
  - label: "ci-bot"
    hash: "<sha256 from keygen>"
    allow: ["*"]
defaults:
  rate_limit: { rps: 2, burst: 5 }
audit:
  output: "./mcpgate-audit.jsonl"  # or stdout (default)
```

Rate limits count every HTTP request, not just tool calls. A client's connect sequence alone (initialize, the initialized notification, tools/list, often prompts/list, resources/list and a GET stream) can use most of the default `burst: 5`, and the 6th rapid request gets a 429. So give interactive keys a burst around 20. Keys without their own `rate_limit` inherit `defaults.rate_limit` (built-in default 2 rps / burst 5). They're never unlimited. Full field reference: `references/config.md`.

The gateway is only a boundary if clients can't reach the upstream directly. Bind the upstream to localhost or a private interface, and remove any existing client entries that point straight at it.

If the upstream needs its own credential, put it under `upstream.headers` (e.g. `Authorization: "Bearer ${UPSTREAM_TOKEN}"`, read from mcpgate's environment). mcpgate strips the client's `Authorization` header before forwarding, so client keys never reach the upstream.

Keep `config.yaml` out of git (add it to `.gitignore`) — the hashes aren't secrets in the password sense, but the file is deployment state, not source.

### 6. Validate and run

```bash
mcpgate validate -c config.yaml   # → OK: 2 key(s), upstream http://localhost:9000/mcp
mcpgate serve -c config.yaml      # → mcpgate listening on 127.0.0.1:8080 -> http://localhost:9000/mcp
```

`validate` does not catch typos in field names (unknown YAML keys are ignored) or misspelled tool names, so re-read the config against step 2's output. mcpgate has no hot reload: **restart `serve` after every config change**. For a long-running setup, offer a systemd unit, launchd plist, or `docker run --restart unless-stopped` — ask which the user wants rather than picking.

### 7. Connect the client

Point the client at the gateway (any path works — mcpgate forwards everything to the upstream's path) and send `Authorization: Bearer <key>`. Keep the key in an environment variable instead of pasting it into a config file that might be committed.

Claude Code:
```bash
claude mcp add --transport http files http://127.0.0.1:8080/mcp --header "Authorization: Bearer $MCPGATE_KEY"
```
Cursor (`.cursor/mcp.json` or `~/.cursor/mcp.json`):
```json
{ "mcpServers": { "files": { "url": "http://127.0.0.1:8080/mcp",
  "headers": { "Authorization": "Bearer ${env:MCPGATE_KEY}" } } } }
```
Shared `.mcp.json`, other clients, and a curl smoke test: `references/clients.md`.

### 8. Smoke test

Prove both directions before calling it done: an allowed tool call succeeds, a disallowed one returns JSON-RPC error `-32001 tool not permitted: <name>`, and `tools/list` through the gateway shows only the allowed tools. The curl sequence is in `references/clients.md`. Then check the audit log shows `"decision":"allow"` and `"decision":"deny:policy"` lines for the right label.

## Day-2 operations

**Add a client:** keygen with a new label → append the entry → validate → restart → connect.

**Rotate or revoke a key** (leaked, person left, routine rotation):
1. `mcpgate keygen --label <same-or-new-label>` and replace the old entry's `hash` (or delete the entry to revoke outright).
2. `validate` → **restart**. Until the restart, the old key still works — say this explicitly.
3. Hand the new key to the legitimate owner.
4. Review what the old key did: `python3 scripts/audit_summary.py mcpgate-audit.jsonl --label <label> --since 7d`. Note that `deny:auth` lines carry no label (the key was unknown), so failed attempts with the old key after rotation show up as unlabeled `deny:auth`.

**Tighten an allowlist:** `python3 scripts/audit_summary.py <log> --config config.yaml` shows, per label, which allowed tools were never called and which tools were denied. Propose removing unused tools; confirm with the user before removing anything a person relies on.

**Grant a missing tool** (client sees `tool not permitted: X`): add the exact name `X` to that key's `allow`, validate, restart. Check with the user first if `X` is destructive (writes, deletes, shell exec).

**Audit questions** ("what did ci-bot call yesterday?", "any denied calls?"): use `scripts/audit_summary.py` with `--label`, `--since`, `--decision`. The log is plain JSON lines, so `jq` works too: `jq -c 'select(.decision|startswith("deny"))' mcpgate-audit.jsonl`.

## When something fails

Read the audit line for the request first — `decision` says exactly which stage stopped it. The common ones:

| Symptom | decision | Fix |
|---|---|---|
| HTTP 401 `unauthorized` | `deny:auth` | Header must be exactly `Authorization: Bearer <key>` (capital B, one space). Key/hash mismatch, or gateway not restarted after editing. |
| HTTP 429 | `deny:ratelimit` | Raise that key's `burst`/`rps`; interactive clients need a burst ~20. |
| JSON-RPC `-32001 tool not permitted: X` | `deny:policy` | `X` not in that key's `allow` (exact match). |
| Tool missing from the client's tool list | (allow) | Same cause — disallowed tools are filtered out of `tools/list`. |
| HTTP 502 | `allow` + status 502 | Upstream down/wrong URL, or a `tools/list` response mcpgate couldn't filter (it fails closed). |

Full table, including `deny:parse`, `deny:params`, `deny:read`, and batch requests: `references/troubleshooting.md`.

## Reference files

- `references/config.md` — every config field, defaults, validation rules.
- `references/clients.md` — Claude Code, Cursor, `.mcp.json`, generic clients, curl smoke test.
- `references/troubleshooting.md` — all decisions/status codes, limitations, gotchas.
- `scripts/install.sh` — install the release binary (checksum-verified).
- `scripts/discover_tools.py` — list an MCP server's exact tool names (stdlib Python 3.8+).
- `scripts/audit_summary.py` — summarize the audit log (stdlib Python 3.8+).
