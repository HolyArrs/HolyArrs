"""The install guides cover the start-up refusals and warnings: Docker Desktop,
a forgotten /config mapping, Unraid paths and rootless runtimes."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
INSTALL = ROOT / "docs" / "start" / "install.mdx"
UNRAID = ROOT / "docs" / "run" / "platforms.md"
RELEASING = ROOT / "docs" / "dev" / "releasing.md"


def read(p: pathlib.Path) -> str:
    return p.read_text(encoding="utf-8")


def section(text: str, heading: str) -> str:
    m = re.search(r"^(#+)\s+" + re.escape(heading) + r"[^\n]*\n(.*?)(?=^#{1,%d}\s|\Z)", text, re.M | re.S)
    if not m:
        return ""
    return m.group(2)


class DockerDesktop(unittest.TestCase):
    def test_section_with_named_volume_compose(self):
        s = section(read(INSTALL), "Docker Desktop (Windows and Mac)")
        self.assertTrue(s, "no 'Docker Desktop (Windows and Mac)' section in install.md")
        self.assertIn("holyarrs-config:/config", s)
        self.assertRegex(s, r"(?m)^volumes:\n\s+holyarrs-config:", "top-level volumes: entry")
        self.assertIn("C:/Media", s)

    def test_moving_data_without_a_shell(self):
        s = section(read(INSTALL), "Docker Desktop (Windows and Mac)")
        self.assertIn("docker create --name hx -v holyarrs-config:/config", s)
        self.assertIn("docker cp", s)
        self.assertIn("docker rm hx", s)
        self.assertIn("docker cp holyarrs:/config/backups .", s)

    def test_wsl_folder_is_only_should_work(self):
        s = section(read(INSTALL), "Docker Desktop (Windows and Mac)")
        if "WSL" in s:
            self.assertIn("should work", s)

    def test_old_advice_is_gone(self):
        text = read(INSTALL)
        self.assertNotIn("| Config folder is on a network share | Move `/config` to a local disk.", text)
        row = re.search(r"^\| Config folder is on a[^\n]*$", text, re.M)
        self.assertIsNotNone(row, "troubleshooting row for the config folder refusal")
        self.assertIn("named volume", row.group(0))
        usage = section(text, "Steps")
        self.assertIn("named volume", usage)


class ForgotToMapConfig(unittest.TestCase):
    def test_section(self):
        s = section(read(INSTALL), "Forgot to map /config?")
        self.assertTrue(s, "no 'Forgot to map /config?' section in install.md")
        self.assertIn("docker cp", s)
        self.assertIn("docker volume ls", s)
        # a live copy can be torn: stop first
        stop = s.find("docker stop holyarrs")
        self.assertTrue(0 <= stop < s.find("docker cp holyarrs:/config"), "stop before docker cp")
        self.assertIn("Docker Desktop", s)


class Unraid(unittest.TestCase):
    def test_pool_path_never_cache(self):
        text = read(UNRAID)
        self.assertIn("/mnt/<pool>/appdata", text)
        self.assertNotIn("/mnt/cache", text)
        self.assertIn("Exclusive", text)

    def test_package_guide_and_template_say_pool(self):
        guide = read(RELEASING)
        self.assertIn("/mnt/<pool>/appdata", guide)
        self.assertNotIn("/mnt/cache/appdata/holyarrs", guide)
        tool = read(ROOT / "deploy" / "release" / "nas-template.py")
        self.assertIn("/mnt/<pool>/appdata", tool)


class Rootless(unittest.TestCase):
    def test_section(self):
        s = section(read(INSTALL), "Rootless Docker and Podman")
        self.assertTrue(s, "no 'Rootless Docker and Podman' section in install.md")
        for want in ("PUID=0", "PGID=0", "keep-id", "podman unshare chown -R 0:0", "rootlesskit chown"):
            with self.subTest(want=want):
                self.assertIn(want, s)
        self.assertRegex(s, r"(?i)only in rootless", "PUID=0 must be marked safe only when rootless")


class Overrides(unittest.TestCase):
    def test_override_names_documented(self):
        text = read(INSTALL)
        for name in ("HOLYARRS_ALLOW_DESKTOP_FOLDER", "HOLYARRS_CONFIG_IN_MEMORY"):
            with self.subTest(name=name):
                self.assertIn(name, text)


if __name__ == "__main__":
    unittest.main()
