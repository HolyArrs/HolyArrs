# HolyArrs Intercom preview audit — 2026-10-17

## Acceptance scope and source

Preview only: http://10.10.25.25:18746 · http://100.87.70.86:18746.
Owned branch: `codex/mobbin-product-review`, working copy `worktrees/mobbin-product-review`.
Frozen product/test commit: `d3ac9e3c5fe8055fab8567e1d95e67bafa3c1308`.
Frontend SHA-256: `45144ef19d962c8cd785e124d352c7f00f42821ea359558b919415073e6f2928`.
The digest includes sorted relative paths and bytes under `web/app/src`. Later documentation-only commits do not change that digest; final smoke evidence must record the resulting checkout commit separately.

**Automated suite acceptance is determined by `coverage-executed-2026-10-17.json`, not this narrative or older reports.** A missing, partial, source-mismatched or failed receipt remains incomplete. Exact manifest case keys, counts, live frontend digest and screenshot existence are gated. Earlier partial/failed batches are excluded from final acceptance.

The installed app on 18730, saved Air preview on 18745 and other working copies are outside this audit. No original password, credential or key was read or reset. No real media or provider account was changed. The disposable backend on 18747 and its fake-provider identities remain separate from installed data. Heavy builds and external publishing were not run.

## Reference baseline and comparisons

See `reference-baseline-2026-10-17.md` for exact Mobbin version, screen/flow IDs, acquisition/capture dates, raster dimensions, mapping and classifications. Approved Intercom November 2025 records contain ordered images, not authenticated computed styles or timed playback. Public Sending a message, Filtering/sorting and Adding a team inbox flows were freshly inspected at 1440×1000 CSS px / DPR 1. They returned ordered stills without a timed video/player. Their source motion duration/easing cannot be certified.

Comparison captures use 1920×1192 CSS px / DPR 1 in both themes. The source images are 1920×1320 raster pixels including approximately 128px of Mobbin footer; their original CSS viewport, DPR and zoom are unknown. Equal raster sizes are not proof of equal capture scale. Each capture records local component geometry, typography, palette, borders, corners, shadows, padding and transition styles. The capture gate is explicitly separate from a visual-match verdict.

Confirmed correspondences are the restrained rail/context/workspace hierarchy, approximately 56px rail, compact controls, layered light/dark surfaces, subtle borders/dividers and list/inspector composition. Licensed Geist is an explicit font substitution; the authenticated reference font is not proven. Own line icons, HolyArrs branding, poster artwork, approval/download/subtitle states, role permissions, Library Movies/TV Shows/Music expansion and mobile/tablet navigation are intentional adaptations. Settings reading width and the bounded Library search field are retained deliberately. No irrelevant support-product controls were invented.

Fresh visual-ralph verdicts apply only to the named screenshots and reference files. They do not establish source font identity, animation timing or native Intercom mobile/tablet behavior. The source-matched scoped visual verdict is **95 / pass**, recorded in `consolidated-visual-verdict-2026-10-17.json`; this qualitative score is not a percentage of pixel equality. Prior verdicts are historical. Final audit completion additionally requires the frozen-hash evidence-gallery and final-checkout smoke receipt to pass all 14 cases.

## Frontend corrections

- Context sidebar uses the shared sidebar surface, separating it correctly from the workspace in light and dark themes.
- Home Activity stacks through 1100px. The health panel moves below the activity content through 1600px, preserving readable event text at tablet and compact-desktop sizes.
- Existing mobile Library, Requests and Downloads layouts, filter/requester counts, removable chips, saved search/view, scroll restoration and Last opened behavior are preserved and rechecked in the final scenarios.

Matching before/after captures retain their own source identities. Only the After images belong to the final frontend digest. The local gallery is generated only after required coverage gates clear:

- `/preview-audit/consolidated-20261017/` — searchable current captures.
- `/preview-audit/consolidated-20261017/before-after.html` — sidebar and Home changes at matching viewports.

## Coverage and verification contract

`coverage-inventory-2026-10-17.json/.md` records the inventory. `coverage-executed-2026-10-17.json/.md` records executed cases, source identities, receipt paths, states, role scenarios, fixtures and limits. Required final suites are:

| Suite | Required cases | Scope |
| --- | ---: | --- |
| Responsive | 744 | 48 route templates plus data/open-state variants, 62 cases at each of 12 CSS widths |
| Interactions | 243 | 83 declared scenarios with viewport eligibility at 360/834/1440; six intentional width skips |
| Touch | 8 | Actual Playwright tap actions at 390/834 |
| Breakpoint/environment | 164 | Six routes at 27 breakpoint-neighbor widths, 640px reflow proxy and DPR-2 detail |
| Comparison | 8 | Library, Requests, Downloads, Settings; light/dark; 1920×1192 |
| Motion | 6 | Normal/reduced motion at 390/834/1440, preference change, entry/exit/toast/font diagnostics and recordings |
| Genuine saves | 7 | Disposable-member and request/approval/denial/withdrawal persistence journeys |

Final executed matrix: all seven required suites cleared their count, source, screenshot and failure gates: 744 responsive, 243 interaction, 8 touch, 164 environment, 8 comparison, 6 motion and 7 genuine-save cases. The 1167-card evidence gallery contains the responsive, interaction, touch, environment and comparison captures. Motion recordings and genuine-save screenshots remain in their named artifact folders. Native zoom remains unavailable. The final-checkout 14-case smoke is separately recorded at `.omx/artifacts/visual-ralph/intercom/audit20261017/gallery/receipt.json`; its commit and status must be read directly rather than inferred from this report.

Responsive CSS widths: 360,390,430,768,834,1024,1280,1440,1920,2560,3440,3840. Heights are 900, 1440 above 2560, and 2160 at 3840, with full-page/bottom captures where applicable. DPR 1 is separate from the DPR-2 environment sample. The 640px case is a reflow proxy, not native zoom. Native 200% Chromium zoom was attempted with new disposable profiles. Chromium did not measurably apply the requested zoom preference, so its fresh receipt is explicitly unavailable with zero route checks and no native-zoom certification. Physical TVs, operating-system scaling and other browser engines are unverified.

Scenarios cover navigation and Back/Forward/refresh; Library filter/search/sort/count/chip persistence, scroll and Last opened; grids/lists; title/season/album/track views; subtitle search/save/sync/rejection; requests/approval/denial/withdrawal; queue/history/decisions; settings validation, conditional controls and rejected saves; role-sensitive menus; focus and keyboard paths; touch; repeat clicks and interrupted saves. Stress fixtures cover 1000 titles, empty/error/retry states, long/unbroken names, missing artwork and partial metadata. This is a finite declared matrix, not every possible combination.

Ordinary audit writes are intercepted in the browser. Those scenarios verify frontend behavior using schema-complete fixture responses and do not prove external provider delivery or backend authorization. Genuine persistence is reported separately below. No decorative or nonfunctional replacement UI was introduced.

Fresh static verification after the final product fix: Svelte check **0 errors, 0 warnings**; Vitest **226 unit tests passed across 33 files**. Tool syntax and Git whitespace checks are recorded separately. No production build or combined backend release is certified.

The initial final-source interaction batch `interaction-pass/consolidated-20261017` recorded 242 passes and one 834px development-server bootstrap failure: its SvelteKit client module failed to fetch before the collection page rendered. That failed receipt is retained and excluded from acceptance. The complete matrix is repeated as `interaction-pass/consolidated-final-20261017` with additional script-request diagnostics; only its final receipt is eligible for the executed report. No product behavior or frontend source was changed to suppress the failure.

The first boundary probe included non-rendered closed-menu boxes and scrollable page content under the fixed mobile tabbar in generic rectangle intersections. Its raw diagnostics remain in `audit20261017/env`. The corrected `env-v3` probe excludes closed-details, screen-reader-only and fully transparent boxes, records fixed-tabbar occlusion separately with labels/rectangles, and keeps every other overlap/outside finding as an acceptance blocker. Scrolling behind fixed navigation is intentional; final-row clearance is separately checked in the responsive bottom captures. The intermediate `env-v2` receipt also preserves transparent hover-only Preview buttons intersecting their poster links. These controls are excluded only while fully transparent at rest; visible hover/focus actions are exercised separately. Neither hidden-control geometry nor ordinary scroll occlusion is presented as a visual product fix.

## Genuine disposable saves and preserved scenarios

Run `3e873997-aeb3-4a60-ab2d-74da62aeed12` passed seven journeys with 14 guarded writes and 19 screenshots against the frozen digest. It created two disposable members through invite forms; persisted and restored a member alert destination; approved a movie and TV season into Library; persisted a denied movie note; withdrew a pending movie; and persisted message-read state. Request decision access was restricted for a household member. TV requested season and keep-up-to-date state were checked.

Cleanup verification confirms both new members disabled, both sessions revoked, original rows/media preserved and relevant process identities unchanged. Three newly approved/denied sample requests are retained as review scenarios. The complete receipt includes exact start/end times and process identities. These fake-provider-backed saves do not download real media.

Fresh music persistence is **unavailable**: all three fake artist identities already belong to preserved samples. Reusing/resetting those identities would invalidate the isolation boundary. A fourth fake artist identity is needed from the backend integration partner. Music sample statuses, artwork, album/track views and browser-fixture interactions are covered; they do not replace a new real-save lifecycle.

## Remaining limitations and backend handoff

1. Exact authenticated Intercom font metrics, source CSS tokens, source viewport/DPR/zoom, hover/focus computed values and timed motion/easing remain unavailable. Ordered flow stills prove sequence only. Local motion and reduced-motion recordings prove HolyArrs behavior only.
2. Native Intercom phone/tablet/TV references, physical 4K hardware, non-Chromium engines and complete native-zoom component/keyboard behavior are unverified.
3. Fresh genuine music lifecycle needs another disposable fake artist identity. Existing sample scenarios remain intact.
4. A settings-only shell can expose Music while `GET /api/v1/settings/music` requires `manage_library`; PUT uses settings permission. Resolve that authorization contract with the backend partner. Frontend role fixtures are not server-authorization certification.
5. External metadata, artwork availability, indexer/downloader/subtitle/notification delivery and filesystem operations remain fake-provider or fixture bounded. No homelab/installed-app validation or combined backend release is claimed.
6. Any unresolved findings or incomplete suites in the executed JSON remain open, even if older screenshots look correct. Final acceptance follows the frozen-source receipts, scoped visual verdict and final preview smoke.


## Music lifecycle follow-up

The fresh music persistence gap above was subsequently closed on the same preview using a fourth additive fake artist, without resetting earlier samples. See `music-lifecycle-2026-10-17.md` for the genuine selected-album request34 → approved Library artist166, preserved initial readiness failure, successful continuation, cleanup and exact committed-checkout verification. Other limitations in this report remain unchanged.
