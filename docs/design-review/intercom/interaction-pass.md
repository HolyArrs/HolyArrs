# Interaction pass: final verification and evidence

The frozen-source pass completed **213/213 interaction checks** at 360px, 834px and 1440px, plus **8/8 touch checks** at 390px and 834px. There were no recorded browser runtime errors, failed assertions or horizontal document overflow. The checks visited 53 concrete routes covering all 48 implemented page templates.

Frontend source SHA-256: `7e39bb2cb51d787f61ae9f35e0995cf4fb12dd3150fd1c276e4cb7d01c387ed9`. Both runners recorded this same hash before and after their runs. The final integration commit is recorded in the shared handoff after the local commit is created.

| Evidence | Result |
| --- | --- |
| Main checks | 71 phone, 70 tablet, 72 desktop; all passing |
| Additional touch checks | Four workflows at two widths; all eight passing |
| Recorded evidence | 180 named steps, 2,395 final visible controls, 142 intercepted write attempts, 615 fixture responses |
| Screenshots | 221 final images and 27 diagnostic before/after pairs |
| Svelte diagnostics | 0 errors, 0 warnings |
| Frontend unit tests | 226 passing tests across 33 files |
| Independent production review | Approved; no findings across 17 production files |
| Harness checks | All 11 interaction scripts parse; gallery matrix, source identity, PNG and interception validation pass |

The visible-control count is a record of final browser states, not a claim that 2,395 distinct controls were each activated. Named scenarios and assertions define the interaction coverage. The diagnostic pairs include both product failures and corrected test assertions.

Review the [screenshot and interaction gallery](http://10.10.25.25:18746/preview-audit/interaction-pass/index.html), also available over [Tailscale](http://100.87.70.86:18746/preview-audit/interaction-pass/index.html). It includes viewport and scenario filters, recorded steps, intercepted requests and expandable diagnostic comparisons.

This pass tests the actual disposable HolyArrs preview on port 18746. It covers the registered navigation, Library, management, activity, settings, account, and form scenarios at every width allowed by each scenario's declared constraints. The runner creates a fresh isolated browser context for each scenario and viewport.

## Repairs

- Search, Music, Wishlist, Downloader and Subtitle settings: repaired generated input-handler crashes caused by a state variable colliding with the compiler's setter name. The same forms now retain edits through successful and rejected fixture saves.
- Search and people: stale provider responses no longer replace the latest selected kind or role. Clearing a search before changing kind also invalidates a pending error. Search addresses retain the current query and kind.
- Collection detail: a failed initial load exposes a working retry; successful recovery clears the stale error. Failed operations retain their error while refreshing the collection.
- Requests and missing subtitles: pending controls prevent repeated read, take-back and bulk-search actions. Request actions fit phone cards, and toast text no longer intercepts clicks on controls underneath it; the toast dismiss button remains interactive.
- Backups and sign-in settings: controls expose pending state, reject duplicate submissions, and roll back optimistic choices after a rejected save.
- Library: returning from another sidebar section retains the last-opened title marker as well as the existing filters, query and scroll memory.
- Navigation: theme switching stays available in the tablet rail and desktop Settings, where the context sidebar is hidden. A rejected sign-out no longer leaves an unhandled browser error; local session cleanup still returns to sign-in and the UI reports that the server could not confirm sign-out.

All production changes are under `web/app/src`. No dependency, API, database, backend, credential, installed-app, saved-Air-preview or media changes are part of this pass.

## Reproduce the final pass

Run these commands from the preview worktree while the disposable preview owns port 18746 and its source is frozen:

```powershell
$env:INTERACTION_BATCH = 'final'
$env:INTERACTION_WIDTHS = '360,834,1440'
Remove-Item Env:INTERACTION_MATCH -ErrorAction SilentlyContinue
node docs/design-review/intercom/interaction-runner.cjs
if ($LASTEXITCODE -ne 0) { throw 'Final interaction runner failed.' }
node docs/design-review/intercom/interaction-touch.cjs
if ($LASTEXITCODE -ne 0) { throw 'Touch interaction runner failed.' }
node docs/design-review/intercom/interaction-gallery.cjs
if ($LASTEXITCODE -ne 0) { throw 'Interaction gallery validation failed.' }
Remove-Item Env:INTERACTION_BATCH, Env:INTERACTION_WIDTHS -ErrorAction SilentlyContinue
```

The raw evidence remains at `.omx/artifacts/visual-ralph/intercom/interaction-pass/final` and `.omx/artifacts/visual-ralph/intercom/interaction-pass/touch`. The static review gallery is generated at `web/app/static/preview-audit/interaction-pass/index.html` and is served from `/preview-audit/interaction-pass/` on the preview.

## What must pass

The gallery refuses to publish unless all of these conditions are true:

- `expected.json`, `manifest.json`, and `receipt.json` exist and parse.
- The expected width list is unique and exactly matches the manifest.
- The manifest scenario constraints reconstruct exactly the expected eligible check matrix.
- The explicit skipped matrix exactly describes every scenario excluded by its viewport constraints.
- Every expected `${width}|${group}|${name}` key appears exactly once in the receipt, with the expected route, and there are no extra checks.
- Every check passed, recorded no browser runtime error, and includes interaction, request-attempt, and screenshot evidence.
- Every recorded non-GET attempt is marked intercepted, and fixture-response counts match their recorded contracts.
- Every screenshot is a unique existing PNG inside the final evidence directory.
- The expected, manifest, receipt-before, and receipt-after source hashes match, and the Git identity did not change during the run.
- The touch manifest contains exactly four scenarios at 390px and 834px; all eight name/width keys appear once, pass, match the same frontend source hash, intercept every recorded write, and have valid PNG evidence.

The gallery groups evidence by scenario family and viewport. It reports step, fixture-response, and intercepted-write counts. Where an exact name/group/width key failed in `red-*`, `bindings-fixed`, `green-second`, `matrix-first`, or `recovery-third`, it pairs that diagnostic state with the final passing screenshot. These earlier failures are diagnostic evidence and can include test-tooling problems as well as product defects. Only `final` and `touch` are acceptance evidence for this pass.

## Interaction boundary

The runner intercepts every non-GET request. A scenario can provide a controlled frontend contract for success, delay, cancellation, or service failure; otherwise the runner returns a service-unavailable response. This protects the disposable preview data while exercising the actual forms, controls, dialogs, menus, routing, keyboard paths, and recovery UI. It also means the pass does not prove that a backend accepted or persisted a write.

The runner blocks external navigation, uses isolated contexts, blocks service workers, masks password and explicitly sensitive fields in screenshots, records page errors, and rejects horizontal overflow after each interaction. The result applies only to the exact frontend source hash recorded in the final evidence. That hash covers `web/app/src`; it does not bind backend or test-harness files.

## Proof limits

This pass does not certify:

- database or filesystem immutability through an independent before/after snapshot;
- real backend mutation, queue, downloader, indexer, subtitle, or media-server integration;
- WebSocket or EventSource behavior beyond the runner's explicit controls;
- every data volume, language, long-text, or assistive-technology combination;
- an exhaustive tab-order, focus-ring, touch-target, or screen-reader audit;
- exact licensed Intercom font rendering or frame-by-frame animation timing;
- the installed HolyArrs app, the saved port-18745 preview, or a later combined integration build.

Screenshots demonstrate the recorded final browser states. They do not replace scenario assertions, source identity checks, accessibility testing, or validation against the eventual integrated artifact.
