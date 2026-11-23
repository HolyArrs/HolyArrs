"""The language packs for the plugin list: one bzip2 file per language and the list entries."""
import bz2
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location("pack_languages", ROOT / "deploy" / "pack_languages.py")
packer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packer)


class PackLanguagesTest(unittest.TestCase):
    def write(self, src: Path, lang: str, app: str = "1.2.1") -> bytes:
        data = json.dumps({"lang": lang, "app": app, "fit": "abc123abc123", "values": ["x", None]}).encode()
        (src / f"{lang}.json").write_bytes(data)
        return data

    def test_packs_each_language_with_its_entry(self):
        with tempfile.TemporaryDirectory() as d:
            src, out = Path(d, "src"), Path(d, "out")
            src.mkdir()
            de = self.write(src, "de")
            self.write(src, "fr")
            entries = packer.pack(src, out)
            packed = (out / "de" / "1.2.1.json.bz2").read_bytes()
            self.assertEqual(bz2.decompress(packed), de)
            self.assertEqual(entries[0], {"id": "lang-de", "version": "1.2.1", "sha256": hashlib.sha256(packed).hexdigest()})
            self.assertEqual(json.loads((out / "entries.json").read_text()), entries)

    def test_refuses_mixed_versions_and_bad_files(self):
        with tempfile.TemporaryDirectory() as d:
            src = Path(d, "src")
            src.mkdir()
            self.write(src, "de")
            self.write(src, "fr", "1.2.0")
            with self.assertRaises(SystemExit):
                packer.pack(src, Path(d, "out"))
            (src / "fr.json").write_text('{"fit":"abc","values":[]}')
            with self.assertRaises(SystemExit):
                packer.pack(src, Path(d, "out"))


if __name__ == "__main__":
    unittest.main()
