"""Private image recovery scenarios; only new copies and isolated docker run calls."""

import argparse
import contextlib
import hashlib
import json
import os
import pathlib
import re
import shutil
import sqlite3
import subprocess
import time
import uuid


DATABASES = ("cache.sqlite", "access.sqlite")
FILES = tuple(name + suffix for name in DATABASES for suffix in ("", "-wal", "-shm"))
FUTURE_VERSION = 2147483000


def validate_paths(source, work, credential):
    source, work, credential = (pathlib.Path(p).absolute() for p in (source, work, credential))
    if source.resolve() != source or credential.resolve() != credential or work.parent.resolve() != work.parent:
        raise ValueError("symlink paths are not permitted")
    if not source.is_dir() or not credential.is_file() or not work.parent.is_dir():
        raise ValueError("required private paths unavailable")
    if work.exists() or not re.fullmatch(r"metadata-recovery-[a-zA-Z0-9][a-zA-Z0-9-]{0,80}", work.name):
        raise ValueError("work path must be a new metadata-recovery prefix")
    if work == source or source in work.parents or work in source.parents or work == credential or work in credential.parents:
        raise ValueError("source and recovery targets must be disjoint")
    for name in DATABASES:
        path = source / name
        if not path.is_file() or path.is_symlink():
            raise ValueError("paired source databases required")
    for name in FILES:
        if (source / name).is_symlink():
            raise ValueError("source links forbidden")
    return source, work, credential


def file_manifest(directory):
    result = {}
    for name in FILES:
        path = pathlib.Path(directory) / name
        if not path.exists():
            continue
        if path.is_symlink() or not path.is_file():
            raise ValueError("unexpected source file type")
        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        result[name] = {"bytes": path.stat().st_size, "sha256": digest.hexdigest()}
    return result


def secure_tree(directory):
    for path in (pathlib.Path(directory), *pathlib.Path(directory).rglob("*")):
        if path.is_symlink():
            raise ValueError("recovery links forbidden")
        os.chown(path, 10001, 10001)
        path.chmod(0o700 if path.is_dir() else 0o600)


def copy_pair(source, destination):
    destination.mkdir(mode=0o700)
    for name in FILES:
        original = source / name
        if original.exists():
            if original.is_symlink() or not original.is_file():
                raise ValueError("invalid paired copy input")
            shutil.copy2(original, destination / name)
    secure_tree(destination)


def confined_path(path):
    path = pathlib.Path(path).absolute()
    if path.resolve() != path or not any(part.startswith("metadata-recovery-") for part in path.parts):
        raise ValueError("docker mount outside recovery prefix")
    if "," in str(path) or "\n" in str(path):
        raise ValueError("invalid bind mount path")
    return path


def docker_command(image, data, label, arguments, credentials=None, backup=None):
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}", image):
        raise ValueError("invalid private image reference")
    if arguments[0] not in ("check", "token-revoke", "backup"):
        raise ValueError("unsupported recovery command")
    data = confined_path(data)
    name = "metadata-recovery-" + uuid.uuid4().hex[:16]
    command = ["docker", "run", "--rm", "--pull=never", "--name", name,
               "--restart=no", "--label", "com.centurylinklabs.watchtower.enable=false",
               "--network=none", "--cpus=0.5", "--memory=512m", "--memory-swap=512m",
               "--pids-limit=64", "--read-only", "--user=10001:10001",
               "--cap-drop=ALL", "--security-opt=no-new-privileges",
               "--mount", f"type=bind,source={data},target=/data"]
    if credentials is not None:
        command += ["--mount", f"type=bind,source={confined_path(credentials)},target=/credentials/token,readonly"]
    if backup is not None:
        command += ["--mount", f"type=bind,source={confined_path(backup)},target=/backup"]
    command += [image, *arguments]
    started = time.monotonic()
    timed_out, code = False, None
    try:
        process = subprocess.run(command, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                 stderr=subprocess.DEVNULL, timeout=90, check=False)
        code = process.returncode
    except subprocess.TimeoutExpired:
        timed_out = True
    return {"name": label, "operation": arguments[0], "container_name": name,
            "exit_status": code, "timed_out": timed_out, "seconds": round(time.monotonic() - started, 3)}


def read_db(path):
    return contextlib.closing(sqlite3.connect(path.absolute().as_uri() + "?mode=ro", uri=True, timeout=5))


def digest_rows(rows):
    digest = hashlib.sha256()
    for row in rows:
        for value in row:
            encoded = b"B" + value if isinstance(value, bytes) else b"J" + json.dumps(value, sort_keys=True).encode()
            digest.update(str(len(encoded)).encode() + b":" + encoded)
    return digest.hexdigest()


def contains_secret(path, secret):
    tail = b""
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            block = tail + chunk
            if secret in block:
                return True
            tail = block[-max(1, len(secret) - 1):]
    return False


def inspect_pair(directory, secret):
    versions = {}
    for name in DATABASES:
        with read_db(directory / name) as db:
            if db.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
                raise RuntimeError("paired integrity failed")
            versions[name] = db.execute("PRAGMA user_version").fetchone()[0]
    with read_db(directory / "cache.sqlite") as db:
        entries = db.execute("SELECT COUNT(*) FROM entries").fetchone()[0]
        cache_digest = digest_rows(db.execute("SELECT * FROM entries ORDER BY key"))
    with read_db(directory / "access.sqlite") as db:
        tokens = db.execute("SELECT hash,revoked FROM tokens ORDER BY hash").fetchall()
    expected = hashlib.sha256(secret).digest()
    selected = [revoked for token_hash, revoked in tokens if token_hash == expected]
    hash_only = all(isinstance(token_hash, bytes) and len(token_hash) == 32 and revoked in (0, 1) for token_hash, revoked in tokens)
    for name in FILES:
        path = directory / name
        if path.exists() and contains_secret(path, secret):
            hash_only = False
    return {"integrity": True, "versions": versions, "entries": entries,
            "cache_digest": cache_digest, "token_digest": digest_rows(tokens),
            "token_count": len(tokens), "revoked_count": sum(row[1] for row in tokens),
            "selected_token_present": len(selected) == 1,
            "selected_token_revoked": selected == [1], "hash_only": hash_only}


def public_inspection(info):
    return {key: value for key, value in info.items() if not key.endswith("_digest")}


def schema_version(path):
    with read_db(path) as db:
        return db.execute("PRAGMA user_version").fetchone()[0]


def run_recovery(image, source, work, credential):
    source, work, credential = validate_paths(source, work, credential)
    before = file_manifest(source)
    size = sum(item["bytes"] for item in before.values())
    if size > 4 * 1024**3 or shutil.disk_usage(work.parent).free < size * 11 + 64 * 1024**2:
        raise ValueError("insufficient bounded recovery capacity")
    secret = credential.read_bytes().strip()
    if not 1 <= len(secret) <= 1024 or b"\n" in secret or b"\r" in secret:
        raise ValueError("invalid private credential file")
    work.mkdir(mode=0o700)
    secure_tree(work)
    result = {"passed": False, "synthetic_private_only": True, "source_bytes": size,
              "commands": [], "scenarios": {}, "source_unchanged": False}
    def command(directory, label, operation, **mounts):
        value = docker_command(image, directory, label, operation, **mounts)
        result["commands"].append(value)
        if value["timed_out"] or value["exit_status"] in (None, 125, 126, 127):
            raise RuntimeError("container execution unavailable")
        return value["exit_status"]
    try:
        pristine = work / "pristine"
        copy_pair(source, pristine)
        clean_status = command(pristine, "clean_copy", ["check", "--data-dir", "/data"])
        baseline = inspect_pair(pristine, secret)
        result["scenarios"]["clean_copy"] = {**public_inspection(baseline), "exit_status": clean_status}
        if clean_status != 0 or not baseline["hash_only"] or not baseline["selected_token_present"]:
            raise RuntimeError("clean paired snapshot failed")

        credentials = work / "credentials"
        credentials.mkdir(mode=0o700)
        private_token = credentials / "token"
        private_token.write_bytes(secret + b"\n")
        secure_tree(credentials)
        private_token.chmod(0o400)
        revoked = work / "revoked"
        copy_pair(source, revoked)
        status = command(revoked, "revoke", ["token-revoke", "--data-dir", "/data", "--token-file", "/credentials/token"], credentials=private_token)
        revoked_info = inspect_pair(revoked, secret)
        result["scenarios"]["revocation"] = {**public_inspection(revoked_info), "exit_status": status}
        if status != 0 or not revoked_info["selected_token_revoked"] or not revoked_info["hash_only"] or revoked_info["cache_digest"] != baseline["cache_digest"]:
            raise RuntimeError("revocation not isolated and durable")

        backups = work / "backups"
        backups.mkdir(mode=0o700)
        secure_tree(backups)
        status = command(revoked, "paired_backup", ["backup", "--data-dir", "/data", "--destination", "/backup/snapshot"], backup=backups)
        snapshot = backups / "snapshot"
        backup_info = inspect_pair(snapshot, secret)
        result["scenarios"]["paired_backup"] = {**public_inspection(backup_info), "exit_status": status}
        if status != 0 or backup_info["cache_digest"] != revoked_info["cache_digest"] or backup_info["token_digest"] != revoked_info["token_digest"]:
            raise RuntimeError("paired backup mismatch")
        restored = work / "restored"
        copy_pair(snapshot, restored)
        status = command(restored, "restored_backup", ["check", "--data-dir", "/data"])
        restored_info = inspect_pair(restored, secret)
        preserved = restored_info["token_digest"] == revoked_info["token_digest"] and restored_info["selected_token_revoked"] and restored_info["cache_digest"] == baseline["cache_digest"]
        result["scenarios"]["restored_backup"] = {**public_inspection(restored_info), "revocation_preserved": preserved, "exit_status": status}
        if status != 0 or not preserved or not restored_info["hash_only"]:
            raise RuntimeError("restored revocation mismatch")

        for database in DATABASES:
            kind = database.split(".")[0]
            future = work / ("future-" + kind)
            copy_pair(source, future)
            with contextlib.closing(sqlite3.connect(future / database)) as db:
                db.execute("PRAGMA user_version=" + str(FUTURE_VERSION))
                db.commit()
            secure_tree(future)
            status = command(future, "future_" + kind, ["check", "--data-dir", "/data"])
            unchanged = schema_version(future / database) == FUTURE_VERSION
            refused = status != 0 and unchanged
            result["scenarios"]["future_" + kind] = {"refused": refused, "future_version_preserved": unchanged, "exit_status": status}
            if not refused:
                raise RuntimeError("unknown schema was accepted or downgraded")

            corrupt = work / ("corrupt-" + kind)
            copy_pair(source, corrupt)
            with (corrupt / database).open("r+b") as stream:
                stream.write(b"synthetic-corrupt-header!".ljust(32, b"!"))
            status = command(corrupt, "corrupt_" + kind, ["check", "--data-dir", "/data"])
            result["scenarios"]["corrupt_" + kind] = {"refused": status != 0, "corrupted_bytes": 32, "exit_status": status}
            if status == 0:
                raise RuntimeError("corrupt database was accepted")

        rollback = work / "rollback"
        copy_pair(source, rollback)
        status = command(rollback, "rollback", ["check", "--data-dir", "/data"])
        rolled = inspect_pair(rollback, secret)
        preserved = rolled["cache_digest"] == baseline["cache_digest"] and rolled["token_digest"] == baseline["token_digest"]
        result["scenarios"]["rollback"] = {**public_inspection(rolled), "original_pair_preserved": preserved, "exit_status": status}
        result["passed"] = status == 0 and preserved
    except (RuntimeError, OSError, ValueError, sqlite3.DatabaseError):
        result["error"] = "recovery scenario failed; inspect scenario status and isolated copies"
    finally:
        result["source_unchanged"] = file_manifest(source) == before
        result["passed"] = result["passed"] and result["source_unchanged"]
        result["work_bytes"] = sum(path.stat().st_size for path in work.rglob("*") if path.is_file() and not path.is_symlink())
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for flag in ("image", "source-data", "work-dir", "token-file", "output"):
        parser.add_argument("--" + flag, required=True)
    args = parser.parse_args()
    try:
        source, work, credential = validate_paths(args.source_data, args.work_dir, args.token_file)
        output = pathlib.Path(args.output).absolute()
        if output.suffix != ".json" or output.exists() or work not in output.parents or output.parent != work:
            raise ValueError("output must be a new JSON file directly inside work directory")
        result = run_recovery(args.image, source, work, credential)
        output.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        output.chmod(0o600)
    except (OSError, ValueError):
        parser.error("invalid isolated paths, permissions or private recovery configuration")
    print(json.dumps({"passed": result["passed"], "source_unchanged": result["source_unchanged"], "scenarios": len(result["scenarios"])}, sort_keys=True))
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
