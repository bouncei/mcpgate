#!/usr/bin/env python3
"""List an MCP server's exact tool names so mcpgate allowlists can be written
without guessing.

Runs the Streamable HTTP handshake (initialize -> notifications/initialized ->
tools/list, following pagination) and handles both application/json and
text/event-stream responses. Standard library only.

  discover_tools.py http://localhost:9000/mcp
  discover_tools.py http://localhost:8080/mcp -H "Authorization: Bearer $MCPGATE_KEY"
  discover_tools.py http://localhost:9000/mcp --json
"""
import argparse
import json
import re
import sys
import urllib.error
import urllib.request

PROTOCOL_VERSION = "2025-06-18"


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Refuse redirects: urllib would otherwise replay our headers, including
    any -H Authorization credential, to whatever host the server names."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


_OPENER = urllib.request.build_opener(_NoRedirect)
_CONTROL = re.compile(r"[\x00-\x1f\x7f-\x9f]")


def _clean(s):
    """Strip control characters from server-supplied text before printing it,
    so a hostile server can't inject terminal escape sequences."""
    return _CONTROL.sub("", str(s))


class Client:
    def __init__(self, url, headers, timeout):
        self.url = url
        self.headers = headers
        self.timeout = timeout
        self.session = None
        self.next_id = 0

    def _post(self, payload):
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "MCP-Protocol-Version": PROTOCOL_VERSION,
            **self.headers,
        }
        if self.session:
            headers["Mcp-Session-Id"] = self.session
        req = urllib.request.Request(
            self.url, data=json.dumps(payload).encode(), headers=headers, method="POST"
        )
        try:
            resp = _OPENER.open(req, timeout=self.timeout)
        except urllib.error.HTTPError as e:
            if 300 <= e.code < 400:
                raise SystemExit(f"HTTP {e.code} redirect from {self.url} to "
                                 f"{_clean(e.headers.get('Location', '?'))}; redirects are not "
                                 "followed (they could leak -H credentials), pass the final URL")
            body = _clean(e.read().decode(errors="replace").strip())
            raise SystemExit(f"HTTP {e.code} from {self.url}: {body[:300]}")
        except urllib.error.URLError as e:
            raise SystemExit(f"cannot reach {self.url}: {e.reason}")
        with resp:
            sid = resp.headers.get("Mcp-Session-Id")
            if sid:
                self.session = sid
            ctype = resp.headers.get("Content-Type", "")
            raw = resp.read().decode(errors="replace")
        return ctype, raw

    def notify(self, method, params=None):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        self._post(msg)

    def request(self, method, params=None):
        self.next_id += 1
        rid = self.next_id
        msg = {"jsonrpc": "2.0", "id": rid, "method": method, "params": params or {}}
        ctype, raw = self._post(msg)
        for m in _messages(ctype, raw):
            if m.get("id") == rid:
                if "error" in m:
                    err = m["error"]
                    raise SystemExit(f"{method} failed: {err.get('code')} {_clean(err.get('message'))}")
                return m.get("result", {})
        raise SystemExit(f"no response to {method} (content-type {ctype!r}): {_clean(raw[:300])}")


def _messages(ctype, raw):
    """Yield JSON-RPC messages from a JSON body or an SSE stream."""
    if ctype.startswith("text/event-stream"):
        data = []
        for line in raw.splitlines() + [""]:
            if line.startswith("data:"):
                data.append(line[5:].lstrip())
            elif line == "" and data:
                yield from _flatten(json.loads("\n".join(data)))
                data = []
    elif raw.strip():
        yield from _flatten(json.loads(raw))


def _flatten(obj):
    return obj if isinstance(obj, list) else [obj]


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("url", help="MCP endpoint, e.g. http://localhost:9000/mcp")
    ap.add_argument("-H", "--header", action="append", default=[], help='extra header, "Name: value" (repeatable)')
    ap.add_argument("--timeout", type=float, default=15)
    ap.add_argument("--json", action="store_true", help="print the raw tool list as JSON")
    args = ap.parse_args()

    headers = {}
    for h in args.header:
        name, sep, value = h.partition(":")
        if not sep:
            ap.error(f"bad header {h!r}; expected 'Name: value'")
        headers[name.strip()] = value.strip()

    c = Client(args.url, headers, args.timeout)
    init = c.request("initialize", {
        "protocolVersion": PROTOCOL_VERSION,
        "capabilities": {},
        "clientInfo": {"name": "mcpgate-discover-tools", "version": "1"},
    })
    c.notify("notifications/initialized")

    tools, cursor = [], None
    while True:
        res = c.request("tools/list", {"cursor": cursor} if cursor else {})
        tools.extend(res.get("tools", []))
        cursor = res.get("nextCursor")
        if not cursor:
            break

    if args.json:
        json.dump(tools, sys.stdout, indent=2)
        print()
        return

    server = init.get("serverInfo", {})
    print(f"Server: {_clean(server.get('name', '?'))} {_clean(server.get('version', ''))}".rstrip())
    print(f"Tools ({len(tools)}):")
    read_only, other = [], []
    for t in tools:
        ann = t.get("annotations") or {}
        flags = []
        if ann.get("readOnlyHint"):
            flags.append("read-only")
        if ann.get("destructiveHint"):
            flags.append("destructive")
        desc = " ".join(_clean(t.get("description") or "").split())
        if len(desc) > 90:
            desc = desc[:87] + "..."
        tag = f" [{', '.join(flags)}]" if flags else ""
        print(f"  {_clean(t['name'])}{tag}  {desc}")
        (read_only if ann.get("readOnlyHint") else other).append(t["name"])

    print("\nSuggested allow lists (exact names; adjust to the client's needs):")
    print(f"  all tools:  allow: {json.dumps([t['name'] for t in tools])}")
    if read_only:
        print(f"  read-only:  allow: {json.dumps(read_only)}")
    else:
        print("  read-only:  (server sets no readOnlyHint annotations; decide from names/descriptions)")


if __name__ == "__main__":
    main()
