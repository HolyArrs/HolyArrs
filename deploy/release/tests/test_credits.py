"""credits.py: GitHub reporters and commenters join the credits, the maintainer and
bots never do, and a Reddit name only gets a login through credits-map.json."""
import importlib.util
import pathlib
import tomllib
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("credits", HERE.parent / "credits.py")
credits = importlib.util.module_from_spec(spec)
spec.loader.exec_module(credits)
nspec = importlib.util.spec_from_file_location("notes", HERE.parent / "notes.py")
notes = importlib.util.module_from_spec(nspec)
nspec.loader.exec_module(notes)

U = "https://github.com/HolyArrs/HolyArrs/issues/"


def user(login, kind="User"):
    return {"login": login, "type": kind}


FAKE = {
    "repos/HolyArrs/HolyArrs/milestones?state=all&per_page=100": [{"title": "0.9.0", "number": 1},
                                                                   {"title": "1.0.0", "number": 2}],
    "repos/HolyArrs/HolyArrs/issues?milestone=2&state=all&per_page=100": [
        {"number": 7, "user": user("ann-gh"), "html_url": U + "7"},
        {"number": 5, "user": user("CoderLuii"), "html_url": U + "5"},
    ],
    "repos/HolyArrs/HolyArrs/issues/5/comments?per_page=100": [
        {"user": user("clay-gh"), "html_url": U + "5#issuecomment-1"},
        {"user": user("helper[bot]", "Bot"), "html_url": U + "5#issuecomment-2"},
        {"user": user("coderluii"), "html_url": U + "5#issuecomment-3"},
    ],
    "repos/HolyArrs/HolyArrs/issues/7/comments?per_page=100": [
        {"user": user("clay-gh"), "html_url": U + "7#issuecomment-4"},
        {"user": user("ann-gh"), "html_url": U + "7#issuecomment-5"},
    ],
}

NOTES = '''version = "1.0.0"
credits = [
  {name = "u/clayuk62", link = "https://www.reddit.com/x/", issues = [5]},
]

[stats]
people_credited = 1
'''


def api(path):
    return FAKE.get(path, [])


class Collect(unittest.TestCase):
    def test_skips_maintainer_and_bots_keeps_first_link(self):
        people = credits.collect("1.0.0", api)
        self.assertEqual(set(people), {"ann-gh", "clay-gh"})
        self.assertEqual(people["clay-gh"], {"link": U + "5#issuecomment-1", "issues": [5, 7]})
        self.assertEqual(people["ann-gh"], {"link": U + "7", "issues": [7]})

    def test_missing_milestone_stops(self):
        with self.assertRaises(SystemExit):
            credits.collect("9.9.9", api)


class Merge(unittest.TestCase):
    def run_merge(self, cmap):
        people = credits.collect("1.0.0", api)
        text, changes = credits.merge(NOTES, tomllib.loads(NOTES), people, cmap)
        return tomllib.loads(text), changes

    def test_unmapped_people_get_their_own_credit(self):
        n, changes = self.run_merge({})
        gh = [c for c in n["credits"] if c.get("github")]
        self.assertEqual({c["github"] for c in gh}, {"ann-gh", "clay-gh"})
        self.assertNotIn("github", n["credits"][0])
        self.assertEqual(len(changes), 2)
        line = notes.credit_line([c for c in gh if c["github"] == "ann-gh"][0], {})
        self.assertEqual(line, "[@ann-gh](%s7)" % U)

    def test_mapped_person_merges_into_reddit_credit(self):
        n, changes = self.run_merge({"u/clayuk62": "clay-gh"})
        self.assertEqual(n["credits"][0]["github"], "clay-gh")
        self.assertEqual(len(n["credits"]), 2)
        self.assertEqual(notes.credit_line(n["credits"][0], {}), "[u/clayuk62](https://www.reddit.com/x/) (@clay-gh)")

    def test_second_run_adds_nobody(self):
        people = credits.collect("1.0.0", api)
        once, _ = credits.merge(NOTES, tomllib.loads(NOTES), people, {})
        twice, changes = credits.merge(once, tomllib.loads(once), people, {})
        self.assertEqual(changes, [])
        self.assertEqual(once, twice)

    def test_empty_credits_list(self):
        src = 'version = "1.0.0"\ncredits = []\n'
        text, _ = credits.merge(src, tomllib.loads(src), {"ann-gh": {"link": U + "7", "issues": [7]}}, {})
        self.assertEqual(tomllib.loads(text)["credits"], [{"github": "ann-gh", "link": U + "7", "issues": [7]}])

    def test_only_maintainer_adds_nobody(self):
        only = dict(FAKE)
        only["repos/HolyArrs/HolyArrs/issues/5/comments?per_page=100"] = []
        only["repos/HolyArrs/HolyArrs/issues/7/comments?per_page=100"] = []
        only["repos/HolyArrs/HolyArrs/issues?milestone=2&state=all&per_page=100"] = [
            {"number": 5, "user": user("CoderLuii"), "html_url": U + "5"}]
        self.assertEqual(credits.collect("1.0.0", lambda p: only.get(p, [])), {})


class Render(unittest.TestCase):
    def test_github_only_credit_counts_and_checks(self):
        n = notes.load_notes(HERE / "testdata" / "notes-1.0.0.toml")
        n["credits"].append({"github": "ann-gh", "link": U + "7", "issues": [7]})
        n["stats"]["people_credited"] = 49
        self.assertEqual(notes.check(n, {}), ([], []))
        self.assertIn("[@ann-gh](%s7)" % U, notes.render_github(n, {}))


if __name__ == "__main__":
    unittest.main()
