"""The release pipeline publishes safely: the release build pushes only the
version tag and beta, a separate hand-started promote workflow moves the
stable tags by digest, both registries get the same image, and CI watches
the Go modules for known holes. Text checks only, so no YAML library is
needed on the runner."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
WORKFLOWS = ROOT / ".github" / "workflows"
GHCR = "ghcr.io/coderluii/holyarrs"


def read(p: pathlib.Path) -> str:
    return p.read_text(encoding="utf-8")


def version(s: str) -> tuple:
    return tuple(int(x) for x in re.findall(r"\d+", s)[:3])


def docker_hub_named(text: str) -> bool:
    """Docker Hub's coderluii/holyarrs, written with or without docker.io/."""
    return re.search(r"(?<![\w./-])(docker\.io/)?coderluii/holyarrs\b", text.replace(GHCR, "")) is not None


def triggers(text: str) -> list:
    """The event names under the top-level on: key."""
    on = re.search(r"(?ms)^on:\s*\n(.*?)(?=^\S)", text)
    return re.findall(r"(?m)^  ([\w-]+):", on.group(1)) if on else []


def run_blocks(text: str) -> list:
    """The shell scripts of every run: step (single line or block)."""
    out = []
    lines = text.splitlines()
    i = 0
    while i < len(lines):
        m = re.match(r"^(\s*)(?:-\s+)?run:\s*(.*)$", lines[i])
        if not m:
            i += 1
            continue
        indent, rest = len(m.group(1)), m.group(2)
        if rest and rest[0] not in "|>":
            out.append(rest)
            i += 1
            continue
        i += 1
        block = []
        while i < len(lines) and (not lines[i].strip() or len(lines[i]) - len(lines[i].lstrip()) > indent):
            block.append(lines[i])
            i += 1
        out.append("\n".join(block))
    return out


class GoModules(unittest.TestCase):
    def test_x_image_has_the_webp_fix(self):
        m = re.search(r"^\s*golang\.org/x/image (v[\d.]+)", read(ROOT / "go.mod"), re.M)
        self.assertIsNotNone(m, "golang.org/x/image is not in go.mod")
        self.assertGreaterEqual(version(m.group(1)), (0, 47, 0), "x/image " + m.group(1) + " has GO-2026-6222")

    def test_go_sum_matches(self):
        m = re.search(r"^\s*golang\.org/x/image (v[\d.]+)", read(ROOT / "go.mod"), re.M)
        self.assertIn("golang.org/x/image " + m.group(1) + " ", read(ROOT / "go.sum"))


class VulnerabilityWatch(unittest.TestCase):
    def test_dependabot_watches_go_modules_weekly_grouped(self):
        text = read(ROOT / ".github" / "dependabot.yml")
        m = re.search(r"package-ecosystem:\s*[\"']?gomod[\"']?(.*?)(?=\n\s*-\s+package-ecosystem:|\Z)", text, re.S)
        self.assertIsNotNone(m, "no gomod entry in dependabot.yml")
        entry = m.group(1)
        self.assertRegex(entry, r"interval:\s*[\"']?weekly")
        self.assertRegex(entry, r"\n\s*groups:")

    def test_ci_runs_pinned_govulncheck(self):
        text = read(WORKFLOWS / "ci.yml")
        self.assertRegex(text, r"golang\.org/x/vuln/cmd/govulncheck@v\d+\.\d+\.\d+",
                         "ci.yml must run govulncheck at a pinned version")
        self.assertNotRegex(text, r"govulncheck@latest")

    def test_release_is_not_hard_gated_on_govulncheck(self):
        text = read(WORKFLOWS / "release.yml")
        if "govulncheck" in text:
            self.assertIn("continue-on-error: true", text)


class ReleaseBuild(unittest.TestCase):
    def setUp(self):
        self.text = read(WORKFLOWS / "release.yml")

    def test_stays_inert_until_approved(self):
        # Publishing waits for Luis's release approval; this change must not switch it on.
        self.assertRegex(self.text, r"\n\s+if:\s*\$\{\{\s*false\s*\}\}")
        self.assertEqual(triggers(self.text), ["workflow_dispatch"])

    def test_never_moves_latest(self):
        self.assertRegex(self.text, r"flavor:\s*\|?\s*\n?\s*latest=false")

    def test_pushes_only_version_and_beta(self):
        self.assertIn("type=semver,pattern={{version}}", self.text)
        self.assertNotIn("{{major}}.{{minor}}", self.text)
        self.assertNotIn("pattern={{major}}", self.text)
        self.assertRegex(self.text, r"value=beta\b")
        self.assertNotRegex(self.text, r"value=(latest|stable)\b")

    def test_beta_moves_only_forward(self):
        self.assertRegex(self.text, r"id:\s*betacheck")
        self.assertRegex(self.text, r"value=beta\b[^\n]*enable=\$\{\{[^}]*betacheck")

    def test_runs_queue_so_beta_never_goes_back(self):
        # Two tag runs at once would both read the old beta and the last push
        # would win, so runs queue like promote.yml.
        self.assertRegex(self.text, r"\nconcurrency:\s*\n\s+group:\s*release\s*\n\s+cancel-in-progress:\s*false")

    def test_ref_and_tag_guards(self):
        scripts = "\n".join(run_blocks(self.text))
        self.assertIn("refs/tags/", scripts, "no ref guard: the build must run from a version tag")
        for word in ("latest", "stable"):
            self.assertIn(word, scripts, "the tag guard must fail on " + word)

    def test_digest_in_run_summary(self):
        scripts = "\n".join(run_blocks(self.text))
        self.assertIn("GITHUB_STEP_SUMMARY", scripts)
        self.assertRegex(scripts, r"(?i)digest")

    def test_both_registries_ghcr_main(self):
        self.assertIn(GHCR, self.text)
        self.assertTrue(docker_hub_named(self.text), "release.yml does not push to Docker Hub")
        self.assertRegex(self.text, r"environment:\s*\n?\s*(name:\s*)?release\b")

    def test_no_nightly(self):
        self.assertNotIn("nightly", self.text.lower())


class PromoteWorkflow(unittest.TestCase):
    def setUp(self):
        p = WORKFLOWS / "promote.yml"
        self.assertTrue(p.exists(), "no promote.yml")
        self.text = read(p)
        self.scripts = "\n".join(run_blocks(self.text))

    def test_started_by_hand_only(self):
        self.assertEqual(triggers(self.text), ["workflow_dispatch"])

    def test_dry_run_by_default(self):
        self.assertRegex(self.text, r"dry[_-]?run:\s*\n(?:\s+.*\n)*?\s+default:\s*['\"]?true")

    def test_inputs_only_through_env(self):
        for line in self.text.splitlines():
            if re.search(r"\$\{\{\s*(github\.event\.)?inputs\.", line):
                self.assertRegex(line, r"^\s*[A-Za-z_][A-Za-z0-9_]*:\s*\$\{\{\s*(github\.event\.)?inputs\.[\w-]+\s*\}\}\s*$",
                                 "an input is used outside env: " + line.strip())

    def test_digest_checked_and_signature_verified(self):
        self.assertRegex(self.scripts, r"sha256:\[0-9a-f\]\{64\}|sha256:\[\[:xdigit:\]\]\{64\}|\[0-9a-f\]\{64\}")
        self.assertIn("cosign verify", self.scripts)

    def test_downgrade_guard_reads_version_label(self):
        self.assertIn("org.opencontainers.image.version", self.scripts)

    def test_moves_stable_tags_by_digest_and_reads_back(self):
        self.assertIn("imagetools create", self.scripts)
        self.assertIn("imagetools inspect", self.scripts)
        for word in ("stable", "latest"):
            self.assertIn(word, self.scripts)

    def test_both_registries(self):
        self.assertIn(GHCR, self.text)
        self.assertTrue(docker_hub_named(self.text), "promote.yml does not move Docker Hub tags")
        self.assertRegex(self.text, r"environment:\s*\n?\s*(name:\s*)?release\b")

    def test_least_permissions(self):
        self.assertRegex(self.text, r"(?m)^permissions:\s*\n\s+contents:\s*read")

    def test_no_nightly(self):
        self.assertNotIn("nightly", self.text.lower())


class InstallDocsTag(unittest.TestCase):
    def test_docs_point_at_beta_until_first_stable(self):
        text = read(ROOT / "docs" / "user" / "getting-started" / "install.md")
        self.assertIn("ghcr.io/coderluii/holyarrs:beta", text)
        self.assertNotIn("holyarrs:latest", text)


if __name__ == "__main__":
    unittest.main()
