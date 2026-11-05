# Support info

When something goes wrong, these are the things to collect before you ask for
help. They also answer most of the questions people would otherwise open a
shell for. The examples use the container name `holyarrs` from the
[install guide](install.md#usage); use your own name if it differs.

## Watch the log

    docker logs -f holyarrs

This follows the program's output live. Secrets are masked in it the same way
as in the log file (see [Logs and storage](logs.md#secrets-are-never-shown)). Press Ctrl+C to stop following; the
container keeps running.

At start, after HolyArrs opens its folders, it writes one summary line such as
`folders: 4 ok, 1 not writable`, then one warning line per folder with a
problem and a short fix. These checks have a time limit of 3 seconds in
total, so start-up never hangs. A folder that does not answer in time is
skipped and counted as `not checked (slow to answer)`. This is most often a
disk waking from sleep or a slow network share. Run the doctor below to check
it without the start-up time limit.

## Run the doctor

When the container is running:

    docker exec holyarrs /holyarrs doctor

`docker exec` only works on a running container. When HolyArrs keeps stopping
or restarting, run the doctor once in a new container from the same image,
with the same folders and the same `PUID`, `PGID` and `TZ`:

    docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> -e TZ=<tz> ghcr.io/coderluii/holyarrs:<tag> doctor

`--volumes-from` also works when `holyarrs` is stopped. Use the same image tag
you run. With Compose, `docker compose run --rm holyarrs doctor` does the same
with the settings from your compose file.

The doctor checks the setup from inside the container, as the same user the
app runs as, so what it finds is what the app itself meets. It switches to
`PUID:PGID` the same way start-up and `reset-owner` do: when it starts as root
and the config folder's owner differs from `PUID:PGID`, it first gives the
config folder and everything in it to `PUID:PGID`, as start-up would. Apart
from that it only reads: it works while the server is running, and the only
other thing it writes is a small test file it makes and removes again to check
that a folder can be written.

It prints a short plain-text report in sections:

| Section | What it shows |
| --- | --- |
| App | Version, the user and group it runs as (`uid:gid`), extra groups, the umask, and whether it is inside a container. |
| Config folder | The path, its owner and mode, whether it can be written, free space, and whether it is on a local disk or a network share. The database must be on a local disk. |
| Library and download folders | Each folder saved in the settings: whether it exists, its owner, whether it can be written, free space, and whether hardlinks work between the download folder and the Library folders. |
| Network | Whether the metadata server's name resolves (and to what), and whether one HTTPS request to it answers within a short time. |
| Clock | The current time, the time zone (`TZ`), and a warning when the clock is clearly wrong. |

Every line is marked `OK`, `WARN` or `FAIL`. A `WARN` or `FAIL` line comes with
one plain sentence on how to fix it, for example which `chown` to run on the
host or which `PUID`/`PGID` to set. The report never prints API keys,
passwords, tokens or links that carry a key.

The doctor exits with code 0 when nothing failed and 1 when at least one line
says `FAIL`, so a script can use it too.

Options:

- `-config <dir>`: check another config folder than the default `/config`.
  Started as root, the doctor gives that folder to `PUID:PGID` first, as
  described above, so point it only at a HolyArrs config folder.
- `-json`: print the report as JSON instead of plain text.

## Download a support bundle

A support bundle is one zip file with what a helper usually asks for. Owners
and admins can download it, the same people who may download the log.

A button on the Status and Settings pages is coming. Until then, the API route
works now:

    GET /api/v1/support/bundle

It needs sign-in (or an API key with the same rights as the log download, see
[Logs and storage](logs.md)). The file is named
`holyarrs-support-<UTC time>.zip` and holds:

| File | What it is |
| --- | --- |
| `log.txt` | The newest log, with the usual masking plus one more pass that removes API keys, tokens, passwords, `apikey=` values in links and `Authorization` headers. |
| `about.json` | The same details as the About view. |
| `doctor.txt` | The doctor report described above. |
| `health.json` | The current health check results. |

Secrets are masked, but the bundle can still hold folder paths and host names.
Look through the files, and any `docker logs` output, before you share them
anywhere public.

## Find the version

The image carries its version as a label:

    docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' holyarrs

The doctor's App section and the About view show the same version.

## Why there is no shell

The image has no shell and no command-line tools on purpose. It holds only the
HolyArrs program, which keeps it small and leaves less for an attacker to use.
`docker exec holyarrs sh` does not work, and that is expected. The doctor, the
log and the support bundle cover the usual reasons to open a shell: checking
owners, rights, free space, hardlinks, the network and the clock.

For the rare case that needs a closer look, start a throwaway helper container
next to HolyArrs:

    docker run --rm -it --pid container:holyarrs --network container:holyarrs --volumes-from holyarrs busybox sh

What it can see:

- `--pid container:holyarrs`: the HolyArrs process, so `ps` lists it.
- `--network container:holyarrs`: the same network as HolyArrs, so name
  lookups and connections behave as they do for the app.
- `--volumes-from holyarrs`: the same mounted folders (`/config`, `/movies`,
  `/tv`, `/music`, `/downloads` and whatever else you mounted), at the same
  paths.

`--rm` removes the helper when you leave the shell. The helper runs as root by default, so use it
to look, not to change things: a file written there as root may later be
unreadable for HolyArrs. If you must test as the app user, add
`--user <PUID>:<PGID>` to the command.

`docker debug holyarrs` does a similar job, but it is a Docker Desktop only
feature.
