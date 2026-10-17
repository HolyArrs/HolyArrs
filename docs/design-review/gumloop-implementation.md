# Approved desktop workspace implementation

Luis approved the Gumloop direction after reviewing the Library, Downloads and Wishlist images. This supersedes the earlier unapproved concepts. The approved composition uses a near-black sidebar, charcoal canvas, compact Geist typography, quiet borders, blue actions and contextual details. The images are visual references, not functional specifications: implemented capabilities and the glossary govern product behavior and wording.

## Scope

Keep the existing Svelte application, API contracts, authorization, part visibility, safe mode, base paths and confirmations. No backend, database, real config, credentials or media changes. Use only real API data in product screens. Preserve light-theme support. No new dependencies.

## Implementation sequence

- [x] Shared tokens and controls: neutral surfaces, typography, blue actions, readable contrast, visible focus, restrained motion, fluid fields and dialogs.
- [x] Shell: grouped desktop sidebar and account footer; tablet rail; phone up to four primary destinations plus a complete modal navigation menu. Preserve capability/part filters and route specificity.
- [x] Media: real Library filters/grid/list, title detail, releases, shows/music, Search and people; all library operations remain reachable.
- [x] Operations: Home, Downloads, decision recovery, History, Calendar and Wishlist; retain exact consequences and request approval fields.
- [x] Settings and access: grouped section navigation that stays usable on narrow screens, forms, accounts, login/invites and first run.
- [ ] Isolated verification: exact-source typecheck, unit tests, vocabulary/build; headless desktop/tablet/phone/narrow screenshots, keyboard/menu/focus, axe, member/part/base-path behavior and affected functional tests. Heavy work is serial under the shared allocation log.
- [ ] Review and integration: fix review findings, commit only owned frontend/review files, hand off exact source plus fresh evidence and backend dependencies. Combined app must be reverified after backend integration.

## Preview and test boundaries

The earlier static review preview used port 18744. Verify current process ownership before reusing it. The real redesigned preview needs its own disposable config and freshly checked ports; do not proxy it to the installed app. Test data is disposable and explicit. Use isolated headless browser contexts and no personal profiles. Source checks cannot substitute for runtime verification.

Source verification: all 49 changed Svelte components compile without warnings; the vocabulary scan is clean. Independent source review fixes cover Library retry/focus recovery, Calendar feed errors and keyboard scrolling, coarse-pointer controls, section visibility and color contrast. The v5 exact-source build passed the full check (zero errors or warnings), 223 unit tests, frontend/glossary build and both disposable test binaries. The v6 browser run passed35 checks, including320/390/834/1440 reflow and phone navigation, and produced3 reviewed responsive screenshots. It found a movie-series error contrast issue and an ambiguous Settings test label; both are corrected. The complete functional chain and remaining redesign checks need a fresh run; partial results do not certify them.

## Responsive rules

Use the existing 768px boundary plus a 1024px desktop expansion. Phone controls are at least 44px, long text wraps, tables scroll within their own labelled regions, menus trap focus through native dialogs and return focus on close. Tablet rail icons retain accessible names. Desktop keeps useful information density and full-width workspace layouts.
