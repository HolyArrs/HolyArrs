"""Enable TV and music on the exact disposable Intercom preview Source.

This helper is intentionally narrower than the normal settings API. It is for
the isolated preview only, where the local-network owner session cannot satisfy
the password-strength guard on Source edits. It changes exactly two columns on
the one known fake Source and refuses every other database, URL, or prior state.
"""

from __future__ import annotations

import argparse
import json
import sqlite3
import tempfile
import urllib.request
from pathlib import Path


EXPECTED_DB = (
    Path(tempfile.gettempdir())
    / "holyarrs-intercom-preview-18747"
    / "config"
    / "holyarrs.db"
).resolve()
EXPECTED_URL = "http://127.0.0.1:28767"
ORIGINAL_KINDS = ["movie"]
ORIGINAL_CATEGORIES = {"movie": [2000]}
PREVIEW_KINDS = ["movie", "tv", "music"]
PREVIEW_CATEGORIES = {"movie": [2000], "tv": [5000], "music": [3000]}


def fake_source_is_ready() -> None:
    request = urllib.request.Request(f"{EXPECTED_URL}/?t=caps", method="GET")
    with urllib.request.urlopen(request, timeout=3) as response:
        body = response.read(64_000).decode("utf-8", errors="strict")
        if response.status != 200:
            raise RuntimeError(f"fake Source returned HTTP {response.status}")
    markers = (
        "<caps>",
        '<movie-search available="yes"',
        '<tv-search available="yes"',
        '<music-search available="yes"',
        '<category id="2000"',
        '<category id="3000"',
        '<category id="5000"',
    )
    missing = [marker for marker in markers if marker not in body]
    if missing:
        raise RuntimeError(f"port 28767 is not the expected preview fake Source; missing {missing}")


def read_state(connection: sqlite3.Connection) -> tuple[str, str, list[str], dict[str, list[int]]]:
    rows = connection.execute(
        "SELECT url, kinds, categories FROM sources WHERE id = 1"
    ).fetchall()
    total = connection.execute("SELECT count(*) FROM sources").fetchone()[0]
    if total != 1 or len(rows) != 1:
        raise RuntimeError(f"expected only Source id 1, found {total} Sources")
    url, raw_kinds, raw_categories = rows[0]
    if url != EXPECTED_URL:
        raise RuntimeError(f"Source id 1 has unexpected URL {url!r}")
    try:
        kinds = json.loads(raw_kinds)
        categories = json.loads(raw_categories)
    except (TypeError, json.JSONDecodeError) as error:
        raise RuntimeError("Source id 1 has invalid kinds/categories JSON") from error
    return raw_kinds, raw_categories, kinds, categories


def state_name(kinds: list[str], categories: dict[str, list[int]]) -> str:
    if kinds == ORIGINAL_KINDS and categories == ORIGINAL_CATEGORIES:
        return "movie-only"
    if kinds == PREVIEW_KINDS and categories == PREVIEW_CATEGORIES:
        return "movie-tv-music"
    raise RuntimeError(
        "refusing Source id 1 because its kinds/categories are neither the "
        "original movie-only state nor the exact preview all-kinds state"
    )


def configure(check_only: bool) -> str:
    if EXPECTED_DB != (
        Path(tempfile.gettempdir())
        / "holyarrs-intercom-preview-18747"
        / "config"
        / "holyarrs.db"
    ).resolve():
        raise RuntimeError("preview database path guard failed")
    if not EXPECTED_DB.is_file():
        raise RuntimeError(f"disposable preview database is missing: {EXPECTED_DB}")
    fake_source_is_ready()

    connection = sqlite3.connect(EXPECTED_DB, timeout=5, isolation_level=None)
    try:
        connection.execute("PRAGMA busy_timeout = 5000")
        connection.execute("BEGIN" if check_only else "BEGIN IMMEDIATE")
        raw_kinds, raw_categories, kinds, categories = read_state(connection)
        current = state_name(kinds, categories)
        if check_only or current == "movie-tv-music":
            connection.rollback()
            return current

        desired_kinds = json.dumps(PREVIEW_KINDS, separators=(",", ":"))
        desired_categories = json.dumps(PREVIEW_CATEGORIES, separators=(",", ":"))
        changed = connection.execute(
            "UPDATE sources SET kinds = ?, categories = ? "
            "WHERE id = 1 AND url = ? AND kinds = ? AND categories = ?",
            (desired_kinds, desired_categories, EXPECTED_URL, raw_kinds, raw_categories),
        ).rowcount
        if changed != 1:
            raise RuntimeError(f"guarded Source update changed {changed} rows, expected 1")
        _, _, after_kinds, after_categories = read_state(connection)
        if state_name(after_kinds, after_categories) != "movie-tv-music":
            raise RuntimeError("Source verification failed inside the transaction")
        connection.commit()
        return "movie-tv-music"
    except Exception:
        if connection.in_transaction:
            connection.rollback()
        raise
    finally:
        connection.close()


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Enable TV/music only on the exact disposable Intercom preview Source."
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="validate the fake Source and database state without changing it",
    )
    args = parser.parse_args()
    state = configure(args.check)
    action = "validated" if args.check else "configured"
    print(f"Disposable preview Source {action}: {state}.")


if __name__ == "__main__":
    main()
