import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("rc_check", Path(__file__).with_name("rc-check.py"))
rc = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rc)


class CandidateCheck(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "source"
        self.repo.mkdir()
        self.run_git("init", "-q")
        self.run_git("config", "user.name", "Test")
        self.run_git("config", "user.email", "test@example.invalid")
        self.api = self.repo / "app/backend/cmd/holyarrs/openapi.json"
        self.api.parent.mkdir(parents=True)
        self.api.write_text(json.dumps({"paths": {"/api/v1/old": {"get": {}}}}))
        self.run_git("add", "app/backend/cmd/holyarrs/openapi.json")
        self.run_git("commit", "-qm", "baseline")
        self.base = self.run_git("rev-parse", "HEAD")
        self.run_git("commit", "--allow-empty", "-qm", "synthetic candidate")
        self.artifact = self.root / "candidate.bin"
        self.artifact.write_bytes(b"synthetic candidate; not a release")
        self.baseline_artifact = self.root / "baseline.bin"
        self.baseline_artifact.write_bytes(b"synthetic prior artifact")
        self.state_snapshot = self.root / "state.snapshot"
        self.state_snapshot.write_bytes(b"synthetic consistent state archive")
        self.evidence = self.root / "evidence"
        self.evidence.mkdir()

    def run_git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.repo), *args], text=True).strip()

    def manifest(self):
        source = self.run_git("rev-parse", "HEAD")
        artifact = rc.digest(self.artifact)
        data_bindings = {"baseline_artifact_sha256": rc.digest(self.baseline_artifact),
                         "state_snapshot_sha256": rc.digest(self.state_snapshot)}
        checks = {}
        for name in rc.COMMON | rc.DATA | rc.API | {"candidate_trial"}:
            path = self.evidence / f"{name}.txt"
            path.write_text("Synthetic checker fixture, no release claim.\n")
            checks[name] = {"passed": True, "source": source, "baseline": self.base,
                            "artifact_sha256": artifact, "evidence": path.name,
                            "evidence_sha256": rc.digest(path)}
            if name in rc.DATA:
                checks[name].update(data_bindings)
        return {"format": "holyarrs-rc", "version": 1, "source": source, "baseline": self.base,
                "artifact_sha256": artifact, **data_bindings, "users_on_1_0": False,
                "stored_data_change": False, "checks": checks}

    def check(self, manifest):
        return rc.check(manifest, self.repo, self.artifact, self.evidence, self.base,
                        self.baseline_artifact, self.state_snapshot)

    def test_rehearsal_does_not_claim_real_candidate_gate(self):
        result = self.check(self.manifest())
        self.assertFalse(result["rc_required"])
        self.assertEqual(result["checks_verified"], sorted(rc.COMMON))

    def test_detects_data_and_removed_route_and_requires_trial(self):
        self.api.write_text('{"paths":{}}')
        migration = self.repo / "app/backend/internal/core/database/migrations/main/0002_fixture.sql"
        migration.parent.mkdir(parents=True)
        migration.write_text("-- original synthetic migration fixture\n")
        self.run_git("add", "app/backend/cmd/holyarrs/openapi.json", "app/backend/internal/core/database/migrations/main/0002_fixture.sql")
        self.run_git("commit", "-qm", "candidate")
        manifest = self.manifest()
        manifest["users_on_1_0"] = True
        result = self.check(manifest)
        self.assertTrue(result["rc_required"])
        self.assertTrue(result["stored_data_change"])
        self.assertEqual(result["removed_routes"], ["GET /api/v1/old"])
        del manifest["checks"]["candidate_trial"]
        with self.assertRaisesRegex(ValueError, "candidate_trial"):
            self.check(manifest)

    def test_missing_failed_and_stale_receipts(self):
        for change in ("missing", "failed", "source", "artifact_sha256", "baseline"):
            with self.subTest(change=change):
                manifest = self.manifest()
                if change == "missing":
                    del manifest["checks"]["tests"]
                elif change == "failed":
                    manifest["checks"]["tests"]["passed"] = False
                else:
                    manifest["checks"]["tests"][change] = "stale"
                with self.assertRaises(ValueError):
                    self.check(manifest)

    def test_declared_non_sql_format_change_requires_data_checks(self):
        manifest = self.manifest()
        manifest["stored_data_change"] = True
        del manifest["checks"]["disposable_restore"]
        with self.assertRaisesRegex(ValueError, "disposable_restore"):
            self.check(manifest)
        manifest["stored_data_change"] = "false"
        with self.assertRaisesRegex(ValueError, "stored_data_change"):
            self.check(manifest)

    def test_self_baseline_cannot_hide_removed_route(self):
        self.api.write_text('{"paths":{}}')
        self.run_git("add", "app/backend/cmd/holyarrs/openapi.json")
        self.run_git("commit", "-qm", "remove synthetic route")
        manifest = self.manifest()
        manifest["baseline"] = manifest["source"]
        for record in manifest["checks"].values():
            record["baseline"] = manifest["source"]
        with self.assertRaisesRegex(ValueError, "independently selected"):
            self.check(manifest)

    def test_data_inputs_and_receipts_are_bound(self):
        manifest = self.manifest()
        manifest["stored_data_change"] = True
        for field in ("baseline_artifact_sha256", "state_snapshot_sha256"):
            with self.subTest(field=field):
                altered = copy.deepcopy(manifest)
                altered.pop(field)
                with self.assertRaisesRegex(ValueError, "data input digest"):
                    self.check(altered)
                for name in rc.DATA:
                    altered = copy.deepcopy(manifest)
                    altered["checks"][name][field] = "stale"
                    with self.assertRaisesRegex(ValueError, "stale backup or prior artifact"):
                        self.check(altered)
        with self.assertRaisesRegex(ValueError, "missing or unsafe data input"):
            rc.check(manifest, self.repo, self.artifact, self.evidence, self.base)

    def test_changed_or_deleted_data_inputs_reject_candidate(self):
        for path in (self.baseline_artifact, self.state_snapshot):
            original = path.read_bytes()
            for change in ("tamper", "delete"):
                with self.subTest(path=path.name, change=change):
                    manifest = self.manifest()
                    manifest["stored_data_change"] = True
                    if change == "tamper":
                        path.write_bytes(b"changed input")
                    else:
                        path.unlink()
                    with self.assertRaisesRegex(ValueError, "data input"):
                        self.check(manifest)
                    path.write_bytes(original)

    def test_artifact_and_evidence_tampering(self):
        manifest = self.manifest()
        self.artifact.write_bytes(b"different")
        with self.assertRaisesRegex(ValueError, "artifact digest"):
            self.check(manifest)
        manifest = self.manifest()
        (self.evidence / "tests.txt").write_text("changed")
        with self.assertRaisesRegex(ValueError, "changed evidence"):
            self.check(manifest)

    def test_source_changes_invalidate_frozen_candidate(self):
        manifest = self.manifest()
        self.api.write_text('{"paths":{}}')
        with self.assertRaisesRegex(ValueError, "source changed"):
            self.check(manifest)

    def test_untracked_source_invalidates_frozen_candidate(self):
        manifest = self.manifest()
        (self.repo / "extra.go").write_text("package unexpected\n")
        with self.assertRaisesRegex(ValueError, "source changed"):
            self.check(manifest)

    def test_unsafe_evidence_and_nonboolean_user_flag(self):
        manifest = self.manifest()
        for name in ("../tests.txt", "C:/tests.txt", "/tests.txt"):
            altered = copy.deepcopy(manifest)
            altered["checks"]["tests"]["evidence"] = name
            with self.assertRaisesRegex(ValueError, "invalid evidence"):
                self.check(altered)
        manifest["users_on_1_0"] = "false"
        with self.assertRaisesRegex(ValueError, "true or false"):
            self.check(manifest)


if __name__ == "__main__":
    unittest.main()
