import argparse
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import xml.etree.ElementTree as ET
import zipfile


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).resolve().parent.parent / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


package = module("package", "package.py")
nas = module("nas_template", "nas-template.py")


class PackagingTests(unittest.TestCase):
    def test_low_resource_profile_caps_containers_and_preserves_fixture_archives(self):
        commit = "a" * 40
        version = "0.0.0-candidate.1+g" + commit[:12]
        commands = []

        def snapshot(ref, destination):
            (destination / "LICENSE").write_bytes(b"project license\n")
            return commit, 1700000000, {"LICENSE": 0o644}

        def docker(command):
            if command[:2] == ["git", "rev-parse"]:
                return ("b" * 40 + "\n").encode()
            commands.append(command)
            source = Path(command[command.index("-v") + 1].removesuffix(":/src"))
            go_args = command[command.index(package.GO_IMAGE) + 1:]
            if go_args == ["go", "version"]:
                return b"go version go1.27.2 linux/amd64\n"
            if go_args[:2] == ["go", "build"]:
                executable = Path(go_args[go_args.index("-o") + 1]).name
                (source / executable).write_bytes(b"fixture executable " + version.encode())
            if go_args == ["cat", "/usr/local/go/LICENSE"]:
                return b"Go license\n"
            return b""

        with tempfile.TemporaryDirectory() as directory:
            ui_dist = Path(directory) / "ui-dist"
            ui_dist.mkdir()
            outputs = []
            profiles = []
            for low_resource in (False, True):
                commands.clear()
                output = Path(directory) / str(low_resource)
                args = argparse.Namespace(ref=commit, version="0.0.0-candidate.1", output=str(output),
                                          targets=",".join(package.TARGETS), module_cache="existing-cache",
                                          web_node_modules="existing-modules", low_resource=low_resource)
                with patch.object(package, "snapshot", side_effect=snapshot), \
                     patch.object(package, "web_notices", return_value=b"web notices\n"), \
                     patch.object(package, "build_ui", return_value=ui_dist), \
                     patch.object(package, "run", side_effect=docker):
                    package.build(args)
                self.assertEqual(len(commands), 12)
                normalized = []
                for command in commands:
                    plain = command.copy()
                    image_index = command.index(package.GO_IMAGE)
                    for flag, value in (("--cpus", "1"), ("--memory", "2g"),
                                        ("--memory-swap", "2g"), ("--pids-limit", "256")):
                        self.assertEqual(command.count(flag), int(low_resource))
                        if low_resource:
                            index = command.index(flag)
                            self.assertLess(index + 1, image_index)
                            self.assertEqual(command[index + 1], value)
                            index = plain.index(flag)
                            del plain[index:index + 2]
                    for value in ("GOMAXPROCS=1", "GOMEMLIMIT=768MiB", "GOGC=50", "GOFLAGS=-p=1"):
                        self.assertEqual(command.count(value), int(low_resource))
                        if low_resource:
                            index = command.index(value)
                            self.assertLess(index, image_index)
                            self.assertEqual(command[index - 1], "-e")
                            index = plain.index(value)
                            del plain[index - 1:index + 1]
                    plain[plain.index("-v") + 1] = "<source>:/src"
                    for i, value in enumerate(plain):
                        if i and plain[i - 1] == "-v" and value.endswith(":/gocache"):
                            plain[i] = "<cache>:/gocache"
                    normalized.append(plain)
                profiles.append(normalized)
                outputs.append({p.name: p.read_bytes() for p in output.iterdir()})
            self.assertEqual(profiles[0], profiles[1])
            self.assertEqual(outputs[0], outputs[1])
            for low_resource in (False, True):
                argv = ["package.py", "--ref", commit, "--version", "0.0.0-candidate.1",
                        "--output", "new-output", "--module-cache", "existing-cache",
                        "--web-node-modules", "existing-modules"]
                if low_resource:
                    argv.append("--low-resource")
                with patch("sys.argv", argv), patch.object(package, "build") as build:
                    package.main()
                    self.assertIs(build.call_args.args[0].low_resource, low_resource)

    def test_rejects_unsafe_versions(self):
        for version in ("../bad", "1.2.3/other", "1.2.3\n", "v1.2.3", "1.2.3;echo", "1.2.3-a..b", ""):
            with self.subTest(version=version), self.assertRaises(ValueError):
                package.checked_version(version)
        self.assertEqual(package.checked_version("0.0.0-candidate.1+gabcdef0"), "0.0.0-candidate.1+gabcdef0")

    def test_archive_bytes_are_reproducible_and_modes_survive(self):
        files = {"docs/start/install.mdx": (b"instructions\n", 0o644), "holyarrs": (b"binary", 0o755)}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for suffix in ("tar.gz", "zip"):
                a, b = root / ("a." + suffix), root / ("b." + suffix)
                package.archive(a, files, 1700000000)
                package.archive(b, dict(reversed(list(files.items()))), 1700000000)
                self.assertEqual(a.read_bytes(), b.read_bytes())
            with zipfile.ZipFile(root / "a.zip") as archive:
                self.assertEqual(archive.getinfo("holyarrs").external_attr >> 16 & 0o777, 0o755)

    def test_all_selected_targets_have_exact_tuples(self):
        self.assertEqual(package.TARGETS["linux-armv7"], ("linux", "arm", "7"))
        self.assertEqual(set(package.TARGETS), {"linux-amd64", "linux-arm64", "linux-armv7", "windows-amd64"})

    def test_archive_rejects_absolute_and_traversal_members(self):
        with tempfile.TemporaryDirectory() as directory:
            for name in ("/etc/config", "../config", "docs/../../config", "C:/config", "docs\\file"):
                with self.subTest(name=name), self.assertRaises(ValueError):
                    package.archive(Path(directory) / "a.zip", {name: (b"data", 0o644)}, 1700000000)

    def test_notice_version_mismatch_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "app/frontend").mkdir(parents=True)
            (root / "app/frontend/package-lock.json").write_text(json.dumps({"packages": {"node_modules/svelte": {"version": "1"}}}))
            modules = root / "modules"
            (modules / "svelte").mkdir(parents=True)
            (modules / "svelte/package.json").write_text('{"version":"2"}')
            # The module root normally has the literal node_modules name.
            modules.rename(root / "node_modules")
            with self.assertRaises(ValueError):
                package.web_notices(root, root / "node_modules")

    def test_missing_nonoptional_web_package_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "app/frontend").mkdir(parents=True)
            (root / "app/frontend/package-lock.json").write_text(json.dumps({"packages": {"node_modules/other": {"version": "1"}}}))
            (root / "node_modules").mkdir()
            with self.assertRaises(ValueError):
                package.web_notices(root, root / "node_modules")

    def test_linked_module_root_and_license_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "app/frontend").mkdir(parents=True)
            (root / "app/frontend/package-lock.json").write_text(json.dumps({"packages": {"node_modules/svelte": {"version": "1"}}}))
            modules = root / "node_modules"
            (modules / "svelte").mkdir(parents=True)
            (modules / "svelte/package.json").write_text('{"version":"1"}')
            (modules / "svelte/LICENSE").write_text('license')
            for linked in (modules, modules / "svelte/LICENSE"):
                with self.subTest(linked=linked), patch.object(Path, "is_symlink", lambda p: p == linked), self.assertRaises(ValueError):
                    package.web_notices(root, modules)

    def test_nas_requires_immutable_image_local_config_and_separate_media(self):
        image = "docker.io/holyarrs/holyarrs@sha256:" + "a" * 64
        config = "/mnt/cache/appdata/holyarrs"
        for bad in ("holyarrs:latest", "https://host/image", "owner:password@host/image"):
            with self.assertRaises(ValueError):
                nas.render(bad, config, 6262)
        for bad in ("/mnt/user/appdata/holyarrs", "/mnt/user0/appdata/holyarrs", "/mnt/remotes/appdata/holyarrs", "/mnt/cache/../appdata/holyarrs", "/mnt/cache"):
            with self.assertRaises(ValueError):
                nas.render(image, bad, 6262)
        for media in ({"/movies": "/mnt/cache/appdata"}, {"/tv": "/mnt/user/../etc"}, {"/data": "/mnt/user/media"}, {"/music": "relative"}):
            with self.subTest(media=media), self.assertRaises(ValueError):
                nas.render(image, config, 6262, media)
        with self.assertRaises(ValueError):
            nas.render(image, config, 80)
        doc = ET.fromstring(nas.render(image, config, 6262, {"/movies": "/mnt/user/media/movies", "/downloads": "/mnt/user/media/downloads"}))
        self.assertEqual(doc.findtext("Repository"), image)
        self.assertEqual(doc.findtext("Privileged"), "false")
        # In-app Restart, safe mode and restore exit and rely on a restart policy.
        self.assertIn("--restart unless-stopped", doc.findtext("ExtraParams") or "")
        self.assertEqual(doc.findtext("WebUI"), "http://[IP]:[PORT:6262]/")
        configs = {c.attrib["Target"]: c for c in doc.findall("Config")}
        paths = [(c.attrib["Target"], c.text or "", c.attrib["Required"]) for c in doc.findall("Config") if c.attrib["Type"] == "Path"]
        self.assertEqual(paths, [("/config", config, "true"), ("/movies", "/mnt/user/media/movies", "false"),
                                 ("/tv", "", "false"), ("/music", "", "false"),
                                 ("/downloads", "/mnt/user/media/downloads", "false")])
        self.assertNotIn("/data", configs)
        self.assertEqual(configs["6262"].attrib["Type"], "Port")
        for target, default in (("PUID", "99"), ("PGID", "100"), ("UMASK", "022")):
            with self.subTest(target=target):
                self.assertEqual(configs[target].attrib["Type"], "Variable")
                self.assertEqual(configs[target].attrib["Default"], default)
                self.assertEqual(configs[target].text, default)
                self.assertEqual(configs[target].attrib["Display"], "advanced")
        self.assertNotIn("PAGID", configs)
        self.assertEqual(configs["HOLYARRS_SAVE_PATH"].attrib["Default"], "")
        self.assertIn("Downloader's own setting", configs["HOLYARRS_SAVE_PATH"].attrib["Description"])
        self.assertIn("PUID:PGID", doc.findtext("Overview"))


    def test_nas_offers_an_empty_allowed_host_names_field(self):
        image = "docker.io/holyarrs/holyarrs@sha256:" + "a" * 64
        config = "/mnt/cache/appdata/holyarrs"
        doc = ET.fromstring(nas.render(image, config, 6262))
        configs = {c.attrib["Target"]: c for c in doc.findall("Config")}
        hosts = configs.get("HOLYARRS_ALLOWED_HOSTS")
        self.assertIsNotNone(hosts, "the template has no HOLYARRS_ALLOWED_HOSTS field")
        self.assertEqual(hosts.attrib["Type"], "Variable")
        self.assertEqual(hosts.attrib["Default"], "")
        self.assertEqual(hosts.text or "", "")
        self.assertEqual(hosts.attrib["Required"], "false")
        self.assertIn("tower.local", hosts.attrib["Description"])
        # Never a trusted proxy preset.
        self.assertNotIn("HOLYARRS_TRUSTED_PROXIES", configs)

    def test_nas_allowed_host_names_are_checked(self):
        image = "docker.io/holyarrs/holyarrs@sha256:" + "a" * 64
        config = "/mnt/cache/appdata/holyarrs"
        doc = ET.fromstring(nas.render(image, config, 6262, allowed_hosts="tower.local,nas"))
        configs = {c.attrib["Target"]: c for c in doc.findall("Config")}
        self.assertEqual(configs["HOLYARRS_ALLOWED_HOSTS"].attrib["Default"], "tower.local,nas")
        for bad in ("tower.local/x", "a b", "*.example", "user@host", "tower.local;rm", "<b>"):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                nas.render(image, config, 6262, allowed_hosts=bad)


if __name__ == "__main__":
    unittest.main()
