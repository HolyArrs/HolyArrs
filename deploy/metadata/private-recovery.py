"""Bounded private metadata backup verification and recovery helpers.

The Oracle entry point creates encrypted recovery points only while the two
private metadata containers are stopped.  The reusable helpers are deliberately
platform neutral so the same archive and database checks run on Windows.
"""
import argparse
import contextlib
import datetime
import gzip
import hashlib
import json
import os
import pathlib
import queue
import re
import shutil
import sqlite3
import stat
import subprocess
import tarfile
import tempfile
import threading
import time
import uuid


ROOT = pathlib.Path("/home/home/holyarrs-metadata-server")
OWNER = "holyarrs-metadata-private"
EXISTING = ["holycore", "sacris", "cloudflared", "watchtower", "portainer_agent",
            "buildx_buildkit_cw-pretag150-arm-node"]
MAINTENANCE = "holyarrs-metadata-private-recovery"
API_IMAGE = "sha256:3f4e2fbe701e70ec502a6ab2958629c47d63b3cc82cbe510d144d31795e82af5"
FIXTURE_IMAGE = "sha256:1cbba147d02d9bff2ccbfcf0c63155e8efca0eae57361d91f20f02aced150def"
CRYPTO_IMAGE = "sha256:007a316059246276cb214df93950be77837545dc78007a73e0291a4963d5d3bf"
OPERATOR_REVISION = "c9ddc1deabd41c33e5cb58729a4255bc436ddaf7"
RUN_RE = re.compile(r"run-\d{8}T\d{6}Z-[0-9a-f]{8}\Z")
HASH_RE = re.compile(r"[0-9a-f]{64}\Z")
MIN_FREE_BYTES = 25 * 1024**3
CIPHERTEXT_CAP = 512 * 1024**2
EXPANSION_CAP = 3 * 1024**3
DATABASE_MEMBER_CAP = 2 * 1024**3

CACHE_COLUMNS = [
    ("key", "TEXT", 1, 1), ("data", "BLOB", 1, 0),
    ("cursor", "TEXT", 1, 0), ("fetched_at", "TEXT", 1, 0),
    ("expires_at", "TEXT", 1, 0), ("age", "INTEGER", 1, 0),
    ("size", "INTEGER", 1, 0),
]
ACCESS_COLUMNS = [("hash", "BLOB", 1, 1), ("revoked", "INTEGER", 1, 0)]
IMMUTABLE_PACKAGE_MEMBERS = {
    "config/private.json", "source/runtime.tar.gz", "source/recovery-tools.tar.gz",
    "operator/private-ops.py", "operator/current-build.json",
}


class RecoveryError(RuntimeError):
    pass


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0)


def iso(value=None):
    return (value or utc_now()).isoformat().replace("+00:00", "Z")


def sha256_file(path, cap=None):
    total = 0
    value = hashlib.sha256()
    with pathlib.Path(path).open("rb") as source:
        while True:
            block = source.read(1024 * 1024)
            if not block:
                break
            total += len(block)
            if cap is not None and total > cap:
                raise RecoveryError("file exceeds size limit")
            value.update(block)
    return total, value.hexdigest()


def sanitize_error(_error):
    return "operation failed; inspect the private local diagnostic staging"


def run_command(args, *, runner=subprocess.run, timeout=420, check=True):
    try:
        result = runner(args, text=True, stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE, timeout=timeout)
    except (OSError, subprocess.SubprocessError) as error:
        raise RecoveryError(sanitize_error(error)) from None
    if check and result.returncode:
        raise RecoveryError("operation failed: " + pathlib.Path(str(args[0])).name)
    return result


def _owned_docker_launch(args):
    if not args or pathlib.Path(str(args[0])).name.lower() not in {"docker", "docker.exe"}:
        return None
    try:
        name = str(args[args.index("--name") + 1])
    except (ValueError, IndexError):
        return None
    labels = {}
    for index, value in enumerate(args[:-1]):
        if value == "--label" and "=" in str(args[index + 1]):
            key, label_value = str(args[index + 1]).split("=", 1)
            labels[key] = label_value
    if (not name.startswith("holyarrs-metadata-offhost-") or CRYPTO_IMAGE not in args
            or labels.get("holyforge.owner") != OWNER):
        return None
    return name, labels.get("holyforge.schedule")


def _cleanup_owned_docker(args, runner):
    launch = _owned_docker_launch(args)
    if launch is None:
        return
    name, schedule = launch
    try:
        inspected = runner(["docker", "container", "inspect", name], text=True,
                           stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                           timeout=30, check=False)
        if inspected.returncode:
            return
        value = json.loads(inspected.stdout)
        if not isinstance(value, list) or len(value) != 1 or not isinstance(value[0], dict):
            return
        item = value[0]
        labels = item.get("Config", {}).get("Labels") or {}
        identifier = item.get("Id")
        if (not isinstance(identifier, str) or HASH_RE.fullmatch(identifier) is None
                or item.get("Name") != "/" + name or item.get("Image") != CRYPTO_IMAGE
                or labels.get("holyforge.owner") != OWNER
                or labels.get("holyforge.schedule") != schedule):
            return
        runner(["docker", "rm", "-f", identifier], stdout=subprocess.DEVNULL,
               stderr=subprocess.DEVNULL, timeout=30, check=False)
    except (AttributeError, json.JSONDecodeError, OSError, subprocess.SubprocessError, TypeError):
        return


def run_binary_to_capped_file(args, target, cap, *, popen=subprocess.Popen,
                              cleanup_runner=subprocess.run, timeout=600):
    target = pathlib.Path(target)
    if target.exists() or target.is_symlink():
        raise RecoveryError("plaintext target already exists")
    process = None
    complete = False
    deadline = time.monotonic() + timeout
    reader = None
    messages = queue.Queue(maxsize=2)
    stop_reader = threading.Event()
    aborted = False
    try:
        process = popen(args, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)

        def drain():
            def deliver(value):
                while not stop_reader.is_set():
                    try:
                        messages.put(value, timeout=.1)
                        return True
                    except queue.Full:
                        pass
                return False
            try:
                while True:
                    block = process.stdout.read(1024 * 1024)
                    if not deliver(("data", block)) or not block:
                        return
            except Exception:
                deliver(("error", None))

        reader = threading.Thread(target=drain, daemon=True)
        reader.start()
        total = 0
        with target.open("xb") as output:
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise subprocess.TimeoutExpired(args, timeout)
                try:
                    kind, block = messages.get(timeout=remaining)
                except queue.Empty:
                    raise subprocess.TimeoutExpired(args, timeout) from None
                if kind == "error":
                    raise RecoveryError("operation failed: binary output")
                if not block:
                    break
                total += len(block)
                if total > cap:
                    process.kill()
                    raise RecoveryError("plaintext output exceeds limit")
                output.write(block)
        remaining = deadline - time.monotonic()
        if remaining <= 0 or process.wait(timeout=remaining):
            raise RecoveryError("operation failed: " + pathlib.Path(str(args[0])).name)
        complete = True
        return total
    except (OSError, subprocess.SubprocessError, RecoveryError) as error:
        aborted = True
        stop_reader.set()
        if process is not None:
            with contextlib.suppress(OSError):
                process.kill()
            with contextlib.suppress(OSError, subprocess.SubprocessError):
                process.wait(timeout=10)
        _cleanup_owned_docker(args, cleanup_runner)
        if isinstance(error, RecoveryError) and "exceeds" in str(error):
            raise
        raise RecoveryError(sanitize_error(error)) from None
    finally:
        stop_reader.set()
        if reader is not None and aborted:
            reader.join(timeout=10)
        if target.exists() and not complete:
            target.unlink()


def atomic_json(path, value):
    path = pathlib.Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".tmp-" + uuid.uuid4().hex)
    try:
        with temporary.open("x", encoding="utf-8", newline="\n") as output:
            json.dump(value, output, indent=2, sort_keys=True)
            output.write("\n")
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def write_failure(path, stage, error):
    atomic_json(path, {"schema_version": 1, "status": "failed", "stage": stage,
                       "error": sanitize_error(error), "failed_at": iso()})


def parse_time(value):
    if not isinstance(value, str) or not value.endswith("Z"):
        raise RecoveryError("invalid timestamp")
    try:
        return datetime.datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError:
        raise RecoveryError("invalid timestamp") from None


def read_status(latest_path, failure_path=None, *, now=None):
    latest_path = pathlib.Path(latest_path)
    result = {"schema_version": 1, "status": "never", "stale": True}
    current = parse_time(now) if isinstance(now, str) else (now or utc_now())
    if latest_path.is_file():
        success = json.loads(latest_path.read_text(encoding="utf-8"))
        timestamp = success.get("verified_at") or success.get("success_at")
        if timestamp:
            age = max(0, int((current - parse_time(timestamp)).total_seconds()))
            result.update({"status": "verified", "last_success": timestamp,
                           "age_seconds": age, "stale": age > 86400})
    if failure_path and pathlib.Path(failure_path).is_file():
        failed = json.loads(pathlib.Path(failure_path).read_text(encoding="utf-8"))
        result["last_failure"] = {key: failed[key] for key in ("stage", "error", "failed_at") if key in failed}
    return result


def validate_run_name(name):
    if not isinstance(name, str) or RUN_RE.fullmatch(name) is None:
        raise RecoveryError("invalid recovery run name")
    return name


def expected_identity_manifest():
    return {"schema_version": 1, "runtime_build": "2026-10-15-build-7",
            "operator_revision": OPERATOR_REVISION, "api_image": API_IMAGE,
            "fixture_image": FIXTURE_IMAGE}


def validate_identities(expected, actual):
    keys = ("schema_version", "runtime_build", "operator_revision", "api_image", "fixture_image")
    if (not isinstance(expected, dict) or not isinstance(actual, dict)
            or set(actual) != set(keys)
            or any(actual.get(key) != expected.get(key) for key in keys)):
        raise RecoveryError("source or image identity mismatch")


def validate_private_state(state, identities):
    if state.get("private_test_active") or state.get("maintenance_exists"):
        raise RecoveryError("private validation or maintenance is active")
    for name, image_key in (("api", "api_image"), ("fixture", "fixture_image")):
        item = state.get(name, {})
        if (not item.get("exists") or item.get("owner") != OWNER or item.get("running")
                or item.get("ports") or item.get("image") != identities[image_key]):
            raise RecoveryError("private container state mismatch")


def validate_existing_services(expected, actual):
    fields = ("name", "id", "started", "restarts", "running", "health")
    if (not isinstance(expected, list) or len(expected) != len(EXISTING)
            or not isinstance(actual, list) or len(actual) != len(EXISTING)):
        raise RecoveryError("existing service baseline is incomplete")
    for name, before, now in zip(EXISTING, expected, actual):
        if before.get("name") != name or now.get("name") != name:
            raise RecoveryError("existing service baseline order mismatch")
        if any(before.get(field) != now.get(field) for field in fields):
            raise RecoveryError("existing service state changed")
        if not now.get("running") or (before.get("health") == "healthy" and now.get("health") != "healthy"):
            raise RecoveryError("existing service is no longer healthy")


def validate_current_descriptor(value):
    expected = {"runtime_folder": "2026-10-15-build-7",
                "operator_folder": "2026-10-15-build-8",
                "source_commit": OPERATOR_REVISION,
                "public_deployment": False}
    if value != expected:
        raise RecoveryError("current runtime or operator identity mismatch")


def estimate_staging_peak(database_bytes, config_and_sources_bytes, image_export_bytes):
    # Snapshot + plaintext inputs + compressed package + ciphertext, with bounded
    # tar/export overhead. Compression is deliberately not assumed.
    return database_bytes * 3 + config_and_sources_bytes * 2 + image_export_bytes * 2 + 64 * 1024**2


def require_disk_space(available, peak):
    if available - peak < MIN_FREE_BYTES:
        raise RecoveryError("disk reserve below 25 GiB after staging estimate")


def validate_export_directory(path, uid, gid, *, info=None):
    path = pathlib.Path(path)
    if info is None:
        try:
            info = path.lstat()
        except OSError:
            raise RecoveryError("scheduled export directory is unavailable") from None
    if (path.is_symlink() or not stat.S_ISDIR(info.st_mode) or info.st_uid != uid
            or info.st_gid != gid or stat.S_IMODE(info.st_mode) != 0o700):
        raise RecoveryError("scheduled export directory ownership or mode mismatch")
    return path


def ensure_export_directory(path, uid, gid):
    path = pathlib.Path(path)
    try:
        path.mkdir(mode=0o700)
        os.chown(path, uid, gid)
        os.chmod(path, 0o700)
    except FileExistsError:
        pass
    return validate_export_directory(path, uid, gid)


class ServiceLock:
    """Cross-platform nonblocking lock; the lock file itself is never stale-deleted."""
    def __init__(self, path):
        self.path = pathlib.Path(path)
        self.handle = None

    def acquire(self):
        self.handle = self.path.open("a+b")
        try:
            if os.name == "nt":
                import msvcrt
                self.handle.seek(0)
                if self.handle.tell() == 0:
                    self.handle.write(b"0")
                    self.handle.flush()
                self.handle.seek(0)
                msvcrt.locking(self.handle.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(self.handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except (OSError, BlockingIOError):
            self.handle.close()
            self.handle = None
            raise RecoveryError("backup already running") from None
        return self

    def release(self):
        if self.handle is None:
            return
        try:
            if os.name == "nt":
                import msvcrt
                self.handle.seek(0)
                msvcrt.locking(self.handle.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                import fcntl
                fcntl.flock(self.handle.fileno(), fcntl.LOCK_UN)
        finally:
            self.handle.close()
            self.handle = None

    def __enter__(self):
        return self.acquire()

    def __exit__(self, *_args):
        self.release()


def _columns(db, table):
    return [(row[1], row[2].upper(), row[3], row[4], row[5], row[6])
            for row in db.execute(f"PRAGMA table_xinfo({table})")]


def _schema_sql(db, name):
    row = db.execute("SELECT sql FROM sqlite_schema WHERE name=?", (name,)).fetchone()
    return re.sub(r"\s+", "", row[0]).lower() if row and isinstance(row[0], str) else None


def _validate_database(path, kind, revoke=False):
    path = pathlib.Path(path)
    if not path.is_file() or path.is_symlink():
        raise RecoveryError("database is not an owned regular file")
    try:
        with contextlib.closing(sqlite3.connect(f"file:{path}?mode={'rw' if revoke else 'ro'}", uri=True)) as db:
            if db.execute("PRAGMA user_version").fetchone()[0] != 1:
                raise RecoveryError("unsupported database schema")
            if db.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
                raise RecoveryError("database integrity check failed")
            objects = db.execute(
                "SELECT type,name FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name"
            ).fetchall()
            if kind == "cache":
                if objects != [("index", "entries_age"), ("table", "entries")]:
                    raise RecoveryError("cache schema objects do not match version 1")
                expected_columns = [(name, kind, required, None, primary, 0)
                                    for name, kind, required, primary in CACHE_COLUMNS]
                if _columns(db, "entries") != expected_columns:
                    raise RecoveryError("cache columns do not match version 1")
                if _schema_sql(db, "entries") != re.sub(r"\s+", "", """CREATE TABLE entries (
                        key TEXT PRIMARY KEY, data BLOB NOT NULL, cursor TEXT NOT NULL,
                        fetched_at TEXT NOT NULL, expires_at TEXT NOT NULL, age INTEGER NOT NULL,
                        size INTEGER NOT NULL CHECK(size>0)) WITHOUT ROWID""").lower():
                    raise RecoveryError("cache table constraints do not match version 1")
                indexes = db.execute("PRAGMA index_list(entries)").fetchall()
                named = [row for row in indexes if row[1] == "entries_age"]
                details = db.execute("PRAGMA index_xinfo(entries_age)").fetchall()
                if (len(named) != 1 or named[0][2] != 0 or named[0][3] != "c" or named[0][4] != 0
                        or [(row[2], row[3], row[4], row[5]) for row in details]
                        != [("age", 0, "BINARY", 1), ("key", 0, "BINARY", 1)]
                        or _schema_sql(db, "entries_age") != "createindexentries_ageonentries(age,key)"):
                    raise RecoveryError("cache index does not match version 1")
            else:
                expected_columns = [(name, kind, required, None, primary, 0)
                                    for name, kind, required, primary in ACCESS_COLUMNS]
                if objects != [("table", "tokens")] or _columns(db, "tokens") != expected_columns:
                    raise RecoveryError("access schema does not match version 1")
                if _schema_sql(db, "tokens") != re.sub(r"\s+", "", """CREATE TABLE tokens (
                        hash BLOB PRIMARY KEY CHECK(length(hash)=32),
                        revoked INTEGER NOT NULL CHECK(revoked IN (0,1))) WITHOUT ROWID""").lower():
                    raise RecoveryError("access table constraints do not match version 1")
                invalid = db.execute(
                    "SELECT COUNT(*) FROM tokens WHERE typeof(hash)!='blob' OR length(hash)!=32 "
                    "OR revoked NOT IN (0,1)"
                ).fetchone()[0]
                if invalid:
                    raise RecoveryError("access rows do not match version 1")
                if revoke:
                    with db:
                        db.execute("UPDATE tokens SET revoked=1")
    except sqlite3.Error as error:
        raise RecoveryError("database validation failed") from None


def validate_database_pair(directory):
    directory = pathlib.Path(directory)
    _validate_database(directory / "cache.sqlite", "cache")
    _validate_database(directory / "access.sqlite", "access")


def prepare_restore(source, destination):
    source, destination = pathlib.Path(source), pathlib.Path(destination)
    validate_database_pair(source)
    if destination.exists() or destination.is_symlink():
        raise RecoveryError("restore destination must not exist")
    destination.mkdir(mode=0o700)
    try:
        for name in ("cache.sqlite", "access.sqlite"):
            shutil.copyfile(source / name, destination / name)
            os.chmod(destination / name, 0o600)
        _validate_database(destination / "access.sqlite", "access", revoke=True)
        validate_database_pair(destination)
    except Exception:
        for name in ("cache.sqlite", "access.sqlite"):
            candidate = destination / name
            if candidate.is_file() and not candidate.is_symlink():
                candidate.unlink()
        destination.rmdir()
        raise
    return destination


def _safe_member_name(name):
    if not isinstance(name, str) or not name or "\\" in name or name.startswith("/"):
        return False
    pure = pathlib.PurePosixPath(name)
    return not pure.is_absolute() and all(part not in ("", ".", "..") for part in pure.parts)


def bounded_gzip_tar_size(package, expansion_cap=EXPANSION_CAP,
                          *, header_cap=128, extension_cap=1024 * 1024):
    total = 0

    def read_exact(source, count, *, retain=False):
        nonlocal total
        chunks = [] if retain else None
        remaining = count
        while remaining:
            block = source.read(min(1024 * 1024, remaining, expansion_cap - total + 1))
            if not block:
                raise RecoveryError("archive decompression failed")
            total += len(block)
            if total > expansion_cap:
                raise RecoveryError("archive decompression limit exceeded")
            if retain:
                chunks.append(block)
            remaining -= len(block)
        return b"".join(chunks) if retain else None

    try:
        with gzip.open(package, "rb") as source:
            headers = 0
            while True:
                header = read_exact(source, 512, retain=True)
                if header == b"\0" * 512:
                    second = read_exact(source, 512, retain=True)
                    if second != b"\0" * 512:
                        raise RecoveryError("archive end marker is invalid")
                    while True:
                        block = source.read(min(1024 * 1024, expansion_cap - total + 1))
                        if not block:
                            break
                        total += len(block)
                        if total > expansion_cap:
                            raise RecoveryError("archive decompression limit exceeded")
                        if block.strip(b"\0"):
                            raise RecoveryError("archive trailing data is invalid")
                    break
                headers += 1
                if headers > header_cap:
                    raise RecoveryError("archive header limit exceeded")
                raw_size = header[124:136].rstrip(b"\0 ") or b"0"
                if raw_size[:1] and raw_size[0] & 0x80:
                    raise RecoveryError("archive header size encoding is unsupported")
                try:
                    size = int(raw_size, 8)
                except ValueError:
                    raise RecoveryError("archive header size is invalid") from None
                if size < 0:
                    raise RecoveryError("archive header size is invalid")
                kind = header[156:157]
                if kind in {b"x", b"g"} and size > extension_cap:
                    raise RecoveryError("archive extension header exceeds limit")
                if kind in {b"L", b"K"}:
                    raise RecoveryError("archive extension header is unsafe")
                padded = ((size + 511) // 512) * 512
                if padded > expansion_cap - total:
                    raise RecoveryError("archive decompression limit exceeded")
                read_exact(source, padded,
                           retain=kind in {b"x", b"g"})
    except (OSError, EOFError, gzip.BadGzipFile):
        raise RecoveryError("archive decompression failed") from None
    return total


def inspect_archive(package, expected_members, *, member_cap=DATABASE_MEMBER_CAP,
                    expansion_cap=EXPANSION_CAP):
    package = pathlib.Path(package)
    seen, total = set(), 0
    bounded_gzip_tar_size(package, expansion_cap)
    try:
        with tarfile.open(package, "r:gz") as archive:
            for member in archive:
                if not member.isfile() or not _safe_member_name(member.name):
                    raise RecoveryError("archive contains unsafe member")
                if member.name in seen:
                    raise RecoveryError("archive contains duplicate member")
                if member.name not in expected_members:
                    raise RecoveryError("archive contains unexpected member")
                expected = expected_members[member.name]
                if member.size < 0 or member.size > member_cap or member.size != expected["bytes"]:
                    raise RecoveryError("archive member size mismatch")
                total += member.size
                if total > expansion_cap:
                    raise RecoveryError("archive expansion limit exceeded")
                seen.add(member.name)
            if seen != set(expected_members):
                raise RecoveryError("archive members are incomplete")
    except (tarfile.TarError, OSError) as error:
        if isinstance(error, RecoveryError):
            raise
        raise RecoveryError("archive inspection failed") from None
    return total


def verify_and_publish_archive(package, destination, expected_members,
                               *, expansion_cap=EXPANSION_CAP):
    inspect_archive(package, expected_members, expansion_cap=expansion_cap)
    destination = pathlib.Path(destination)
    if destination.exists() or destination.is_symlink():
        raise RecoveryError("verification destination already exists")
    temporary = destination.with_name(destination.name + ".staging-" + uuid.uuid4().hex)
    temporary.mkdir(mode=0o700)
    created = []
    try:
        with tarfile.open(package, "r:gz") as archive:
            for member in archive:
                target = temporary.joinpath(*pathlib.PurePosixPath(member.name).parts)
                target.parent.mkdir(parents=True, exist_ok=True)
                value, count = hashlib.sha256(), 0
                source = archive.extractfile(member)
                if source is None:
                    raise RecoveryError("archive member cannot be read")
                with source, target.open("xb") as output:
                    while True:
                        block = source.read(1024 * 1024)
                        if not block:
                            break
                        count += len(block)
                        if count > expected_members[member.name]["bytes"]:
                            raise RecoveryError("archive member exceeds declared size")
                        value.update(block)
                        output.write(block)
                created.append(target)
                if count != expected_members[member.name]["bytes"] or value.hexdigest() != expected_members[member.name]["sha256"]:
                    raise RecoveryError("archive member hash mismatch")
        os.replace(temporary, destination)
    except Exception:
        for target in reversed(created):
            if target.is_file() and not target.is_symlink():
                target.unlink()
        for folder in sorted((p for p in temporary.rglob("*") if p.is_dir()), key=lambda p: len(p.parts), reverse=True):
            folder.rmdir()
        if temporary.exists():
            temporary.rmdir()
        raise
    return destination


def cleanup_owned_staging(stage, owned_files):
    """Delete an exact, prevalidated set of staging files and empty parents."""
    stage = pathlib.Path(stage)
    if stage.is_symlink() or not stage.is_dir():
        raise RecoveryError("owned staging directory mismatch")
    expected = set()
    for value in owned_files:
        path = pathlib.Path(value)
        try:
            path.relative_to(stage)
        except ValueError:
            raise RecoveryError("owned staging file escapes staging root") from None
        if path.is_symlink() or not path.is_file():
            raise RecoveryError("owned staging file mismatch")
        expected.add(path)
    actual = {path for path in stage.rglob("*") if path.is_file() or path.is_symlink()}
    if actual != expected:
        raise RecoveryError("owned staging contains unrecognized files")
    for path in sorted(expected, key=lambda item: len(item.parts), reverse=True):
        path.unlink()
    for folder in sorted((p for p in stage.rglob("*") if p.is_dir()),
                         key=lambda item: len(item.parts), reverse=True):
        folder.rmdir()
    stage.rmdir()


def _validate_root_owned_manifest_path(root, path, *, leaf_regular=True):
    root, path = pathlib.Path(root), pathlib.Path(path)
    try:
        relative = path.relative_to(root)
    except ValueError:
        raise RecoveryError("recovery manifest path escapes root") from None
    current = root
    for index, part in enumerate(relative.parts):
        current /= part
        try:
            info = current.lstat()
        except OSError:
            raise RecoveryError("recovery manifest path is unavailable") from None
        final = index == len(relative.parts) - 1
        expected_type = stat.S_ISREG(info.st_mode) if final and leaf_regular else stat.S_ISDIR(info.st_mode)
        if (current.is_symlink() or not expected_type or info.st_uid != 0
                or stat.S_IMODE(info.st_mode) & 0o022):
            raise RecoveryError("recovery manifest path ownership or mode mismatch")
    return path


def validate_recovery_manifest(root, value):
    required = {"schema_version", "identities", "existing_services", "files"}
    if not isinstance(value, dict) or set(value) != required or value["schema_version"] != 1:
        raise RecoveryError("invalid recovery manifest")
    validate_identities(expected_identity_manifest(), value["identities"])
    if not isinstance(value["files"], dict) or set(value["files"]) != IMMUTABLE_PACKAGE_MEMBERS:
        raise RecoveryError("invalid recovery manifest files")
    validate_existing_services(value["existing_services"], value["existing_services"])
    resolved = {}
    for member, item in value["files"].items():
        if not _safe_member_name(member) or not isinstance(item, dict) or set(item) != {"path", "bytes", "sha256"}:
            raise RecoveryError("invalid recovery manifest member")
        relative = item["path"]
        if not _safe_member_name(relative):
            raise RecoveryError("recovery manifest source path is unsafe")
        path = pathlib.Path(root) / pathlib.PurePosixPath(relative)
        try:
            resolved_path = path.resolve(strict=True)
            resolved_path.relative_to(pathlib.Path(root))
        except (OSError, ValueError):
            raise RecoveryError("recovery manifest source escapes root") from None
        current = pathlib.Path(root)
        for part in pathlib.PurePosixPath(relative).parts:
            current /= part
            if current.is_symlink():
                raise RecoveryError("recovery manifest source has symlink ancestor")
        _validate_root_owned_manifest_path(root, path)
        if item["bytes"] < 0 or not HASH_RE.fullmatch(item["sha256"]):
            raise RecoveryError("recovery manifest source is unsafe")
        size, value_hash = sha256_file(path)
        if size != item["bytes"] or value_hash != item["sha256"]:
            raise RecoveryError("recovery manifest source hash mismatch")
        resolved[member] = path
    try:
        descriptor = json.loads(resolved["operator/current-build.json"].read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError):
        raise RecoveryError("current build descriptor is invalid") from None
    validate_current_descriptor(descriptor)
    return resolved


def image_export_identity(path, expected_id, expected_architecture="arm64"):
    """Verify a Docker save archive config object without loading or executing it."""
    path = pathlib.Path(path)
    if (not isinstance(expected_id, str) or not expected_id.startswith("sha256:")
            or not HASH_RE.fullmatch(expected_id[7:])):
        raise RecoveryError("image export identity invalid")
    if path.stat().st_size > EXPANSION_CAP:
        raise RecoveryError("image export exceeds limit")
    with tarfile.open(path, "r:") as archive:
        members = {}
        total = 0
        for member in archive:
            if member.name in members or not _safe_member_name(member.name):
                raise RecoveryError("image export contains duplicate or unsafe member")
            if not (member.isfile() or member.isdir()):
                raise RecoveryError("image export contains link or special member")
            members[member.name] = member
            if member.isfile():
                if member.size < 0:
                    raise RecoveryError("image export contains invalid member")
                total += member.size
                if total > EXPANSION_CAP:
                    raise RecoveryError("image export expansion exceeds limit")

        def read_json_member(name, limit, label):
            member = members.get(name)
            if (member is None or not member.isfile() or member.size > limit):
                raise RecoveryError("image export " + label + " missing")
            source = archive.extractfile(member)
            raw = source.read(limit + 1) if source is not None else b""
            if len(raw) != member.size or len(raw) > limit:
                raise RecoveryError("image export " + label + " exceeds limit")
            try:
                return raw, json.loads(raw)
            except (UnicodeDecodeError, json.JSONDecodeError):
                raise RecoveryError("image export " + label + " invalid") from None

        def descriptor_blob(descriptor, media_types, label, limit):
            if (not isinstance(descriptor, dict)
                    or set(descriptor) != {"mediaType", "digest", "size"}
                    or descriptor.get("mediaType") not in media_types
                    or not isinstance(descriptor.get("digest"), str)
                    or not descriptor["digest"].startswith("sha256:")
                    or not HASH_RE.fullmatch(descriptor["digest"][7:])
                    or isinstance(descriptor.get("size"), bool)
                    or not isinstance(descriptor.get("size"), int)
                    or descriptor["size"] < 0 or descriptor["size"] > limit):
                raise RecoveryError("image export " + label + " descriptor invalid")
            blob_digest = descriptor["digest"][7:]
            name = "blobs/sha256/" + blob_digest
            member = members.get(name)
            if (member is None or not member.isfile() or member.size != descriptor["size"]):
                raise RecoveryError("image export " + label + " size mismatch")
            source = archive.extractfile(member)
            if source is None:
                raise RecoveryError("image export " + label + " missing")
            actual = hashlib.sha256()
            remaining = member.size
            while remaining:
                chunk = source.read(min(1024 * 1024, remaining))
                if not chunk:
                    raise RecoveryError("image export " + label + " truncated")
                actual.update(chunk)
                remaining -= len(chunk)
            if source.read(1) or actual.hexdigest() != blob_digest:
                raise RecoveryError("image export " + label + " hash mismatch")
            return name

        _, manifest = read_json_member("manifest.json", 1024 * 1024, "manifest")
        if (not isinstance(manifest, list) or len(manifest) != 1
                or not isinstance(manifest[0], dict)):
            raise RecoveryError("image export manifest invalid")
        compatibility = manifest[0]
        config_name = compatibility.get("Config")
        layers = compatibility.get("Layers")
        digest = expected_id[7:]

        if "index.json" not in members:
            config_path = pathlib.PurePosixPath(config_name) if isinstance(config_name, str) else None
            valid_name = config_path is not None and (
                config_path.parts == (digest + ".json",)
                or config_path.parts == ("blobs", "sha256", digest))
            config_member = members.get(config_name)
            if (config_member is None or not config_member.isfile()
                    or config_member.size > 2 * 1024 * 1024 or not valid_name):
                raise RecoveryError("image export config identity mismatch")
            source = archive.extractfile(config_member)
            raw = source.read(2 * 1024 * 1024 + 1) if source is not None else b""
            if (len(raw) != config_member.size or len(raw) > 2 * 1024 * 1024
                    or hashlib.sha256(raw).hexdigest() != digest):
                raise RecoveryError("image export config hash mismatch")
            try:
                config = json.loads(raw)
            except (UnicodeDecodeError, json.JSONDecodeError):
                raise RecoveryError("image export config invalid") from None
        else:
            _, layout = read_json_member("oci-layout", 1024, "OCI layout")
            if layout != {"imageLayoutVersion": "1.0.0"}:
                raise RecoveryError("image export OCI layout invalid")
            _, index = read_json_member("index.json", 1024 * 1024, "OCI index")
            if (not isinstance(index, dict)
                    or set(index) != {"schemaVersion", "mediaType", "manifests"}
                    or index.get("schemaVersion") != 2
                    or index.get("mediaType") != "application/vnd.oci.image.index.v1+json"
                    or not isinstance(index.get("manifests"), list)
                    or len(index["manifests"]) != 1):
                raise RecoveryError("image export OCI index invalid")
            image_descriptor = index["manifests"][0]
            if (not isinstance(image_descriptor, dict)
                    or not {"mediaType", "digest", "size"}.issubset(image_descriptor)
                    or set(image_descriptor) - {"mediaType", "digest", "size", "platform"}
                    or image_descriptor.get("digest") != expected_id):
                raise RecoveryError("image export OCI image descriptor invalid")
            platform = image_descriptor.get("platform")
            if (platform is not None and
                    (not isinstance(platform, dict)
                     or platform.get("architecture") != expected_architecture
                     or platform.get("os") != "linux")):
                raise RecoveryError("image export OCI platform mismatch")
            descriptor = {key: image_descriptor[key]
                          for key in ("mediaType", "digest", "size")}
            image_name = descriptor_blob(
                descriptor, {"application/vnd.oci.image.manifest.v1+json"},
                "OCI image manifest", 2 * 1024 * 1024)
            _, image_manifest = read_json_member(
                image_name, 2 * 1024 * 1024, "OCI image manifest")
            if (not isinstance(image_manifest, dict)
                    or set(image_manifest) != {"schemaVersion", "mediaType", "config", "layers"}
                    or image_manifest.get("schemaVersion") != 2
                    or image_manifest.get("mediaType") != "application/vnd.oci.image.manifest.v1+json"
                    or not isinstance(image_manifest.get("layers"), list)):
                raise RecoveryError("image export OCI image manifest invalid")
            config_name_from_descriptor = descriptor_blob(
                image_manifest.get("config"), {"application/vnd.oci.image.config.v1+json"},
                "OCI config", 2 * 1024 * 1024)
            layer_names = []
            for layer_number, layer_descriptor in enumerate(image_manifest["layers"]):
                layer_names.append(descriptor_blob(
                    layer_descriptor, {"application/vnd.oci.image.layer.v1.tar+gzip"},
                    "OCI layer " + str(layer_number), EXPANSION_CAP))
            if config_name != config_name_from_descriptor or layers != layer_names:
                raise RecoveryError("image export compatibility manifest mismatch")
            _, config = read_json_member(config_name, 2 * 1024 * 1024, "OCI config")

        if (not isinstance(config, dict) or config.get("architecture") != expected_architecture
                or config.get("os") != "linux"):
            raise RecoveryError("image export architecture mismatch")


def _real_fixed_root(root):
    root = pathlib.Path(root)
    if root != ROOT or root.resolve(strict=True) != ROOT:
        raise RecoveryError("Oracle root mismatch")
    current = pathlib.Path(root.anchor)
    for part in root.parts[1:]:
        current /= part
        if current.is_symlink():
            raise RecoveryError("Oracle root has symlink ancestor")


def _validate_real_directory(path, uid, mode, label):
    path = pathlib.Path(path)
    try:
        info = path.lstat()
    except OSError:
        raise RecoveryError(label + " is unavailable") from None
    if (path.is_symlink() or not stat.S_ISDIR(info.st_mode) or info.st_uid != uid
            or stat.S_IMODE(info.st_mode) != mode):
        raise RecoveryError(label + " ownership or mode mismatch")
    return path


def validate_private_output_paths(root, uid=0):
    root = pathlib.Path(root)
    for relative in ("backups", "results"):
        path = root / relative
        if path.is_symlink() or not path.is_dir():
            raise RecoveryError("Oracle output parent is unsafe")
    for relative, label in (("backups/staging", "staging directory"),
                            ("results/locks", "lock directory"),
                            ("results/latest", "result directory")):
        _validate_real_directory(root / relative, uid, 0o700, label)


def _private_state(root, raw, maintenance_exists):
    state = {"private_test_active": (pathlib.Path(root) / "results/private-test-active.json").exists(),
             "maintenance_exists": maintenance_exists}
    for key, item in zip(("api", "fixture"), raw):
        state[key] = {"exists": True, "id": item.get("Id"),
                      "owner": item.get("Config", {}).get("Labels", {}).get("holyforge.owner"),
                      "image": item.get("Image"), "running": item.get("State", {}).get("Running"),
                      "ports": list((item.get("NetworkSettings", {}).get("Ports") or {}).keys())}
    return state


def recheck_private_state(root, before, identities, runner=subprocess.run):
    raw = json.loads(run_command(
        ["docker", "container", "inspect", "holyarrs-metadata-private",
         "holyarrs-metadata-private-fixture"], runner=runner).stdout)
    maintenance = run_command(
        ["docker", "container", "inspect", "--format", "{{.Id}}", MAINTENANCE],
        runner=runner, check=False).returncode == 0
    after = _private_state(root, raw, maintenance)
    validate_private_state(after, identities)
    if after != before:
        raise RecoveryError("private container state changed")
    return after


def _oracle_status(root):
    return read_status(root / "results/latest/scheduled-backup.json",
                       root / "results/latest/scheduled-backup-failure.json")


def _existing_from_inspect(raw):
    result = []
    for name, item in zip(EXISTING, raw):
        state = item.get("State", {})
        result.append({"name": name, "id": item.get("Id"), "started": state.get("StartedAt"),
                       "restarts": item.get("RestartCount"), "running": state.get("Running"),
                       "health": (state.get("Health") or {}).get("Status")})
    return result


def _oracle_backup(root=ROOT, runner=subprocess.run):
    """Create a recovery point. Deployment acceptance must exercise this path."""
    if not hasattr(os, "geteuid") or os.geteuid() != 0:
        raise RecoveryError("backup requires root")
    _real_fixed_root(root)
    validate_private_output_paths(root)
    lock = ServiceLock(root / "results/locks/scheduled-backup.lock")
    with lock:
        # The root-installed manifest binds every immutable package input. The
        # remaining orchestration is intentionally explicit and fail-closed.
        manifest_path = root / "settings/recovery-manifest.json"
        _validate_root_owned_manifest_path(root, manifest_path)
        try:
            manifest_stat = manifest_path.lstat()
        except OSError:
            raise RecoveryError("recovery manifest is unavailable") from None
        if (manifest_path.is_symlink() or not manifest_path.is_file() or manifest_stat.st_uid != 0
                or manifest_stat.st_mode & 0o022):
            raise RecoveryError("recovery manifest ownership or mode mismatch")
        try:
            manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        except (OSError, UnicodeDecodeError, json.JSONDecodeError):
            raise RecoveryError("recovery manifest is invalid") from None
        immutable = validate_recovery_manifest(root, manifest)
        existing_before = _existing_from_inspect(json.loads(run_command(
            ["docker", "container", "inspect", *EXISTING], runner=runner).stdout))
        validate_existing_services(manifest["existing_services"], existing_before)
        run_name = "run-" + utc_now().strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:8]
        staging_root = root / "backups/staging"
        prior = [p for p in staging_root.glob("run-*") if p.is_dir() and not p.is_symlink()]
        if prior:
            size = sum(p.stat().st_size for d in prior for p in d.rglob("*") if p.is_file() and not p.is_symlink())
            raise RecoveryError(f"owned failed staging exists ({size} bytes)")
        stage = staging_root / run_name
        stage.mkdir(parents=True, mode=0o700)
        atomic_json(stage / ".owned.json", {"schema_version": 1, "owner": OWNER, "run_name": run_name})
        try:
            # Runtime-specific inspection, snapshot and encryption are command
            # driven. Output is never reflected in receipts or exceptions.
            state_result = run_command(["docker", "container", "inspect", "holyarrs-metadata-private", "holyarrs-metadata-private-fixture"], runner=runner)
            state_raw = json.loads(state_result.stdout)
            maintenance_exists = run_command(
                ["docker", "container", "inspect", "--format", "{{.Id}}", MAINTENANCE],
                runner=runner, check=False).returncode == 0
            state = _private_state(root, state_raw, maintenance_exists)
            validate_private_state(state, manifest["identities"])
            data = root / "data/test-runs/2026-10-15-build-7"
            database_bytes = sum((data / name).stat().st_size for name in ("cache.sqlite", "access.sqlite"))
            immutable_bytes = sum(path.stat().st_size for path in immutable.values())
            image_estimate = sum(int(x.get("Size", 0)) for x in json.loads(run_command(
                ["docker", "image", "inspect", API_IMAGE, FIXTURE_IMAGE], runner=runner).stdout))
            peak = estimate_staging_peak(database_bytes, immutable_bytes, image_estimate)
            require_disk_space(shutil.disk_usage(root).free, peak)
            snapshot_parent = stage / "snapshot-parent"
            snapshot_parent.mkdir(mode=0o700)
            os.chown(snapshot_parent, 10001, 10001)
            snapshot = snapshot_parent / "snapshot"
            if snapshot.exists():
                raise RecoveryError("snapshot destination must not exist")
            backup_name = "holyarrs-metadata-private-recovery-" + uuid.uuid4().hex[:12]
            backup_args = ["docker", "run", "--rm", "--pull=never", "--name", backup_name,
                           "--restart", "no", "--cpus", "1", "--memory", "512m", "--pids-limit", "128",
                           "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
                           "--network", "none", "--label", "holyforge.owner=" + OWNER,
                           "--label", "com.centurylinklabs.watchtower.enable=false",
                           "-v", f"{data}:/data", "-v", f"{root / 'settings/versions/2026-10-15-build-7'}:/config:ro",
                           "-v", f"{snapshot_parent}:/backup", API_IMAGE, "backup", "--config", "/config/private.json",
                           "--data-dir", "/data", "--destination", "/backup/snapshot"]
            run_command(backup_args, runner=runner, timeout=420)
            validate_database_pair(snapshot)
            package_files = dict(immutable)
            package_files["data/cache.sqlite"] = snapshot / "cache.sqlite"
            package_files["data/access.sqlite"] = snapshot / "access.sqlite"
            owned_files = [stage / ".owned.json", snapshot / "cache.sqlite", snapshot / "access.sqlite"]
            for image_name, image_id in (("images/api.tar", API_IMAGE), ("images/fixture.tar", FIXTURE_IMAGE)):
                target = stage / pathlib.PurePosixPath(image_name).name
                run_command(["docker", "image", "save", "-o", str(target), image_id], runner=runner)
                image_export_identity(target, image_id)
                package_files[image_name] = target
                owned_files.append(target)
            package_manifest = {name: {"bytes": sha256_file(path)[0], "sha256": sha256_file(path)[1]} for name, path in package_files.items()}
            manifest_bytes = json.dumps({"schema_version": 1, "created_at": iso(), "identities": manifest["identities"],
                                         "members": package_manifest}, sort_keys=True, separators=(",", ":")).encode()
            package_files["manifest.json"] = stage / "manifest.json"
            package_files["manifest.json"].write_bytes(manifest_bytes)
            owned_files.append(package_files["manifest.json"])
            package_manifest["manifest.json"] = {"bytes": len(manifest_bytes), "sha256": hashlib.sha256(manifest_bytes).hexdigest()}
            package = stage / "package.tar.gz"
            with tarfile.open(package, "w:gz", format=tarfile.PAX_FORMAT) as archive:
                for name in sorted(package_files):
                    archive.add(package_files[name], arcname=name, recursive=False)
            owned_files.append(package)
            recipient = root / "settings/private/backup-recipient.pem"
            certificate = run_command(["openssl", "x509", "-in", str(recipient), "-noout"], runner=runner)
            if certificate.returncode or b"PRIVATE KEY" in recipient.read_bytes():
                raise RecoveryError("backup recipient is not certificate-only")
            ciphertext_temp = stage / (run_name + ".cms.tmp")
            run_command(["openssl", "cms", "-encrypt", "-binary", "-aes-256-gcm", "-outform", "DER",
                         "-in", str(package), "-out", str(ciphertext_temp), str(recipient)], runner=runner)
            size, value_hash = sha256_file(ciphertext_temp, CIPHERTEXT_CAP)
            # Recheck stopped containers and unrelated baseline before publishing.
            recheck_private_state(root, state, manifest["identities"], runner)
            existing_after = _existing_from_inspect(json.loads(run_command(
                ["docker", "container", "inspect", *EXISTING], runner=runner).stdout))
            validate_existing_services(existing_before, existing_after)
            import pwd
            ghost = pwd.getpwnam("ghost")
            scheduled = ensure_export_directory(root / "backups/scheduled", ghost.pw_uid, ghost.pw_gid)
            final = scheduled / run_name
            final.mkdir(parents=True, mode=0o700)
            ciphertext = final / (run_name + ".cms")
            os.replace(ciphertext_temp, ciphertext)
            receipt = {"schema_version": 1, "status": "created", "success_at": iso(), "run_name": run_name,
                       "ciphertext_path": str(ciphertext), "ciphertext_bytes": size,
                       "ciphertext_sha256": value_hash, "identities": manifest["identities"],
                       "services_unchanged": True, "api_stopped": True, "fixture_stopped": True,
                       "offhost_copy_verified": False}
            cleanup_owned_staging(stage, owned_files)
            atomic_json(final / "receipt.json", receipt)
            # Only encrypted content and the sanitized receipt are handed to
            # the fixed SCP account. Plaintext never leaves root-only staging.
            for exported in (ciphertext, final / "receipt.json"):
                os.chown(exported, ghost.pw_uid, ghost.pw_gid)
                os.chmod(exported, 0o600)
            os.chown(final, ghost.pw_uid, ghost.pw_gid)
            os.chmod(final, 0o700)
            atomic_json(root / "results/latest/scheduled-backup.json", receipt)
            return receipt
        except Exception as error:
            write_failure(root / "results/latest/scheduled-backup-failure.json", "backup", error)
            raise


def oracle_backup(root=ROOT, runner=subprocess.run):
    try:
        return _oracle_backup(root, runner)
    except Exception as error:
        with contextlib.suppress(Exception):
            validate_private_output_paths(root)
            write_failure(pathlib.Path(root) / "results/latest/scheduled-backup-failure.json", "backup", error)
        if isinstance(error, RecoveryError):
            raise
        raise RecoveryError(sanitize_error(error)) from None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("backup", "status", "prepare-restore"))
    parser.add_argument("--source", type=pathlib.Path)
    parser.add_argument("--destination", type=pathlib.Path)
    args = parser.parse_args()
    try:
        if args.action == "backup":
            result = oracle_backup()
        elif args.action == "status":
            result = _oracle_status(ROOT)
            if result.get("stale"):
                print(json.dumps(result, sort_keys=True))
                raise SystemExit(2)
        else:
            if not args.source or not args.destination:
                raise RecoveryError("prepare-restore requires source and destination")
            prepare_restore(args.source, args.destination)
            result = {"schema_version": 1, "status": "prepared", "destination": str(args.destination),
                      "all_tokens_revoked": True}
        print(json.dumps(result, sort_keys=True))
    except Exception as error:
        message = str(error) if isinstance(error, RecoveryError) else sanitize_error(error)
        print(json.dumps({"schema_version": 1, "status": "failed", "error": message}, sort_keys=True))
        raise SystemExit(1)


if __name__ == "__main__":
    main()
