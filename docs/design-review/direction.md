# HolyArrs app redesign: three-direction review

Owner: redesign chat `01a146e0-0468-7e42-a35b-dab4c289493a`.
State: Luis rejected direction v1 (red/charcoal) and requested three materially different, polished iOS-clean designs. No direction is approved and no product rollout has started.
Source base: `4b660863ab8c73977e9e2a58aa24811a4d64660b`.
Own working copy: `worktrees/app-redesign-review`, branch `codex/app-redesign-review`.
Rejected v1 remains reviewable in commit `72076f66c59c192b639472b15e83b7ede65ddef8`.

## Scope and review boundary

Prompt 2 owns the implemented app frontend across desktop, tablet and phone. This milestone supplies a source-based screen inventory and three interactive review prototypes. Every title, download and address is a fictional disposable example. No implemented Svelte product, backend, API, database, installed container, credentials or real media has changed. Luis selects or revises a concrete direction before rollout. Ordinary implementation proceeds autonomously after that decision.

## Inventory

| Family | Implemented screens and functions | Design checks |
| --- | --- | --- |
| App shell | Capability-filtered navigation, parts, live connection state, safe mode, account, theme, sign out | Three alternative navigation models under review; preserve capability filtering, skip link and focus return |
| Home | Setup steps, downloads, searches, missing titles, events and health | Prioritize decisions and failures; setup replaces empty content; keep real status visible |
| Library | Kind/state filters, name search, poster/list view; held, turned down, subtitles, import, rename, file codecs, movie series and people | Group tools; retain direct actions; long names; large library; grid/list parity |
| Title | Movie/show/artist; versions, seasons, episodes, albums, releases, subtitles, files, edit/remove | Clear identity and current state; reveal detailed actions without losing them; preserve confirmation scope |
| Search and people | Movie/show/music search, add/ask dialogs, people and filmography | Keyboard results; appropriate role actions; loading/empty/error; selection return |
| Downloads | Active/finished/import rows, nested episodes, progress, remove and mark failed | Text state plus color; full release names; touch actions; removal consequences |
| Decisions and history | Retry, choose release, version, turn down, stop; filters, details, pagination | Recovery adjacent to cause; no destructive one-click shortcut |
| Calendar | Month/list, month controls, kind filters, phone feed | Desktop month; tablet readable cells; phone agenda; preserve feed function |
| Wishlist | Personal asks or approval inbox, filters, messages, approve/turn down | Member/admin variants; long names; badges; accessible decision dialog |
| Library operations | Import scan/adopt, rename plans, copy selection and bulk search | Staged preview/review/apply; preserve protection, warnings, cancellation and exact file scope |
| Settings | Account, people/users, linked apps, folders, pick rules, naming, music, subtitles, Wishlist, lists, schedules, parts, sign-in, alerts, health/logs, backups, migration, API | Searchable section index; persistent section selection on tablet/phone; credentials remain masked; separate test/save feedback |
| Access | Sign-in/OTP, SSO/Plex/proxy, invitation, first-run welcome | Actual available options only; forms, validation, password reveal, recovery and keyboard access |

Source inspection identifies a 232px desktop sidebar and a seven-item possible phone bar; settings section navigation disappears below 900px. These are source findings, not live-browser failure claims.

## Three independent concepts

| Concept | Desktop | Tablet | Phone | Interaction thesis |
| --- | --- | --- | --- | --- |
| 01 Library | Open warm-white canvas, large title, artwork grid, floating four-destination dock | Four-column grid, same dock; roomy side inspector | Two-column posters, horizontal groups, dock; bottom title sheet | Browse freely; keep your place while opening a title; full-page Menu index |
| 02 Focus | Sources / task list / detail in three panes | Two panes with navigation drawer | Grouped index -> list -> detail with Back, no bottom bar | Work through decisions and operational detail; arrow-key row navigation; compact artwork |
| 03 Cinema | Minimal horizontal navigation, feature artwork canvas, horizontal shelves | Artwork-led browse, shelves, full title page | Stacked shelves, compact three-destination capsule, full title page | Stories lead; activity is a temporary overlay rather than a permanent workspace |

These differ in information hierarchy, menus, content density, title transitions, activity placement and responsive structure. They are not recolours of v1. Each demonstrates Library/title, Downloads and settings navigation using the same review collection. Proposed visual groupings and appearance ideas are labelled as proposals, not existing product capabilities. The app is not a media player; Cinema offers title management, not playback.

## Reference study

Luis explicitly requested Mobbin. Nine high-resolution reference stills were inspected and retained only under `releases/app-redesign-20261016/references/`, with source URLs in `mobbin.json`. No personal browser or login was used; no reference UI asset ships in the prototypes. This is a still-screen study, not a recorded video.

- Library: [Apple Music](https://mobbin.com/screens/98264793-0d40-4f0f-93c9-c5ca32a2cb78): large native headings, open light surfaces, artwork rhythm and floating navigation.
- Focus: [Things 3](https://mobbin.com/screens/97952802-d1f3-4324-8a5b-fab1bc02c29e): grouped lists, quiet dividers, compact hierarchy and forward/back navigation.
- Cinema: [Apple TV](https://mobbin.com/screens/c58760da-44bb-489d-bb3b-a19c060dfa8f): feature artwork, horizontal shelves and restrained overlay controls.

Geist and the existing HolyArrs mark are reused. The six-panel artwork is original fictional review art. It is never injected into production flows. No dependency was added.

## Functional and input contracts

Retain session.can authorization, hidden-part handling, safe mode, base-path routing and live state in the eventual implementation. Actual download actions are Mark failed and Remove; there is no per-job pause. Keep the existing consequence confirmations. The downloader connection test uses saved configuration; preserve save-then-test order. A prototype explanation or local state change is not evidence of backend operation.

Use at least 44px primary touch controls, visible focus, native dialogs with accessible names and focus return, inert off-screen panes, keyboard alternatives and reduced motion. Preserve long-title access, narrow 320px layouts, filtering recovery, pagination/large-library operations and real loading/empty/error paths during product rollout. Prototype fixtures do not certify those product workflows.

## Preview and verification

Comparison: `http://10.10.25.25:18744` (Tailscale `http://100.87.70.86:18744`). Own static GET-only allowlist server; no API proxy. Port ownership verified before initial bind and exact owned process restart. Current server serves eight allowlisted resources (gallery, three concepts, artwork, mark and two bundled fonts).

Source checks: all inline scripts parse; HTTP responses match exact concept source; gallery/art/font/mark return 200; API/unlisted paths and POST are rejected; diff whitespace checks pass. Independent source review identified and fixed dialog naming, disclosure semantics, Music fixture content, download removal confirmation, inactive responsive panes and focus return. Source-only review cannot certify rendering or accessibility.

Headless browser, responsive/keyboard checks, axe and screenshots remain queued while the backend race diagnostic owns the sole heavy-work slot. Proposed next capture is one isolated headless Chromium, one page, no build/simulation, 180-second outer deadline. Do not silently launch it before explicit slot release. Capture exact final concept hashes at 1440/834/390/320 widths; test dialogs/navigation/search/settings; store fresh receipts. Never use checks against an older version to certify this set or the final product.

## Integration milestones

1. Three-direction review: this inventory and prototypes; Luis chooses/revises before product rollout.
2. Approved direction: implement tokens/shell and representative real routes; targeted role/part/base-path/keyboard verification; exact commit handoff.
3. Apply consistently across every inventory family; real state/large-library/error recovery checks; exact frontend commit and backend needs.
4. Backend integrates final source, reruns affected workflows/packages and preservation checks; its uninterrupted 24-hour endurance run remains last.

No API/schema change is currently requested. Any concrete need must name endpoint, fields, compatibility, owner and delivering commit in the shared interfaces file. Website may not use review prototypes as integrated product screenshots.
