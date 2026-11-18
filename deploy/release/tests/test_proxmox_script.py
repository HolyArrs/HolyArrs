"""The Proxmox scripts fetch what the Linux downloads really publish."""
import pathlib
import re
import shutil
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
PROXMOX = ROOT / "deploy" / "proxmox"
INSTALL = PROXMOX / "holyarrs-install.sh"
LXC = PROXMOX / "holyarrs-lxc.sh"
DOC = ROOT / "docs" / "run" / "proxmox.md"
PACKAGE = (ROOT / "deploy" / "release" / "linux_package.py").read_text(encoding="utf-8")


class ProxmoxScripts(unittest.TestCase):
    def test_shell_syntax(self):
        bash = shutil.which("bash")
        if not bash:
            self.skipTest("no bash")
        for script in (INSTALL, LXC):
            with self.subTest(script=script.name):
                result = subprocess.run([bash, "-n", str(script)], capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_file_names_match_the_download_packager(self):
        text = INSTALL.read_text(encoding="utf-8")
        # The same name the packager gives: holyarrs_<version>_linux_<arch>.tar.gz.
        self.assertIn('stem="holyarrs_${wanted}_linux_${arch}"', text)
        self.assertIn('stem = f"holyarrs_{version}_linux_{arch}"', PACKAGE)
        self.assertIn("SHA256SUMS", PACKAGE)
        self.assertIn('/download/$wanted/$stem.tar.gz', text)
        # The program sits in a folder named like the archive.
        self.assertIn('$work/$stem/holyarrs', text)

    def test_every_published_arch_is_installable(self):
        published = set(re.findall(r'"(\w+)": \("', PACKAGE))
        text = INSTALL.read_text(encoding="utf-8")
        installable = set(re.findall(r"^\s+\w+\) arch=(\w+) ;;", text, re.M))
        self.assertEqual(published, installable)

    def test_port_and_health_exist(self):
        text = INSTALL.read_text(encoding="utf-8")
        router = (ROOT / "app/backend/cmd/holyarrs/router.go").read_text(encoding="utf-8")
        expose = re.search(r"^EXPOSE\s+(\d+)", (ROOT / "deploy" / "Dockerfile").read_text(encoding="utf-8"), re.M).group(1)
        self.assertIn(f"PORT={expose}", text)
        self.assertIn('GET /healthz"', router)
        self.assertIn("/healthz", text)

    def test_doc_names_the_settings_the_script_reads(self):
        doc = DOC.read_text(encoding="utf-8")
        script = LXC.read_text(encoding="utf-8")
        for name in re.findall(r"^\| `([A-Z_]+)`", doc, re.M):
            with self.subTest(name=name):
                self.assertIn(name, script)


if __name__ == "__main__":
    unittest.main()
