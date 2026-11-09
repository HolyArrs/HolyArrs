---
title: Setup
description: Install the tools, build HolyArrs from source and run it on your own machine.
level: Setup
sidebar: { order: 2 }
---

Build HolyArrs from source and open it in your browser on port `6262`.

## Before you start

- Go, at the version `go.mod` names.
- Node 24 and npm.
- A terminal in the repo root.

You need no C compiler. Builds use `CGO_ENABLED=0`.

## Build and run

1. Install the web app packages:

   ```sh
   npm --prefix app/frontend ci
   ```

2. Build the web app:

   ```sh
   npm --prefix app/frontend run build
   ```

   You should see the build finish and `app/frontend/dist` fill with files.

3. Build the program:

   ```sh
   go build -o bin/holyarrs ./app/backend/cmd/holyarrs
   ```

4. Run it with a config folder of your own:

   ```sh
   ./bin/holyarrs -config ./config
   ```

5. Open `http://localhost:6262`.

   You should see the first-run wizard. Make the owner account.

:::caution
The program embeds `app/frontend/dist`. After a web app change, rebuild the web app before the program, or you get the old pages.
:::

`-config` takes a folder on local disk and defaults to `/config`. `-addr` changes the listen address and defaults to `:6262`.

## Work on the web app

Keep the program running. In a second terminal, start the dev server:

```sh
npm --prefix app/frontend run dev
```

The dev server reloads on save and sends API calls to the running program. You should see your edit in the browser within 2 s.

## Choices

### Use the Makefile

| Command | Does |
|---|---|
| `make ui` | Install the web packages and build the web app |
| `make build` | `make ui`, then build `bin/holyarrs` |
| `make test` | `go test` with a 25 min timeout |
| `make vet` | `go vet ./...` |
| `make docker` | Build the image as `holyarrs:dev` |
| `make e2e` | Run the end-to-end tests |

### Turn on a held-back feature

Some features are held back and stay off. Name them in `HOLYARRS_FLAGS`, comma separated:

```sh
HOLYARRS_FLAGS=subtitle_sites,ai_apps ./bin/holyarrs -config ./config
```

The pages hide a feature that is off, and its routes do not exist. Users read about them in [Held-back features](../run/updates.md#held-back-features).

### Fill the Library with demo data

The demo tool fills a config folder with a demo collection. It works only on a stopped HolyArrs and only on a folder you marked as a lab. Mark it once:

```sh
go run ./app/backend/tools/demo seed -config ./config -init-lab
```

Then `seed -config ./config` adds the demo library and `seed -config ./config -reset-demo` removes only what it added. The media folder must be `/data/media` unless you pass `-force`. The dev tools stay out of the image. See [Dev tools](#for-experts).

## If it does not work

**Symptom.** The pages look old after you changed the web app.
**Cause.** The program serves the build it embedded.
**Fix.** Run the web build, then the Go build, then start the program.

**Symptom.** `go build` fails on a missing `app/frontend/dist`.
**Cause.** The web app is not built yet.
**Fix.** Run step 2 first.

**Symptom.** The program says the port is in use.
**Cause.** Another HolyArrs or app holds `6262`.
**Fix.** Pass another address: `./bin/holyarrs -config ./config -addr :6263`.

## For experts

<details class="ha-more"><summary>Held-back switches</summary>

| Switch | Feature |
|---|---|
| `ai_apps` | AI app connections and keys |
| `plex_watchlists` | Plex watchlists become requests |
| `row_title_stats` | Explore row statistics for the owner |
| `trakt_lists` | Trakt lists |
| `alert_channels_more` | Alert channels beyond Discord and email |
| `subtitle_sites` | Subtitle sites and subtitle searching |
| `emby_signin` | Sign in with Emby |
| `go_back` | Steps to go back to the previous version |

To add one, register its name with `startup.RegisterFeature` and check `startup.FeatureOn` before you register its routes. The names live in `app/backend/internal/core/startup` (`feature.go`).

</details>

<details class="ha-more"><summary>Dev tools</summary>

| Tool | Command | Does |
|---|---|---|
| Demo | `go run ./app/backend/tools/demo <command> -config DIR` | Works on a stopped config folder. Commands: `inspect` (counts), `fingerprint`, `query "SELECT ..."` (read only), `doctor`, `seed`, `showcase` |
| Fake subtitle site | `go run ./app/backend/tools/fakesubs` | Serves fake subtitle sites for tests on `127.0.0.1:18727`. `-addr` moves it; `-accounts-addr` adds the account-site fakes |
| Route list | `go run ./app/backend/tools/apidoc` | Rewrites [API routes](api-routes.md) |

</details>

<details class="ha-more"><summary>Build the image</summary>

```sh
docker build --file deploy/Dockerfile --tag holyarrs:dev .
docker run -p 6262:6262 -v ./config:/config holyarrs:dev
```

The image starts `FROM scratch` and holds the program, CA certificates and licences. It runs `-config /config -addr :6262`.

</details>

## Related

- [Contributing](contributing.md)
- [Testing](testing.md)
- [Modules](modules.md)
