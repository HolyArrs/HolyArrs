# Release candidates

Once there are users on 1.0, a release that changes stored data or removes an API route needs a candidate trial before the final release. A local checker rehearsal does not satisfy that real-release gate.

Freeze the candidate source commit and artifact first. Keep its compatible prior artifact and consistent state backup. Test restoration and the candidate against disposable copied state before changing an installation. Never run an older binary against an incompatible migrated database. Record how to retain or export changes made after an update before rollback.

Run `python scripts/rc-check.py candidate.json --repo . --artifact candidate.bin --evidence-dir receipts --baseline-ref PRIOR_RELEASE` against a clean frozen checkout. Replace `PRIOR_RELEASE` with the independently verified previous release's tag or full source commit, selected from release provenance before reading the candidate manifest. The manifest cannot choose its own comparison baseline; it must match that input and differ from the candidate. This verifies receipt bindings and detects migration-file changes and removed OpenAPI routes against the baseline commit. It does not run the checks or prove the truth of a receipt; a reviewer must inspect the referenced evidence. Other stored-format changes need the same candidate process even when they occur outside SQL migrations.

The JSON manifest uses format `holyarrs-rc`, version `1`, full 40-character `source` and `baseline` commit IDs, the artifact's `artifact_sha256`, explicit booleans `users_on_1_0` and `stored_data_change`, and a `checks` object. Review every stored format before setting `stored_data_change`; SQL migration changes force the flag on regardless. Each named check has `passed: true`, the same `source`, `baseline` and `artifact_sha256`, a plain evidence filename and that file's `evidence_sha256`. Keep evidence files directly in the supplied directory. Never put credentials, state exports or personal inventories in receipts.

For stored-data changes, also pass `--baseline-artifact prior.bin --state-snapshot consistent-state.archive`. These must be regular files: the compatible prior binary and the exact consistent snapshot archive used for restore, migration and rollback checks. Include their `baseline_artifact_sha256` and `state_snapshot_sha256` in the manifest and every data-check receipt. The checker hashes both actual files again; replacing or deleting either invalidates the result. Keep the snapshot private and outside the receipt directory; receipts contain only its digest and the check result.

Required receipts:

| Trigger | Check names |
| --- | --- |
| Every candidate | `tests`, `build`, `secret_scan` |
| Stored-data changes | `consistent_backup`, `disposable_restore`, `forward_migration`, `compatible_rollback` |
| Removed API routes | `deprecation_window`, `consumer_transition` |
| Either change with users on 1.0 | `candidate_trial` |

The API evidence must prove the approved minimum 12-month deprecation window for outside-app routes and show the replacement/consumer transition. The data evidence must describe version compatibility, backup consistency, a successful disposable restore, a forward migration and compatible rollback. A failed or unavailable check stays failed or unavailable; it cannot become a passing receipt.

Every source, artifact or baseline change requires renewed bindings and fresh affected checks. Changed evidence files invalidate their digests. Preserve failed-run evidence and diagnosis. Run `python -m unittest discover -s scripts -p test_rc_check.py` to exercise the checker against synthetic commits, routes, migrations and receipts. Those fixtures create no product tag, release or user cohort.
