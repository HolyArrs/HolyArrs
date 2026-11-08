"""Release notes generator: one yml renders every place, hidden sections stay
hidden, credits only show known logins, and the Discord DM cuts safely."""
import copy
import importlib.util
import pathlib
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
DATA = HERE / "testdata"
spec = importlib.util.spec_from_file_location("notes", HERE.parent / "notes.py")
notes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(notes)

FILES = ["changelog.md", "github.md", "update-notes.json", "reddit.txt",
         "discord-dm.txt", "discord-announce.md", "roadmap-card.json"]


def load():
    return notes.load_yaml((DATA / "notes-1.0.0.yml").read_text(encoding="utf-8-sig"))


class Golden(unittest.TestCase):
    def test_renders_all_seven_files_like_golden(self):
        with tempfile.TemporaryDirectory() as out:
            rc = notes.main(["1.0.0", "--file", str(DATA / "notes-1.0.0.yml"),
                             "--map", str(DATA / "missing.json"), "--date", "2026-11-08", "--out", out])
            self.assertEqual(rc, 0)
            for name in FILES:
                got = (pathlib.Path(out) / name).read_bytes()
                want = (DATA / "golden-1.0.0" / name).read_bytes().replace(b"\r\n", b"\n")
                self.assertEqual(got, want, name)
                self.assertNotIn(b"\r\n", got, name)

    def test_known_issues_hidden(self):
        self.assertNotIn("Known issues", notes.render_github(load(), {}))

    def test_check_passes_on_1_0_0(self):
        errors, _ = notes.check(load(), {})
        self.assertEqual(errors, [])


class Hidden(unittest.TestCase):
    def test_empty_security_and_before_show_no_heading(self):
        n = load()
        n["security"] = []
        n["before_you_update"] = {"breaking": [], "manual_steps": [], "backup": "x", "rollback": "y"}
        gh = notes.render_github(n, {})
        self.assertNotIn("## Security", gh)
        self.assertNotIn("Before you update", gh)
        self.assertNotIn("heads up", notes.render_reddit(n))
        self.assertNotIn("Before you update", notes.render_discord(n))

    def test_empty_group_dropped(self):
        n = load()
        n["groups"]["Empty"] = {"added": [], "fixed": None}
        self.assertNotIn("## Empty", notes.render_github(n, {}))


class Credits(unittest.TestCase):
    def test_map_entry_adds_login(self):
        n = load()
        gh = notes.render_github(n, {"u/clayuk62": "clay-gh"})
        self.assertIn("(@clay-gh)", gh)
        self.assertNotIn("u/The_Mdk](https://www.reddit.com/r/SideProject/comments/1xjjuwp/comment/19rwcbh51glqb/) (@", gh)

    def test_bad_login_is_error(self):
        errors, _ = notes.check(load(), {"u/clayuk62": "not a login!"})
        self.assertTrue(any("github login" in e for e in errors))


class Discord(unittest.TestCase):
    def test_long_dm_drops_theme_first_keeps_link(self):
        n = load()
        n["theme"] = "word " * 150
        dm = notes.render_discord(n)
        self.assertLessEqual(len(dm), 1000)
        self.assertNotIn("word word", dm)
        self.assertIn("Before you update:", dm)
        self.assertTrue(dm.rstrip().endswith("/releases/tag/1.0.0"))

    def test_then_drops_before_line(self):
        n = load()
        n["theme"] = "word " * 150
        n["before_you_update"]["breaking"] = ["b " * 450]
        dm = notes.render_discord(n)
        self.assertNotIn("Before you update:", dm)
        self.assertIn("What changed: https://github.com/HolyArrs/holyarrs/releases/tag/1.0.0", dm)
        self.assertTrue(notes.render_discord(n, bold=True).startswith("**HolyArrs 1.0.0 is ready**"))


class Check(unittest.TestCase):
    def errors(self, n):
        return notes.check(n, {})[0]

    def test_v_prefix_and_highlight_count(self):
        n = load()
        n["version"] = "v1.0.0"
        n["highlights"] = n["highlights"][:2]
        e = self.errors(n)
        self.assertTrue(any("X.Y.Z" in x for x in e))
        self.assertTrue(any("exactly 3" in x for x in e))

    def test_line_without_ref_and_launch_talk(self):
        n = load()
        n["groups"]["Setup"]["added"].append("Coming soon to your server.")
        e = self.errors(n)
        self.assertTrue(any("no (#n)" in x for x in e))
        self.assertTrue(any("launch talk" in x for x in e))

    def test_banned_words_and_em_dash_warn(self):
        n = load()
        n["groups"]["Setup"]["added"].append("A robust — seamless thing. (#75)")
        w = notes.check(n, {})[1]
        self.assertTrue(any("robust" in x for x in w))
        self.assertTrue(any("em dash" in x for x in w))

    def test_before_needs_backup_and_rollback(self):
        n = load()
        n["before_you_update"]["rollback"] = ""
        self.assertTrue(any("rollback" in x for x in self.errors(n)))

    def test_highlight_issue_must_be_in_group(self):
        n = copy.deepcopy(load())
        n["highlights"][0]["issue"] = 9999
        self.assertTrue(any("#9999" in x for x in self.errors(n)))


class Yaml(unittest.TestCase):
    def test_subset_shapes(self):
        n = load()
        self.assertEqual(n["version"], "1.0.0")
        self.assertEqual(n["known_issues"], [])
        self.assertEqual(n["highlights"][0]["issue"], 78)
        self.assertEqual(n["credits"][0]["issues"][0], 91)
        self.assertEqual(list(n["groups"])[0], "Setup")


if __name__ == "__main__":
    unittest.main()
