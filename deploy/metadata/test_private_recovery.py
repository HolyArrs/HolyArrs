import contextlib
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import sqlite3
import stat
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import tracemalloc
import types
import unittest
from unittest import mock


HERE = pathlib.Path(__file__).parent


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, HERE / filename)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


recovery = load("private_recovery", "private-recovery.py")
offhost = load("offhost_backup", "offhost-backup.py")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def make_databases(directory, *, version=1, extra=False):
    directory.mkdir(parents=True)
    with contextlib.closing(sqlite3.connect(directory / "cache.sqlite")) as db, db:
        db.executescript(
            "CREATE TABLE entries (key TEXT PRIMARY KEY, data BLOB NOT NULL, "
            "cursor TEXT NOT NULL, fetched_at TEXT NOT NULL, expires_at TEXT NOT NULL, "
            "age INTEGER NOT NULL, size INTEGER NOT NULL CHECK(size>0)) WITHOUT ROWID; "
            "CREATE INDEX entries_age ON entries(age,key);"
        )
        db.execute(f"PRAGMA user_version={version}")
        db.execute("INSERT INTO entries VALUES ('one',x'01','', 'a','b',1,1)")
        if extra:
            db.execute("CREATE TABLE surprise(value TEXT)")
    with contextlib.closing(sqlite3.connect(directory / "access.sqlite")) as db, db:
        db.executescript(
            "CREATE TABLE tokens (hash BLOB PRIMARY KEY CHECK(length(hash)=32), "
            "revoked INTEGER NOT NULL CHECK(revoked IN (0,1))) WITHOUT ROWID;"
        )
        db.execute(f"PRAGMA user_version={version}")
        db.execute("INSERT INTO tokens VALUES (?,0)", (hashlib.sha256(b"synthetic").digest(),))
        db.execute("INSERT INTO tokens VALUES (?,1)", (hashlib.sha256(b"revoked").digest(),))


def tar_bytes(entries):
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as archive:
        for name, data, kind in entries:
            info = tarfile.TarInfo(name)
            if kind == "file":
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            elif kind == "link":
                info.type = tarfile.SYMTYPE
                info.linkname = data.decode()
                archive.addfile(info)
    return buffer.getvalue()


def make_verified_point(root, name, ciphertext):
    point = root / name
    point.mkdir()
    payload = point / (name + ".cms")
    payload.write_bytes(ciphertext)
    receipt = {"schema_version": 1, "status": "verified", "verified_at": "2026-10-15T12:34:56Z",
               "run_name": name, "ciphertext_bytes": len(ciphertext),
               "ciphertext_sha256": digest(ciphertext), "identities": recovery.expected_identity_manifest(),
               "all_tokens_revoked_on_restore": True}
    (point / "verified-receipt.json").write_text(json.dumps(receipt))
    return point


def synthetic_config():
    return {"listen": ":8080", "data_dir": "/data", "private_fixtures": True,
            "providers": {"tmdb": {"base_url": "http://fixture:8081/tmdb/3", "interval_ms": 200},
                          "tvmaze": {"base_url": "http://fixture:8081/tvmaze", "interval_ms": 1000},
                          "musicbrainz": {"base_url": "http://fixture:8081/musicbrainz/ws/2", "interval_ms": 1100}},
            "cache_bytes": 2147483648, "cache_entries": 100000, "min_free_bytes": 21474836480,
            "ttl_seconds": 86400, "stale_seconds": 0, "per_minute": 60000, "burst": 1000,
            "per_token_concurrent": 100, "max_concurrent": 100, "max_pending": 200}


class ArchiveSafetyTests(unittest.TestCase):
    def test_tar_preflight_rejects_negative_member_size_without_readall(self):
        with tempfile.TemporaryDirectory() as tmp:
            package = pathlib.Path(tmp) / "negative.tar.gz"
            header = bytearray(512)
            header[124:136] = b"-0000000001\0"
            package.write_bytes(recovery.gzip.compress(bytes(header) + b"\0" * 1024))
            with self.assertRaisesRegex(recovery.RecoveryError, "header size is invalid"):
                recovery.bounded_gzip_tar_size(package, 4096)

    def test_tar_preflight_discards_large_regular_payload_in_bounded_chunks(self):
        with tempfile.TemporaryDirectory() as tmp:
            package = pathlib.Path(tmp) / "large.tar.gz"
            payload = pathlib.Path(tmp) / "payload"
            with payload.open("wb") as target:
                target.truncate(16 * 1024 * 1024)
            with tarfile.open(package, "w:gz") as archive:
                archive.add(payload, arcname="payload", recursive=False)
            tracemalloc.start()
            try:
                recovery.bounded_gzip_tar_size(package, 20 * 1024 * 1024)
                _, peak = tracemalloc.get_traced_memory()
            finally:
                tracemalloc.stop()
            self.assertLess(peak, 6 * 1024 * 1024)

    def test_gzip_expansion_is_bounded_before_late_manifest_scan(self):
        with tempfile.TemporaryDirectory() as tmp:
            package = pathlib.Path(tmp) / "package.tar.gz"
            manifest = json.dumps({"schema_version": 1}).encode()
            package.write_bytes(tar_bytes([("data/cache.sqlite", b"x" * 4096, "file"),
                                           ("manifest.json", manifest, "file")]))
            with mock.patch.object(recovery, "EXPANSION_CAP", 1024):
                with self.assertRaisesRegex(recovery.RecoveryError, "decompression limit"):
                    offhost._read_package_manifest(package)

    def test_pax_extension_growth_is_included_in_decompression_bound(self):
        with tempfile.TemporaryDirectory() as tmp:
            package = pathlib.Path(tmp) / "pax.tar.gz"
            with tarfile.open(package, "w:gz", format=tarfile.PAX_FORMAT) as archive:
                member = tarfile.TarInfo("manifest.json")
                member.pax_headers = {"comment": "x" * 4096}
                member.size = 2
                archive.addfile(member, io.BytesIO(b"{}"))
            with self.assertRaisesRegex(recovery.RecoveryError, "decompression limit"):
                recovery.bounded_gzip_tar_size(package, 1024)

    def test_rejects_traversal_duplicate_link_and_unexpected_members(self):
        cases = {
            "traversal": [("../escape", b"x", "file")],
            "backslash": [("data\\escape", b"x", "file")],
            "duplicate": [("data/cache.sqlite", b"x", "file"), ("data/cache.sqlite", b"y", "file")],
            "link": [("data/cache.sqlite", b"target", "link")],
            "unexpected": [("data/other.sqlite", b"x", "file")],
        }
        for label, entries in cases.items():
            with self.subTest(label=label), tempfile.TemporaryDirectory() as tmp:
                package = pathlib.Path(tmp) / "package.tar.gz"
                package.write_bytes(tar_bytes(entries))
                with self.assertRaises(recovery.RecoveryError):
                    recovery.inspect_archive(package, {"data/cache.sqlite": {"bytes": 1, "sha256": digest(b"x")}})

    def test_rejects_member_and_total_expansion_bounds(self):
        with tempfile.TemporaryDirectory() as tmp:
            package = pathlib.Path(tmp) / "package.tar.gz"
            package.write_bytes(tar_bytes([("data/cache.sqlite", b"12345", "file")]))
            expected = {"data/cache.sqlite": {"bytes": 5, "sha256": digest(b"12345")}}
            with self.assertRaises(recovery.RecoveryError):
                recovery.inspect_archive(package, expected, member_cap=4)
            with self.assertRaises(recovery.RecoveryError):
                recovery.inspect_archive(package, expected, expansion_cap=4)

    def test_streaming_extract_refuses_tampered_member_and_keeps_destination_unpublished(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            package = root / "package.tar.gz"
            package.write_bytes(tar_bytes([("data/cache.sqlite", b"tampered", "file")]))
            destination = root / "verified"
            expected = {"data/cache.sqlite": {"bytes": 8, "sha256": digest(b"expected")}}
            with self.assertRaises(recovery.RecoveryError):
                recovery.verify_and_publish_archive(package, destination, expected)
            self.assertFalse(destination.exists())


class DatabaseRecoveryTests(unittest.TestCase):
    def test_hidden_columns_index_semantics_and_constraints_are_exact(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            hidden = root / "hidden"
            make_databases(hidden)
            with contextlib.closing(sqlite3.connect(hidden / "access.sqlite")) as db, db:
                db.execute("ALTER TABLE tokens ADD COLUMN concealed TEXT GENERATED ALWAYS AS ('x') VIRTUAL")
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_database_pair(hidden)

            index_drift = root / "index"
            make_databases(index_drift)
            with contextlib.closing(sqlite3.connect(index_drift / "cache.sqlite")) as db, db:
                db.execute("DROP INDEX entries_age")
                db.execute("CREATE UNIQUE INDEX entries_age ON entries(age DESC,key DESC) WHERE age<0")
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_database_pair(index_drift)

            constraints = root / "constraints"
            make_databases(constraints)
            access = constraints / "access.sqlite"
            access.unlink()
            with contextlib.closing(sqlite3.connect(access)) as db, db:
                db.executescript("CREATE TABLE tokens (hash BLOB PRIMARY KEY, revoked INTEGER NOT NULL) WITHOUT ROWID; PRAGMA user_version=1;")
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_database_pair(constraints)

    def test_exact_schema_and_integrity_are_required(self):
        with tempfile.TemporaryDirectory() as tmp:
            pair = pathlib.Path(tmp) / "pair"
            make_databases(pair, extra=True)
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_database_pair(pair)

    def test_prepare_restore_revokes_all_tokens_and_preserves_cache(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source"
            make_databases(source)
            before = (source / "cache.sqlite").read_bytes()
            destination = root / "restore"
            recovery.prepare_restore(source, destination)
            self.assertEqual((source / "cache.sqlite").read_bytes(), before)
            with contextlib.closing(sqlite3.connect(destination / "access.sqlite")) as db:
                self.assertEqual(db.execute("SELECT revoked FROM tokens").fetchall(), [(1,), (1,)])
            recovery.validate_database_pair(destination)

    def test_prepare_restore_refuses_future_schema_and_existing_destination(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source"
            make_databases(source, version=2)
            with self.assertRaises(recovery.RecoveryError):
                recovery.prepare_restore(source, root / "restore")
            good = root / "good"
            make_databases(good)
            existing = root / "existing"
            existing.mkdir()
            with self.assertRaises(recovery.RecoveryError):
                recovery.prepare_restore(good, existing)


class OracleGateTests(unittest.TestCase):
    def test_manifest_path_requires_root_owned_nonwritable_real_ancestors(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "root"
            settings = root / "settings"
            settings.mkdir(parents=True, mode=0o700)
            manifest = settings / "recovery-manifest.json"
            manifest.write_text("{}", encoding="utf-8")
            os.chmod(manifest, 0o600)
            mode_bits = lambda mode: 0o700 if stat.S_ISDIR(mode) else 0o600
            with mock.patch.object(recovery.stat, "S_IMODE", side_effect=mode_bits):
                recovery._validate_root_owned_manifest_path(root, manifest)
            target = pathlib.Path(tmp) / "redirect"
            target.mkdir()
            linked = root / "linked"
            try:
                linked.symlink_to(target, target_is_directory=True)
            except OSError:
                self.skipTest("directory symlinks unavailable")
            linked_manifest = linked / "recovery-manifest.json"
            linked_manifest.write_text("{}", encoding="utf-8")
            with mock.patch.object(recovery.stat, "S_IMODE", side_effect=mode_bits), \
                    self.assertRaises(recovery.RecoveryError):
                recovery._validate_root_owned_manifest_path(root, linked_manifest)

    def test_root_output_symlink_parent_is_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "root"
            (root / "backups").mkdir(parents=True)
            (root / "results/locks").mkdir(parents=True, mode=0o700)
            (root / "results/latest").mkdir(mode=0o700)
            target = pathlib.Path(tmp) / "redirect"
            target.mkdir()
            try:
                (root / "backups/staging").symlink_to(target, target_is_directory=True)
            except OSError:
                self.skipTest("directory symlinks unavailable")
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_private_output_paths(root, (root / "results/locks").stat().st_uid)

    def test_stopped_private_image_or_port_drift_is_refused_on_recheck(self):
        identities = recovery.expected_identity_manifest()
        original = []
        for name, image in (("api", recovery.API_IMAGE), ("fixture", recovery.FIXTURE_IMAGE)):
            original.append({"Id": name + "-id", "Image": image, "State": {"Running": False},
                             "Config": {"Labels": {"holyforge.owner": recovery.OWNER}},
                             "NetworkSettings": {"Ports": {}}})
        before = recovery._private_state(pathlib.Path("unused"), original, False)
        for mutation in ("image", "ports"):
            changed = json.loads(json.dumps(original))
            if mutation == "image":
                changed[0]["Image"] = "sha256:" + "0" * 64
            else:
                changed[0]["NetworkSettings"]["Ports"] = {"8080/tcp": [{"HostPort": "1"}]}
            calls = iter((types.SimpleNamespace(returncode=0, stdout=json.dumps(changed), stderr=""),
                          types.SimpleNamespace(returncode=1, stdout="", stderr="")))
            with self.subTest(mutation=mutation), self.assertRaises(recovery.RecoveryError):
                recovery.recheck_private_state(pathlib.Path("unused"), before, identities,
                                               runner=lambda *_args, **_kwargs: next(calls))

    def test_safe_run_name_and_remote_path_are_exact(self):
        name = "run-20261015T123456Z-a1b2c3d4"
        self.assertEqual(recovery.validate_run_name(name), name)
        expected = f"/home/home/holyarrs-metadata-server/backups/scheduled/{name}/{name}.cms"
        self.assertEqual(offhost.validate_remote_ciphertext_path(name, expected), expected)
        for bad in ("run-bad", "../" + name, name + "/x"):
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_run_name(bad)

    def test_identity_drift_and_active_or_unowned_services_refuse(self):
        manifest = recovery.expected_identity_manifest()
        recovery.validate_identities(manifest, dict(manifest))
        changed = dict(manifest, api_image="sha256:" + "0" * 64)
        with self.assertRaises(recovery.RecoveryError):
            recovery.validate_identities(manifest, changed)
        stopped = {"api": {"exists": True, "owner": recovery.OWNER, "image": manifest["api_image"], "running": False, "ports": []},
                   "fixture": {"exists": True, "owner": recovery.OWNER, "image": manifest["fixture_image"], "running": False, "ports": []},
                   "private_test_active": False, "maintenance_exists": False}
        recovery.validate_private_state(stopped, manifest)
        for key, value in (("private_test_active", True), ("maintenance_exists", True)):
            bad = dict(stopped, **{key: value})
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_private_state(bad, manifest)
        bad = json.loads(json.dumps(stopped))
        bad["api"]["owner"] = "other"
        with self.assertRaises(recovery.RecoveryError):
            recovery.validate_private_state(bad, manifest)

    def test_peak_disk_estimate_and_reserve_refusal(self):
        peak = recovery.estimate_staging_peak(100, 50, 200)
        self.assertGreaterEqual(peak, 100 * 3 + 50 + 200)
        with self.assertRaises(recovery.RecoveryError):
            recovery.require_disk_space(peak + recovery.MIN_FREE_BYTES - 1, peak)

    def test_existing_service_baseline_drift_refuses(self):
        expected = [{"name": name, "id": name + "-id", "started": "time", "restarts": 0,
                     "running": True, "health": "healthy"} for name in recovery.EXISTING]
        recovery.validate_existing_services(expected, json.loads(json.dumps(expected)))
        changed = json.loads(json.dumps(expected))
        changed[0]["restarts"] = 1
        with self.assertRaises(recovery.RecoveryError):
            recovery.validate_existing_services(expected, changed)

    def test_current_descriptor_must_keep_runtime_and_operator_selection(self):
        expected = {"runtime_folder": "2026-10-15-build-7", "operator_folder": "2026-10-15-build-8",
                    "source_commit": recovery.OPERATOR_REVISION, "public_deployment": False}
        recovery.validate_current_descriptor(expected)
        for key, value in (("runtime_folder", "2026-10-15-build-8"), ("public_deployment", True)):
            changed = dict(expected, **{key: value})
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_current_descriptor(changed)

    def test_image_export_config_hash_and_architecture_are_bound(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            raw = json.dumps({"architecture": "arm64", "os": "linux"}, separators=(",", ":")).encode()
            image_id = "sha256:" + digest(raw)
            archive_path = root / "image.tar"
            manifest = json.dumps([{"Config": digest(raw) + ".json", "RepoTags": None, "Layers": []}]).encode()
            archive_path.write_bytes(tar_bytes([]))
            with tarfile.open(archive_path, "w:") as archive:
                for name, data in (("manifest.json", manifest), (digest(raw) + ".json", raw)):
                    info = tarfile.TarInfo(name); info.size = len(data); archive.addfile(info, io.BytesIO(data))
            recovery.image_export_identity(archive_path, image_id)
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, image_id, "amd64")

    def test_image_export_oci_index_binds_manifest_config_and_layers(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            config = json.dumps({"architecture": "arm64", "os": "linux"},
                                separators=(",", ":")).encode()
            layer = b"synthetic-layer"
            config_digest = digest(config)
            layer_digest = digest(layer)
            image_manifest = json.dumps({
                "schemaVersion": 2,
                "mediaType": "application/vnd.oci.image.manifest.v1+json",
                "config": {
                    "mediaType": "application/vnd.oci.image.config.v1+json",
                    "digest": "sha256:" + config_digest,
                    "size": len(config),
                },
                "layers": [{
                    "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
                    "digest": "sha256:" + layer_digest,
                    "size": len(layer),
                }],
            }, separators=(",", ":")).encode()
            image_digest = digest(image_manifest)
            index = json.dumps({
                "schemaVersion": 2,
                "mediaType": "application/vnd.oci.image.index.v1+json",
                "manifests": [{
                    "mediaType": "application/vnd.oci.image.manifest.v1+json",
                    "digest": "sha256:" + image_digest,
                    "size": len(image_manifest),
                }],
            }, separators=(",", ":")).encode()
            compatibility = json.dumps([{
                "Config": "blobs/sha256/" + config_digest,
                "RepoTags": None,
                "Layers": ["blobs/sha256/" + layer_digest],
            }], separators=(",", ":")).encode()

            def write_archive(path, *, compatibility_raw=compatibility, index_raw=index,
                              manifest_raw=image_manifest, config_raw=config, layer_raw=layer):
                with tarfile.open(path, "w:") as archive:
                    entries = (
                        ("manifest.json", compatibility_raw),
                        ("index.json", index_raw),
                        ("oci-layout", b'{"imageLayoutVersion":"1.0.0"}'),
                        ("blobs/sha256/" + image_digest, manifest_raw),
                        ("blobs/sha256/" + config_digest, config_raw),
                        ("blobs/sha256/" + layer_digest, layer_raw),
                    )
                    for name, data in entries:
                        info = tarfile.TarInfo(name)
                        info.size = len(data)
                        archive.addfile(info, io.BytesIO(data))

            archive_path = root / "oci-image.tar"
            write_archive(archive_path)
            recovery.image_export_identity(archive_path, "sha256:" + image_digest)

            write_archive(archive_path, layer_raw=b"x" * len(layer))
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, "sha256:" + image_digest)

            write_archive(archive_path, config_raw=b"x" * len(config))
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, "sha256:" + image_digest)

            write_archive(archive_path, manifest_raw=b"{}")
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, "sha256:" + image_digest)

            wrong_compatibility = json.loads(compatibility)
            wrong_compatibility[0]["Layers"] = []
            write_archive(archive_path, compatibility_raw=json.dumps(wrong_compatibility).encode())
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, "sha256:" + image_digest)

            ambiguous = json.loads(index)
            ambiguous["manifests"].append(dict(ambiguous["manifests"][0]))
            write_archive(archive_path, index_raw=json.dumps(ambiguous).encode())
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, "sha256:" + image_digest)

            wrong_platform = json.loads(index)
            wrong_platform["manifests"][0]["platform"] = {"architecture": "amd64", "os": "linux"}
            write_archive(archive_path, index_raw=json.dumps(wrong_platform).encode())
            with self.assertRaises(recovery.RecoveryError):
                recovery.image_export_identity(archive_path, "sha256:" + image_digest)

    def test_lock_refuses_overlap_without_deleting_existing_lock(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "backup.lock"
            first = recovery.ServiceLock(path)
            first.acquire()
            try:
                with self.assertRaises(recovery.RecoveryError):
                    recovery.ServiceLock(path).acquire()
                self.assertTrue(path.exists())
            finally:
                first.release()

    def test_export_directory_requires_exact_owner_mode_and_real_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            safe = root / "scheduled"
            safe.mkdir(mode=0o700)
            info = safe.lstat()
            safe_info = types.SimpleNamespace(st_mode=stat.S_IFDIR | 0o700,
                                              st_uid=info.st_uid, st_gid=info.st_gid)
            recovery.validate_export_directory(safe, info.st_uid, info.st_gid, info=safe_info)
            loose_info = types.SimpleNamespace(st_mode=stat.S_IFDIR | 0o750,
                                               st_uid=info.st_uid, st_gid=info.st_gid)
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_export_directory(safe, info.st_uid, info.st_gid, info=loose_info)
            link = root / "scheduled-link"
            try:
                link.symlink_to(safe, target_is_directory=True)
            except OSError:
                return
            with self.assertRaises(recovery.RecoveryError):
                recovery.validate_export_directory(link, info.st_uid, info.st_gid, info=safe_info)


class ReceiptAndStatusTests(unittest.TestCase):
    def test_receipt_tamper_cannot_replace_previous_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            latest = root / "latest.json"
            old = {"schema_version": 1, "status": "verified", "run_name": "run-20261014T010203Z-deadbeef"}
            recovery.atomic_json(latest, old)
            bad = offhost.sample_receipt()
            bad["ciphertext_sha256"] = "0" * 64
            ciphertext = root / "incoming.cms"
            ciphertext.write_bytes(b"ciphertext")
            with self.assertRaises(recovery.RecoveryError):
                offhost.validate_downloaded_ciphertext(bad, ciphertext)
            self.assertEqual(json.loads(latest.read_text()), old)

    def test_status_is_idempotent_and_preserves_previous_success_on_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            latest = root / "latest.json"
            failure = root / "failure.json"
            success = {"schema_version": 1, "status": "verified", "verified_at": "2026-10-15T00:00:00Z"}
            recovery.atomic_json(latest, success)
            recovery.write_failure(failure, "download", RuntimeError("secret token=abc stderr"))
            first = recovery.read_status(latest, failure, now="2026-10-15T01:00:00Z")
            second = recovery.read_status(latest, failure, now="2026-10-15T01:00:00Z")
            self.assertEqual(first, second)
            self.assertEqual(json.loads(latest.read_text()), success)
            self.assertNotIn("secret", failure.read_text().lower())
            self.assertNotIn("abc", failure.read_text())


class DestinationAndRetentionTests(unittest.TestCase):
    def test_windows_defaults_use_the_fixed_profile_project(self):
        root = pathlib.Path("C:/Users/Luis/holyarrs-metadata-backup")
        self.assertEqual(offhost.DEFAULT_SECRETS_ROOT, root)
        self.assertEqual(offhost.DEFAULT_DESTINATION, root / "backups")
        self.assertEqual(offhost.PRIVATE_KEY, root / "keys/backup-key.pem")
        self.assertEqual(offhost.RECIPIENT, root / "keys/recipient.pem")

    def test_destination_must_be_under_restricted_secrets_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            secrets = pathlib.Path(tmp) / "secrets"
            secrets.mkdir()
            destination = secrets / "backups"
            offhost.validate_destination(destination, secrets)
            with self.assertRaises(recovery.RecoveryError):
                offhost.validate_destination(secrets / "child" / ".." / ".." / "outside", secrets)
            with self.assertRaises(recovery.RecoveryError):
                offhost.validate_destination(pathlib.Path(tmp) / "outside", secrets)
            if hasattr(os, "symlink"):
                target = pathlib.Path(tmp) / "target"
                target.mkdir()
                link = secrets / "link"
                try:
                    link.symlink_to(target, target_is_directory=True)
                except OSError:
                    return
                with self.assertRaises(recovery.RecoveryError):
                    offhost.validate_destination(link / "backups", secrets)

    def test_retention_refuses_unknown_symlink_and_unverified_content_without_deleting(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            old = make_verified_point(root, "run-20261013T010203Z-aaaaaaaa", b"old")
            prior = make_verified_point(root, "run-20261014T010203Z-bbbbbbbb", b"prior")
            latest = make_verified_point(root, "run-20261015T010203Z-cccccccc", b"latest")
            unknown = root / "notes.txt"
            unknown.write_text("keep")
            with self.assertRaises(recovery.RecoveryError):
                offhost.apply_retention(root, latest.name, max_points=2, budget=1024)
            self.assertTrue(old.exists())
            self.assertTrue(prior.exists())
            self.assertTrue(latest.exists())
            self.assertTrue(unknown.exists())

    def test_retention_keeps_latest_and_prior_when_budget_cannot_fit_them(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            prior = make_verified_point(root, "run-20261014T010203Z-bbbbbbbb", b"12345")
            latest = make_verified_point(root, "run-20261015T010203Z-cccccccc", b"67890")
            with self.assertRaises(recovery.RecoveryError):
                offhost.apply_retention(root, latest.name, max_points=7, budget=1)
            self.assertTrue(prior.exists())
            self.assertTrue(latest.exists())

    def test_local_reserve_counts_compressed_and_extracted_plaintext(self):
        ciphertext = 100
        needed = offhost.required_local_free(ciphertext)
        one_plaintext_only = offhost.LOCAL_RESERVE + ciphertext + recovery.EXPANSION_CAP + 1
        self.assertGreater(needed, one_plaintext_only)

    def test_synthetic_configuration_is_exact_and_fixture_only(self):
        offhost.validate_synthetic_config(synthetic_config())
        cases = []
        changed_url = synthetic_config(); changed_url["providers"]["tmdb"]["base_url"] = "https://example.invalid"
        cases.append(changed_url)
        extra = synthetic_config(); extra["unexpected"] = True; cases.append(extra)
        credential = synthetic_config(); credential["api_key"] = "value"; cases.append(credential)
        for value in cases:
            with self.assertRaises(recovery.RecoveryError):
                offhost.validate_synthetic_config(value)

    def test_ssh_and_disk_failures_preserve_success_and_write_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            secrets = root / "secrets"; destination = secrets / "backups"; destination.mkdir(parents=True)
            previous = {"schema_version": 1, "status": "verified", "verified_at": "2026-10-14T00:00:00Z"}
            (destination / "latest.json").write_text(json.dumps(previous))

            def ssh_failed(args, **kwargs):
                return types.SimpleNamespace(returncode=1, stdout="sensitive", stderr="private")

            with self.assertRaises(recovery.RecoveryError):
                offhost.run_backup(destination, runner=ssh_failed, secrets_root=secrets)
            self.assertEqual(json.loads((destination / "latest.json").read_text()), previous)
            self.assertEqual(json.loads((destination / "failure.json").read_text())["status"], "failed")

            receipt = offhost.sample_receipt()
            def ssh_ok(args, **kwargs):
                return types.SimpleNamespace(returncode=0, stdout=json.dumps(receipt), stderr="")
            with mock.patch.object(offhost.shutil, "disk_usage", return_value=types.SimpleNamespace(free=1)):
                with self.assertRaisesRegex(recovery.RecoveryError, "disk reserve"):
                    offhost.run_backup(destination, runner=ssh_ok, secrets_root=secrets)
            self.assertEqual(json.loads((destination / "latest.json").read_text()), previous)
            self.assertEqual(json.loads((destination / "failure.json").read_text())["status"], "failed")


class CommandSafetyTests(unittest.TestCase):
    def test_binary_deadline_stops_stalls_and_cleans_only_owned_docker(self):
        class BlockingOutput:
            def __init__(self, process, first):
                self.process, self.first = process, first
            def read(self, _size):
                if self.first is not None:
                    value, self.first = self.first, None
                    return value
                self.process.killed.wait(2)
                return b""

        class Process:
            def __init__(self, first):
                self.killed = threading.Event()
                self.stdout = BlockingOutput(self, first)
                self.returncode = None
            def kill(self):
                self.returncode = -1; self.killed.set()
            def wait(self, timeout=None):
                if not self.killed.wait(timeout):
                    raise subprocess.TimeoutExpired("docker", timeout)
                return self.returncode

        args = ["docker", "run", "--name", "holyarrs-metadata-offhost-deadbeef",
                "--label", "holyforge.owner=" + recovery.OWNER, recovery.CRYPTO_IMAGE]
        for first in (None, b"partial"):
            with self.subTest(first=first), tempfile.TemporaryDirectory() as tmp:
                process = Process(first)
                cleanup = []
                container_id = "a" * 64
                def cleanup_runner(command, **_kwargs):
                    cleanup.append(command)
                    if command[1:3] == ["container", "inspect"]:
                        value = [{"Id": container_id, "Name": "/holyarrs-metadata-offhost-deadbeef",
                                  "Image": recovery.CRYPTO_IMAGE,
                                  "Config": {"Labels": {"holyforge.owner": recovery.OWNER}}}]
                        return types.SimpleNamespace(returncode=0, stdout=json.dumps(value))
                    return types.SimpleNamespace(returncode=0, stdout="")
                started = time.monotonic()
                with self.assertRaises(recovery.RecoveryError):
                    recovery.run_binary_to_capped_file(
                        args, pathlib.Path(tmp) / "plain", 1024,
                        popen=lambda *_args, **_kwargs: process,
                        cleanup_runner=cleanup_runner, timeout=.05)
                self.assertLess(time.monotonic() - started, 1)
                self.assertTrue(process.killed.is_set())
                self.assertEqual(cleanup[-1], ["docker", "rm", "-f", container_id])
                self.assertFalse((pathlib.Path(tmp) / "plain").exists())

    def test_binary_abort_leaves_name_collision_with_mismatched_identity_untouched(self):
        args = ["docker", "run", "--name", "holyarrs-metadata-offhost-deadbeef",
                "--label", "holyforge.owner=" + recovery.OWNER, recovery.CRYPTO_IMAGE]
        for mutation in ("owner", "image"):
            inspected = [{"Id": "b" * 64, "Name": "/holyarrs-metadata-offhost-deadbeef",
                          "Image": recovery.CRYPTO_IMAGE,
                          "Config": {"Labels": {"holyforge.owner": recovery.OWNER}}}]
            if mutation == "owner":
                inspected[0]["Config"]["Labels"]["holyforge.owner"] = "someone-else"
            else:
                inspected[0]["Image"] = "sha256:" + "0" * 64
            calls = []
            def runner(command, **_kwargs):
                calls.append(command)
                return types.SimpleNamespace(returncode=0, stdout=json.dumps(inspected))
            with self.subTest(mutation=mutation):
                recovery._cleanup_owned_docker(args, runner)
                self.assertEqual(calls, [["docker", "container", "inspect",
                                          "holyarrs-metadata-offhost-deadbeef"]])
    def test_subprocess_failure_is_redacted_and_docker_profile_is_bounded(self):
        def runner(args, **kwargs):
            return type("Result", (), {"returncode": 1, "stdout": "token=secret", "stderr": "password=hunter2"})()

        with self.assertRaisesRegex(recovery.RecoveryError, "operation failed") as caught:
            recovery.run_command(["tool", "arg"], runner=runner)
        self.assertNotIn("secret", str(caught.exception))
        self.assertNotIn("hunter2", str(caught.exception))
        args = offhost.crypto_docker_args("name", pathlib.Path("C:/key.pem"), pathlib.Path("C:/recipient.pem"), pathlib.Path("C:/in"), pathlib.Path("C:/out"))
        joined = " ".join(map(str, args))
        for required in ("--pull=never", "--network none", "--read-only", "--cap-drop ALL", "no-new-privileges:true", "--pids-limit 64", recovery.CRYPTO_IMAGE):
            self.assertIn(required, joined)
        with mock.patch.dict(os.environ, {"HOLYARRS_METADATA_SCHEDULE_ID": "b" * 32}):
            labeled = " ".join(map(str, offhost.crypto_docker_args(
                "name", pathlib.Path("C:/key.pem"), pathlib.Path("C:/recipient.pem"),
                pathlib.Path("C:/in"), pathlib.Path("C:/out"))))
        self.assertIn("holyforge.schedule=" + "b" * 32, labeled)

    def test_binary_output_cap_removes_partial_plaintext(self):
        class Process:
            def __init__(self, *_args, **_kwargs):
                self.stdout = io.BytesIO(b"12345")
                self.returncode = 0
            def wait(self, timeout=None):
                return self.returncode
            def kill(self):
                self.returncode = -1

        with tempfile.TemporaryDirectory() as tmp:
            target = pathlib.Path(tmp) / "plaintext"
            with self.assertRaises(recovery.RecoveryError):
                recovery.run_binary_to_capped_file(["tool"], target, 4, popen=Process)
            self.assertFalse(target.exists())

    def test_owned_staging_cleanup_refuses_unknown_file_without_deleting(self):
        with tempfile.TemporaryDirectory() as tmp:
            stage = pathlib.Path(tmp) / "stage"
            child = stage / "snapshot-parent/snapshot/cache.sqlite"
            child.parent.mkdir(parents=True)
            child.write_bytes(b"db")
            unknown = stage / "unknown"
            unknown.write_bytes(b"keep")
            with self.assertRaises(recovery.RecoveryError):
                recovery.cleanup_owned_staging(stage, [child])
            self.assertTrue(child.exists())
            self.assertTrue(unknown.exists())


if __name__ == "__main__":
    unittest.main()
