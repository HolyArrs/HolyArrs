# Intercom redesign responsive capture plan

Status: prepared, not executed. The capture waits for a source freeze so the evidence represents one stable implementation state.

## Coverage

The harness captures 24 implemented routes at three fixed viewports, for 72 screenshots and 72 matching layout and accessibility audits.

| Area | Routes |
| --- | --- |
| Home | `/` |
| Library | `/library?kind=movie`, `/library/1`, `/library/collections` |
| Library management | `/library/held`, `/library/turned-down`, `/library/copies`, `/library/missing-subtitles`, `/library/rename`, `/library/import` |
| People and search | `/people`, `/search` |
| Requests | `/wishlist` |
| Downloads | `/downloads`, `/downloads/decide`, `/downloads/history` |
| Calendar | `/calendar` |
| Settings | `/settings`, `/settings/naming`, `/settings/search`, `/settings/parts`, `/settings/schedules`, `/settings/this-app` |
| Account | `/account` |

Viewports:

- Desktop: 1920 × 1192
- Tablet: 834 × 1112
- Phone: 390 × 844

`/library/1` is the only approved fixture detail route. The plan does not invent IDs, tokens, query data, or account state.

## Deliberate exclusions

Parameterized routes are excluded unless a safe fixture is explicitly approved: `/library/[id]/albums/[album]`, `/library/collections/[id]`, `/people/[id]`, and `/settings/sources/[id]`.

Authentication and setup routes are excluded because this pass neither establishes nor changes identity: `/login`, `/welcome`, and `/invite/[token]`.

Settings that can display credentials, keys, provider configuration, user data, local paths, or operational logs are not direct screenshot targets. This includes library folders, sources, subtitles, downloaders, outside lists, alerts, media servers, TMDB, sign-in, users, API, logs, and settings account pages. The personal `/account` surface remains in scope, while its session and API-key requests are blocked before a response can be read.

The representative settings sample covers the overview, naming/files, search, parts, schedules, and this-app surfaces without navigating to credential-bearing child pages.

## Isolation and data handling

- One isolated headless Chromium browser, context, and page run sequentially. The harness accepts `HOLYARRS_CAPTURE_VIEWPORT=desktop|tablet|phone` so each viewport can run as a separate pass under the 60-second cap.
- Service workers are blocked and reduced motion is forced.
- Only GET requests are allowed. Every other method is aborted and recorded.
- Allowed network origins are the disposable app preview and public TMDB images.
- Sensitive settings, credential, key, provider, user, and session API requests are aborted before response.
- Response bodies are never stored.
- Recorded URLs remove all query parameters except the allowlisted library `kind` value.
- Console text redacts credential-like fields. Page error stacks and axe HTML fragments are not stored.
- No forms, searches, buttons, downloads, mutations, credentials, personal browser profiles, or real-media operations are used.

The naming page asks its preview calculator for a result with `POST /api/v1/settings/naming/example` during initial render. The harness still aborts that request. It records the event as an intentional capture gap, separately from unexpected non-GET attempts, because the GET-only pass cannot certify the calculator preview.

## Stable-source evidence

Before and after capture, the harness records:

- Git commit.
- Dirty-state hash for `web/app`.
- Full `web/app` tree hash and file count.
- Capture-script hash.

The receipt fails if the app source changes during the run. Each screenshot is hashed and tied to its route and viewport.

## Checks and stop conditions

Every route receives a viewport screenshot, horizontal-overflow audit, and axe WCAG 2 A/AA, 2.1 AA, and 2.2 AA scan. Console errors, page errors, HTTP errors, request failures, blocked origins, attempted non-GET requests, authentication redirects, and the hard run deadline are recorded.

The fixture route `/library/1` also records a render assertion: it must have a primary heading and must not show the missing-title or error state.

The final verification passes only when all 72 screenshots and audits complete against one unchanged source identity and the recorded failure classes are empty. Intentionally blocked sensitive requests are reported separately and do not expose their responses.

The browser capture must not start while implementation edits or a heavy shared-machine build/test are active. The runner should check resource ownership immediately before launch and keep each bounded capture pass to at most 60 seconds when the final execution is authorized.

## Targeted Library navigation receipt

`library-nav-regression.cjs` provides a bounded, GET-only interaction check for the responsive Library navigation without running the broad route matrix. Its canonical source-freeze receipt is `.omx/artifacts/visual-ralph/intercom/verification/library-nav-source-freeze-3/receipt.json`.

That run passed 27 of 27 checks at 390, 834, and 1440 pixels. It verified the native mobile/tablet chooser, real media-kind filter URLs, close-on-selection, Escape and focus return, the desktop context toggle and inert collapsed children, and visible skip-link contrast in both themes. It recorded no non-GET attempts, blocked origins, HTTP errors, console errors, page errors, or source changes. Measured skip-link contrast was 16.32:1 in light mode and 16.28:1 in dark mode.
