# Developer setup

You want to build HolyArrs and run it on your own machine. This page gets you there.

## What you need

- Go, the version in `go.mod`
- Node 24 and npm
- Docker, if you want to build the image
- Python 3, only for the release scripts

Builds use `CGO_ENABLED=0`. You don't need a C compiler.

## Build and run

```sh
npm --prefix app/frontend ci
npm --prefix app/frontend run build
go build -o bin/holyarrs ./app/backend/cmd/holyarrs
./bin/holyarrs -config ./config
```

Open `http://localhost:6262` and make the owner account.

The web build lands in `app/frontend/dist`, and the program embeds it. After a change to the web app, build it again, or the program serves the old one.

The Makefile wraps the same steps: `make ui`, `make build`, `make test`, `make vet`.

## Work on the web app

Run the program as above. Then, in a second terminal:

```sh
npm --prefix app/frontend run dev
```

The dev server reloads on save and sends API calls to the running program.

## Fake data

`app/backend/tools/demo` works on a stopped config folder. It can fill the Library with a demo collection, take that collection out again, and count what is stored.

`app/backend/tools/fakesubs` runs a fake subtitle site for the subtitle tests. Neither tool ships in the image.

## Held-back features

Some features are held back. They stay off unless their switch name is in `HOLYARRS_FLAGS`. Turn one on while you work on it:

```sh
HOLYARRS_FLAGS=subtitle_sites,ai_apps ./bin/holyarrs -config ./config
```

The switches today:

| Switch | Feature |
|---|---|
| `ai_apps` | AI app connections and keys |
| `plex_watchlists` | Plex watchlists become requests |
| `row_title_stats` | Explore row statistics for the owner |
| `trakt_lists` | Trakt lists |
| `alert_channels_more` | alert channels beyond Discord and email |
| `subtitle_sites` | subtitle sites and subtitle searching |
| `emby_signin` | sign in with Emby |
| `go_back` | steps to go back to the previous version |

To add one, register its name with `startup.RegisterFeature` and check `startup.FeatureOn` before you register its routes. The web app gets the list of switched-off features and hides them. Users read about them in [Held back](../user/help/held-back.md).

## Build the image

```sh
docker build --file deploy/Dockerfile --tag holyarrs:dev .
docker run -p 6262:6262 -v ./config:/config holyarrs:dev
```

The image starts `FROM scratch`. It holds the program, CA certificates and licences.

## Before you open a pull request

Run the checks in [Testing](testing.md). Read [Modules](modules.md) so your change lands in the right place.
