#!/usr/bin/env python3
"""Guard disposable real-save lifecycle data on the Intercom preview.

The helper speaks one JSON object on stdin and one JSON object on stdout.  It
is intentionally locked to the retained preview database on port 18747.  It
never accepts a database override and never reads or writes an installed app.
"""

from __future__ import annotations

import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import sqlite3
import stat
import subprocess
import sys
import tempfile
from datetime import datetime, timedelta, timezone
from typing import Any, Iterable


HERE = Path(__file__).resolve().parent
REPO = HERE.parents[2]
PREVIEW_ROOT = Path(tempfile.gettempdir()) / "holyarrs-intercom-preview-18747"
PREVIEW_MEDIA_ROOT = Path(tempfile.gettempdir()) / "holyarrs-intercom-preview-media-18747"
DB = PREVIEW_ROOT / "config" / "holyarrs.db"
EVIDENCE_PARENT = REPO / ".omx" / "artifacts" / "visual-ralph" / "intercom" / "real-save"
RUN_RE = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
NAME_PREFIX_RE = re.compile(r"^[a-z0-9][a-z0-9._-]{2,39}$")
RUN_INVITE_COUNT = 2
INVITE_PERMS = '["ask"]'
INVITE_ROLE = "member"
SESSION_ABSOLUTE_TIMEOUT = timedelta(days=30)
SESSION_USER_AGENT = "HolyArrs real-save fixture bootstrap"

EXACT_TABLES = ("settings", "api_keys", "secrets", "user_links")
ORIGINAL_EXACT_TABLES = ("users", "asks", "ask_people", "invites")
STABLE_COLUMNS = {
    "titles": (
        "id", "created_at", "created_by", "kind", "name", "sort_name",
        "source_ids", "metadata_source", "why_added",
    ),
    "slots": ("id", "created_at", "created_by", "item_id", "version_id"),
    "imports": (
        "id", "created_at", "created_by", "download_id", "from_path",
        "kind", "mode",
    ),
    "downloads": (
        "id", "created_at", "created_by", "downloader_id", "release_id",
        "external_id", "save_path", "started_by",
    ),
}
COUNT_TABLES = (
    "users", "invites", "sessions", "settings", "api_keys", "secrets",
    "user_links", "asks", "ask_people", "titles", "slots", "imports",
    "downloads",
)


class GuardError(RuntimeError):
    pass


def utc_now() -> str:
    return store_time(datetime.now(timezone.utc))


def store_time(value: datetime) -> str:
    # Match the backend's fixed-width nanosecond TimeLayout exactly.
    return value.strftime("%Y-%m-%dT%H:%M:%S.") + f"{value.microsecond:06d}000Z"


def normalized(path: Path) -> str:
    return os.path.normcase(os.path.abspath(str(path)))


def is_link(path: Path) -> bool:
    try:
        if path.is_symlink():
            return True
        is_junction = getattr(path, "is_junction", None)
        return bool(is_junction and is_junction())
    except OSError as exc:
        raise GuardError(f"could not inspect path component: {path}") from exc


def require_exact_path(actual: Path, expected: Path, label: str, *, directory: bool = False) -> Path:
    if normalized(actual) != normalized(expected):
        raise GuardError(f"{label} is not the canonical disposable preview path")
    if not actual.exists():
        raise GuardError(f"{label} does not exist")
    if is_link(actual):
        raise GuardError(f"{label} may not be a symlink or junction")
    if directory and not actual.is_dir():
        raise GuardError(f"{label} is not a directory")
    if not directory and not actual.is_file():
        raise GuardError(f"{label} is not a file")
    if normalized(Path(os.path.realpath(actual))) != normalized(expected):
        raise GuardError(f"{label} resolves outside its canonical path")
    return actual


def require_tree_without_links(root: Path) -> None:
    require_exact_path(root, root, "preview data root", directory=True)
    for current, dirs, files in os.walk(root, followlinks=False):
        current_path = Path(current)
        for name in [*dirs, *files]:
            child = current_path / name
            if is_link(child):
                raise GuardError(f"preview data contains a symlink or junction: {child}")


def ensure_evidence_path(run_id: str, supplied: Any, *, create: bool | None) -> Path:
    if not isinstance(supplied, str) or not supplied:
        raise GuardError("evidenceDir is required")
    expected = EVIDENCE_PARENT / run_id
    actual = Path(supplied)
    if normalized(actual) != normalized(expected):
        raise GuardError("evidenceDir must be the run-specific directory in this worktree")
    if normalized(Path(os.path.commonpath([REPO, actual]))) != normalized(REPO):
        raise GuardError("evidenceDir is outside this worktree")

    relative_probe = expected.relative_to(REPO) / "baseline.db"
    ignored = subprocess.run(
        ["git", "check-ignore", "--quiet", "--", str(relative_probe)],
        cwd=REPO,
        check=False,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    if ignored.returncode != 0:
        raise GuardError("the private baseline path is not ignored by git")

    cursor = REPO
    for part in expected.relative_to(REPO).parts:
        cursor = cursor / part
        if cursor.exists() and is_link(cursor):
            raise GuardError("evidenceDir contains a symlink or junction")
    if create:
        expected.mkdir(parents=True, exist_ok=True)
        try:
            os.chmod(expected, stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR)
        except OSError:
            pass
    elif create is False and not expected.is_dir():
        raise GuardError("the run evidence directory does not exist")
    return expected


def connect(path: Path, *, read_only: bool = False) -> sqlite3.Connection:
    if read_only:
        uri = f"file:{path.as_posix()}?mode=ro"
        connection = sqlite3.connect(uri, uri=True, timeout=10)
    else:
        connection = sqlite3.connect(path, timeout=10, isolation_level=None)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA foreign_keys = ON")
    connection.execute("PRAGMA busy_timeout = 10000")
    return connection


def require_schema(connection: sqlite3.Connection) -> None:
    required = set(COUNT_TABLES) | {
        "sources", "downloaders", "library_folders", "alerts",
    }
    present = {
        row[0]
        for row in connection.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
        )
    }
    missing = sorted(required - present)
    if missing:
        raise GuardError(f"preview database schema is missing: {', '.join(missing)}")


def is_loopback_host(value: str) -> bool:
    host = value.strip().lower().strip("[]")
    if host == "localhost":
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def preflight_environment(connection: sqlite3.Connection) -> dict[str, Any]:
    require_schema(connection)
    require_exact_path(DB, DB, "preview database")
    require_tree_without_links(PREVIEW_ROOT)
    require_tree_without_links(PREVIEW_MEDIA_ROOT)

    sources = connection.execute(
        "SELECT name, url, state FROM sources"
    ).fetchall()
    if len(sources) != 1 or dict(sources[0]) != {
        "name": "Lab fake indexer",
        "url": "http://127.0.0.1:28767",
        "state": "enabled",
    }:
        raise GuardError("preview Sources are not the single expected loopback fake")

    downloaders = connection.execute(
        "SELECT name, url, state, removed FROM downloaders"
    ).fetchall()
    if len(downloaders) != 1 or dict(downloaders[0]) != {
        "name": "Lab qBittorrent",
        "url": "http://127.0.0.1:28768",
        "state": "ok",
        "removed": 0,
    }:
        raise GuardError("preview Downloaders are not the single expected loopback fake")

    allowed_folders = {
        ("movie", normalized(PREVIEW_ROOT / "movies")),
        ("show", normalized(PREVIEW_MEDIA_ROOT / "shows")),
        ("music", normalized(PREVIEW_MEDIA_ROOT / "music")),
    }
    folders = connection.execute("SELECT kind, path FROM library_folders").fetchall()
    actual_folders = {(row["kind"], normalized(Path(row["path"]))) for row in folders}
    if actual_folders != allowed_folders or len(folders) != len(allowed_folders):
        raise GuardError("Library folders are not the three exact disposable preview folders")
    for _, folder in allowed_folders:
        path = Path(folder)
        if not path.is_dir() or is_link(path):
            raise GuardError("a disposable Library folder is missing or linked")

    setting = connection.execute(
        "SELECT value FROM settings WHERE grp='this_app' AND key='ask_search_on_approve'"
    ).fetchall()
    if len(setting) != 1 or setting[0]["value"].strip().lower() != "false":
        raise GuardError("search_on_approve must be false for the real-save pass")

    enabled_alerts = connection.execute(
        "SELECT kind, smtp_host, smtp_port FROM alerts WHERE enabled=1"
    ).fetchall()
    for alert in enabled_alerts:
        clearly_fake_email = (
            alert["kind"] == "email"
            and is_loopback_host(alert["smtp_host"])
            and 28760 <= int(alert["smtp_port"]) <= 28774
        )
        if not clearly_fake_email:
            raise GuardError("an enabled Alert is not provably bound to a loopback fake")

    owners = connection.execute(
        "SELECT id FROM users WHERE role='owner' AND state='active'"
    ).fetchall()
    if len(owners) != 1:
        raise GuardError("the disposable preview must have exactly one active owner")

    return {
        "ownerId": int(owners[0]["id"]),
        "source": "http://127.0.0.1:28767",
        "downloader": "http://127.0.0.1:28768",
        "libraryFolders": len(folders),
        "enabledFakeAlerts": len(enabled_alerts),
    }


def media_inventory() -> dict[str, list[dict[str, Any]]]:
    inventory: dict[str, list[dict[str, Any]]] = {}
    for label, root in (("preview", PREVIEW_ROOT), ("previewMedia", PREVIEW_MEDIA_ROOT)):
        require_tree_without_links(root)
        rows: list[dict[str, Any]] = []
        for current, dirs, files in os.walk(root, followlinks=False):
            dirs.sort()
            files.sort()
            current_path = Path(current)
            for name in files:
                path = current_path / name
                if normalized(path).startswith(normalized(PREVIEW_ROOT / "config") + os.sep):
                    continue
                info = path.stat(follow_symlinks=False)
                rows.append({
                    "path": path.relative_to(root).as_posix(),
                    "size": info.st_size,
                    "mtimeNs": info.st_mtime_ns,
                })
        inventory[label] = rows
    return inventory


def counts(connection: sqlite3.Connection) -> dict[str, int]:
    return {
        table: int(connection.execute(f'SELECT COUNT(*) FROM "{table}"').fetchone()[0])
        for table in COUNT_TABLES
    }


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def write_json_private(path: Path, value: dict[str, Any]) -> None:
    temporary = path.with_suffix(path.suffix + ".tmp")
    with temporary.open("x", encoding="utf-8", newline="\n") as handle:
        json.dump(value, handle, indent=2, sort_keys=True)
        handle.write("\n")
    try:
        os.chmod(temporary, stat.S_IRUSR | stat.S_IWUSR)
    except OSError:
        pass
    os.replace(temporary, path)


def create_backup(source: sqlite3.Connection, path: Path) -> None:
    temporary = path.with_suffix(".db.tmp")
    if path.exists() or temporary.exists():
        raise GuardError("a baseline already exists for this run")
    destination = sqlite3.connect(temporary)
    try:
        source.backup(destination)
    finally:
        destination.close()
    try:
        os.chmod(temporary, stat.S_IRUSR | stat.S_IWUSR)
    except OSError:
        pass
    os.replace(temporary, path)


def table_columns(connection: sqlite3.Connection, schema: str, table: str) -> list[str]:
    return [
        row[1]
        for row in connection.execute(f'PRAGMA {schema}.table_info("{table}")')
    ]


def quote_names(names: Iterable[str]) -> str:
    return ", ".join('"' + name.replace('"', '""') + '"' for name in names)


def baseline_missing_or_changed(
    connection: sqlite3.Connection,
    table: str,
    columns: Iterable[str] | None = None,
) -> bool:
    selected = list(columns or table_columns(connection, "baseline", table))
    if not selected:
        raise GuardError(f"cannot compare table without columns: {table}")
    names = quote_names(selected)
    row = connection.execute(
        f'SELECT {names} FROM baseline."{table}" '
        f'EXCEPT SELECT {names} FROM main."{table}" LIMIT 1'
    ).fetchone()
    return row is not None


def exact_table_changed(connection: sqlite3.Connection, table: str) -> bool:
    columns = table_columns(connection, "baseline", table)
    names = quote_names(columns)
    forward = connection.execute(
        f'SELECT {names} FROM baseline."{table}" '
        f'EXCEPT SELECT {names} FROM main."{table}" LIMIT 1'
    ).fetchone()
    reverse = connection.execute(
        f'SELECT {names} FROM main."{table}" '
        f'EXCEPT SELECT {names} FROM baseline."{table}" LIMIT 1'
    ).fetchone()
    return forward is not None or reverse is not None


def load_manifest(evidence: Path, run_id: str) -> tuple[dict[str, Any], Path]:
    manifest_path = evidence / "baseline.json"
    baseline_path = evidence / "baseline.db"
    if not manifest_path.is_file() or not baseline_path.is_file():
        raise GuardError("the private baseline is incomplete")
    if is_link(manifest_path) or is_link(baseline_path):
        raise GuardError("baseline evidence may not be linked")
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise GuardError("the baseline manifest is unreadable") from exc
    if manifest.get("runId") != run_id:
        raise GuardError("the baseline belongs to a different run")
    if manifest.get("database") != str(DB):
        raise GuardError("the baseline database path does not match the disposable preview")
    if manifest.get("baselineSha256") != sha256_file(baseline_path):
        raise GuardError("the private baseline database changed")
    return manifest, baseline_path


def validate_common(payload: dict[str, Any]) -> tuple[str, Path]:
    run_id = payload.get("runId")
    if not isinstance(run_id, str) or not RUN_RE.fullmatch(run_id):
        raise GuardError("runId must be a lowercase UUID")
    action = payload.get("action")
    evidence = ensure_evidence_path(
        run_id,
        payload.get("evidenceDir"),
        create=None if action == "init" else False,
    )
    require_exact_path(DB, DB, "preview database")
    return run_id, evidence


def parse_expected_requests(value: Any, *, allow_empty: bool = False) -> list[dict[str, Any]]:
    if not isinstance(value, list) or (not value and not allow_empty):
        qualifier = "a list" if allow_empty else "a non-empty list"
        raise GuardError(f"expectedRequests must be {qualifier}")
    parsed: list[dict[str, Any]] = []
    for index, item in enumerate(value):
        if not isinstance(item, dict):
            raise GuardError(f"expectedRequests[{index}] must be an object")
        required = (
            "id", "kind", "reference", "name", "requester", "state",
            "seasons", "albums", "titleMapped", "titleId",
        )
        if any(key not in item for key in required):
            raise GuardError(f"expectedRequests[{index}] is missing a required field")
        if not isinstance(item["id"], int) or item["id"] <= 0:
            raise GuardError(f"expectedRequests[{index}] has an invalid id")
        if item["kind"] not in {"movie", "show", "artist"}:
            raise GuardError(f"expectedRequests[{index}] has an invalid kind")
        if item["state"] not in {"asked", "approved", "turned_down", "available", "withdrawn"}:
            raise GuardError(f"expectedRequests[{index}] has an invalid state")
        if not isinstance(item["name"], str) or not item["name"]:
            raise GuardError(f"expectedRequests[{index}] needs a name")
        if not isinstance(item["reference"], str):
            raise GuardError(f"expectedRequests[{index}] reference must be a string")
        requester = item["requester"]
        if (
            not isinstance(requester, list)
            or not requester
            or any(not isinstance(user_id, int) or user_id <= 0 for user_id in requester)
            or len(set(requester)) != len(requester)
        ):
            raise GuardError(f"expectedRequests[{index}] requester must be unique user IDs")
        if not isinstance(item["seasons"], list) or not isinstance(item["albums"], list):
            raise GuardError(f"expectedRequests[{index}] seasons and albums must be lists")
        if not isinstance(item["titleMapped"], bool):
            raise GuardError(f"expectedRequests[{index}] titleMapped must be boolean")
        if item["titleId"] is not None and (
            not isinstance(item["titleId"], int) or item["titleId"] <= 0
        ):
            raise GuardError(f"expectedRequests[{index}] titleId is invalid")
        if item["titleMapped"] != (item["titleId"] is not None):
            raise GuardError(f"expectedRequests[{index}] title mapping fields disagree")
        parsed.append(item)
    return parsed


def json_value(value: str, label: str) -> Any:
    try:
        return json.loads(value)
    except json.JSONDecodeError as exc:
        raise GuardError(f"stored {label} is not valid JSON") from exc


def attach_baseline(connection: sqlite3.Connection, path: Path) -> None:
    connection.execute("ATTACH DATABASE ? AS baseline", (str(path),))
    require_schema(connection)


def assert_original_data(connection: sqlite3.Connection) -> None:
    for table in ORIGINAL_EXACT_TABLES:
        if baseline_missing_or_changed(connection, table):
            raise GuardError(f"an original {table} row was deleted or edited")
    for table in EXACT_TABLES:
        if exact_table_changed(connection, table):
            raise GuardError(f"{table} changed during the real-save pass")
    for table, columns in STABLE_COLUMNS.items():
        if baseline_missing_or_changed(connection, table, columns):
            raise GuardError(f"a stable original {table} row was deleted or edited")

    baseline_sessions = connection.execute(
        "SELECT hash FROM baseline.sessions"
    ).fetchall()
    for row in baseline_sessions:
        if not connection.execute(
            "SELECT 1 FROM main.sessions WHERE hash=?", (row["hash"],)
        ).fetchone():
            raise GuardError("an original session was removed")


def run_invites_and_users(
    connection: sqlite3.Connection,
    run_id: str,
    name_prefix: str,
    *,
    allow_disabled: bool,
    allow_unused: bool,
) -> tuple[list[sqlite3.Row], list[sqlite3.Row]]:
    invites = connection.execute(
        "SELECT hash, role, perms, note, used_by FROM invites WHERE note=? ORDER BY hash",
        (run_id,),
    ).fetchall()
    if len(invites) != RUN_INVITE_COUNT:
        raise GuardError("the run does not own exactly two invites")
    if any(row["role"] != INVITE_ROLE or row["perms"] != INVITE_PERMS for row in invites):
        raise GuardError("a run invite changed role or permissions")
    if not allow_unused and any(row["used_by"] is None for row in invites):
        raise GuardError("both run invites must be redeemed before verification")
    user_ids = [int(row["used_by"]) for row in invites if row["used_by"] is not None]
    if len(set(user_ids)) != len(user_ids):
        raise GuardError("the two run invites must belong to different users")

    users: list[sqlite3.Row] = []
    if user_ids:
        placeholders = ",".join("?" for _ in user_ids)
        users = connection.execute(
            f"SELECT id, name, role, perms, state FROM users WHERE id IN ({placeholders}) ORDER BY id",
            user_ids,
        ).fetchall()
    if len(users) != len(user_ids):
        raise GuardError("a run-owned member is missing")
    if not allow_unused and len(users) != RUN_INVITE_COUNT:
        raise GuardError("both run-owned members are required for verification")
    allowed_states = {"active", "disabled"} if allow_disabled else {"active"}
    for user in users:
        if (
            not user["name"].startswith(name_prefix)
            or user["role"] != INVITE_ROLE
            or user["perms"] != INVITE_PERMS
            or user["state"] not in allowed_states
        ):
            raise GuardError("a run-owned member no longer matches its guard")

    baseline_user_ids = {
        int(row[0]) for row in connection.execute("SELECT id FROM baseline.users")
    }
    new_user_ids = {
        int(row[0])
        for row in connection.execute(
            "SELECT id FROM main.users EXCEPT SELECT id FROM baseline.users"
        )
    }
    if new_user_ids != set(user_ids) or new_user_ids & baseline_user_ids:
        raise GuardError("unexpected users were created during the real-save pass")

    new_invites = connection.execute(
        "SELECT hash, note FROM main.invites EXCEPT SELECT hash, note FROM baseline.invites"
    ).fetchall()
    if len(new_invites) != RUN_INVITE_COUNT or any(row["note"] != run_id for row in new_invites):
        raise GuardError("unexpected invites were created during the real-save pass")
    return invites, users


def assert_expected_requests(
    connection: sqlite3.Connection,
    expected: list[dict[str, Any]],
    run_user_ids: set[int],
    *,
    exact: bool,
) -> dict[str, int]:
    actual_new_asks = {
        int(row[0])
        for row in connection.execute(
            "SELECT id FROM main.asks EXCEPT SELECT id FROM baseline.asks"
        )
    }
    actual_new_pairs = {
        (int(row[0]), int(row[1]))
        for row in connection.execute(
            """
            SELECT ask_id, user_id FROM main.ask_people
            EXCEPT SELECT ask_id, user_id FROM baseline.ask_people
            """
        )
    }

    # Cleanup after a failed assertion must not trust a possibly stale result
    # object. Derive the live partial state and require every new row to be
    # reachable only from this run's redeemed members.
    if not exact:
        if any(
            ask_id not in actual_new_asks or user_id not in run_user_ids
            for ask_id, user_id in actual_new_pairs
        ):
            raise GuardError("a new request-person link is not owned by a run member")
        for ask_id in actual_new_asks:
            ask = connection.execute(
                "SELECT asked_by FROM asks WHERE id=?", (ask_id,)
            ).fetchone()
            linked = {user_id for candidate, user_id in actual_new_pairs if candidate == ask_id}
            if (
                not ask
                or int(ask["asked_by"]) not in run_user_ids
                or not linked
                or not linked.issubset(run_user_ids)
            ):
                raise GuardError("a new request is not exclusively owned by run members")
        return {
            "requests": len(actual_new_asks),
            "requesterLinks": len(actual_new_pairs),
        }

    expected_pairs: set[tuple[int, int]] = set()
    expected_ask_ids: set[int] = set()

    for item in expected:
        requester_ids = {int(user_id) for user_id in item["requester"]}
        if not requester_ids.issubset(run_user_ids):
            raise GuardError(f"a requester is not run-owned for {item['name']}")
        if item["state"] == "withdrawn":
            if item["titleMapped"] or item["titleId"] is not None:
                raise GuardError(f"withdrawn request has a Library mapping: {item['name']}")
            if connection.execute(
                "SELECT 1 FROM baseline.asks WHERE id=? UNION ALL SELECT 1 FROM main.asks WHERE id=?",
                (item["id"], item["id"]),
            ).fetchone():
                raise GuardError(f"withdrawn request row still exists: {item['name']}")
            if connection.execute(
                "SELECT 1 FROM ask_people WHERE ask_id=?", (item["id"],)
            ).fetchone():
                raise GuardError(f"withdrawn requester links still exist: {item['name']}")
            source, separator, source_id = item["reference"].partition(":")
            source_key = "musicbrainz" if source == "mbid" else source
            if not separator or not source_id:
                raise GuardError(f"withdrawn request reference is invalid: {item['name']}")
            for row in connection.execute("SELECT source_ids FROM titles"):
                source_ids = json_value(row["source_ids"], "title source identifiers")
                if isinstance(source_ids, dict) and str(source_ids.get(source_key, "")) == source_id:
                    raise GuardError(f"withdrawn request created a Library title: {item['name']}")
            continue
        ask = connection.execute(
            """
            SELECT a.*
            FROM asks a
            WHERE a.id=?
              AND NOT EXISTS (SELECT 1 FROM baseline.asks b WHERE b.id=a.id)
            """,
            (item["id"],),
        ).fetchone()
        if not ask:
            raise GuardError(f"expected new request is missing: {item['name']}")
        ask_id = int(ask["id"])
        expected_ask_ids.add(ask_id)
        expected_pairs.update((ask_id, user_id) for user_id in requester_ids)

        if ask["kind"] != item["kind"] or ask["name"].casefold() != item["name"].casefold():
            raise GuardError(f"request identity does not match for {item['name']}")
        if ask["ref"] != item["reference"]:
            raise GuardError(f"request reference does not match for {item['name']}")
        if ask["state"] != item["state"]:
            raise GuardError(f"request state does not match for {item['name']}")
        if json_value(ask["seasons"], "request seasons") != item["seasons"]:
            raise GuardError(f"request seasons do not match for {item['name']}")
        # Storage retains album objects (and null for non-music requests);
        # the lifecycle receipt records the selected album identifiers.
        stored_albums = json_value(ask["albums"], "request albums")
        if stored_albums is None:
            stored_albums = []
        if not isinstance(stored_albums, list) or any(
            not isinstance(album, dict) or not isinstance(album.get("mbid"), str)
            for album in stored_albums
        ):
            raise GuardError(f"stored request albums are invalid for {item['name']}")
        if [album["mbid"] for album in stored_albums] != item["albums"]:
            raise GuardError(f"request albums do not match for {item['name']}")
        actual_requesters = {
            int(row[0])
            for row in connection.execute(
                "SELECT user_id FROM ask_people WHERE ask_id=?", (ask_id,)
            )
        }
        if actual_requesters != requester_ids:
            raise GuardError(f"requester links do not match for {item['name']}")
        if int(ask["asked_by"]) not in run_user_ids:
            raise GuardError(f"request owner is not run-owned for {item['name']}")

        title_id = ask["title_id"]
        if item["titleMapped"]:
            if title_id is None or int(title_id) != item["titleId"]:
                raise GuardError(f"approved request has no Library mapping: {item['name']}")
            title = connection.execute(
                "SELECT kind, name FROM titles WHERE id=?", (title_id,)
            ).fetchone()
            expected_title_name = item.get("titleName", item["name"])
            if not title or title["kind"] != item["kind"] or title["name"].casefold() != expected_title_name.casefold():
                raise GuardError(f"Library mapping does not match for {item['name']}")
        elif title_id is not None:
            raise GuardError(f"unmapped request unexpectedly has a Library title: {item['name']}")

    if actual_new_asks != expected_ask_ids:
        raise GuardError("the pass created an unexpected request")
    if actual_new_pairs != expected_pairs:
        raise GuardError("the pass created an unexpected request-person link")

    return {"requests": len(expected_ask_ids), "requesterLinks": len(expected_pairs)}


def verify_state(
    payload: dict[str, Any],
    run_id: str,
    evidence: Path,
    *,
    allow_disabled: bool,
    allow_partial: bool,
) -> tuple[sqlite3.Connection, dict[str, Any], list[sqlite3.Row]]:
    manifest, baseline_path = load_manifest(evidence, run_id)
    expected = parse_expected_requests(
        payload.get("expectedRequests", []), allow_empty=allow_partial
    )
    name_prefix = manifest.get("namePrefix")
    if not isinstance(name_prefix, str) or not NAME_PREFIX_RE.fullmatch(name_prefix):
        raise GuardError("the baseline name prefix is invalid")

    connection = connect(DB)
    try:
        connection.execute("BEGIN IMMEDIATE")
        guards = preflight_environment(connection)
        attach_baseline(connection, baseline_path)
        assert_original_data(connection)
        if media_inventory() != manifest.get("mediaInventory"):
            raise GuardError("disposable media files changed during the real-save pass")
        _, users = run_invites_and_users(
            connection,
            run_id,
            name_prefix,
            allow_disabled=allow_disabled,
            allow_unused=allow_partial,
        )
        request_summary = assert_expected_requests(
            connection,
            expected,
            {int(user["id"]) for user in users},
            exact=not allow_partial,
        )
        summary = {
            "guards": guards,
            "members": len(users),
            **request_summary,
        }
        return connection, summary, users
    except Exception:
        connection.rollback()
        connection.close()
        raise


def init(payload: dict[str, Any], run_id: str, evidence: Path) -> dict[str, Any]:
    tokens = payload.get("inviteTokens")
    name_prefix = payload.get("namePrefix")
    if (
        not isinstance(tokens, list)
        or len(tokens) != RUN_INVITE_COUNT
        or any(not isinstance(token, str) or len(token) < 24 for token in tokens)
        or len(set(tokens)) != RUN_INVITE_COUNT
    ):
        raise GuardError("inviteTokens must contain two different random strings of at least 24 characters")
    if not isinstance(name_prefix, str) or not NAME_PREFIX_RE.fullmatch(name_prefix):
        raise GuardError("namePrefix must be 3-40 safe lowercase characters")

    baseline_path = evidence / "baseline.db"
    manifest_path = evidence / "baseline.json"
    if baseline_path.exists() or manifest_path.exists():
        raise GuardError("this run already has baseline evidence")

    read = connect(DB, read_only=True)
    try:
        guards = preflight_environment(read)
        baseline_counts = counts(read)
        inventory = media_inventory()
        if read.execute("SELECT 1 FROM invites WHERE note=?", (run_id,)).fetchone():
            raise GuardError("this runId already owns an invite")
        invite_hashes = [hashlib.sha256(token.encode("utf-8")).hexdigest() for token in tokens]
        placeholders = ",".join("?" for _ in invite_hashes)
        if read.execute(
            f"SELECT 1 FROM invites WHERE hash IN ({placeholders})", invite_hashes
        ).fetchone():
            raise GuardError("an invite token collides with an existing invite")
        # Create the private evidence directory only after every live-data and
        # destination guard above has passed.
        ensure_evidence_path(run_id, str(evidence), create=True)
        create_backup(read, baseline_path)
    finally:
        read.close()

    manifest = {
        "version": 1,
        "runId": run_id,
        "database": str(DB),
        "createdAt": utc_now(),
        "namePrefix": name_prefix,
        "baselineCounts": baseline_counts,
        "mediaInventory": inventory,
        "baselineSha256": sha256_file(baseline_path),
    }
    write_json_private(manifest_path, manifest)

    write = connect(DB)
    committed = False
    try:
        write.execute("BEGIN IMMEDIATE")
        live_guards = preflight_environment(write)
        if live_guards != guards or counts(write) != baseline_counts:
            raise GuardError("preview data changed while the baseline was being prepared")
        if media_inventory() != inventory:
            raise GuardError("preview media changed while the baseline was being prepared")
        if write.execute("SELECT 1 FROM invites WHERE note=?", (run_id,)).fetchone():
            raise GuardError("this runId already owns an invite")
        placeholders = ",".join("?" for _ in invite_hashes)
        if write.execute(
            f"SELECT 1 FROM invites WHERE hash IN ({placeholders})", invite_hashes
        ).fetchone():
            raise GuardError("an invite token collides with an existing invite")

        now = utc_now()
        expires = store_time(datetime.now(timezone.utc) + timedelta(hours=1))
        for invite_hash in invite_hashes:
            write.execute(
                """
                INSERT INTO invites(hash, created_at, created_by, role, perms, note, expires_at)
                VALUES(?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    invite_hash, now, guards["ownerId"], INVITE_ROLE,
                    INVITE_PERMS, run_id, expires,
                ),
            )
        write.commit()
        committed = True
    except Exception:
        write.rollback()
        raise
    finally:
        write.close()
        if not committed:
            for path in (manifest_path, baseline_path):
                try:
                    path.unlink()
                except FileNotFoundError:
                    pass

    return {
        "action": "init",
        "runId": run_id,
        "database": str(DB),
        "evidenceDir": str(evidence),
        "baselineCounts": baseline_counts,
        "inviteCount": RUN_INVITE_COUNT,
        "guards": {
            "canonicalDatabase": True,
            "privateIgnoredBaseline": True,
            "fakeSource": True,
            "fakeDownloader": True,
            "disposableLibraryFolders": guards["libraryFolders"],
            "searchOnApprove": False,
            "externalAlerts": False,
        },
    }


def verify(payload: dict[str, Any], run_id: str, evidence: Path) -> dict[str, Any]:
    connection, summary, _ = verify_state(
        payload,
        run_id,
        evidence,
        allow_disabled=False,
        allow_partial=False,
    )
    try:
        connection.rollback()
    finally:
        connection.close()
    return {
        "action": "verify",
        "runId": run_id,
        "database": str(DB),
        "evidenceDir": str(evidence),
        "verified": True,
        "members": summary["members"],
        "requests": summary["requests"],
        "requesterLinks": summary["requesterLinks"],
        "originalRowsPreserved": True,
        "originalMediaPreserved": True,
    }


def resume(payload: dict[str, Any], run_id: str, evidence: Path) -> dict[str, Any]:
    session_tokens = payload.get("sessionTokens")
    csrf_tokens = payload.get("csrfTokens")
    for label, tokens in (
        ("sessionTokens", session_tokens),
        ("csrfTokens", csrf_tokens),
    ):
        if (
            not isinstance(tokens, list)
            or len(tokens) != RUN_INVITE_COUNT
            or any(not isinstance(token, str) or len(token) < 24 for token in tokens)
            or len(set(tokens)) != RUN_INVITE_COUNT
        ):
            raise GuardError(
                f"{label} must contain two different random strings of at least 24 characters"
            )

    session_hashes = [
        hashlib.sha256(token.encode("utf-8")).hexdigest()
        for token in session_tokens
    ]
    connection, summary, users = verify_state(
        payload,
        run_id,
        evidence,
        allow_disabled=True,
        allow_partial=True,
    )
    user_ids = [int(user["id"]) for user in users]
    placeholders = ",".join("?" for _ in user_ids)
    try:
        if len(users) != RUN_INVITE_COUNT:
            raise GuardError("both run-owned members are required to resume the fixture")
        baseline_user_ids = {
            int(row[0]) for row in connection.execute("SELECT id FROM baseline.users")
        }
        if set(user_ids) & baseline_user_ids:
            raise GuardError("refusing to reactivate an original user")

        hash_placeholders = ",".join("?" for _ in session_hashes)
        if connection.execute(
            f"SELECT 1 FROM sessions WHERE hash IN ({hash_placeholders})",
            session_hashes,
        ).fetchone():
            raise GuardError("a session token collides with an existing session")

        actor = json.dumps(
            {"kind": "user", "id": summary["guards"]["ownerId"]},
            separators=(",", ":"),
        )
        now_value = datetime.now(timezone.utc)
        now = store_time(now_value)
        expires = store_time(now_value + SESSION_ABSOLUTE_TIMEOUT)
        connection.execute(
            f"DELETE FROM sessions WHERE user_id IN ({placeholders})",
            user_ids,
        )
        connection.execute(
            f"UPDATE users SET state='active', updated_at=?, updated_by=? "
            f"WHERE id IN ({placeholders})",
            (now, actor, *user_ids),
        )
        for user, session_hash, csrf in zip(
            users, session_hashes, csrf_tokens, strict=True
        ):
            connection.execute(
                """
                INSERT INTO sessions(
                    hash, user_id, csrf, created_at, rotated_at, last_seen_at,
                    expires_at, user_agent, family, auth_method, local_auth_at
                ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'local_password', ?)
                """,
                (
                    session_hash,
                    int(user["id"]),
                    csrf,
                    now,
                    now,
                    now,
                    expires,
                    SESSION_USER_AGENT,
                    secrets.token_urlsafe(16),
                    now,
                ),
            )

        rows = connection.execute(
            f"""
            SELECT user_id, created_at, rotated_at, last_seen_at, expires_at,
                   user_agent, family, replaced, auth_method, local_auth_at
            FROM sessions
            WHERE user_id IN ({placeholders})
            ORDER BY user_id
            """,
            user_ids,
        ).fetchall()
        if (
            len(rows) != RUN_INVITE_COUNT
            or [int(row["user_id"]) for row in rows] != user_ids
            or any(
                row["created_at"] != now
                or row["rotated_at"] != now
                or row["last_seen_at"] != now
                or row["expires_at"] != expires
                or row["user_agent"] != SESSION_USER_AGENT
                or not row["family"]
                or int(row["replaced"]) != 0
                or row["auth_method"] != "local_password"
                or row["local_auth_at"] != now
                for row in rows
            )
        ):
            raise GuardError("fixture sessions do not match the backend session contract")
        states = connection.execute(
            f"SELECT id, state FROM users WHERE id IN ({placeholders}) ORDER BY id",
            user_ids,
        ).fetchall()
        if len(states) != RUN_INVITE_COUNT or any(row["state"] != "active" for row in states):
            raise GuardError("run-owned members were not reactivated")

        original_ask_ids = [
            int(row[0])
            for row in connection.execute("SELECT id FROM baseline.asks ORDER BY id")
        ]
        owned_ask_ids = [
            int(row[0])
            for row in connection.execute(
                """
                SELECT id FROM main.asks
                EXCEPT SELECT id FROM baseline.asks
                ORDER BY id
                """
            )
        ]
        connection.commit()
    except Exception:
        connection.rollback()
        raise
    finally:
        connection.close()

    return {
        "action": "resume",
        "runId": run_id,
        "database": str(DB),
        "evidenceDir": str(evidence),
        "fixtureBootstrap": True,
        "realAuthenticationProof": False,
        "activatedMembers": len(user_ids),
        "sessionsCreated": RUN_INVITE_COUNT,
        "users": [
            {"id": int(user["id"]), "name": user["name"]}
            for user in users
        ],
        "originalAskIDs": original_ask_ids,
        "ownedAskIDs": owned_ask_ids,
        "originalRowsPreserved": True,
        "originalMediaPreserved": True,
    }


def finalize(payload: dict[str, Any], run_id: str, evidence: Path) -> dict[str, Any]:
    allow_partial = payload.get("allowPartial", False)
    if not isinstance(allow_partial, bool):
        raise GuardError("allowPartial must be boolean")
    connection, summary, users = verify_state(
        payload,
        run_id,
        evidence,
        allow_disabled=True,
        allow_partial=allow_partial,
    )
    user_ids = [int(user["id"]) for user in users]
    placeholders = ",".join("?" for _ in user_ids)
    try:
        baseline_user_ids = {
            int(row[0]) for row in connection.execute("SELECT id FROM baseline.users")
        }
        if set(user_ids) & baseline_user_ids:
            raise GuardError("refusing to disable an original user")
        actor = json.dumps(
            {"kind": "user", "id": summary["guards"]["ownerId"]},
            separators=(",", ":"),
        )
        now = utc_now()
        deleted_sessions = 0
        if user_ids:
            connection.execute(
                f"UPDATE users SET state='disabled', updated_at=?, updated_by=? "
                f"WHERE id IN ({placeholders})",
                (now, actor, *user_ids),
            )
            deleted_sessions = connection.execute(
                f"DELETE FROM sessions WHERE user_id IN ({placeholders})", user_ids
            ).rowcount
            if connection.execute(
                f"SELECT 1 FROM sessions WHERE user_id IN ({placeholders})", user_ids
            ).fetchone():
                raise GuardError("a run-owned member session could not be revoked")
            states = connection.execute(
                f"SELECT state FROM users WHERE id IN ({placeholders})", user_ids
            ).fetchall()
            if len(states) != len(user_ids) or any(row["state"] != "disabled" for row in states):
                raise GuardError("run-owned members were not disabled")
        connection.commit()
    except Exception:
        connection.rollback()
        raise
    finally:
        connection.close()

    return {
        "action": "finalize",
        "runId": run_id,
        "database": str(DB),
        "evidenceDir": str(evidence),
        "verified": True,
        "disabledMembers": len(user_ids),
        "revokedSessions": max(0, int(deleted_sessions)),
        "requestsPreserved": summary["requests"],
        "originalRowsPreserved": True,
        "originalMediaPreserved": True,
    }


def main() -> int:
    try:
        raw = sys.stdin.read()
        payload = json.loads(raw)
        if not isinstance(payload, dict):
            raise GuardError("stdin must contain one JSON object")
        action = payload.get("action")
        if action not in {"init", "verify", "resume", "finalize"}:
            raise GuardError("action must be init, verify, resume, or finalize")
        run_id, evidence = validate_common(payload)
        if action == "init":
            result = init(payload, run_id, evidence)
        elif action == "verify":
            result = verify(payload, run_id, evidence)
        elif action == "resume":
            result = resume(payload, run_id, evidence)
        else:
            result = finalize(payload, run_id, evidence)
        print(json.dumps({"ok": True, **result}, separators=(",", ":")))
        return 0
    except (GuardError, json.JSONDecodeError, sqlite3.Error, OSError, TypeError, ValueError) as exc:
        print(
            json.dumps(
                {"ok": False, "error": str(exc)},
                separators=(",", ":"),
            )
        )
        return 1
    except Exception:
        # Preserve the JSON-only protocol without reflecting arbitrary values
        # from an unexpected exception into runner logs.
        print('{"ok":false,"error":"unexpected helper failure"}')
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
