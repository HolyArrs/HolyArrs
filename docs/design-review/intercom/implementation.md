# Intercom-derived actual application

The approved target is Intercom web, November 2025, Mobbin version `6e269961-628f-4a8b-b490-a34aea334a69`. The saved Air baseline remains on port 18745. This branch changes the actual Svelte application on port 18746, using a separate disposable backend on loopback 18747. It does not install over the running app.

## Implemented

- Shared neutral tokens, compact icon rail, contextual navigation, inset workspace, divider-led tables, restrained controls, typography and semantic status colors.
- Arrowless expandable Library with actual Movies, TV Shows and Music routes. Tablet and phone use an accessible media chooser; opening it does not navigate to Movies.
- Home, Library, requests, movie/show/artist and album details, collections, people, search, download queue/decisions/history, calendar and library tools use the same visual system.
- Grouped settings navigation and shared form styling; centered sign-in/invitation forms. Existing actions, permission/parts gates, URL-base routing and error handling remain.
- Protected setup, calendar feed and storage reads require the real permission and recent-proof conditions. An unavailable protected read stays unknown or requests account confirmation.
- Colored HolyArrs mark, keyboard focus, native dialog Escape/focus restoration, semantic status contrast and 44px rail targets remain intentional product requirements.

## Reference record

Local, untracked evidence is under `.omx/artifacts/visual-ralph/intercom/`:

- `full-export/`: all 563 reference PNGs.
- `flow-exports/`: 118 ordered flow ZIPs containing 681 entries. `manifest.json` records archive and entry hashes. Manifest SHA-256: `a643700e09951892740552b1ae3b0cc544b13d8b7012de0a663f45230eebebcc`.
- `acquisition.md`: acquisition scope and exclusions.
- `verification/`: isolated browser receipts and screenshots, including failed attempts retained as failures.

Every inspected flow exposed Screens and Prototype. No Video tab or timed recording was available. These archives preserve ordered states, not measured motion timing. Intercom's exact font files, easing, durations and responsive implementation were not exposed. The bundled Geist font and documented motion values are provisional; this branch is not certified as an exact pixel/font/motion clone. Source branding, customers and Mobbin footer are excluded.

## Verification boundaries

Strict Svelte diagnostics: 0 errors, 0 warnings. Unit suite: 32 files, 223 tests pass. Library navigation regression: 27/27 at desktop, tablet and phone, including Escape/focus return. Final route matrix: 72/72 screenshots and audits over 24 routes at 1920×1192, 834×1112 and 390×844, with zero axe violations, horizontal overflow, unexpected mutations, browser/HTTP/request errors or source drift. All sizes rendered the disposable movie detail successfully.

Canonical receipts are `verification/final-independent-desktop-3`, `verification/final-independent-tablet` and `verification/final-independent-phone` under the local evidence root. They bind app-tree SHA-256 `5ca72c878d29b83ee8a3197b8c732516872b12ef56521387137f509b777f782d`. The earlier two desktop attempts captured zero screens after a development-server stall during Svelte regeneration; they remain failed receipts. Restarting only the owned Vite process resolved it.

Each final pass separately reports blocked protected reads and the naming-example calculator POST. These excluded operations were not verified by the GET-only matrix. The full dark-theme matrix, password sign-in lifecycle, populated show/artist/provider states and full download/request lifecycle were not certified by these captures.

The disposable backend reuses a hash-pinned earlier executable. Current frontend renders are current-source evidence, but do not certify a newly built combined executable. A fresh bounded build and complete functional lifecycle suite require separate serial resource admission. Earlier Air/Gumloop receipts do not certify this source.

The seven made-up fixture titles are confined to the disposable database. Fake poster URLs were rejected by backend image-host validation, so fixture artwork can be absent. No host validation was weakened. No real provider, media, credentials or installed database was changed.

## Integration

Frontend ownership only; no API, backend or schema changes. Protected operations use existing account confirmation. Backend partners should integrate the exact frontend commit and run combined build/lifecycle checks on that result. Preserve existing API authorization; do not add credential reads to make setup indicators appear complete.

Preview configuration: `web/app/vite.intercom-preview.config.ts`. Session and OIDC cookies are namespaced so this preview does not replace another HolyArrs port's cookies. Launcher and isolation details are in `preview-18747.md`.

The scoped cleanup removed the proven unused `.nav-divider` style after the first frozen capture. Calendar's repeated proof transition is a documented small cleanup candidate, not a reason to introduce a cross-route abstraction.
