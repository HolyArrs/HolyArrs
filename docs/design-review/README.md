# App direction review

Review-only HTML for Luis's desktop, tablet and phone design decision. This does not change the Svelte app or its real data. All titles, downloads, activity, addresses and account fields are explicitly fictional. Actions explain the retained product behavior or update this preview tab only.

## View

After checking port 18744 is free, run from this working copy:

```powershell
node docs/design-review/serve.mjs
```

Open `http://10.10.25.25:18744` on the home network or `http://100.87.70.86:18744` over Tailscale. The server is a standalone static preview with four allowlisted GET assets. It does not proxy or contact the installed app. Keep its process while the direction review is open; the redesign owner stops that exact process when the preview is retired.

Use the Review screens links for Library, title detail, Downloads and settings. Library includes state examples, filtering, name search and poster/list layouts. Theme changes are local to the tab. More opens the phone menu. Home, Search, Wishlist, Calendar and History labels in the shell are context for the proposed navigation; the review does not claim those screens are implemented here.

## Verify

Do not launch capture while another chat holds the shared heavy-work slot. Reserve a bounded allocation in the shared interfaces file first. Reuse cached Playwright and axe; no dependency install, browser download or personal profile is needed.

```powershell
node docs/design-review/capture.cjs <absolute-cached-web-app-tooling-directory> <absolute-fresh-capture-directory>
```

The script runs one isolated headless Chromium with one page, permits only the review origin, closes its browser, and records screenshot dimensions, artifact hash, errors, automated accessibility findings and interaction assertions in `receipt.json`. It has a 150-second internal deadline; use an external process-tree deadline of 180 seconds too. Output directories should be new for every attempt so failed evidence remains intact.

Screens are exercised at 1440, 834, 390 and 320px. This proves only the review artifact. Automated checks do not establish assistive technology usability or certify the eventual integrated app. Product builds, route regressions, real artwork, role/part variants and final combined acceptance belong to the later implementation milestones.

## Ownership

Branch: `codex/app-redesign-review`. Base: `4b660863ab8c73977e9e2a58aa24811a4d64660b`. Only `docs/design-review` is changed at this milestone. Existing bundled fonts and the existing HolyArrs mark are reused, with font licenses retained. No new dependency, artwork download, backend/API/schema change or installed-app action is involved.

The shared plan is `plans/active/2026-10-16-app-redesign-direction.md` at the project root. Integration notes are in `plans/active/2026-10-16-shared-interfaces.md`. Stop at Luis's direction review; after approval, implement actual frontend screens consistently and provide exact source commits to the backend chat.
