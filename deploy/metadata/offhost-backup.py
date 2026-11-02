"""Pull, authenticate and verify a private metadata recovery point on Windows."""
import argparse
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tarfile
import uuid


recovery = sys.modules.get("private_recovery")
if recovery is None:
    _spec = importlib.util.spec_from_file_location(
        "holyarrs_private_recovery", pathlib.Path(__file__).with_name("private-recovery.py"))
    recovery = importlib.util.module_from_spec(_spec)
    _spec.loader.exec_module(recovery)

RecoveryError = recovery.RecoveryError
REMOTE_ALIAS = "coderluii-vm"
REMOTE_ROOT = "/home/home/holyarrs-metadata-server"
REMOTE_COMMAND = "sudo python3 -B /home/home/holyarrs-metadata-server/application/tools/private-recovery.py backup"
DEFAULT_SECRETS_ROOT = pathlib.Path("C:/Users/Luis/holyarrs-metadata-backup")
DEFAULT_DESTINATION = DEFAULT_SECRETS_ROOT / "backups"
PRIVATE_KEY = DEFAULT_SECRETS_ROOT / "keys/backup-key.pem"
RECIPIENT = DEFAULT_SECRETS_ROOT / "keys/recipient.pem"
MAX_POINTS = 7
RETENTION_BUDGET = 2 * 1024**3
LOCAL_RESERVE = 10 * 1024**3
PLAINTEXT_PEAK = 2 * recovery.EXPANSION_CAP + 64 * 1024**2
EXPECTED_PACKAGE_MEMBERS = {
    "data/cache.sqlite", "data/access.sqlite", "config/private.json",
    "source/runtime.tar.gz", "source/recovery-tools.tar.gz",
    "operator/private-ops.py", "operator/current-build.json",
    "images/api.tar", "images/fixture.tar",
}
CONTROL_FILES = {"latest.json", "failure.json", "status.json"}
SCHEDULE_ID_RE = re.compile(r"[0-9a-f]{32}\Z")


def _is_reparse(path):
    try:
        value = pathlib.Path(path).lstat()
    except FileNotFoundError:
        return False
    return bool(getattr(value, "st_file_attributes", 0) & 0x400)


def validate_destination(destination, secrets_root=DEFAULT_SECRETS_ROOT):
    destination, secrets_root = pathlib.Path(destination), pathlib.Path(secrets_root)
    try:
        base = secrets_root.resolve(strict=True)
    except OSError:
        raise RecoveryError("restricted secrets root is unavailable") from None
    candidate = destination.absolute()
    if ".." in pathlib.Path(destination).parts:
        raise RecoveryError("destination parent traversal is not allowed")
    current = pathlib.Path(candidate.anchor)
    for part in candidate.parts[1:]:
        current /= part
        if current.exists() and (current.is_symlink() or _is_reparse(current)):
            raise RecoveryError("destination has a symlink or reparse ancestor")
    try:
        resolved = candidate.resolve(strict=False)
        resolved.relative_to(base)
    except (OSError, ValueError):
        raise RecoveryError("destination must remain under the restricted secrets tree") from None
    if candidate.drive.upper() == "O:":
        raise RecoveryError("O: is not an allowed backup destination")
    return candidate


def validate_remote_ciphertext_path(run_name, remote_path):
    recovery.validate_run_name(run_name)
    expected = f"{REMOTE_ROOT}/backups/scheduled/{run_name}/{run_name}.cms"
    if remote_path != expected:
        raise RecoveryError("remote ciphertext path mismatch")
    return remote_path


def _exact_keys(value, keys, label):
    if not isinstance(value, dict) or set(value) != set(keys):
        raise RecoveryError("invalid " + label)


def validate_oracle_receipt(receipt):
    keys = {"schema_version", "status", "success_at", "run_name", "ciphertext_path",
            "ciphertext_bytes", "ciphertext_sha256", "identities", "services_unchanged",
            "api_stopped", "fixture_stopped", "offhost_copy_verified"}
    _exact_keys(receipt, keys, "Oracle receipt")
    if receipt["schema_version"] != 1 or receipt["status"] != "created":
        raise RecoveryError("Oracle receipt status mismatch")
    recovery.parse_time(receipt["success_at"])
    recovery.validate_run_name(receipt["run_name"])
    validate_remote_ciphertext_path(receipt["run_name"], receipt["ciphertext_path"])
    if not isinstance(receipt["ciphertext_bytes"], int) or not 0 < receipt["ciphertext_bytes"] <= recovery.CIPHERTEXT_CAP:
        raise RecoveryError("Oracle ciphertext size is invalid")
    if recovery.HASH_RE.fullmatch(receipt["ciphertext_sha256"]) is None:
        raise RecoveryError("Oracle ciphertext hash is invalid")
    recovery.validate_identities(recovery.expected_identity_manifest(), receipt["identities"])
    if not all(receipt[key] is expected for key, expected in (
            ("services_unchanged", True), ("api_stopped", True),
            ("fixture_stopped", True), ("offhost_copy_verified", False))):
        raise RecoveryError("Oracle stopped-state proof is invalid")
    return receipt


def validate_downloaded_ciphertext(receipt, path):
    validate_oracle_receipt(receipt)
    size, value_hash = recovery.sha256_file(path, recovery.CIPHERTEXT_CAP)
    if size != receipt["ciphertext_bytes"] or value_hash != receipt["ciphertext_sha256"]:
        raise RecoveryError("downloaded ciphertext does not match Oracle receipt")


def sample_receipt(*, name="run-20261015T123456Z-a1b2c3d4", ciphertext=b"ciphertext"):
    """Documented minimal receipt example, also useful to callers constructing fixtures."""
    return {"schema_version": 1, "status": "created", "success_at": "2026-10-15T12:34:56Z",
            "run_name": name,
            "ciphertext_path": f"{REMOTE_ROOT}/backups/scheduled/{name}/{name}.cms",
            "ciphertext_bytes": len(ciphertext), "ciphertext_sha256": hashlib.sha256(ciphertext).hexdigest(),
            "identities": recovery.expected_identity_manifest(), "services_unchanged": True,
            "api_stopped": True, "fixture_stopped": True, "offhost_copy_verified": False}


def crypto_docker_args(name, key, recipient, input_dir, output_dir):
    args = ["docker", "run", "--rm", "--pull=never", "--name", name,
            "--restart", "no", "--cpus", "1", "--memory", "512m",
            "--pids-limit", "64", "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges:true", "--network", "none",
            "--label", "holyforge.owner=" + recovery.OWNER,
            "--label", "com.centurylinklabs.watchtower.enable=false",
            "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m"]
    schedule_id = os.environ.get("HOLYARRS_METADATA_SCHEDULE_ID", "")
    if SCHEDULE_ID_RE.fullmatch(schedule_id):
        args.extend(["--label", "holyforge.schedule=" + schedule_id])
    args.extend([
            "-v", f"{pathlib.Path(key).parent}:/keys:ro",
            "-v", f"{pathlib.Path(input_dir)}:/input:ro",
            recovery.CRYPTO_IMAGE, "openssl", "cms", "-decrypt", "-binary",
            "-inform", "DER", "-in", "/input/ciphertext.cms",
            "-inkey", "/keys/" + pathlib.Path(key).name,
            "-recip", "/keys/" + pathlib.Path(recipient).name,
            "-out", "/dev/stdout"])
    return args


def _read_package_manifest(package):
    recovery.bounded_gzip_tar_size(package, recovery.EXPANSION_CAP)
    try:
        with tarfile.open(package, "r:gz") as archive:
            matches = []
            count = 0
            for member in archive:
                count += 1
                if count > len(EXPECTED_PACKAGE_MEMBERS) + 1:
                    raise RecoveryError("package has too many archive members")
                if (not member.isfile() or not recovery._safe_member_name(member.name)
                        or member.name not in EXPECTED_PACKAGE_MEMBERS | {"manifest.json"}):
                    raise RecoveryError("package contains unsafe or unexpected member")
                if member.name == "manifest.json":
                    matches.append(member)
            if len(matches) != 1 or not matches[0].isfile() or matches[0].size > 1024 * 1024:
                raise RecoveryError("package manifest is missing or unsafe")
            raw = archive.extractfile(matches[0]).read(1024 * 1024 + 1)
    except (OSError, tarfile.TarError):
        raise RecoveryError("package manifest cannot be read") from None
    if len(raw) > 1024 * 1024:
        raise RecoveryError("package manifest exceeds limit")
    try:
        value = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise RecoveryError("package manifest is invalid") from None
    _exact_keys(value, {"schema_version", "created_at", "identities", "members"}, "package manifest")
    if value["schema_version"] != 1:
        raise RecoveryError("package schema version is unsupported")
    recovery.parse_time(value["created_at"])
    recovery.validate_identities(recovery.expected_identity_manifest(), value["identities"])
    if set(value["members"]) != EXPECTED_PACKAGE_MEMBERS:
        raise RecoveryError("package member allowlist mismatch")
    for item in value["members"].values():
        if (not isinstance(item, dict) or set(item) != {"bytes", "sha256"}
                or not isinstance(item["bytes"], int) or item["bytes"] < 0
                or recovery.HASH_RE.fullmatch(item["sha256"]) is None):
            raise RecoveryError("package member declaration is invalid")
    expected = dict(value["members"])
    expected["manifest.json"] = {"bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}
    return value, expected


def validate_synthetic_config(config):
    fixed = {"listen": ":8080", "data_dir": "/data", "private_fixtures": True,
             "cache_bytes": 2147483648, "cache_entries": 100000,
             "min_free_bytes": 21474836480, "ttl_seconds": 86400,
             "stale_seconds": 0, "max_concurrent": 100, "max_pending": 200}
    quota_sets = {(60000, 1000, 100), (60, 10, 4)}
    provider_values = {
        "tmdb": ("http://fixture:8081/tmdb/3", {5, 200}),
        "tvmaze": ("http://fixture:8081/tvmaze", {5, 1000}),
        "musicbrainz": ("http://fixture:8081/musicbrainz/ws/2", {5, 1100}),
    }
    expected_keys = set(fixed) | {"providers", "per_minute", "burst", "per_token_concurrent"}
    if not isinstance(config, dict) or set(config) != expected_keys:
        raise RecoveryError("private configuration is not the expected synthetic configuration")
    if any(config.get(key) != value for key, value in fixed.items()):
        raise RecoveryError("private configuration is not the expected synthetic configuration")
    if (config["per_minute"], config["burst"], config["per_token_concurrent"]) not in quota_sets:
        raise RecoveryError("private configuration is not the expected synthetic configuration")
    providers = config.get("providers")
    if not isinstance(providers, dict) or set(providers) != set(provider_values):
        raise RecoveryError("private configuration is not the expected synthetic configuration")
    intervals = {}
    for name, (url, allowed) in provider_values.items():
        item = providers.get(name)
        if (not isinstance(item, dict) or set(item) != {"base_url", "interval_ms"}
                or item.get("base_url") != url or item.get("interval_ms") not in allowed):
            raise RecoveryError("private configuration is not the expected synthetic configuration")
        intervals[name] = item["interval_ms"]
    if intervals not in ({"tmdb": 5, "tvmaze": 5, "musicbrainz": 5},
                          {"tmdb": 200, "tvmaze": 1000, "musicbrainz": 1100}):
        raise RecoveryError("private configuration is not the expected synthetic configuration")
    return config


def verify_plaintext_package(package, verification_destination):
    manifest, expected = _read_package_manifest(package)
    recovery.verify_and_publish_archive(package, verification_destination, expected)
    recovery.validate_database_pair(verification_destination / "data")
    config = json.loads((verification_destination / "config/private.json").read_text(encoding="utf-8"))
    validate_synthetic_config(config)
    recovery.image_export_identity(verification_destination / "images/api.tar", recovery.API_IMAGE)
    recovery.image_export_identity(verification_destination / "images/fixture.tar", recovery.FIXTURE_IMAGE)
    return manifest


def _verified_point(directory):
    directory = pathlib.Path(directory)
    if directory.is_symlink() or _is_reparse(directory) or not directory.is_dir():
        raise RecoveryError("recovery point is not an owned regular directory")
    recovery.validate_run_name(directory.name)
    allowed = {directory.name + ".cms", "verified-receipt.json"}
    if {p.name for p in directory.iterdir()} != allowed:
        raise RecoveryError("recovery point contains unknown content")
    ciphertext = directory / (directory.name + ".cms")
    receipt_path = directory / "verified-receipt.json"
    if (ciphertext.is_symlink() or receipt_path.is_symlink() or _is_reparse(ciphertext)
            or _is_reparse(receipt_path) or not ciphertext.is_file() or not receipt_path.is_file()):
        raise RecoveryError("recovery point contains unsafe content")
    try:
        receipt = json.loads(receipt_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        raise RecoveryError("verified receipt cannot be read") from None
    required = {"schema_version", "status", "verified_at", "run_name", "ciphertext_bytes",
                "ciphertext_sha256", "identities", "all_tokens_revoked_on_restore"}
    _exact_keys(receipt, required, "verified receipt")
    if receipt["schema_version"] != 1 or receipt["status"] != "verified" or receipt["run_name"] != directory.name:
        raise RecoveryError("verified receipt status mismatch")
    recovery.parse_time(receipt["verified_at"])
    recovery.validate_identities(recovery.expected_identity_manifest(), receipt["identities"])
    size, value_hash = recovery.sha256_file(ciphertext, recovery.CIPHERTEXT_CAP)
    if size != receipt["ciphertext_bytes"] or value_hash != receipt["ciphertext_sha256"]:
        raise RecoveryError("verified recovery point hash mismatch")
    return receipt, size + receipt_path.stat().st_size


def apply_retention(root, latest_name, *, max_points=MAX_POINTS, budget=RETENTION_BUDGET):
    root = pathlib.Path(root)
    recovery.validate_run_name(latest_name)
    unknown = [p for p in root.iterdir() if p.name not in CONTROL_FILES and not recovery.RUN_RE.fullmatch(p.name)]
    if unknown:
        raise RecoveryError("backup destination contains unknown content")
    points = []
    for child in root.iterdir():
        if recovery.RUN_RE.fullmatch(child.name):
            receipt, size = _verified_point(child)
            points.append((child.name, child, size, receipt))
    points.sort(key=lambda item: item[0])
    if not points or points[-1][0] != latest_name:
        raise RecoveryError("latest verified recovery point is missing")
    protected = points[-2:] if len(points) >= 2 else points
    protected_size = sum(item[2] for item in protected)
    if protected_size > budget:
        raise RecoveryError("latest and prior recovery points exceed retention budget")
    keep = list(points)
    removals = []
    while len(keep) > max_points or sum(item[2] for item in keep) > budget:
        candidate = keep[0]
        if candidate in protected:
            raise RecoveryError("retention cannot preserve latest and prior")
        keep.pop(0)
        removals.append(candidate)
    # Validation above is atomic with respect to policy: delete only after every
    # point and destination entry has been verified.
    for _name, directory, _size, _receipt in removals:
        (directory / (directory.name + ".cms")).unlink()
        (directory / "verified-receipt.json").unlink()
        directory.rmdir()
    return [item[0] for item in keep]


def _reject_incomplete_runs(destination):
    for child in destination.iterdir():
        if recovery.RUN_RE.fullmatch(child.name):
            _verified_point(child)
        elif child.name not in CONTROL_FILES:
            raise RecoveryError("backup destination contains unknown content")


def _cleanup_verification(directory, manifest):
    members = sorted(manifest["members"], key=lambda value: len(pathlib.PurePosixPath(value).parts), reverse=True)
    members.append("manifest.json")
    for name in members:
        target = directory.joinpath(*pathlib.PurePosixPath(name).parts)
        if not target.is_file() or target.is_symlink():
            raise RecoveryError("verification cleanup ownership mismatch")
        target.unlink()
    for folder in sorted((p for p in directory.rglob("*") if p.is_dir()), key=lambda p: len(p.parts), reverse=True):
        folder.rmdir()
    directory.rmdir()


def required_local_free(ciphertext_bytes):
    return LOCAL_RESERVE + ciphertext_bytes + PLAINTEXT_PEAK


def run_backup(destination, *, runner=subprocess.run, secrets_root=DEFAULT_SECRETS_ROOT):
    destination = validate_destination(destination, secrets_root)
    destination.mkdir(mode=0o700, parents=True, exist_ok=True)
    try:
        destination = validate_destination(destination, secrets_root)
        _reject_incomplete_runs(destination)
        ssh = ["ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
               "-o", "UpdateHostKeys=no", REMOTE_ALIAS, REMOTE_COMMAND]
        result = recovery.run_command(ssh, runner=runner, timeout=480)
        try:
            receipt = validate_oracle_receipt(json.loads(result.stdout))
        except (json.JSONDecodeError, UnicodeDecodeError):
            raise RecoveryError("Oracle returned an invalid sanitized receipt") from None
        needed = required_local_free(receipt["ciphertext_bytes"])
        if shutil.disk_usage(destination).free <= needed:
            raise RecoveryError("local disk reserve below verification requirement")
        run_dir = destination / receipt["run_name"]
        if run_dir.exists() or run_dir.is_symlink() or _is_reparse(run_dir):
            raise RecoveryError("local recovery run already exists")
        run_dir.mkdir(mode=0o700)
        ciphertext = run_dir / (receipt["run_name"] + ".cms")
        temporary = run_dir / "ciphertext.cms.download"
        verification = run_dir / ("verify-" + uuid.uuid4().hex)
        verification.mkdir(mode=0o700)
        remote = REMOTE_ALIAS + ":" + receipt["ciphertext_path"]
        recovery.run_command(["scp", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
                              "-o", "UpdateHostKeys=no", remote, str(temporary)], runner=runner, timeout=600)
        os.replace(temporary, ciphertext)
        validate_downloaded_ciphertext(receipt, ciphertext)
        image = recovery.run_command(["docker", "image", "inspect", "--format", "{{.Id}} {{.Architecture}}", recovery.CRYPTO_IMAGE], runner=runner)
        if image.stdout.strip() != recovery.CRYPTO_IMAGE + " amd64":
            raise RecoveryError("local crypto image identity mismatch")
        input_dir = run_dir / "crypto-input"
        output_dir = verification / "crypto-output"
        input_dir.mkdir(mode=0o700)
        output_dir.mkdir(mode=0o700)
        os.link(ciphertext, input_dir / "ciphertext.cms")
        name = "holyarrs-metadata-offhost-" + uuid.uuid4().hex[:12]
        package = output_dir / "package.tar.gz"
        recovery.run_binary_to_capped_file(
            crypto_docker_args(name, PRIVATE_KEY, RECIPIENT, input_dir, output_dir),
            package, recovery.EXPANSION_CAP, timeout=600)
        extracted = verification / "extracted"
        package_manifest = verify_plaintext_package(package, extracted)
        verified = {"schema_version": 1, "status": "verified", "verified_at": recovery.iso(),
                    "run_name": receipt["run_name"], "ciphertext_bytes": receipt["ciphertext_bytes"],
                    "ciphertext_sha256": receipt["ciphertext_sha256"], "identities": receipt["identities"],
                    "all_tokens_revoked_on_restore": True}
        _cleanup_verification(extracted, package_manifest)
        package.unlink()
        (input_dir / "ciphertext.cms").unlink()
        input_dir.rmdir()
        output_dir.rmdir()
        verification.rmdir()
        recovery.atomic_json(run_dir / "verified-receipt.json", verified)
        apply_retention(destination, receipt["run_name"])
        recovery.atomic_json(destination / "latest.json", verified)
        recovery.atomic_json(destination / "status.json", recovery.read_status(destination / "latest.json", destination / "failure.json"))
        return verified
    except Exception as error:
        recovery.write_failure(destination / "failure.json", "offhost-verification", error)
        raise


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--destination", required=True, type=pathlib.Path)
    parser.add_argument("--status", action="store_true")
    args = parser.parse_args()
    try:
        destination = validate_destination(args.destination)
        if os.path.normcase(str(destination)) != os.path.normcase(str(DEFAULT_DESTINATION.absolute())):
            raise RecoveryError("destination does not match the fixed backup destination")
        if args.status:
            result = recovery.read_status(destination / "latest.json", destination / "failure.json")
            print(json.dumps(result, sort_keys=True))
            if result.get("stale"):
                raise SystemExit(2)
            return
        result = run_backup(destination)
        print(json.dumps(result, sort_keys=True))
    except Exception as error:
        message = str(error) if isinstance(error, RecoveryError) else recovery.sanitize_error(error)
        print(json.dumps({"schema_version": 1, "status": "failed", "error": message}, sort_keys=True))
        raise SystemExit(1)


if __name__ == "__main__":
    main()
