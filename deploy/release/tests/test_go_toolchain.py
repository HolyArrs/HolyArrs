"""The Go that builds images and release files is the one go.mod asks for,
and never older than the release with the HTTP/1 server crash fix."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
# Oldest Go whose net/http no longer crashes on the MaxBytesReader header race.
FLOOR = (1, 27, 4)
BUILDERS = [ROOT / "deploy" / "Dockerfile", ROOT / "deploy" / "release" / "package.py"]


def version(text: str) -> tuple:
    return tuple(int(part) for part in text.split("."))


def go_line() -> tuple:
    found = re.search(r"^go\s+(\d+(?:\.\d+)+)\s*$", (ROOT / "go.mod").read_text(encoding="utf-8"), re.M)
    return version(found.group(1))


def images(path: pathlib.Path) -> list:
    # Every golang image the file names, with its digest when pinned.
    return re.findall(r"golang:(\d+(?:\.\d+)+)(@sha256:[0-9a-f]{64})?", path.read_text(encoding="utf-8"))


class GoToolchain(unittest.TestCase):
    def test_go_mod_asks_for_the_crash_fix(self):
        self.assertGreaterEqual(go_line(), FLOOR)

    def test_build_images_are_pinned_and_new_enough(self):
        # The golang images build with GOTOOLCHAIN=local, so an image older
        # than go.mod's go line cannot build this module at all.
        want = go_line()
        for path in BUILDERS:
            found = images(path)
            self.assertTrue(found, f"{path.name} names no golang image")
            for tag, pin in found:
                with self.subTest(file=path.name, tag=tag):
                    self.assertGreaterEqual(version(tag), want, f"{path.name} builds with Go {tag}")
                    self.assertTrue(pin, f"{path.name} names golang:{tag} without a digest")


if __name__ == "__main__":
    unittest.main()
