# Disposable completed imports and decisions

`sample-completion.mjs` creates two genuine completed movie imports and verifies the two existing Needs a decision examples. A completed sample counts only when the real import record is `done`, its result contains `placed`, the title's slot is in the library and the placed file exists in that title's disposable library folder.

The script is hard-locked to backend port 18747, fake qBittorrent port 28768 and the private `holyarrs-intercom-preview-18747` runtime under the operating system temp directory. Its `.mkv` fixtures contain one short text line and are never real media. File creation is exclusive; an existing file is reused only when its contents exactly match the expected fixture.

The preview's fake-file checks are disabled through the real settings API only while these tiny fixtures pass through the importer. The original settings object is restored in a `finally` block and checked after the run. Existing completed imports are verified and reused on repeat runs.

Run it only while the disposable preview harness is active:

```powershell
node docs/design-review/intercom/sample-completion.mjs
```

The JSON result lists the completed title, import record, download record and relative placed file. It also reports the final download phase counts, Needs a decision count and settings restoration check.

## Verified preview

Godzilla vs. Kong (import 7, download 15) and Oppenheimer (import 8, download 16) have successful placed imports and synthetic files in the disposable Library. After We Collided and Below Zero have genuine import-failure decisions. The normal fake-file checks were restored to enabled.

Nine isolated headless checks covered Downloads, Needs a decision and History at 1440, 834 and 390 pixels, with no overflow, page errors or axe accessibility violations. The receipt requires at least two placed import outcomes in addition to completed download rows. Screenshots and receipt: `.omx/artifacts/visual-ralph/intercom/completion-verification/`.

The earlier tiny-file attempts for Driftwood Moon and The Flash remain in History as rejected imports; the pinned backend projects their four terminal downloader records as Done even without placed files. This mismatch is documented in the shared backend handoff and is not counted as successful fixture completion. The existing preview binary and these checks do not certify a newly combined application build.
