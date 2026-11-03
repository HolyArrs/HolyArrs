"""The install guides name only settings and files that exist."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parent.parent
GUIDES = [ROOT / "docs" / "install.md", ROOT / "docs" / "nas.md"]


def go_source() -> str:
    return "\n".join(p.read_text(encoding="utf-8") for d in ("cmd", "internal") for p in (ROOT / d).rglob("*.go"))


class InstallGuides(unittest.TestCase):
    def test_env_names_exist(self):
        src = go_source()
        for g in GUIDES:
            for name in set(re.findall(r"HOLYARRS_[A-Z_]+", g.read_text(encoding="utf-8"))):
                with self.subTest(guide=g.name, name=name):
                    self.assertIn(name.rstrip("_"), src)

    def test_port_matches_image(self):
        expose = re.search(r"^EXPOSE\s+(\d+)", (ROOT / "Dockerfile").read_text(encoding="utf-8"), re.M).group(1)
        for g in GUIDES:
            self.assertIn(expose, g.read_text(encoding="utf-8"), g.name)

    def test_local_links_resolve(self):
        for g in GUIDES:
            for link in re.findall(r"\]\(([^)#:]+)(?:#[^)]*)?\)", g.read_text(encoding="utf-8")):
                with self.subTest(guide=g.name, link=link):
                    self.assertTrue((g.parent / link).exists())


if __name__ == "__main__":
    unittest.main()
