# Intercom coverage inventory — 2026-10-17

The implemented app contains 48 route templates. Coverage is organized around navigation, Library, activity, settings, and authentication. Existing interaction modules cover the major buttons, forms, menus, dialogs, drawers, validation, rejected saves, confirmation cancellation, keyboard paths, touch paths, and request approval journeys.

The responsive target matrix is 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, and 3840px, with 200% zoom, DPR variants, reduced motion, keyboard and touch checks. Stress states include long text, empty data, large libraries, rejected/error responses, household members, administrators, and full request → approval → Library journeys.

Navigation changes by breakpoint: mobile uses the tabbar and More dialog; tablet uses the global rail; desktop uses the global rail plus context sidebar; Settings has its own context behavior. Library is a chooser with Movies, TV Shows, and Music and has grid/list, search, status/requester filters, chips, preview, title detail, collections, copies, held/turned-down, import, rename, and subtitle management.

Activity coverage includes downloads, decision queue, history, Wishlist requests, messages, and approval/turn-down/withdraw flows. Settings coverage includes validation, conditional controls, destructive confirmations, protected credentials, rejected-save recovery, schedules, backups, and system controls. Auth coverage includes login recovery, two-factor, invite, account, sign-out, and welcome part selection.

Known boundaries are fixture-bounded metadata/download providers, the lack of one receipt proving every route at every matrix point, and weaker direct coverage for the nested album route (`/library/[id]/albums/[album]`) compared with artist-page album controls. Keep successful fixtures schema-complete and intercept all writes in audit runs.
