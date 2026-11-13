"""Packs the built language files with bzip2 for the image.

    python3 deploy/pack_languages.py app/frontend/dist

The image build leaves each language file plain (HOLYARRS_LANGUAGES_PLAIN in
the web stage) and this step replaces it with name.bz2, which is smaller than
the brotli copy for every language. The program unpacks it when a browser asks
for the language (app/frontend/embed.go). Other builds keep the brotli copies.
"""

import bz2
import pathlib
import sys


def main() -> int:
    dist = pathlib.Path(sys.argv[1])
    packed = before = after = 0
    for path in sorted((dist / "_app" / "immutable" / "assets").glob("*.json")):
        data = path.read_bytes()
        if not data.startswith(b'{"fit":"'):
            continue
        small = bz2.compress(data, 9)
        if bz2.decompress(small) != data:
            raise SystemExit(f"{path.name}: the packed copy does not unpack to the same bytes")
        path.with_name(path.name + ".bz2").write_bytes(small)
        path.unlink()
        packed += 1
        before += len(data)
        after += len(small)
    if packed == 0:
        raise SystemExit("no plain language files found: was the web stage built with HOLYARRS_LANGUAGES_PLAIN=1?")
    print(f"languages: {packed} files, {before // 1024} KB -> {after // 1024} KB")
    return 0


if __name__ == "__main__":
    sys.exit(main())
