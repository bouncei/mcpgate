#!/usr/bin/env python3
"""Summarize an mcpgate audit log (JSON lines: label, method, tool, decision,
status, latency_ms). Standard library only.

  audit_summary.py mcpgate-audit.jsonl
  audit_summary.py mcpgate-audit.jsonl --label ci-bot --since 7d
  audit_summary.py mcpgate-audit.jsonl --decision deny --limit 50
  audit_summary.py mcpgate-audit.jsonl --config config.yaml   # + allowed-but-unused tools
  docker logs mcpgate 2>&1 | audit_summary.py -
"""
import argparse
import json
import re
import sys
from collections import Counter, defaultdict
from datetime import datetime, timedelta, timezone


def parse_since(s):
    m = re.fullmatch(r"(\d+)([mhdw])", s)
    if m:
        n, unit = int(m.group(1)), m.group(2)
        delta = {"m": timedelta(minutes=n), "h": timedelta(hours=n),
                 "d": timedelta(days=n), "w": timedelta(weeks=n)}[unit]
        return datetime.now(timezone.utc) - delta
    return parse_time(s)


def parse_time(s):
    # slog writes RFC3339 with nanoseconds; trim to microseconds for fromisoformat.
    s = s.replace("Z", "+00:00")
    s = re.sub(r"(\.\d{6})\d+", r"\1", s)
    t = datetime.fromisoformat(s)
    return t if t.tzinfo else t.replace(tzinfo=timezone.utc)


def read_events(f):
    for line in f:
        line = line.strip()
        if not line.startswith("{"):
            continue  # skip non-JSON lines (e.g. "mcpgate listening on ...")
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if "decision" in ev:
            yield ev


def config_allowlists(path):
    """Minimal parse of keys[].label / keys[].allow from mcpgate's config.yaml,
    accepting flow (`allow: [a, b]`) and block (`allow:` + `- a` lines) lists.
    Avoids a PyYAML dependency."""
    allow, label, block = {}, None, None
    with open(path) as f:
        for line in f:
            line = line.split("#", 1)[0].rstrip()
            if block is not None:
                m = re.match(r"\s*-\s*[\"']?([^\"']+?)[\"']?\s*$", line)
                if m and ":" not in m.group(1):
                    allow[label].append(m.group(1))
                    continue
                block = None
            m = re.match(r"\s*-?\s*label:\s*[\"']?([^\"']+?)[\"']?\s*$", line)
            if m:
                label = m.group(1)
                continue
            m = re.match(r"\s*allow:\s*\[(.*)\]\s*$", line)
            if m and label:
                allow[label] = [x.strip().strip("\"'") for x in m.group(1).split(",") if x.strip()]
                continue
            if re.match(r"\s*allow:\s*$", line) and label:
                allow[label], block = [], label
    return allow


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("log", help="audit log path, or - for stdin")
    ap.add_argument("--label", help="only this key label")
    ap.add_argument("--since", help="relative (30m, 24h, 7d, 2w) or ISO timestamp")
    ap.add_argument("--decision", help="prefix filter, e.g. deny, deny:policy, allow")
    ap.add_argument("--config", help="config.yaml, to report allowed-but-unused tools per label")
    ap.add_argument("--limit", type=int, default=20, help="recent denied events to list (default 20)")
    args = ap.parse_args()

    since = parse_since(args.since) if args.since else None
    f = sys.stdin if args.log == "-" else open(args.log)
    events = []
    with f:
        for ev in read_events(f):
            if args.label and ev.get("label") != args.label:
                continue
            if args.decision and not ev.get("decision", "").startswith(args.decision):
                continue
            if since and "time" in ev and parse_time(ev["time"]) < since:
                continue
            events.append(ev)

    if not events:
        print("No matching audit events.")
        return

    print(f"{len(events)} events  {events[0].get('time', '?')}  →  {events[-1].get('time', '?')}\n")

    by_label = defaultdict(Counter)
    for ev in events:
        by_label[ev.get("label") or "(unauthenticated)"][ev["decision"]] += 1
    print("Decisions by label:")
    for label in sorted(by_label):
        counts = ", ".join(f"{d}={n}" for d, n in by_label[label].most_common())
        print(f"  {label:<28} {counts}")

    calls = defaultdict(Counter)
    for ev in events:
        if ev.get("method") == "tools/call" and ev.get("tool"):
            calls[ev.get("label") or "(unauthenticated)"][(ev["tool"], ev["decision"])] += 1
    if calls:
        print("\nTool calls by label (tool  decision  count):")
        for label in sorted(calls):
            print(f"  {label}")
            for (tool, dec), n in sorted(calls[label].items(), key=lambda kv: -kv[1]):
                print(f"    {tool:<32} {dec:<14} {n}")

    errors = [ev for ev in events if ev["decision"] == "allow" and int(ev.get("status") or 0) >= 400]
    if errors:
        print(f"\nAllowed but upstream returned an error status: {len(errors)}")
        for st, n in Counter(ev.get("status") for ev in errors).most_common():
            print(f"  status {st}: {n}")

    denies = [ev for ev in events if ev["decision"].startswith("deny")]
    if denies:
        print(f"\nMost recent denials (up to {args.limit}):")
        for ev in denies[-args.limit:]:
            print(f"  {ev.get('time', '?')}  {ev.get('label') or '-':<20} {ev['decision']:<15} "
                  f"{ev.get('method') or '-'} {ev.get('tool') or ''}".rstrip())

    if args.config:
        allow = config_allowlists(args.config)
        print("\nAllowed but never called in this window (candidates to remove):")
        any_unused = False
        for label, tools in sorted(allow.items()):
            if args.label and label != args.label:
                continue
            if "*" in tools:
                used = sorted({t for (t, d) in calls.get(label, {}) if d == "allow"})
                print(f"  {label}: allow is \"*\"; tools actually used: {used or 'none'}")
                any_unused = True
                continue
            used = {t for (t, d) in calls.get(label, {}) if d == "allow"}
            unused = [t for t in tools if t not in used]
            if unused:
                any_unused = True
                print(f"  {label}: {unused}")
        if not any_unused:
            print("  none")


if __name__ == "__main__":
    main()
