import argparse
import datetime as dt
import hashlib
import importlib.util
import inspect
import io
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch


HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("device_simulation", HERE / "device_simulation.py")
sim = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(sim)


def write(path, value):
    path.write_text(json.dumps(value, sort_keys=True) + "\n", encoding="utf-8")
    return hashlib.sha256(path.read_bytes()).hexdigest()


def preparation(commit, tree, manifest_hash, tool_hash, build_log_hash, binary_hash, child_logs,
                graph, cache, steps, architecture="arm64"):
    arm64 = architecture == "arm64"
    executor = "/qemu-aarch64-static" if arm64 else "/qemu-arm-static"
    variant = "linux-arm64" if arm64 else "linux-arm-v7"
    environment = sim._qemu_graph_environment(variant)
    executor_hash = "2" * 64
    test_args = ["-test.run=^TestTestCommandReexec$", "-test.v", "-test.count=1", "-test.timeout=2m"]
    entrypoint = [executor, "/holyarrs.test"]
    command = [*entrypoint, *test_args]
    return {
        "schema": "holyarrs.qemu-build.v2",
        "variant": variant,
        "preparation_tool": {"path": "tools/qemu-proof.py", "sha256_raw": tool_hash},
        "source": {"commit": commit, "tree": tree,
                   "manifest": {"path": "source.json", "sha256_raw": manifest_hash},
                   "archive_sha256_raw": "4" * 64,
                   "verified_tree_before": tree, "verified_tree_after": tree},
        "build": {"command": ["go", "test", "-mod=readonly", "-c", "-o",
                              "holyarrs.test", "./cmd/holyarrs"],
                  "environment": environment, "go_version": sim.QEMU_GO_VERSION,
                  "exit_code": 0, "log": {"path": "build.log", "sha256_raw": build_log_hash},
                  "steps": steps},
        "target_graph": {
            "schema": graph["schema"],
            "manifest": {"path": "target-graph.json", "sha256_raw": graph["manifest_hash"]},
            "target": graph["target"], "target_import_path": graph["target_import_path"],
            "tool": graph["tool"], "variant_modules": graph["variants"][variant]["modules"],
            "variant_package_log": graph["variants"][variant]["package_log"],
            "module_union_count": graph["module_union_count"],
            "verified_before_and_after": True,
        },
        "module_cache": {
            "schema": "holyarrs.go-target-cache.v2",
            "archive": {"path": "module-cache.tar", "sha256_raw": cache["archive_hash"]},
            "manifest": {"path": "module-cache.json", "sha256_raw": cache["manifest_hash"]},
            "files": cache["files"], "bytes": cache["bytes"],
            "required_modules": cache["required_modules"],
            "verified_before_and_after": True,
            "generated_inventory": steps["module-inventory"],
        },
        "test_binary": {"path": "holyarrs.test", "sha256_raw": binary_hash},
        "executor": {"mode": "userspace-qemu", "path": executor, "sha256_raw": executor_hash},
        "image": {"tool_image_id": "sha256:" + "6" * 64,
                  "test_image_id": "sha256:" + "7" * 64,
                  "platform": "linux/arm64" if arm64 else "linux/arm/v7",
                  "entrypoint": entrypoint,
                  "extracted_test_binary_sha256_raw": binary_hash,
                  "extracted_executor_sha256_raw": executor_hash},
        "child_proof": {"test": "TestTestCommandReexec",
                        "binding": {"image_id": "sha256:" + "7" * 64,
                                    "test_binary_sha256_raw": binary_hash,
                                    "source_helper_path": "cmd/holyarrs/testexec_test.go",
                                    "source_helper_sha256_raw": "0" * 64,
                                    "test": "TestTestCommandReexec", "args": test_args,
                                    "parent_entrypoint": entrypoint},
                        "missing_executor": {"environment": {"HOLYARRS_TEST_EXEC": None},
                                             "command": command, "exit_code": 1,
                                             "outcome": "exec-format-reproduced",
                                             **child_logs["missing"]},
                        "nonexistent_executor": {
                            "environment": {"HOLYARRS_TEST_EXEC": "/holyarrs-nonexistent-executor"},
                            "command": command, "exit_code": 1,
                            "observed_path": "/holyarrs-nonexistent-executor",
                            "error_class": "not-found", **child_logs["nonexistent"]},
                        "explicit_executor": {"environment": {"HOLYARRS_TEST_EXEC": executor},
                                              "command": command, "exit_code": 0, "result": "PASS",
                                              "executor_sha256_raw": executor_hash,
                                              **child_logs["explicit"]}},
        "evidence": {"output": "fixture", "containers_cleaned": True,
                     "retained_image_id": "sha256:" + "7" * 64},
    }


def qemu_inputs(root, commit, tree, go_mod_hash, go_sum_hash):
    target_import = "github.com/coderluii/holyarrs/cmd/holyarrs"
    modules = {
        "linux-arm64": [{"path": "github.com/example/arm64dep", "version": "v1.2.3"}],
        "linux-arm-v7": [{"path": "golang.org/x/armdep", "version": "v0.4.0"}],
    }
    variants = {}
    raw_logs = {}
    for variant, declared in modules.items():
        dependency = declared[0]
        objects = [
            {"ImportPath": "fmt", "Standard": True},
            {"ImportPath": dependency["path"] + "/pkg",
             "Module": {"Path": dependency["path"], "Version": dependency["version"]}},
            {"ImportPath": target_import,
             "Module": {"Path": "github.com/coderluii/holyarrs", "Main": True},
             "TestGoFiles": ["testexec_test.go"]},
        ]
        raw = b"".join(json.dumps(item, sort_keys=True).encode() + b"\n" for item in objects)
        raw_logs[variant] = raw
        log_path = root / f"target-graph-{variant}.json"
        log_path.write_bytes(raw)
        variants[variant] = {
            "command": ["go", "list", "-mod=readonly", "-deps", "-test", "-json",
                        "./cmd/holyarrs"],
            "environment": sim._qemu_graph_environment(variant), "exit_code": 0,
            "package_log": {"path": log_path.name, "bytes": len(raw),
                            "sha256_raw": hashlib.sha256(raw).hexdigest()},
            "modules": declared,
        }
    graph_value = {
        "schema": "holyarrs.go-target-graph.v1", "source_commit": commit,
        "source_tree": tree, "go_mod_sha256_raw": go_mod_hash,
        "go_sum_sha256_raw": go_sum_hash, "target": "./cmd/holyarrs",
        "target_import_path": target_import,
        "tool": {"image_id": sim.QEMU_TOOL_IMAGE_ID, "go_version": sim.QEMU_GO_VERSION},
        "variants": variants,
    }
    graph_path = root / "target-graph.json"
    graph_hash = write(graph_path, graph_value)
    union = sorted((item["path"], item["version"])
                   for values in modules.values() for item in values)
    cache_members = {}
    for module, version in union:
        prefix = f"cache/download/{sim._qemu_cache_component(module)}/@v/{sim._qemu_cache_component(version)}"
        for suffix in (".info", ".mod", ".zip", ".ziphash"):
            cache_members[prefix + suffix] = f"{module}@{version}{suffix}\n".encode()
    archive_path = root / "module-cache.tar"
    with tarfile.open(archive_path, "w:") as archive:
        for name, raw in sorted(cache_members.items()):
            info = tarfile.TarInfo(name)
            info.size = len(raw)
            archive.addfile(info, io.BytesIO(raw))
    archive_hash = hashlib.sha256(archive_path.read_bytes()).hexdigest()
    cache_files = [{"path": name, "bytes": len(raw),
                    "sha256_raw": hashlib.sha256(raw).hexdigest()}
                   for name, raw in sorted(cache_members.items())]
    cache_value = {
        "schema": "holyarrs.go-target-cache.v2", "source_commit": commit,
        "source_tree": tree, "go_mod_sha256_raw": go_mod_hash,
        "go_sum_sha256_raw": go_sum_hash, "target_graph_sha256_raw": graph_hash,
        "archive_sha256_raw": archive_hash, "files": cache_files,
    }
    cache_path = root / "module-cache.json"
    cache_hash = write(cache_path, cache_value)
    step_raw = {
        "go-version": (sim.QEMU_GO_VERSION + "\n").encode(),
        "target-graph": raw_logs["linux-arm64"],
        "module-download": b"download checksums passed\n",
        "module-inventory": b"inventory passed\n",
        "cross-build": b"cross build passed\n",
    }
    steps = {}
    for name, raw in step_raw.items():
        path = root / f"{name}.log"
        path.write_bytes(raw)
        steps[name] = {"exit_code": 0, "log": {"path": path.name,
                       "sha256_raw": hashlib.sha256(raw).hexdigest()}}
    summary = dict(graph_value)
    summary.update(manifest_hash=graph_hash, module_union_count=len(union))
    cache = {"archive_hash": archive_hash, "manifest_hash": cache_hash,
             "files": len(cache_files), "bytes": sum(len(raw) for raw in cache_members.values()),
             "required_modules": len(union)}
    return summary, cache, steps, graph_path, cache_path, archive_path


def container_states(root, commit, receipt_hash, variant="linux-arm64"):
    prefix = f"holyarrs-qemu-prep-{variant}-{commit[:12]}-"
    records = []
    for index, suffix in enumerate(("build", "extract", "missing", "nonexistent", "positive"), 1):
        name = prefix + suffix
        path = root / f"container-{name}.json"
        identity = f"{index:064x}"
        raw = (json.dumps({"id": identity, "name": name}, sort_keys=True) + "\n").encode()
        path.write_bytes(raw)
        records.append({"path": path.name, "bytes": len(raw),
                        "sha256_raw": hashlib.sha256(raw).hexdigest(),
                        "id": identity, "name": name})
    manifest = root / "container-states.json"
    manifest_hash = write(manifest, {
        "schema": "holyarrs.qemu-container-states.v1", "source_commit": commit,
        "variant": variant, "preparation_receipt_sha256_raw": receipt_hash,
        "files": records,
    })
    return manifest, manifest_hash


def make_arm_plan(root, idle="24h", receipt_commit=None, receipt_tree=None,
                  target_commit=None, target_tree=None, historical=False,
                  missing_outcome="exec-format-reproduced", missing_log_override=None,
                  prepare=True):
    target_commit = target_commit or "a" * 40
    target_tree = target_tree or "b" * 40
    receipt_commit = receipt_commit or target_commit
    receipt_tree = receipt_tree or target_tree
    manifest = root / "source.json"
    go_mod_hash, go_sum_hash = "1" * 64, "3" * 64
    manifest_hash = write(manifest, [
        {"path": "cmd/holyarrs/testexec_test.go", "sha256": "0" * 64},
        {"path": "go.mod", "sha256": go_mod_hash},
        {"path": "go.sum", "sha256": go_sum_hash},
    ])
    (root / "tools").mkdir()
    tool = root / "tools" / "qemu-proof.py"
    tool.write_bytes(b"bounded preparation tool\n")
    build_log = root / "build.log"
    build_log.write_bytes(b"go test build passed\n")
    binary = root / "holyarrs.test"
    binary.write_bytes(b"synthetic arm test binary\n")
    tool_hash = hashlib.sha256(tool.read_bytes()).hexdigest()
    build_log_hash = hashlib.sha256(build_log.read_bytes()).hexdigest()
    binary_hash = hashlib.sha256(binary.read_bytes()).hexdigest()
    missing_raw = missing_log_override or (b"=== RUN TestTestCommandReexec\n--- PASS: TestTestCommandReexec (0.01s)\nPASS\n"
                   if missing_outcome == "implicit-execution-available"
                   else b"fork/exec child: exec format error\n")
    child_raw = {"missing": missing_raw,
                 "nonexistent": b"fork/exec /holyarrs-nonexistent-executor: no such file or directory\n",
                 "explicit": b"=== RUN TestTestCommandReexec\n--- PASS: TestTestCommandReexec (0.01s)\nPASS\n"}
    child_names = {"missing": "proof-missing-executor.log",
                   "nonexistent": "proof-nonexistent-executor.log",
                   "explicit": "proof-explicit-executor.log"}
    child_logs = {}
    for key, raw in child_raw.items():
        (root / child_names[key]).write_bytes(raw)
        child_logs[key] = {"log_path": child_names[key], "log_bytes": len(raw),
                           "log_sha256_raw": hashlib.sha256(raw).hexdigest()}
    graph, cache, steps, graph_path, cache_path, archive_path = qemu_inputs(
        root, receipt_commit, receipt_tree, go_mod_hash, go_sum_hash)
    prep = root / "prep.json"
    prepared = preparation(receipt_commit, receipt_tree, manifest_hash,
                           tool_hash, build_log_hash, binary_hash, child_logs,
                           graph, cache, steps)
    prepared["child_proof"]["missing_executor"]["outcome"] = missing_outcome
    prepared["child_proof"]["missing_executor"]["exit_code"] = (
        0 if missing_outcome == "implicit-execution-available" else 1)
    prep_hash = write(prep, prepared)
    states_path, states_hash = container_states(root, receipt_commit, prep_hash)
    args = argparse.Namespace(
        workspace=root, source_commit=target_commit, source_tree=target_tree,
        source_manifest=manifest, source_manifest_sha256=manifest_hash,
        prepare_receipt=prep, prepare_receipt_sha256=prep_hash,
        preparation_tool_sha256=tool_hash, allow_historical_harness=historical,
        target_graph_manifest=graph_path, target_graph_manifest_sha256=graph["manifest_hash"],
        module_cache_manifest=cache_path, module_cache_manifest_sha256=cache["manifest_hash"],
        module_cache_archive=archive_path, module_cache_archive_sha256=cache["archive_hash"],
        container_states_manifest=states_path, container_states_manifest_sha256=states_hash,
        architecture="arm64", idle=idle, run_id="arm64-soak-0001", output=root / "plan.json")
    return sim.prepare_arm(args) if prepare else args


def rewrite_preparation(args, value):
    args.prepare_receipt_sha256 = write(args.prepare_receipt, value)
    states = json.loads(args.container_states_manifest.read_text(encoding="utf-8"))
    states["preparation_receipt_sha256_raw"] = args.prepare_receipt_sha256
    args.container_states_manifest_sha256 = write(args.container_states_manifest, states)


def arm_created(plan, identity="f" * 64):
    return {
        "Id": identity, "Name": "/" + plan["container_name"], "RestartCount": 0,
        "State": {"Status": "created", "Running": False, "Pid": 0,
                  "StartedAt": "0001-01-01T00:00:00Z",
                  "FinishedAt": "0001-01-01T00:00:00Z"},
        "Image": plan["image_id"], "Mounts": [],
        "Config": {"User": "1000:1000", "Labels": {"io.holyarrs.device-simulation": plan["run_id"]},
                   "Cmd": sim.arm_create_command(plan)[-4:],
                   "Entrypoint": plan["image_defaults"]["entrypoint"],
                   "Env": ["GOMEMLIMIT=384MiB", "HOLYARRS_SCALE=combined",
                           f"HOLYARRS_SCALE_IDLE={plan['idle']}"]},
        "HostConfig": {"NetworkMode": "none", "Memory": 1073741824, "MemorySwap": 1073741824,
                       "NanoCpus": 4_000_000_000, "PidsLimit": 256, "ReadonlyRootfs": True,
                       "CapDrop": ["ALL"], "CapAdd": None, "Privileged": False, "AutoRemove": False,
                       "RestartPolicy": {"Name": "no"}, "SecurityOpt": ["no-new-privileges"],
                       "Tmpfs": {"/tmp": "size=536870912,mode=1777"}, "Binds": []},
    }


def arm_receipt(plan, root):
    start = dt.datetime(2026, 10, 15, tzinfo=dt.timezone.utc)
    idle_seconds = sim.IDLE[plan["idle"]]["seconds"]
    finish = start + dt.timedelta(seconds=idle_seconds)
    receipt = {key: plan[key] for key in (
        "source_commit", "target_source_commit", "source_tree", "target_source_tree",
        "final_source_gate", "preparation", "run_id", "architecture", "platform",
        "execution_mode", "idle", "image_id")}
    receipt.update(
        status="passed", resources=sim.ARM_LIMITS, container_oom_killed=False,
        container_removed=True, owned_resources_absent=True,
        container_started_at=start.isoformat(), container_finished_at=finish.isoformat(),
        result=dict(sim.ARM_SHAPE, idle_seconds=idle_seconds, idle_writes=0, idle_write_bytes=0,
                    idle_rss_max=250_000_000, peak_rss=500_000_000,
                    write_wait_samples=100, write_wait_p95_ms=99.9))
    if plan["idle"] == "24h":
        receipt["supervision"] = {
            "complete": True,
            "poll_seconds": 300,
            "observations": [{"wall_utc": (start + dt.timedelta(minutes=5 * n)).isoformat(),
                              "monotonic_ns": 1_000_000_000 * (1 + 300 * n)}
                             for n in range(289)]}
    raw = b"SCALERESULT " + json.dumps(receipt["result"], sort_keys=True).encode("utf-8") + b"\n"
    (root / "arm-runtime.log").write_bytes(raw)
    receipt["log"] = {"path": "arm-runtime.log", "bytes": len(raw),
                      "sha256_raw": hashlib.sha256(raw).hexdigest()}
    return receipt


def helper_receipt(image):
    return {"id": image, "dockerfile_sha256": "d" * 64,
            "user": "", "entrypoint": ["/bin/sh"], "stdin_transport": True}


def make_nas_plan(root, target_commit=None, actual_commit=None, historical=False, same_image=False):
    target_commit = target_commit or "a" * 40
    actual_commit = actual_commit or target_commit
    helper = "sha256:" + "d" * 64
    helper_path = root / "helper.json"
    helper_hash = write(helper_path, helper_receipt(helper))
    source_manifest = root / "nas-source.json"
    source_manifest_hash = write(source_manifest, [{"path": "cmd/holyarrs/main.go", "sha256": "1" * 64}])
    binary = root / "holyarrs"
    binary.write_bytes(b"synthetic tested holyarrs binary\n")
    test_log = root / "nas-test.log"
    test_log.write_bytes(b"PASS\n")
    candidate_image = "sha256:" + "b" * 64
    baseline_image = candidate_image if same_image else "sha256:" + "c" * 64
    baseline_revision = actual_commit if same_image else "e" * 40
    candidate_receipt = root / "candidate.json"
    candidate_value = {
        "schema": "holyarrs.nas-image.v1",
        "source": {"commit": actual_commit, "tree": "f" * 40,
                   "manifest": {"path": "nas-source.json", "sha256_raw": source_manifest_hash},
                   "verified_tree_before": "f" * 40, "verified_tree_after": "f" * 40},
        "tested_binary": {"path": "holyarrs", "sha256_raw": hashlib.sha256(binary.read_bytes()).hexdigest()},
        "image": {"id": candidate_image,
                  "tested_binary_sha256_raw": hashlib.sha256(binary.read_bytes()).hexdigest()},
        "test": {"exit_code": 0, "log": {"path": "nas-test.log",
                                             "sha256_raw": hashlib.sha256(test_log.read_bytes()).hexdigest()}},
    }
    candidate_hash = write(candidate_receipt, candidate_value)
    app_images_receipt = root / "app-images.json"
    app_images_value = {
        "schema": "holyarrs.nas-app-images.v1",
        "images": {
            candidate_image: {
                "revision": actual_commit,
                "binary_entrypoint": ["/holyarrs"],
                "image_defaults": {"entrypoint": ["/holyarrs"], "cmd": None,
                                   "env": ["GOMEMLIMIT=256MiB"], "user": "1000:1000"},
            },
            baseline_image: {
                "revision": baseline_revision,
                "binary_entrypoint": ["/holyarrs"],
                "image_defaults": {"entrypoint": ["/bin/sh", "-c"], "cmd": ["/holyarrs"],
                                   "env": ["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                                           "TZ=UTC"], "user": ""},
            },
        },
    }
    app_images_hash = write(app_images_receipt, app_images_value)
    if same_image:
        app_images_value["images"][candidate_image] = {
            "revision": actual_commit,
            "binary_entrypoint": ["/holyarrs"],
            "image_defaults": {"entrypoint": ["/holyarrs"], "cmd": None,
                               "env": ["GOMEMLIMIT=256MiB"], "user": "1000:1000"},
        }
        app_images_hash = write(app_images_receipt, app_images_value)
    args = argparse.Namespace(
        workspace=root, source_commit=target_commit, source_tree="f" * 40,
        source_manifest=source_manifest, source_manifest_sha256=source_manifest_hash,
        candidate_image=candidate_image,
        candidate_revision=actual_commit, baseline_image=baseline_image,
        candidate_receipt=candidate_receipt, candidate_receipt_sha256=candidate_hash,
        allow_historical_harness=historical,
        baseline_revision=baseline_revision, helper_image=helper, helper_receipt=helper_path,
        helper_receipt_sha256=helper_hash, port=18746, run_id="nas-sim-0001",
        app_images_receipt=app_images_receipt, app_images_receipt_sha256=app_images_hash,
        output=root / "nas-plan.json")
    return sim.prepare_nas(args)


def manifest(entries=None, content=None):
    entries = entries or "directory|755|1000|1000|0|.\nregular file|644|1000|1000|4|./item\n"
    content = content or ("f" * 64 + "  ./item\n")
    return {"entries": sim._sanitized_manifest(entries.encode(), "entries"),
            "content": sim._sanitized_manifest(content.encode(), "content")}


def nas_receipt(plan):
    common = manifest()
    config = manifest("directory|755|1000|1000|0|.\nregular file|600|1000|1000|4|./db\n",
                      "e" * 64 + "  ./db\n")
    start = "2026-10-15T00:00:01+00:00"
    finish = "2026-10-15T00:00:02+00:00"
    facts = []
    roles = ["baseline-first-run", "restart", "update", "restore", "empty"]
    for index, role in enumerate(roles, 1):
        image_id = plan["baseline_image_id"] if role in roles[:2] else plan["candidate_image_id"]
        app_contract = plan["app_contracts"][image_id]
        config_name = plan["names"]["restore"] if role in ("restore", "empty") else plan["names"]["config"]
        data_name = plan["names"]["empty-data"] if role == "empty" else plan["names"]["data"]
        facts.append({"id": f"{index:x}" * 64, "role": role,
                      "image_id": image_id,
                      "created": "2026-10-15T00:00:00+00:00", "started_at": start,
                      "finished_at": finish, "exit_code": 0, "oom_killed": False,
                      "mounts": [["/config", {"rw": True, "type": "volume", "source": config_name}],
                                 ["/data", {"rw": False, "type": "volume", "source": data_name}]],
                      "contract": {"user": "1000:1000", "cmd": app_contract["cmd"],
                                   "entrypoint": app_contract["entrypoint"],
                                   "env": app_contract["env"],
                                   "network_mode": plan["names"]["network"],
                                   "memory": 2147483648, "memory_swap": 2147483648,
                                   "nano_cpus": 1_000_000_000, "pids_limit": 256,
                                   "cap_drop": ["ALL"], "cap_add": None, "privileged": False,
                                   "auto_remove": False, "restart": "no",
                                   "security": ["no-new-privileges"],
                                   "ports": {"18730/tcp": [{"HostIp": "127.0.0.1",
                                                             "HostPort": "18746"}]}}})
    invariants = {name: True for name in (
        "fresh_owner_login", "uid_1000_config_write", "config_source_backup_restore_equal",
        "data_entry_manifest_unchanged", "data_content_manifest_unchanged",
        "same_filesystem_hardlink_same_inode", "split_filesystem_copy_hash_equal_source_preserved",
        "owned_resources_absent")}
    invariants.update(network_config_refused="source-contract", local_config_network_data_allowed=False)
    return {
        "schema": 1, "status": "passed", "source_commit": plan["source_commit"],
        "source_tree": plan["source_tree"], "target_source_commit": plan["target_source_commit"],
        "target_source_tree": plan["target_source_tree"], "final_source_gate": plan["final_source_gate"],
        "candidate_provenance": plan["candidate_provenance"],
        "run_id": plan["run_id"], "candidate_image_id": plan["candidate_image_id"],
        "candidate_revision": plan["candidate_revision"], "baseline_image_id": plan["baseline_image_id"],
        "baseline_revision": plan["baseline_revision"], "helper_image_id": plan["helper_image_id"],
        "helper_expectations": plan["helper_expectations"], "resources": sim.ORDINARY_LIMITS,
        "app_images_receipt_sha256": plan["app_images_receipt_sha256"],
        "app_contracts": plan["app_contracts"],
        "scenarios": sim.NAS_EXPECTED,
        "filesystem_evidence": {"data_before": common, "data_after": common,
                                "config_before_backup": config, "backup": config, "restore": config,
                                "facts": {"hardlink_source": "1:2", "hardlink_target": "1:2",
                                          "copy_source_device": "1", "copy_target_device": "2",
                                          "source_hash": "a" * 64, "copy_hash": "a" * 64,
                                          "source_preserved": "true", "interrupted_source_hash": "b" * 64,
                                          "interrupted_partial_removed": "true",
                                          "interrupted_source_preserved": "true"}},
        "application_evidence": [
            {"role": "baseline-first-run", "owner": "setup-authenticated",
             "encrypted_setting": "present-masked"},
            *[{"role": role, "owner": "login-authenticated", "encrypted_setting": "present-masked"}
              for role in roles[1:]],
        ],
        "container_facts": facts,
        "resource_facts": [
            *[{"kind": "volume", "name": plan["names"][purpose], "id": plan["names"][purpose],
               "driver": "local", "scope": "local", "internal": None, "labels": plan["labels"]}
              for purpose in ("config", "data", "backup", "restore", "empty-data")],
            {"kind": "network", "name": plan["names"]["network"], "id": "6" * 64,
             "driver": "bridge", "scope": "local", "internal": True, "labels": plan["labels"]},
        ],
        "invariants": invariants,
        "protocol_coverage": {"smb": "not tested", "nfs": "not tested"}}


class DeviceSimulationTests(unittest.TestCase):
    def test_arm_plan_binds_source_tool_binary_executor_and_24h(self):
        with tempfile.TemporaryDirectory() as directory:
            plan = make_arm_plan(Path(directory))
            self.assertTrue(plan["final_source_gate"])
            self.assertEqual(plan["minimum_real_wall_seconds"], 86400)
            self.assertRegex(plan["preparation"]["tool_sha256_raw"], r"^[0-9a-f]{64}$")
            self.assertEqual(plan["preparation"]["executor_mode"], "userspace-qemu")
            self.assertEqual(plan["preparation"]["target_graph_module_union_count"], 2)
            self.assertEqual(plan["preparation"]["container_state_files"], 5)
            self.assertRegex(plan["preparation"]["module_cache_archive_sha256_raw"],
                             r"^[0-9a-f]{64}$")
            self.assertEqual(plan["create_command"], sim.arm_create_command(plan))

    def test_arm_plan_accepts_recorded_implicit_execution_without_changing_handlers(self):
        with tempfile.TemporaryDirectory() as directory:
            plan = make_arm_plan(Path(directory), missing_outcome="implicit-execution-available")
            self.assertTrue(plan["final_source_gate"])
            self.assertEqual(plan["preparation"]["executor_mode"], "userspace-qemu")

    def test_arm_child_logs_are_real_bounded_and_classified(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(sim.ContractError, "exact PASS"):
                make_arm_plan(Path(directory), missing_outcome="implicit-execution-available",
                              missing_log_override=b"--- FAIL: TestTestCommandReexec\nFAIL\n")

    def test_arm_v2_receipt_rejects_schema_graph_steps_and_cleanup_drift(self):
        mutations = ("schema", "selected-modules", "steps", "cleanup")
        for mutation in mutations:
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as directory:
                args = make_arm_plan(Path(directory), prepare=False)
                receipt = json.loads(args.prepare_receipt.read_text(encoding="utf-8"))
                if mutation == "schema":
                    receipt["schema"] = "holyarrs.qemu-build.v1"
                elif mutation == "selected-modules":
                    receipt["target_graph"]["variant_modules"] = []
                elif mutation == "steps":
                    receipt["build"]["steps"]["module-verify"] = receipt["build"]["steps"].pop("target-graph")
                else:
                    receipt["evidence"]["containers_cleaned"] = False
                rewrite_preparation(args, receipt)
                with self.assertRaises(sim.ContractError):
                    sim.prepare_arm(args)

    def test_arm_target_graph_rejects_errors_missing_target_test_and_architecture(self):
        target = "github.com/coderluii/holyarrs/cmd/holyarrs"
        base = {"ImportPath": target,
                "Module": {"Path": "github.com/coderluii/holyarrs", "Main": True},
                "TestGoFiles": ["testexec_test.go"]}
        for mutation in ("deps-errors", "missing-test"):
            with self.subTest(mutation=mutation):
                dependency = {"ImportPath": "github.com/example/dep/pkg",
                              "Module": {"Path": "github.com/example/dep", "Version": "v1.0.0"}}
                changed = dict(base)
                if mutation == "deps-errors":
                    dependency = dict(dependency, DepsErrors=[])
                else:
                    changed["TestGoFiles"] = []
                raw = (json.dumps(dependency) + "\n" + json.dumps(changed) + "\n").encode()
                with self.assertRaises(sim.ContractError):
                    sim._qemu_package_modules(raw, target)
        with tempfile.TemporaryDirectory() as directory:
            args = make_arm_plan(Path(directory), prepare=False)
            graph = json.loads(args.target_graph_manifest.read_text(encoding="utf-8"))
            del graph["variants"]["linux-arm-v7"]
            args.target_graph_manifest_sha256 = write(args.target_graph_manifest, graph)
            with self.assertRaisesRegex(sim.ContractError, "variant set"):
                sim._load_qemu_target_graph(args, args.source_commit, args.source_tree,
                                            args.source_manifest)

    def test_arm_cache_and_container_state_inputs_are_exact(self):
        with tempfile.TemporaryDirectory() as directory:
            args = make_arm_plan(Path(directory), prepare=False)
            graph = sim._load_qemu_target_graph(args, args.source_commit, args.source_tree,
                                                args.source_manifest)
            with tarfile.open(args.module_cache_archive, "r:") as archive:
                members = [(member.name, archive.extractfile(member).read())
                           for member in archive.getmembers()]
            with tarfile.open(args.module_cache_archive, "w:") as archive:
                for name, raw in members[:-1]:
                    info = tarfile.TarInfo(name)
                    info.size = len(raw)
                    archive.addfile(info, io.BytesIO(raw))
            args.module_cache_archive_sha256 = hashlib.sha256(
                args.module_cache_archive.read_bytes()).hexdigest()
            cache = json.loads(args.module_cache_manifest.read_text(encoding="utf-8"))
            cache["archive_sha256_raw"] = args.module_cache_archive_sha256
            args.module_cache_manifest_sha256 = write(args.module_cache_manifest, cache)
            with self.assertRaisesRegex(sim.ContractError, "incomplete"):
                sim._verify_qemu_cache(args, graph, args.source_commit, args.source_tree,
                                       args.source_manifest)

        with tempfile.TemporaryDirectory() as directory:
            args = make_arm_plan(Path(directory), prepare=False)
            states = json.loads(args.container_states_manifest.read_text(encoding="utf-8"))
            states["files"][1]["id"] = states["files"][0]["id"]
            args.container_states_manifest_sha256 = write(args.container_states_manifest, states)
            with self.assertRaisesRegex(sim.ContractError, "state record"):
                sim.prepare_arm(args)

        with tempfile.TemporaryDirectory() as directory:
            args = make_arm_plan(Path(directory), prepare=False)
            args.target_graph_manifest_sha256 = "0" * 64
            with self.assertRaisesRegex(sim.ContractError, "changed"):
                sim.prepare_arm(args)

    def test_arm_v2_inputs_reject_linked_or_reparse_ancestors(self):
        def rejects_reparse(args, blocked, operation):
            with patch.object(
                    sim, "_qemu_reparse",
                    side_effect=lambda path, info: Path(path) == blocked):
                with self.assertRaisesRegex(sim.ContractError, "linked or reparse ancestor"):
                    operation()

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            rejects_reparse(args, root, lambda: sim.prepare_arm(args))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            parent = root / "graph-manifest-parent"
            parent.mkdir()
            args.target_graph_manifest = args.target_graph_manifest.replace(
                parent / args.target_graph_manifest.name)
            rejects_reparse(
                args, parent,
                lambda: sim._load_qemu_target_graph(
                    args, args.source_commit, args.source_tree, args.source_manifest))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            graph = json.loads(args.target_graph_manifest.read_text(encoding="utf-8"))
            parent = root / "graph-log-parent"
            parent.mkdir()
            record = graph["variants"]["linux-arm64"]["package_log"]
            source = root / record["path"]
            destination = parent / source.name
            source.replace(destination)
            record["path"] = destination.relative_to(root).as_posix()
            args.target_graph_manifest_sha256 = write(args.target_graph_manifest, graph)
            rejects_reparse(
                args, parent,
                lambda: sim._load_qemu_target_graph(
                    args, args.source_commit, args.source_tree, args.source_manifest))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            graph = sim._load_qemu_target_graph(
                args, args.source_commit, args.source_tree, args.source_manifest)
            parent = root / "cache-manifest-parent"
            parent.mkdir()
            args.module_cache_manifest = args.module_cache_manifest.replace(
                parent / args.module_cache_manifest.name)
            rejects_reparse(
                args, parent,
                lambda: sim._verify_qemu_cache(
                    args, graph, args.source_commit, args.source_tree, args.source_manifest))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            graph = sim._load_qemu_target_graph(
                args, args.source_commit, args.source_tree, args.source_manifest)
            parent = root / "cache-archive-parent"
            parent.mkdir()
            args.module_cache_archive = args.module_cache_archive.replace(
                parent / args.module_cache_archive.name)
            rejects_reparse(
                args, parent,
                lambda: sim._verify_qemu_cache(
                    args, graph, args.source_commit, args.source_tree, args.source_manifest))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            parent = root / "state-manifest-parent"
            parent.mkdir()
            args.container_states_manifest = args.container_states_manifest.replace(
                parent / args.container_states_manifest.name)
            rejects_reparse(
                args, parent,
                lambda: sim._verify_qemu_container_states(
                    args, args.source_commit, "linux-arm64"))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = make_arm_plan(root, prepare=False)
            states = json.loads(args.container_states_manifest.read_text(encoding="utf-8"))
            blocked = root / states["files"][0]["path"]
            rejects_reparse(
                args, blocked,
                lambda: sim._verify_qemu_container_states(
                    args, args.source_commit, "linux-arm64"))

    def test_arm_v2_inputs_reject_real_symlink_ancestor(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            real = root / "real"
            real.mkdir()
            evidence = real / "evidence.json"
            evidence.write_bytes(b"{}\n")
            linked = root / "linked"
            os.symlink(real, linked, target_is_directory=True)
            digest = hashlib.sha256(evidence.read_bytes()).hexdigest()
            with self.assertRaisesRegex(sim.ContractError, "linked or reparse ancestor"):
                sim._qemu_read(linked / evidence.name, digest, root,
                               "ARM linked evidence", 4096)

    def test_historical_arm_harness_is_explicit_and_never_final_source(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaisesRegex(sim.ContractError, "intended source"):
                make_arm_plan(root, receipt_commit="c" * 40, receipt_tree="d" * 40)
        with tempfile.TemporaryDirectory() as directory:
            plan = make_arm_plan(Path(directory), receipt_commit="c" * 40,
                                 receipt_tree="d" * 40, historical=True)
            self.assertFalse(plan["final_source_gate"])
            self.assertIn("historical", plan["claim"])

    def test_arm_exact_inspection_rejects_command_security_and_tmpfs_tampering(self):
        with tempfile.TemporaryDirectory() as directory:
            plan = make_arm_plan(Path(directory), idle="30m")
            for mutation in ("cmd", "security", "tmpfs"):
                created = arm_created(plan)
                if mutation == "cmd":
                    created["Config"]["Cmd"] = ["-test.run=."]
                elif mutation == "security":
                    created["HostConfig"]["SecurityOpt"] = []
                else:
                    created["HostConfig"]["Tmpfs"] = {}
                with self.assertRaisesRegex(sim.ContractError, "exact plan"):
                    sim._arm_exact_created(plan, created)

    def test_arm_first_start_requires_exact_new_container_identity_and_state(self):
        with tempfile.TemporaryDirectory() as directory:
            plan = make_arm_plan(Path(directory), idle="30m")
            identity = "f" * 64
            sim._arm_exact_created(plan, arm_created(plan, identity), identity, require_new=True)
            for mutation in ("id", "name", "exited"):
                created = arm_created(plan, identity)
                if mutation == "id":
                    created["Id"] = "e" * 64
                elif mutation == "name":
                    created["Name"] = "/wrong"
                else:
                    created["State"].update(Status="exited", FinishedAt="2026-10-16T00:00:00Z")
                with self.assertRaisesRegex(sim.ContractError, "newly-created"):
                    sim._arm_exact_created(plan, created, identity, require_new=True)

    def test_arm_24h_requires_real_time_and_strict_monotonic_supervision(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_arm_plan(root)
            receipt = arm_receipt(plan, root)
            ph = hashlib.sha256((root / "plan.json").read_bytes()).hexdigest()
            rp = root / "receipt.json"
            rh = write(rp, receipt)
            args = argparse.Namespace(plan=root / "plan.json", plan_sha256=ph,
                                      receipt=rp, receipt_sha256=rh)
            self.assertEqual(sim.verify_arm(args)["wall_seconds"], 86400)
            receipt["supervision"]["observations"][4]["monotonic_ns"] = receipt["supervision"]["observations"][3]["monotonic_ns"]
            rh = write(root / "bad.json", receipt)
            args.receipt, args.receipt_sha256 = root / "bad.json", rh
            with self.assertRaisesRegex(sim.ContractError, "monotonic"):
                sim.verify_arm(args)
            self.assertGreater((86400 + 3600) // 30, 2000)

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_arm_plan(root)
            receipt = arm_receipt(plan, root)
            for index, observation in enumerate(receipt["supervision"]["observations"]):
                observation["monotonic_ns"] = index + 1
            ph = hashlib.sha256((root / "plan.json").read_bytes()).hexdigest()
            rh = write(root / "tiny-span.json", receipt)
            with self.assertRaisesRegex(sim.ContractError, "coverage"):
                sim.verify_arm(argparse.Namespace(plan=root / "plan.json", plan_sha256=ph,
                                                  receipt=root / "tiny-span.json", receipt_sha256=rh))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_arm_plan(root)
            receipt = arm_receipt(plan, root)
            for index, observation in enumerate(receipt["supervision"]["observations"]):
                observation["monotonic_ns"] = 1_000_000_000 * (1 + 200 * index)
            ph = hashlib.sha256((root / "plan.json").read_bytes()).hexdigest()
            rh = write(root / "clock-drift.json", receipt)
            with self.assertRaisesRegex(sim.ContractError, "coverage"):
                sim.verify_arm(argparse.Namespace(plan=root / "plan.json", plan_sha256=ph,
                                                  receipt=root / "clock-drift.json", receipt_sha256=rh))

    def test_arm_verifier_requires_retained_log_matching_result(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_arm_plan(root, idle="30m")
            receipt = arm_receipt(plan, root)
            plan_hash = hashlib.sha256((root / "plan.json").read_bytes()).hexdigest()
            receipt_path = root / "receipt.json"
            receipt_hash = write(receipt_path, receipt)
            args = argparse.Namespace(plan=root / "plan.json", plan_sha256=plan_hash,
                                      receipt=receipt_path, receipt_sha256=receipt_hash)
            self.assertEqual(sim.verify_arm(args)["status"], "passed")

            original = (root / "arm-runtime.log").read_bytes()
            (root / "arm-runtime.log").unlink()
            with self.assertRaisesRegex(sim.ContractError, "missing or unreadable"):
                sim.verify_arm(args)

            (root / "arm-runtime.log").write_bytes(original + b"tampered\n")
            with self.assertRaisesRegex(sim.ContractError, "length or raw hash"):
                sim.verify_arm(args)

            different = dict(receipt["result"])
            different["idle_rss_max"] -= 1
            raw = b"SCALERESULT " + json.dumps(different, sort_keys=True).encode("utf-8") + b"\n"
            (root / "arm-runtime.log").write_bytes(raw)
            receipt["log"].update(bytes=len(raw), sha256_raw=hashlib.sha256(raw).hexdigest())
            args.receipt_sha256 = write(receipt_path, receipt)
            with self.assertRaisesRegex(sim.ContractError, "result differs"):
                sim.verify_arm(args)

    def test_run_limited_rejects_output_beyond_cap(self):
        with patch.object(sim, "MAX_OUTPUT", 128):
            with self.assertRaisesRegex(sim.ContractError, "output limit|exceeded limit"):
                sim.run_limited([sys.executable, "-c", "print('x' * 4096)"], timeout=5)
        with patch.object(sim, "MAX_OUTPUT", 128):
            retained = bytearray()
            exceeded = sim.threading.Event()
            sim._capture_bounded(io.BytesIO(b"x" * 4096), retained, exceeded)
            self.assertTrue(exceeded.is_set())
            self.assertEqual(len(retained), 128)

    def test_delayed_container_visibility_is_polled_and_bound(self):
        identity = "f" * 64
        current = {"Id": identity, "Name": "/owned-name",
                   "Config": {"Labels": {"io.holyarrs.device-simulation": "nas-sim-0001"}}}
        with patch.object(sim, "_resource_ids", side_effect=[[], [], [identity]]), \
                patch.object(sim, "_one_inspect", return_value=current), \
                patch.object(sim.time, "sleep"):
            self.assertEqual(sim._discover_owned_container("owned-name", "nas-sim-0001"), identity)

    def test_ambiguous_create_without_visible_id_remains_durably_unresolved(self):
        with tempfile.TemporaryDirectory() as directory:
            state_path = Path(directory) / "state.json"
            state = {"active_containers": [], "container_history": [], "creation_intents": []}
            with patch.object(sim, "_discover_owned_container", side_effect=[None, None]), \
                    patch.object(sim, "run_limited", side_effect=sim.ContractError("ambiguous create")):
                with self.assertRaisesRegex(sim.ContractError, "ambiguous create"):
                    sim._create_owned_container({"run_id": "nas-sim-0001"}, state, state_path,
                                                "late-container", ["docker", "create"])
            persisted = json.loads(state_path.read_text(encoding="utf-8"))
            self.assertEqual(persisted["creation_intents"][-1]["status"], "outcome-unknown")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_arm_plan(root, idle="30m")
            plan_hash = hashlib.sha256((root / "plan.json").read_bytes()).hexdigest()
            identity = "f" * 64
            state_path = root / "late-state.json"
            state_hash = write(state_path, {
                "schema": 1, "kind": "arm-run-state", "run_id": plan["run_id"],
                "plan_sha256": plan_hash, "container_id": None, "status": "create-outcome-unknown",
                "creation_intent": {"kind": "container", "name": plan["container_name"],
                                    "status": "outcome-unknown", "id": None},
            })
            with patch.object(sim, "_discover_owned_container", return_value=identity), \
                    patch.object(sim, "_owned_container_ids", return_value=[identity]), \
                    patch.object(sim, "_remove_owned_container") as remove, \
                    patch.object(sim, "_stable_absence"):
                result = sim.cleanup_arm(argparse.Namespace(plan=root / "plan.json",
                                                            plan_sha256=plan_hash,
                                                            state=state_path,
                                                            state_sha256=state_hash))
            self.assertEqual(result["status"], "removed")
            remove.assert_called_once_with(identity, plan["run_id"])
            self.assertEqual(json.loads(state_path.read_text(encoding="utf-8"))["creation_intent"]["status"],
                             "cleaned")

        with tempfile.TemporaryDirectory() as directory:
            state_path = Path(directory) / "state.json"
            state = {"resources": [], "creation_intents": []}
            state_path.write_text("{}\n", encoding="utf-8")
            with patch.object(sim, "_stable_absence"), \
                    patch.object(sim, "run_limited", side_effect=sim.ContractError("ambiguous create")), \
                    patch.object(sim, "_resource_ids", return_value=[]), \
                    patch.object(sim, "DISCOVERY_SECONDS", -1):
                with self.assertRaisesRegex(sim.ContractError, "did not expose"):
                    sim._docker_create_owned("volume", "late-volume", "nas-sim-0001", state, state_path)
            persisted = json.loads(state_path.read_text(encoding="utf-8"))
            self.assertEqual(persisted["creation_intents"][-1]["status"], "outcome-unknown")

    def test_ambiguous_container_create_is_discovered_and_persisted(self):
        identity = "f" * 64
        with tempfile.TemporaryDirectory() as directory:
            state_path = Path(directory) / "state.json"
            state = {"active_containers": [], "container_history": []}
            with patch.object(sim, "_discover_owned_container", side_effect=[None, identity]), \
                    patch.object(sim, "run_limited", side_effect=sim.ContractError("create failed")):
                with self.assertRaisesRegex(sim.ContractError, "create failed"):
                    sim._create_owned_container({"run_id": "nas-sim-0001"}, state, state_path,
                                                "owned-name", ["docker", "create"])
            self.assertEqual(state["active_containers"], [identity])
            self.assertTrue(state_path.is_file())

    def test_arm_failed_create_discovers_persists_and_cleans_owned_id(self):
        identity = "f" * 64
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_arm_plan(root, idle="30m")
            plan_hash = hashlib.sha256((root / "plan.json").read_bytes()).hexdigest()
            state_path = root / "state.json"
            image = {"Id": plan["image_id"], "Architecture": "arm64", "Variant": None,
                     "Config": {"Env": [], "Entrypoint": plan["image_defaults"]["entrypoint"]}}
            with patch.object(sim, "_one_inspect", return_value=image), \
                    patch.object(sim, "run_limited", side_effect=sim.ContractError("ambiguous create")), \
                    patch.object(sim, "_discover_owned_container", side_effect=[None, identity]), \
                    patch.object(sim, "_remove_owned_container") as remove:
                with self.assertRaisesRegex(sim.ContractError, "ambiguous create"):
                    sim.start_arm(argparse.Namespace(plan=root / "plan.json", plan_sha256=plan_hash,
                                                     state=state_path))
            state = json.loads(state_path.read_text(encoding="utf-8"))
            self.assertEqual(state["container_id"], identity)
            self.assertEqual(state["status"], "create-failed-cleaned")
            remove.assert_called_once_with(identity, plan["run_id"])

    def test_nas_plan_binds_candidate_baseline_and_helper(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_nas_plan(root)
            self.assertEqual(plan["candidate_revision"], plan["source_commit"])
            self.assertTrue(plan["final_source_gate"])
            self.assertRegex(plan["candidate_provenance"]["tested_binary_sha256_raw"], r"^[0-9a-f]{64}$")
            self.assertEqual(plan["helper_expectations"]["dockerfile_sha256"], "d" * 64)
            self.assertRegex(plan["app_images_receipt_sha256"], r"^[0-9a-f]{64}$")
            baseline = plan["app_contracts"][plan["baseline_image_id"]]
            candidate_contract = plan["app_contracts"][plan["candidate_image_id"]]
            self.assertEqual(baseline["entrypoint"], ["/holyarrs"])
            self.assertEqual(baseline["cmd"], ["-config", "/config", "-addr", ":18730"])
            self.assertEqual(baseline["env"], [
                "GOMEMLIMIT=384MiB", "HOLYARRS_SAVE_PATH=/data/downloads",
                "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                "TZ=UTC"])
            self.assertEqual(candidate_contract["env"],
                             ["GOMEMLIMIT=384MiB", "HOLYARRS_SAVE_PATH=/data/downloads"])
            self.assertEqual(plan["protocol_coverage"], {"smb": "not tested", "nfs": "not tested"})
            bad = argparse.Namespace(
                workspace=root, source_commit="a" * 40, source_tree="f" * 40,
                source_manifest=root / "nas-source.json",
                source_manifest_sha256=hashlib.sha256((root / "nas-source.json").read_bytes()).hexdigest(),
                candidate_image="sha256:" + "b" * 64,
                candidate_revision="9" * 40, baseline_image="sha256:" + "c" * 64,
                candidate_receipt=root / "candidate.json",
                candidate_receipt_sha256=hashlib.sha256((root / "candidate.json").read_bytes()).hexdigest(),
                allow_historical_harness=False,
                baseline_revision="e" * 40, helper_image="sha256:" + "d" * 64,
                helper_receipt=root / "helper.json",
                helper_receipt_sha256=hashlib.sha256((root / "helper.json").read_bytes()).hexdigest(),
                app_images_receipt=root / "app-images.json",
                app_images_receipt_sha256=hashlib.sha256((root / "app-images.json").read_bytes()).hexdigest(),
                port=18746, run_id="nas-sim-0002", output=root / "bad.json")
            with self.assertRaisesRegex(sim.ContractError, "revision binding"):
                sim.prepare_nas(bad)

            candidate = json.loads((root / "candidate.json").read_text(encoding="utf-8"))
            candidate["tested_binary"]["sha256_raw"] = "9" * 64
            bad.candidate_revision = "a" * 40
            bad.candidate_receipt_sha256 = write(root / "candidate-tampered.json", candidate)
            bad.candidate_receipt = root / "candidate-tampered.json"
            with self.assertRaisesRegex(sim.ContractError, "tested-binary binding|tested binary"):
                sim.prepare_nas(bad)

            app_images = json.loads((root / "app-images.json").read_text(encoding="utf-8"))
            app_images["images"]["sha256:" + "c" * 64]["image_defaults"]["env"].append("SECRET=value")
            bad.candidate_receipt = root / "candidate.json"
            bad.candidate_receipt_sha256 = hashlib.sha256(bad.candidate_receipt.read_bytes()).hexdigest()
            bad.app_images_receipt = root / "app-images-unexpected-env.json"
            bad.app_images_receipt_sha256 = write(bad.app_images_receipt, app_images)
            with self.assertRaisesRegex(sim.ContractError, "environment is invalid"):
                sim.prepare_nas(bad)

            app_images = json.loads((root / "app-images.json").read_text(encoding="utf-8"))
            app_images["images"]["sha256:" + "c" * 64]["image_defaults"]["env"].append("TZ=Etc/UTC")
            bad.app_images_receipt = root / "app-images-duplicate-env.json"
            bad.app_images_receipt_sha256 = write(bad.app_images_receipt, app_images)
            with self.assertRaisesRegex(sim.ContractError, "environment is invalid"):
                sim.prepare_nas(bad)

        with tempfile.TemporaryDirectory() as directory:
            plan = make_nas_plan(Path(directory), target_commit="a" * 40,
                                 actual_commit="9" * 40, historical=True)
            self.assertFalse(plan["final_source_gate"])
            self.assertEqual(plan["source_commit"], "9" * 40)
            self.assertIn("historical", plan["claim"])

        with tempfile.TemporaryDirectory() as directory:
            plan = make_nas_plan(Path(directory), same_image=True)
            self.assertEqual(plan["baseline_image_id"], plan["candidate_image_id"])
            self.assertEqual(len(plan["app_contracts"]), 1)

    def test_network_resource_requires_full_identity_labels_and_internal_mode(self):
        identity = "6" * 64
        current = {"Id": identity, "Name": "owned-network", "Driver": "bridge", "Scope": "local",
                   "Internal": False, "Labels": {"io.holyarrs.device-simulation": "nas-sim-0001"}}
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(sim, "_stable_absence"), \
                patch.object(sim, "run_limited", return_value=b""), \
                patch.object(sim, "_resource_ids", return_value=[identity]), \
                patch.object(sim, "_one_inspect", return_value=current):
            with self.assertRaisesRegex(sim.ContractError, "exact plan"):
                sim._docker_create_owned("network", "owned-network", "nas-sim-0001", {},
                                         Path(directory) / "state.json")

    def test_setup_code_reader_uses_owned_create_and_readonly_config_mount(self):
        plan = {"run_id": "nas-sim-0001", "helper_image_id": "sha256:" + "d" * 64}
        identity = "f" * 64
        script = "cat /config/setup-code.txt"
        created = {
            "Id": identity, "Name": "/holyarrs-device-helper-read-setup-code-nas-sim-0001",
            "RestartCount": 0,
            "State": {"Status": "created", "Running": False, "Pid": 0,
                      "StartedAt": "0001-01-01T00:00:00Z",
                      "FinishedAt": "0001-01-01T00:00:00Z"},
            "Image": plan["helper_image_id"],
            "Config": {"User": "1000:1000", "Cmd": ["-eu", "-c", script],
                       "Entrypoint": ["sh"], "Labels": {"io.holyarrs.device-simulation": plan["run_id"]}},
            "HostConfig": {"NetworkMode": "none", "Memory": 268435456, "MemorySwap": 268435456,
                           "NanoCpus": 1_000_000_000, "PidsLimit": 128, "CapDrop": ["ALL"],
                           "CapAdd": None, "Privileged": False, "AutoRemove": False,
                           "RestartPolicy": {"Name": "no"}, "SecurityOpt": ["no-new-privileges"]},
            "Mounts": [{"Destination": "/config", "RW": False, "Type": "volume", "Name": "config"}]}
        stopped = {"State": {"ExitCode": 0}}
        state = {"active_containers": [identity]}
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(sim, "_create_owned_container", return_value=identity) as create, \
                patch.object(sim, "_one_inspect", side_effect=[created, stopped]), \
                patch.object(sim, "run_limited", return_value=b"setup-code\n"), \
                patch.object(sim, "_remove_owned_container"), \
                patch.object(sim, "_unregister_container"):
            value = sim._helper(plan, state, Path(directory) / "state.json", "read-setup-code",
                                ["type=volume,src=config,dst=/config,readonly"], script,
                                user="1000:1000")
        command = create.call_args.args[-1]
        self.assertEqual(command[:2], ["docker", "create"])
        self.assertNotIn("--rm", command)
        self.assertEqual(value, b"setup-code\n")

        for field, value in (("Name", "wrong"), ("RW", True)):
            created["Mounts"] = [{"Destination": "/config", "RW": False,
                                   "Type": "volume", "Name": "config"}]
            created["Mounts"][0][field] = value
            with tempfile.TemporaryDirectory() as directory, \
                    patch.object(sim, "_create_owned_container", return_value=identity), \
                    patch.object(sim, "_one_inspect", return_value=created), \
                    patch.object(sim, "run_limited") as start, \
                    patch.object(sim, "_remove_owned_container"), \
                    patch.object(sim, "_unregister_container"):
                with self.assertRaisesRegex(sim.ContractError, "differs from its plan"):
                    sim._helper(plan, state, Path(directory) / "state.json", "read-setup-code",
                                ["type=volume,src=config,dst=/config,readonly"], script,
                                user="1000:1000")
                start.assert_not_called()

        created["Mounts"] = [
            {"Destination": "/config", "RW": False, "Type": "volume", "Name": "wrong"},
            {"Destination": "/config", "RW": False, "Type": "volume", "Name": "config"},
        ]
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(sim, "_create_owned_container", return_value=identity), \
                patch.object(sim, "_one_inspect", return_value=created), \
                patch.object(sim, "run_limited") as start, \
                patch.object(sim, "_remove_owned_container"), \
                patch.object(sim, "_unregister_container"):
            with self.assertRaisesRegex(sim.ContractError, "duplicate or invalid"):
                sim._helper(plan, state, Path(directory) / "state.json", "read-setup-code",
                            ["type=volume,src=config,dst=/config,readonly"], script,
                            user="1000:1000")
            start.assert_not_called()

    def test_initialize_helper_has_only_chown_and_later_helpers_have_no_added_caps(self):
        plan = {"run_id": "nas-sim-0001", "helper_image_id": "sha256:" + "d" * 64}
        identity = "f" * 64
        script = "chown -R 1000:1000 /config"
        created = {
            "Id": identity, "Name": "/holyarrs-device-helper-initialize-nas-sim-0001",
            "RestartCount": 0,
            "State": {"Status": "created", "Running": False, "Pid": 0,
                      "StartedAt": "0001-01-01T00:00:00Z", "FinishedAt": "0001-01-01T00:00:00Z"},
            "Image": plan["helper_image_id"],
            "Config": {"User": "0:0", "Cmd": ["-eu", "-c", script], "Entrypoint": ["sh"],
                       "Labels": {"io.holyarrs.device-simulation": plan["run_id"]}},
            "HostConfig": {"NetworkMode": "none", "Memory": 268435456, "MemorySwap": 268435456,
                           "NanoCpus": 1_000_000_000, "PidsLimit": 128, "CapDrop": ["ALL"],
                           "CapAdd": ["CHOWN"], "Privileged": False, "AutoRemove": False,
                           "RestartPolicy": {"Name": "no"}, "SecurityOpt": ["no-new-privileges"]},
            "Mounts": [{"Destination": "/config", "RW": True, "Type": "volume", "Name": "config"}],
        }
        stopped = {"State": {"ExitCode": 0}}
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(sim, "_create_owned_container", return_value=identity) as create, \
                patch.object(sim, "_one_inspect", side_effect=[created, stopped]), \
                patch.object(sim, "run_limited", return_value=b""), \
                patch.object(sim, "_remove_owned_container"), \
                patch.object(sim, "_unregister_container"):
            sim._helper(plan, {"active_containers": [identity]}, Path(directory) / "state.json",
                        "initialize", ["type=volume,src=config,dst=/config"], script,
                        user="0:0", cap_add=("CHOWN",))
        command = create.call_args.args[-1]
        self.assertEqual(command.count("--cap-add"), 1)
        self.assertEqual(command[command.index("--cap-add") + 1], "CHOWN")

        for inspected_caps in (None, [], ["CHOWN", "DAC_OVERRIDE"], ["CHOWN", "CHOWN"]):
            created["HostConfig"]["CapAdd"] = inspected_caps
            with tempfile.TemporaryDirectory() as directory, \
                    patch.object(sim, "_create_owned_container", return_value=identity), \
                    patch.object(sim, "_one_inspect", return_value=created), \
                    patch.object(sim, "run_limited") as start, \
                    patch.object(sim, "_remove_owned_container"), \
                    patch.object(sim, "_unregister_container"):
                with self.assertRaisesRegex(sim.ContractError, "differs from its plan"):
                    sim._helper(plan, {}, Path(directory) / "state.json", "initialize",
                                ["type=volume,src=config,dst=/config"], script,
                                user="0:0", cap_add=("CHOWN",))
                start.assert_not_called()
        with self.assertRaisesRegex(sim.ContractError, "capability plan"):
            sim._helper(plan, {}, Path("unused"), "initialize", [], script,
                        user="0:0", cap_add=("FOWNER",))

    def test_nas_runtime_source_keeps_privilege_to_initialization_only(self):
        source = inspect.getsource(sim.run_nas)
        self.assertIn('user="0:0", cap_add=("CHOWN",)', source)
        self.assertEqual(source.count('cap_add=("CHOWN",)'), 1)
        self.assertNotIn("DAC_OVERRIDE", source)
        self.assertNotIn("FOWNER", source)
        self.assertLess(source.index("hardlink_source=%s"), source.index("chown -R 1000:1000"))
        for purpose in ("uid-write", "readonly-denial", "read-setup-code", "backup",
                        "interrupted-copy", "restore-copy"):
            start = source.index(f'_helper(plan, state, args.state, "{purpose}"')
            self.assertIn('user="1000:1000"', source[start:start + 1800])
        for destination in ("/config", "/data", "/backup", "/restore", "/empty-data"):
            self.assertIn(f"dst={destination}", source)

    def test_volume_manifests_are_read_as_uid_1000_without_caps(self):
        entries = b"directory|755|1000|1000|0|.\n"
        with patch.object(sim, "_helper", side_effect=[entries, b""]) as helper:
            result = sim._volume_manifests({"run_id": "nas-sim-0001"}, {}, Path("state.json"),
                                           "config-before-backup", "config")
        self.assertEqual(result["entries"]["records"], ["directory|755|1000|1000|0|."])
        self.assertEqual(len(helper.call_args_list), 2)
        for call in helper.call_args_list:
            self.assertEqual(call.kwargs, {"user": "1000:1000"})

    def test_app_container_is_inspected_before_start(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_nas_plan(root)
            contract = plan["app_contracts"][plan["candidate_image_id"]]
            identity = "f" * 64
            created = {
                "Id": identity, "Name": "/app", "RestartCount": 0,
                "State": {"Status": "created", "Running": False, "Pid": 0,
                          "StartedAt": "0001-01-01T00:00:00Z",
                          "FinishedAt": "0001-01-01T00:00:00Z"},
                "Image": plan["candidate_image_id"],
                "Config": {"User": "1000:1000", "Env": contract["env"],
                           "Cmd": contract["cmd"],
                           "Entrypoint": contract["entrypoint"],
                           "Labels": {"io.holyarrs.device-simulation": plan["run_id"]}},
                "HostConfig": {"NetworkMode": plan["names"]["network"], "Memory": 2147483648,
                               "MemorySwap": 2147483648, "NanoCpus": 1_000_000_000, "PidsLimit": 256,
                               "CapDrop": ["ALL"], "CapAdd": None, "Privileged": False,
                               "AutoRemove": False, "RestartPolicy": {"Name": "no"},
                               "SecurityOpt": ["no-new-privileges"],
                               "PortBindings": {"18730/tcp": [{"HostIp": "127.0.0.1", "HostPort": "18746"}]}},
                "Mounts": [{"Destination": "/config", "RW": True, "Type": "volume", "Name": "config"},
                           {"Destination": "/data", "RW": False, "Type": "volume", "Name": "data"}]}
            with patch.object(sim, "_create_owned_container", return_value=identity), \
                    patch.object(sim, "_one_inspect", return_value=created):
                self.assertEqual(sim._app_create(plan, {}, root / "state.json", plan["candidate_image_id"],
                                                 "app", "config", "data"), identity)
                create_command = sim._create_owned_container.call_args.args[-1]
                self.assertEqual(create_command[create_command.index("--entrypoint") + 1], "/holyarrs")
                self.assertEqual(create_command[-5:], [plan["candidate_image_id"],
                                                       "-config", "/config", "-addr", ":18730"])
                created["Mounts"][1]["RW"] = True
                with self.assertRaisesRegex(sim.ContractError, "exact plan"):
                    sim._app_create(plan, {}, root / "state.json", plan["candidate_image_id"],
                                    "app", "config", "data")
                created["Mounts"][1]["RW"] = False
                created["Config"].pop("Cmd")
                with self.assertRaisesRegex(sim.ContractError, "exact plan"):
                    sim._app_create(plan, {}, root / "state.json", plan["candidate_image_id"],
                                    "app", "config", "data")
                created["Config"]["Cmd"] = contract["cmd"]
                created["State"].update(Status="exited", FinishedAt="2026-10-16T00:00:00Z")
                with self.assertRaisesRegex(sim.ContractError, "newly-created"):
                    sim._app_create(plan, {}, root / "state.json", plan["candidate_image_id"],
                                    "app", "config", "data")

    def test_nas_verifier_compares_sanitized_evidence_and_inspected_facts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = make_nas_plan(root)
            receipt = nas_receipt(plan)
            ph = hashlib.sha256((root / "nas-plan.json").read_bytes()).hexdigest()
            rp = root / "receipt.json"
            rh = write(rp, receipt)
            args = argparse.Namespace(plan=root / "nas-plan.json", plan_sha256=ph,
                                      receipt=rp, receipt_sha256=rh)
            self.assertFalse(sim.verify_nas(args)["nas_certification"])
            receipt["filesystem_evidence"]["data_after"]["entries"]["records"][0] += "-changed"
            rh = write(root / "bad.json", receipt)
            args.receipt, args.receipt_sha256 = root / "bad.json", rh
            with self.assertRaisesRegex(sim.ContractError, "manifest evidence"):
                sim.verify_nas(args)

            receipt = nas_receipt(plan)
            receipt["container_facts"][0]["contract"].pop("cmd")
            rh = write(root / "missing-command.json", receipt)
            args.receipt, args.receipt_sha256 = root / "missing-command.json", rh
            with self.assertRaisesRegex(sim.ContractError, "container evidence"):
                sim.verify_nas(args)

            receipt = nas_receipt(plan)
            receipt["filesystem_evidence"]["backup"] = manifest(
                "directory|755|1000|1000|0|.\nregular file|600|1000|1000|5|./db\n",
                "d" * 64 + "  ./db\n")
            rh = write(root / "source-backup-drift.json", receipt)
            args.receipt, args.receipt_sha256 = root / "source-backup-drift.json", rh
            with self.assertRaisesRegex(sim.ContractError, "comparison evidence"):
                sim.verify_nas(args)

    def test_protocol_gaps_and_interrupted_copy_are_explicit(self):
        self.assertEqual(sim.NAS_EXPECTED["local-config-network-data"], "not-covered")
        self.assertEqual(sim.NAS_EXPECTED["network-config-refused"], "source-contract")
        self.assertEqual(sim.NAS_EXPECTED["interrupted-copy-recovery"], "passed")
        contract = json.loads((HERE / "scenarios.json").read_text(encoding="utf-8"))
        self.assertEqual(contract["nas_like"]["scenarios"], sim.NAS_SCENARIOS)
        self.assertEqual(contract["nas_like"]["expected"], sim.NAS_EXPECTED)


if __name__ == "__main__":
    unittest.main()
