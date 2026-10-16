#!/usr/bin/env python3
"""Prepare and verify isolated Pi-like ARM and NAS filesystem simulations.

This is a fail-closed orchestration surface.  ``prepare-arm`` and
``prepare-nas`` write immutable plans without starting Docker.  A later
runtime owner may execute the recorded commands and use ``verify-arm`` or
``verify-nas`` to validate a sanitized receipt.  The verifier deliberately
does not convert emulation into native Raspberry Pi or NAS acceptance.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import math
import os
from pathlib import Path, PurePosixPath
import re
import signal
import subprocess
import socket
import stat
import tarfile
import tempfile
import threading
import time
from typing import Any
import urllib.error
import urllib.request
import http.cookiejar


HEX40 = re.compile(r"^[0-9a-f]{40}$")
HEX64 = re.compile(r"^[0-9a-f]{64}$")
IMAGE_ID = re.compile(r"^sha256:[0-9a-f]{64}$")
RUN_ID = re.compile(r"^[a-z0-9][a-z0-9-]{7,47}$")
ARM = {
    "arm64": {"platform": "linux/arm64", "image_arch": "arm64", "variant": None},
    "armv7": {"platform": "linux/arm/v7", "image_arch": "arm", "variant": "v7"},
}
IDLE = {
    "30m": {"seconds": 1800, "timeout": "75m", "claim": "scale-acceptance"},
    "24h": {"seconds": 86400, "timeout": "26h", "claim": "emulated-soak"},
}
ARM_LIMITS = {
    "cpus": 4,
    "memory_bytes": 1073741824,
    "memory_swap_bytes": 1073741824,
    "pids_limit": 256,
    "gomemlimit": "384MiB",
    "network": "none",
    "user": "1000:1000",
    "read_only_rootfs": True,
    "tmpfs": "/tmp:size=512m,mode=1777",
}
ORDINARY_LIMITS = {
    "cpus": 1,
    "memory_bytes": 2147483648,
    "memory_swap_bytes": 2147483648,
    "pids_limit": 256,
    "network": "internal task-owned bridge",
    "user": "1000:1000",
}
ARM_SHAPE = {
    "name": "combined",
    "titles": 10000,
    "items": 55000,
    "feeds": 20,
    "rescan_folders": 2,
    "rescan_files": 29000,
    "rescan_found": 2,
    "rescan_unreachable": 0,
    "live_clients": 10,
    "live_rescan_events": 10,
}
NAS_SCENARIOS = [
    "fresh-owner-setup",
    "uid-1000-config-write",
    "synthetic-data-read-only",
    "restart-persistence",
    "candidate-update-preservation",
    "stopped-config-backup",
    "disposable-restore",
    "interrupted-copy-recovery",
    "empty-data-mount-recovery",
    "same-filesystem-hardlink",
    "split-filesystem-copy-fallback",
    "local-config-network-data",
    "network-config-refused",
    "owned-resource-cleanup",
]
NAS_EXPECTED = {name: "passed" for name in NAS_SCENARIOS}
NAS_EXPECTED["network-config-refused"] = "source-contract"
NAS_EXPECTED["local-config-network-data"] = "not-covered"
QEMU_TOOL_IMAGE_ID = "sha256:895ab9d0de0df91dcaf353e795e48b6147ac6323481d4e6cbac578b7a04eb606"
QEMU_GO_VERSION = "go version go1.27.2 linux/amd64"
QEMU_GRAPH_LIMIT = 32 * 1024 * 1024
QEMU_GRAPH_OBJECT_LIMIT = 4096
QEMU_CACHE_FILE_LIMIT = 256 * 1024 * 1024
QEMU_CACHE_TOTAL_LIMIT = 1024 * 1024 * 1024
QEMU_CACHE_ARCHIVE_LIMIT = QEMU_CACHE_TOTAL_LIMIT + 64 * 1024 * 1024
QEMU_CACHE_MANIFEST_LIMIT = 16 * 1024 * 1024
QEMU_GRAPH_KEYS = {"schema", "source_commit", "source_tree", "go_mod_sha256_raw",
                   "go_sum_sha256_raw", "target", "target_import_path", "tool", "variants"}
QEMU_GRAPH_VARIANT_KEYS = {"command", "environment", "exit_code", "package_log", "modules"}


class ContractError(RuntimeError):
    pass


MAX_OUTPUT = 2 * 1024 * 1024
DISCOVERY_SECONDS = 2.0
DISCOVERY_INTERVAL = 0.05


def _terminate_tree(process: subprocess.Popen[bytes]) -> None:
    """Terminate the whole command tree, then prove the direct child exited."""
    if process.poll() is not None:
        return
    if os.name == "nt":
        try:
            subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"],
                           stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=5, check=False)
        except (OSError, subprocess.TimeoutExpired):
            process.kill()
    else:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired as exc:
        process.kill()
        process.wait(timeout=5)
        raise ContractError("bounded command tree did not terminate cleanly") from exc


def _capture_bounded(stream: Any, retained: bytearray, exceeded: threading.Event) -> None:
    """Drain a pipe while retaining no more than MAX_OUTPUT bytes."""
    while True:
        chunk = stream.read(65536)
        if not chunk:
            return
        remaining = MAX_OUTPUT - len(retained)
        if remaining > 0:
            retained.extend(chunk[:remaining])
        if len(chunk) > remaining:
            exceeded.set()


def run_limited(command: list[str], timeout: int = 60) -> bytes:
    """Run with streaming bounded output, timeout and process-tree termination."""
    creationflags = getattr(subprocess, "CREATE_NO_WINDOW", 0)
    if os.name == "nt":
        creationflags |= getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
    try:
        process = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, creationflags=creationflags,
                                   start_new_session=os.name != "nt")
    except OSError as exc:
        raise ContractError(f"bounded command could not start: {Path(command[0]).name}") from exc
    if process.stdout is None:
        _terminate_tree(process)
        raise ContractError("bounded command output pipe was unavailable")
    retained = bytearray()
    exceeded = threading.Event()
    reader = threading.Thread(target=_capture_bounded, args=(process.stdout, retained, exceeded), daemon=True)
    reader.start()
    deadline = time.monotonic() + timeout
    timed_out = False
    try:
        while process.poll() is None and not exceeded.is_set() and time.monotonic() < deadline:
            time.sleep(0.01)
        if process.poll() is None:
            timed_out = not exceeded.is_set()
            _terminate_tree(process)
        reader.join(timeout=5)
        if reader.is_alive():
            raise ContractError("bounded command output reader did not terminate")
        if exceeded.is_set():
            raise ContractError(f"bounded command exceeded output limit: {Path(command[0]).name}")
        if timed_out:
            raise ContractError(f"bounded command exceeded time limit: {Path(command[0]).name}")
        if process.returncode:
            raise ContractError(f"bounded command failed without exposing output: {Path(command[0]).name}")
        return bytes(retained)
    finally:
        if process.poll() is None:
            _terminate_tree(process)
        process.stdout.close()


def docker_json(*args: str) -> Any:
    try:
        return json.loads(run_limited(["docker", *args]).decode("utf-8"))
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError("Docker returned invalid JSON") from exc


def atomic_replace_json(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = json.dumps(value, indent=2, sort_keys=True).encode("utf-8") + b"\n"
    fd, temporary = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(raw)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def require_bound_file(path: Path, expected: str, label: str) -> None:
    if not HEX64.fullmatch(expected):
        raise ContractError(f"{label} SHA-256 is invalid")
    if not path.is_file() or path.is_symlink() or sha256_file(path) != expected:
        raise ContractError(f"{label} is missing, linked, or changed")


def require_workspace_evidence(workspace: Path, value: Any, expected: Any, label: str) -> Path:
    if not isinstance(value, str) or not value or PurePosixPath(value).is_absolute() or ".." in PurePosixPath(value).parts:
        raise ContractError(f"{label} path is invalid")
    path = workspace.joinpath(*PurePosixPath(value).parts)
    try:
        path.resolve().relative_to(workspace.resolve())
    except ValueError as exc:
        raise ContractError(f"{label} escapes the workspace") from exc
    require_bound_file(path, str(expected), label)
    return path


def require_child_log(receipt_path: Path, case: dict[str, Any], filename: str,
                      classification: str) -> None:
    if case.get("log_path") != filename:
        raise ContractError("ARM child-proof log path differs")
    size = case.get("log_bytes")
    if not isinstance(size, int) or isinstance(size, bool) or not 1 <= size <= 1048576:
        raise ContractError("ARM child-proof log byte count is invalid")
    path = receipt_path.parent / filename
    try:
        info = path.lstat()
    except OSError as exc:
        raise ContractError("ARM child-proof log is missing") from exc
    attributes = getattr(info, "st_file_attributes", 0)
    reparse = getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0)
    if not stat.S_ISREG(info.st_mode) or path.is_symlink() or (reparse and attributes & reparse):
        raise ContractError("ARM child-proof log is not a safe regular file")
    raw = path.read_bytes()
    if len(raw) != size or hashlib.sha256(raw).hexdigest() != case.get("log_sha256_raw"):
        raise ContractError("ARM child-proof log length or hash differs")
    text = raw.decode("utf-8", errors="replace")
    passed = "--- PASS: TestTestCommandReexec" in text and re.search(r"(?m)^PASS\s*$", text) is not None
    if classification == "pass" and not passed:
        raise ContractError("ARM child-proof log lacks the exact PASS evidence")
    if classification == "exec-format" and "exec format error" not in text.lower():
        raise ContractError("ARM child-proof log lacks an exec-format observation")
    if classification == "not-found" and ("/holyarrs-nonexistent-executor" not in text
                                             or not re.search(r"not found|no such file", text, re.I)):
        raise ContractError("ARM child-proof log lacks the configured not-found observation")


def load_bound(path: Path, expected: str, label: str) -> dict[str, Any]:
    require_bound_file(path, expected, label)
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError(f"{label} is not UTF-8 JSON") from exc
    if not isinstance(value, dict):
        raise ContractError(f"{label} root must be an object")
    return value


def atomic_new_json(path: Path, value: dict[str, Any]) -> None:
    if path.exists():
        raise ContractError("output already exists")
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = json.dumps(value, indent=2, sort_keys=True).encode("utf-8") + b"\n"
    fd, temporary = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(raw)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass


def atomic_new_bytes(path: Path, raw: bytes) -> None:
    if path.exists():
        raise ContractError("output already exists")
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(raw)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass


def relative_artifact(path: Path, root: Path) -> dict[str, Any]:
    resolved = path.resolve()
    try:
        relative = resolved.relative_to(root.resolve()).as_posix()
    except ValueError as exc:
        raise ContractError("bound input must stay within the workspace") from exc
    return {"path": relative, "bytes": path.stat().st_size, "sha256": sha256_file(path)}


def manifest_file_hash(path: Path, target: str) -> str:
    try:
        entries = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError("source manifest is not UTF-8 JSON") from exc
    if not isinstance(entries, list):
        raise ContractError("source manifest root must be an array")
    matches = [entry for entry in entries if isinstance(entry, dict) and entry.get("path") == target]
    if len(matches) != 1:
        raise ContractError("source manifest does not bind the child-proof helper")
    value = matches[0].get("sha256") or matches[0].get("sha256_raw")
    if not HEX64.fullmatch(str(value)):
        raise ContractError("source manifest helper hash is invalid")
    return value


def arm_create_command(plan: dict[str, Any]) -> list[str]:
    limits = ARM_LIMITS
    idle = plan["idle"]
    return [
        "docker", "create", "--pull=never", "--name", plan["container_name"],
        "--label", f"io.holyarrs.device-simulation={plan['run_id']}",
        "--platform", plan["platform"], "--network", "none",
        "--memory", "1g", "--memory-swap", "1g", "--cpus", "4",
        "--pids-limit", str(limits["pids_limit"]), "--read-only",
        "--tmpfs", limits["tmpfs"], "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges", "--user", limits["user"],
        "--env", "GOMEMLIMIT=384MiB", "--env", "HOLYARRS_SCALE=combined",
        "--env", f"HOLYARRS_SCALE_IDLE={idle}", plan["image_id"],
        "-test.run=^TestScaleAcceptance$", "-test.v", "-test.count=1",
        f"-test.timeout={IDLE[idle]['timeout']}",
    ]


def _qemu_module_pair(path: Any, version: Any) -> tuple[str, str]:
    if (not isinstance(path, str) or not isinstance(version, str)
            or not path.startswith(("github.com/", "golang.org/", "modernc.org/"))
            or any(value in path for value in ("\\", "..", "@", "!"))
            or not re.fullmatch(r"[A-Za-z0-9._~+-]+(?:/[A-Za-z0-9._~+-]+)+", path)
            or not re.fullmatch(r"v[0-9A-Za-z.+_-]+", version)):
        raise ContractError("ARM target graph module identity is invalid")
    return path, version


def _qemu_graph_environment(variant: str) -> dict[str, str]:
    values = {
        "GOMAXPROCS": "1", "GOFLAGS": "-p=1 -mod=readonly", "GOMEMLIMIT": "768MiB",
        "GOGC": "50", "CGO_ENABLED": "0", "GOOS": "linux",
        "GOARCH": "arm64" if variant == "linux-arm64" else "arm",
        "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOMODCACHE": "/tmp/gomodcache",
    }
    if variant == "linux-arm-v7":
        values["GOARM"] = "7"
    return values


def _qemu_graph_log(manifest: Path, workspace: Path, record: Any, variant: str) -> bytes:
    if (not isinstance(record, dict) or set(record) != {"path", "bytes", "sha256_raw"}
            or not isinstance(record.get("path"), str)
            or PurePosixPath(record["path"]).is_absolute() or ".." in PurePosixPath(record["path"]).parts
            or not isinstance(record.get("bytes"), int) or isinstance(record.get("bytes"), bool)
            or not 1 <= record["bytes"] <= QEMU_GRAPH_LIMIT
            or not HEX64.fullmatch(str(record.get("sha256_raw")))):
        raise ContractError(f"ARM {variant} package log record is invalid")
    path = manifest.parent.joinpath(*PurePosixPath(record["path"]).parts)
    raw, _ = _qemu_read(path, record["sha256_raw"], workspace,
                        f"ARM {variant} package log", QEMU_GRAPH_LIMIT)
    if len(raw) != record["bytes"]:
        raise ContractError(f"ARM {variant} package log length differs")
    return raw


def _qemu_reparse(path: Path, info: os.stat_result) -> bool:
    return bool(getattr(info, "st_file_attributes", 0)
                & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))


def _qemu_path(path: Path, workspace: Path, label: str) -> tuple[Path, os.stat_result]:
    root = Path(os.path.abspath(workspace))
    target = Path(os.path.abspath(path))
    try:
        relative = target.relative_to(root)
    except ValueError as exc:
        raise ContractError(f"{label} is outside the workspace") from exc
    components = [root]
    current = root
    for part in relative.parts:
        current /= part
        components.append(current)
    leaf = None
    try:
        for index, component in enumerate(components):
            info = component.lstat()
            if component.is_symlink() or _qemu_reparse(component, info):
                raise ContractError(f"{label} has a linked or reparse ancestor")
            if index < len(components) - 1 and not stat.S_ISDIR(info.st_mode):
                raise ContractError(f"{label} ancestor is not a directory")
            leaf = info
    except OSError as exc:
        raise ContractError(f"{label} is unavailable") from exc
    if leaf is None or not stat.S_ISREG(leaf.st_mode):
        raise ContractError(f"{label} is not a regular file")
    return target, leaf


def _qemu_same_file(first: os.stat_result, second: os.stat_result, label: str) -> None:
    first_id = (getattr(first, "st_dev", 0), getattr(first, "st_ino", 0))
    second_id = (getattr(second, "st_dev", 0), getattr(second, "st_ino", 0))
    if not all(first_id) or not all(second_id):
        raise ContractError(f"{label} file identity is unavailable")
    if first_id != second_id:
        raise ContractError(f"{label} changed while opening")


def _qemu_read(path: Path, expected: str, workspace: Path, label: str,
               maximum: int) -> tuple[bytes, Path]:
    if not HEX64.fullmatch(expected):
        raise ContractError(f"{label} SHA-256 is invalid")
    target, before = _qemu_path(path, workspace, label)
    if before.st_size > maximum:
        raise ContractError(f"{label} is not a bounded regular owned file")
    try:
        with target.open("rb") as handle:
            opened = os.fstat(handle.fileno())
            _qemu_same_file(before, opened, label)
            if not stat.S_ISREG(opened.st_mode) or opened.st_size > maximum:
                raise ContractError(f"{label} opened as an invalid file")
            raw = handle.read(maximum + 1)
    except OSError as exc:
        raise ContractError(f"{label} became unreadable") from exc
    if len(raw) > maximum or hashlib.sha256(raw).hexdigest() != expected:
        raise ContractError(f"{label} is changed")
    after_target, after = _qemu_path(path, workspace, label)
    _qemu_same_file(opened, after, label)
    if after_target != target:
        raise ContractError(f"{label} path changed")
    return raw, target


def _qemu_artifact(path: Path, workspace: Path, raw_size: int, digest: str) -> dict[str, Any]:
    target = Path(os.path.abspath(path))
    root = Path(os.path.abspath(workspace))
    return {"path": target.relative_to(root).as_posix(), "bytes": raw_size, "sha256": digest}


def _qemu_package_modules(raw: bytes, expected_import: str) -> list[tuple[str, str]]:
    try:
        text = raw.decode("utf-8", errors="strict")
    except UnicodeDecodeError as exc:
        raise ContractError("ARM package graph is not UTF-8") from exc
    decoder = json.JSONDecoder()
    offset = 0
    objects = 0
    packages: set[str] = set()
    modules: set[tuple[str, str]] = set()
    target_found = False
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset == len(text):
            break
        try:
            item, offset = decoder.raw_decode(text, offset)
        except json.JSONDecodeError as exc:
            raise ContractError("ARM package graph contains malformed JSON") from exc
        objects += 1
        if objects > QEMU_GRAPH_OBJECT_LIMIT or not isinstance(item, dict):
            raise ContractError("ARM package graph object set is invalid")
        import_path = item.get("ImportPath")
        if not isinstance(import_path, str) or not import_path or import_path in packages:
            raise ContractError("ARM package graph package identity is invalid")
        packages.add(import_path)
        if item.get("Incomplete") or "Error" in item or "DepsErrors" in item:
            raise ContractError("ARM package graph contains diagnostic errors")
        module = item.get("Module")
        if item.get("Standard") is True:
            if module is not None:
                raise ContractError("ARM standard package contains module metadata")
        elif not isinstance(module, dict) or "Error" in module or "Replace" in module:
            raise ContractError("ARM package graph module metadata is invalid")
        elif module.get("Main") is not True:
            modules.add(_qemu_module_pair(module.get("Path"), module.get("Version")))
        if import_path == expected_import:
            files = item.get("TestGoFiles")
            main_path = expected_import.removesuffix("/cmd/holyarrs")
            if (target_found or not isinstance(files, list)
                    or any(not isinstance(value, str) for value in files)
                    or "testexec_test.go" not in files or module.get("Main") is not True
                    or module.get("Path") != main_path):
                raise ContractError("ARM package graph base target is invalid")
            target_found = True
    if not objects or not target_found or not modules:
        raise ContractError("ARM package graph omits the base target or module closure")
    return sorted(modules)


def _load_qemu_target_graph(args: argparse.Namespace, actual_commit: str, actual_tree: str,
                            source_manifest: Path) -> dict[str, Any]:
    raw, manifest_path = _qemu_read(
        args.target_graph_manifest, args.target_graph_manifest_sha256, args.workspace,
        "ARM target graph manifest", QEMU_CACHE_MANIFEST_LIMIT)
    try:
        graph = json.loads(raw)
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError("ARM target graph manifest is not UTF-8 JSON") from exc
    if (not isinstance(graph, dict) or set(graph) != QEMU_GRAPH_KEYS
            or graph.get("schema") != "holyarrs.go-target-graph.v1"
            or graph.get("source_commit") != actual_commit or graph.get("source_tree") != actual_tree
            or graph.get("go_mod_sha256_raw") != manifest_file_hash(source_manifest, "go.mod")
            or graph.get("go_sum_sha256_raw") != manifest_file_hash(source_manifest, "go.sum")
            or graph.get("target") != "./cmd/holyarrs"):
        raise ContractError("ARM target graph source or field set differs")
    target_import = graph.get("target_import_path")
    tool = graph.get("tool")
    if (not isinstance(target_import, str) or not target_import.endswith("/cmd/holyarrs")
            or not isinstance(tool, dict) or set(tool) != {"image_id", "go_version"}
            or tool != {"image_id": QEMU_TOOL_IMAGE_ID, "go_version": QEMU_GO_VERSION}):
        raise ContractError("ARM target graph target or tool differs")
    variants = graph.get("variants")
    if not isinstance(variants, dict) or set(variants) != {"linux-arm64", "linux-arm-v7"}:
        raise ContractError("ARM target graph variant set differs")
    loaded = {}
    union: set[tuple[str, str]] = set()
    expected_command = ["go", "list", "-mod=readonly", "-deps", "-test", "-json", "./cmd/holyarrs"]
    for variant in ("linux-arm64", "linux-arm-v7"):
        value = variants[variant]
        if (not isinstance(value, dict) or set(value) != QEMU_GRAPH_VARIANT_KEYS
                or value.get("command") != expected_command
                or value.get("environment") != _qemu_graph_environment(variant)
                or value.get("exit_code") != 0 or not isinstance(value.get("modules"), list)
                or not value["modules"]):
            raise ContractError("ARM target graph variant contract differs")
        pairs = []
        for item in value["modules"]:
            if not isinstance(item, dict) or set(item) != {"path", "version"}:
                raise ContractError("ARM target graph module record differs")
            pairs.append(_qemu_module_pair(item.get("path"), item.get("version")))
        if pairs != sorted(set(pairs)):
            raise ContractError("ARM target graph modules are not unique and ordered")
        package_raw = _qemu_graph_log(manifest_path, args.workspace,
                              value.get("package_log"), variant)
        if _qemu_package_modules(package_raw, target_import) != pairs:
            raise ContractError("ARM target graph modules differ from retained package JSON")
        loaded[variant] = {"modules": pairs, "package_log": value["package_log"]}
        union.update(pairs)
    return {"schema": graph["schema"], "target": graph["target"],
            "target_import_path": target_import, "tool": tool, "variants": loaded,
            "module_union": sorted(union), "manifest_sha256_raw": args.target_graph_manifest_sha256,
            "artifact": _qemu_artifact(manifest_path, args.workspace, len(raw),
                                       args.target_graph_manifest_sha256)}


def _qemu_cache_component(value: str) -> str:
    output = []
    for character in value:
        if "A" <= character <= "Z":
            output.append("!" + character.lower())
        elif character.isascii() and (character.isalnum() or character in "-._~/+"):
            output.append(character)
        else:
            raise ContractError("ARM module cache path requires unsupported escaping")
    return "".join(output)


def _verify_qemu_cache(args: argparse.Namespace, graph: dict[str, Any], actual_commit: str,
                       actual_tree: str, source_manifest: Path) -> dict[str, Any]:
    manifest_raw, manifest_path = _qemu_read(
        args.module_cache_manifest, args.module_cache_manifest_sha256, args.workspace,
        "ARM module cache manifest", QEMU_CACHE_MANIFEST_LIMIT)
    try:
        manifest = json.loads(manifest_raw)
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError("ARM module cache manifest is not UTF-8 JSON") from exc
    keys = {"schema", "source_commit", "source_tree", "go_mod_sha256_raw",
            "go_sum_sha256_raw", "target_graph_sha256_raw", "archive_sha256_raw", "files"}
    union = graph["module_union"]
    if (set(manifest) != keys or manifest.get("schema") != "holyarrs.go-target-cache.v2"
            or manifest.get("source_commit") != actual_commit or manifest.get("source_tree") != actual_tree
            or manifest.get("go_mod_sha256_raw") != manifest_file_hash(source_manifest, "go.mod")
            or manifest.get("go_sum_sha256_raw") != manifest_file_hash(source_manifest, "go.sum")
            or manifest.get("target_graph_sha256_raw") != graph["manifest_sha256_raw"]
            or manifest.get("archive_sha256_raw") != args.module_cache_archive_sha256
            or not isinstance(manifest.get("files"), list)):
        raise ContractError("ARM module cache source or field set differs")
    expected = set()
    for path, version in union:
        prefix = f"cache/download/{_qemu_cache_component(path)}/@v/{_qemu_cache_component(version)}"
        expected.update(prefix + suffix for suffix in (".info", ".mod", ".zip", ".ziphash"))
    declared = {}
    previous = None
    for item in manifest["files"]:
        if (not isinstance(item, dict) or set(item) != {"path", "bytes", "sha256_raw"}
                or not isinstance(item.get("path"), str) or item["path"] in declared
                or (previous is not None and item["path"] <= previous)
                or not isinstance(item.get("bytes"), int) or isinstance(item.get("bytes"), bool)
                or not 0 <= item["bytes"] <= QEMU_CACHE_FILE_LIMIT
                or not HEX64.fullmatch(str(item.get("sha256_raw")))):
            raise ContractError("ARM module cache file manifest differs")
        declared[item["path"]] = item
        previous = item["path"]
    if set(declared) != expected or len(declared) != len(union) * 4:
        raise ContractError("ARM module cache does not contain exactly four files per union pair")
    found = set()
    total = 0
    archive_path, archive_before = _qemu_path(
        args.module_cache_archive, args.workspace, "ARM module cache archive")
    if archive_before.st_size > QEMU_CACHE_ARCHIVE_LIMIT:
        raise ContractError("ARM module cache archive is oversized")
    try:
        with archive_path.open("rb") as archive_handle:
            opened = os.fstat(archive_handle.fileno())
            _qemu_same_file(archive_before, opened, "ARM module cache archive")
            digest = hashlib.sha256()
            archive_bytes = 0
            while chunk := archive_handle.read(1024 * 1024):
                digest.update(chunk)
                archive_bytes += len(chunk)
                if archive_bytes > QEMU_CACHE_ARCHIVE_LIMIT:
                    raise ContractError("ARM module cache archive is oversized")
            if digest.hexdigest() != args.module_cache_archive_sha256:
                raise ContractError("ARM module cache archive is changed")
            _qemu_path(args.module_cache_archive, args.workspace, "ARM module cache archive")
            archive_handle.seek(0)
            with tarfile.open(fileobj=archive_handle, mode="r:") as archive:
                for member in archive:
                    name = member.name
                    item = declared.get(name)
                    if (not member.isfile() or member.issym() or member.islnk() or member.isdev()
                            or name in found or item is None or PurePosixPath(name).is_absolute()
                            or ".." in PurePosixPath(name).parts):
                        raise ContractError("ARM module cache archive membership differs")
                    handle = archive.extractfile(member)
                    if handle is None or member.size != item["bytes"]:
                        raise ContractError("ARM module cache archive content differs")
                    member_digest = hashlib.sha256()
                    size = 0
                    while chunk := handle.read(1024 * 1024):
                        member_digest.update(chunk)
                        size += len(chunk)
                    if size != item["bytes"] or member_digest.hexdigest() != item["sha256_raw"]:
                        raise ContractError("ARM module cache archive content differs")
                    total += size
                    if total > QEMU_CACHE_TOTAL_LIMIT:
                        raise ContractError("ARM module cache archive exceeds its total bound")
                    found.add(name)
            archive_handle.seek(0)
            final_digest = hashlib.sha256()
            while chunk := archive_handle.read(1024 * 1024):
                final_digest.update(chunk)
            if final_digest.hexdigest() != args.module_cache_archive_sha256:
                raise ContractError("ARM module cache archive changed while reading")
            _, archive_after = _qemu_path(
                args.module_cache_archive, args.workspace, "ARM module cache archive")
            _qemu_same_file(opened, archive_after, "ARM module cache archive")
    except (OSError, tarfile.TarError) as exc:
        raise ContractError("ARM module cache archive is invalid") from exc
    if found != expected:
        raise ContractError("ARM module cache archive is incomplete")
    return {"schema": manifest["schema"], "files": len(found), "bytes": total,
            "required_modules": len(union), "archive_sha256_raw": args.module_cache_archive_sha256,
            "manifest_sha256_raw": args.module_cache_manifest_sha256,
            "manifest_artifact": _qemu_artifact(manifest_path, args.workspace, len(manifest_raw),
                                                args.module_cache_manifest_sha256),
            "archive_artifact": _qemu_artifact(archive_path, args.workspace, archive_bytes,
                                               args.module_cache_archive_sha256)}


def _verify_qemu_container_states(args: argparse.Namespace, actual_commit: str,
                                  variant: str) -> dict[str, Any]:
    manifest_raw, manifest_path = _qemu_read(
        args.container_states_manifest, args.container_states_manifest_sha256,
        args.workspace, "ARM container states manifest", QEMU_CACHE_MANIFEST_LIMIT)
    try:
        manifest = json.loads(manifest_raw)
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise ContractError("ARM container states manifest is not UTF-8 JSON") from exc
    keys = {"schema", "source_commit", "variant", "preparation_receipt_sha256_raw", "files"}
    if (set(manifest) != keys or manifest.get("schema") != "holyarrs.qemu-container-states.v1"
            or manifest.get("source_commit") != actual_commit or manifest.get("variant") != variant
            or manifest.get("preparation_receipt_sha256_raw") != args.prepare_receipt_sha256
            or not isinstance(manifest.get("files"), list) or len(manifest["files"]) != 5):
        raise ContractError("ARM container states manifest source or field set differs")
    prefix = f"holyarrs-qemu-prep-{variant}-{actual_commit[:12]}-"
    expected_names = [prefix + suffix for suffix in
                      ("build", "extract", "missing", "nonexistent", "positive")]
    expected_paths = {f"container-{name}.json" for name in expected_names}
    identities: set[str] = set()
    names: set[str] = set()
    paths: set[str] = set()
    for record in manifest["files"]:
        if (not isinstance(record, dict)
                or set(record) != {"path", "bytes", "sha256_raw", "id", "name"}
                or not isinstance(record.get("path"), str) or record["path"] in paths
                or not isinstance(record.get("bytes"), int) or isinstance(record.get("bytes"), bool)
                or not 1 <= record["bytes"] <= 4096
                or not HEX64.fullmatch(str(record.get("sha256_raw")))
                or not HEX64.fullmatch(str(record.get("id"))) or record["id"] in identities
                or record.get("name") not in expected_names or record["name"] in names
                or record["path"] != f"container-{record['name']}.json"):
            raise ContractError("ARM container state record differs")
        path = manifest_path.parent / record["path"]
        raw, _ = _qemu_read(path, record["sha256_raw"], args.workspace,
                            "ARM container state", 4096)
        if len(raw) != record["bytes"]:
            raise ContractError("ARM container state byte count differs")
        try:
            value = json.loads(raw)
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise ContractError("ARM container state is not UTF-8 JSON") from exc
        if (not isinstance(value, dict) or set(value) != {"id", "name"}
                or value != {"id": record["id"], "name": record["name"]}):
            raise ContractError("ARM container state content differs")
        paths.add(record["path"])
        identities.add(record["id"])
        names.add(record["name"])
    if paths != expected_paths or names != set(expected_names):
        raise ContractError("ARM container state set differs")
    return {"schema": manifest["schema"], "files": len(paths),
            "manifest_sha256_raw": args.container_states_manifest_sha256,
            "artifact": _qemu_artifact(manifest_path, args.workspace, len(manifest_raw),
                                       args.container_states_manifest_sha256)}


def _qemu_receipt_record(value: Any, expected_hash: str, label: str) -> None:
    if (not isinstance(value, dict) or set(value) != {"path", "sha256_raw"}
            or not isinstance(value.get("path"), str) or not value["path"]
            or PurePosixPath(value["path"]).is_absolute() or ".." in PurePosixPath(value["path"]).parts
            or value.get("sha256_raw") != expected_hash):
        raise ContractError(f"{label} receipt record differs")


def _require_receipt_log(receipt: Path, value: Any, expected_name: str, label: str) -> None:
    if (not isinstance(value, dict) or set(value) != {"path", "sha256_raw"}
            or value.get("path") != expected_name):
        raise ContractError(f"{label} record differs")
    path = receipt.parent / expected_name
    _qemu_read(path, str(value.get("sha256_raw")), receipt.parent, label, QEMU_GRAPH_LIMIT)


def prepare_arm(args: argparse.Namespace) -> dict[str, Any]:
    if not HEX40.fullmatch(args.source_commit) or not HEX40.fullmatch(args.source_tree):
        raise ContractError("source commit and tree must be lowercase 40-hex")
    if not RUN_ID.fullmatch(args.run_id):
        raise ContractError("run ID is unsafe")
    require_bound_file(args.source_manifest, args.source_manifest_sha256, "source manifest")
    helper_path = "cmd/holyarrs/testexec_test.go"
    helper_source_hash = manifest_file_hash(args.source_manifest, helper_path)
    prepared = load_bound(args.prepare_receipt, args.prepare_receipt_sha256, "ARM preparation receipt")
    profile = ARM[args.architecture]
    variant = "linux-arm64" if args.architecture == "arm64" else "linux-arm-v7"
    if prepared.get("schema") != "holyarrs.qemu-build.v2" or prepared.get("variant") != variant:
        raise ContractError("ARM preparation receipt schema or variant differs")
    source = prepared.get("source") or {}
    actual_commit = source.get("commit")
    historical = actual_commit != args.source_commit
    if historical and not args.allow_historical_harness:
        raise ContractError("ARM preparation receipt is not bound to the intended source")
    actual_tree = source.get("tree")
    if (not HEX40.fullmatch(str(actual_commit)) or not HEX40.fullmatch(str(actual_tree))
            or (not historical and actual_tree != args.source_tree)
            or source.get("verified_tree_before") != actual_tree
            or source.get("verified_tree_after") != actual_tree
            or (source.get("manifest") or {}).get("sha256_raw") != args.source_manifest_sha256
            or not HEX64.fullmatch(str(source.get("archive_sha256_raw")))):
        raise ContractError("ARM preparation source binding differs")
    if (not HEX64.fullmatch(args.preparation_tool_sha256)
            or (prepared.get("preparation_tool") or {}).get("sha256_raw") != args.preparation_tool_sha256):
        raise ContractError("ARM preparation tool binding differs")
    graph = _load_qemu_target_graph(args, actual_commit, actual_tree, args.source_manifest)
    cache = _verify_qemu_cache(args, graph, actual_commit, actual_tree, args.source_manifest)
    states = _verify_qemu_container_states(args, actual_commit, variant)
    target_graph = prepared.get("target_graph")
    target_keys = {"schema", "manifest", "target", "target_import_path", "tool",
                   "variant_modules", "variant_package_log", "module_union_count",
                   "verified_before_and_after"}
    selected_modules = [{"path": path, "version": version}
                        for path, version in graph["variants"][variant]["modules"]]
    selected_log = graph["variants"][variant]["package_log"]
    if (not isinstance(target_graph, dict) or set(target_graph) != target_keys
            or target_graph.get("schema") != graph["schema"]
            or target_graph.get("target") != graph["target"]
            or target_graph.get("target_import_path") != graph["target_import_path"]
            or target_graph.get("tool") != graph["tool"]
            or target_graph.get("variant_modules") != selected_modules
            or target_graph.get("module_union_count") != len(graph["module_union"])
            or target_graph.get("verified_before_and_after") is not True):
        raise ContractError("ARM target graph receipt binding differs")
    _qemu_receipt_record(target_graph.get("manifest"), graph["manifest_sha256_raw"],
                         "ARM target graph")
    receipt_log = target_graph.get("variant_package_log")
    if (not isinstance(receipt_log, dict) or set(receipt_log) != {"path", "bytes", "sha256_raw"}
            or not isinstance(receipt_log.get("path"), str)
            or PurePosixPath(receipt_log["path"]).is_absolute()
            or ".." in PurePosixPath(receipt_log["path"]).parts
            or PurePosixPath(receipt_log["path"]).name != PurePosixPath(selected_log["path"]).name
            or receipt_log.get("bytes") != selected_log["bytes"]
            or receipt_log.get("sha256_raw") != selected_log["sha256_raw"]):
        raise ContractError("ARM selected package-log receipt binding differs")
    cache_receipt = prepared.get("module_cache")
    cache_keys = {"schema", "archive", "manifest", "files", "bytes", "required_modules",
                  "verified_before_and_after", "generated_inventory"}
    if (not isinstance(cache_receipt, dict) or set(cache_receipt) != cache_keys
            or cache_receipt.get("schema") != cache["schema"]
            or cache_receipt.get("files") != cache["files"]
            or cache_receipt.get("bytes") != cache["bytes"]
            or cache_receipt.get("required_modules") != cache["required_modules"]
            or cache_receipt.get("verified_before_and_after") is not True):
        raise ContractError("ARM module cache receipt binding differs")
    _qemu_receipt_record(cache_receipt.get("archive"), cache["archive_sha256_raw"],
                         "ARM module cache archive")
    _qemu_receipt_record(cache_receipt.get("manifest"), cache["manifest_sha256_raw"],
                         "ARM module cache manifest")
    build = prepared.get("build") or {}
    expected_env = _qemu_graph_environment(variant)
    if (build.get("command") != ["go", "test", "-mod=readonly", "-c", "-o",
                                  "holyarrs.test", "./cmd/holyarrs"]
            or build.get("environment") != expected_env or build.get("exit_code") != 0
            or build.get("go_version") != QEMU_GO_VERSION
            or not HEX64.fullmatch(str((build.get("log") or {}).get("sha256_raw")))):
        raise ContractError("ARM build evidence differs")
    expected_steps = {"go-version", "target-graph", "module-download",
                      "module-inventory", "cross-build"}
    steps = build.get("steps")
    if not isinstance(steps, dict) or set(steps) != expected_steps:
        raise ContractError("ARM build step evidence set differs")
    for step in sorted(expected_steps):
        record = steps[step]
        if (not isinstance(record, dict) or set(record) != {"exit_code", "log"}
                or record.get("exit_code") != 0):
            raise ContractError("ARM build step evidence differs")
        _require_receipt_log(args.prepare_receipt, record.get("log"), f"{step}.log",
                             f"ARM {step} log")
    if ((args.prepare_receipt.parent / "go-version.log").read_bytes()
            != (QEMU_GO_VERSION + "\n").encode("utf-8")):
        raise ContractError("ARM retained Go version log differs")
    retained_graph = (args.prepare_receipt.parent / "target-graph.log").read_bytes()
    if _qemu_package_modules(retained_graph, graph["target_import_path"]) != graph["variants"][variant]["modules"]:
        raise ContractError("ARM retained build target graph differs")
    if cache_receipt.get("generated_inventory") != steps["module-inventory"]:
        raise ContractError("ARM generated module inventory binding differs")
    binary, executor = prepared.get("test_binary") or {}, prepared.get("executor") or {}
    image, child = prepared.get("image") or {}, prepared.get("child_proof") or {}
    binding = child.get("binding") or {}
    missing = child.get("missing_executor") or {}
    nonexistent = child.get("nonexistent_executor") or {}
    explicit = child.get("explicit_executor") or {}
    test_args = ["-test.run=^TestTestCommandReexec$", "-test.v", "-test.count=1", "-test.timeout=2m"]
    commands = [missing.get("command"), nonexistent.get("command"), explicit.get("command")]
    missing_outcome = missing.get("outcome")
    missing_exit = missing.get("exit_code")
    missing_exit_valid = (isinstance(missing_exit, int) and not isinstance(missing_exit, bool)
                          and ((missing_outcome == "exec-format-reproduced" and missing_exit == 1)
                               or (missing_outcome == "implicit-execution-available" and missing_exit == 0)))
    expected_executor_path = "/qemu-aarch64-static" if args.architecture == "arm64" else "/qemu-arm-static"
    if (binary.get("path") != "holyarrs.test" or not HEX64.fullmatch(str(binary.get("sha256_raw")))
            or executor.get("mode") != "userspace-qemu" or executor.get("path") != expected_executor_path
            or not HEX64.fullmatch(str(executor.get("sha256_raw")))
            or not IMAGE_ID.fullmatch(str(image.get("tool_image_id")))
            or image.get("platform") != profile["platform"]
            or image.get("entrypoint") != [executor.get("path"), "/holyarrs.test"]
            or image.get("extracted_test_binary_sha256_raw") != binary.get("sha256_raw")
            or image.get("extracted_executor_sha256_raw") != executor.get("sha256_raw")
            or child.get("test") != "TestTestCommandReexec"
            or binding != {"image_id": image.get("test_image_id"),
                           "test_binary_sha256_raw": binary.get("sha256_raw"),
                           "source_helper_path": helper_path,
                           "source_helper_sha256_raw": helper_source_hash,
                           "test": "TestTestCommandReexec", "args": test_args,
                           "parent_entrypoint": image.get("entrypoint")}
            or not all(isinstance(command, list) and command
                       and all(isinstance(item, str) and item for item in command) for command in commands)
            or commands[0] != commands[1] or commands[1] != commands[2]
            or missing.get("environment") != {"HOLYARRS_TEST_EXEC": None}
            or not missing_exit_valid
            or nonexistent.get("environment") != {"HOLYARRS_TEST_EXEC": "/holyarrs-nonexistent-executor"}
            or not isinstance(nonexistent.get("exit_code"), int)
            or isinstance(nonexistent.get("exit_code"), bool) or nonexistent.get("exit_code") == 0
            or nonexistent.get("observed_path") != "/holyarrs-nonexistent-executor"
            or nonexistent.get("error_class") != "not-found"
            or explicit.get("environment") != {"HOLYARRS_TEST_EXEC": executor.get("path")}
            or not isinstance(explicit.get("exit_code"), int) or isinstance(explicit.get("exit_code"), bool)
            or explicit.get("exit_code") != 0 or explicit.get("result") != "PASS"
            or explicit.get("executor_sha256_raw") != executor.get("sha256_raw")
            or not HEX64.fullmatch(str(missing.get("log_sha256_raw")))
            or not HEX64.fullmatch(str(nonexistent.get("log_sha256_raw")))
            or not HEX64.fullmatch(str(explicit.get("log_sha256_raw")))):
        raise ContractError("ARM binary, userspace executor, image, or child proof differs")
    evidence = prepared.get("evidence")
    if (not isinstance(evidence, dict) or evidence.get("containers_cleaned") is not True
            or evidence.get("retained_image_id") != image.get("test_image_id")):
        raise ContractError("ARM producer cleanup or retained-image evidence differs")
    require_workspace_evidence(args.workspace, (prepared.get("preparation_tool") or {}).get("path"),
                               args.preparation_tool_sha256, "ARM preparation tool")
    _require_receipt_log(args.prepare_receipt, build.get("log"), "build.log", "ARM build log")
    _require_receipt_log(args.prepare_receipt, binary, "holyarrs.test", "ARM test binary")
    require_child_log(args.prepare_receipt, missing, "proof-missing-executor.log",
                      "pass" if missing_outcome == "implicit-execution-available" else "exec-format")
    require_child_log(args.prepare_receipt, nonexistent, "proof-nonexistent-executor.log", "not-found")
    require_child_log(args.prepare_receipt, explicit, "proof-explicit-executor.log", "pass")
    image_id = image.get("test_image_id")
    if not isinstance(image_id, str) or not IMAGE_ID.fullmatch(image_id):
        raise ContractError("ARM preparation receipt lacks an immutable local image ID")
    plan = {
        "schema": 1,
        "kind": "pi-like-arm-emulation",
        "claim_boundary": "QEMU userspace emulation; not native Raspberry Pi, thermal, power, storage, or hardware acceptance",
        "source_commit": actual_commit,
        "target_source_commit": args.source_commit,
        "source_tree": actual_tree,
        "target_source_tree": args.source_tree,
        "final_source_gate": not historical,
        "run_id": args.run_id,
        "architecture": args.architecture,
        "platform": profile["platform"],
        "execution_mode": "emulated",
        "idle": args.idle,
        "minimum_real_wall_seconds": IDLE[args.idle]["seconds"],
        "claim": "historical harness only; not final-source acceptance" if historical else IDLE[args.idle]["claim"],
        "image_id": image_id,
        "tool_image_id": image.get("tool_image_id"),
        "image_defaults": {"env": [], "entrypoint": image.get("entrypoint")},
        "preparation": {"receipt_sha256": args.prepare_receipt_sha256,
                        "tool_sha256_raw": args.preparation_tool_sha256,
                        "test_binary_sha256_raw": binary.get("sha256_raw"),
                        "executor_sha256_raw": executor.get("sha256_raw"),
                        "executor_mode": "userspace-qemu",
                        "target_graph_sha256_raw": graph["manifest_sha256_raw"],
                        "target_graph_module_union_count": len(graph["module_union"]),
                        "module_cache_manifest_sha256_raw": cache["manifest_sha256_raw"],
                        "module_cache_archive_sha256_raw": cache["archive_sha256_raw"],
                        "container_states_manifest_sha256_raw": states["manifest_sha256_raw"],
                        "container_state_files": states["files"],
                        "child_logs": [
                            {"path": case["log_path"], "bytes": case["log_bytes"],
                             "sha256_raw": case["log_sha256_raw"]}
                            for case in (missing, nonexistent, explicit)
                        ]},
        "container_name": f"holyarrs-device-{args.architecture}-{args.run_id}",
        "resources": ARM_LIMITS,
        "workload": ARM_SHAPE,
        "thresholds": {"idle_rss_max_bytes": 250000000, "peak_rss_max_bytes": 500000000,
                       "idle_writes": 0, "idle_write_bytes": 0,
                       "writer_wait_p95_ms_exclusive": 100, "writer_wait_min_samples": 100},
        "inputs": {
            "source_manifest": relative_artifact(args.source_manifest, args.workspace),
            "prepare_receipt": relative_artifact(args.prepare_receipt, args.workspace),
            "target_graph_manifest": graph["artifact"],
            "module_cache_manifest": cache["manifest_artifact"],
            "module_cache_archive": cache["archive_artifact"],
            "container_states_manifest": states["artifact"],
        },
        "lifecycle": ["create", "inspect-exact", "start-detached", "status-resumable",
                      "collect-bounded-log", "verify", "remove-owned-container"],
    }
    plan["create_command"] = arm_create_command(plan)
    atomic_new_json(args.output, plan)
    return plan


def _one_inspect(kind: str, identity: str) -> dict[str, Any]:
    value = docker_json(kind, "inspect", identity)
    if not isinstance(value, list) or len(value) != 1 or not isinstance(value[0], dict):
        raise ContractError("Docker inspect returned an unexpected shape")
    return value[0]


def _load_plan(args: argparse.Namespace, expected_kind: str) -> dict[str, Any]:
    plan = load_bound(args.plan, args.plan_sha256, "simulation plan")
    if plan.get("kind") != expected_kind:
        raise ContractError("simulation plan kind differs")
    return plan


def _require_new_container(created: dict[str, Any], identity: str, name: str) -> None:
    state = created.get("State") or {}
    zero_time = re.compile(r"^0001-01-01T00:00:00(?:\.0+)?Z$")
    if (created.get("Id") != identity or created.get("Name") != "/" + name
            or created.get("RestartCount") != 0 or state.get("Status") != "created"
            or state.get("Running") is not False or state.get("Pid") != 0
            or not zero_time.fullmatch(str(state.get("StartedAt")))
            or not zero_time.fullmatch(str(state.get("FinishedAt")))):
        raise ContractError("container is not the exact newly-created instance")


def _arm_exact_created(plan: dict[str, Any], created: dict[str, Any], identity: str | None = None,
                       require_new: bool = False) -> None:
    host = created.get("HostConfig") or {}
    config = created.get("Config") or {}
    expected_env = {}
    for item in plan["image_defaults"]["env"]:
        if "=" not in item or item.split("=", 1)[0] in expected_env:
            raise ContractError("ARM plan image environment is invalid")
        expected_env[item.split("=", 1)[0]] = item.split("=", 1)[1]
    expected_env.update(GOMEMLIMIT="384MiB", HOLYARRS_SCALE="combined", HOLYARRS_SCALE_IDLE=plan["idle"])
    actual_env = {}
    for item in config.get("Env", []):
        if "=" not in item or item.split("=", 1)[0] in actual_env:
            raise ContractError("created ARM environment is invalid")
        actual_env[item.split("=", 1)[0]] = item.split("=", 1)[1]
    expected_cmd = arm_create_command(plan)[-4:]
    tmpfs = host.get("Tmpfs") or {}
    security = host.get("SecurityOpt") or []
    if require_new:
        if identity is None:
            raise ContractError("new ARM container validation lacks its full ID")
        _require_new_container(created, identity, plan["container_name"])
    if (created.get("Image") != plan["image_id"] or config.get("User") != "1000:1000"
            or (config.get("Labels") or {}).get("io.holyarrs.device-simulation") != plan["run_id"]
            or config.get("Cmd") != expected_cmd
            or config.get("Entrypoint") != plan["image_defaults"]["entrypoint"]
            or actual_env != expected_env or host.get("NetworkMode") != "none"
            or host.get("Memory") != ARM_LIMITS["memory_bytes"]
            or host.get("MemorySwap") != ARM_LIMITS["memory_swap_bytes"]
            or host.get("NanoCpus") != 4_000_000_000 or host.get("PidsLimit") != 256
            or host.get("ReadonlyRootfs") is not True or host.get("CapDrop") != ["ALL"]
            or host.get("CapAdd") not in (None, []) or host.get("Privileged") is not False
            or host.get("AutoRemove") is not False or (host.get("RestartPolicy") or {}).get("Name") != "no"
            or security != ["no-new-privileges"]
            or tmpfs != {"/tmp": "size=536870912,mode=1777"}
            or (host.get("Binds") or []) != [] or (created.get("Mounts") or []) != []):
        raise ContractError("created ARM container differs from its exact plan")


def start_arm(args: argparse.Namespace) -> dict[str, Any]:
    plan = _load_plan(args, "pi-like-arm-emulation")
    if args.state.exists():
        raise ContractError("ARM state already exists; use status or cleanup")
    if plan.get("create_command") != arm_create_command(plan):
        raise ContractError("recorded ARM command differs from reconstructed command")
    image = _one_inspect("image", plan["image_id"])
    profile = ARM[plan["architecture"]]
    if (image.get("Id") != plan["image_id"] or image.get("Architecture") != profile["image_arch"]
            or image.get("Variant") != profile["variant"]
            or ((image.get("Config") or {}).get("Env") or []) != plan["image_defaults"]["env"]
            or (image.get("Config") or {}).get("Entrypoint") != plan["image_defaults"]["entrypoint"]):
        raise ContractError("prepared ARM image identity or architecture differs")
    if _discover_owned_container(plan["container_name"], plan["run_id"]):
        raise ContractError("planned ARM container name already exists")
    state = {"schema": 1, "kind": "arm-run-state", "status": "create-requested", "run_id": plan["run_id"],
             "plan_sha256": args.plan_sha256, "container_id": None,
             "created_at": dt.datetime.now(dt.timezone.utc).isoformat(), "start_requested": False,
             "creation_intent": {"kind": "container", "name": plan["container_name"],
                                 "status": "requested", "id": None}}
    atomic_new_json(args.state, state)
    try:
        raw_identity = run_limited(arm_create_command(plan)).decode().strip()
    except BaseException as create_error:
        recovered = _discover_owned_container(plan["container_name"], plan["run_id"])
        if recovered:
            state.update(status="create-failed-cleanup-required", container_id=recovered)
            state["creation_intent"].update(status="resolved", id=recovered)
            atomic_replace_json(args.state, state)
            try:
                _remove_owned_container(recovered, plan["run_id"])
                state["status"] = "create-failed-cleaned"
                state["creation_intent"]["status"] = "cleaned"
                atomic_replace_json(args.state, state)
            except BaseException as cleanup_error:
                raise ContractError("ARM create failed and recovered container cleanup also failed") from cleanup_error
        else:
            state["status"] = "create-outcome-unknown"
            state["creation_intent"]["status"] = "outcome-unknown"
            atomic_replace_json(args.state, state)
        raise create_error
    identity = raw_identity if HEX64.fullmatch(raw_identity) else _discover_owned_container(plan["container_name"], plan["run_id"])
    try:
        if not isinstance(identity, str) or not HEX64.fullmatch(identity):
            state["status"] = "create-outcome-unknown"
            state["creation_intent"]["status"] = "outcome-unknown"
            atomic_replace_json(args.state, state)
            raise ContractError("Docker did not return a full container ID")
        state.update(status="created", container_id=identity)
        state["creation_intent"].update(status="resolved", id=identity)
        atomic_replace_json(args.state, state)
        created = _one_inspect("container", identity)
        _arm_exact_created(plan, created, identity, require_new=True)
        state["start_requested"] = True
        state["status"] = "start-requested"
        atomic_replace_json(args.state, state)
        run_limited(["docker", "start", identity])
        running = _one_inspect("container", identity)
        _arm_exact_created(plan, running)
        if not (running.get("State") or {}).get("Running"):
            raise ContractError("ARM container did not start")
        state.update(status="running", container_started_at=running["State"].get("StartedAt"),
                     supervision={"poll_seconds": None, "observations": [], "complete": False})
        atomic_replace_json(args.state, state)
        return {"status": "running", "container_id": identity, "run_id": plan["run_id"],
                "resume": "invoke status-arm with the same hash-bound plan and state"}
    except BaseException as original:
        if isinstance(identity, str) and HEX64.fullmatch(identity):
            try:
                _remove_owned_container(identity, plan["run_id"])
                if args.state.exists():
                    state["status"] = "failed-cleaned"
                    atomic_replace_json(args.state, state)
            except BaseException as cleanup_error:
                raise ContractError("ARM create failed and exact container cleanup also failed") from cleanup_error
        raise original


def _result_from_log(raw: bytes) -> dict[str, Any]:
    matches = []
    for line in raw.decode("utf-8", errors="replace").splitlines():
        if "SCALERESULT " in line:
            try:
                matches.append(json.loads(line.split("SCALERESULT ", 1)[1]))
            except json.JSONDecodeError as exc:
                raise ContractError("SCALERESULT is invalid JSON") from exc
    if len(matches) != 1 or not isinstance(matches[0], dict):
        raise ContractError("expected exactly one SCALERESULT")
    return matches[0]


def watch_arm(args: argparse.Namespace) -> dict[str, Any]:
    """Supervise one detached ARM run; interruption leaves an incomplete claim."""
    plan = _load_plan(args, "pi-like-arm-emulation")
    state = load_bound(args.state, args.state_sha256, "ARM state")
    if state.get("plan_sha256") != args.plan_sha256 or state.get("run_id") != plan["run_id"]:
        raise ContractError("ARM state does not match plan")
    identity = state.get("container_id")
    if not isinstance(identity, str) or not HEX64.fullmatch(identity):
        raise ContractError("ARM state lacks a full container ID")
    supervision = state.get("supervision") or {"observations": [], "complete": False}
    observations = supervision.get("observations")
    maximum = math.ceil((IDLE[plan["idle"]]["seconds"] + 3600) / args.poll_seconds) + 10
    if not isinstance(observations, list) or len(observations) > maximum:
        raise ContractError("ARM supervision state is invalid")
    if observations:
        raise ContractError("interrupted ARM supervision cannot be resumed as one continuous run")
    supervision["poll_seconds"] = args.poll_seconds
    supervision["complete"] = False
    while True:
        current = _one_inspect("container", identity)
        _arm_exact_created(plan, current)
        now = dt.datetime.now(dt.timezone.utc)
        monotonic_ns = time.monotonic_ns()
        if observations:
            previous = observations[-1]
            gap = (now - parse_timestamp(previous.get("wall_utc"))).total_seconds()
            if monotonic_ns <= previous.get("monotonic_ns", 0):
                raise ContractError("ARM supervision monotonic clock did not increase")
            if gap > args.poll_seconds + 30:
                supervision["failure"] = "supervision-gap"
                state["supervision"] = supervision
                atomic_replace_json(args.state, state)
                raise ContractError("ARM supervision gap exceeded the allowed bound")
        observations.append({"wall_utc": now.isoformat(), "monotonic_ns": monotonic_ns})
        state["supervision"] = supervision
        atomic_replace_json(args.state, state)
        docker_state = current.get("State") or {}
        if not docker_state.get("Running"):
            supervision["complete"] = True
            supervision["container_finished_at"] = docker_state.get("FinishedAt")
            state["status"] = "stopped-supervised"
            atomic_replace_json(args.state, state)
            return {"status": "stopped-supervised", "observations": len(observations)}
        if len(observations) >= maximum:
            supervision["failure"] = "observation-bound-exceeded"
            state["supervision"] = supervision
            atomic_replace_json(args.state, state)
            raise ContractError("ARM supervision exceeded its bounded observation count")
        time.sleep(args.poll_seconds)


def status_arm(args: argparse.Namespace) -> dict[str, Any]:
    plan = _load_plan(args, "pi-like-arm-emulation")
    state = load_bound(args.state, args.state_sha256, "ARM state")
    if state.get("plan_sha256") != args.plan_sha256 or state.get("run_id") != plan["run_id"]:
        raise ContractError("ARM state does not match plan")
    identity = state.get("container_id")
    if not isinstance(identity, str) or not re.fullmatch(r"[0-9a-f]{64}", identity):
        raise ContractError("ARM state lacks a full container ID")
    current = _one_inspect("container", identity)
    _arm_exact_created(plan, current)
    docker_state = current.get("State") or {}
    if docker_state.get("Running"):
        state.update(status="running", checked_at=dt.datetime.now(dt.timezone.utc).isoformat())
        atomic_replace_json(args.state, state)
        return {"status": "running", "container_id": identity,
                "started_at": docker_state.get("StartedAt"), "claim": "pending"}
    runtime_log = args.receipt.parent / "arm-runtime.log"
    if args.log.resolve(strict=False) != runtime_log.resolve(strict=False):
        raise ContractError("ARM runtime log must be the fixed receipt-relative arm-runtime.log")
    raw = run_limited(["docker", "logs", "--tail", "2000", identity])
    redacted = re.sub(rb"(setup code )\S+", rb"\1[redacted]", raw)
    if len(redacted) > MAX_OUTPUT:
        raise ContractError("ARM log exceeded the retained bound")
    result = _result_from_log(redacted)
    atomic_new_bytes(runtime_log, redacted)
    receipt = {
        "schema": 1, "status": "passed" if docker_state.get("ExitCode") == 0 and not docker_state.get("OOMKilled") else "failed",
        "source_commit": plan["source_commit"], "target_source_commit": plan["target_source_commit"],
        "source_tree": plan["source_tree"], "target_source_tree": plan["target_source_tree"],
        "final_source_gate": plan["final_source_gate"], "preparation": plan["preparation"],
        "run_id": plan["run_id"], "architecture": plan["architecture"],
        "platform": plan["platform"], "execution_mode": "emulated", "idle": plan["idle"], "image_id": plan["image_id"],
        "container_id": identity, "container_started_at": docker_state.get("StartedAt"),
        "container_finished_at": docker_state.get("FinishedAt"), "container_exit_code": docker_state.get("ExitCode"),
        "container_oom_killed": docker_state.get("OOMKilled"), "resources": ARM_LIMITS, "result": result,
        "supervision": state.get("supervision"),
        "log": {"path": "arm-runtime.log", "bytes": len(redacted),
                "sha256_raw": hashlib.sha256(redacted).hexdigest()},
        "container_removed": False, "owned_resources_absent": False,
    }
    atomic_new_json(args.receipt, receipt)
    if receipt["status"] != "passed":
        raise ContractError("ARM container failed; receipt retained")
    _remove_owned_container(identity, plan["run_id"])
    for orphan in _owned_container_ids(plan["run_id"]):
        _remove_owned_container(orphan, plan["run_id"])
    _stable_absence("container", run_id=plan["run_id"])
    if _owned_container_ids(plan["run_id"]):
        raise ContractError("owned ARM container remains after cleanup")
    receipt["container_removed"] = True
    receipt["owned_resources_absent"] = True
    atomic_replace_json(args.receipt, receipt)
    verify_args = argparse.Namespace(plan=args.plan, plan_sha256=args.plan_sha256,
                                     receipt=args.receipt, receipt_sha256=sha256_file(args.receipt))
    verify_arm(verify_args)
    state.update(status="complete", receipt_sha256=sha256_file(args.receipt), completed_at=dt.datetime.now(dt.timezone.utc).isoformat())
    atomic_replace_json(args.state, state)
    return {"status": "passed", "receipt": str(args.receipt), "receipt_sha256": sha256_file(args.receipt)}


def cleanup_arm(args: argparse.Namespace) -> dict[str, Any]:
    plan = _load_plan(args, "pi-like-arm-emulation")
    state = load_bound(args.state, args.state_sha256, "ARM state")
    identity = state.get("container_id")
    intent = state.get("creation_intent") or {}
    if not isinstance(identity, str) or not HEX64.fullmatch(identity):
        identity = _discover_owned_container(plan["container_name"], plan["run_id"])
        if not identity:
            state["status"] = "create-outcome-unknown"
            intent["status"] = "outcome-unknown"
            state["creation_intent"] = intent
            atomic_replace_json(args.state, state)
            raise ContractError("ARM create outcome remains unresolved; cleanup cannot claim absence")
        state["container_id"] = identity
        intent.update(status="resolved", id=identity)
        state["creation_intent"] = intent
        atomic_replace_json(args.state, state)
    owned = _owned_container_ids(plan["run_id"])
    if not owned:
        _stable_absence("container", run_id=plan["run_id"])
        intent["status"] = "cleaned"
        state["creation_intent"] = intent
        state["status"] = "cleaned"
        atomic_replace_json(args.state, state)
        return {"status": "already-absent"}
    for owned_id in owned:
        _remove_owned_container(owned_id, plan["run_id"])
    _stable_absence("container", run_id=plan["run_id"])
    intent["status"] = "cleaned"
    state["creation_intent"] = intent
    state["status"] = "cleaned"
    atomic_replace_json(args.state, state)
    return {"status": "removed", "container_ids": owned}


def safe_posix_data_root(value: str) -> None:
    path = PurePosixPath(value)
    if not value.startswith("/data/") or ".." in path.parts or len(path.parts) < 3:
        raise ContractError("synthetic data paths must be contained below /data")


APP_IMAGE_ENV = {"PATH", "GOMEMLIMIT", "GOGC", "TZ"}


def _exact_env(values: Any, label: str) -> list[str]:
    if not isinstance(values, list) or any(not isinstance(value, str) for value in values):
        raise ContractError(f"{label} environment is invalid")
    names = []
    for value in values:
        if "=" not in value:
            raise ContractError(f"{label} environment is invalid")
        name, _ = value.split("=", 1)
        if name not in APP_IMAGE_ENV or name in names:
            raise ContractError(f"{label} environment is invalid")
        names.append(name)
    return values


def _container_env(image_env: list[str]) -> list[str]:
    overrides = [("GOMEMLIMIT", "384MiB"), ("HOLYARRS_SAVE_PATH", "/data/downloads")]
    overridden = {name for name, _ in overrides}
    return [*(f"{name}={value}" for name, value in overrides),
            *(value for value in image_env if value.split("=", 1)[0] not in overridden)]


def prepare_nas(args: argparse.Namespace) -> dict[str, Any]:
    if (not HEX40.fullmatch(args.source_commit) or not HEX40.fullmatch(args.source_tree)
            or not RUN_ID.fullmatch(args.run_id)):
        raise ContractError("source commit, tree, or run ID is invalid")
    require_bound_file(args.source_manifest, args.source_manifest_sha256, "NAS source manifest")
    source_manifest_artifact = relative_artifact(args.source_manifest, args.workspace)
    candidate = load_bound(args.candidate_receipt, args.candidate_receipt_sha256,
                           "candidate image receipt")
    candidate_receipt_artifact = relative_artifact(args.candidate_receipt, args.workspace)
    source = candidate.get("source") or {}
    binary = candidate.get("tested_binary") or {}
    image = candidate.get("image") or {}
    test = candidate.get("test") or {}
    actual_commit, actual_tree = source.get("commit"), source.get("tree")
    historical = actual_commit != args.source_commit
    if historical and not args.allow_historical_harness:
        raise ContractError("candidate image receipt is not bound to the intended source")
    if (candidate.get("schema") != "holyarrs.nas-image.v1"
            or not HEX40.fullmatch(str(actual_commit)) or not HEX40.fullmatch(str(actual_tree))
            or (not historical and actual_tree != args.source_tree)
            or source.get("verified_tree_before") != actual_tree
            or source.get("verified_tree_after") != actual_tree
            or (source.get("manifest") or {}).get("path") != source_manifest_artifact["path"]
            or (source.get("manifest") or {}).get("sha256_raw") != args.source_manifest_sha256
            or image.get("id") != args.candidate_image
            or image.get("tested_binary_sha256_raw") != binary.get("sha256_raw")
            or not HEX64.fullmatch(str(binary.get("sha256_raw")))
            or test.get("exit_code") != 0
            or not HEX64.fullmatch(str((test.get("log") or {}).get("sha256_raw")))):
        raise ContractError("candidate image source or tested-binary binding is invalid")
    require_workspace_evidence(args.workspace, binary.get("path"), binary.get("sha256_raw"),
                               "NAS tested binary")
    test_log = require_workspace_evidence(args.workspace, (test.get("log") or {}).get("path"),
                                          (test.get("log") or {}).get("sha256_raw"), "NAS test log")
    if re.search(rb"(?m)^PASS\s*$", test_log.read_bytes()) is None:
        raise ContractError("NAS test log lacks passing test evidence")
    if args.candidate_revision != actual_commit or not HEX40.fullmatch(args.baseline_revision):
        raise ContractError("candidate or baseline image revision binding is invalid")
    for label, value in (("candidate", args.candidate_image), ("baseline", args.baseline_image),
                         ("helper", args.helper_image)):
        if not IMAGE_ID.fullmatch(value):
            raise ContractError(f"{label} image must be an immutable local image ID")
    if not 18731 <= args.port <= 18749:
        raise ContractError("NAS simulation port must be in 18731-18749")
    helper = load_bound(args.helper_receipt, args.helper_receipt_sha256, "helper image receipt")
    if (helper.get("id") != args.helper_image or not HEX64.fullmatch(str(helper.get("dockerfile_sha256")))
            or helper.get("entrypoint") is not None and not isinstance(helper.get("entrypoint"), list)
            or not isinstance(helper.get("user"), str)):
        raise ContractError("helper image receipt identity or metadata is invalid")
    app_images = load_bound(args.app_images_receipt, args.app_images_receipt_sha256,
                            "NAS app images receipt")
    app_images_artifact = relative_artifact(args.app_images_receipt, args.workspace)
    images = app_images.get("images")
    if app_images.get("schema") != "holyarrs.nas-app-images.v1" or not isinstance(images, dict):
        raise ContractError("NAS app images receipt is invalid")
    expected_ids = {args.baseline_image, args.candidate_image}
    if set(images) != expected_ids:
        raise ContractError("NAS app images receipt does not bind the exact image set")
    expected_revisions = {args.baseline_image: args.baseline_revision,
                          args.candidate_image: args.candidate_revision}
    if args.baseline_image == args.candidate_image and args.baseline_revision != args.candidate_revision:
        raise ContractError("one NAS image ID cannot bind different revisions")
    app_contracts = {}
    for image_id in sorted(expected_ids):
        item = images.get(image_id)
        defaults = (item or {}).get("image_defaults")
        if (not isinstance(item, dict) or item.get("revision") != expected_revisions[image_id]
                or item.get("binary_entrypoint") != ["/holyarrs"] or not isinstance(defaults, dict)
                or set(defaults) != {"entrypoint", "cmd", "env", "user"}
                or defaults.get("entrypoint") is not None and not isinstance(defaults.get("entrypoint"), list)
                or defaults.get("cmd") is not None and not isinstance(defaults.get("cmd"), list)
                or any(not isinstance(value, str) for value in (defaults.get("entrypoint") or []))
                or any(not isinstance(value, str) for value in (defaults.get("cmd") or []))
                or not isinstance(defaults.get("user"), str)):
            raise ContractError("NAS app image defaults are invalid")
        image_env = _exact_env(defaults.get("env"), "NAS app image")
        app_contracts[image_id] = {
            "revision": item["revision"],
            "image_defaults": defaults,
            "entrypoint": ["/holyarrs"],
            "cmd": ["-config", "/config", "-addr", ":18730"],
            "env": _container_env(image_env),
        }
    for path in ("/data/downloads", "/data/library/movies", "/data/library/shows", "/data/library/music"):
        safe_posix_data_root(path)
    names = {purpose: f"holyarrs-device-{purpose}-{args.run_id}" for purpose in
             ("config", "data", "backup", "restore", "empty-data", "network", "app")}
    plan = {
        "schema": 1,
        "kind": "nas-filesystem-contract-simulation",
        "claim_boundary": "isolated Docker volumes and local filesystem contracts; not NAS hardware, UI/catalog, SMB, NFS, ACL, or vendor certification",
        "source_commit": actual_commit,
        "source_tree": actual_tree,
        "target_source_commit": args.source_commit,
        "target_source_tree": args.source_tree,
        "final_source_gate": not historical,
        "claim": "historical NAS harness only; not final-source acceptance" if historical else "final-source filesystem-contract simulation",
        "run_id": args.run_id,
        "candidate_image_id": args.candidate_image,
        "candidate_revision": args.candidate_revision,
        "candidate_provenance": {
            "receipt_sha256": args.candidate_receipt_sha256,
            "source_manifest_sha256_raw": args.source_manifest_sha256,
            "tested_binary_sha256_raw": binary.get("sha256_raw"),
            "test_log_sha256_raw": (test.get("log") or {}).get("sha256_raw"),
        },
        "inputs": {"source_manifest": source_manifest_artifact,
                   "candidate_receipt": candidate_receipt_artifact,
                   "app_images_receipt": app_images_artifact},
        "app_images_receipt_sha256": args.app_images_receipt_sha256,
        "app_contracts": app_contracts,
        "baseline_image_id": args.baseline_image,
        "baseline_revision": args.baseline_revision,
        "helper_image_id": args.helper_image,
        "helper_expectations": {"receipt_sha256": args.helper_receipt_sha256,
                                "dockerfile_sha256": helper["dockerfile_sha256"],
                                "user": helper["user"], "entrypoint": helper["entrypoint"]},
        "port": args.port,
        "resources": ORDINARY_LIMITS,
        "names": names,
        "labels": {"io.holyarrs.device-simulation": args.run_id},
        "synthetic_layout": ["/data/downloads", "/data/library/movies", "/data/library/shows", "/data/library/music"],
        "mount_contract": {"config": {"target": "/config", "mode": "rw", "uid_gid": "1000:1000", "filesystem": "local"},
                           "data": {"target": "/data", "mode": "ro during preservation/failure checks", "synthetic_only": True}},
        "scenarios": NAS_SCENARIOS,
        "required_evidence": [
            "candidate receipt binds image ID to source tree, manifest, tested binary and test log",
            "full container/image IDs and exact labels", "port preflight", "fresh setup/login/logout",
            "config and data entry/content manifests", "restart and update identity",
            "stopped backup equals disposable restore", "data manifest unchanged",
            "real device/inode hardlink on one volume", "real EXDEV or two-volume copy fallback",
            "network-config refusal using product source contract",
            "owned resource absence after cleanup",
        ],
        "lifecycle": ["preflight", "create-labeled-volumes-and-internal-network", "populate-synthetic-data",
                      "baseline-first-run", "restart", "candidate-update", "backup", "restore",
                      "empty-data-recovery", "filesystem-copy-semantics", "cleanup-by-full-id-and-label"],
        "forbidden_targets": ["X:\\", "\\\\10.10.25.50", "host port 445", "host firewall", "host network adapters"],
        "protocol_coverage": {"smb": "not tested", "nfs": "not tested"},
    }
    atomic_new_json(args.output, plan)
    return plan


def _resource_ids(kind: str, name: str | None = None, run_id: str | None = None) -> list[str]:
    command = ["docker", kind, "ls"]
    if kind == "container":
        command.append("--all")
    command.extend(["--quiet", "--no-trunc"] if kind in ("container", "network") else ["--quiet"])
    if name is not None:
        command.extend(["--filter", f"name={'^/' if kind == 'container' else '^'}{name}$"])
    if run_id is not None:
        command.extend(["--filter", f"label=io.holyarrs.device-simulation={run_id}"])
    return [value for value in run_limited(command).decode().splitlines() if value]


def _stable_absence(kind: str, name: str | None = None, run_id: str | None = None) -> None:
    deadline = time.monotonic() + DISCOVERY_SECONDS
    while time.monotonic() <= deadline:
        if _resource_ids(kind, name, run_id):
            raise ContractError(f"planned Docker {kind} resource already exists")
        time.sleep(DISCOVERY_INTERVAL)
    return


def _docker_create_owned(kind: str, name: str, run_id: str, state: dict[str, Any],
                         state_path: Path) -> dict[str, Any]:
    _stable_absence(kind, name=name)
    intent = _record_creation_intent(state, state_path, kind, name)
    command = ["docker", kind, "create", "--label", f"io.holyarrs.device-simulation={run_id}"]
    if kind == "network":
        command.append("--internal")
    command.append(name)
    try:
        run_limited(command)
    except ContractError:
        # A daemon may have committed the create before the client observed its reply.
        pass
    deadline = time.monotonic() + DISCOVERY_SECONDS
    while True:
        ids = _resource_ids(kind, name=name)
        if ids:
            break
        if time.monotonic() >= deadline:
            _resolve_creation_intent(state, state_path, intent, "outcome-unknown")
            raise ContractError(f"Docker {kind} create did not expose the owned resource")
        time.sleep(DISCOVERY_INTERVAL)
    if len(ids) != 1:
        raise ContractError(f"Docker {kind} create resolved ambiguously")
    current = _one_inspect(kind, ids[0])
    labels = current.get("Labels") or {}
    if kind == "volume":
        valid = (ids[0] == name and current.get("Name") == name and current.get("Driver") == "local"
                 and current.get("Scope") == "local")
        identity = name
    else:
        identity = current.get("Id")
        valid = (IMAGE_ID.fullmatch("sha256:" + str(identity)) is not None and current.get("Name") == name
                 and current.get("Driver") == "bridge" and current.get("Scope") == "local"
                 and current.get("Internal") is True)
    if not valid or labels.get("io.holyarrs.device-simulation") != run_id:
        raise ContractError(f"created Docker {kind} differs from its exact plan")
    fact = {"kind": kind, "name": name, "id": identity, "driver": current.get("Driver"),
            "scope": current.get("Scope"), "internal": current.get("Internal") if kind == "network" else None,
            "labels": labels}
    state.setdefault("resources", []).append(fact)
    intent.update(status="resolved", id=identity)
    atomic_replace_json(state_path, state)
    return fact


def _register_container(state: dict[str, Any], state_path: Path, identity: str, name: str) -> None:
    active = state.setdefault("active_containers", [])
    if identity not in active:
        active.append(identity)
    state.setdefault("container_history", []).append({"id": identity, "name": name})
    atomic_replace_json(state_path, state)


def _record_creation_intent(state: dict[str, Any], state_path: Path, kind: str, name: str) -> dict[str, Any]:
    intent = {"kind": kind, "name": name, "status": "requested", "id": None}
    state.setdefault("creation_intents", []).append(intent)
    atomic_replace_json(state_path, state)
    return intent


def _resolve_creation_intent(state: dict[str, Any], state_path: Path, intent: dict[str, Any],
                             status: str, identity: str | None = None) -> None:
    intent.update(status=status, id=identity)
    atomic_replace_json(state_path, state)


def _unresolved_creation_intents(state: dict[str, Any]) -> list[dict[str, Any]]:
    return [intent for intent in state.get("creation_intents", [])
            if isinstance(intent, dict) and intent.get("status") in ("requested", "outcome-unknown")]


def _unregister_container(state: dict[str, Any], state_path: Path, identity: str) -> None:
    state["active_containers"] = [value for value in state.get("active_containers", []) if value != identity]
    atomic_replace_json(state_path, state)


def _discover_owned_container(name: str, run_id: str) -> str | None:
    deadline = time.monotonic() + DISCOVERY_SECONDS
    while True:
        raw = _resource_ids("container", name=name)
        if raw:
            if len(raw) != 1 or not HEX64.fullmatch(raw[0]):
                raise ContractError("ambiguous Docker create did not resolve to one full container ID")
            current = _one_inspect("container", raw[0])
            if (current.get("Id") != raw[0] or current.get("Name") != "/" + name
                    or (current.get("Config", {}).get("Labels") or {}).get("io.holyarrs.device-simulation") != run_id):
                raise ContractError("ambiguous Docker create resolved to an unowned container")
            return raw[0]
        if time.monotonic() >= deadline:
            return None
        time.sleep(DISCOVERY_INTERVAL)


def _owned_container_ids(run_id: str) -> list[str]:
    identities = _resource_ids("container", run_id=run_id)
    for identity in identities:
        if not HEX64.fullmatch(identity):
            raise ContractError("owned container listing did not return full IDs")
        current = _one_inspect("container", identity)
        if (current.get("Id") != identity
                or (current.get("Config", {}).get("Labels") or {}).get("io.holyarrs.device-simulation") != run_id):
            raise ContractError("owned container listing contained an inexact resource")
    return identities


def _create_owned_container(plan: dict[str, Any], state: dict[str, Any], state_path: Path,
                            name: str, command: list[str]) -> str:
    if _discover_owned_container(name, plan["run_id"]):
        raise ContractError("planned Docker container name already exists")
    intent = _record_creation_intent(state, state_path, "container", name)
    try:
        raw = run_limited(command).decode().strip()
    except ContractError:
        recovered = _discover_owned_container(name, plan["run_id"])
        if recovered:
            _resolve_creation_intent(state, state_path, intent, "resolved", recovered)
            _register_container(state, state_path, recovered, name)
        else:
            _resolve_creation_intent(state, state_path, intent, "outcome-unknown")
        raise
    identity = raw if HEX64.fullmatch(raw) else _discover_owned_container(name, plan["run_id"])
    if not identity:
        _resolve_creation_intent(state, state_path, intent, "outcome-unknown")
        raise ContractError("Docker did not return or expose a full owned container ID")
    _resolve_creation_intent(state, state_path, intent, "resolved", identity)
    _register_container(state, state_path, identity, name)
    return identity


def _helper(plan: dict[str, Any], state: dict[str, Any], state_path: Path, purpose: str,
            mounts: list[str], script: str, timeout: int = 60, user: str = "1000:1000",
            cap_add: tuple[str, ...] = ()) -> bytes:
    if len(cap_add) != len(set(cap_add)) or any(capability != "CHOWN" for capability in cap_add):
        raise ContractError("helper capability plan is invalid")
    name = f"holyarrs-device-helper-{purpose}-{plan['run_id']}"
    command = ["docker", "create", "--pull=never", "--name", name, "--network", "none",
               "--cpus", "1", "--memory", "256m", "--memory-swap", "256m",
               "--pids-limit", "128", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
               "--user", user, "--entrypoint", "sh",
               "--label", f"io.holyarrs.device-simulation={plan['run_id']}"]
    for capability in cap_add:
        command.extend(["--cap-add", capability])
    for mount in mounts:
        command.extend(["--mount", mount])
    command.extend([plan["helper_image_id"], "-eu", "-c", script])
    identity = _create_owned_container(plan, state, state_path, name, command)
    try:
        created = _one_inspect("container", identity)
        _require_new_container(created, identity, name)
        host, config = created.get("HostConfig") or {}, created.get("Config") or {}
        mount_items = created.get("Mounts") or []
        if (not isinstance(mount_items, list) or any(not isinstance(item, dict) for item in mount_items)
                or len({item.get("Destination") for item in mount_items}) != len(mount_items)):
            raise ContractError("helper container has duplicate or invalid mount destinations")
        actual_mounts = {item.get("Destination"): {"rw": item.get("RW"), "type": item.get("Type"),
                                                    "source": item.get("Name")}
                         for item in mount_items}
        expected_mounts = {}
        for mount in mounts:
            fields = dict(item.split("=", 1) for item in mount.split(",") if "=" in item)
            if fields.get("dst") in expected_mounts or not fields.get("src"):
                raise ContractError("helper plan has duplicate or invalid mount destinations")
            expected_mounts[fields["dst"]] = {"rw": "readonly" not in mount, "type": fields["type"],
                                               "source": fields["src"]}
        if (created.get("Image") != plan["helper_image_id"] or config.get("User") != user
                or config.get("Cmd") != ["-eu", "-c", script] or config.get("Entrypoint") != ["sh"]
                or (config.get("Labels") or {}).get("io.holyarrs.device-simulation") != plan["run_id"]
                or host.get("NetworkMode") != "none" or host.get("Memory") != 268435456
                or host.get("MemorySwap") != 268435456 or host.get("NanoCpus") != 1_000_000_000
                or host.get("PidsLimit") != 128 or host.get("CapDrop") != ["ALL"]
                or not isinstance(host.get("CapAdd") or [], list)
                or (host.get("CapAdd") or []) != list(cap_add) or host.get("Privileged") is not False
                or host.get("AutoRemove") is not False
                or (host.get("RestartPolicy") or {}).get("Name") != "no"
                or (host.get("SecurityOpt") or []) != ["no-new-privileges"]
                or actual_mounts != expected_mounts):
            raise ContractError("helper container differs from its plan")
        raw = run_limited(["docker", "start", "--attach", identity], timeout=timeout)
        stopped = _one_inspect("container", identity)
        if (stopped.get("State") or {}).get("ExitCode") != 0:
            raise ContractError("helper container failed")
        return raw
    finally:
        _remove_owned_container(identity, plan["run_id"])
        _unregister_container(state, state_path, identity)


def _sanitized_manifest(raw: bytes, kind: str) -> dict[str, Any]:
    text = raw.decode("utf-8")
    records = text.splitlines()
    if len(records) > 1000 or any(len(line) > 4096 for line in records):
        raise ContractError("filesystem manifest exceeded its record bounds")
    forbidden = ("10.10.25.50", "X:\\", "\\\\")
    if any(value in text for value in forbidden):
        raise ContractError("filesystem manifest contains a forbidden real-data reference")
    if kind == "entries":
        if any(line.count("|") != 5 or not line.rsplit("|", 1)[-1].startswith(".") for line in records):
            raise ContractError("filesystem entry manifest is malformed")
    elif any(not re.fullmatch(r"[0-9a-f]{64}  \./.+", line) for line in records):
        raise ContractError("filesystem content manifest is malformed")
    return {"sha256": hashlib.sha256(raw).hexdigest(), "records": records}


def _volume_manifests(plan: dict[str, Any], state: dict[str, Any], state_path: Path,
                      purpose: str, volume: str) -> dict[str, dict[str, Any]]:
    mount = [f"type=volume,src={volume},dst=/subject,readonly"]
    entries = _helper(plan, state, state_path, purpose + "-entries", mount,
                      "cd /subject; find . -xdev -exec stat -c '%F|%a|%u|%g|%s|%n' {} + | sort",
                      user="1000:1000")
    content = _helper(plan, state, state_path, purpose + "-content", mount,
                      "cd /subject; find . -xdev -type f -print | sort | while IFS= read -r file; do sha256sum \"$file\"; done",
                      user="1000:1000")
    return {"entries": _sanitized_manifest(entries, "entries"),
            "content": _sanitized_manifest(content, "content")}


def _port_free(port: int) -> None:
    with socket.socket() as probe:
        probe.settimeout(0.2)
        if probe.connect_ex(("127.0.0.1", port)) == 0:
            raise ContractError("planned loopback port is already in use")


def _wait_health(port: int, timeout: int = 60) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz", timeout=2) as response:
                if response.status == 200 and response.read(16) == b"ok\n":
                    return
        except (OSError, urllib.error.URLError):
            time.sleep(0.25)
    raise ContractError("HolyArrs did not become healthy within the bounded wait")


def _app_create(plan: dict[str, Any], state: dict[str, Any], state_path: Path,
                image: str, name: str, config: str, data: str) -> str:
    contract = plan["app_contracts"].get(image)
    if not isinstance(contract, dict):
        raise ContractError("app image lacks an exact container contract")
    command = [
        "docker", "create", "--pull=never", "--name", name,
        "--label", f"io.holyarrs.device-simulation={plan['run_id']}",
        "--network", plan["names"]["network"], "--cpus", "1", "--memory", "2g",
        "--memory-swap", "2g", "--pids-limit", "256", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges", "--user", "1000:1000",
        "--publish", f"127.0.0.1:{plan['port']}:18730",
        "--entrypoint", "/holyarrs",
        "--env", "GOMEMLIMIT=384MiB",
        "--env", "HOLYARRS_SAVE_PATH=/data/downloads",
        "--mount", f"type=volume,src={config},dst=/config",
        "--mount", f"type=volume,src={data},dst=/data,readonly", image,
        *contract["cmd"],
    ]
    identity = _create_owned_container(plan, state, state_path, name, command)
    created = _one_inspect("container", identity)
    _require_new_container(created, identity, name)
    host, app = created.get("HostConfig") or {}, created.get("Config") or {}
    mount_items = created.get("Mounts") or []
    if (not isinstance(mount_items, list) or any(not isinstance(item, dict) for item in mount_items)
            or len({item.get("Destination") for item in mount_items}) != len(mount_items)):
        raise ContractError("app container has duplicate or invalid mount destinations")
    mounts = {item.get("Destination"): {"rw": item.get("RW"), "type": item.get("Type"),
                                         "source": item.get("Name")}
              for item in mount_items}
    ports = host.get("PortBindings") or {}
    actual_env = app.get("Env") or []
    if (created.get("Image") != image or app.get("User") != "1000:1000"
            or (app.get("Labels") or {}).get("io.holyarrs.device-simulation") != plan["run_id"]
            or app.get("Entrypoint") != contract["entrypoint"]
            or app.get("Cmd") != contract["cmd"]
            or actual_env != contract["env"]
            or host.get("NetworkMode") != plan["names"]["network"]
            or host.get("Memory") != ORDINARY_LIMITS["memory_bytes"]
            or host.get("MemorySwap") != ORDINARY_LIMITS["memory_swap_bytes"]
            or host.get("NanoCpus") != 1_000_000_000 or host.get("PidsLimit") != 256
            or host.get("CapDrop") != ["ALL"] or host.get("CapAdd") not in (None, [])
            or host.get("Privileged") is not False or host.get("AutoRemove") is not False
            or (host.get("RestartPolicy") or {}).get("Name") != "no"
            or (host.get("SecurityOpt") or []) != ["no-new-privileges"]
            or ports != {"18730/tcp": [{"HostIp": "127.0.0.1", "HostPort": str(plan["port"])}]}
            or mounts != {"/config": {"rw": True, "type": "volume", "source": config},
                          "/data": {"rw": False, "type": "volume", "source": data}}):
        raise ContractError("app container differs from its exact plan")
    return identity


def _container_fact(current: dict[str, Any]) -> dict[str, Any]:
    state = current.get("State") or {}
    host, config = current.get("HostConfig") or {}, current.get("Config") or {}
    mounts = sorted({item.get("Destination"): {"rw": item.get("RW"), "type": item.get("Type"),
                                                "source": item.get("Name")}
                     for item in current.get("Mounts", []) if isinstance(item, dict)}.items())
    return {"id": current.get("Id"), "image_id": current.get("Image"), "created": current.get("Created"),
            "started_at": state.get("StartedAt"), "finished_at": state.get("FinishedAt"),
            "exit_code": state.get("ExitCode"), "oom_killed": state.get("OOMKilled"), "mounts": mounts,
            "contract": {"user": config.get("User"), "cmd": config.get("Cmd"),
                         "entrypoint": config.get("Entrypoint"), "env": config.get("Env"),
                         "network_mode": host.get("NetworkMode"),
                         "memory": host.get("Memory"), "memory_swap": host.get("MemorySwap"),
                         "nano_cpus": host.get("NanoCpus"), "pids_limit": host.get("PidsLimit"),
                         "cap_drop": host.get("CapDrop"), "cap_add": host.get("CapAdd"),
                         "privileged": host.get("Privileged"), "auto_remove": host.get("AutoRemove"),
                         "restart": (host.get("RestartPolicy") or {}).get("Name"),
                         "security": host.get("SecurityOpt"), "ports": host.get("PortBindings")}}


def _stop_collect_remove(plan: dict[str, Any], state: dict[str, Any], state_path: Path,
                         identity: str, role: str, facts: list[dict[str, Any]]) -> None:
    run_limited(["docker", "stop", "--time", "20", identity], timeout=30)
    fact = _container_fact(_one_inspect("container", identity))
    fact["role"] = role
    facts.append(fact)
    _remove_owned_container(identity, plan["run_id"])
    _unregister_container(state, state_path, identity)


def _parse_key_values(raw: bytes) -> dict[str, str]:
    result = {}
    for line in raw.decode("utf-8").splitlines():
        if "=" not in line:
            raise ContractError("filesystem evidence is malformed")
        key, value = line.split("=", 1)
        if not re.fullmatch(r"[a-z_]+", key) or key in result or not value:
            raise ContractError("filesystem evidence is malformed")
        result[key] = value
    return result


def _http_json(opener: urllib.request.OpenerDirector, port: int, method: str,
               path: str, value: dict[str, str] | None = None, csrf: str = "") -> dict[str, Any]:
    data = None if value is None else json.dumps(value).encode("utf-8")
    request = urllib.request.Request(f"http://127.0.0.1:{port}{path}", data=data, method=method)
    if data is not None:
        request.add_header("Content-Type", "application/json")
    if csrf:
        request.add_header("X-CSRF-Token", csrf)
    try:
        with opener.open(request, timeout=5) as response:
            raw = response.read(256 * 1024)
            if response.status not in (200, 201):
                raise ContractError("HolyArrs API returned an unexpected status")
    except (OSError, urllib.error.URLError) as exc:
        raise ContractError("HolyArrs API request failed") from exc
    try:
        result = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ContractError("HolyArrs API returned invalid JSON") from exc
    if not isinstance(result, dict):
        raise ContractError("HolyArrs API returned an unexpected shape")
    return result


def _remove_owned_container(identity: str, run_id: str) -> None:
    current = _one_inspect("container", identity)
    if current.get("Id") != identity or (current.get("Config", {}).get("Labels") or {}).get("io.holyarrs.device-simulation") != run_id:
        raise ContractError("refusing to remove an unowned NAS container")
    run_limited(["docker", "container", "rm", "--force", identity])


def run_nas(args: argparse.Namespace) -> dict[str, Any]:
    """Run the bounded filesystem-contract lifecycle; retain state on failure."""
    plan = _load_plan(args, "nas-filesystem-contract-simulation")
    if args.state.exists() or args.receipt.exists():
        raise ContractError("NAS state or receipt already exists")
    _port_free(plan["port"])
    for image_id, contract in plan["app_contracts"].items():
        image = _one_inspect("image", image_id)
        if image.get("Id") != image_id:
            raise ContractError("NAS image identity differs")
        config = image.get("Config") or {}
        defaults = contract["image_defaults"]
        if ((config.get("Labels") or {}).get("org.opencontainers.image.revision") != contract["revision"]
                or config.get("Entrypoint") != defaults["entrypoint"]
                or config.get("Cmd") != defaults["cmd"]
                or (config.get("Env") or []) != defaults["env"]
                or config.get("User", "") != defaults["user"]):
            raise ContractError("NAS app image defaults differ from the bound receipt")
    helper_image = _one_inspect("image", plan["helper_image_id"])
    helper_config = helper_image.get("Config") or {}
    if (helper_image.get("Id") != plan["helper_image_id"]
            or helper_config.get("User") != plan["helper_expectations"]["user"]
            or helper_config.get("Entrypoint") != plan["helper_expectations"]["entrypoint"]):
        raise ContractError("NAS helper image metadata differs")
    state = {"schema": 1, "kind": "nas-run-state", "run_id": plan["run_id"],
             "plan_sha256": args.plan_sha256, "phase": "preflight",
             "active_containers": [], "container_history": [], "resources": []}
    atomic_new_json(args.state, state)
    names = plan["names"]
    try:
        for purpose in ("config", "data", "backup", "restore", "empty-data"):
            _docker_create_owned("volume", names[purpose], plan["run_id"], state, args.state)
        _docker_create_owned("network", names["network"], plan["run_id"], state, args.state)
        state["phase"] = "resources-created"
        fs_raw = _helper(plan, state, args.state, "initialize",
                [f"type=volume,src={names['config']},dst=/config",
                 f"type=volume,src={names['data']},dst=/data",
                 f"type=volume,src={names['backup']},dst=/backup",
                 f"type=volume,src={names['restore']},dst=/restore",
                 f"type=volume,src={names['empty-data']},dst=/empty-data"],
                "mkdir -p /data/downloads /data/library/movies /data/library/shows /data/library/music; "
                "printf 'synthetic movie\\n' >/data/downloads/movie.part; printf 'synthetic library\\n' >/data/library/movies/movie.mkv; "
                "ln /data/library/movies/movie.mkv /data/library/movies/movie-hardlink.mkv; "
                "test \"$(stat -c %d:%i /data/library/movies/movie.mkv)\" = \"$(stat -c %d:%i /data/library/movies/movie-hardlink.mkv)\"; "
                "cp /data/library/movies/movie.mkv /tmp/movie-copy; test -f /data/library/movies/movie.mkv; "
                "test \"$(stat -c %d /data/library/movies/movie.mkv)\" != \"$(stat -c %d /tmp/movie-copy)\"; "
                "test \"$(sha256sum /data/library/movies/movie.mkv | cut -d' ' -f1)\" = \"$(sha256sum /tmp/movie-copy | cut -d' ' -f1)\"; "
                "printf 'hardlink_source=%s\\nhardlink_target=%s\\ncopy_source_device=%s\\ncopy_target_device=%s\\nsource_hash=%s\\ncopy_hash=%s\\nsource_preserved=true\\n' "
                "\"$(stat -c %d:%i /data/library/movies/movie.mkv)\" \"$(stat -c %d:%i /data/library/movies/movie-hardlink.mkv)\" "
                "\"$(stat -c %d /data/library/movies/movie.mkv)\" \"$(stat -c %d /tmp/movie-copy)\" "
                "\"$(sha256sum /data/library/movies/movie.mkv | cut -d' ' -f1)\" \"$(sha256sum /tmp/movie-copy | cut -d' ' -f1)\"; "
                "chown -R 1000:1000 /config /data /backup /restore /empty-data",
                user="0:0", cap_add=("CHOWN",))
        filesystem_facts = _parse_key_values(fs_raw)
        _helper(plan, state, args.state, "uid-write",
                [f"type=volume,src={names['config']},dst=/config"],
                "touch /config/.uid-1000-write; rm /config/.uid-1000-write", user="1000:1000")
        data_before = _volume_manifests(plan, state, args.state, "data-before", names["data"])
        # A second unprivileged helper proves that the preservation mount itself
        # denies writes; the application receives the same read-only mount.
        _helper(plan, state, args.state, "readonly-denial",
                [f"type=volume,src={names['data']},dst=/data,readonly"],
                "if touch /data/application-write-must-fail 2>/dev/null; then exit 1; fi", user="1000:1000")
        password = hashlib.sha256(os.urandom(32)).hexdigest()
        dummy_setting = "simulation-only-" + hashlib.sha256(os.urandom(32)).hexdigest()
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        app_facts: list[dict[str, Any]] = []
        application_evidence: list[dict[str, Any]] = []
        current_id = _app_create(plan, state, args.state, plan["baseline_image_id"], names["app"], names["config"], names["data"])
        state["phase"] = "baseline-created"
        atomic_replace_json(args.state, state)
        run_limited(["docker", "start", current_id])
        _wait_health(plan["port"])
        setup_code = _helper(plan, state, args.state, "read-setup-code",
                             [f"type=volume,src={names['config']},dst=/config,readonly"],
                             "cat /config/setup-code.txt", user="1000:1000").decode().strip()
        if not setup_code:
            raise ContractError("fresh setup code was unavailable")
        session = _http_json(opener, plan["port"], "POST", "/api/v1/auth/setup",
                             {"name": "simulation-owner", "password": password, "code": setup_code})
        if not session.get("csrf"):
            raise ContractError("fresh owner session lacked CSRF evidence")
        stored = _http_json(opener, plan["port"], "PUT", "/api/v1/settings/tvdb-key",
                            {"key": dummy_setting}, csrf=str(session["csrf"]))
        if dummy_setting in json.dumps(stored):
            raise ContractError("dummy encrypted setting was reflected")
        persisted = _http_json(opener, plan["port"], "GET", "/api/v1/settings/tvdb-key")
        if not persisted.get("key") or dummy_setting in json.dumps(persisted):
            raise ContractError("fresh encrypted test setting was unavailable")
        application_evidence.append({"role": "baseline-first-run", "owner": "setup-authenticated",
                                     "encrypted_setting": "present-masked"})
        _stop_collect_remove(plan, state, args.state, current_id, "baseline-first-run", app_facts)
        # Restart the baseline and then the candidate with the same config.
        for image, suffix in ((plan["baseline_image_id"], "restart"), (plan["candidate_image_id"], "update")):
            container_name = f"{names['app']}-{suffix}"
            current_id = _app_create(plan, state, args.state, image, container_name, names["config"], names["data"])
            state["phase"] = suffix
            atomic_replace_json(args.state, state)
            run_limited(["docker", "start", current_id])
            _wait_health(plan["port"])
            fresh = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            logged = _http_json(fresh, plan["port"], "POST", "/api/v1/auth/login",
                                {"name": "simulation-owner", "password": password})
            if not logged.get("csrf"):
                raise ContractError("persisted owner login failed")
            persisted = _http_json(fresh, plan["port"], "GET", "/api/v1/settings/tvdb-key")
            if not persisted.get("key") or dummy_setting in json.dumps(persisted):
                raise ContractError("encrypted test setting did not persist safely")
            application_evidence.append({"role": suffix, "owner": "login-authenticated",
                                         "encrypted_setting": "present-masked"})
            _stop_collect_remove(plan, state, args.state, current_id, suffix, app_facts)
        config_before_backup = _volume_manifests(plan, state, args.state, "config-before-backup", names["config"])
        _helper(plan, state, args.state, "backup",
                [f"type=volume,src={names['config']},dst=/from,readonly",
                 f"type=volume,src={names['backup']},dst=/to"], "cp -a /from/. /to/", user="1000:1000")
        backup_manifest = _volume_manifests(plan, state, args.state, "backup", names["backup"])
        interrupted_raw = _helper(plan, state, args.state, "interrupted-copy",
                [f"type=volume,src={names['backup']},dst=/from,readonly",
                 f"type=volume,src={names['restore']},dst=/to"],
                "source=$(find /from -xdev -type f | sort | head -n 1); test -n \"$source\"; "
                "source_hash=$(sha256sum \"$source\" | cut -d' ' -f1); "
                "dd if=\"$source\" of=/to/.interrupted-copy bs=1 count=1 2>/dev/null; "
                "test -s /to/.interrupted-copy; rm /to/.interrupted-copy; "
                "test ! -e /to/.interrupted-copy; test \"$(sha256sum \"$source\" | cut -d' ' -f1)\" = \"$source_hash\"; "
                "printf 'interrupted_source_hash=%s\\ninterrupted_partial_removed=true\\ninterrupted_source_preserved=true\\n' \"$source_hash\"",
                user="1000:1000")
        interrupted_facts = _parse_key_values(interrupted_raw)
        _helper(plan, state, args.state, "restore-copy",
                [f"type=volume,src={names['backup']},dst=/from,readonly",
                 f"type=volume,src={names['restore']},dst=/to"], "cp -a /from/. /to/", user="1000:1000")
        restore_manifest = _volume_manifests(plan, state, args.state, "restore", names["restore"])
        if config_before_backup != backup_manifest or backup_manifest != restore_manifest:
            raise ContractError("stopped config source, backup, and restore differ")
        for config, data, suffix in ((names["restore"], names["data"], "restore"),
                                     (names["restore"], names["empty-data"], "empty")):
            current_id = _app_create(plan, state, args.state, plan["candidate_image_id"], f"{names['app']}-{suffix}", config, data)
            state["phase"] = suffix
            atomic_replace_json(args.state, state)
            run_limited(["docker", "start", current_id])
            _wait_health(plan["port"])
            fresh = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            if not _http_json(fresh, plan["port"], "POST", "/api/v1/auth/login",
                              {"name": "simulation-owner", "password": password}).get("csrf"):
                raise ContractError("restored owner login failed")
            persisted = _http_json(fresh, plan["port"], "GET", "/api/v1/settings/tvdb-key")
            if not persisted.get("key") or dummy_setting in json.dumps(persisted):
                raise ContractError("restored encrypted test setting was unavailable")
            application_evidence.append({"role": suffix, "owner": "login-authenticated",
                                         "encrypted_setting": "present-masked"})
            _stop_collect_remove(plan, state, args.state, current_id, suffix, app_facts)
        data_after = _volume_manifests(plan, state, args.state, "data-after", names["data"])
        if data_before != data_after:
            raise ContractError("read-only synthetic data changed")
        receipt = {
            "schema": 1, "status": "passed", "source_commit": plan["source_commit"],
            "run_id": plan["run_id"], "candidate_image_id": plan["candidate_image_id"],
            "candidate_revision": plan["candidate_revision"],
            "baseline_image_id": plan["baseline_image_id"], "baseline_revision": plan["baseline_revision"],
            "helper_image_id": plan["helper_image_id"], "helper_expectations": plan["helper_expectations"],
            "app_images_receipt_sha256": plan["app_images_receipt_sha256"],
            "app_contracts": plan["app_contracts"],
            "source_tree": plan["source_tree"], "target_source_commit": plan["target_source_commit"],
            "target_source_tree": plan["target_source_tree"], "final_source_gate": plan["final_source_gate"],
            "candidate_provenance": plan["candidate_provenance"],
            "resources": ORDINARY_LIMITS, "scenarios": NAS_EXPECTED,
            "filesystem_evidence": {"data_before": data_before, "data_after": data_after,
                                    "config_before_backup": config_before_backup,
                                    "backup": backup_manifest, "restore": restore_manifest,
                                    "facts": {**filesystem_facts, **interrupted_facts}},
            "application_evidence": application_evidence,
            "container_facts": app_facts,
            "resource_facts": state["resources"],
            "invariants": {"fresh_owner_login": True, "uid_1000_config_write": True,
                           "config_source_backup_restore_equal": True, "data_entry_manifest_unchanged": True,
                           "data_content_manifest_unchanged": True, "same_filesystem_hardlink_same_inode": True,
                           "split_filesystem_copy_hash_equal_source_preserved": True,
                           "network_config_refused": "source-contract", "local_config_network_data_allowed": False,
                           "owned_resources_absent": False},
            "protocol_coverage": {"smb": "not tested", "nfs": "not tested"},
            "gaps": ["SMB/NFS transport and local-config-plus-network-data require a reviewed cached user-space protocol service"],
        }
        atomic_new_json(args.receipt, receipt)
        state["phase"] = "checks-passed-cleanup-required"
        atomic_replace_json(args.state, state)
        return {"status": "checks-passed-cleanup-required", "receipt_sha256": sha256_file(args.receipt)}
    except BaseException:
        state["phase"] = "failed-cleanup-required"
        atomic_replace_json(args.state, state)
        raise


def cleanup_nas(args: argparse.Namespace) -> dict[str, Any]:
    plan = _load_plan(args, "nas-filesystem-contract-simulation")
    state = load_bound(args.state, args.state_sha256, "NAS state")
    if state.get("run_id") != plan["run_id"] or state.get("plan_sha256") != args.plan_sha256:
        raise ContractError("NAS state does not match plan")
    unresolved = []
    for intent in _unresolved_creation_intents(state):
        if intent.get("kind") == "container":
            identity = _discover_owned_container(str(intent.get("name")), plan["run_id"])
            found = [identity] if identity else []
        elif intent.get("kind") in ("volume", "network"):
            found = _resource_ids(intent["kind"], name=str(intent.get("name")))
        else:
            found = []
        if len(found) == 1:
            intent.update(status="resolved", id=found[0])
        else:
            intent["status"] = "outcome-unknown"
            unresolved.append(intent)
    if unresolved:
        state["phase"] = "cleanup-outcome-unknown"
        atomic_replace_json(args.state, state)
        raise ContractError("Docker create outcome remains unresolved; cleanup cannot claim absence")
    atomic_replace_json(args.state, state)
    container_ids = list(state.get("active_containers", []))
    for identity in _owned_container_ids(plan["run_id"]):
        if identity not in container_ids:
            container_ids.append(identity)
    for identity in container_ids:
        if isinstance(identity, str) and HEX64.fullmatch(identity):
            _remove_owned_container(identity, plan["run_id"])
            _unregister_container(state, args.state, identity)
    _stable_absence("container", run_id=plan["run_id"])
    for purpose in ("config", "data", "backup", "restore", "empty-data"):
        name = plan["names"][purpose]
        present = run_limited(["docker", "volume", "ls", "--quiet", "--filter", f"name=^{name}$"]).decode().strip()
        if present:
            inspect = _one_inspect("volume", name)
            if inspect.get("Name") != name or (inspect.get("Labels") or {}).get("io.holyarrs.device-simulation") != plan["run_id"]:
                raise ContractError("refusing to remove an unowned NAS volume")
            run_limited(["docker", "volume", "rm", name])
        _stable_absence("volume", name=name)
    network = plan["names"]["network"]
    present = run_limited(["docker", "network", "ls", "--quiet", "--filter", f"name=^{network}$"]).decode().strip()
    if present:
        inspect = _one_inspect("network", network)
        if inspect.get("Name") != network or (inspect.get("Labels") or {}).get("io.holyarrs.device-simulation") != plan["run_id"]:
            raise ContractError("refusing to remove an unowned NAS network")
        run_limited(["docker", "network", "rm", network])
    _stable_absence("network", name=network)
    for kind in ("container", "volume", "network"):
        command = ["docker", kind, "ls"]
        if kind == "container":
            command.append("--all")
        command.extend(["--quiet", "--filter", f"label=io.holyarrs.device-simulation={plan['run_id']}"])
        leftovers = run_limited(command).decode().strip()
        if leftovers:
            raise ContractError(f"owned NAS {kind} resources remain after cleanup")
    if args.receipt is not None and args.receipt.exists():
        if not args.receipt_sha256:
            raise ContractError("NAS receipt hash is required when a receipt exists")
        receipt = load_bound(args.receipt, args.receipt_sha256, "NAS receipt")
        receipt["invariants"]["owned_resources_absent"] = True
        atomic_replace_json(args.receipt, receipt)
        verify_nas(argparse.Namespace(plan=args.plan, plan_sha256=args.plan_sha256,
                                     receipt=args.receipt, receipt_sha256=sha256_file(args.receipt)))
    state["active_containers"] = []
    for intent in state.get("creation_intents", []):
        if isinstance(intent, dict):
            intent["status"] = "cleaned"
    state["phase"] = "complete"
    atomic_replace_json(args.state, state)
    return {"status": "complete", "owned_resources_absent": True}


def parse_timestamp(value: Any) -> dt.datetime:
    if not isinstance(value, str) or not value:
        raise ContractError("receipt timestamp is missing")
    text = value.replace("Z", "+00:00")
    match = re.match(r"^(.*\.\d{6})\d*(\+00:00)$", text)
    if match:
        text = match.group(1) + match.group(2)
    try:
        result = dt.datetime.fromisoformat(text)
    except ValueError as exc:
        raise ContractError("receipt timestamp is invalid") from exc
    if result.tzinfo is None:
        raise ContractError("receipt timestamp must include a timezone")
    return result


def verify_arm(args: argparse.Namespace) -> dict[str, Any]:
    plan = load_bound(args.plan, args.plan_sha256, "ARM plan")
    receipt = load_bound(args.receipt, args.receipt_sha256, "ARM receipt")
    if plan.get("kind") != "pi-like-arm-emulation" or receipt.get("status") != "passed":
        raise ContractError("ARM plan kind or receipt status differs")
    for key in ("source_commit", "target_source_commit", "source_tree", "target_source_tree",
                "final_source_gate", "preparation", "run_id", "architecture", "platform",
                "execution_mode", "idle", "image_id"):
        if receipt.get(key) != plan.get(key):
            raise ContractError(f"ARM receipt does not match plan: {key}")
    if receipt.get("resources") != ARM_LIMITS or receipt.get("container_oom_killed") is not False:
        raise ContractError("ARM resource or OOM evidence differs")
    if receipt.get("container_removed") is not True or receipt.get("owned_resources_absent") is not True:
        raise ContractError("ARM cleanup is not proven")
    result = receipt.get("result")
    if not isinstance(result, dict):
        raise ContractError("ARM result is missing")
    log = receipt.get("log") or {}
    if log.get("path") != "arm-runtime.log" or not isinstance(log.get("bytes"), int):
        raise ContractError("ARM receipt lacks fixed runtime-log evidence")
    runtime_log = args.receipt.parent / "arm-runtime.log"
    try:
        info = runtime_log.lstat()
        attributes = getattr(info, "st_file_attributes", 0)
        reparse = getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400)
        if (not stat.S_ISREG(info.st_mode) or runtime_log.is_symlink()
                or attributes & reparse):
            raise ContractError("ARM runtime log is not a safe regular file")
        raw_log = runtime_log.read_bytes()
    except OSError as exc:
        raise ContractError("ARM runtime log is missing or unreadable") from exc
    if (not 1 <= len(raw_log) <= MAX_OUTPUT or log.get("bytes") != len(raw_log)
            or log.get("sha256_raw") != hashlib.sha256(raw_log).hexdigest()):
        raise ContractError("ARM runtime log length or raw hash differs")
    if _result_from_log(raw_log) != result:
        raise ContractError("ARM runtime log result differs from the receipt")
    for key, expected in ARM_SHAPE.items():
        if result.get(key) != expected:
            raise ContractError(f"ARM workload differs: {key}")
    idle_seconds = IDLE[plan["idle"]]["seconds"]
    if result.get("idle_seconds") != idle_seconds:
        raise ContractError("application-reported idle duration differs")
    elapsed = (parse_timestamp(receipt.get("container_finished_at")) - parse_timestamp(receipt.get("container_started_at"))).total_seconds()
    if elapsed < idle_seconds:
        raise ContractError("real container wall time was shorter than the claimed idle duration")
    if plan["idle"] == "24h":
        supervision = receipt.get("supervision") or {}
        observed = supervision.get("observations")
        if supervision.get("complete") is not True or not isinstance(observed, list) or len(observed) < 288:
            raise ContractError("24-hour claim lacks complete continuous supervision")
        poll_seconds = supervision.get("poll_seconds")
        if not isinstance(poll_seconds, int) or not 30 <= poll_seconds <= 300:
            raise ContractError("24-hour supervision poll interval is invalid")
        points = [parse_timestamp(value.get("wall_utc")) for value in observed if isinstance(value, dict)]
        monotonic = [value.get("monotonic_ns") for value in observed if isinstance(value, dict)]
        if len(points) != len(observed) or any(not isinstance(value, int) for value in monotonic):
            raise ContractError("24-hour supervision observations are invalid")
        if any(right <= left for left, right in zip(monotonic, monotonic[1:])):
            raise ContractError("24-hour supervision monotonic timestamps did not increase")
        if any(right <= left for left, right in zip(points, points[1:])):
            raise ContractError("24-hour supervision wall timestamps did not increase")
        maximum_gap = poll_seconds + 30
        if (any((right - left).total_seconds() > maximum_gap for left, right in zip(points, points[1:]))
                or any((right - left) > maximum_gap * 1_000_000_000
                       for left, right in zip(monotonic, monotonic[1:]))):
            raise ContractError("24-hour supervision contains a gap")
        started = parse_timestamp(receipt.get("container_started_at"))
        finished = parse_timestamp(receipt.get("container_finished_at"))
        start_offset = (points[0] - started).total_seconds()
        finish_offset = (finished - points[-1]).total_seconds()
        wall_span = (points[-1] - points[0]).total_seconds()
        monotonic_span = (monotonic[-1] - monotonic[0]) / 1_000_000_000
        endpoint_tolerance = poll_seconds + 30
        if not 0 <= start_offset <= endpoint_tolerance:
            raise ContractError("24-hour supervision began too late")
        if abs(finish_offset) > endpoint_tolerance:
            raise ContractError("24-hour supervision ended too early")
        if (abs(monotonic_span - wall_span) > endpoint_tolerance
                or monotonic_span < elapsed - 2 * endpoint_tolerance
                or monotonic_span > elapsed + endpoint_tolerance):
            raise ContractError("24-hour supervision monotonic coverage differs from real wall time")
    if result.get("idle_writes") != 0 or result.get("idle_write_bytes") != 0:
        raise ContractError("idle writes were not zero")
    if not 0 < result.get("idle_rss_max", 0) <= 250000000 or not 0 < result.get("peak_rss", 0) <= 500000000:
        raise ContractError("memory thresholds failed")
    if result.get("write_wait_samples", 0) < 100 or not 0 <= result.get("write_wait_p95_ms", -1) < 100:
        raise ContractError("writer-wait threshold failed")
    return {"status": "passed", "claim": plan["claim"], "execution_mode": "emulated",
            "final_source_gate": plan["final_source_gate"], "native_pi": False,
            "wall_seconds": elapsed}


def verify_nas(args: argparse.Namespace) -> dict[str, Any]:
    plan = load_bound(args.plan, args.plan_sha256, "NAS plan")
    receipt = load_bound(args.receipt, args.receipt_sha256, "NAS receipt")
    if plan.get("kind") != "nas-filesystem-contract-simulation" or receipt.get("status") != "passed":
        raise ContractError("NAS plan kind or receipt status differs")
    for key in ("source_commit", "source_tree", "target_source_commit", "target_source_tree",
                "final_source_gate", "candidate_provenance", "run_id", "candidate_image_id",
                "candidate_revision", "baseline_image_id", "baseline_revision", "helper_image_id",
                "helper_expectations", "app_images_receipt_sha256", "app_contracts"):
        if receipt.get(key) != plan.get(key):
            raise ContractError(f"NAS receipt does not match plan: {key}")
    if receipt.get("resources") != ORDINARY_LIMITS:
        raise ContractError("NAS resource evidence differs")
    results = receipt.get("scenarios")
    if not isinstance(results, dict) or set(results) != set(NAS_SCENARIOS):
        raise ContractError("NAS scenario coverage is incomplete")
    if results != NAS_EXPECTED:
        raise ContractError("one or more NAS scenarios failed or overstated coverage")
    evidence = receipt.get("filesystem_evidence") or {}
    before, after = evidence.get("data_before") or {}, evidence.get("data_after") or {}
    source_config = evidence.get("config_before_backup") or {}
    backup, restore = evidence.get("backup") or {}, evidence.get("restore") or {}
    for manifest in (before, after, source_config, backup, restore):
        if set(manifest) != {"entries", "content"}:
            raise ContractError("NAS filesystem manifest evidence is invalid")
        for kind, item in manifest.items():
            if (not isinstance(item, dict) or set(item) != {"sha256", "records"}
                    or not HEX64.fullmatch(str(item.get("sha256")))
                    or not isinstance(item.get("records"), list)
                    or _sanitized_manifest(("\n".join(item["records"]) + ("\n" if item["records"] else "")).encode(), kind) != item):
                raise ContractError("NAS filesystem manifest evidence is invalid")
    if before != after or source_config != backup or backup != restore:
        raise ContractError("NAS filesystem comparison evidence differs")
    facts = evidence.get("facts") or {}
    if (facts.get("hardlink_source") != facts.get("hardlink_target")
            or facts.get("copy_source_device") == facts.get("copy_target_device")
            or facts.get("source_hash") != facts.get("copy_hash")
            or not HEX64.fullmatch(str(facts.get("source_hash")))
            or facts.get("source_preserved") != "true"
            or not HEX64.fullmatch(str(facts.get("interrupted_source_hash")))
            or facts.get("interrupted_partial_removed") != "true"
            or facts.get("interrupted_source_preserved") != "true"):
        raise ContractError("NAS filesystem identity/copy evidence differs")
    container_facts = receipt.get("container_facts")
    expected_roles = ["baseline-first-run", "restart", "update", "restore", "empty"]
    application = receipt.get("application_evidence")
    expected_application = [
        {"role": "baseline-first-run", "owner": "setup-authenticated", "encrypted_setting": "present-masked"},
        *[{"role": role, "owner": "login-authenticated", "encrypted_setting": "present-masked"}
          for role in expected_roles[1:]],
    ]
    if application != expected_application:
        raise ContractError("NAS owner or encrypted-setting evidence differs")
    if not isinstance(container_facts, list) or [fact.get("role") for fact in container_facts] != expected_roles:
        raise ContractError("NAS inspected container evidence is incomplete")
    expected_resource_facts = [
        {"kind": "volume", "name": plan["names"][purpose], "id": plan["names"][purpose],
         "driver": "local", "scope": "local", "internal": None, "labels": plan["labels"]}
        for purpose in ("config", "data", "backup", "restore", "empty-data")
    ] + [{"kind": "network", "name": plan["names"]["network"],
          "id": (receipt.get("resource_facts") or [{}])[-1].get("id"), "driver": "bridge",
          "scope": "local", "internal": True, "labels": plan["labels"]}]
    resource_facts = receipt.get("resource_facts")
    if (resource_facts != expected_resource_facts
            or not isinstance(expected_resource_facts[-1]["id"], str)
            or not HEX64.fullmatch(expected_resource_facts[-1]["id"])):
        raise ContractError("NAS inspected volume or network evidence differs")
    seen = set()
    for fact in container_facts:
        identity = fact.get("id")
        expected_image = plan["baseline_image_id"] if fact["role"] in ("baseline-first-run", "restart") else plan["candidate_image_id"]
        mounts = dict(fact.get("mounts") or [])
        contract = fact.get("contract") or {}
        expected_config = plan["names"]["restore"] if fact["role"] in ("restore", "empty") else plan["names"]["config"]
        expected_data = plan["names"]["empty-data"] if fact["role"] == "empty" else plan["names"]["data"]
        image_contract = plan["app_contracts"].get(expected_image) or {}
        if (not isinstance(identity, str) or not HEX64.fullmatch(identity) or identity in seen
                or fact.get("image_id") != expected_image or fact.get("exit_code") != 0
                or fact.get("oom_killed") is not False
                or parse_timestamp(fact.get("started_at")) < parse_timestamp(fact.get("created"))
                or parse_timestamp(fact.get("finished_at")) < parse_timestamp(fact.get("started_at"))
                or mounts.get("/config") != {"rw": True, "type": "volume", "source": expected_config}
                or mounts.get("/data") != {"rw": False, "type": "volume", "source": expected_data}
                or contract != {"user": "1000:1000", "cmd": image_contract.get("cmd"),
                                "entrypoint": image_contract.get("entrypoint"),
                                "env": image_contract.get("env"),
                                "network_mode": plan["names"]["network"],
                                "memory": ORDINARY_LIMITS["memory_bytes"],
                                "memory_swap": ORDINARY_LIMITS["memory_swap_bytes"],
                                "nano_cpus": 1_000_000_000, "pids_limit": 256,
                                "cap_drop": ["ALL"], "cap_add": None, "privileged": False,
                                "auto_remove": False, "restart": "no",
                                "security": ["no-new-privileges"],
                                "ports": {"18730/tcp": [{"HostIp": "127.0.0.1",
                                                          "HostPort": str(plan["port"])}]}}):
            raise ContractError("NAS inspected container evidence differs")
        seen.add(identity)
    invariants = receipt.get("invariants") or {}
    required = ("fresh_owner_login", "uid_1000_config_write", "config_source_backup_restore_equal",
                "data_entry_manifest_unchanged", "data_content_manifest_unchanged",
                "same_filesystem_hardlink_same_inode", "split_filesystem_copy_hash_equal_source_preserved",
                "owned_resources_absent")
    if any(invariants.get(key) is not True for key in required):
        raise ContractError("NAS invariants are incomplete")
    if invariants.get("local_config_network_data_allowed") is not False:
        raise ContractError("NAS receipt overstates the untested network-data invariant")
    if invariants.get("network_config_refused") != "source-contract":
        raise ContractError("NAS receipt overstates the source-only network-config contract")
    if receipt.get("protocol_coverage") != {"smb": "not tested", "nfs": "not tested"}:
        raise ContractError("NAS receipt overstates protocol coverage")
    return {"status": "passed", "simulation": "filesystem-contract", "nas_certification": False,
            "final_source_gate": plan["final_source_gate"],
            "smb": "not tested", "nfs": "not tested"}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    arm = sub.add_parser("prepare-arm")
    arm.add_argument("--workspace", required=True, type=Path)
    arm.add_argument("--source-commit", required=True)
    arm.add_argument("--source-tree", required=True)
    arm.add_argument("--source-manifest", required=True, type=Path)
    arm.add_argument("--source-manifest-sha256", required=True)
    arm.add_argument("--prepare-receipt", required=True, type=Path)
    arm.add_argument("--prepare-receipt-sha256", required=True)
    arm.add_argument("--preparation-tool-sha256", required=True)
    arm.add_argument("--target-graph-manifest", required=True, type=Path)
    arm.add_argument("--target-graph-manifest-sha256", required=True)
    arm.add_argument("--module-cache-manifest", required=True, type=Path)
    arm.add_argument("--module-cache-manifest-sha256", required=True)
    arm.add_argument("--module-cache-archive", required=True, type=Path)
    arm.add_argument("--module-cache-archive-sha256", required=True)
    arm.add_argument("--container-states-manifest", required=True, type=Path)
    arm.add_argument("--container-states-manifest-sha256", required=True)
    arm.add_argument("--allow-historical-harness", action="store_true")
    arm.add_argument("--architecture", choices=sorted(ARM), required=True)
    arm.add_argument("--idle", choices=sorted(IDLE), required=True)
    arm.add_argument("--run-id", required=True)
    arm.add_argument("--output", required=True, type=Path)
    nas = sub.add_parser("prepare-nas")
    nas.add_argument("--workspace", required=True, type=Path)
    nas.add_argument("--source-commit", required=True)
    nas.add_argument("--source-tree", required=True)
    nas.add_argument("--source-manifest", required=True, type=Path)
    nas.add_argument("--source-manifest-sha256", required=True)
    nas.add_argument("--candidate-image", required=True)
    nas.add_argument("--candidate-revision", required=True)
    nas.add_argument("--candidate-receipt", required=True, type=Path)
    nas.add_argument("--candidate-receipt-sha256", required=True)
    nas.add_argument("--allow-historical-harness", action="store_true")
    nas.add_argument("--baseline-image", required=True)
    nas.add_argument("--baseline-revision", required=True)
    nas.add_argument("--helper-image", required=True)
    nas.add_argument("--helper-receipt", required=True, type=Path)
    nas.add_argument("--helper-receipt-sha256", required=True)
    nas.add_argument("--app-images-receipt", required=True, type=Path)
    nas.add_argument("--app-images-receipt-sha256", required=True)
    nas.add_argument("--port", required=True, type=int)
    nas.add_argument("--run-id", required=True)
    nas.add_argument("--output", required=True, type=Path)
    for name, function in (("verify-arm", verify_arm), ("verify-nas", verify_nas)):
        verify = sub.add_parser(name)
        verify.add_argument("--plan", required=True, type=Path)
        verify.add_argument("--plan-sha256", required=True)
        verify.add_argument("--receipt", required=True, type=Path)
        verify.add_argument("--receipt-sha256", required=True)
        verify.set_defaults(function=function)
    arm.set_defaults(function=prepare_arm)
    nas.set_defaults(function=prepare_nas)
    start = sub.add_parser("start-arm")
    start.add_argument("--plan", required=True, type=Path)
    start.add_argument("--plan-sha256", required=True)
    start.add_argument("--state", required=True, type=Path)
    start.set_defaults(function=start_arm)
    status = sub.add_parser("status-arm")
    status.add_argument("--plan", required=True, type=Path)
    status.add_argument("--plan-sha256", required=True)
    status.add_argument("--state", required=True, type=Path)
    status.add_argument("--state-sha256", required=True)
    status.add_argument("--receipt", required=True, type=Path)
    status.add_argument("--log", required=True, type=Path)
    status.set_defaults(function=status_arm)
    watch = sub.add_parser("watch-arm")
    watch.add_argument("--plan", required=True, type=Path)
    watch.add_argument("--plan-sha256", required=True)
    watch.add_argument("--state", required=True, type=Path)
    watch.add_argument("--state-sha256", required=True)
    watch.add_argument("--poll-seconds", type=int, choices=range(30, 301), default=60)
    watch.set_defaults(function=watch_arm)
    cleanup = sub.add_parser("cleanup-arm")
    cleanup.add_argument("--plan", required=True, type=Path)
    cleanup.add_argument("--plan-sha256", required=True)
    cleanup.add_argument("--state", required=True, type=Path)
    cleanup.add_argument("--state-sha256", required=True)
    cleanup.set_defaults(function=cleanup_arm)
    run_nas_parser = sub.add_parser("run-nas")
    run_nas_parser.add_argument("--plan", required=True, type=Path)
    run_nas_parser.add_argument("--plan-sha256", required=True)
    run_nas_parser.add_argument("--state", required=True, type=Path)
    run_nas_parser.add_argument("--receipt", required=True, type=Path)
    run_nas_parser.set_defaults(function=run_nas)
    cleanup_nas_parser = sub.add_parser("cleanup-nas")
    cleanup_nas_parser.add_argument("--plan", required=True, type=Path)
    cleanup_nas_parser.add_argument("--plan-sha256", required=True)
    cleanup_nas_parser.add_argument("--state", required=True, type=Path)
    cleanup_nas_parser.add_argument("--state-sha256", required=True)
    cleanup_nas_parser.add_argument("--receipt", type=Path)
    cleanup_nas_parser.add_argument("--receipt-sha256")
    cleanup_nas_parser.set_defaults(function=cleanup_nas)
    return parser


def cli() -> None:
    parser = build_parser()
    args = parser.parse_args()
    try:
        value = args.function(args)
    except (ContractError, OSError) as exc:
        parser.exit(1, f"device-simulation: {exc}\n")
    print(json.dumps(value, sort_keys=True))


if __name__ == "__main__":
    cli()
