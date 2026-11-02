import contextlib
import hashlib
import importlib.util
import io
import pathlib
import sqlite3
import tempfile
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("recovery", pathlib.Path(__file__).with_name("recovery.py"))
recovery = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(recovery)


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name)
        self.source = self.root / "stopped-validation"
        self.source.mkdir()
        self.token = self.root / "credential"
        self.token.write_text("synthetic-test-secret\n")
        with contextlib.closing(sqlite3.connect(self.source / "cache.sqlite")) as db, db:
            db.executescript("PRAGMA user_version=1; CREATE TABLE entries(key TEXT PRIMARY KEY, data BLOB);")
            db.execute("INSERT INTO entries VALUES (?,?)", ("synthetic", b'{"title":"Synthetic"}'))
        with contextlib.closing(sqlite3.connect(self.source / "access.sqlite")) as db, db:
            db.executescript("PRAGMA user_version=1; CREATE TABLE tokens(hash BLOB PRIMARY KEY, revoked INTEGER NOT NULL);")
            db.execute("INSERT INTO tokens VALUES (?,0)", (hashlib.sha256(b"synthetic-test-secret").digest(),))

    def tearDown(self):
        self.temp.cleanup()

    def test_guards_reject_source_existing_targets_and_ambiguous_prefixes(self):
        for destination in (self.source, self.source / "metadata-recovery-child", self.root, self.root / "existing-service"):
            with self.subTest(destination=destination), self.assertRaises(ValueError):
                recovery.validate_paths(self.source, destination, self.token)
        work = self.root / "metadata-recovery-valid"
        recovery.validate_paths(self.source, work, self.token)
        work.mkdir()
        with self.assertRaises(ValueError):
            recovery.validate_paths(self.source, work, self.token)

    def test_docker_is_confined_and_uses_no_shell_or_token_value(self):
        work = self.root / "metadata-recovery-valid"
        work.mkdir()
        with mock.patch.object(recovery.subprocess, "run") as run:
            run.return_value.returncode = 0
            result = recovery.docker_command("synthetic-image:private", work, "check", ["check", "--data-dir", "/data"])
        args, kwargs = run.call_args
        command = args[0]
        self.assertEqual(command[:2], ["docker", "run"])
        for flag in ("--read-only", "--restart=no", "--network=none", "--cpus=0.5", "--memory=512m", "--user=10001:10001", "--pull=never"):
            self.assertIn(flag, command)
        self.assertIn("com.centurylinklabs.watchtower.enable=false", command)
        self.assertFalse(any(arg.startswith(("-p", "--publish", "--privileged")) for arg in command))
        self.assertNotIn("synthetic-test-secret", str(command))
        self.assertNotIn("shell", kwargs)
        self.assertEqual(result["exit_status"], 0)

    def test_recovery_scenarios_never_mutate_source_and_preserve_revocation(self):
        before = recovery.file_manifest(self.source)
        work = self.root / "metadata-recovery-suite"
        def fake_docker(image, data, label, arguments, **mounts):
            data = pathlib.Path(data)
            code = 0
            if arguments[0] == "token-revoke":
                with contextlib.closing(sqlite3.connect(data / "access.sqlite")) as db, db:
                    db.execute("UPDATE tokens SET revoked=1")
            elif arguments[0] == "backup":
                destination = pathlib.Path(mounts["backup"]) / "snapshot"
                destination.mkdir()
                for name in recovery.DATABASES:
                    with contextlib.closing(sqlite3.connect(data / name)) as original, contextlib.closing(sqlite3.connect(destination / name)) as copied:
                        original.backup(copied)
            else:
                try:
                    for name in recovery.DATABASES:
                        with contextlib.closing(sqlite3.connect(data / name)) as db:
                            if db.execute("PRAGMA user_version").fetchone()[0] != 1 or db.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
                                code = 1
                except sqlite3.DatabaseError:
                    code = 1
            return {"name": label, "exit_status": code, "timed_out": False}
        with mock.patch.object(recovery, "docker_command", side_effect=fake_docker), mock.patch.object(recovery, "secure_tree"):
            result = recovery.run_recovery("synthetic-image:private", self.source, work, self.token)
        self.assertTrue(result["passed"], result)
        self.assertEqual(before, recovery.file_manifest(self.source))
        self.assertTrue(result["scenarios"]["revocation"]["selected_token_revoked"])
        self.assertTrue(result["scenarios"]["restored_backup"]["revocation_preserved"])
        for name in ("future_cache", "future_access", "corrupt_cache", "corrupt_access"):
            self.assertTrue(result["scenarios"][name]["refused"])
        output = str(result)
        self.assertNotIn("synthetic-test-secret", output)


if __name__ == "__main__":
    unittest.main()
