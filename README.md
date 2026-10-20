# HolyArrs

One self-hosted app that keeps a library of movies, shows and music: it finds
Releases on your Sources, picks one with your Pick rules, sends it to your
Downloader, and imports the finished file into your Library folder.

## Status

Local v1 pre-release build. The image is not published yet.

## Documentation

- [Install with Docker](docs/install.md)
- [Screen-by-screen documentation index](docs/index.md)

## Run

    make build
    ./bin/holyarrs -config ./config -addr :18730

Or with Docker:

    make docker
    docker run -p 18730:18730 -v ./config:/config holyarrs:dev

The config folder must sit on a local disk. HolyArrs refuses to start its
database on a network share.

## First run and sign-in

On first start HolyArrs writes a one-time setup code to `setup-code.txt` in the
config folder and to the log. Open the page, enter the code, and pick the
owner name and password (there is no default password). The code file is
deleted once the owner exists. Every page and API route needs sign-in except
`/healthz` and `/login`. Scripts use an API key in the `X-Api-Key` header.
Each key holds scopes (read, write, admin, control, or all), and how long a
route stays after it is marked to go is in
[docs/parts-and-safe-mode.md](docs/parts-and-safe-mode.md#api-keys-and-scopes).

Host names other than IP addresses and `localhost` must be listed in
`HOLYARRS_ALLOWED_HOSTS` (comma-separated), for example
`HOLYARRS_ALLOWED_HOSTS=holyarrs.home.lan`. Behind a reverse proxy, list the
proxy in `HOLYARRS_TRUSTED_PROXIES` if you turn on "skip sign-in on the local
network" (off by default).

## Metadata

Metadata needs no keys. Names, dates, pictures, Explore and music details come
from the HolyArrs metadata server at `https://holyarrs-api.coderluii.dev`;
each install registers itself on its first lookup. `HOLYARRS_METADATA_URL`
points HolyArrs at another server. See [docs/metadata-keys.md](docs/metadata-keys.md).

## Explore and trailers

Explore builds a page of rows for each person from the Library, their asks and
the metadata server's charts, and can play a muted trailer over the art. See
[Explore](docs/explore.md) and [Trailers](docs/trailers.md). The Docker image
is built on `debian:12-slim` and bundles yt-dlp for trailers
([details](docs/install.md#the-image)).

## Master key and backups

Passwords, Source keys and tokens are stored encrypted with a master key. On
first start HolyArrs makes `holyarrs.key` (mode 0600) in the config folder, or
reads the file named by `HOLYARRS_MASTER_KEY_FILE`.

**Backups do not hold the master key.** Keep a copy of the key file somewhere
safe. A backup restored without its key cannot open the stored secrets, and
HolyArrs refuses such a restore.

Backups are zips in `backups/` in the config folder: a safe copy of both
databases and a version and date file, never media. Presets: Balanced (every
night, keeps 7 nightly and 4 weekly, the default), Light (weekly, 28 days),
Careful (every night, 30 days). HolyArrs also backs up before every database
migration and update; backups you make by hand stay until you delete them.
To restore, upload a zip (`POST /api/v1/backups/restore`); HolyArrs checks it,
stages it and restarts (exit status 75) to put it in place, keeping the
replaced databases in `backups/before-restore-<time>/`. Run the container with
a restart policy such as `--restart unless-stopped`.

## Develop

    make test
    make vet

Builds use `CGO_ENABLED=0` and Go 1.27.

## Security

See [SECURITY.md](SECURITY.md) to report a problem privately.

## Licence

GPL-3.0.
