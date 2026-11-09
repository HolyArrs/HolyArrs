"""Release notes generator: one TOML file renders every place, hidden sections
stay hidden, credits only show known logins, and the Discord DM cuts safely."""
import copy
import importlib.util
import pathlib
import re
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
DATA = HERE / "testdata"
spec = importlib.util.spec_from_file_location("notes", HERE.parent / "notes.py")
notes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(notes)


def load():
    return notes.load_notes(DATA / "notes-1.0.0.toml")


def errors(n, cmap=None):
    return notes.check(n, cmap or {})[0]


def warnings(n):
    return notes.check(n, {})[1]


def headings(gh):
    return re.findall(r"^## (.+)$", gh, re.M)


class Golden(unittest.TestCase):
    def test_renders_all_seven_files_like_golden(self):
        with tempfile.TemporaryDirectory() as out:
            rc = notes.main(["1.0.0", "--file", str(DATA / "notes-1.0.0.toml"),
                             "--map", str(DATA / "missing.json"), "--date", "2026-11-08", "--out", out])
            self.assertEqual(rc, 0)
            for name in notes.FILES:
                got = (pathlib.Path(out) / name).read_bytes()
                want = (DATA / "golden-1.0.0" / name).read_bytes().replace(b"\r\n", b"\n")
                self.assertEqual(got, want, name)
                self.assertNotIn(b"\r\n", got, name)

    def test_check_clean_on_1_0_0(self):
        self.assertEqual(notes.check(load(), {}), ([], []))

    def test_section_order(self):
        self.assertEqual(headings(notes.render_github(load(), {})), [
            "Highlights", "Requests", "Library", "Downloads", "Indexers", "Accounts", "Health",
            "Setup", "Also new", "Security", "Install", "Thanks"])
        gh = notes.render_github(load(), {})
        self.assertTrue(gh.rstrip().endswith("_95 issues done, 4 still building, 48 people credited._"))
        self.assertTrue(gh.startswith(load()["theme"]))
        self.assertNotIn("## Assets", gh)


class Shape(unittest.TestCase):
    def test_no_sub_headings_and_small_places_in_also_new(self):
        gh = notes.render_github(load(), {})
        self.assertNotIn("### ", gh)
        also = gh.split("## Also new\n")[1].split("\n\n")[0]
        n = load()
        for place in ("Migration", "Shows", "Movies", "Music", "Alerts", "Updates"):
            self.assertNotIn("## " + place, gh)
            for line in n["groups"][place]:
                self.assertIn(notes.md_links(line), also)

    def test_highlight_issues_only_in_highlights(self):
        gh = notes.render_github(load(), {})
        top, rest = gh.split("## Highlights\n")[1].split("\n\n", 1)
        for i in (78, 6, 46, 47):
            self.assertIn("[#%d]" % i, top)
            self.assertNotIn("[#%d]" % i, rest)

    def test_changelog_prefixes_go_to_their_group(self):
        cl = notes.render_changelog(load(), "2026-11-08")
        self.assertIn("### Changed\n- Settings cut down", cl)
        self.assertNotIn("Changed: ", cl)
        self.assertTrue(cl.split("### Added\n")[1].startswith("- Move over from your old apps"))

    def test_image_line(self):
        body = notes.render_github(load(), {})
        self.assertIn("## Install\n- Step-by-step install guide. ([#73]", body)
        self.assertIn("([#84](https://github.com/HolyArrs/HolyArrs/issues/84))\n\nDocker Hub:\n\n"
                      "`holyarrs/holyarrs:1.0.0`\n\n`holyarrs/holyarrs:latest`\n\n## Thanks", body)
        self.assertNotIn("Get the image", body)
        self.assertNotIn("ghcr", body.lower())
        self.assertNotIn("## Docs", body)

    def test_install_without_place_lines_is_just_the_image(self):
        n = load()
        del n["groups"]["Install"]
        self.assertIn("## Install\nDocker Hub:\n\n`holyarrs/holyarrs:1.0.0`", notes.render_github(n, {}))

    def test_docs_link_only_when_set(self):
        n = load()
        n["docs_url"] = "https://example.org/docs"
        self.assertIn("## Docs\n[Read the docs](https://example.org/docs)", notes.render_github(n, {}))

    def test_old_map_format_and_docs_kind(self):
        n = load()
        n["groups"]["Setup"] = {"added": ["A (#901)", "B (#902)"], "fixed": ["C (#903)"],
                                "docs": ["D (#904)"]}
        gh = notes.render_github(n, {})
        self.assertIn("## Setup\n- A ([#901]", gh)
        self.assertIn("- Fixed: C ([#903]", gh)
        self.assertIn("- Docs: D ([#904]", gh)
        cl = notes.render_changelog(n, "2026-11-08")
        self.assertIn("### Fixed\n- C ([#903]", cl)
        self.assertIn("### Docs\n- D ([#904]", cl)
        self.assertEqual(errors(n), [])

    def test_old_single_issue_highlight(self):
        n = load()
        n["highlights"][0] = {"line": "X.", "issue": 78, "shot": "s"}
        self.assertIn("- X. ([#78]", notes.render_github(n, {}))


class Hidden(unittest.TestCase):
    def test_known_issues_hidden(self):
        self.assertNotIn("Known issues", notes.render_github(load(), {}))

    def test_empty_security_shows_no_heading(self):
        n = load()
        n["security"] = []
        self.assertNotIn("## Security", notes.render_github(n, {}))

    def test_no_before_you_update_anywhere(self):
        n = load()
        self.assertNotIn("Before you update", notes.render_github(n, {}))
        self.assertNotIn("Before you update", notes.render_changelog(n, "2026-11-08"))
        self.assertNotIn("heads up", notes.render_reddit(n))
        self.assertNotIn("Before you update", notes.render_discord(n))
        self.assertNotIn('"before"', notes.render_update_notes(n))

    def test_empty_group_dropped(self):
        n = load()
        n["groups"]["Empty"] = []
        self.assertNotIn("## Empty", notes.render_github(n, {}))


class Credits(unittest.TestCase):
    def test_map_entry_adds_login_bare_or_u(self):
        for key in ("u/clayuk62", "clayuk62"):
            gh = notes.render_github(load(), {key: "clay-gh"})
            self.assertIn("(@clay-gh)", gh, key)
            self.assertNotIn("19rwcbh51glqb/) (@", gh)

    def test_bad_login_is_error_bare_or_u(self):
        for key in ("u/clayuk62", "clayuk62"):
            self.assertTrue(any("github login" in e for e in errors(load(), {key: "not a login!"})), key)

    def test_all_48_credited(self):
        self.assertEqual(len(load()["credits"]), 48)


class Discord(unittest.TestCase):
    def test_long_dm_drops_theme_first_keeps_link(self):
        n = load()
        n["theme"] = "word " * 200
        dm = notes.render_discord(n)
        self.assertLessEqual(len(dm), 1000)
        self.assertNotIn("word word", dm)
        self.assertTrue(dm.rstrip().endswith("/releases/tag/1.0.0"))

    def test_link_and_bold_title(self):
        n = load()
        n["theme"] = "word " * 200
        dm = notes.render_discord(n)
        self.assertIn("What's in it: https://github.com/HolyArrs/HolyArrs/releases/tag/1.0.0", dm)
        self.assertTrue(notes.render_discord(n, bold=True).startswith("**HolyArrs 1.0.0 is ready**"))

    def test_announce_length_checked(self):
        n = load()
        n["highlights"][0]["line"] = "x" * 990
        e = errors(n)
        self.assertTrue(any("announce over 1000" in x for x in e))


class Check(unittest.TestCase):
    def add(self, n, line):
        n["groups"]["Setup"].append(line)
        return n

    def test_v_prefix_and_highlight_count(self):
        n = load()
        n["version"] = "v1.0.0"
        n["highlights"] = n["highlights"][:2]
        e = errors(n)
        self.assertTrue(any("X.Y.Z" in x for x in e))
        self.assertTrue(any("exactly 3" in x for x in e))

    def test_line_without_ref_and_launch_talk(self):
        e = errors(self.add(load(), "Coming soon to your server."))
        self.assertTrue(any("no (#n)" in x for x in e))
        self.assertTrue(any("launch talk" in x for x in e))

    def test_launch_words_not_launcher(self):
        for bad in ("Launching with music. (#75)", "Launch day fixes. (#75)", "It launches soon. (#75)"):
            self.assertTrue(any("launch talk" in x for x in errors(self.add(load(), bad))), bad)
        self.assertFalse(any("launch talk" in x for x in errors(self.add(load(), "Opens from the launcher. (#75)"))))

    def test_word_dates(self):
        for bad in ("Out in December. (#75)", "Ready on Friday. (#75)", "Ships tonight. (#75)",
                    "Lands tomorrow. (#75)", "More next week. (#75)", "Out today. (#75)", "Due May 4. (#75)"):
            self.assertTrue(any("launch talk" in x for x in errors(self.add(load(), bad))), bad)
        self.assertFalse(any("launch talk" in x for x in errors(self.add(load(), "You may pick seasons. (#75)"))))

    def test_banned_words_and_em_dash_are_errors(self):
        for bad, word in (("A robust — thing. (#75)", "robust"), ("We're excited for this. (#75)", "excited"),
                          ("Built in a clean room. (#75)", "clean room"), ("Each session saved. (#75)", "session"),
                          ("Agent picks it. (#75)", "Agent"), ("Worktree gone. (#75)", "Worktree"),
                          ("Works like v1.2.3 did. (#75)", "v1.2.3")):
            e = errors(self.add(load(), bad))
            self.assertTrue(any("banned word" in x and word in x for x in e), bad)
        self.assertTrue(any("em dash" in x for x in errors(self.add(load(), "A — B. (#75)"))))

    def test_before_you_update_is_rejected(self):
        n = load()
        n["before_you_update"] = {"breaking": ["x"]}
        self.assertTrue(any("before_you_update is no longer used" in x for x in errors(n)))

    def test_highlight_needs_issues(self):
        n = load()
        n["highlights"][0]["issues"] = []
        self.assertTrue(any("no issues" in x for x in errors(n)))

    def test_highlight_issue_in_group_is_error(self):
        n = copy.deepcopy(load())
        self.add(n, "Import again later. (#78)")
        self.assertTrue(any("#78" in x and "show it once" in x for x in errors(n)))

    def test_new_prefix_is_error(self):
        self.assertTrue(any("drop the prefix" in x for x in errors(self.add(load(), "New: thing. (#900)"))))

    def test_repeats_warn(self):
        n = self.add(load(), "Turn off the parts you dont use. (#901)")
        self.assertTrue(any("near-identical" in x for x in warnings(n)))
        n = self.add(load(), "Other words entirely. (#76)")
        self.assertTrue(any("same issues" in x for x in warnings(n)))

    def test_image_required(self):
        n = load()
        n.pop("image", None)
        n.pop("images", None)
        self.assertTrue(any("image" in x for x in errors(n)))


class Cli(unittest.TestCase):
    def test_check_exits_1_on_banned_word(self):
        src = (DATA / "notes-1.0.0.toml").read_text(encoding="utf-8")
        src = src.replace("Health at a glance.", "Robust health at a glance.")
        with tempfile.TemporaryDirectory() as d:
            p = pathlib.Path(d) / "n.toml"
            p.write_text(src, encoding="utf-8")
            self.assertEqual(notes.main(["1.0.0", "--file", str(p), "--map", str(p) + ".none", "--check"]), 1)

    def test_toml_shapes(self):
        n = load()
        self.assertEqual(n["version"], "1.0.0")
        self.assertEqual(n["known_issues"], [])
        self.assertEqual(n["highlights"][2]["issues"], [46, 47])
        self.assertEqual(list(n["groups"])[0], "Requests")


if __name__ == "__main__":
    unittest.main()
