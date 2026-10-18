# HolyArrs consolidated Intercom audit — 2026-10-17

## Scope and evidence

This report consolidates the current frontend audit for the preview application. The implementation was evaluated against the approved Intercom-inspired reference set and the repository-local audit plans. The reference set is a visual and interaction direction, not a complete specification: public Mobbin flow pages expose screenshots and flow steps, but no media/player timing evidence. Exact animation timing, easing, and playback behavior therefore remain unavailable where the reference does not expose them.

The current turn reports 226 automated checks across 33 files with 0 errors and 0 warnings. These are local evidence for the checked source and preview configuration; they do not certify the installed app, production data, backend providers, or a combined release artifact. No heavy build was run.

## Responsive and interaction scope

The planned responsive matrix covers 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, and 3840 CSS pixels, breakpoint-neighbor checks, dark/light themes, DPR variants, reduced motion, keyboard and touch paths, long text, empty states, large libraries, error recovery, household members, administrators, and complete request → approval → Library journeys. The planned visual set contains 744 responsive cases and remains **PENDING** until the final receipt is produced.

Native 200% desktop browser zoom has not been proven. The harness records a 640 CSS-pixel reflow proxy and labels it as such. Physical television/4K hardware behavior is unproven; 3840 CSS-pixel headless captures are only a browser viewport approximation.

## Navigation and surface direction

HolyArrs uses a breakpoint-specific navigation contract: mobile tabbar plus More dialog, tablet global rail, desktop global rail plus context sidebar, and a Settings-specific sidebar treatment. The recent sidebar surface correction is tracked by the responsive and sidebar batches; before/after evidence is retained under `.omx/artifacts/visual-ralph/intercom/` in the corresponding sidebar and responsive batch directories. The exact source commit and final source hash remain **PENDING FREEZE**.

The Library contract includes Movies, TV Shows, and Music through the expandable chooser, grid/list layouts, search, status/requester filters, removable filter chips, preview, title detail, collections, copies, held/turned-down releases, import, rename, and subtitle management.

## Interaction coverage

Existing interaction modules cover navigation, Library state retention, title dialogs, downloads, decision actions, history, Wishlist requests, approval, denial and withdrawal, search races, subtitle controls, settings forms, conditional fields, protected credential boundaries, confirmations, rejected saves, account/login/invite/two-factor/sign-out recovery, and welcome part selection. Real-save lifecycle references cover movie, show-season, and music-album approval, with the approved scope proven on title detail via season or album `keep_up_to_date` state.

## Remaining gaps

- Mobbin reference limits prevent exact timing claims for animations and media/player behavior.
- The 744-case responsive plan is pending its final receipt.
- Native browser zoom and physical TV behavior remain unverified.
- Metadata, downloader, indexer, artwork-host, and notification behavior is fixture-bounded in preview checks.
- A route-level nested album detail check is weaker than the artist-page album controls and should remain explicit in the final matrix.
- Source commit/hash freeze and final preview artifact identity are pending.
- No claim here certifies the installed app, saved preview, real credentials, real media, or a production deployment.

## Stop condition

This report becomes final only after the source commit/hash is frozen, the final responsive and interaction receipts are written, all declared matrix cases are accounted for, and the preview artifact is verified against that frozen source identity.
