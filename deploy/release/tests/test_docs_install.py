"""The install guides name only settings and files that exist."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
GUIDES = [ROOT / "docs" / "install.md", ROOT / "docs" / "nas.md"]


def go_source() -> str:
    return "\n".join(p.read_text(encoding="utf-8") for d in ("app/backend/cmd", "app/backend/internal") for p in (ROOT / d).rglob("*.go"))


class InstallGuides(unittest.TestCase):
    def test_env_names_exist(self):
        src = go_source()
        for g in GUIDES:
            for name in set(re.findall(r"HOLYARRS_[A-Z_]+", g.read_text(encoding="utf-8"))):
                with self.subTest(guide=g.name, name=name):
                    self.assertIn(name.rstrip("_"), src)

    def test_port_matches_image(self):
        expose = re.search(r"^EXPOSE\s+(\d+)", (ROOT / "deploy" / "Dockerfile").read_text(encoding="utf-8"), re.M).group(1)
        for g in GUIDES:
            self.assertIn(expose, g.read_text(encoding="utf-8"), g.name)

    def test_local_links_resolve(self):
        for g in GUIDES:
            for link in re.findall(r"\]\(([^)#:]+)(?:#[^)]*)?\)", g.read_text(encoding="utf-8")):
                with self.subTest(guide=g.name, link=link):
                    self.assertTrue((g.parent / link).exists())

    def test_anchors_resolve(self):
        def slugs(text):
            return {re.sub(r"[^a-z0-9 _-]", "", h.strip().lower()).replace(" ", "-")
                    for h in re.findall(r"^#+\s+(.+)$", text, re.M)} | set(re.findall(r'<a id="([^"]+)"', text))
        for g in [*GUIDES, *sorted((ROOT / "docs").glob("*.md")), ROOT / "README.md"]:
            text = g.read_text(encoding="utf-8")
            for target, anchor in re.findall(r"\]\(([^)#:]*)#([^)]+)\)", text):
                page = (g.parent / target) if target else g
                with self.subTest(guide=g.name, anchor=anchor):
                    self.assertIn(anchor, slugs(page.read_text(encoding="utf-8")))

    def test_image_switches_user(self):
        # The docs describe the start-as-root switch; the image and the code must have it.
        src = go_source()
        for name in ("PUID", "PGID", "UMASK", "FILE__"):
            with self.subTest(name=name):
                self.assertIn(name, src)
        self.assertNotRegex((ROOT / "deploy" / "Dockerfile").read_text(encoding="utf-8"), r"(?m)^USER\s")

    def test_linuxserver_style_setup(self):
        text = (ROOT / "docs" / "install.md").read_text(encoding="utf-8")
        for old in ("There are no\n`PUID`", "no `PUID`", 'user: "1000:1000"\n    ports', "PAGID"):
            self.assertFalse(old in text, old)
        compose = re.search(r"```yaml\n(services:\n  holyarrs:\n    image: .*?)```", text, re.S).group(1)
        cli = re.search(r"```sh\n(docker run -d .*?)```", text, re.S).group(1)
        for block in (compose, cli):
            for want in ("ghcr.io/coderluii/holyarrs:beta", "holyarrs", "PUID=1000", "PGID=1000", "TZ=Etc/UTC",
                         ":/config", "6262:6262", "unless-stopped"):
                with self.subTest(want=want):
                    self.assertIn(want, block)
            for folder in ("/movies", "/tv", "/music", "/downloads"):
                with self.subTest(folder=folder):
                    self.assertRegex(block, re.escape(":" + folder) + r"`? *`?#optional")
            self.assertNotIn(":/data", block)
        headings = re.findall(r"^## (.+)$", text, re.M)
        order = ["Parameters", "Environment variables from files (FILE__)", "Umask",
                 "User / Group identifiers", "Read-only operation", "Non-root operation", "Media folders"]
        self.assertEqual([h for h in headings if h in order], order)
        self.assertLess(text.index("### Docker Compose"), text.index("## Parameters"))
        for row in ("PUID", "PGID", "UMASK", "TZ", "/config", "/movies", "/tv", "/music", "/downloads"):
            with self.subTest(row=row):
                self.assertRegex(text, r"\n\| `-[evp] " + re.escape(row))
        for phrase in ("022", "002", "id youruser", "--read-only", "tmpfs", "--user", "hardlink"):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, text)

    def test_nas_guides_match(self):
        for name in ("nas.md", "nas-packages.md"):
            text = (ROOT / "docs" / name).read_text(encoding="utf-8")
            with self.subTest(guide=name):
                self.assertIn("PUID", text)
                self.assertNotIn("runs as `1000:1000`", text)
                self.assertNotIn("No `PUID`", text)
        packages = (ROOT / "docs" / "nas-packages.md").read_text(encoding="utf-8")
        self.assertNotIn("--data ", packages)
        self.assertIn("--movies", packages)


if __name__ == "__main__":
    unittest.main()
