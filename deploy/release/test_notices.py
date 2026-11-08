import importlib.util
import tempfile
import unittest
from unittest import mock
from pathlib import Path

spec = importlib.util.spec_from_file_location("notices", Path(__file__).with_name("notices.py"))
notices = importlib.util.module_from_spec(spec)
spec.loader.exec_module(notices)

STATIC = {"ours": ["brand/", "favicon.ico"], "covered_by": [{"path": "vendor/hls-", "package": "hls.js"}],
          "listed": [{"path": "clients/deluge.svg", "name": "Deluge logo", "licence": "CC-BY-SA-2.0", "used": "x"}]}


class LicenceNames(unittest.TestCase):
    def test_each_licence(self):
        self.assertEqual(notices.spdx("Mozilla Public License, version 2.0"), "MPL-2.0")
        self.assertEqual(notices.spdx("Apache License\n  Version 2.0, January 2004"), "Apache-2.0")
        self.assertEqual(notices.spdx("Redistributions in binary form must reproduce ... Neither the name of Google"), "BSD-3-Clause")
        self.assertEqual(notices.spdx("Redistributions in binary form must reproduce the above"), "BSD-2-Clause")
        self.assertEqual(notices.spdx("Permission is hereby granted, free of charge, to any person"), "MIT")
        self.assertEqual(notices.spdx("Permission to use, copy, modify, and/or distribute this software for any purpose"), "ISC")
        self.assertEqual(notices.spdx("Boost Software License - Version 1.0 ... Permission is hereby granted, free of charge"), "BSL-1.0")
        self.assertEqual(notices.spdx("Redistributions in binary form must reproduce x. All advertising materials mentioning features y. Neither the name of z"), "BSD-4-Clause")
        self.assertEqual(notices.spdx("The author disclaims copyright to this source code."), "Public domain")
        self.assertEqual(notices.spdx("SQLite Is Public Domain\n\nAll of the code has been dedicated to the public\ndomain by the authors."), "Public domain")
        self.assertEqual(notices.spdx("something else"), "see the text")

    def test_the_first_licence_a_file_states_wins(self):
        self.assertEqual(notices.spdx("Redistributions in binary form must reproduce x. Neither the name of Google y. Files: z Apache License, Version 2.0"), "BSD-3-Clause")
        self.assertEqual(notices.spdx("Redistributions in binary form must reproduce x. Neither the names of the authors"), "BSD-3-Clause")
        self.assertEqual(notices.spdx("Redistributions in binary form must reproduce x. * My name, U K, may not be used to endorse or promote products"), "BSD-3-Clause")

    def test_a_gathered_file_names_every_licence(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            (root / "LICENSE").write_text("Redistributions in binary form must reproduce x. Neither the name of y")
            (root / "LICENSE-3RD-PARTY.md").write_text("Go: Redistributions in binary form must reproduce x. Neither the name of y\n\nmusl: Permission is hereby granted, free of charge\n")
            (root / "LICENSE-SQLITE").write_text("The author disclaims copyright to this source code.")
            m = {"files": sorted(root.iterdir())}
            self.assertEqual(notices.go_licence(m), "BSD-3-Clause (some parts MIT, Public domain)")


class Layout(unittest.TestCase):
    def test_fence_outlasts_backticks_in_the_text(self):
        self.assertTrue(notices.fence("a ``` b").startswith("````text\n"))
        self.assertEqual(notices.fence("plain\n"), "```text\nplain\n```")

    def test_headers_skip_fenced_licence_texts(self):
        self.assertEqual(notices.headers("# x\n### a@1\ntext\n### b\n"), ["a@1", "b"])
        self.assertEqual(notices.headers("### a\n```text\n### inside\n```\n### b\n"), ["a", "b"])

    def test_release_targets_come_from_packaging(self):
        targets = notices.package_info()
        self.assertIn(("windows", "amd64", ""), targets.values())


class Check(unittest.TestCase):
    def run_check(self, have_docs, dist_files=()):
        go = [{"name": "example.org/m", "version": "v1.0.0", "files": [Path("LICENSE")], "rel": Path(".")}]
        web = [{"name": "svelte", "version": "5.0.0", "declared": "MIT", "files": []}]
        others = [{"name": "yt-dlp", "version": "1", "licence": "x", "in": "y", "used": "z", "files": []}]
        with mock.patch.object(notices, "go_licence", lambda m: "MIT"):
            want, table = notices.build(go, web, others, STATIC)
        summary = "# T\n\n" + notices.TABLE_START + "\n"
        summary = notices.summary_with(summary, table)
        with tempfile.TemporaryDirectory() as d:
            for f in dist_files:
                p = Path(d) / f
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text("")
            return notices.problems(go, web, others, STATIC, want, table, have_docs if have_docs is not None else want, summary, Path(d))

    def test_ytdlp_bundled_parts_are_never_listed_or_flagged(self):
        self.assertTrue(notices.SKIPPED.search("yt-dlp bundled parts"))
        self.assertTrue(notices.SKIPPED.search("THIRD_PARTY_LICENSES.txt"))
        self.assertFalse(notices.SKIPPED.search("yt-dlp@2026.08.19"))
        self.assertEqual(self.run_check(None, ["index.html"]), [])
        found = self.run_check("### example.org/m@v1.0.0\n### svelte@5.0.0\n### yt-dlp@1\n### yt-dlp bundled parts\n### Deluge logo (/clients/deluge.svg)\n")
        self.assertFalse(any("bundled" in f for f in found), found)

    def test_complete_notices_pass(self):
        self.assertEqual(self.run_check(None, ["index.html", "brand/mark.svg", "clients/deluge.svg", "vendor/hls-1.min.js", "_app/immutable/assets/geist.woff2"]), [])

    def test_a_missing_part_and_a_stray_file_fail(self):
        found = self.run_check("### example.org/m@v1.0.0\n### svelte@4.0.0\n", ["clients/plex.svg", "fonts/paid.woff", "favicon.ico.br"])
        self.assertTrue(any(f.startswith("missing in docs/third-party-notices.md: svelte@5.0.0") for f in found), found)
        self.assertTrue(any("no longer shipped" in f and "svelte@4.0.0" in f for f in found), found)
        self.assertTrue(any("app/frontend/dist/clients/plex.svg" in f for f in found), found)
        self.assertTrue(any("font outside" in f and "fonts/paid.woff" in f for f in found), found)
        self.assertFalse(any("favicon" in f for f in found), found)


if __name__ == "__main__":
    unittest.main()
