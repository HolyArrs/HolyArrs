"""Packs the language files for the plugin list.

    python3 deploy/pack_languages.py app/frontend/lang-out OUT

The image ships English only. `npm run build` (scripts/pack-lang.mjs) writes one
file per other language to app/frontend/lang-out, each naming its language and
app version. This step packs each with bzip2 as OUT/<lang>/<app>.json.bz2 and
writes OUT/entries.json: the entries the signed languages.json list needs
({"id": "lang-<lang>", "version": "<app>", "sha256": ...}). The app downloads a
pack, checks its hash against the signed list and unpacks it with Go's own
bzip2 reader (internal/plugins/languages.go).
"""

import bz2
import hashlib
import json
import pathlib
import sys


def pack(src: pathlib.Path, out: pathlib.Path) -> list[dict]:
    entries = []
    for path in sorted(src.glob("*.json")):
        data = path.read_bytes()
        head = json.loads(data)
        lang, app = head.get("lang"), head.get("app")
        if not lang or not app or path.stem != lang or not head.get("fit") or not head.get("values"):
            raise SystemExit(f"{path.name}: not a language pack (needs lang, app, fit and values)")
        small = bz2.compress(data, 9)
        if bz2.decompress(small) != data:
            raise SystemExit(f"{path.name}: the packed copy does not unpack to the same bytes")
        dest = out / lang / f"{app}.json.bz2"
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(small)
        entries.append({"id": f"lang-{lang}", "version": app, "sha256": hashlib.sha256(small).hexdigest()})
    if not entries:
        raise SystemExit(f"no language files in {src}: run npm run build first")
    if len({e["version"] for e in entries}) != 1:
        raise SystemExit("the language files are for more than one app version")
    (out / "entries.json").write_text(json.dumps(entries, indent=1) + "\n")
    return entries


def main() -> int:
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    entries = pack(pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]))
    print(f"languages: {len(entries)} packs for {entries[0]['version']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
