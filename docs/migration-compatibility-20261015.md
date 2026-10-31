# Migration compatibility continuation plan

**Goal:** Close the migration gaps that fit the existing HolyArrs data model while preserving least privilege, source relationships, repeat safety, undo safety and read-only media handling.

**Architecture:** Extend migration snapshots and plans with source-stable person identities, per-kind request limits, supported Request defaults, and Lidarr artist policy. Apply those fields only through the existing store and service contracts. Keep unsupported source values explicit in the plan report and keep real-media trials disposable.

**Tech stack:** Go, SQLite, existing `internal/migrate`, `internal/asks`, `internal/library` and store test fixtures.

## Global constraints

- Change only migration-owned code, tests and migration documentation.
- Do not change shared schema, authentication routes or UI.
- Do not activate imported people, issue invitations, or grant broader permissions than the source value can represent.
- Keep real media read-only; use disposable databases and private aggregate-only evidence.
- Do not include private paths, names, inventories, credentials or raw snapshots in tracked files.

## Task 1: Acceptance and gap lock

- [x] Add focused failing tests for supported Seerr defaults, per-kind person limits, least-privilege permission mapping, source-stable identity, request ownership and decider timestamps.
- [x] Add focused failing tests for Lidarr new-album policy, metadata filters, monitored album selections and watched editions.
- [x] Run the focused tests and confirm they fail because the mappings are absent.

## Task 2: Snapshot and plan mappings

- [x] Read stable external person ids when the source exposes them without linking sign-in automatically.
- [x] Read Lidarr albums and their watched edition through bounded API GETs and backups.
- [x] Map exact permissions only; retain broader source-only permission variants as not moved.
- [x] Map exact seven-day per-kind limits and supported Request defaults; report other windows and ambiguous per-person zero values as not moved.
- [x] Reject out-of-range limits instead of clamping them, and bound source timestamps by the persisted snapshot read time.
- [x] Map Lidarr new-album policy, filter kinds/statuses, album monitoring and watched editions; retain invalid or unrepresentable values with precise reasons.

## Task 3: Apply, repeat and undo

- [x] Apply supported Request defaults with before/after marks and guarded undo.
- [x] Apply per-kind person limits and preserve request asker/decider relationships.
- [x] Apply artist policy after adoption and choose a watched edition only when the imported Pressing exists.
- [x] Verify a repeat creates no duplicate rows and an undo removes only task-created relationships/settings while leaving media untouched.
- [x] Keep later request owners and changed relationship timestamps during undo; use the persisted move start for deterministic missing-time fallback.
- [x] Keep a dependency claim when a later move imports the same request for the same resolved person.
- [x] Compare and restore settings and music inside one write transaction, and fail closed when an expected row or saved baseline cannot be safely restored.
- [x] Treat legacy natural-row marks without fingerprints as unverifiable instead of trusting missing or recycled row ids.

## Task 4: Private compatibility evidence

- [x] Preserve the prior aggregate classification of the 40 absent folders from private GET reads and an authorized read-only media root.
- [ ] Rerun the full GET-only 348-title media-folder classifier. Its prior aggregate result remains 308 present and 40 absent. The current trial intentionally had no media mount, so it does not replace that full-library evidence.
- [x] Add a private disposable current-source runner using the supported GET-only Seerr and Lidarr readers. It has a fresh temporary database, no workers or media mounts, bounded aggregate output, and exact relationship, default-limit and music-policy assertions.
- [x] Independently review, build and run the disposable current-source trial. The source reads returned 61 artists, 253 requests and 5 users. The exact move exercise applied 5 people, 253 asks, 1 supported default, and 1 selected music-policy album and Pressing. Preview was read-only, repeat made no further changes, undo restored the tested baseline, and all relationship, ownership and policy assertions passed.
- [x] Keep all 396 not-moved entries in the trial report. These are explicit unsupported or unmapped observations, not dropped records. The snapshots were secret-free and nothing needed secret re-entry.
- [x] Save only aggregate/private evidence under `_cache/runtime/autonomous-continuation-20261015/migration/`.
- [x] Publish a public-safe handoff at `releases/autonomous-continuation-20261015/migration-handoff.md`.

The disposable trial proves the supported compatibility paths against current source and real GET responses. It does not prove full live-library parity: it exercised one chosen music-policy album and Pressing, mounted no media, started no workers and performed no live cutover. The 308-present and 40-absent folder classification remains prior evidence.

## Verification

- [x] `go test -count=1 ./internal/migrate` (103 tests passed; one existing test skipped)
- [ ] `go test -race -count=1 ./internal/migrate` (this Windows Go environment has CGO disabled, so the race build is unavailable)
- [x] `go vet ./internal/migrate`
- [x] `go mod verify`
- [ ] Root-owned final union tests after integration.
- [x] Secret scanner on the exact staged files.
