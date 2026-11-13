"""The plain Linux downloads: names, contents, checksums and the Docker image's build flags."""
import hashlib
import importlib.util
from pathlib import Path
import re
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location("linux_package", ROOT / "deploy" / "release" / "linux_package.py")
linux = importlib.util.module_from_spec(spec)
spec.loader.exec_module(linux)


def run_main(args):
    with patch.object(sys, "argv", ["linux_package.py", *args]):
        linux.main()


class LinuxPackageTests(unittest.TestCase):
    def source(self, root):
        root.mkdir(parents=True, exist_ok=True)
        (root / "LICENSE").write_bytes(b"licence\n")
        (root / "VERSION").write_text("1.2.3\n")
        return root

    def fake_build(self, version, arch, out):
        out.write_bytes(b"ELF " + arch.encode() + b" holyarrs " + version.encode())

    def test_makes_one_archive_per_arch_with_sums(self):
        with tempfile.TemporaryDirectory() as t:
            src, out = self.source(Path(t) / "src"), Path(t) / "out"
            with patch.object(linux, "build_web"), \
                    patch.object(linux, "go_build", lambda s, v, a, o: self.fake_build(v, a, o)):
                run_main(["--version", "1.2.3", "--output", str(out), "--source", str(src)])
            names = sorted(p.name for p in out.iterdir())
            self.assertEqual(names, ["SHA256SUMS"] + [f"holyarrs_1.2.3_linux_{a}.tar.gz" for a in ("amd64", "arm64", "armv7")])
            for line in (out / "SHA256SUMS").read_text().splitlines():
                digest, name = line.split("  ")
                self.assertEqual(digest, hashlib.sha256((out / name).read_bytes()).hexdigest())
            with tarfile.open(out / "holyarrs_1.2.3_linux_arm64.tar.gz") as tar:
                members = {m.name: m for m in tar}
                folder = "holyarrs_1.2.3_linux_arm64/"
                self.assertEqual(sorted(members), [folder + n for n in ("LICENSE", "README.txt", "holyarrs")])
                self.assertEqual(members[folder + "holyarrs"].mode, 0o755)
                self.assertEqual(members[folder + "holyarrs"].uid, 0)
                readme = tar.extractfile(folder + "README.txt").read().decode()
                self.assertIn("./holyarrs -config /var/lib/holyarrs", readme)
                self.assertIn("6262", readme)

    def test_same_inputs_make_the_same_bytes_and_notices_ride_along(self):
        sums = []
        for _ in range(2):
            with tempfile.TemporaryDirectory() as t:
                src, out = self.source(Path(t) / "src"), Path(t) / "out"
                (src / "THIRD_PARTY_NOTICES.md").write_bytes(b"notices\n")
                with patch.object(linux, "build_web"), \
                        patch.object(linux, "go_build", lambda s, v, a, o: self.fake_build(v, a, o)):
                    run_main(["--version", "1.2.3", "--output", str(out), "--source", str(src), "--arches", "amd64"])
                sums.append((out / "SHA256SUMS").read_text().split()[0])
                with tarfile.open(out / "holyarrs_1.2.3_linux_amd64.tar.gz") as tar:
                    self.assertIn("holyarrs_1.2.3_linux_amd64/THIRD_PARTY_NOTICES.md", tar.getnames())
        self.assertEqual(sums[0], sums[1])

    def test_bad_version_or_arch_is_refused(self):
        for args in (["--version", "v1.2.3"], ["--version", "1.2"], ["--version", "1.2.3", "--arches", "mips"],
                     ["--version", "1.2.3", "--arches", "amd64,amd64"]):
            with tempfile.TemporaryDirectory() as t, self.assertRaises(SystemExit) as caught:
                run_main([*args, "--output", str(Path(t) / "out"), "--source", t])
            self.assertEqual(caught.exception.code, 1)

    def test_a_program_without_the_version_is_refused(self):
        with tempfile.TemporaryDirectory() as t:
            src = self.source(Path(t) / "src")
            with patch.object(linux, "build_web"), \
                    patch.object(linux, "go_build", lambda s, v, a, o: o.write_bytes(b"ELF")), \
                    self.assertRaises(SystemExit):
                run_main(["--version", "1.2.3", "--output", str(Path(t) / "out"), "--source", str(src)])

    def test_go_targets(self):
        self.assertEqual(linux.ARCHES, {"amd64": ("amd64", ""), "arm64": ("arm64", ""), "armv7": ("arm", "7")})

    def test_build_flags_match_the_image(self):
        dockerfile = (ROOT / "deploy" / "Dockerfile").read_text(encoding="utf-8")
        listed = re.search(r"for p in (.*?); do", dockerfile.replace(chr(92) + chr(10), " "), re.S).group(1).split()
        self.assertEqual(linux.NO_INLINE, listed)
        self.assertIn('ldflags="-s -w"', dockerfile)
        self.assertIn("-X main.version=", dockerfile)

    def test_workflow_runs_after_the_release_and_uses_no_secrets(self):
        text = (ROOT / ".github" / "workflows" / "linux.yml").read_text(encoding="utf-8")
        self.assertIn("workflows: ['HolyArrs - Release']", text)
        self.assertNotIn("secrets.", text)
        self.assertIn("gh release upload", text)
        self.assertIn("SHA256SUMS", text)
        dockerfile = (ROOT / "deploy" / "Dockerfile").read_text(encoding="utf-8")
        image = re.search(r"NODE_IMAGE: (\S+)", text).group(1)
        self.assertIn("FROM --platform=$BUILDPLATFORM " + image, dockerfile)
        for use in re.findall(r"uses: (\S+)", text):
            self.assertRegex(use, r"@[0-9a-f]{40}$")


if __name__ == "__main__":
    unittest.main()
