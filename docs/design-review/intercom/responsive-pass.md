# Responsive pass after typography and motion

Baseline: `38c467483531161df3d42a93c7cf0b4c3b4e6ce6`, branch `codex/mobbin-product-review`. Only preview 18746 is owned by this pass. Installed app 18730, saved Air preview 18745, disposable backend 18747, credentials and data stay unchanged.

CODE_STYLE_POLICY=match_local_patterns_small_diff

Plan: inventory every implemented page; capture all route templates at 360, 834, 1440 and 3440px with the new type scale; inspect top, full-page or bottom captures and layout diagnostics; review independent visual contact sheets; repair the owning components only; repeat affected interactions and the final route matrix against stable source. Add narrow breakpoint and reduced-motion checks. Use one isolated headless runner at a time, block non-GET requests, redact secret fields, and keep all screenshots and receipts locally. No new dependencies or heavy production build.

Parameterized and authentication pages without persisted disposable records use explicitly labeled browser-only response fixtures to inspect the implemented rendering. Such captures prove layout, not backend behavior. Actual backend responses are used for ordinary pages. Native device input and every possible backend state are outside screenshot proof.

## Repairs

The initial visual sweep found problems that zero document overflow alone did not detect. Download and history messages, Parts rows, application-health rows and the import safety notice retained cramped desktop columns on phones. Download subnavigation clipped History at 360px. These now reflow into full-width text and action rows while preserving the existing desktop layouts and actions.

Nested-page review also found touching person-role buttons, a protected-copy badge squeezing collection titles and blank charts for Sources with no recorded calls. Role controls now use touch-sized segmented buttons, the person's biography spans the phone width, protected-copy badges move below the title, and empty charts explain the missing data. API address/copy controls have a dedicated row. Filmography years retain a visible space after the title.

A second visual review found download status chips still competing with long active titles. They now sit beneath the title on phones. The first full-matrix iteration was stopped at its verified owned runner, its child browser processes were closed, and its 137 captures remain in `iteration-one/` as intermediate evidence.

Desktop and ultrawide review then found two visual defects despite the next 248 automated checks passing: Rename's zero-padding card pressed the helper text against its border, and the password-settings fieldset retained its browser border. Rename now inherits the standard responsive card padding; the password group uses the existing borderless form spacing. Its handlers and disabled state are unchanged. The twelve focused captures at 320, 360, 834, 1440, 3440 and 3840px pass. The previous complete matrix remains in `iteration-two/`; it is not the final repaired-source proof.

No API calls, data writes, filtering, request lifecycle, Library expansion or view-memory behavior were replaced. Thirteen app files contain the repairs. Existing tokens and native controls are reused; there are no new dependencies, font files, backend changes or alternate production data paths.

## Evidence scope and reproduction

`audit-responsive.cjs` discovers 48 implemented route templates, adds four Library variants and ten open-form/filter/list states. The final matrix covers 62 route/state variants at 360, 834, 1440 and 3440px. Every context is fresh and headless. All non-GET requests are blocked. Isolated response fixtures supply unavailable nested records, signed-out/invite views, one user editor and the outside-list form; the manifest and gallery identify these as browser-only fixtures.

The runner asserts HTTP success, expected routing, a rendered application heading, open-dialog focus containment, document overflow, out-of-viewport text/controls, unintentional clipping, sibling control/text overlap, browser errors and WCAG-tagged axe findings. It retains full-page, page-end and open-dialog viewport screenshots. Intentional bounded table scrolling and explicitly marked ellipsis/clamping are excluded from accidental clipping checks. Automated geometry cannot determine whether a layout looks cramped; independent visual review is a separate gate.

The anonymous headless context receives HTTP 403 from privileged Users endpoints; the user editor and outside-list form therefore use marked browser-only fixtures. No credential is entered or changed. These fixtures establish layout coverage, not privileged API acceptance. Dedicated dialog viewports and focus measurements distinguish an offscreen skip-link fragment in stitched full-page captures from the normal live viewport: focus stays inside the native dialog and the skip link remains at -80px, unfocused.

```powershell
$env:RESPONSIVE_BATCH='final'
$env:RESPONSIVE_WIDTHS='360,834,1440,3440'
node docs/design-review/intercom/audit-responsive.cjs
$env:RESPONSIVE_GALLERY_BATCH='baseline'
node docs/design-review/intercom/responsive-gallery.cjs
$env:RESPONSIVE_GALLERY_BATCH='final'
node docs/design-review/intercom/responsive-gallery.cjs
```

Additional samples use the same runner, with `RESPONSIVE_ROUTES` limiting the set:

```powershell
$env:RESPONSIVE_BATCH='breakpoints'
$env:RESPONSIVE_WIDTHS='320,768,1024,3840'
$env:RESPONSIVE_ROUTES='/library?kind=movie,/library/76,/downloads,/downloads/history,/settings/parts,/people/900001,/settings/sources#add,/settings/users#edit-user'
node docs/design-review/intercom/audit-responsive.cjs
$env:RESPONSIVE_BATCH='dark'
$env:RESPONSIVE_WIDTHS='360,1440'
$env:RESPONSIVE_ROUTES='/library?kind=movie,/downloads,/downloads/history,/people/900001'
$env:RESPONSIVE_THEME='dark'
node docs/design-review/intercom/audit-responsive.cjs
$env:MOTION_BATCH='responsive-final'
$env:MOTION_ASSERT='1'
node docs/design-review/intercom/audit-motion.cjs
```

The 3840px samples use a 2160px viewport height. Other breakpoint samples use 900px. The main 3440px matrix uses 1440px height. Dark theme is a targeted sample, not a second complete route matrix. Clear `RESPONSIVE_ROUTES` and `RESPONSIVE_THEME` before rerunning the complete light-theme matrix.

The gallery validates manifest coverage and unique capture keys before describing the final matrix as complete. Raw screenshots and receipts live under local ignored `.omx/artifacts/visual-ralph/intercom/responsive-pass/`. The gallery copies only HolyArrs PNGs into ignored preview static files. Initial baseline collection/person captures accidentally intercepted the HTML document; corrected `states-before` captures supersede those comparisons. Initial closed-disclosure geometry false positives are retained as intermediate evidence. Neither is counted as a product defect or final pass.

## Scoped cleanup review

Plan: inspect only these thirteen app diffs and the new audit/gallery tools for duplicate layout rules, dead scaffolding, masking fallbacks and needless abstractions. Preserve existing behavior and the before/after screenshot evidence. Run strict Svelte diagnostics, existing unit tests and the focused browser regressions before removing any candidate. Do not broaden into unrelated cleanup.

Result: existing tokens and local page patterns are reused; no behavior-masking fallback or new abstraction was added. Independent code review requested stronger evidence-tooling gates and correct Library inventory metadata. Those fixes were applied and independently approved. The scoped app repairs remain small; no additional cleanup candidate justified a behavior change.

## Results

App source SHA256: `e9f30b71717cacee71fc51443ba8a18f0b1ad3e7b483443c99f66566f3daf526`.

Strict Svelte diagnostics on this source: 0 errors, 0 warnings. Existing unit suite: 32 files, 225 tests passed. Scoped code review approved all thirteen app diffs and the audit gates. The last two card/form repairs passed twelve focused captures from 320px through 3840px; independent visual review found both defects corrected with no new issues.

The complete final matrix passed all 248 checks: 62 route/state variants at each of 360, 834, 1440 and 3440px. Independent final-source visual reviews inspected all 248 captures and found no remaining responsive defects. The gallery reports complete, unique coverage and stable before/after source hashes. No horizontal document overflow, unintended clipping, out-of-viewport elements, sibling overlaps, axe violations or browser errors were recorded.

Additional final-source checks passed: twelve focused Rename/password captures at 320/360/834/1440/3440/3840px; 32 breakpoint samples at 320/768/1024/3840px, including open source/user forms; and eight dark-theme samples at 360/1440px. These bring responsive capture contexts to 300. Earlier receipts under `iteration-one/` and `iteration-two/` remain intermediate evidence and do not certify the final edits.

Six fresh motion contexts passed at 390/834/1440px with normal and reduced motion. They cover Library expansion, dialog entry, immediate native close, Escape/focus return, toast entry/dismissal, live reduced-motion changes, safe handling of an invalid duration token and the current type roles. Source hashes remained stable; no page errors were recorded. Screenshots, motion samples and recordings are retained under `typography-motion/responsive-final/`.

Review the [before/after gallery](http://10.10.25.25:18746/preview-audit/responsive-pass/final/index.html), [breakpoint samples](http://10.10.25.25:18746/preview-audit/responsive-pass/breakpoints/index.html), [dark samples](http://10.10.25.25:18746/preview-audit/responsive-pass/dark/index.html) and [focused card/form repairs](http://10.10.25.25:18746/preview-audit/responsive-pass/card-form-repairs/index.html). Tailscale uses the same paths at `http://100.87.70.86:18746`. These are disposable preview assets, not shipped reference material.

The app has no separate lint script; `npm run check` provides strict Svelte/TypeScript diagnostics. No heavy production build was run. Delivered-commit smoke and gallery checks are recorded separately in the shared integration handoff after committing, so this report does not contain a self-referential commit identifier.

## Integration boundary

Only this working copy's preview frontend is changed. Port ownership was checked before stopping and restarting its Vite process for generated-file synchronization. Disposable backend 18747, saved Air preview 18745 and installed app 18730 retained their existing processes. No password, configuration, sample data, media file, API handler, database schema or backend executable was changed. The backend executable remains the previously pinned SHA256 `D1FA075167517E84FD834EC112C211EE84F27AA8BE8191E07DB9AE62E316C914`.

Backend dependencies for these repairs: none. Apply this frontend commit through the normal integration lane, then run the combined app's checks on that exact source and backend. This pass does not certify a production bundle, combined backend integration, provider mutations, native phone/TV input, every possible dataset, or exact Intercom font and motion parity. The font/timing evidence gap remains documented in `type-motion.md`.
