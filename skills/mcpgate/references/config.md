# mcpgate config reference

One YAML file (conventionally `config.yaml`), passed with `-c`. Loaded once at startup — **restart `mcpgate serve` after any change**.

## Full example

```yaml
upstream:
  url: "http://localhost:9000/mcp"   # required
  timeout: 30s                       # optional, default 30s
  headers:                           # optional; set on every upstream request
    Authorization: "Bearer ${UPSTREAM_TOKEN}"   # ${VAR} expanded from the environment

listen: "127.0.0.1:8080"             # optional, default ":8080" (all interfaces)

keys:                                # required, at least one
  - label: "claude-code-alice"       # required
    hash: "9f86d081884c7d65..."      # required: SHA-256 hex from `mcpgate keygen`
    allow: ["read_file", "list_directory"]   # optional; omitted/empty = deny all tools
    rate_limit: { rps: 5, burst: 20 }        # optional; falls back to defaults.rate_limit

  - label: "ci-bot"
    hash: "..."
    allow: ["*"]                     # every tool, including ones the upstream adds later

defaults:
  rate_limit: { rps: 2, burst: 5 }   # optional; this is also the built-in default

audit:
  output: stdout                     # optional; "stdout" (default) or a file path (appended, mode 0644)
```

## Fields

| Field | Required | Default | Notes |
|---|---|---|---|
| `upstream.url` | yes | — | Absolute URL with scheme and host, e.g. `http://localhost:9000/mcp`. Every request to the gateway, whatever its path, is forwarded to exactly this path and query. |
| `upstream.timeout` | no | `30s` | Go duration (`10s`, `2m`). It's the time to wait for the upstream's **response headers**. A tool that takes longer than this before responding returns 502, so raise it for slow tools. |
| `upstream.headers` | no | none | Map of header name → value, set on every request to the upstream (overriding any client-sent value). Use it to give the upstream its own credential: the client's `Authorization` (its mcpgate key) is always stripped before forwarding. Values expand `${VAR}` from mcpgate's environment, so secrets stay out of the file. Only the braced form is expanded; a bare `$` is kept literally. |
| `listen` | no | `:8080` | `host:port`. `:8080` binds every interface. Prefer `127.0.0.1:8080` unless remote clients need it, and then put TLS in front. |
| `keys[].label` | yes | — | Shown in the audit log, and the rate-limit bucket key. Keep labels unique: two keys with the same label share one bucket and can't be told apart in the log. |
| `keys[].hash` | yes | — | Lowercase SHA-256 hex of the plaintext key. Generate with `mcpgate keygen`. You can also compute it from an existing key with `printf %s "$KEY" \| shasum -a 256`. |
| `keys[].allow` | no | `[]` (deny all) | Exact tool names, case-sensitive. `"*"` matches everything. No globs, prefixes or regexes, and no deny list. |
| `keys[].rate_limit.rps` | no | inherits | Sustained requests per second (float, e.g. `0.5`). |
| `keys[].rate_limit.burst` | no | inherits | Bucket size. Counts **every** HTTP request (initialize, notifications, list calls, GET streams), not just tool calls. |
| `defaults.rate_limit` | no | `{rps: 2, burst: 5}` | Applies to keys without their own `rate_limit`. If `rps` is 0 or missing, the built-in default is used. |
| `audit.output` | no | `stdout` | `stdout` or a file path. The file is opened in append mode. There's no built-in rotation; use logrotate with `copytruncate`. |

## Validation

`mcpgate validate -c config.yaml` checks:
- `upstream.url` is present and parses.
- At least one key exists.
- Every key has a `label` and a `hash`.
- Every `upstream.headers` entry has a valid header name and a non-empty value without control characters (no CR/LF), and isn't one mcpgate manages (`Host`, `Content-Length`, `Transfer-Encoding`, `Connection`).
- Every `${VAR}` in `upstream.headers` is set in the environment. An unset variable is an error, so run `validate` with the same environment as `serve`. A variable set to an empty string fails the empty-value check.

On success it prints `OK: N key(s), upstream <url>`.

It does **not** catch:
- Misspelled field names. Unknown YAML keys are ignored, so `alow:` silently means "deny all".
- Tool names that don't exist on the upstream. Compare against `scripts/discover_tools.py`.
- Duplicate labels or duplicate hashes.
- A relative upstream URL like `localhost:9000/mcp`. Validate accepts it, but `serve` rejects it with "upstream url must be absolute".

## Keys

`mcpgate keygen --label <label>` prints:

```
API key (give to the client; shown once):
  mcpg_<64 hex chars>

Config entry (paste under keys: in config.yaml):
  - label: "<label>"
    hash: "<sha256 hex>"
    allow: []
```

The entry it prints has `allow: []`, which denies every tool. Fill in `allow` before expecting anything to work.
