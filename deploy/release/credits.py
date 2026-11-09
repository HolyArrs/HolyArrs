#!/usr/bin/env python3
"""Add GitHub people to a release's credits: everyone who opened or commented on
an issue in the milestone, except the maintainer and bots.

    python deploy/release/credits.py <version> [--file NOTES] [--map MAP] [--dry-run]

Reads GitHub with `gh api` (read only). A login already mapped to a Reddit name in
credits-map.json is added to that person's credit as `github`; anyone else gets
their own `{github, link, issues}` credit. Logins are never guessed.
"""
import argparse
import json
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
REPO = "HolyArrs/HolyArrs"
MAINTAINER = "CoderLuii"


def gh(path):
    """One `gh api` GET, every page, as a flat list."""
    out = subprocess.run(["gh", "api", "--paginate", "--slurp", path],
                         capture_output=True, text=True, encoding="utf-8", check=True).stdout
    data = json.loads(out or "[]")
    flat = []
    for page in data:
        flat.extend(page if isinstance(page, list) else [page])
    return flat


def is_person(user):
    login = (user or {}).get("login") or ""
    if not login or login.lower() == MAINTAINER.lower():
        return False
    return (user.get("type") or "User") == "User" and not login.lower().endswith("[bot]")


def collect(version, api=gh, repo=REPO):
    """login -> {"link": first issue or comment url, "issues": [numbers]} in issue order."""
    ms = [m for m in api("repos/%s/milestones?state=all&per_page=100" % repo) if str(m.get("title")) == str(version)]
    if not ms:
        raise SystemExit("no milestone named %s on %s" % (version, repo))
    issues = api("repos/%s/issues?milestone=%d&state=all&per_page=100" % (repo, ms[0]["number"]))
    people = {}

    def add(user, url, num):
        if not is_person(user):
            return
        p = people.setdefault(user["login"], {"link": url, "issues": []})
        if num not in p["issues"]:
            p["issues"].append(num)

    for issue in sorted(issues, key=lambda i: i["number"]):
        add(issue.get("user"), issue.get("html_url"), issue["number"])
        for c in api("repos/%s/issues/%d/comments?per_page=100" % (repo, issue["number"])):
            add(c.get("user"), c.get("html_url"), issue["number"])
    return people


def bare(name):
    name = str(name)
    return name[2:] if name.startswith("u/") else name


def merge(text, notes, people, cmap):
    """Return (new notes text, list of what changed). Edits only the credits list."""
    credits = notes.get("credits") or []
    known = {str(c.get("github", "")).lower() for c in credits if c.get("github")}
    by_login = {}
    for reddit, login in cmap.items():
        if isinstance(login, str) and login:
            by_login[login.lower()] = bare(reddit)
    names = {bare(c.get("name", "")).lower() for c in credits if c.get("name")}
    lines = text.split("\n")
    start = next((i for i, l in enumerate(lines) if re.match(r"credits\s*=\s*\[", l)), None)
    if start is None:
        raise SystemExit("notes file has no credits = [ ... ] list")
    end = next(i for i in range(start, len(lines)) if lines[i].strip() == "]" or lines[i].rstrip().endswith("[]"))
    changes, new = [], []
    for login, p in people.items():
        if login.lower() in known:
            continue
        reddit = by_login.get(login.lower())
        if reddit and reddit.lower() in names:
            pat = re.compile(r'(\{\s*name\s*=\s*"(?:u/)?%s"\s*,)' % re.escape(reddit), re.I)
            for i in range(start, end):
                if pat.search(lines[i]) and "github" not in lines[i]:
                    lines[i] = pat.sub(r'\1 github = "%s",' % login, lines[i], count=1)
                    changes.append("u/%s gets @%s" % (reddit, login))
                    break
            continue
        new.append('  {github = "%s", link = "%s", issues = [%s]},'
                   % (login, p["link"], ", ".join(str(i) for i in p["issues"])))
        changes.append("added @%s" % login)
    if new:
        if lines[end].rstrip().endswith("[]"):
            lines[end] = re.sub(r"\[\]\s*(#.*)?$", "[", lines[end])
            lines[end + 1:end + 1] = new + ["]"]
        else:
            lines[end:end] = new
    return "\n".join(lines), changes


def load_map(path):
    if not path.exists():
        return {}
    data = json.loads(path.read_text(encoding="utf-8-sig"))
    return data.get("map", data) if isinstance(data, dict) else {}


def main(argv=None, api=gh):
    import tomllib
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("version")
    ap.add_argument("--file", type=pathlib.Path)
    ap.add_argument("--map", type=pathlib.Path)
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args(argv)
    src = a.file or ROOT / "releases" / ("notes-%s.toml" % a.version)
    text = src.read_text(encoding="utf-8-sig")
    people = collect(a.version, api)
    new, changes = merge(text, tomllib.loads(text), people,
                         load_map(a.map or ROOT / "releases" / "credits-map.json"))
    for c in changes:
        print(c)
    if not changes:
        print("no one new on GitHub for %s" % a.version)
    elif not a.dry_run:
        with open(src, "w", encoding="utf-8", newline="\n") as f:
            f.write(new)
        print("check stats.people_credited, then run notes.py --check")
    return 0


if __name__ == "__main__":
    sys.exit(main())
