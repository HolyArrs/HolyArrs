"""Check candidate receipts against the exact source and artifact being reviewed."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys


COMMON = {"tests", "build", "secret_scan"}
DATA = {"consistent_backup", "disposable_restore", "forward_migration", "compatible_rollback"}
API = {"deprecation_window", "consumer_transition"}
HEX = re.compile(r"^[0-9a-f]{40}$")
SHA256 = re.compile(r"^[0-9a-f]{64}$")


def git(repo, *args):
    result = subprocess.run(["git", "-C", str(repo), *args], capture_output=True, text=True)
    if result.returncode:
        raise ValueError("the source or baseline is not available in the local repository")
    return result.stdout.strip()


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def routes(raw):
    doc = json.loads(raw)
    methods = {"get", "post", "put", "patch", "delete", "head", "options"}
    return {f"{method.upper()} {path}" for path, ops in doc["paths"].items()
            for method in ops if method in methods}


def check(manifest, repo, artifact, evidence_dir, expected_baseline,
          baseline_artifact=None, state_snapshot=None):
    if (not isinstance(manifest, dict) or manifest.get("format") != "holyarrs-rc"
            or type(manifest.get("version")) is not int or manifest["version"] != 1):
        raise ValueError("unsupported candidate manifest")
    source = manifest.get("source", "")
    baseline = manifest.get("baseline", "")
    if not isinstance(source, str) or not HEX.fullmatch(source):
        raise ValueError("source must be a full commit id")
    if not isinstance(baseline, str) or not HEX.fullmatch(baseline):
        raise ValueError("baseline must be a full commit id")
    if baseline != expected_baseline or baseline == source:
        raise ValueError("baseline must match the independently selected prior-release commit")
    if git(repo, "rev-parse", "HEAD") != source:
        raise ValueError("source does not match the frozen checkout")
    if git(repo, "status", "--porcelain", "--untracked-files=all"):
        raise ValueError("source changed after the candidate was frozen")
    git(repo, "merge-base", "--is-ancestor", baseline, source)
    actual_artifact = digest(artifact)
    if manifest.get("artifact_sha256") != actual_artifact:
        raise ValueError("artifact digest does not match")
    declared_data = manifest.get("stored_data_change")
    if type(declared_data) is not bool:
        raise ValueError("stored_data_change must explicitly be true or false after format review")
    data_change = declared_data or bool(git(repo, "diff", "--name-only", baseline, source,
                                           "--", "app/backend/internal/core/database/migrations"))
    old_routes = routes(git(repo, "show", f"{baseline}:app/backend/cmd/holyarrs/openapi.json"))
    new_routes = routes(git(repo, "show", f"{source}:app/backend/cmd/holyarrs/openapi.json"))
    removed = sorted(old_routes - new_routes)
    users = manifest.get("users_on_1_0")
    if type(users) is not bool:
        raise ValueError("users_on_1_0 must explicitly be true or false")
    required = COMMON | (DATA if data_change else set()) | (API if removed else set())
    rc_required = users and (data_change or bool(removed))
    if rc_required:
        required |= {"candidate_trial"}
    data_bindings = {}
    if data_change:
        for field, path in (("baseline_artifact_sha256", baseline_artifact),
                            ("state_snapshot_sha256", state_snapshot)):
            if path is None or path.is_symlink() or not path.is_file():
                raise ValueError(f"missing or unsafe data input: {field}")
            actual = digest(path)
            if manifest.get(field) != actual:
                raise ValueError(f"data input digest does not match: {field}")
            data_bindings[field] = actual
    records = manifest.get("checks")
    if not isinstance(records, dict):
        raise ValueError("candidate check receipts are missing")
    for name in sorted(required):
        record = records.get(name)
        if not isinstance(record, dict) or record.get("passed") is not True:
            raise ValueError(f"missing or failed candidate check: {name}")
        if record.get("source") != source or record.get("artifact_sha256") != actual_artifact:
            raise ValueError(f"stale source or artifact receipt: {name}")
        if record.get("baseline") != baseline:
            raise ValueError(f"stale baseline receipt: {name}")
        if name in DATA and any(record.get(field) != value for field, value in data_bindings.items()):
            raise ValueError(f"stale backup or prior artifact receipt: {name}")
        filename = record.get("evidence", "")
        # A receipt is one file in the reviewed directory, never an absolute or escaped path.
        if not isinstance(filename, str) or not re.fullmatch(r"[a-z0-9][a-z0-9._-]{0,119}", filename):
            raise ValueError(f"invalid evidence filename: {name}")
        path = evidence_dir / filename
        if path.is_symlink() or not path.is_file() or path.resolve().parent != evidence_dir.resolve():
            raise ValueError(f"missing or unsafe evidence file: {name}")
        expected = record.get("evidence_sha256", "")
        if not isinstance(expected, str) or not SHA256.fullmatch(expected) or digest(path) != expected:
            raise ValueError(f"changed evidence file: {name}")
    return {"source": source, "artifact_sha256": actual_artifact, "stored_data_change": data_change,
            "removed_routes": removed, "rc_required": rc_required,
            "checks_verified": sorted(required), **data_bindings, "result": "receipts_match"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--artifact", type=Path, required=True)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--baseline-ref", required=True,
                        help="independently selected prior-release ref; cannot come from the manifest")
    parser.add_argument("--baseline-artifact", type=Path,
                        help="compatible prior artifact; required for stored-data changes")
    parser.add_argument("--state-snapshot", type=Path,
                        help="consistent state snapshot archive; required for stored-data changes")
    args = parser.parse_args()
    try:
        if args.manifest.stat().st_size > 256 * 1024:
            raise ValueError("candidate manifest is too large")
        baseline = git(args.repo, "rev-parse", "--verify", args.baseline_ref + "^{commit}")
        result = check(json.loads(args.manifest.read_text(encoding="utf-8")),
                       args.repo, args.artifact, args.evidence_dir, baseline,
                       args.baseline_artifact, args.state_snapshot)
        print(json.dumps(result, sort_keys=True))
        return 0
    except (ValueError, OSError, KeyError, TypeError) as error:
        # Provider output and local paths do not belong in a public candidate receipt.
        message = str(error) if isinstance(error, ValueError) else "candidate input could not be read"
        print(f"candidate rejected: {message}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
