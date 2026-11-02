"""Run the fixed Windows metadata backup on its daily or logon schedule."""
import argparse
import contextlib
import datetime as dt
import importlib.util
import json
import os
import pathlib
import re
import signal
import subprocess
import sys
import uuid


def _load_offhost():
    path = pathlib.Path(__file__).with_name("offhost-backup.py")
    spec = importlib.util.spec_from_file_location("holyarrs_offhost_backup", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


offhost = _load_offhost()
DEFAULT_DESTINATION = offhost.DEFAULT_DESTINATION
DEFAULT_STATE_DIR = offhost.DEFAULT_SECRETS_ROOT / "task"
STATUS_NAME = "schedule-status.json"
LOCK_NAME = "schedule.lock"
GUARD_NAME = "in-progress.json"
BACKUP_TIMEOUT_SECONDS = 1800
SCHEDULE_ID_RE = re.compile(r"[0-9a-f]{32}\Z")


class ScheduleError(RuntimeError):
    pass


def _same_path(left, right):
    return os.path.normcase(str(pathlib.Path(left).absolute())) == os.path.normcase(
        str(pathlib.Path(right).absolute()))


def validate_paths(destination, state_dir, secrets_root=offhost.DEFAULT_SECRETS_ROOT,
                   fixed_destination=DEFAULT_DESTINATION):
    destination = offhost.validate_destination(destination, secrets_root)
    state_dir = offhost.validate_destination(state_dir, secrets_root)
    if not _same_path(destination, fixed_destination):
        raise ScheduleError("backup destination does not match the fixed task destination")
    if state_dir.parent != destination.parent or state_dir.name != DEFAULT_STATE_DIR.name:
        raise ScheduleError("task state directory must be the dedicated task sibling")
    if _same_path(state_dir, destination) or destination in state_dir.parents or state_dir in destination.parents:
        raise ScheduleError("task state directory must be separate from backup data")
    return destination, state_dir


def latest_slot_date(now):
    if not isinstance(now, dt.datetime) or now.tzinfo is None or now.utcoffset() is None:
        raise ScheduleError("current local time must be timezone aware")
    if now.timetz().replace(tzinfo=None) < dt.time(2, 15):
        return now.date() - dt.timedelta(days=1)
    return now.date()


def verified_covers_latest_slot(verified_at, now, *, system_local=False):
    if verified_at.tzinfo is None or verified_at.utcoffset() is None:
        raise ScheduleError("verified time must be timezone aware")
    if verified_at > now.astimezone(dt.timezone.utc):
        return False
    slot_date = latest_slot_date(now)
    local_verified = verified_at.astimezone() if system_local else verified_at.astimezone(now.tzinfo)
    wall = (local_verified.date(), local_verified.timetz().replace(tzinfo=None))
    return wall >= (slot_date, dt.time(2, 15))


def _iso_utc(value):
    return value.astimezone(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def latest_verified_at(destination):
    latest_path = pathlib.Path(destination) / "latest.json"
    try:
        if (latest_path.is_symlink() or offhost._is_reparse(latest_path)
                or not latest_path.is_file() or latest_path.stat().st_size > 64 * 1024):
            return None
        latest = json.loads(latest_path.read_text(encoding="utf-8"))
        if not isinstance(latest, dict) or latest.get("status") != "verified":
            return None
        run_name = latest.get("run_name")
        offhost.recovery.validate_run_name(run_name)
        point = pathlib.Path(destination) / run_name
        if point.parent != pathlib.Path(destination):
            return None
        receipt, _size = offhost._verified_point(point)
        if receipt != latest:
            return None
        return offhost.recovery.parse_time(receipt["verified_at"])
    except (OSError, ValueError, TypeError, KeyError, json.JSONDecodeError, offhost.RecoveryError):
        return None


def _empty_status():
    return {"schema_version": 1, "last_attempt_at": None, "last_mode": None,
            "last_result": "never", "last_success_at": None,
            "last_success_mode": None, "error": None}


def read_schedule_status(path):
    path = pathlib.Path(path)
    if path.is_symlink() or offhost._is_reparse(path):
        raise ScheduleError("schedule status path is unsafe")
    if not path.exists():
        return _empty_status()
    if not path.is_file():
        raise ScheduleError("schedule status path is unsafe")
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        raise ScheduleError("schedule status is invalid") from None
    required = set(_empty_status())
    if not isinstance(value, dict) or set(value) != required or value["schema_version"] != 1:
        raise ScheduleError("schedule status schema is invalid")
    if value["last_result"] not in {"never", "success", "failed", "skipped"}:
        raise ScheduleError("schedule status result is invalid")
    for key in ("last_attempt_at", "last_success_at"):
        if value[key] is not None:
            offhost.recovery.parse_time(value[key])
    for key in ("last_mode", "last_success_mode"):
        if value[key] not in {None, "daily", "logon"}:
            raise ScheduleError("schedule status mode is invalid")
    if value["error"] not in {None, "scheduled backup failed"}:
        raise ScheduleError("schedule status error is invalid")
    return value


def read_run_guard(path):
    path = pathlib.Path(path)
    if path.is_symlink() or offhost._is_reparse(path):
        raise ScheduleError("schedule run guard is unsafe")
    if not path.exists():
        return None
    if not path.is_file() or path.stat().st_size > 64 * 1024:
        raise ScheduleError("schedule run guard is unsafe")
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        raise ScheduleError("schedule run guard is invalid") from None
    required = {"schema_version", "schedule_id", "started_at", "mode"}
    if (not isinstance(value, dict) or set(value) != required or value["schema_version"] != 1
            or SCHEDULE_ID_RE.fullmatch(value.get("schedule_id", "")) is None
            or value.get("mode") not in {"daily", "logon"}):
        raise ScheduleError("schedule run guard is invalid")
    try:
        offhost.recovery.parse_time(value.get("started_at"))
    except offhost.RecoveryError:
        raise ScheduleError("schedule run guard is invalid") from None
    return value


def create_run_guard(path, mode, schedule_id, started_at):
    path = pathlib.Path(path)
    if read_run_guard(path) is not None:
        raise ScheduleError("scheduled backup requires manual recovery")
    value = {"schema_version": 1, "schedule_id": schedule_id,
             "started_at": started_at, "mode": mode}
    offhost.recovery.atomic_json(path, value)
    if read_run_guard(path) != value:
        raise ScheduleError("schedule run guard could not be verified")
    return value


def clear_run_guard(path, expected):
    path = pathlib.Path(path)
    if read_run_guard(path) != expected:
        raise ScheduleError("schedule run guard changed")
    path.unlink()


class ScheduleLock:
    def __init__(self, path):
        self.path = pathlib.Path(path)
        self.handle = None

    def acquire(self):
        if self.handle is not None:
            raise ScheduleError("schedule lock is already held")
        if (self.path.exists()
                and (self.path.is_symlink() or offhost._is_reparse(self.path) or not self.path.is_file())):
            raise ScheduleError("schedule lock path is unsafe")
        self.handle = self.path.open("a+b")
        try:
            self.handle.seek(0, os.SEEK_END)
            if self.handle.tell() == 0:
                self.handle.write(b"\0")
                self.handle.flush()
            self.handle.seek(0)
            if os.name == "nt":
                import msvcrt
                msvcrt.locking(self.handle.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(self.handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except (OSError, BlockingIOError):
            self.handle.close()
            self.handle = None
            raise ScheduleError("another scheduled backup is already running") from None

    def release(self):
        if self.handle is None:
            return
        try:
            self.handle.seek(0)
            if os.name == "nt":
                import msvcrt
                msvcrt.locking(self.handle.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                import fcntl
                fcntl.flock(self.handle.fileno(), fcntl.LOCK_UN)
        finally:
            self.handle.close()
            self.handle = None

    def __enter__(self):
        self.acquire()
        return self

    def __exit__(self, _kind, _value, _traceback):
        self.release()


def _bounded_run(runner, args, **kwargs):
    return runner(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30,
                  text=True, **kwargs)


def cleanup_scheduled_crypto(schedule_id, runner=subprocess.run):
    if SCHEDULE_ID_RE.fullmatch(schedule_id) is None:
        raise ScheduleError("scheduled cleanup identity is invalid")
    filters = ["--filter", "label=holyforge.owner=" + offhost.recovery.OWNER,
               "--filter", "label=holyforge.schedule=" + schedule_id,
               "--filter", "ancestor=" + offhost.recovery.CRYPTO_IMAGE]
    listed = _bounded_run(runner, ["docker", "container", "ls", "-aq", *filters])
    if listed.returncode:
        raise ScheduleError("scheduled descendant cleanup failed")
    identifiers = [line.strip() for line in listed.stdout.splitlines() if line.strip()]
    if any(re.fullmatch(r"[0-9a-f]{12,64}", value) is None for value in identifiers):
        raise ScheduleError("scheduled descendant cleanup failed")
    for identifier in identifiers:
        checked = _bounded_run(
            runner, ["docker", "container", "inspect", "--format",
                     "{{.Id}} {{.Image}} {{index .Config.Labels \"holyforge.owner\"}} {{index .Config.Labels \"holyforge.schedule\"}}",
                     identifier])
        fields = checked.stdout.strip().split()
        if (checked.returncode or len(fields) != 4 or fields[1] != offhost.recovery.CRYPTO_IMAGE
                or fields[2] != offhost.recovery.OWNER or fields[3] != schedule_id):
            raise ScheduleError("scheduled descendant cleanup identity mismatch")
        removed = _bounded_run(runner, ["docker", "rm", "-f", identifier])
        if removed.returncode:
            raise ScheduleError("scheduled descendant cleanup failed")
    remaining = _bounded_run(runner, ["docker", "container", "ls", "-aq", *filters])
    if remaining.returncode or remaining.stdout.strip():
        raise ScheduleError("scheduled descendant cleanup is incomplete")


def run_backup_process(command, schedule_id, *, timeout=BACKUP_TIMEOUT_SECONDS,
                       popen=subprocess.Popen, cleanup_runner=subprocess.run,
                       tree_runner=subprocess.run):
    environment = os.environ.copy()
    environment["HOLYARRS_METADATA_SCHEDULE_ID"] = schedule_id
    options = {"text": True, "stdout": subprocess.PIPE, "stderr": subprocess.PIPE, "env": environment}
    if os.name == "nt":
        options["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    else:
        options["start_new_session"] = True
    process = popen(command, **options)
    try:
        stdout, stderr = process.communicate(timeout=timeout)
        return subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
    except subprocess.TimeoutExpired:
        if os.name == "nt":
            killed = tree_runner(["taskkill", "/PID", str(process.pid), "/T", "/F"],
                                 stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                 timeout=30, check=False)
            if killed.returncode:
                with contextlib.suppress(OSError):
                    process.kill()
        else:
            with contextlib.suppress(OSError):
                os.killpg(process.pid, signal.SIGKILL)
        with contextlib.suppress(OSError, subprocess.SubprocessError):
            process.wait(timeout=30)
        cleanup_scheduled_crypto(schedule_id, cleanup_runner)
        raise ScheduleError("scheduled backup failed") from None


def run_schedule(mode, destination, *, state_dir=DEFAULT_STATE_DIR,
                 secrets_root=offhost.DEFAULT_SECRETS_ROOT,
                 fixed_destination=DEFAULT_DESTINATION, runner=None, now=None):
    if mode not in {"daily", "logon"}:
        raise ScheduleError("schedule mode is invalid")
    destination, state_dir = validate_paths(destination, state_dir, secrets_root, fixed_destination)
    state_dir.mkdir(mode=0o700, parents=False, exist_ok=True)
    # Revalidate after creation so a concurrent reparse substitution is refused.
    destination, state_dir = validate_paths(destination, state_dir, secrets_root, fixed_destination)
    status_path = state_dir / STATUS_NAME
    use_system_local = now is None
    current = now or dt.datetime.now().astimezone()
    if current.tzinfo is None or current.utcoffset() is None:
        raise ScheduleError("current local time must be timezone aware")
    attempt_at = _iso_utc(current)
    with ScheduleLock(state_dir / LOCK_NAME):
        guard_path = state_dir / GUARD_NAME
        if read_run_guard(guard_path) is not None:
            raise ScheduleError("scheduled backup requires manual recovery")
        previous = read_schedule_status(status_path)
        if mode == "logon":
            latest = latest_verified_at(destination)
            if (latest is not None
                    and verified_covers_latest_slot(latest, current, system_local=use_system_local)):
                result = dict(previous, last_attempt_at=attempt_at, last_mode=mode,
                              last_result="skipped", error=None)
                offhost.recovery.atomic_json(status_path, result)
                return result
        command = [sys.executable, "-I", "-S", "-B",
                   str(pathlib.Path(__file__).with_name("offhost-backup.py")),
                   "--destination", str(destination)]
        schedule_id = uuid.uuid4().hex
        guard = create_run_guard(guard_path, mode, schedule_id, attempt_at)
        try:
            if runner is None:
                completed = run_backup_process(command, schedule_id)
            else:
                environment = os.environ.copy()
                environment["HOLYARRS_METADATA_SCHEDULE_ID"] = schedule_id
                completed = runner(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   timeout=BACKUP_TIMEOUT_SECONDS, env=environment)
            if not hasattr(completed, "returncode") or completed.returncode != 0:
                raise ScheduleError("scheduled backup failed")
            clear_run_guard(guard_path, guard)
        except (OSError, subprocess.SubprocessError, ScheduleError):
            failed = dict(previous, last_attempt_at=attempt_at, last_mode=mode,
                          last_result="failed", error="scheduled backup failed")
            offhost.recovery.atomic_json(status_path, failed)
            raise ScheduleError("scheduled backup failed") from None
        result = dict(previous, last_attempt_at=attempt_at, last_mode=mode,
                      last_result="success", last_success_at=attempt_at,
                      last_success_mode=mode, error=None)
        offhost.recovery.atomic_json(status_path, result)
        return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("daily", "logon"), required=True)
    parser.add_argument("--destination", required=True, type=pathlib.Path)
    args = parser.parse_args()
    try:
        result = run_schedule(args.mode, args.destination)
        print(json.dumps(result, sort_keys=True))
    except Exception as error:
        message = str(error) if isinstance(error, ScheduleError) else "scheduled backup failed"
        print(json.dumps({"schema_version": 1, "status": "failed", "error": message}, sort_keys=True))
        raise SystemExit(1)


if __name__ == "__main__":
    main()
