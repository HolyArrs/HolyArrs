# Full preview audit

Preview audit pass complete, with explicit coverage limits below. Baseline: `2245f8f`, branch `codex/mobbin-product-review`, preview 18746 with disposable backend 18747. Saved Air preview 18745 and installed app 18730 are outside scope.

## Outcome and limits

Audit and repair the implemented media manager against the approved Intercom reference system. Preserve real functionality, expandable Library, filter/navigation memory, and preview fixtures. Keep before/after screenshots and bind results to the tested source. No credentials, real media, backend/schema changes, publication or installation.

Reference corpus: 563 desktop screenshots, 118 flow exports. Prototype sequences establish states and navigation, not exact timing, easing or font files. Responsive adaptations require usability checks; no unobserved Intercom layout is claimed exact.

## Execution

Use the visual-ralph comparison loop through native Codex tools; no tmux runtime or hook-state manipulation. Inventory routes and shared components, capture baseline, record findings, repair bounded batches, repeat captures and regressions, obtain independent review. Heavy builds require the shared serial allocation; ordinary source work proceeds independently.

Coverage must distinguish executed, failed, unavailable and not yet tested. Owner-role checks do not certify member permissions. The pinned preview backend does not certify a future combined build.

Evidence directory: `.omx/artifacts/visual-ralph/intercom/full-audit/`.

## Findings and repairs

| Finding | Repair | Evidence |
| --- | --- | --- |
| Library search, Clear filters and Add clipped out of desktop workspace | Keep search/action band in normal layout instead of negative absolute offset | Desktop before/after screenshots; visible-control interaction checks |
| Light status text failed contrast (4.15:1 red, 4.42:1 green on Library cards) | Darker semantic foregrounds; retain hue and surface treatment | Library and Downloads axe rechecks at 390/834/1440 |
| Phone request message columns squeezed content into narrow strips | Stack status/time and full-width message text; restore real mobile Review with close/focus return | Phone before/after captures and responsive interaction tests |
| Long phone synopsis sat beside poster in a narrow column | Full-width synopsis below media summary; accessible disclosure with 44px target | TV detail captures and disclosure tests |
| Music album rows squeezed title, metadata, state and controls into a desktop row | Stack album information above status and grouped 44px actions on phones | Fresh music screenshots and end-of-page check |
| History could show an older response after a fast filter change | Latest-request guard | Deferred out-of-order regression test |
| Reversed History dates silently yielded misleading results | Inline date validation connected to both inputs; no invalid API request | Unit and browser validation checks |
| Impossible URL dates appeared blank but reached the History query | Reject non-calendar dates when parsing shared links | Regression assertion failed before repair, passed after; leap day preserved |
| History title label excluded music | Include artists in label and default option | Actual History UI |
| Ultrawide download rows stretched across excessive width | Bound operational workspace to 1600px | 3440/3840 captures |

## Evidence interpretation

Initial exploration captures predate the final source freeze. One intermediate mobile repair matrix explicitly failed its source-stability check because an agent completed a repair during capture. It is retained as exploration, never a final pass. `audit-matrix.cjs` records commit plus a recursive app-source hash before and after every completed batch and fails on source change, page errors, overflow or axe violations.

The screenshot gallery is generated locally with `node docs/design-review/intercom/audit-gallery.cjs` at `/preview-audit/index.html`. Generated screenshots/HTML are ignored and are not product assets.

Automated accessibility scans do not establish visual polish or exhaustive keyboard access. Independent visual review identified mobile reflow problems despite zero automated findings. Member and signed-out rendering tests use browser-only response fixtures; they do not establish backend authorization. Ordinary loaded owner pages use the real disposable preview backend.

Still unavailable: exact Intercom font files and timed-motion evidence, native Intercom tablet/phone references, native TV behavior, production credentials/provider lifecycles and final combined backend/frontend build acceptance. Sensitive settings and authentication transitions must use redacted/test-only evidence without reading secrets or changing credentials. These gaps remain explicit rather than being counted as passes.

## Route and scenario scope

The source inventory contains 48 page components. The final read-only matrix covers 34 route states: Home; Library movie/show/artist filters; movie, TV, artist and album details; Wishlist; active downloads, decisions and History; Calendar; collections; held and turned-down releases; copies; missing subtitles; rename and import entry screens; People; Search; Settings overview, naming, search, parts, schedules, This app, music, Wishlist, Library folders, backups, moving media and users.

Nested collection/person pages, invitation/welcome completion, account changes, provider setup and credential-bearing settings are not certified by this matrix. Empty collections and inactive provider flows do not establish populated provider lifecycle correctness. Real delete, approve, import, download, credential and file operations were deliberately not executed: browser checks abort non-GET requests. Existing function handlers and permissions remain in place.

Separate browser-only fixtures check signed-out and member rendering without entering credentials or changing backend roles. Interaction checks cover request Review disclosure and focus return, synopsis expansion, invalid-date recovery, requester filter/clear behavior, approval-dialog Escape, Library error/retry, and existing filter/scroll/back/sidebar memory regressions.

Screen widths include 360, 390, 430, 720, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440 and 3840 CSS pixels across the audit. A 720px reflow capture is not a real browser zoom test, and desktop 4K viewport checks do not certify native television input or hardware. Axe scans cover WCAG A/AA tags through 2.2; they are not a complete accessibility conformance claim.

## Preview recovery

The Vite process stopped responding after Svelte generated-file synchronization. A clean restart of only the verified 18746 frontend restored responses. Backend 18747, preview fixtures, credentials, Air 18745 and installed 18730 were not restarted or modified. Failed navigation attempts are excluded from pass counts; the final matrix uses isolated pages and asserts document status, path, requested query parameters and a real app heading before capture.

## Final source and core results

- Frozen app-source SHA-256: `12bd5828d7bd4e1c3fa49157b63835d42abd7704915504eb4e94ffe175a7e00d`.
- `final-complete/receipt.json`: **102/102**, 34 route states × 390/834/1440, zero recorded overflow, page errors or axe violations; before/after source hashes match.
- `final-responsive/receipt.json`: **50/50**, five core routes at ten additional widths; zero recorded overflow, page errors or axe violations; same frozen source. Includes ultrawide 3440×1440 and 4K 3840×2160 viewports.
- Unit suite: **32 files, 225 tests passing**. Strict Svelte diagnostics: **0 errors, 0 warnings**.
- Independent architecture review: **CLEAR**; see `full-audit-architecture-verdict.json`.
- Independent core mobile still-image review: **91/100**, see `full-audit-final-verdict.json`. This is a qualitative pass, not a pixel-identical score. The earlier Messages-alignment concern was withdrawn after reopening the exact image; geometry checks confirm alignment.
- Fresh interaction receipts: keyboard Review/focus return and synopsis disclosure at360/390; invalid-date recovery at360/390/834/1440; six sidebar-memory and six title/back/refresh-memory cases; four filter/error/dialog state cases; three role-rendering fixtures; three loading-to-real-response cases. All pass.
- Clearance receipt: last album is scrolled into the visible viewport and measured above fixed navigation at360/390/834, with mobile actions at least44px. A separate settled page-end screenshot retains the following History section. Message headings align with row contents inside the card. These checks require the target to be onscreen, rather than accepting an already-scrolled-off row.
- Exact font identity and timed-motion parity remain unknown. No production build or combined backend integration acceptance is claimed.
