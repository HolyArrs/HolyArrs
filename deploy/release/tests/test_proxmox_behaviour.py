"""The Proxmox scripts run against stand-in commands: what they stop, copy, create and print."""
import hashlib
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
LXC = ROOT / "deploy" / "proxmox" / "holyarrs-lxc.sh"
INSTALL = ROOT / "deploy" / "proxmox" / "holyarrs-install.sh"
GUIDE = ROOT / "docs" / "run" / "proxmox.md"
BASH = shutil.which("bash")


def posix(p) -> str:
    return pathlib.Path(p).as_posix()


def unix_path(p) -> str:
    cyg = shutil.which("cygpath")
    if cyg:
        return subprocess.run([cyg, "-u", str(p)], capture_output=True, text=True).stdout.strip()
    return posix(p)


def write_exe(path: pathlib.Path, body: str) -> None:
    path.write_text("#!/usr/bin/env bash\n" + body, encoding="utf-8", newline="\n")
    path.chmod(0o755)


@unittest.skipUnless(BASH, "bash is needed")
class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp(prefix="hapx-"))
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.stubs = self.tmp / "stubs"
        self.stubs.mkdir()
        self.log = self.tmp / "calls.log"
        self.log.write_text("", encoding="utf-8")

    def env(self, **extra):
        e = dict(os.environ)
        e["PATH"] = posix(self.stubs) + os.pathsep + e["PATH"]
        e["CALLS"] = posix(self.log)
        e.update({k: str(v) for k, v in extra.items()})
        return e

    def calls(self) -> str:
        return self.log.read_text(encoding="utf-8")


class Install(Base):
    def setUp(self):
        super().setUp()
        t = self.tmp
        self.app, self.cfg, self.bak = t / "opt", t / "var-lib", t / "backups"
        self.unit, self.sysrun, self.dl = t / "units", t / "run-systemd", t / "dl"
        for d in (self.app, self.cfg, self.unit, self.sysrun, self.dl):
            d.mkdir()
        (self.cfg / "holyarrs.db").write_text("data", encoding="utf-8")
        for sub, name in (("artwork", "poster.jpg"), ("backups", "old.zip"), ("logs", "app.log")):
            (self.cfg / sub).mkdir()
            (self.cfg / sub / name).write_text("x" * 5000, encoding="utf-8")
        write_exe(self.app / "holyarrs", "exit 0\n")
        (self.app / "version").write_text("1.1.3\n", encoding="utf-8")
        self.health_ok = t / "health-ok"
        self.health_ok.write_text("0", encoding="utf-8")
        s = self.stubs
        write_exe(s / "id", 'echo 0\n')
        write_exe(s / "dpkg", 'echo amd64\n')
        write_exe(s / "apt-get", "exit 0\n")
        write_exe(s / "getent", "exit 0\n")
        write_exe(s / "useradd", "exit 0\n")
        write_exe(s / "hostname", "echo 192.168.1.60\n")
        write_exe(s / "sleep", "exit 0\n")
        self.down_first = t / "down-first"
        write_exe(
            s / "systemctl",
            'if [ "$1" = show ]; then\n'
            '  case "$*" in\n'
            '    *NRestarts*) [ -n "${RESTART_STEP:-}" ] || exit 0\n'
            '      n=$(cat "$RESTARTS" 2>/dev/null || echo 0); echo "$n"; echo $((n + RESTART_STEP)) > "$RESTARTS";;\n'
            '    *Environment*) echo "${UNIT_ENV:-}";;\n'
            '    *ExecStart*) echo "${UNIT_EXEC:-}";;\n'
            '  esac\n  exit 0\nfi\n'
            'echo "systemctl $*" >> "$CALLS"\n'
            'if [ "$1" = is-active ]; then\n'
            '  n=$(cat "$DOWN_FIRST" 2>/dev/null || echo 0)\n'
            '  if [ "$n" -gt 0 ]; then echo $((n - 1)) > "$DOWN_FIRST"; exit 3; fi\n'
            '  exit "${ACTIVE_RC:-0}"\nfi\nexit 0\n',
        )
        write_exe(
            s / "install",
            'a=()\nwhile [ $# -gt 0 ]; do case "$1" in -o|-g|-m) shift 2;; *) a+=("$1"); shift;; esac; done\n'
            'exec /usr/bin/install "${a[@]}"\n',
        )
        write_exe(
            s / "curl",
            'out=""; url=""\nwhile [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift 2;; -*) shift;; *) url="$1"; shift;; esac; done\n'
            'case "$url" in\n'
            '  */latest) printf "%s" "http://stub/releases/tag/${LATEST:-1.1.4}";;\n'
            '  */healthz) echo "healthz" >> "$CALLS"; [ -z "${HEALTH_BODY:-}" ] || printf "%s" "$HEALTH_BODY"; exit 0;;\n'
            '  */readyz) echo "readyz" >> "$CALLS"; echo "probe ${url%/*}" >> "$CALLS"; [ -f "$HEALTH_OK" ] || exit 22\n'
            '    n=$(cat "$HEALTH_OK"); [ "$n" -le 0 ] && exit 0; echo $((n-1)) > "$HEALTH_OK"; exit 22;;\n'
            '  *) cp "$DL/$(basename "$url")" "$out";;\n'
            'esac\n',
        )
        self.make_release("1.1.4")
        self.make_release("1.1.2")

    def make_release(self, version):
        stem = f"holyarrs_{version}_linux_amd64"
        pkg = self.tmp / "pkg" / stem
        pkg.mkdir(parents=True, exist_ok=True)
        write_exe(pkg / "holyarrs", "exit 0\n")
        tgz = self.dl / f"{stem}.tar.gz"
        with tarfile.open(tgz, "w:gz") as t:
            t.add(pkg, arcname=stem)
        sums = self.dl / "SHA256SUMS"
        line = f"{hashlib.sha256(tgz.read_bytes()).hexdigest()}  {tgz.name}\n"
        sums.write_text((sums.read_text() if sums.exists() else "") + line, encoding="utf-8")

    def run_install(self, *args, **extra):
        env = self.env(
            HOLYARRS_APP_DIR=unix_path(self.app), HOLYARRS_CONFIG_DIR=unix_path(self.cfg), HOLYARRS_BACKUP_DIR=unix_path(self.bak),
            HOLYARRS_UNIT_DIR=unix_path(self.unit), HOLYARRS_SYSTEMD_RUN=unix_path(self.sysrun), HOLYARRS_SELF_COPY=unix_path(self.tmp / "self"),
            HOLYARRS_DOWNLOAD_BASE="http://stub/releases", DL=unix_path(self.dl), HEALTH_OK=unix_path(self.health_ok),
            RESTARTS=unix_path(self.tmp / "restarts"), DOWN_FIRST=unix_path(self.down_first), **extra,
        )
        return subprocess.run([BASH, posix(INSTALL), *args], env=env, capture_output=True, text=True)

    def test_update_copy_leaves_out_pictures_logs_and_app_backups(self):
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        copies = list(self.bak.glob("config-before-*.tar.gz"))
        self.assertEqual(len(copies), 1)
        with tarfile.open(copies[0]) as t:
            names = t.getnames()
        self.assertTrue(any(n.endswith("holyarrs.db") for n in names), names)
        for skipped in ("artwork", "backups", "logs"):
            self.assertFalse(any(f"/{skipped}" in n for n in names), (skipped, names))

    def test_update_keeps_only_the_newest_two_copies(self):
        self.bak.mkdir()
        for stamp in ("20260101-000001", "20260101-000002", "20260101-000003"):
            old = self.bak / f"config-before-1.1.0-{stamp}.tar.gz"
            old.write_text("old", encoding="utf-8")
            when = 1_700_000_000 + int(stamp[-1])
            os.utime(old, (when, when))
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        left = sorted(p.name for p in self.bak.glob("config-before-*.tar.gz"))
        self.assertEqual(len(left), 2, left)
        self.assertIn("20260101-000003", left[0] + left[1])

    def test_failed_copy_starts_the_service_again(self):
        write_exe(
            self.stubs / "tar",
            'for a in "$@"; do case "$a" in *config-before*) exit 1;; esac; done\nexec /usr/bin/tar "$@"\n',
        )
        r = self.run_install("update", "1.1.4")
        self.assertNotEqual(r.returncode, 0)
        calls = self.calls().splitlines()
        self.assertIn("systemctl stop holyarrs", calls)
        self.assertEqual(calls[-1], "systemctl start holyarrs", calls)

    def test_too_little_room_stops_before_the_service_is_touched(self):
        write_exe(self.stubs / "df", 'echo "Filesystem 1024-blocks Used Available Capacity Mounted on"\necho "x 100 99 1 99% /"\n')
        (self.cfg / "big.bin").write_bytes(b"0" * 3_000_000)
        r = self.run_install("update", "1.1.4")
        self.assertNotEqual(r.returncode, 0)
        self.assertNotIn("systemctl stop", self.calls())
        self.assertIn("room", r.stderr)

    def test_older_version_is_refused_unless_forced(self):
        r = self.run_install("update", "1.1.2")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("older", r.stderr)
        self.assertNotIn("systemctl stop", self.calls())
        r = self.run_install("update", "1.1.2", "force")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)

    def test_update_waits_while_the_app_only_shows_its_starting_page(self):
        self.health_ok.write_text("7", encoding="utf-8")
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertEqual(self.calls().count("readyz"), 8)
        self.assertLess(r.stdout.index("is running"), len(r.stdout))

    def test_slow_start_waits_and_names_the_way_back(self):
        self.health_ok.unlink()
        r = self.run_install("update", "1.1.4")
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(self.calls().count("readyz"), 300)
        self.assertIn("holyarrs.previous", r.stderr)
        self.assertIn("/backups", r.stderr)

    def newer_program(self):
        (self.app / "holyarrs").write_text("#!/usr/bin/env bash\n# newer\nexit 0\n", encoding="utf-8", newline="\n")

    def way_back(self):
        return (
            sorted(p.name for p in self.bak.glob("config-before-*.tar.gz")),
            (self.app / "holyarrs.previous").read_bytes(),
            (self.app / "version.previous").read_text(encoding="utf-8").strip(),
        )

    def test_update_to_the_version_already_installed_changes_nothing(self):
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        before = self.way_back()
        self.assertEqual(before[2], "1.1.3")
        self.newer_program()
        self.log.write_text("", encoding="utf-8")
        for args in (("update", "1.1.4"), ("update",)):
            with self.subTest(args=args):
                r = self.run_install(*args)
                self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
                self.assertIn("1.1.4 is already installed", r.stdout)
                self.assertNotIn("systemctl", self.calls())
                self.assertEqual(self.way_back(), before)

    def test_forced_same_version_keeps_the_way_back(self):
        self.assertEqual(self.run_install("update", "1.1.4").returncode, 0)
        before = self.way_back()
        self.newer_program()
        for _ in range(3):
            r = self.run_install("update", "1.1.4", "force")
            self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertEqual(self.way_back(), before)

    def test_failed_data_update_is_reported_at_once(self):
        self.health_ok.unlink()
        r = self.run_install("update", "1.1.4", HEALTH_BODY='{"state":"failed"}')
        self.assertNotEqual(r.returncode, 0)
        self.assertLess(self.calls().count("readyz"), 3)
        self.assertIn("could not start", r.stderr)
        self.assertNotIn("may still be updating", r.stderr)
        self.assertIn("holyarrs.previous", r.stderr)
        self.assertIn("journalctl -u holyarrs", r.stderr)

    def test_one_stop_is_not_called_keeps_stopping(self):
        self.health_ok.write_text("12", encoding="utf-8")
        self.down_first.write_text("4", encoding="utf-8")
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertNotIn("keeps stopping", r.stderr)
        self.assertIn("1.1.4 is running", r.stdout)

    def test_stops_that_are_not_in_a_row_do_not_add_up(self):
        self.health_ok.write_text("40", encoding="utf-8")
        write_exe(
            self.stubs / "systemctl",
            '[ "$1" = show ] && exit 0\n'
            'if [ "$1" = is-active ]; then n=$(cat "$DOWN_FIRST" 2>/dev/null || echo 0); echo $((n + 1)) > "$DOWN_FIRST"; '
            '[ $((n % 3)) = 2 ] && exit 0; exit 3; fi\nexit 0\n',
        )
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)

    def test_program_started_again_three_times_is_reported_at_once(self):
        self.health_ok.unlink()
        r = self.run_install("update", "1.1.4", RESTART_STEP=1)
        self.assertNotEqual(r.returncode, 0)
        self.assertLess(self.calls().count("readyz"), 6)
        self.assertIn("keeps stopping", r.stderr)

    def test_ready_check_cannot_hang(self):
        text = INSTALL.read_text(encoding="utf-8")
        self.assertIn('curl -fsS --max-time 5 -o /dev/null "http://$HOST:$PORT/readyz"', text)
        self.assertIn('[ "$SECONDS" -lt 300 ] || break', text)

    def test_version_file_is_written_on_one_line(self):
        self.assertIn("""printf '%s\\n' "$wanted" > "$APP_DIR/version\"""", INSTALL.read_text(encoding="utf-8"))
        self.assertEqual(self.run_install("update", "1.1.4").returncode, 0)
        self.assertEqual((self.app / "version").read_bytes(), b"1.1.4\n")

    def test_unknown_installed_version_changes_nothing_unless_forced(self):
        self.assertEqual(self.run_install("update", "1.1.4").returncode, 0)
        before = self.way_back()
        self.newer_program()
        for bad in (None, "garbled\n"):
            with self.subTest(bad=bad):
                (self.app / "version").unlink(missing_ok=True)
                if bad:
                    (self.app / "version").write_text(bad, encoding="utf-8")
                self.log.write_text("", encoding="utf-8")
                r = self.run_install("update", "1.1.4")
                self.assertNotEqual(r.returncode, 0)
                self.assertIn("could not read which version is installed", r.stderr)
                self.assertIn("update 1.1.4 force", r.stderr)
                self.assertNotIn("systemctl", self.calls())
                self.assertEqual(self.way_back()[:2], before[:2])
        r = self.run_install("update", "1.1.4", "force")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertEqual((self.app / "version").read_text(encoding="utf-8"), "1.1.4\n")

    def test_port_from_a_drop_in_is_waited_on_and_printed(self):
        for unit_env, port in (("HOLYARRS_PORT=8080", "8080"), ("TZ=UTC HOLYARRS_ADDR=:9090 HOLYARRS_PORT=8080", "9090"),
                               ("HOLYARRS_PORT=abc", "6262"), ("", "6262")):
            with self.subTest(unit_env=unit_env):
                (self.app / "version").write_text("1.1.3\n", encoding="utf-8")
                self.log.write_text("", encoding="utf-8")
                r = self.run_install("update", "1.1.4", UNIT_ENV=unit_env)
                self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
                self.assertIn(f"probe http://127.0.0.1:{port}\n", self.calls())
                self.assertIn(f"Open http://192.168.1.60:{port}\n", r.stdout)

    def test_listen_address_from_a_drop_in_is_the_one_polled(self):
        exec_line = "{ path=/opt/holyarrs/holyarrs ; argv[]=/opt/holyarrs/holyarrs -config /x %s ; ignore_errors=no }"
        for extra, probe in (
            (dict(UNIT_ENV="HOLYARRS_ADDR=192.168.1.5:8080"), "192.168.1.5:8080"),
            (dict(UNIT_ENV="HOLYARRS_ADDR=0.0.0.0:8080"), "127.0.0.1:8080"),
            (dict(UNIT_EXEC=exec_line % "-addr 10.0.0.9:7070"), "10.0.0.9:7070"),
            (dict(UNIT_EXEC=exec_line % "-addr=:7070", UNIT_ENV="HOLYARRS_PORT=8080"), "127.0.0.1:7070"),
        ):
            with self.subTest(extra=extra):
                (self.app / "version").write_text("1.1.3\n", encoding="utf-8")
                self.log.write_text("", encoding="utf-8")
                r = self.run_install("update", "1.1.4", **extra)
                self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
                self.assertIn(f"probe http://{probe}\n", self.calls())

    def test_program_that_stays_down_is_reported_without_a_restart_count(self):
        self.health_ok.unlink()
        r = self.run_install("update", "1.1.4", ACTIVE_RC=3)
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(self.calls().count("readyz"), 10)
        self.assertIn("keeps stopping", r.stderr)
        self.assertNotIn("may still be updating", r.stderr)
        self.assertIn("journalctl -u holyarrs", r.stderr)

    def test_update_does_not_ask_for_a_new_owner_account(self):
        r = self.run_install("update", "1.1.4")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn("http://192.168.1.60:6262", r.stdout)
        self.assertNotIn("owner account", r.stdout)

    def test_older_version_refusal_names_the_host_command_too(self):
        r = self.run_install("update", "1.1.2")
        self.assertIn("FORCE=1 VERSION=1.1.2 bash holyarrs-lxc.sh update", r.stderr)
        self.assertIn("update 1.1.2 force", r.stderr)


class Lxc(Base):
    def setUp(self):
        super().setUp()
        self.pve = "8.2.4"
        self.templates = "debian-12-standard_12.7-1_amd64.tar.zst\ndebian-13-standard_13.1-2_amd64.tar.zst\n"
        self.uid = "999"
        self.net_ok = True
        self.fail_templates = ""

    def stub_host(self, mount_uid="999"):
        s = self.stubs
        for n in ("pveam-list-empty",):
            pass
        write_exe(s / "id", "echo 0\n")
        write_exe(s / "sleep", "exit 0\n")
        write_exe(s / "ip", "exit 0\n")
        write_exe(s / "qm", "exit 1\n")
        write_exe(s / "pvesh", "echo 150\n")
        write_exe(s / "pvesm", 'echo "Name Type Status Total Used Available %"\necho "local dir active 1 1 1 1%"\n')
        write_exe(s / "pveversion", f'echo "pve-manager/{self.pve}/abcdef (running kernel: 6.8)"\n')
        write_exe(
            s / "pveam",
            'case "$1" in\n  available) while read -r l; do echo "system $l"; done < "$TEMPLATES";;\n  *) exit 0;;\nesac\n',
        )
        write_exe(
            s / "pct",
            'echo "pct $*" >> "$CALLS"\n'
            'case "$1" in\n'
            '  status) exit 1;;\n'
            '  create)\n'
            '    for t in $FAIL_TEMPLATES; do case "$*" in *"$t"*) echo "unsupported debian version" >&2; exit 255;; esac; done\n'
            '    exit 0;;\n'
            '  exec) shift 2\n'
            '    case "$*" in\n'
            '      *"id -u holyarrs"*) echo "$UID_IN";;\n'
            '      *"id -g holyarrs"*) echo "$UID_IN";;\n'
            '      *"getent hosts"*) [ "$NET_OK" = 1 ] && exit 0 || exit 2;;\n'
            '      *hostname*) echo "192.168.1.50";;\n'
            '    esac; exit 0;;\n'
            '  *) exit 0;;\nesac\n',
        )

    def run_lxc(self, *args, **extra):
        (self.tmp / "templates").write_text(self.templates, encoding="utf-8")
        self.stub_host()
        env = self.env(TEMPLATES=posix(self.tmp / "templates"), UID_IN=self.uid, NET_OK="1" if self.net_ok else "0",
                       FAIL_TEMPLATES=self.fail_templates, **extra)
        return subprocess.run([BASH, posix(LXC), *args], env=env, capture_output=True, text=True)

    def created(self) -> list:
        return [c for c in self.calls().splitlines() if c.startswith("pct create")]

    def test_container_gets_nesting(self):
        r = self.run_lxc()
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn("--features nesting=1", self.created()[0])

    def test_proxmox_8_uses_debian_12(self):
        r = self.run_lxc()
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn("debian-12-standard", self.created()[0])

    def test_proxmox_9_uses_debian_13_and_falls_back_with_the_real_error(self):
        self.pve = "9.0.3"
        r = self.run_lxc()
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn("debian-13-standard", self.created()[0])
        self.fail_templates = "debian-13-standard"
        self.log.write_text("", encoding="utf-8")
        r = self.run_lxc()
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertEqual(len(self.created()), 2, self.created())
        self.assertIn("debian-12-standard", self.created()[1])
        self.assertIn("unsupported debian version", r.stdout + r.stderr)

    def test_failure_after_the_container_exists_says_it_was_left(self):
        self.net_ok = False
        r = self.run_lxc()
        self.assertNotEqual(r.returncode, 0)
        out = r.stdout + r.stderr
        self.assertIn("could not reach github.com", out)
        self.assertIn("pct destroy 150", out)

    def test_shared_folder_prints_the_exact_chown(self):
        shared = self.tmp / "srv-data"
        shared.mkdir()
        shared_u = unix_path(shared)
        r = self.run_lxc(MOUNT=shared_u)
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn(f"chown -R 100999:100999 {shared_u}", r.stdout)


    def test_privileged_shared_folder_prints_the_plain_chown(self):
        shared = self.tmp / "srv-data"
        shared.mkdir()
        shared_u = unix_path(shared)
        r = self.run_lxc(MOUNT=shared_u, PRIVILEGED=1)
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn(f"chown -R 999:999 {shared_u}", r.stdout)
        self.assertNotIn("100999", r.stdout)

    def test_privileged_folder_hint_without_numbers_has_no_offset(self):
        shared = self.tmp / "srv-data"
        shared.mkdir()
        self.uid = ""
        r = self.run_lxc(MOUNT=unix_path(shared), PRIVILEGED=1)
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn("chown -R <the user number>:<the group number>", r.stdout)
        self.assertNotIn("100000", r.stdout)

    def test_final_hint_has_no_path_of_the_running_script(self):
        r = self.run_lxc()
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        self.assertIn("bash holyarrs-lxc.sh update 150", r.stdout)
        self.assertNotIn(posix(LXC), r.stdout)

    def test_update_from_the_host_pushes_the_newer_installer_first(self):
        (self.tmp / "templates").write_text(self.templates, encoding="utf-8")
        self.stub_host()
        write_exe(self.stubs / "pct", 'echo "pct $*" >> "$CALLS"\ncase "$1" in status) echo "status: running";; esac\nexit 0\n')
        env = self.env()
        r = subprocess.run([BASH, posix(LXC), "update", "150"], env=env, capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        calls = self.calls().splitlines()
        push = [i for i, c in enumerate(calls) if c.startswith("pct push 150") and "/usr/local/bin/update-holyarrs" in c]
        run = [i for i, c in enumerate(calls) if "exec 150" in c and "update-holyarrs update" in c]
        self.assertTrue(push and run and push[0] < run[0], calls)

    def test_wrong_update_command_from_the_host_touches_nothing(self):
        (self.tmp / "templates").write_text(self.templates, encoding="utf-8")
        self.stub_host()
        write_exe(self.stubs / "pct", 'echo "pct $*" >> "$CALLS"\ncase "$1" in status) echo "status: running";; esac\nexit 0\n')
        r = subprocess.run([BASH, posix(LXC), "update", "150"], env=self.env(FORCE="1"), capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("FORCE=1 needs VERSION", r.stderr)
        self.assertNotIn("pct push", self.calls())


class Guide(unittest.TestCase):
    def test_guide_does_not_claim_a_real_debian_test(self):
        text = GUIDE.read_text(encoding="utf-8")
        self.assertNotIn("clean Debian container", text)
        self.assertIn("stand-in commands", text)

    def test_go_back_steps_are_exact(self):
        text = GUIDE.read_text(encoding="utf-8")
        for needle in ("systemctl stop holyarrs", "holyarrs.previous", "journalctl -u holyarrs", "/var/lib/holyarrs.newer"):
            self.assertIn(needle, text)

    def test_go_back_takes_the_old_version_from_the_saved_file(self):
        text = GUIDE.read_text(encoding="utf-8")
        self.assertIn("mv /opt/holyarrs/version.previous /opt/holyarrs/version", text)
        self.assertNotIn("echo <old version>", text)

    def test_symptoms_quote_only_text_the_scripts_print(self):
        text = GUIDE.read_text(encoding="utf-8")
        self.assertNotIn("404", text)
        self.assertIn("already installed", text)
        self.assertIn("could not read which version is installed here", text)
        self.assertIn("could not read which version is installed here", INSTALL.read_text(encoding="utf-8"))

    def test_same_version_run_does_not_claim_holyarrs_is_running(self):
        text = GUIDE.read_text(encoding="utf-8")
        self.assertNotIn("leaves HolyArrs running", text)
        self.assertIn("changes nothing", text)

    def test_port_change_goes_in_a_drop_in(self):
        text = GUIDE.read_text(encoding="utf-8")
        self.assertIn("systemctl edit holyarrs", text)
        self.assertIn("Environment=HOLYARRS_PORT=8080", text)

    def test_chown_step_changes_both_numbers(self):
        text = GUIDE.read_text(encoding="utf-8")
        self.assertNotIn("replace the second 100000", text)
        self.assertIn("100999:100999", text)
        self.assertIn("user and the group", text)


if __name__ == "__main__":
    unittest.main()
