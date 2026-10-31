# Intercom coverage inventory — 2026-10-17

The implemented app contains 48 route templates. Coverage is organized around navigation, Library, activity, settings, and authentication. Existing interaction modules cover the major buttons, forms, menus, dialogs, drawers, validation, rejected saves, confirmation cancellation, keyboard paths, touch paths, and request approval journeys.

The responsive target matrix is 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, and 3840px, with 200% zoom, DPR variants, reduced motion, keyboard and touch checks. Stress states include long text, empty data, large libraries, rejected/error responses, household members, administrators, and full request → approval → Library journeys.

Navigation changes by breakpoint: mobile uses the tabbar and More dialog; tablet uses the global rail; desktop uses the global rail plus context sidebar; Settings has its own context behavior. Library is a chooser with Movies, TV Shows, and Music and has grid/list, search, status/requester filters, chips, preview, title detail, collections, copies, held/turned-down, import, rename, and subtitle management.

Event and queue coverage includes downloads, decision queue, history, Requests requests, messages, and approval/turn-down/withdraw flows. Settings coverage includes validation, conditional controls, destructive confirmations, protected credentials, rejected-save recovery, schedules, backups, and system controls. Auth coverage includes login recovery, two-factor, invite, account, sign-out, and welcome part selection.

This file describes the inventory and target, not acceptance results. The executed matrix and exact source identities are in `coverage-executed-2026-10-17.json` and its Markdown companion. The final responsive runner enumerates all 48 route templates plus data/open-state variants. The interaction runner declares 83 scenarios, including a dedicated nested album/track scenario, subtitle search/save/sync/recovery, and role/navigation interruption checks. Only the final receipts establish which cases passed.

Provider operations remain fixture-bounded. Browser-only write fixtures are separate from genuine disposable-backend saves. Fresh music approval persistence needs another fake artist identity because all three existing fake-provider identities belong to preserved samples; existing music views and browser-only music interactions do not close that gap. Keep successful fixtures schema-complete and intercept all writes in ordinary audit runs.
