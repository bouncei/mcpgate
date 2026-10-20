# Connecting clients through mcpgate

Every client needs two things:
- The gateway URL, e.g. `http://127.0.0.1:8080/mcp`. Any path works, since mcpgate forwards to the upstream's path, but `/mcp` keeps things readable.
- The header `Authorization: Bearer <key>`. The scheme is case-sensitive, so use `Bearer` with one space.

Keep the plaintext key in an environment variable or secret store, and reference it from client config. Don't paste the literal key into files that might be committed or shared.

```bash
export MCPGATE_KEY='mcpg_...'   # e.g. in ~/.zshrc, a .env that's gitignored, or a secret manager
```

## Claude Code

CLI. The default scope is local, meaning just you in this project:
```bash
claude mcp add --transport http files http://127.0.0.1:8080/mcp \
  --header "Authorization: Bearer $MCPGATE_KEY"
```
The shell expands `$MCPGATE_KEY` right away, so the literal key ends up in your local Claude config. That's fine for local or user scope.

For a team-shared `.mcp.json` (`--scope project`, committed to the repo), use env-var expansion so each person supplies their own key:
```json
{
  "mcpServers": {
    "files": {
      "type": "http",
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer ${MCPGATE_KEY}" }
    }
  }
}
```
Claude Code expands `${VAR}` and `${VAR:-default}` in `url` and `headers`.

Verify the connection with `claude mcp list`, or `/mcp` inside a session. Only allowed tools should appear.

## Cursor

`.cursor/mcp.json` (project) or `~/.cursor/mcp.json` (global):
```json
{
  "mcpServers": {
    "files": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer ${env:MCPGATE_KEY}" }
    }
  }
}
```
Cursor uses `${env:NAME}`, not `${NAME}`. Remote servers can't use `envFile`, so the variable has to be set in the environment Cursor was launched from. Restart Cursor after you change it.

## Other clients

Any MCP client that supports Streamable HTTP (or SSE) with custom headers works the same way: put the gateway URL in, and add the `Authorization` header.

Clients that only support stdio need a stdio→HTTP bridge on the client side, one that can add headers.

## curl smoke test

Run this against the gateway after setup. It checks auth, filtering, allow and deny. Replace the tool names with an allowed one and a disallowed one.

```bash
GW=http://127.0.0.1:8080/mcp
H=(-H "Authorization: Bearer $MCPGATE_KEY" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream")

# 1. no key → 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST "$GW" -d '{}'

# 2. initialize, capturing the session id if the upstream issues one
SID=$(curl -s -D - -o /dev/null "${H[@]}" "$GW" -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' \
  | awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}' | tr -d '\r')
S=(); [ -n "$SID" ] && S=(-H "Mcp-Session-Id: $SID")
curl -s "${H[@]}" "${S[@]}" "$GW" -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# 3. tools/list → only allowed tools
curl -s "${H[@]}" "${S[@]}" "$GW" -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# 4. allowed tool → a result
curl -s "${H[@]}" "${S[@]}" "$GW" -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ALLOWED_TOOL","arguments":{}}}'

# 5. disallowed tool → {"error":{"code":-32001,"message":"tool not permitted: ..."}}
curl -s "${H[@]}" "${S[@]}" "$GW" -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"DISALLOWED_TOOL","arguments":{}}}'
```

There's a quicker check for steps 2 and 3:
```bash
python3 scripts/discover_tools.py http://127.0.0.1:8080/mcp -H "Authorization: Bearer $MCPGATE_KEY"
```
It should list only the allowed tools. Run it against the upstream URL as well to compare.
