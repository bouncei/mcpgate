# Troubleshooting mcpgate

Every request produces exactly one audit line. Start there:

```json
{"time":"2026-10-20T09:12:01.5+01:00","level":"INFO","msg":"request","label":"cursor-bob","method":"tools/call","tool":"write_file","decision":"deny:policy","status":200,"latency_ms":0}
```

`decision` tells you which stage stopped the request. The stages run in order: auth → rate limit → read body → parse → policy → proxy.

## Decisions

| decision | HTTP status | Client sees | Cause | Fix |
|---|---|---|---|---|
| `allow` | upstream's status | the upstream's response | Passed every check. If `status` is ≥ 400, the upstream itself returned an error. | Look at the upstream. |
| `allow` + `status: 502` | 502 | empty 502 | Upstream unreachable or wrong `upstream.url`. The upstream took longer than `upstream.timeout` to send headers. Or a `tools/list` response came back in a content type other than JSON or SSE, and mcpgate fails closed. | `curl` the upstream directly. Run `discover_tools.py` against the upstream. Raise `timeout` for slow tools. |
| `deny:auth` | 401 | `unauthorized` | Missing header; scheme not exactly `Bearer `; key not in config; config edited without a restart. **These lines have no `label`**, because the key is unknown. | Check the header spelling. Recompute the hash with `printf %s "$KEY" \| shasum -a 256` and compare it to the config. Restart. |
| `deny:ratelimit` | 429, `Retry-After: 1` | `rate limit exceeded` | The token bucket for that **label** is empty. Every HTTP request counts, including initialize, notifications and list calls. | Raise `rate_limit.burst` (about 20 for interactive clients) and/or `rps`. Make sure two keys don't share a label. |
| `deny:read` | 400 | `cannot read body` | Body larger than 16 MiB, or the connection broke mid-read. | Shrink the request. The limit isn't configurable yet. |
| `deny:parse` | 400 | JSON-RPC `-32600` | Body isn't valid JSON-RPC, **or it's a batch** (a JSON array). Batching was removed from MCP in spec 2025-06-18, so mcpgate rejects it. | Send single messages. Upgrade an old client that batches. |
| `deny:params` | 200 | JSON-RPC `-32602 missing tool name` | `tools/call` without `params.name`. | Client bug. |
| `deny:policy` | 200 | JSON-RPC `-32001 tool not permitted: <name>` | `<name>` isn't in that key's `allow` (exact, case-sensitive match). | Add the exact name, or `"*"` only if asked. Then validate and restart. |

## Symptoms that aren't obvious from the table

- **A tool is missing from the client's tool list.** mcpgate removes disallowed tools from `tools/list`, so the client never sees them. That's working as designed. Compare the client's tool list against `discover_tools.py` run on the upstream.
- **The allowlist "has" the tool, but it's still denied.** Check for:
  - a typo or case difference;
  - a misspelled field (`allowed:`, `alow:`), which is silently ignored and means deny-all;
  - the entry sitting under the wrong key;
  - a missing restart.
- **429s right when the client connects.** The default burst of 5 is smaller than a typical client's connect sequence. Raise `burst`.
- **The upstream needs its own auth header.** mcpgate strips the client's `Authorization` header (its mcpgate key) before forwarding, so the upstream never sees mcpgate keys and gets no `Authorization` by default. Set the upstream's credential in `upstream.headers`, e.g. `Authorization: "Bearer ${UPSTREAM_TOKEN}"`, with `UPSTREAM_TOKEN` exported in mcpgate's environment. Every client then reaches the upstream with that one credential. Other client headers still pass through.
- **401 from the upstream (`decision: allow`, `status: 401`).** The upstream wants a credential mcpgate isn't sending. Add it under `upstream.headers` and restart. Clients that used to rely on their `Authorization` reaching the upstream stopped working when mcpgate began stripping it; this is the fix.
- **Startup fails with `upstream.headers.<Name>: environment variable ${VAR} is not set`.** Export the variable in the environment `serve` runs in (systemd `Environment=`, `docker run -e`, etc.).
- **Resources or prompts are reachable by a restricted key.** Only `tools/call` and `tools/list` are policed. `resources/*`, `prompts/*` and other methods pass through for any authenticated key.
- **Long-running tools fail with 502 after about 30s.** That's `upstream.timeout`. Raise it.
- **A config change has no effect.** There's no hot reload. Restart `serve`.

## Known limitations (v0.1)

- **Transport:** Streamable HTTP and SSE only. Stdio upstreams need a bridge.
- **Auth:** static Bearer API keys only. No OAuth/OIDC, no mTLS.
- **Upstreams:** one per instance.
- **TLS:** none built in. Terminate TLS in front of mcpgate whenever it leaves localhost.
- **Request bodies:** fixed 16 MiB limit.
- **Config:** no hot reload.
- **Audit log:** no built-in rotation.
