# Three-direction review

Luis rejected v1 and requested three materially different polished, iOS-clean alternatives. Open the comparison at [home-network preview](http://10.10.25.25:18744) or [Tailscale preview](http://100.87.70.86:18744).

- `concepts/one.html`: Library, an open light canvas with floating dock and title inspector/sheet.
- `concepts/two.html`: Focus, a three-pane organiser becoming forward/back navigation on phone.
- `concepts/three.html`: Cinema, feature artwork and horizontal shelves with full title pages and temporary activity overlay.

All are review-only HTML. Titles, addresses, progress and local actions are fictional. No app API, real media or database is contacted. Proposed visual groupings and appearance controls are identified as proposals. No product rollout is approved. The source inventory and decision record are in `direction.md`; shared project files remain authoritative for coordination.

## Serve

Check port 18744 ownership before launch. Run `node docs/design-review/serve.mjs` from this isolated working copy. The server binds the network interface and serves eight allowlisted GET resources. It does not proxy the installed app. Keep the exact owned process available during review; stop only that process when retiring it.

## Verify

All three inline scripts parse, HTTP content matches the saved concept source, and method/path isolation checks pass. Independent source review led to dialog naming, focus restoration, inactive-pane, removal confirmation and fixture accuracy fixes. These source checks do not prove rendering or accessibility.

A first bounded headless run at commit `36b1b25` captured 21 screenshots and completed 24 checks with no script/network/overflow/interaction errors, but failed axe with contrast and selection-semantics findings. Visual review also found a desktop grid defect in Focus. Source fixes are applied; a fresh capture is queued behind the backend's heavy-work reservation in `plans/active/2026-10-16-shared-interfaces.md`. Do not launch until explicitly released and allocated. Use cached Playwright/axe, one isolated Chromium and one page, with a 180-second outer process-tree deadline:

```powershell
node docs/design-review/capture-concepts.cjs <absolute-cached-web-app-tooling-directory> <absolute-new-output-directory>
```

The script has a 150-second internal deadline, blocks other origins, captures 1440/834/390/320px and representative title/download/settings states, checks selected keyboard/dialog interactions and axe, records final source hashes, and closes its browser. Inspect `receipt.json`; nonzero exit or violations are failures. The output directory must be fresh. No personal profile, browser download or dependency install is needed. `capture.cjs` targets the rejected v1 and must not be used to certify these concepts.

The first capture applies only to `36b1b25`, not the source fixes. The new capture adds comparison-page audits and desktop/tablet pane geometry assertions. Revised source is not browser-certified yet. Automated checks also cannot establish screen-reader usability or final product acceptance. The approved design must subsequently be implemented on real routes with permission/part/base-path variants, long text, large libraries, loading/empty/error states and recovery checks, followed by combined-source verification.

## References and ownership

Mobbin study: [Apple Music](https://mobbin.com/screens/98264793-0d40-4f0f-93c9-c5ca32a2cb78), [Things 3](https://mobbin.com/screens/97952802-d1f3-4324-8a5b-fab1bc02c29e), [Apple TV](https://mobbin.com/screens/c58760da-44bb-489d-bb3b-a19c060dfa8f). Nine high-resolution reference stills and their URLs are retained outside this repo under `releases/app-redesign-20261016/references`. Reference screenshots are not shipped assets. The six-title artwork sheet is original fictional review art; Geist and the HolyArrs mark come from existing project assets, with font licenses retained.

Own branch `codex/app-redesign-review`, base `4b660863ab8c73977e9e2a58aa24811a4d64660b`. Rejected v1 commit `72076f66c59c192b639472b15e83b7ede65ddef8`. No product, backend, API, schema, installed app, credential, media, release or publishing change is included. Backend dependencies: none currently. Luis's direction review precedes autonomous implementation across the real app.
