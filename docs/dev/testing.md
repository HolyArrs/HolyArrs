---
title: Testing
description: Run the checks CI runs, then the end-to-end tests for the area you changed.
level: Setup
sidebar: { order: 5 }
---

Check your change on your machine before you send it.

## Before you start

- Finish [Setup](setup.md). The Go tests serve the built web app, so build it first.
- Install the test browser once:

  ```sh
  npx --prefix app/frontend playwright install chromium
  ```

## Run the everyday checks

1. Build and vet the Go code:

   ```sh
   go build ./...
   go vet ./...
   ```

2. Run the Go tests:

   ```sh
   go test ./...
   ```

3. Check and test the web app:

   ```sh
   npm --prefix app/frontend run check
   npm --prefix app/frontend run lint
   npm --prefix app/frontend run test
   ```

4. Run the layer and file-layout rules:

   ```sh
   go test -tags structure ./app/backend/tools/structurecheck
   ```

You should see no failure. CI runs the same commands, the Go ones on `amd64` and `arm64`, plus one run with the race detector (`go test -race ./...`).

:::note
The full `go test ./...` runs long. Use `make test`, which sets a 25 min timeout. Go's default is 10 min.
:::

## Run the end-to-end tests

End-to-end tests drive a real browser against a real program. Run the project for the area you touched:

```sh
npm --prefix app/frontend run e2e -- --project=movies
```

The run starts its own HolyArrs on a fresh config folder. The `setup` project makes the owner first, and most other projects wait for it. Run the full suite with no `--project` before a release.

## Choices

### Which projects to run

| You changed | Run |
|---|---|
| Movies | `movies` |
| Shows | `shows` |
| Requests | `requests`, `lists`, `reports` |
| Downloads | `sources`, `downloaders`, `downloads` |
| Sign-in and users | `accounts`, `kid` |
| Health and schedules | `schedules`, `controls` |
| Page layout | `desktop`, `phone`, `a11y` |

Projects are named in `app/frontend/playwright.config.ts`. A project that depends on another runs it first.

### Where a new test goes

| Kind | Where |
|---|---|
| Go | Beside the file it tests |
| Web unit | Beside its file, as `name.test.ts` |
| End-to-end | `app/tests/<module>/` |

Never call a real outside service. Add a stand-in under `app/tests/fakes`. A test that passes only on your machine is broken.

## If it does not work

**Symptom.** A structure check fails.
**Cause.** The message names the file and the rule you broke.
**Fix.** Move the code, or pass plain data in from the higher module. See [Modules](modules.md#the-checks).

**Symptom.** End-to-end tests cannot start.
**Cause.** The test browser is not installed.
**Fix.** Run the `playwright install` line above.

**Symptom.** The Go tests fail on missing pages.
**Cause.** The web app is not built.
**Fix.** Run `npm --prefix app/frontend run build`.

## For experts

<details class="ha-more"><summary>Build tags</summary>

A test with a tag skips in a plain `go test ./...`. Pass `-tags <tag>` to run it.

| Tag | Holds |
|---|---|
| `structure` | The layer, file-layout, SQL-placement and comment rules in `tools/structurecheck` |
| `measure` | Download timing checks |
| `lab` | Tests against real apps in a test lab. They read `HOLYARRS_LAB_*` and skip when those are unset |
| `labms`, `livesubs` | Lab runs for media servers and live subtitle sites |

</details>

<details class="ha-more"><summary>Long and fault runs</summary>

CI runs these on top of the plain tests. Run them before a release.

```sh
go test -count=1 -run '^TestFault|^TestCrashInEveryLoopIsContained$' ./app/backend/cmd/holyarrs
HOLYARRS_RESTART_TIMING=1 go test -count=1 -run '^TestCrashRestartTime$' ./app/backend/cmd/holyarrs
HOLYARRS_MEMORY=1 go test -count=1 -timeout 30m -run '^TestMemoryPartsRuns$' ./app/backend/cmd/holyarrs
go test -count=1 -run '^TestDownloaderConformance$' ./app/backend/internal/downloads
```

| Variable | Does |
|---|---|
| `HOLYARRS_FAULT_OUT` | File the fault runs write their table to |
| `HOLYARRS_RESTART_TIMING`, `HOLYARRS_RESTART_TIMING_OUT` | Turn on the crash-to-sign-in timing and name its output file |
| `HOLYARRS_MEMORY`, `HOLYARRS_MEMORY_OUT` | Turn on the memory runs and name their output file |
| `HOLYARRS_CRASH_RUNS`, `HOLYARRS_CRASH_SEED` | Rounds and seed for the crash loop in `app/backend/internal/mediafiles/dbtests` (`crash_test.go`), which skips under `-short` |

</details>

<details class="ha-more"><summary>Stand-in servers and variables</summary>

End-to-end tests start stand-in servers for Downloaders, Sources and the metadata server, so no test reaches a real service. The tests set variables such as `HOLYARRS_TMDB_URL`, `HOLYARRS_MUSICBRAINZ_URL`, `HOLYARRS_PLEX_URL` and `HOLYARRS_QBITTORRENT_URL` to point the program at them. A real install never needs them.

The API contract has its own tests. `TestOpenAPIGenerated` fails when `openapi.json` drifts from the routes, and the route-table snapshot in `tools/structurecheck/testdata` fails on any added, removed or moved route.

</details>

## Related

- [Contributing](contributing.md)
- [Modules](modules.md)
- [Releasing](releasing.md)
