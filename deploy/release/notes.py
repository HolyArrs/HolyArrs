#!/usr/bin/env python3
"""Build every release text from one file: releases/notes-<version>.toml.

    python deploy/release/notes.py <version> [--out DIR] [--check]

Writes changelog.md, github.md, update-notes.json, reddit.txt, discord-dm.txt,
discord-announce.md and roadmap-card.json. --check only validates and exits 1
on any error (warnings are printed, errors fail). Standard library only: the
notes file is TOML, read with tomllib. No network calls.
"""
import argparse
import datetime
import difflib
import json
import pathlib
import re
import sys
import tomllib

ROOT = pathlib.Path(__file__).resolve().parents[2]
ISSUES = "https://github.com/HolyArrs/HolyArrs/issues/"
RELEASES = "https://github.com/HolyArrs/HolyArrs/releases/tag/"
KNOWN = {"version", "theme", "image", "images", "docs_url", "highlights", "groups",
         "security", "known_issues", "credits", "stats"}
# Old map format kinds -> the prefix the list format uses ("" = new).
KINDS = {"added": "", "fixed": "Fixed: ", "changed": "Changed: ", "docs": "Docs: "}
PREFIXES = ("Fixed: ", "Changed: ", "Docs: ")
# Places in the order users touch them most; any other place follows in file order.
PLACE_ORDER = ("Requests", "Library", "Downloads", "Indexers", "Accounts", "Health", "Setup", "Install")
DM_LIMIT = 1000
EMBED_LIMIT = 4096
BANNED = re.compile(r"\b(seamless\w*|robust\w*|enhanced|leverag\w*|excited|refactor\w*|modules?|"
                    r"clean room|sessions?|agents?|worktrees?)\b|\bv\d+\.\d+(\.\d+)?\b", re.I)
MONTHS = "january|february|march|april|june|july|august|september|october|november|december"
WEEKDAYS = "monday|tuesday|wednesday|thursday|friday|saturday|sunday"
FORBIDDEN = re.compile(r"\b(coming soon|launch|launches|launching|today|tonight|tomorrow|"
                       r"next (?:week|month|year)|%s|%s|may \d{1,2}|\d{4}-\d{2}-\d{2})\b"
                       % (MONTHS, WEEKDAYS), re.I)
REF = re.compile(r"\s*\(#(\d+)\)")
LOGIN = re.compile(r"^[A-Za-z0-9-]{1,39}$")
FILES = ("changelog.md", "github.md", "update-notes.json", "reddit.txt",
         "discord-dm.txt", "discord-announce.md", "roadmap-card.json")


# ---------- helpers ----------

def empty(v):
    return v is None or v == "" or v == [] or v == {}


def items(v):
    return [x for x in (v or []) if not empty(x)]


def md_links(line):
    return re.sub(r"\(#(\d+)\)", lambda m: "([#%s](%s%s))" % (m.group(1), ISSUES, m.group(1)), line)


def no_refs(line):
    return REF.sub("", line).strip()


def refs(line):
    return {int(x) for x in re.findall(r"\(#(\d+)\)", line)}


def hl_issues(h):
    v = h.get("issues")
    if v is None and h.get("issue") is not None:
        v = [h["issue"]]
    return [x for x in (v or []) if isinstance(x, int)]


def hl_line(h):
    return h["line"] + "".join(" (#%d)" % i for i in hl_issues(h))


def stats_line(n):
    s = n.get("stats") or {}
    parts = [(s.get("issues_done"), "issues done"), (s.get("still_building"), "still building"),
             (s.get("people_credited"), "people credited")]
    parts = ["%d %s" % (v, label) for v, label in parts if isinstance(v, int) and v > 0]
    return (", ".join(parts) + ".") if parts else None


def release_link(n):
    return RELEASES + str(n["version"])


def group_lines(g):
    """A place is a list of lines; the old {added, fixed, changed, docs} map still works."""
    if isinstance(g, dict):
        return [KINDS[k] + l for k in KINDS for l in items(g.get(k))]
    return items(g)


def groups(n):
    gs = [(place, group_lines(g)) for place, g in (n.get("groups") or {}).items()]
    rank = {p: i for i, p in enumerate(PLACE_ORDER)}
    return sorted(gs, key=lambda pg: rank.get(pg[0], len(rank)))


def all_group_lines(n):
    return [l for _, ls in groups(n) for l in ls]


def bare(name):
    name = str(name)
    return name[2:] if name.startswith("u/") else name


def login_for(c, cmap):
    """github from the notes file, else the map under the bare or u/ name."""
    if c.get("github"):
        return c["github"]
    b = bare(c.get("name", ""))
    return cmap.get(b) or cmap.get("u/" + b)


def credit_key(c):
    """One name per person: the Reddit name, else the GitHub login."""
    return bare(c["name"]) if not empty(c.get("name")) else "@" + str(c.get("github", ""))


def credit_line(c, cmap):
    login = login_for(c, cmap)
    if empty(c.get("name")):
        name = "@" + str(c["github"])
        return "[%s](%s)" % (name, c["link"]) if c.get("link") else name
    name = "u/" + bare(c["name"])
    text = "[%s](%s)" % (name, c["link"]) if c.get("link") else name
    return text + (" (@%s)" % login if login else "")


def sorted_credits(n):
    cs = items(n.get("credits"))
    return sorted(cs, key=lambda c: (-len(c.get("issues") or []), credit_key(c).lstrip("@").lower()))


def bullets(lines):
    return "\n".join("- " + md_links(l) for l in lines)


# ---------- renderers ----------

def render_changelog(n, today):
    out = ["## [%s] - %s" % (n["version"], today)]
    buckets = {"Added": [hl_line(h) for h in items(n.get("highlights"))],
               "Fixed": [], "Changed": [], "Docs": []}
    for l in all_group_lines(n):
        for p in PREFIXES:
            if l.startswith(p):
                buckets[p[:-2]].append(l[len(p):])
                break
        else:
            buckets["Added"].append(l)
    for kind, lines in buckets.items():
        if lines:
            out += ["### " + kind, bullets(lines)]
    return "\n".join(out) + "\n"


def images(n):
    """Registries to install from, main first; a lone old `image` still works."""
    out = []
    for i in n.get("images") or []:
        if isinstance(i, dict) and not empty(i.get("name")):
            out.append({"registry": i.get("registry") or i["name"].split("/")[0], "name": i["name"]})
    if not out and not empty(n.get("image")):
        out.append({"registry": "Docker Hub", "name": n["image"]})
    return out


def render_github(n, cmap):
    blocks = []
    if not empty(n.get("theme")):
        blocks.append(n["theme"])
    hl = items(n.get("highlights"))
    if hl:
        blocks.append("## Highlights\n" + bullets(hl_line(h) for h in hl))
    also, install = [], []
    for place, lines in groups(n):
        if place == "Install":
            install = lines  # shown with the image lines further down
        elif len(lines) >= 3:
            blocks.append("## %s\n%s" % (place, bullets(lines)))
        else:
            also += lines
    if also:
        blocks.append("## Also new\n" + bullets(also))
    for key, title in (("security", "Security"), ("known_issues", "Known issues")):
        lines = items(n.get(key))
        if lines:
            blocks.append("## %s\n%s" % (title, bullets(lines)))
    # Practical and credit parts sit at the bottom; assets stay in GitHub's own box.
    imgs = images(n)
    rows = ["%s:\n\n`%s:%s`\n\n`%s:latest`" % (i["registry"], i["name"], n["version"], i["name"])
            for i in imgs]
    if install or rows:
        parts = ([bullets(install)] if install else []) + rows
        blocks.append("## Install\n" + "\n\n".join(parts))
    if not empty(n.get("docs_url")):
        blocks.append("## Docs\n[Read the docs](%s)" % n["docs_url"])
    cs = sorted_credits(n)
    if cs:
        blocks.append("## Thanks\n" + ", ".join(credit_line(c, cmap) for c in cs))
    if stats_line(n):
        blocks.append("_%s_" % stats_line(n))
    return "\n\n".join(blocks) + "\n"


def render_update_notes(n):
    return json.dumps({
        "version": str(n["version"]),
        "title": "HolyArrs %s is ready" % n["version"],
        "theme": n.get("theme"),
        "lines": [no_refs(h["line"]) for h in items(n.get("highlights"))],
        "link": release_link(n),
    }, indent=2, ensure_ascii=False) + "\n"


def _low(s):
    return re.sub(r"\s*—\s*", ", ", no_refs(s)).lower()


def render_reddit(n):
    out = ["what's in %s" % n["version"], ""]
    if not empty(n.get("theme")):
        out += [_low(n["theme"]), ""]
    hl = [_low(h["line"]).rstrip(".") for h in items(n.get("highlights"))]
    if hl:
        out += ["- " + h for h in hl] + [""]
    people = (n.get("stats") or {}).get("people_credited")
    if isinstance(people, int) and people > 0:
        out += ["thanks to the %d of you who asked for stuff, you're all named in the notes" % people, ""]
    out.append(release_link(n))
    return "\n".join(out) + "\n"


def render_discord(n, bold=False):
    def build(theme):
        title = "HolyArrs %s is ready" % n["version"]
        out = ["**%s**" % title if bold else title]
        if theme and not empty(n.get("theme")):
            out.append(n["theme"])
        out += ["- " + no_refs(h["line"]) for h in items(n.get("highlights"))]
        # The first release has nothing to compare with, so its posts say what it has.
        label = "What's in it: " if n["version"] == "1.0.0" else "What changed: "
        out.append(label + release_link(n))
        return "\n".join(out) + "\n"
    for theme in (True, False):
        text = build(theme)
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
    if "before_you_update" in n:
        errors.append("before_you_update is no longer used, remove that section from the notes file")
    for k in n:
        if k not in KNOWN and k != "before_you_update":
            warnings.append("unknown key ignored: %s" % k)
    for place, g in (n.get("groups") or {}).items():
        if isinstance(g, dict):
            for k in g:
                if k not in KINDS:
                    warnings.append("unknown kind ignored in %s: %s" % (place, k))
    if not re.fullmatch(r"\d+\.\d+\.\d+", str(n.get("version", ""))):
        errors.append("version must be plain X.Y.Z (no v): %r" % n.get("version"))
    if empty(n.get("theme")):
        errors.append("theme is missing")
    if not images(n):
        errors.append("images is missing (registry + name)")
    hl = items(n.get("highlights"))
    if len(hl) != 3:
        errors.append("need exactly 3 highlights, found %d" % len(hl))
    glines = all_group_lines(n)
    if not glines:
        errors.append("groups has no lines")
    group_refs = {i for l in glines for i in refs(l)}
    for h in hl:
        iss = hl_issues(h)
        if not iss:
            errors.append("highlight has no issues: %r" % h.get("line"))
        for i in iss:
            if i in group_refs:
                errors.append("highlight issue #%d is also in a group line (show it once)" % i)
        if refs(h.get("line") or ""):
            errors.append("highlight line carries (#n), use issues instead: %r" % h.get("line"))
        if empty(h.get("shot")):
            errors.append("highlight has no shot: %r" % h.get("line"))
    for l in glines + items(n.get("security")) + items(n.get("known_issues")):
        if not re.search(r"\(#\d+\)\s*$", l):
            errors.append("line has no (#n) at the end: %r" % l)
    for l in glines:
        if re.match(r"(added|new)\s*:", l, re.I):
            errors.append("new is the default, drop the prefix: %r" % l)
    every = [n.get("theme") or ""] + [h.get("line") or "" for h in hl] + glines + \
        items(n.get("security")) + items(n.get("known_issues"))
    for l in every:
        words = len(no_refs(l).split())
        if words > 12:
            warnings.append("over 12 words (%d): %r" % (words, l))
        m = BANNED.search(l)
        if m:
            errors.append("banned word %r: %r" % (m.group(0), l))
        if "—" in l:
            errors.append("em dash: %r" % l)
        m = FORBIDDEN.search(l)
        if m:
            errors.append("no dates or launch talk (%r): %r" % (m.group(0), l))
    rep = [hl_line(h) for h in hl] + glines + items(n.get("security"))
    for i, a in enumerate(rep):
        for c in rep[i + 1:]:
            if refs(a) and refs(a) == refs(c):
                warnings.append("same issues on two lines: %r / %r" % (a, c))
            elif difflib.SequenceMatcher(None, no_refs(a).lower(), no_refs(c).lower()).ratio() > 0.8:
                warnings.append("near-identical lines: %r / %r" % (a, c))
    cs = items(n.get("credits"))
    names = {credit_key(c).lower() for c in cs if not (empty(c.get("name")) and empty(c.get("github")))}
    people = (n.get("stats") or {}).get("people_credited")
    if isinstance(people, int) and cs and people != len(names):
        warnings.append("stats.people_credited is %d but credits list %d names" % (people, len(names)))
    for c in cs:
        if empty(c.get("name")) and empty(c.get("github")):
            errors.append("credit without a name or github login")
            continue
        login = login_for(c, cmap)
        if login and not LOGIN.fullmatch(str(login)):
            errors.append("github login looks wrong for %s: %r" % (c.get("name"), login))
    if len(render_discord(n)) > DM_LIMIT:
        errors.append("discord DM over %d chars even after cutting" % DM_LIMIT)
    ann = render_discord(n, bold=True)
    if len(ann) > DM_LIMIT:
        errors.append("discord announce over %d chars (title included) even after cutting" % DM_LIMIT)
    if len(ann) > EMBED_LIMIT:
        errors.append("discord announce over the %d char embed limit" % EMBED_LIMIT)
    return errors, warnings


# ---------- cli ----------

def load_notes(path):
    return tomllib.loads(pathlib.Path(path).read_text(encoding="utf-8-sig"))


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
    ap.add_argument("--file", type=pathlib.Path, help="notes file (default releases/notes-<version>.toml)")
    ap.add_argument("--map", type=pathlib.Path, help="credits map (default releases/credits-map.json)")
    ap.add_argument("--date", help="changelog date (default today)")
    a = ap.parse_args(argv)
    src = a.file or ROOT / "releases" / ("notes-%s.toml" % a.version)
    notes = load_notes(src)
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
    print("wrote %d files to %s" % (len(FILES), out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
