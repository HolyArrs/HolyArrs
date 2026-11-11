"""Docs and messages match the bare image: image tags, restart policy,
Podman, the health table and the words on the buttons."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
DOCS = ROOT / "docs"
INSTALL = DOCS / "start" / "install.mdx"
PLATFORMS = DOCS / "run" / "platforms.md"


def pages():
    # Every user and developer page, old .md and new .mdx alike.
    return sorted(p for p in DOCS.rglob("*.md*") if p.name != "third-party-notices.md")


def read(p: pathlib.Path) -> str:
    return p.read_text(encoding="utf-8")


def section(text: str, heading: str) -> str:
    m = re.search(r"^(#+)\s+" + re.escape(heading) + r"[^\n]*\n(.*?)(?=^#{1,%d}\s|\Z)" % 6, text, re.M | re.S)
    return m.group(2) if m else ""


class ImageTags(unittest.TestCase):
    def test_docs_pull_the_latest_tag(self):
        # 1.0.0 is promoted to :latest before launch; every example pulls the
        # same tag as the main compose file. Placeholders like <tag> are fine.
        main = re.search(r"(?<![\w./-])holyarrs/holyarrs:([\w.-]+)", read(INSTALL)).group(1)
        self.assertEqual(main, "latest")
        for page in [*pages(), ROOT / "README.md"]:
            for tag in re.findall(r"(?<![\w./-])holyarrs/holyarrs:([\w.<>-]+)", read(page)):
                with self.subTest(page=page.name, tag=tag):
                    self.assertTrue(tag == main or tag.startswith("<"), f"{page.name} pulls :{tag}")


class RestartPolicy(unittest.TestCase):
    def test_nas_guide_names_auto_restart(self):
        text = read(PLATFORMS)
        self.assertIn("unless-stopped", text)
        self.assertRegex(text, r"(?i)auto-?restart")


class Podman(unittest.TestCase):
    def test_quadlet_is_complete(self):
        s = section(read(INSTALL), "Rootless Docker and Podman")
        quadlet = re.search(r"```ini\n(.*?)```", s, re.S).group(1)
        self.assertRegex(quadlet, r"(?m)^Volume=.*:/config:Z$")
        # No shell in the image: a plain-text health command runs through /bin/sh.
        self.assertIn('HealthCmd=["/holyarrs","-healthcheck"]', quadlet)
        self.assertRegex(quadlet, r"(?m)^\[Service\]\nRestart=always")
        run = s.split("podman run -d", 1)[1]
        self.assertIn(":/config:Z", run)
        self.assertIn("""--health-cmd '["/holyarrs","-healthcheck"]'""", run)
        # podman-restart.service (Podman 4.x) only starts "always" containers.
        self.assertIn("--restart always", run)
        self.assertNotIn("unless-stopped", s)
        # Rootless: the user units only start at boot with lingering on.
        self.assertIn("loginctl enable-linger", s)
        self.assertIn("systemctl --user enable podman-restart.service", s)

    def test_selinux_media_uses_shared_label(self):
        s = section(read(INSTALL), "Rootless Docker and Podman")
        note = s[s.index("`:Z`"):].split("\n- ", 1)[0]
        # Media is shared with the Downloader and is often a network share.
        self.assertNotIn("Add it to media folders too", note)
        self.assertIn("`:z`", note)
        self.assertRegex(note, r"(?i)network")


class HealthDocs(unittest.TestCase):
    def test_health_table_has_every_check(self):
        s = section(read(DOCS / "run" / "health.md"), "What each check means")
        for row in ("Config folder", "Waiting downloads", "Key kit", "Trailer reader"):
            with self.subTest(row=row):
                self.assertRegex(s, r"(?m)^\| " + re.escape(row))

    def test_key_kit_and_new_machine_pointers(self):
        health = read(DOCS / "run" / "health.md")
        self.assertIn("](backups.md", health)
        install = section(read(INSTALL), "Keep the master key")
        self.assertIn("Download key kit", install)
        self.assertIn("holyarrs.key", install)
        # A named volume has no host folder: point at the docker cp steps.
        self.assertIn("docker cp", install)
        self.assertIn("#docker-desktop-windows-and-mac", install)
        backups = read(DOCS / "run" / "backups.md")
        self.assertIn("key kit", backups.lower())

    def test_trailer_row_warns_only_at_the_limit(self):
        s = section(read(DOCS / "run" / "health.md"), "What each check means")
        row = re.search(r"(?m)^\| Trailer reader[^\n]*", s).group(0)
        warn = row.split("|")[3]
        # health.go: Warning only once TrailerFailLimit lookups failed; a refusal
        # below that only adds a message to the row.
        self.assertNotIn("turned", warn)
        self.assertIn("5 different videos failed in a row", warn)

    def test_paused_row_names_the_share(self):
        text = read(DOCS / "movies" / "downloads.md")
        row = re.search(r"(?m)^\| Paused \|(.*)$", text).group(1)
        self.assertIn("share", row)
        self.assertIn("Check again", row)

    def test_schedules_both_switches(self):
        text = read(DOCS / "run" / "schedules.md")
        switches = re.search(r"(?m)^Two schedules can be turned off:.*$", text).group(0)
        self.assertIn("Refresh movie details", switches)
        self.assertIn("Rescan the Library", switches)
        self.assertIn("`rescan_library`", text)

    def test_moved_path_maps(self):
        s = section(read(DOCS / "migrate" / "sonarr-radarr.md"), "What comes over")
        self.assertRegex(s, r"(?i)remote path mappings")
        self.assertIn("not visible here yet", s)

    def test_unraid_exclusive_switch(self):
        self.assertIn("Permit exclusive shares", read(PLATFORMS))
        self.assertIn("Permit exclusive shares", read(ROOT / "app" / "backend" / "internal" / "core" / "files" / "configfolder.go"))


class Wording(unittest.TestCase):
    def test_button_words_match(self):
        # The Downloads page button says "Check again"; no message may say "Recheck".
        for p in (ROOT / "app" / "backend" / "internal" / "downloads" / "manager.go", ROOT / "app" / "backend" / "internal" / "health" / "health.go"):
            with self.subTest(file=p.name):
                self.assertNotIn("use Recheck", read(p))
                self.assertIn("Check again", read(p))

    def test_no_shell_steps_on_first_run(self):
        # The sign-in words live in the translations since the app speaks several languages.
        page = read(ROOT / "app" / "frontend" / "src" / "routes" / "login" / "+page.svelte")
        words = read(ROOT / "app" / "frontend" / "src" / "lib" / "core" / "i18n" / "messages" / "en" / "accounts.json")
        self.assertNotIn("doctor shows how", page + words)
        self.assertIn("accounts.login.unmapped", page)
        self.assertIn("Forgot to map /config?", words)


if __name__ == "__main__":
    unittest.main()
