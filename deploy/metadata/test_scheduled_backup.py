import datetime as dt
import hashlib
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock


SCRIPT = pathlib.Path(__file__).with_name("scheduled-backup.py")
SPEC = importlib.util.spec_from_file_location("scheduled_backup", SCRIPT)
scheduled = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = scheduled
SPEC.loader.exec_module(scheduled)


class Eastern2026(dt.tzinfo):
    """Small rule-based zone used to exercise the 2026 spring transition."""
    def utcoffset(self, value):
        if value is None:
            return None
        transition = dt.datetime(2026, 3, 8, 3)
        wall = value.replace(tzinfo=None)
        return dt.timedelta(hours=-4 if wall >= transition else -5)

    def dst(self, value):
        return self.utcoffset(value) - dt.timedelta(hours=-5)

    def tzname(self, value):
        return "EDT" if self.dst(value) else "EST"

    def fromutc(self, value):
        naive_utc = value.replace(tzinfo=None)
        transition = dt.datetime(2026, 3, 8, 7)
        offset = dt.timedelta(hours=-4 if naive_utc >= transition else -5)
        return (naive_utc + offset).replace(tzinfo=self)


def write_verified_point(root, name, verified_at):
    point = root / name
    point.mkdir()
    ciphertext = b"ciphertext"
    (point / (name + ".cms")).write_bytes(ciphertext)
    receipt = {
        "schema_version": 1,
        "status": "verified",
        "verified_at": verified_at,
        "run_name": name,
        "ciphertext_bytes": len(ciphertext),
        "ciphertext_sha256": hashlib.sha256(ciphertext).hexdigest(),
        "identities": scheduled.offhost.recovery.expected_identity_manifest(),
        "all_tokens_revoked_on_restore": True,
    }
    (point / "verified-receipt.json").write_text(json.dumps(receipt), encoding="utf-8")
    (root / "latest.json").write_text(json.dumps(receipt), encoding="utf-8")
    return receipt


class SlotTests(unittest.TestCase):
    def test_latest_local_slot_handles_before_trigger_and_spring_forward(self):
        zone = Eastern2026()
        before = dt.datetime(2026, 10, 15, 1, 0, tzinfo=zone)
        self.assertEqual(scheduled.latest_slot_date(before), dt.date(2026, 10, 14))
        after = dt.datetime(2026, 10, 15, 3, 0, tzinfo=zone)
        self.assertEqual(scheduled.latest_slot_date(after), dt.date(2026, 10, 15))
        spring = dt.datetime(2026, 3, 8, 4, 0, tzinfo=zone)
        self.assertEqual(scheduled.latest_slot_date(spring), dt.date(2026, 3, 8))
        before_gap = dt.datetime(2026, 3, 8, 6, 30, tzinfo=dt.timezone.utc)
        after_gap = dt.datetime(2026, 3, 8, 7, 5, tzinfo=dt.timezone.utc)
        self.assertFalse(scheduled.verified_covers_latest_slot(before_gap, spring))
        self.assertTrue(scheduled.verified_covers_latest_slot(after_gap, spring))
        self.assertFalse(scheduled.verified_covers_latest_slot(
            dt.datetime(2026, 3, 9, 7, 5, tzinfo=dt.timezone.utc), spring))


class ScheduleTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)
        self.secrets = self.root / "metadata-backup"
        self.destination = self.secrets / "backups"
        self.state = self.secrets / "task"
        self.destination.mkdir(parents=True)
        self.state.mkdir()
        self.zone = dt.timezone(dt.timedelta(hours=-4), "EDT")

    def tearDown(self):
        self.tmp.cleanup()

    def test_defaults_use_fixed_profile_backups_and_task_siblings(self):
        root = pathlib.Path("C:/Users/Luis/holyarrs-metadata-backup")
        self.assertEqual(scheduled.DEFAULT_DESTINATION, root / "backups")
        self.assertEqual(scheduled.DEFAULT_STATE_DIR, root / "task")

    def test_daily_always_runs_even_after_current_success(self):
        write_verified_point(self.destination, "run-20261015T070000Z-aaaaaaaa", "2026-10-15T07:00:00Z")
        calls = []

        def runner(args, **kwargs):
            calls.append((args, kwargs))
            return subprocess.CompletedProcess(args, 0, '{"status":"verified"}', "")

        result = scheduled.run_schedule(
            "daily", self.destination, state_dir=self.state, secrets_root=self.secrets,
            fixed_destination=self.destination, runner=runner,
            now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))
        self.assertEqual(result["last_result"], "success")
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][0], [sys.executable, "-I", "-S", "-B",
                                      str(SCRIPT.with_name("offhost-backup.py")),
                                      "--destination", str(self.destination.absolute())])
        self.assertLessEqual(calls[0][1]["timeout"], 1800)
        self.assertEqual(calls[0][1]["stdout"], subprocess.PIPE)
        self.assertEqual(calls[0][1]["stderr"], subprocess.PIPE)
        self.assertFalse((self.state / scheduled.GUARD_NAME).exists())

    def test_logon_skips_current_slot_and_runs_missed_slot(self):
        now = dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone)
        write_verified_point(self.destination, "run-20261015T070000Z-aaaaaaaa", "2026-10-15T07:00:00Z")
        calls = []
        runner = lambda *args, **kwargs: calls.append((args, kwargs))
        skipped = scheduled.run_schedule(
            "logon", self.destination, state_dir=self.state, secrets_root=self.secrets,
            fixed_destination=self.destination, runner=runner, now=now)
        self.assertEqual(skipped["last_result"], "skipped")
        self.assertEqual(calls, [])

        write_verified_point(self.destination, "run-20261014T070000Z-bbbbbbbb", "2026-10-14T07:00:00Z")

        def successful(args, **kwargs):
            calls.append((args, kwargs))
            return subprocess.CompletedProcess(args, 0, "safe", "")

        caught_up = scheduled.run_schedule(
            "logon", self.destination, state_dir=self.state, secrets_root=self.secrets,
            fixed_destination=self.destination, runner=successful, now=now)
        self.assertEqual(caught_up["last_result"], "success")
        self.assertEqual(len(calls), 1)

    def test_invalid_failed_latest_never_counts_as_success(self):
        (self.destination / "latest.json").write_text(
            json.dumps({"schema_version": 1, "status": "failed", "verified_at": "2026-10-15T07:00:00Z"}),
            encoding="utf-8")
        calls = []

        def successful(args, **kwargs):
            calls.append(args)
            return subprocess.CompletedProcess(args, 0, "", "")

        result = scheduled.run_schedule(
            "logon", self.destination, state_dir=self.state, secrets_root=self.secrets,
            fixed_destination=self.destination, runner=successful,
            now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))
        self.assertEqual(result["last_result"], "success")
        self.assertEqual(len(calls), 1)

    def test_malformed_latest_types_paths_and_future_times_all_catch_up(self):
        now = dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone)
        malformed = [[], None, 4, "value", {"status": "verified"},
                     {"status": "verified", "run_name": "../run-20261015T070000Z-aaaaaaaa"},
                     {"status": "verified", "run_name": "C:/run-20261015T070000Z-aaaaaaaa"}]
        calls = []
        def successful(args, **kwargs):
            calls.append(args)
            return subprocess.CompletedProcess(args, 0, "", "")
        for value in malformed:
            with self.subTest(value=value):
                (self.destination / "latest.json").write_text(json.dumps(value), encoding="utf-8")
                scheduled.run_schedule("logon", self.destination, state_dir=self.state,
                                       secrets_root=self.secrets, fixed_destination=self.destination,
                                       runner=successful, now=now)
        for index, timestamp in enumerate(("2026-10-15T13:00:00Z", "2026-10-16T07:00:00Z")):
            name = f"run-20261015T07000{index}Z-{index + 1:08x}"
            write_verified_point(self.destination, name, timestamp)
            scheduled.run_schedule("logon", self.destination, state_dir=self.state,
                                   secrets_root=self.secrets, fixed_destination=self.destination,
                                   runner=successful, now=now)
        self.assertEqual(len(calls), len(malformed) + 2)

    def test_nested_identity_type_drift_catches_up_without_exception(self):
        now = dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone)
        name = "run-20261015T070000Z-aaaaaaaa"
        write_verified_point(self.destination, name, "2026-10-15T07:00:00Z")
        latest_path = self.destination / "latest.json"
        receipt_path = self.destination / name / "verified-receipt.json"
        latest = json.loads(latest_path.read_text(encoding="utf-8"))
        latest["identities"] = []
        latest_path.write_text(json.dumps(latest), encoding="utf-8")
        receipt_path.write_text(json.dumps(latest), encoding="utf-8")
        calls = []
        def successful(args, **kwargs):
            calls.append(args)
            return subprocess.CompletedProcess(args, 0, "", "")
        result = scheduled.run_schedule(
            "logon", self.destination, state_dir=self.state, secrets_root=self.secrets,
            fixed_destination=self.destination, runner=successful, now=now)
        self.assertEqual(result["last_result"], "success")
        self.assertEqual(len(calls), 1)

    def test_latest_and_status_reparse_leaves_are_refused(self):
        latest = self.destination / "latest.json"
        latest.write_text("{}", encoding="utf-8")
        status = self.state / "schedule-status.json"
        status.write_text(json.dumps(scheduled._empty_status()), encoding="utf-8")
        original = scheduled.offhost._is_reparse
        with mock.patch.object(scheduled.offhost, "_is_reparse",
                               side_effect=lambda path: pathlib.Path(path).name in {"latest.json", "schedule-status.json"}):
            self.assertIsNone(scheduled.latest_verified_at(self.destination))
            with self.assertRaises(scheduled.ScheduleError):
                scheduled.read_schedule_status(status)
        self.assertFalse(original(latest))

    def test_cross_mode_lock_refuses_overlap(self):
        lock = scheduled.ScheduleLock(self.state / "schedule.lock")
        lock.acquire()
        try:
            with self.assertRaises(scheduled.ScheduleError):
                scheduled.run_schedule(
                    "daily", self.destination, state_dir=self.state, secrets_root=self.secrets,
                    fixed_destination=self.destination,
                    runner=lambda *_args, **_kwargs: self.fail("must not execute"),
                    now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))
        finally:
            lock.release()

    def test_failure_redacts_output_and_preserves_prior_success(self):
        status_path = self.state / "schedule-status.json"
        prior = {
            "schema_version": 1, "last_attempt_at": "2026-10-14T12:00:00Z",
            "last_mode": "daily", "last_result": "success", "last_success_at": "2026-10-14T12:00:00Z",
            "last_success_mode": "daily", "error": None,
        }
        status_path.write_text(json.dumps(prior), encoding="utf-8")

        def failed(args, **kwargs):
            return subprocess.CompletedProcess(args, 1, "sensitive-child-output", "private-child-output")

        with self.assertRaises(scheduled.ScheduleError):
            scheduled.run_schedule(
                "daily", self.destination, state_dir=self.state, secrets_root=self.secrets,
                fixed_destination=self.destination, runner=failed,
                now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))
        saved = json.loads(status_path.read_text(encoding="utf-8"))
        self.assertEqual(saved["last_result"], "failed")
        self.assertEqual(saved["last_success_at"], prior["last_success_at"])
        self.assertEqual(saved["last_success_mode"], prior["last_success_mode"])
        self.assertNotIn("sensitive-child-output", status_path.read_text(encoding="utf-8"))
        self.assertNotIn("private-child-output", status_path.read_text(encoding="utf-8"))
        self.assertTrue((self.state / scheduled.GUARD_NAME).is_file())
        for mode in ("daily", "logon"):
            with self.subTest(mode=mode), self.assertRaisesRegex(
                    scheduled.ScheduleError, "manual recovery"):
                scheduled.run_schedule(
                    mode, self.destination, state_dir=self.state, secrets_root=self.secrets,
                    fixed_destination=self.destination,
                    runner=lambda *_args, **_kwargs: self.fail("guard must block child launch"),
                    now=dt.datetime(2026, 10, 15, 8, 5, tzinfo=self.zone))

    def test_timeout_or_failed_tree_cleanup_retains_guard_and_blocks_both_modes(self):
        with mock.patch.object(scheduled, "run_backup_process",
                               side_effect=scheduled.ScheduleError("scheduled backup failed")):
            with self.assertRaises(scheduled.ScheduleError):
                scheduled.run_schedule(
                    "daily", self.destination, state_dir=self.state, secrets_root=self.secrets,
                    fixed_destination=self.destination, now=dt.datetime(
                        2026, 10, 15, 8, 0, tzinfo=self.zone))
        self.assertTrue((self.state / scheduled.GUARD_NAME).is_file())
        for mode in ("daily", "logon"):
            with self.subTest(mode=mode), self.assertRaisesRegex(
                    scheduled.ScheduleError, "manual recovery"):
                scheduled.run_schedule(
                    mode, self.destination, state_dir=self.state, secrets_root=self.secrets,
                    fixed_destination=self.destination,
                    runner=lambda *_args, **_kwargs: self.fail("guard must block child launch"),
                    now=dt.datetime(2026, 10, 15, 8, 5, tzinfo=self.zone))

    def test_preexisting_malformed_or_reparse_guard_blocks_launch(self):
        guard = self.state / scheduled.GUARD_NAME
        guard.write_text("[]", encoding="utf-8")
        with self.assertRaisesRegex(scheduled.ScheduleError, "guard is invalid"):
            scheduled.run_schedule(
                "daily", self.destination, state_dir=self.state, secrets_root=self.secrets,
                fixed_destination=self.destination,
                runner=lambda *_args, **_kwargs: self.fail("invalid guard must block"),
                now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))
        guard.unlink()
        target = self.state / "guard-target.json"
        target.write_text("{}", encoding="utf-8")
        try:
            guard.symlink_to(target)
        except OSError:
            self.skipTest("file symlinks unavailable")
        with self.assertRaisesRegex(scheduled.ScheduleError, "guard is unsafe"):
            scheduled.run_schedule(
                "logon", self.destination, state_dir=self.state, secrets_root=self.secrets,
                fixed_destination=self.destination,
                runner=lambda *_args, **_kwargs: self.fail("reparse guard must block"),
                now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))

    def test_valid_preexisting_guard_models_abrupt_parent_exit_and_never_expires(self):
        guard = {"schema_version": 1, "schedule_id": "c" * 32,
                 "started_at": "2020-01-01T00:00:00Z", "mode": "daily"}
        (self.state / scheduled.GUARD_NAME).write_text(json.dumps(guard), encoding="utf-8")
        for mode in ("daily", "logon"):
            with self.subTest(mode=mode), self.assertRaisesRegex(
                    scheduled.ScheduleError, "manual recovery"):
                scheduled.run_schedule(
                    mode, self.destination, state_dir=self.state, secrets_root=self.secrets,
                    fixed_destination=self.destination,
                    runner=lambda *_args, **_kwargs: self.fail("durable guard must block"),
                    now=dt.datetime(2026, 10, 15, 8, 0, tzinfo=self.zone))

    def test_fixed_destination_and_separate_state_directory_are_required(self):
        with self.assertRaises(scheduled.ScheduleError):
            scheduled.validate_paths(self.secrets / "other", self.state, self.secrets, self.destination)
        with self.assertRaises(scheduled.ScheduleError):
            scheduled.validate_paths(self.destination, self.destination / "state", self.secrets, self.destination)

    def test_timeout_terminates_exact_process_tree_and_owned_crypto(self):
        class Process:
            pid = 4321
            returncode = None
            def communicate(self, timeout=None):
                raise subprocess.TimeoutExpired("child", timeout)
            def wait(self, timeout=None):
                self.returncode = -1
                return -1
            def kill(self):
                self.returncode = -1

        tree_calls = []
        docker_calls = []
        def tree_runner(args, **kwargs):
            tree_calls.append(args)
            return subprocess.CompletedProcess(args, 0, "", "")
        def docker_runner(args, **kwargs):
            docker_calls.append(args)
            return subprocess.CompletedProcess(args, 0, "", "")
        schedule_id = "a" * 32
        with self.assertRaises(scheduled.ScheduleError):
            scheduled.run_backup_process([sys.executable, "child.py"], schedule_id, timeout=.01,
                                         popen=lambda *_args, **_kwargs: Process(),
                                         tree_runner=tree_runner, cleanup_runner=docker_runner)
        if scheduled.os.name == "nt":
            self.assertEqual(tree_calls, [["taskkill", "/PID", "4321", "/T", "/F"]])
        self.assertEqual(len(docker_calls), 2)
        self.assertTrue(all("holyforge.schedule=" + schedule_id in " ".join(call) for call in docker_calls))

    def test_timeout_terminates_a_real_long_lived_grandchild(self):
        pid_file = self.root / "grandchild.pid"
        child = (
            "import pathlib,subprocess,sys,time;"
            "p=subprocess.Popen([sys.executable,'-I','-S','-B','-c','import time; time.sleep(60)']);"
            "pathlib.Path(sys.argv[1]).write_text(str(p.pid));"
            "time.sleep(60)"
        )

        def no_crypto(args, **_kwargs):
            output = "" if args[1:4] == ["container", "ls", "-aq"] else ""
            return subprocess.CompletedProcess(args, 0, output, "")

        with self.assertRaises(scheduled.ScheduleError):
            scheduled.run_backup_process(
                [sys.executable, "-I", "-S", "-B", "-c", child, str(pid_file)], "b" * 32,
                timeout=1, cleanup_runner=no_crypto)
        self.assertTrue(pid_file.is_file(), "grandchild did not start before the deadline")
        pid = int(pid_file.read_text(encoding="utf-8"))
        deadline = time.monotonic() + 3
        alive = True
        while alive and time.monotonic() < deadline:
            if os.name == "nt":
                listed = subprocess.run(
                    ["tasklist", "/FI", f"PID eq {pid}", "/FO", "CSV", "/NH"],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5)
                alive = f'"{pid}"' in listed.stdout
            else:
                try:
                    os.kill(pid, 0)
                except ProcessLookupError:
                    alive = False
            if alive:
                time.sleep(.05)
        self.assertFalse(alive, "scheduled process-tree cleanup left its grandchild running")


if __name__ == "__main__":
    unittest.main()
