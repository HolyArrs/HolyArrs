# Testing

You want to know your change works before you send it. Run the everyday checks, then the end-to-end tests for the areas you touched.

## The everyday checks

CI runs the same ones.

```sh
go build ./...
go vet ./...
go test ./...
npm --prefix app/frontend run check
npm --prefix app/frontend run test
```

`go test ./...` includes the structure checks that hold the module rules. See [Modules](modules.md#the-checks).

## End-to-end tests

End-to-end tests use Playwright and live in `app/tests/`. Stand-in servers for Downloaders, Sources and the metadata server live there too, so no test reaches a real outside service.

```sh
npx --prefix app/frontend playwright install chromium
npm --prefix app/frontend run e2e
```

The run starts its own HolyArrs on a fresh config folder. The tests are split into projects. The `setup` project makes the owner first, then the others run. To run one area:

```sh
npm --prefix app/frontend run e2e -- --project=movies
```

A full run takes a while. Run the projects you touched while you work, and the full run before a release.

## Long tests

Some program-level tests take a long time: crash recovery in every loop, restart timing, memory under load. They carry the `overnight` build tag and skip in a normal `go test ./...`:

```sh
go test -tags overnight ./app/backend/cmd/holyarrs
```

Run them before a release, when the machine is free for an hour or more.

## Test variables

Most tests need nothing set. Some variables point the program at stand-in servers, for example `HOLYARRS_TMDB_URL`, `HOLYARRS_MUSICBRAINZ_URL`, `HOLYARRS_PLEX_URL` and `HOLYARRS_QBITTORRENT_URL`. The tests set them. You never need them in a real install.

Tests against real apps in a test lab read `HOLYARRS_LAB_*` variables and skip when they aren't set.

## Writing tests

- Put a Go test beside the file it tests.
- Put a web unit test beside its file as `name.test.ts`.
- Put an end-to-end test in `app/tests/<module>/`.
- Never call a real outside service. Add a stand-in instead.
- A test that passes only on your machine is a broken test.

## When something fails

- **Structure check fails.** Read the message. It names the file and the rule. Move the code, or pass plain data in from the higher module.
- **The web app looks old.** Build the web app again before you build the program.
- **End-to-end tests can't start.** Install the browser with the `playwright install` line above.
