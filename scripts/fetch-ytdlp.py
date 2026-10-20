"""Fetch the pinned yt-dlp release files that packages ship next to holyarrs.

Downloads only from yt-dlp's official GitHub release and checks every file
against the SHA-256 pinned in package.py (YTDLP). Usage:
    python scripts/fetch-ytdlp.py <new folder>
then pass that folder to package.py as --ytdlp-dir.
"""
from __future__ import annotations

import hashlib
import sys
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from package import YTDLP, YTDLP_VERSION  # noqa: E402


def main() -> None:
    out = Path(sys.argv[1])
    out.mkdir(parents=True, exist_ok=True)
    for asset, want in sorted(set(YTDLP.values())):
        url = f"https://github.com/yt-dlp/yt-dlp/releases/download/{YTDLP_VERSION}/{asset}"
        with urllib.request.urlopen(url, timeout=300) as r:
            data = r.read(256 << 20)
        if hashlib.sha256(data).hexdigest() != want:
            raise SystemExit(f"{asset}: checksum mismatch")
        (out / asset).write_bytes(data)
        print(f"{asset} ok")


if __name__ == "__main__":
    main()
