# Disposable request and download samples

`sample-activity.mjs` populates the isolated Intercom review backend on port 18747. It uses HolyArrs' public API to create real request, decision, search, release and download records. It never opens the database and never creates media files.

The tracked backend harness starts its private fake Torznab source on port 28767 and fake qBittorrent on port 28768. `HOLYARRS_E2E_INDEXER_NO_KEY=1` is scoped to that harness; normal end-to-end fakes continue to require their test key.

The request fixture removes 20 selected sample-catalog titles with `delete_files: false`, temporarily makes the verified-empty disposable library folders unavailable so owner auto-approval leaves genuine pending requests, and restores every folder in a `finally` block. It then approves eight requests, turns down six and leaves six pending.

The download fixture searches ten other catalog movies through the fake source and sends the accepted releases to the fake downloader. It assigns real downloader states for downloading, paused, waiting and failed rows. The searches and grabs also populate History with real activity events.

Run only while the disposable backend and fakes are active:

```powershell
node docs/design-review/intercom/sample-activity.mjs
```

The command prints request-state and download-phase counts. It is hard-locked to loopback ports 18747, 28767 and 28768 and refuses any library folder outside the Intercom preview's temporary directory.

## Verified populated preview

The live preview contains 20 requests (6 pending, 8 approved, 6 turned down) and 12 downloads (6 downloading, 2 paused, 2 waiting, 2 failed). Repeating the seeder preserved those counts. Existing synthetic fixtures remain alongside real catalog names.

Fifteen isolated headless checks covered Home, Wishlist, Downloads, Needs a decision and History at 1440, 834 and 390 pixels. All had no horizontal overflow, page errors or axe accessibility violations. Screenshots and the JSON receipt are under `.omx/artifacts/visual-ralph/intercom/activity-verification/`. Svelte diagnostics also passed with zero errors and warnings.

Needs a decision and completed-import history have no fabricated records. The running backend is the pinned earlier review executable; these preview checks do not certify a new combined application build.
