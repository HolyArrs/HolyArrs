import hashlib
import importlib.util
import io
import json
import os
import shutil
import struct
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("synology_build", Path(__file__).with_name("build.py"))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


def fake_program(arch):
    """An ELF header for arch, enough for the CPU check."""
    want = build.ARCHES[arch]
    head = b"\x7fELF" + bytes([1 if want["bits"] == 32 else 2, 1, 1]) + b"\0" * 9
    return head + struct.pack("<HH", 2, want["machine"]) + b"\0" * 64


def repack(path, change):
    """Rewrite a .spk with change(files) applied to its {name: bytes}."""
    with tarfile.open(path, mode="r:") as tar:
        members = [(m, tar.extractfile(m).read() if m.isfile() else None) for m in tar.getmembers()]
    files = {m.name: data for m, data in members if data is not None}
    change(files)
    with tarfile.open(path, mode="w", format=tarfile.GNU_FORMAT) as tar:
        for m, data in members:
            if data is None:
                tar.addfile(m)
            elif m.name in files:
                m.size = len(files[m.name])
                tar.addfile(m, io.BytesIO(files[m.name]))


class Package(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)

    def make(self, arch="x86_64"):
        return build.build_spk(fake_program(arch), b"licence text\n", "1.2.3", arch, self.tmp.name, mtime=0)

    def test_each_arch_builds_and_passes(self):
        for arch in build.ARCHES:
            path = self.make(arch)
            self.assertEqual(path.name, "holyarrs-1.2.3-%s.spk" % arch)
            self.assertEqual(build.validate_spk(path, "1.2.3", arch), [])

    def test_info_and_layout(self):
        path = self.make()
        with tarfile.open(path, mode="r:") as tar:
            self.assertEqual(tar.getnames()[0], "INFO")
            info = build.parse_info(tar.extractfile("INFO").read().decode())
            tgz = tar.extractfile("package.tgz").read()
            self.assertEqual(tar.getmember("scripts/start-stop-status").mode, 0o755)
            self.assertEqual(json.loads(tar.extractfile("conf/privilege").read()), {"defaults": {"run-as": "package"}})
        self.assertEqual(info["version"], "1.2.3-0001")
        self.assertEqual(info["checksum"], hashlib.md5(tgz).hexdigest())
        self.assertIn("r1000", info["arch"].split())
        with tarfile.open(fileobj=io.BytesIO(tgz), mode="r:gz") as tar:
            self.assertEqual(tar.getmember("bin/holyarrs").mode, 0o755)

    def test_the_same_input_gives_the_same_file(self):
        first = self.make().read_bytes()
        self.assertEqual(first, self.make().read_bytes())

    def test_a_program_for_another_cpu_is_refused(self):
        with self.assertRaises(build.BuildError):
            build.build_spk(fake_program("aarch64"), b"x", "1.2.3", "x86_64", self.tmp.name)
        with self.assertRaises(build.BuildError):
            build.build_spk(b"MZ not a linux program", b"x", "1.2.3", "x86_64", self.tmp.name)

    def test_a_bad_version_is_refused(self):
        for bad in ("", "1.2", "v1.2.3", "1.2.3-rc1", "1.2.3; rm -rf /"):
            with self.assertRaises(build.BuildError):
                build.build_spk(fake_program("x86_64"), b"x", bad, "x86_64", self.tmp.name)

    def test_the_check_finds_a_wrong_checksum(self):
        path = self.make()
        repack(path, lambda f: f.update({"package.tgz": f["package.tgz"] + b"\0"}))
        self.assertIn("INFO checksum does not match package.tgz", build.validate_spk(path, "1.2.3", "x86_64"))

    def test_the_check_finds_a_missing_script(self):
        path = self.make()
        repack(path, lambda f: f.pop("scripts/postinst"))
        self.assertEqual(build.validate_spk(path), ["missing scripts/postinst"])

    def test_the_check_finds_bad_json_and_root(self):
        path = self.make()
        repack(path, lambda f: f.update({"conf/resource": b"{not json", "conf/privilege": b'{"defaults":{"run-as":"root"}}'}))
        problems = build.validate_spk(path)
        self.assertTrue(any(p.startswith("conf/resource is not valid JSON") for p in problems), problems)
        self.assertIn("conf/privilege does not run as the package user", problems)

    def test_the_check_finds_windows_line_ends_and_other_versions(self):
        path = self.make()
        repack(path, lambda f: f.update({"scripts/preinst": b"#!/bin/sh\r\nexit 0\r\n"}))
        problems = build.validate_spk(path, "9.9.9", "armv7")
        self.assertIn("scripts/preinst has Windows line ends", problems)
        self.assertIn("INFO version '1.2.3-0001' is not 9.9.9", problems)
        self.assertIn("INFO arch is not the armv7 list", problems)

    def test_not_a_tar(self):
        path = Path(self.tmp.name) / "x.spk"
        path.write_bytes(b"nope")
        self.assertEqual(len(build.validate_spk(path)), 1)

    def test_checksum_file(self):
        self.make("x86_64")
        self.make("armv7")
        sums = build.write_sums(self.tmp.name, "1.2.3")
        self.assertEqual(sums.name, "holyarrs-1.2.3-synology-SHA256SUMS")
        lines = sums.read_text().splitlines()
        self.assertEqual([line.split("  ")[1] for line in lines], ["holyarrs-1.2.3-armv7.spk", "holyarrs-1.2.3-x86_64.spk"])
        spk = Path(self.tmp.name) / "holyarrs-1.2.3-armv7.spk"
        self.assertEqual(lines[0].split("  ")[0], hashlib.sha256(spk.read_bytes()).hexdigest())


@unittest.skipUnless(shutil.which("sh"), "needs sh")
class Scripts(unittest.TestCase):
    def test_the_upgrade_wizard_offers_the_saved_folder(self):
        text = build.source_bytes("WIZARD_UIFILES/upgrade_uifile.sh").decode()
        with tempfile.TemporaryDirectory() as tmp:
            saved = Path(tmp) / "media-share"
            saved.write_bytes(b"My Media\n")
            script = Path(tmp) / "wizard.sh"
            script.write_bytes(text.replace("/var/packages/holyarrs/var/media-share", saved.as_posix()).encode())
            for name, want in ((saved, "My Media"), (None, "data")):
                if name is None:
                    saved.unlink()
                answer = Path(tmp) / "wizard.json"
                subprocess.run(["sh", script.as_posix()], check=True,
                               env={"PATH": os.environ["PATH"], "SYNOPKG_TEMP_LOGFILE": answer.as_posix()})
                out = answer.read_text()
                self.assertEqual(json.loads(out)[0]["items"][0]["subitems"][0]["defaultValue"], want)

    def test_every_script_parses(self):
        for name in sorted(build.RUNNABLE):
            with tempfile.TemporaryDirectory() as tmp:
                script = Path(tmp) / "s.sh"
                script.write_bytes(build.source_bytes(name))
                done = subprocess.run(["sh", "-n", script.as_posix()], capture_output=True, text=True)
                self.assertEqual(done.returncode, 0, "%s: %s" % (name, done.stderr))


if __name__ == "__main__":
    unittest.main()
