"""The release notes come from the matching CHANGELOG.md section only."""
import importlib.util
import pathlib
import unittest

HERE = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("changelog_notes", HERE.parent / "changelog_notes.py")
notes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(notes)

TEXT = """# Changelog

Intro.

## 1.1.0

- New thing.

### Fixes

- Fixed thing.

## [1.0.10] - 2026-11-01

- Ten.

## 1.0.1

## 1.0.0

First release.
"""


class Section(unittest.TestCase):
    def test_takes_only_that_version(self):
        got = notes.section(TEXT, "1.1.0")
        self.assertEqual(got, "- New thing.\n\n### Fixes\n\n- Fixed thing.\n")

    def test_tag_with_v_and_bracket_heading(self):
        self.assertEqual(notes.section(TEXT, "v1.0.10"), "- Ten.\n")

    def test_last_section_and_no_prefix_match(self):
        self.assertEqual(notes.section(TEXT, "1.0.0"), "First release.\n")
        with self.assertRaises(ValueError):
            notes.section(TEXT, "1.0")

    def test_missing_or_empty_fails(self):
        for v in ("2.0.0", "1.0.1"):
            with self.assertRaises(ValueError):
                notes.section(TEXT, v)

    def test_real_changelog_has_the_first_release(self):
        text = (HERE.parents[2] / "CHANGELOG.md").read_text(encoding="utf-8")
        self.assertTrue(notes.section(text, "1.0.0").startswith("First release."))


if __name__ == "__main__":
    unittest.main()
