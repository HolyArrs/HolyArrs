# Intercom preview sample library

This catalog is for the disposable Intercom review on ports 18746 and 18747. It adds real public names and imagery to the existing preview through HolyArrs' movie, show, and artist add APIs. It never opens or edits a database directly.

The generated catalog contains 55 movies, 60 shows, and 30 artists. Artists include up to four real albums. Names and imagery come from public metadata sources recorded in `sample-catalog.json`. Movie and show provider IDs used by the local metadata fakes may be synthetic when a public source does not supply every ID HolyArrs expects. Episode lists are intentionally short, and artist country or type is left empty when the source does not provide it.

The preview contains no real media files. Every add request turns automatic refresh off, starts no search, and uses no real indexer, downloader, account, API key, or media server.

## Refresh the catalog

Run from the repository root:

```powershell
node docs/design-review/intercom/sample-catalog.mjs
```

The checked movie candidate file lives in the review worktree's ignored `.omx` evidence folder. The generator fails closed when it is missing or contains fewer than 50 verified movies.

## Resume the known disposable runtime and seed it

Verify port ownership first, then stop only this review's known backend harness. Never stop an unrelated port owner. Run from the repository root:

```powershell
$env:HOLYARRS_PREVIEW_SEED = '1'
node .omx/artifacts/visual-ralph/intercom/resume-preview.mjs
```

The resume harness is pinned to the existing disposable runtime. It loads the catalog into the local metadata fakes before the backend starts, then persists titles through the public APIs. Conflicts are accepted so repeating the command is safe. Sample seeding is hard-locked to `http://127.0.0.1:18747`; it refuses any other address.

The fresh-runtime launcher loads sample metadata but never seeds or creates an owner. After normal preview setup and enabling local-network owner access in that disposable instance, seed the still-running preview separately:

```powershell
node docs/design-review/intercom/sample-preview.mjs
```

The seeder has no destination override and rejects every target except the disposable loopback backend.

## Start the frontend

Restart the frontend after regenerating the catalog because the preview-only artwork map is read at Vite startup:

```powershell
npm --prefix web/app run dev -- --config vite.intercom-preview.config.ts
```

`vite.intercom-preview.config.ts` enriches only title and request list/detail GET responses on this preview. It maps a catalog picture by media kind and exact title name. Production responses, backend storage, and the installed app are unchanged.

## Verification

The live disposable library contains 152 titles: 58 movies, 62 shows and 32 artists, including the seven earlier fixtures. The new artists have 50 albums. Six isolated headless checks covered all three Library sections at desktop and phone widths: real artwork loaded, no horizontal overflow, and no page errors. Screenshots and the receipt are in `.omx/artifacts/visual-ralph/intercom/sample-verification/`.

Fresh browser checks of the network preview and its login page reached the app without a password prompt through the existing local-network access setting. This test account is independent of the installed owner; no installed credential was read, copied or reset.

These checks cover the sample preview only. They do not replace the pending combined application build and lifecycle acceptance.
