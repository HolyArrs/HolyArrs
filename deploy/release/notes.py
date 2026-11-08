#!/usr/bin/env python3
"""Build every release text from one file: releases/notes-<version>.yml.

    python deploy/release/notes.py <version> [--out DIR] [--check]

Writes changelog.md, github.md, update-notes.json, reddit.txt, discord-dm.txt,
discord-announce.md and roadmap-card.json. --check only validates and exits 1
on any error. Standard library only: the yml is read with a small parser that
covers the shapes the notes file uses (block maps and lists, flow {} and [],
quoted strings, ints, comments). No network calls.
"""
import argparse
import datetime
import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
ISSUES = "https://github.com/HolyArrs/holyarrs/issues/"
RELEASES = "https://github.com/HolyArrs/holyarrs/releases/tag/"
KNOWN = {"version", "theme", "highlights", "before_you_update", "groups",
         "security", "known_issues", "credits", "stats"}
KINDS = ("added", "fixed", "changed")
DM_LIMIT = 1000
BANNED = ("seamless", "robust", "enhanced", "leverage", "excited", "refactor", "module")
FORBIDDEN = re.compile(r"\b(coming soon|launch\w*|next week|\d{4}-\d{2}-\d{2})\b", re.I)
REF = re.compile(r"\s*\(#(\d+)\)")
LOGIN = re.compile(r"^[A-Za-z0-9-]{1,39}$")


# ---------- tiny yaml subset ----------

def _strip_comment(line):
    out, quote = [], None
    for i, ch in enumerate(line):
        if quote:
            if ch == quote:
                quote = None
        elif ch in "\"'":
            quote = ch
        elif ch == "#" and (i == 0 or line[i - 1] in " \t"):
            break
        out.append(ch)
    return "".join(out).rstrip()


def _split_flow(text):
    parts, depth, quote, cur = [], 0, None, ""
    for ch in text:
        if quote:
            cur += ch
            if ch == quote:
                quote = None
            continue
        if ch in "\"'":
            quote = ch
        elif ch in "[{":
            depth += 1
        elif ch in "]}":
            depth -= 1
        elif ch == "," and depth == 0:
            parts.append(cur.strip())
            cur = ""
            continue
        cur += ch
    if cur.strip():
        parts.append(cur.strip())
    return parts


def _scalar(text):
    t = text.strip()
    if t == "" or t in ("null", "~"):
        return None
    if t.startswith("[") and t.endswith("]"):
        return [_scalar(p) for p in _split_flow(t[1:-1])]
    if t.startswith("{") and t.endswith("}"):
        out = {}
        for p in _split_flow(t[1:-1]):
            k, _, v = p.partition(":")
            out[k.strip()] = _scalar(v)
        return out
    if len(t) >= 2 and t[0] == t[-1] and t[0] in "\"'":
        return t[1:-1]
    if re.fullmatch(r"-?\d+", t):
        return int(t)
    if t in ("true", "false"):
        return t == "true"
    return t


def _key(text):
    m = re.match(r"^([^\s:\"'][^:]*?):(\s+(.*))?$", text)
    return (m.group(1).strip(), m.group(3) or "") if m else None


def load_yaml(text):
    lines = []
    for raw in text.splitlines():
        s = _strip_comment(raw)
        if s.strip():
            lines.append((len(s) - len(s.lstrip(" ")), s.strip()))
    value, _ = _block(lines, 0, lines[0][0] if lines else 0)
    return value or {}


def _block(lines, i, indent):
    if i >= len(lines):
        return None, i
    if lines[i][1].startswith("- ") or lines[i][1] == "-":
        out = []
        while i < len(lines) and lines[i][0] == indent and lines[i][1].startswith("-"):
            rest = lines[i][1][1:].strip()
            i += 1
            kv = _key(rest) if not rest.startswith(("{", "[", "\"", "'")) else None
            if kv:  # list of maps: "- key: val" then more keys indented
                item = {}
                sub = indent + 2
                lines.insert(i, (sub, rest))
                while i < len(lines) and lines[i][0] == sub and not lines[i][1].startswith("- "):
                    k, v = _key(lines[i][1])
                    i += 1
                    if v == "" and i < len(lines) and lines[i][0] > sub:
                        item[k], i = _block(lines, i, lines[i][0])
                    else:
                        item[k] = _scalar(v)
                out.append(item)
            elif rest == "" and i < len(lines) and lines[i][0] > indent:
                val, i = _block(lines, i, lines[i][0])
                out.append(val)
            else:
                out.append(_scalar(rest))
        return out, i
    out = {}
    while i < len(lines) and lines[i][0] == indent:
        kv = _key(lines[i][1])
        if not kv:
            raise ValueError("cannot read yml line: %r" % lines[i][1])
        k, v = kv
        i += 1
        if v == "" and i < len(lines) and (lines[i][0] > indent or
                                           (lines[i][0] == indent and lines[i][1].startswith("- "))):
            out[k], i = _block(lines, i, lines[i][0])
        else:
            out[k] = _scalar(v)
    return out, i


# ---------- helpers ----------

def empty(v):
    return v is None or v == "" or v == [] or v == {}


def items(v):
    return [x for x in (v or []) if not empty(x)]


def md_links(line):
    return re.sub(r"\(#(\d+)\)", lambda m: "([#%s](%s%s))" % (m.group(1), ISSUES, m.group(1)), line)


def no_refs(line):
    return REF.sub("", line).strip()


def before_shows(n):
    b = n.get("before_you_update") or {}
    return bool(items(b.get("breaking")) or items(b.get("manual_steps")))


def first_breaking(n):
    b = items((n.get("before_you_update") or {}).get("breaking"))
    return b[0] if b else None


def stats_line(n):
    s = n.get("stats") or {}
    parts = [(s.get("issues_done"), "issues done"), (s.get("still_building"), "still building"),
             (s.get("people_credited"), "people credited")]
    parts = ["%d %s" % (v, label) for v, label in parts if isinstance(v, int) and v > 0]
    return (", ".join(parts) + ".") if parts else None


def release_link(n):
    return RELEASES + str(n["version"])


def groups(n):
    return [(place, g or {}) for place, g in (n.get("groups") or {}).items()]


def credit_line(c, cmap):
    name = str(c["name"])
    if not name.startswith("u/"):
        name = "u/" + name
    login = c.get("github") or cmap.get(c["name"]) or cmap.get(name)
    text = "[%s](%s)" % (name, c["link"]) if c.get("link") else name
    return text + (" (@%s)" % login if login else "")


def sorted_credits(n):
    cs = items(n.get("credits"))
    return sorted(cs, key=lambda c: (-len(c.get("issues") or []), str(c["name"]).lower()))


# ---------- renderers ----------

def render_changelog(n, today):
    out = ["## [%s] - %s" % (n["version"], today)]
    for kind in KINDS:
        lines = [l for _, g in groups(n) for l in items(g.get(kind))]
        if lines:
            out.append("### " + kind.capitalize())
            out += ["- " + md_links(l) for l in lines]
    return "\n".join(out) + "\n"


def render_github(n, cmap):
    blocks = []
    head = [n["theme"]] if not empty(n.get("theme")) else []
    if stats_line(n):
        head.append("_%s_" % stats_line(n))
    if head:
        blocks.append("\n\n".join(head))
    hl = items(n.get("highlights"))
    if hl:
        blocks.append("## Highlights\n" + "\n".join("- " + md_links(h["line"]) for h in hl))
    if before_shows(n):
        b = n["before_you_update"]
        rows = ["- " + md_links(x) for x in items(b.get("breaking")) + items(b.get("manual_steps"))]
        if not empty(b.get("backup")):
            rows.append("- Backup: " + b["backup"])
        if not empty(b.get("rollback")):
            rows.append("- Rollback: " + b["rollback"])
        blocks.append("## Before you update\n" + "\n".join(rows))
    for place, g in groups(n):
        parts = []
        for kind in KINDS:
            lines = items(g.get(kind))
            if lines:
                parts.append("### %s\n%s" % (kind.capitalize(), "\n".join("- " + md_links(l) for l in lines)))
        if parts:
            blocks.append("## %s\n%s" % (place, "\n\n".join(parts)))
    for key, title in (("security", "Security"), ("known_issues", "Known issues")):
        lines = items(n.get(key))
        if lines:
            blocks.append("## %s\n%s" % (title, "\n".join("- " + md_links(l) for l in lines)))
    cs = sorted_credits(n)
    if cs:
        blocks.append("## Thanks\n" + ", ".join(credit_line(c, cmap) for c in cs))
    return "\n\n".join(blocks) + "\n"


def render_update_notes(n):
    return json.dumps({
        "version": str(n["version"]),
        "title": "HolyArrs %s is ready" % n["version"],
        "theme": n.get("theme"),
        "lines": [no_refs(h["line"]) for h in items(n.get("highlights"))],
        "before": first_breaking(n),
        "link": release_link(n),
    }, indent=2, ensure_ascii=False) + "\n"


def _low(s):
    return re.sub(r"\s*\u2014\s*", ", ", no_refs(s)).lower()


def render_reddit(n):
    out = ["what's in %s" % n["version"], ""]
    if not empty(n.get("theme")):
        out += [_low(n["theme"]), ""]
    hl = [_low(h["line"]).rstrip(".") for h in items(n.get("highlights"))]
    if hl:
        out += ["- " + h for h in hl] + [""]
    if first_breaking(n):
        out += ["heads up: " + _low(first_breaking(n)), ""]
    people = (n.get("stats") or {}).get("people_credited")
    if isinstance(people, int) and people > 0:
        out += ["thanks to the %d of you who asked for stuff, you're all named in the notes" % people, ""]
    out.append(release_link(n))
    return "\n".join(out) + "\n"


def render_discord(n, bold=False):
    def build(theme, before):
        title = "HolyArrs %s is ready" % n["version"]
        out = ["**%s**" % title if bold else title]
        if theme and not empty(n.get("theme")):
            out.append(n["theme"])
        out += ["- " + no_refs(h["line"]) for h in items(n.get("highlights"))]
        if before and first_breaking(n):
            out.append("Before you update: " + first_breaking(n))
        out.append("What changed: " + release_link(n))
        return "\n".join(out) + "\n"
    for theme, before in ((True, True), (False, True), (False, False)):
        text = build(theme, before)
        if len(text) <= DM_LIMIT:
            return text
    return text


def render_roadmap(n):
    return json.dumps({"version": str(n["version"]), "theme": n.get("theme"),
                       "stats": stats_line(n), "link": release_link(n)},
                      indent=2, ensure_ascii=False) + "\n"


def render_all(n, cmap, today):
    return {
        "changelog.md": render_changelog(n, today),
        "github.md": render_github(n, cmap),
        "update-notes.json": render_update_notes(n),
        "reddit.txt": render_reddit(n),
        "discord-dm.txt": render_discord(n),
        "discord-announce.md": render_discord(n, bold=True),
        "roadmap-card.json": render_roadmap(n),
    }


# ---------- check ----------

def check(n, cmap):
    errors, warnings = [], []
    for k in n:
        if k not in KNOWN:
            warnings.append("unknown key ignored: %s" % k)
    if not re.fullmatch(r"\d+\.\d+\.\d+", str(n.get("version", ""))):
        errors.append("version must be plain X.Y.Z (no v): %r" % n.get("version"))
    if empty(n.get("theme")):
        errors.append("theme is missing")
    hl = items(n.get("highlights"))
    if len(hl) != 3:
        errors.append("need exactly 3 highlights, found %d" % len(hl))
    group_lines = [l for _, g in groups(n) for kind in KINDS for l in items(g.get(kind))]
    if not group_lines:
        errors.append("groups has no lines")
    group_refs = {int(x) for l in group_lines for x in re.findall(r"\(#(\d+)\)", l)}
    for h in hl:
        if not isinstance(h.get("issue"), int):
            errors.append("highlight has no issue number: %r" % h.get("line"))
        elif h["issue"] not in group_refs:
            errors.append("highlight issue #%d is in no group line" % h["issue"])
        if empty(h.get("shot")):
            errors.append("highlight has no shot: %r" % h.get("line"))
    for l in group_lines + items(n.get("security")) + items(n.get("known_issues")):
        if not re.search(r"\(#\d+\)\s*$", l):
            errors.append("line has no (#n) at the end: %r" % l)
    b = n.get("before_you_update") or {}
    every = [n.get("theme") or ""] + [h.get("line") or "" for h in hl] + group_lines + \
        items(n.get("security")) + items(n.get("known_issues")) + items(b.get("breaking")) + \
        items(b.get("manual_steps"))
    for l in every:
        words = len(no_refs(l).split())
        if words > 12:
            warnings.append("over 12 words (%d): %r" % (words, l))
        low = l.lower()
        for w in BANNED:
            if re.search(r"\b%s" % w, low):
                warnings.append("banned word %r: %r" % (w, l))
        if "\u2014" in l:
            warnings.append("em dash: %r" % l)
        m = FORBIDDEN.search(l)
        if m:
            errors.append("no dates or launch talk (%r): %r" % (m.group(0), l))
    if before_shows(n) and (empty(b.get("backup")) or empty(b.get("rollback"))):
        errors.append("before_you_update shows, so backup and rollback must be set")
    cs = items(n.get("credits"))
    people = (n.get("stats") or {}).get("people_credited")
    if isinstance(people, int) and cs and people != len({c.get("name") for c in cs}):
        warnings.append("stats.people_credited is %d but credits list %d names"
                        % (people, len({c.get("name") for c in cs})))
    for c in cs:
        if empty(c.get("name")):
            errors.append("credit without a name")
        login = c.get("github") or cmap.get(c.get("name"))
        if login and not LOGIN.fullmatch(str(login)):
            errors.append("github login looks wrong for %s: %r" % (c.get("name"), login))
    if len(render_discord(n)) > DM_LIMIT:
        errors.append("discord DM over %d chars even after cutting" % DM_LIMIT)
    return errors, warnings


# ---------- cli ----------

def load_map(path):
    if not path or not path.exists():
        return {}
    data = json.loads(path.read_text(encoding="utf-8-sig"))
    return data.get("map", data) if isinstance(data, dict) else {}


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("version")
    ap.add_argument("--out", type=pathlib.Path)
    ap.add_argument("--check", action="store_true")
    ap.add_argument("--file", type=pathlib.Path, help="notes yml (default releases/notes-<version>.yml)")
    ap.add_argument("--map", type=pathlib.Path, help="credits map (default releases/credits-map.json)")
    ap.add_argument("--date", help="changelog date (default today)")
    a = ap.parse_args(argv)
    src = a.file or ROOT / "releases" / ("notes-%s.yml" % a.version)
    notes = load_yaml(src.read_text(encoding="utf-8-sig"))
    cmap = load_map(a.map or ROOT / "releases" / "credits-map.json")
    errors, warnings = check(notes, cmap)
    for w in warnings:
        print("warning: " + w)
    for e in errors:
        print("error: " + e)
    if a.check:
        return 1 if errors else 0
    if errors:
        return 1
    out = a.out or ROOT / "releases" / "out" / a.version
    out.mkdir(parents=True, exist_ok=True)
    today = a.date or datetime.date.today().isoformat()
    for name, text in render_all(notes, cmap, today).items():
        with open(out / name, "w", encoding="utf-8", newline="\n") as f:
            f.write(text)
    print("wrote %d files to %s" % (7, out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
