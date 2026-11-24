#!/usr/bin/env python3
"""Builds and signs HolyArrs plugins and the two signed lists.

    publish.py init      [common flags]
    publish.py publish   --id ID --version X.Y.Z --src DIR [--test] [common flags]
    publish.py build     --src DIR --out DIR   (every platform, nothing signed)
    publish.py block     --id ID [--version X.Y.Z] [--sha256 HASH] --reason TEXT [common flags]

Common flags: --pluginkey BIN --key FILE --out DIR [--pass-env NAME]
[--live-url URL] [--main-pub FILE] [--backup-pub FILE] [--wiring FILE].
The key has no passphrase; --pass-env is only for an old sealed key.

--out is the site tree served at plugins.holyarrs.com (the R2 bucket
holyarrs-plugins). The workflow fills it with the live lists first, runs this,
then uploads the new files to R2: blocklist.json and one signed plugin list per platform. Every version is
built for linux/amd64, linux/arm64 and linux/arm/v7 (the app image's CPU
types). linux-amd64 keeps the first layout, plugins.json and
plugins/<id>/<version>/{manifest.json,plugin}, so older apps still read it;
the others are plugins-<platform>.json and
plugins/<id>/<version>/<platform>/{manifest.json,plugin}. Every run signs
all plugin lists with one shared counter, so an app's last counter carries
over between them.
The current lists are read from that tree and the new ones are written into
it. With --source a publish also writes the exact source commit as
source/<id>/<version>.tar.xz.
When the tree holds images/<id>/icon-128.webp and icon-256.webp (made by the
plugins repo's picture robot), the manifest names icon_url
"images/<id>/icon-256.webp" instead of an inline icon.
Uploading the tree is the workflow's job.

publish --test puts the version on the test lists instead (plugins-test.json
and plugins-test-<platform>.json, kind plugins_test, their own counter). Only
apps from 1.3.1 read them, and only for plugins whose owner asked for test
versions; older apps never see them. A block re-signs them too.

Before signing, the live lists at --live-url are fetched; if a live counter
is ahead of the tree's, the tree is stale and nothing is signed.
--live-url "" skips that check (only before the site exists).

Python standard library only. Never prints the passphrase or the key.
"""
import argparse
import lzma
import base64
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_LIVE = "https://plugins.holyarrs.com/"
# (platform name, GOOS, GOARCH, GOARM): the engine's names in platform.go.
PLATFORMS = [
    ("linux-amd64", "linux", "amd64", ""),
    ("linux-arm64", "linux", "arm64", ""),
    ("linux-arm-v7", "linux", "arm", "7"),
]
FIRST = PLATFORMS[0][0]


def list_name(plat):
    return "plugins.json" if plat == FIRST else f"plugins-{plat}.json"


def test_list_name(plat):
    return "plugins-test.json" if plat == FIRST else f"plugins-test-{plat}.json"


def version_rel(pid, ver, plat):
    rel = f"plugins/{pid}/{ver}/"
    return rel if plat == FIRST else rel + plat + "/"

# Optional page fields copied from holyarrs-plugin.json into the manifest.
DETAIL_KEYS = ("long_description", "how_it_works", "whats_new", "developers", "updated", "licence", "repo", "category",
               "support_email", "support_url", "settings")

ID_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
SEMVER_RE = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$")
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


class Fail(Exception):
    pass


# Length limits, in characters, so a plugin's card, dialog and settings stay tidy whatever its author types.
# The app accepts a little more (its own checks guard safety, not looks); a listing must stay inside these.
LIMITS = {
    "name": 40, "author": 40, "description": 160, "long_description": 2000, "whats_new": 300, "category": 24,
    "step": 160, "steps": 8, "developers": 5,
    "host": 80, "hosts": 10, "folder": 60, "folders": 10, "actions": 12,
    "settings": 20, "label": 48, "help": 160, "option": 40, "options": 12,
    "version": 16,
}


def check_lengths(meta, ver=None):
    """Refuses text that would break the layout, with a message naming the field, its length and the limit."""
    def text(what, v, most):
        if v is not None and len(str(v)) > most:
            raise Fail(f"{what} is {len(str(v))} characters; the most is {most}. Shorten it in holyarrs-plugin.json")

    def count(what, items, most):
        if len(items or []) > most:
            raise Fail(f"{what} has {len(items)} entries; the most is {most}")
    if ver is not None:
        text("--version", ver, LIMITS["version"])
    for k in ("name", "author", "description", "long_description", "whats_new", "category"):
        text(k, meta.get(k), LIMITS[k])
    count("how_it_works", meta.get("how_it_works"), LIMITS["steps"])
    for i, st in enumerate(meta.get("how_it_works") or []):
        text(f"how_it_works step {i + 1}", st, LIMITS["step"])
    count("developers", meta.get("developers"), LIMITS["developers"])
    acc = meta.get("access") or {}
    count("access.hosts", acc.get("hosts"), LIMITS["hosts"])
    count("access.folders", acc.get("folders"), LIMITS["folders"])
    count("access.actions", acc.get("actions"), LIMITS["actions"])
    for h in acc.get("hosts") or []:
        text(f"access host {h[:30]!r}", h, LIMITS["host"])
    for f in acc.get("folders") or []:
        text(f"access folder {f[:30]!r}", f, LIMITS["folder"])
    sets = meta.get("settings") or []
    count("settings", sets, LIMITS["settings"])
    for s in sets:
        key = s.get("key", "?")
        text(f"settings {key!r} label", s.get("label"), LIMITS["label"])
        text(f"settings {key!r} help", s.get("help"), LIMITS["help"])
        count(f"settings {key!r} options", s.get("options"), LIMITS["options"])
        for o in s.get("options") or []:
            text(f"settings {key!r} option {str(o)[:20]!r}", o, LIMITS["option"])


def sha(b):
    return hashlib.sha256(b).hexdigest()


def log(msg):
    print(msg, flush=True)


# ---------- reading lists ----------

def fetch(base, rel):
    """Returns the bytes of base/rel (URL or folder), or None when missing."""
    if base.startswith(("https://", "http://")):
        url = base.rstrip("/") + "/" + rel
        req = urllib.request.Request(url, headers={"Cache-Control": "no-cache", "User-Agent": "holyarrs-release-tools"})
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                return r.read()
        except urllib.error.HTTPError as e:
            if e.code == 404:
                return None
            raise Fail(f"fetching {url}: HTTP {e.code}")
    path = os.path.join(base, *rel.split("/"))
    if not os.path.exists(path):
        return None
    with open(path, "rb") as f:
        return f.read()


def run(cmd, **kw):
    r = subprocess.run(cmd, capture_output=True, text=True, **kw)
    if r.returncode != 0:
        raise Fail(f"{os.path.basename(cmd[0])} {cmd[1]} failed: {(r.stderr or r.stdout).strip()}")
    return r.stdout.strip()


def verify(a, path, pub):
    return run([a.pluginkey, "verify", "-pub", pub, "-in", path])


def payload_of(raw):
    return json.loads(base64.b64decode(json.loads(raw)["payload"]))


def read_list(a, src, name, kind, label):
    """A signed list's payload from src, after checking it with the main key."""
    raw = fetch(src, name)
    if raw is None:
        return None
    tmp = os.path.join(a.work, label + "-" + name)
    with open(tmp, "wb") as f:
        f.write(raw)
    verify(a, tmp, a.main_pub)
    p = payload_of(raw)
    if p.get("kind") != kind:
        raise Fail(f"{label} {name} is kind {p.get('kind')!r}")
    return p


def current(a, name, kind):
    """The tree's list payload; refuses when the live list is ahead of it."""
    p = read_list(a, a.out, name, kind, "tree")
    have = p["counter"] if p else 0
    if a.live_url:
        live = read_list(a, a.live_url, name, kind, "live")
        if live and live["counter"] > have:
            raise Fail(f"{name}: live counter {live['counter']} is ahead of the tree's {have}; "
                       "the tree is stale, pull the plugins repo first")
        if live and live["counter"] == have and live != p:
            raise Fail(f"{name}: live and tree both have counter {have} but differ")
    if p is None:
        log(f"{name}: not published yet, starting empty")
    else:
        log(f"{name}: counter {p['counter']}, {len(p.get('entries') or [])} entries")
    return p


# ---------- signing ----------

def sign(a, kind, payload, last_counter, name=None):
    """Signs payload into the work folder and checks it with both pinned keys."""
    name = name or ("plugins.json" if kind == "plugins" else "blocklist.json")
    if payload["counter"] <= last_counter:
        raise Fail(f"{name}: counter {payload['counter']} does not go up from {last_counter}")
    pin = os.path.join(a.work, name + ".payload.json")
    with open(pin, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2)
    out = os.path.join(a.work, name)
    cmd = [a.pluginkey, "sign", "-key", a.key, "-kind", kind, "-in", pin, "-out", out,
           "-min-counter", str(last_counter)]
    if a.pass_env:
        cmd += ["-pass-env", a.pass_env]
    run(cmd)
    log(f"{name}: signed, " + verify(a, out, a.main_pub))
    if not a.backup_pub:
        return name, out
    try:
        verify(a, out, a.backup_pub)
        log(f"{name}: also valid with the backup key")
    except Fail:
        log(f"{name}: not signed with the backup key (expected while the main key signs)")
    return name, out


def check_pins(a):
    """The key files must match the keys pinned in the app, when given."""
    if not a.wiring:
        return
    src = open(a.wiring, encoding="utf-8").read()
    for label, path in (("main", a.main_pub), ("backup", a.backup_pub)):
        if not path:
            continue
        key = open(path, encoding="utf-8").read().strip()
        if f'"{key}", // {label}' not in src:
            raise Fail(f"{path} is not the {label} key pinned in {a.wiring}")
    log("key files match the keys pinned in the app")


def empty_plugins(min_block):
    return {"kind": "plugins", "counter": 0, "min_blocklist_counter": min_block, "entries": []}


def empty_block():
    return {"kind": "blocklist", "counter": 0, "entries": []}


# ---------- commands ----------

def current_lists(a):
    """Every platform's plugin list payload (None where not published yet)."""
    return {plat: current(a, list_name(plat), "plugins") for plat, *_ in PLATFORMS}


def top_counter(lists):
    return max([p["counter"] for p in lists.values() if p] or [0])


def current_test_lists(a):
    """Every platform's test-version list payload (None where there is none)."""
    return {plat: current(a, test_list_name(plat), "plugins_test") for plat, *_ in PLATFORMS}


def sign_lists(a, lists, counter, min_block, add=None, kind="plugins"):
    """Signs every platform's plugin list (or, with kind plugins_test, test
    list) at one counter; add maps a platform to the new entry for it."""
    namer = test_list_name if kind == "plugins_test" else list_name
    files = []
    for plat, *_ in PLATFORMS:
        old = lists.get(plat) or {}
        entries = list(old.get("entries") or [])
        if add:
            entries.append(add[plat])
        new = {"kind": kind, "counter": counter,
               "min_blocklist_counter": max(old.get("min_blocklist_counter") or 0, min_block),
               "entries": entries}
        files.append(sign(a, kind, new, old.get("counter", 0), namer(plat)))
    return files


def cmd_init(a):
    """Signs empty lists where none exist yet."""
    pls, bl = current_lists(a), current(a, "blocklist.json", "blocklist")
    if bl and all(pls.values()):
        log("all lists already exist; nothing to do")
        return []
    files = []
    if not bl:
        bl = empty_block()
        bl["counter"] = 1
        files.append(sign(a, "blocklist", bl, 0))
    counter = max(top_counter(pls), 1)
    for plat, *_ in PLATFORMS:
        if not pls[plat]:
            p = empty_plugins(bl["counter"])
            p["counter"] = counter
            files.append(sign(a, "plugins", p, 0, list_name(plat)))
    return files


def load_meta(src, pid, listed=False):
    path = os.path.join(src, "holyarrs-plugin.json")
    if not os.path.exists(path):
        raise Fail(f"{path} is missing (name, author, access, min_app_version, ...)")
    meta = json.load(open(path, encoding="utf-8"))
    if meta.get("id", pid) != pid:
        raise Fail(f"holyarrs-plugin.json names id {meta['id']!r}, not {pid!r}")
    # icon_url in holyarrs-plugin.json is the author's own logo link; the
    # workflow's picture robot copies it into the tree before this runs, and
    # the manifest names only that copy (logo_rel), never the author's link.
    meta.pop("icon_url", None)
    icon_file = meta.pop("icon_file", None)
    if icon_file:
        ext = os.path.splitext(icon_file)[1].lower()
        kind = {".svg": "image/svg+xml", ".png": "image/png"}.get(ext)
        if not kind:
            raise Fail("icon_file must be .svg or .png")
        meta["icon"] = f"data:{kind};base64," + base64.b64encode(open(os.path.join(src, icon_file), "rb").read()).decode()
    # People need a way to ask the author for help: the Contact button copies support_email.
    # A new listing must name an email; plugins already listed may keep only support_url.
    if not meta.get("licence"):
        raise Fail("holyarrs-plugin.json needs a licence")
    if not meta.get("support_email") and not (listed and meta.get("support_url")):
        raise Fail("holyarrs-plugin.json needs support_email (support_url is optional)")
    if meta.get("support_url") and not str(meta["support_url"]).startswith("https://"):
        raise Fail("support_url must be an https link")
    check_lengths(meta)
    return meta


def version_stamp(src):
    """Linker flags so a bundled Tailscale reports its own version (1.104.7), not 1.104.7-ERR-BuildInfo.

    Tailscale reads its version from build info, which a plugin build doesn't carry for it;
    its version package takes the stamp from these flags instead."""
    try:
        mod = open(os.path.join(src, "go.mod"), encoding="utf-8").read()
    except OSError:
        return ""
    m = re.search(r"^\s*(?:require\s+)?tailscale\.com\s+v([0-9]+\.[0-9]+\.[0-9]+)\b", mod, re.M)
    if not m:
        return ""
    v = m.group(1)
    return f" -X tailscale.com/version.longStamp={v} -X tailscale.com/version.shortStamp={v}"


def build(src, dest, goos, goarch, goarm):
    env = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=goarch)
    env.pop("GOARM", None)
    if goarm:
        env["GOARM"] = goarm
    run(["go", "build", "-trimpath", "-ldflags", "-s -w -buildid=" + version_stamp(src), "-o", os.path.abspath(dest), "."], cwd=src, env=env)
    log(f"built {goos}/{goarch}{'/v' + goarm if goarm else ''}: {os.path.getsize(dest)} bytes")


def cmd_build(a):
    """Builds every platform into --out/<platform>/plugin; signs nothing."""
    for plat, goos, goarch, goarm in PLATFORMS:
        dest = os.path.join(a.out, plat, "plugin")
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        build(a.src, dest, goos, goarch, goarm)


def cmd_publish(a):
    pid, ver = a.id, a.version
    if not ID_RE.match(pid) or not SEMVER_RE.match(ver):
        raise Fail("--id must be lowercase letters, digits and dashes; --version plain semver")
    check_lengths({}, ver)
    pls, bl = current_lists(a), current(a, "blocklist.json", "blocklist")
    tls = current_test_lists(a)
    for plat, *_ in PLATFORMS:
        rel_man = version_rel(pid, ver, plat) + "manifest.json"
        if fetch(a.out, rel_man) is not None or (a.live_url and fetch(a.live_url, rel_man) is not None) or any(
                e["id"] == pid and e["version"] == ver
                for e in ((pls[plat] or {}).get("entries") or []) + ((tls[plat] or {}).get("entries") or [])):
            raise Fail(f"{pid} {ver} is already published; a published version never changes, use a new version")
    for e in (bl or {}).get("entries") or []:
        if e.get("id") == pid and e.get("version") in ("", ver):
            raise Fail(f"{pid} {ver} is on the blocklist")

    listed = any(e["id"] == pid for plat, *_ in PLATFORMS for e in (pls[plat] or {}).get("entries") or [])
    meta = load_meta(a.src, pid, listed)
    if meta["access"].get("events") and tuple(int(x) for x in meta["min_app_version"].split("-")[0].split(".")) < (1, 3, 1):
        raise Fail("access.events needs min_app_version 1.3.1 or newer: older apps refuse the manifest")
    # The picture robot's copy of the logo, when it made one for this run.
    logo = f"images/{pid}/icon-256.webp"
    if not all(os.path.isfile(os.path.join(a.out, "images", pid, f"icon-{s}.webp")) for s in (128, 256)):
        logo = ""
    files, add, built, files_r2 = [], {}, {}, []
    for plat, goos, goarch, goarm in PLATFORMS:
        rel = version_rel(pid, ver, plat)
        vdir = os.path.join(a.work, *rel.strip("/").split("/"))
        os.makedirs(vdir, exist_ok=True)
        plugin = os.path.join(vdir, "plugin")
        build(a.src, plugin, goos, goarch, goarm)
        manifest = {
            "id": pid, "name": meta["name"], "version": ver, "author": meta["author"],
            **({"description": meta["description"]} if meta.get("description") else {}),
            **({"icon_url": logo} if logo else {"icon": meta["icon"]} if meta.get("icon") else {}),
            **{k: meta[k] for k in DETAIL_KEYS if meta.get(k)},
            "platforms": [p for p, *_ in PLATFORMS],
            "size": os.path.getsize(plugin),
            "access": {**{k: meta["access"].get(k, []) for k in ("hosts", "folders", "actions")},
                       # Events only when listed: apps before 1.3.1 refuse the key.
                       **({"events": meta["access"]["events"]} if meta["access"].get("events") else {})},
            "min_app_version": meta["min_app_version"],
            "sha256": sha(open(plugin, "rb").read()),
        }
        man = json.dumps(manifest, indent=2).encode() + b"\n"
        man_path = os.path.join(vdir, "manifest.json")
        with open(man_path, "wb") as f:
            f.write(man)
        files.append((rel + "manifest.json", man_path))
        if not a.file_url or a.pages_copy:
            files.append((rel + "plugin", plugin))
        add[plat] = {"id": pid, "version": ver, "sha256": manifest["sha256"], "manifest_sha256": sha(man)}
        if a.file_url:
            # The file host (R2) serves the big file; the list names it, signed.
            add[plat]["file_url"] = a.file_url.rstrip("/") + "/" + rel.lstrip("/") + "plugin"
            files_r2.append((rel + "plugin", plugin))
        built[plat] = (man_path, plugin)

    block_path = os.path.join(a.out, "blocklist.json")
    if not bl:  # first run: installs need a blocklist too
        bl = empty_block()
        bl["counter"] = 1
        files.append(sign(a, "blocklist", bl, 0))
        block_path = files[-1][1]
    kind = "plugins_test" if a.test else "plugins"
    base = tls if a.test else pls
    signed = sign_lists(a, base, top_counter(base) + 1, bl["counter"], add, kind)
    files += signed

    # Last gate: the app's own install check on exactly these files.
    for (plat, *_), (_, list_path) in zip(PLATFORMS, signed):
        man_path, plugin = built[plat]
        log(plat + ": " + run([a.pluginkey, "check", "-pub", a.main_pub, "-list", list_path,
                               "-blocklist", block_path, "-manifest", man_path, "-file", plugin, "-kind", kind]))
    if a.files_out:
        files_r2.append(source_archive(a, pid, ver))
        a.r2_files = files_r2
    elif a.source:
        files.append(source_archive(a, pid, ver))
    return files


def source_archive(a, pid, ver):
    """The exact source commit as source/<id>/<version>.tar.xz (xz, max compression)."""
    top = run(["git", "-C", a.src, "rev-parse", "--show-toplevel"]).strip()
    commit = run(["git", "-C", top, "rev-parse", "HEAD"]).strip()
    tar = subprocess.run(["git", "-C", top, "archive", "--format=tar",
                          "--prefix", f"{pid}-{ver}/", commit], check=True, capture_output=True).stdout
    path = os.path.join(a.work, "source.tar.xz")
    with open(path, "wb") as f:
        f.write(lzma.compress(tar, preset=9 | lzma.PRESET_EXTREME))
    log(f"source archive of {commit}: {os.path.getsize(path)} bytes")
    return (f"source/{pid}/{ver}.tar.xz", path)


def cmd_block(a):
    if not a.id and not a.sha256:
        raise Fail("give --id (with or without --version) or --sha256")
    if (a.id and not ID_RE.match(a.id)) or (a.version and not SEMVER_RE.match(a.version)) or \
            (a.sha256 and not SHA_RE.match(a.sha256)):
        raise Fail("bad --id, --version or --sha256")
    if not a.reason.strip():
        raise Fail("--reason is required")
    pls, bl = current_lists(a), current(a, "blocklist.json", "blocklist")
    bl = bl or empty_block()
    entry = {"id": a.id or "", "version": a.version or "", "sha256": a.sha256 or ""}
    entries = list(bl.get("entries") or [])
    if entry in entries:
        raise Fail("that entry is already on the blocklist")
    entries.append(entry)
    new_bl = {"kind": "blocklist", "counter": bl["counter"] + 1, "entries": entries}
    files = [sign(a, "blocklist", new_bl, bl["counter"])]
    # Every plugin list now demands this blocklist, so an install can never
    # be fed the older one that did not block this version.
    files += sign_lists(a, pls, top_counter(pls) + 1, new_bl["counter"])
    tls = current_test_lists(a)
    if any(tls.values()):
        files += sign_lists(a, tls, top_counter(tls) + 1, new_bl["counter"], kind="plugins_test")
    reasons = os.path.join(a.work, "block-reasons.txt")
    prev = fetch(a.out, "block-reasons.txt") or b""
    with open(reasons, "wb") as f:
        f.write(prev + f"blocklist {new_bl['counter']}: {json.dumps(entry)} reason: {a.reason}\n".encode())
    files.insert(0, ("block-reasons.txt", reasons))
    return files


# ---------- writing the tree ----------

def write_tree(a, files):
    """Only after every step passed: versioned files, then blocklist.json, then plugins.json."""
    order = {"blocklist.json": 1, **{list_name(p): 2 for p, *_ in PLATFORMS}, **{test_list_name(p): 2 for p, *_ in PLATFORMS}}
    for rel, path in sorted(files, key=lambda f: order.get(f[0], 0)):
        dest = os.path.join(a.out, *rel.split("/"))
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        shutil.copyfile(path, dest)
        log(f"wrote {rel}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("command", choices=["init", "publish", "block", "build"])
    ap.add_argument("--id")
    ap.add_argument("--version")
    ap.add_argument("--src")
    ap.add_argument("--sha256")
    ap.add_argument("--reason", default="")
    ap.add_argument("--pluginkey")
    ap.add_argument("--key", help="private key file")
    ap.add_argument("--pass-env", help="env var holding the passphrase of an old sealed key (optional)")
    ap.add_argument("--out", required=True, help="the plugins repo tree")
    ap.add_argument("--live-url", default=DEFAULT_LIVE,
                    help="live lists to compare counters with: https URL, a folder, or empty to skip")
    ap.add_argument("--main-pub", default=os.path.join(HERE, "keys", "main.pub"))
    ap.add_argument("--backup-pub", help="a second pinned key to also try (optional; none today)")
    ap.add_argument("--wiring", help="plugins_wiring.go, to check the key files against the app's pins")
    ap.add_argument("--file-url", default="",
                    help="optional https base of a separate file host; the list then names each "
                         "plugin file there (normally empty: files sit next to the lists)")
    ap.add_argument("--files-out", help="folder for the file host tree (plugin files and source archive)")
    ap.add_argument("--source", action="store_true",
                    help="publish: also write source/<id>/<version>.tar.xz into --out")
    ap.add_argument("--test", action="store_true",
                    help="publish: put the version on the test lists, for owners who asked for test versions")
    ap.add_argument("--pages-copy", action="store_true",
                    help="also keep the plugin file in the list tree, for apps before file_url (1.2.0)")
    a = ap.parse_args()
    if a.command == "publish" and bool(a.file_url) != bool(a.files_out):
        ap.error("--file-url and --files-out go together")
    if a.file_url and not a.file_url.startswith("https://"):
        ap.error("--file-url must be https")
    if a.command == "publish" and not (a.id and a.version and a.src):
        ap.error("publish needs --id, --version and --src")
    if a.command == "build":
        if not a.src:
            ap.error("build needs --src")
    elif not (a.pluginkey and a.key):
        ap.error(a.command + " needs --pluginkey and --key")
    os.makedirs(a.out, exist_ok=True)
    a.work = tempfile.mkdtemp(prefix="holyarrs-publish-")
    try:
        check_pins(a)
        if a.command == "build":
            cmd_build(a)
            return 0
        files = {"init": cmd_init, "publish": cmd_publish, "block": cmd_block}[a.command](a)
        if getattr(a, "r2_files", None):
            # File host first: the lists that point at these files come last.
            write_tree(argparse.Namespace(out=a.files_out), a.r2_files)
        write_tree(a, files)
    except Fail as e:
        print("error:", e, file=sys.stderr)
        return 1
    finally:
        shutil.rmtree(a.work, ignore_errors=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
