---
title: Plugins
description: The plugin manifest, the sandbox plugins run in, and how a plugin version is published.
level: Expert
sidebar: { order: 8 }
---

A plugin is one Linux program that HolyArrs downloads, checks and runs in a locked-down sandbox. Only plugins signed by HolyArrs install. Users see them on the Plugins page; see [Plugins](../run/plugins.md).

## Where plugin code lives

Each plugin has a folder in `app/plugins/<id>/` with its Go source and a `holyarrs-plugin.json` file. The release tools turn that folder into a published version.

## The manifest

Each published version has a `manifest.json`. The release tools build it from `holyarrs-plugin.json` plus the built file.

| Field | Content |
|---|---|
| `id` | Lowercase letters, digits and dashes, up to 64 characters |
| `name` | Shown on the Plugins page |
| `version` | Plain `X.Y.Z`. A published version never changes |
| `author` | Who made it |
| `access.hosts` | Websites the plugin may reach, up to 10. Plain lowercase names; `*.example.com` covers every name under `example.com` but not `example.com` itself. No addresses, ports or paths |
| `access.folders` | Folders the owner picks for it at install, up to 10. Either a name (read only), or `{"name": "music", "mode": "read" or "read_write", "why": "...", "required": true}` |
| `access.actions` | HolyArrs actions it may call |
| `access.events` | Optional. What happens in HolyArrs that the plugin wants to be told, such as `["library.add"]`. See [Events](#events) |
| `min_app_version` | Oldest HolyArrs that may install it |
| `sha256` | Hash of the plugin file. Added by the release tools |
| `description` | Optional. One short line |
| `icon` | Optional. A PNG or SVG data URL under 48 KB. In `holyarrs-plugin.json`, set `icon_file` to a `.png` or `.svg` next to it instead |
| `support_email`, `support_url` | How people reach the author for help. A listed plugin needs at least one; the URL must be https. The Plugins page shows them under Contact |
| `size` | Size of the plugin file in bytes for that CPU type. Added by the release tools |

The Plugins page shows these lists to the owner before install. Installing approves the websites and events; each folder gets a real folder only once the owner picks one. An update that asks for more (a new website, event or folder, or `read_write` where `read` was approved) is installed but held until the owner approves it. An update that asks for less starts at once.

### What a plugin gets

- `HTTPS_PROXY`, `HTTP_PROXY` and `ALL_PROXY` point at the plugin's own proxy in HolyArrs. Use your HTTP library's standard proxy support. A connect that ignores them still ends at the proxy, which then reads the TLS server name.
- `HOLYARRS_PLUGIN_FOLDER_<NAME>` for each approved folder (the name in capitals, other characters as `_`), and `HOLYARRS_PLUGIN_FOLDERS` as a JSON map of name to path.
- `HOLYARRS_PLUGIN_NO_UDP=1`: UDP is not available. Only TCP to the listed websites on ports 443 and 80 works.

## Events

A plugin that lists `access.events` is told each one as it happens: one line of JSON on its standard input per event.

```json
{"event":"library.add","id":4182,"delivery":"c562d0875d914e3b","at":"2026-11-24T06:15:47Z","payload":{"moment":"add","event_kind":"title_added","subject":"Added: Arrival (2016)","text":"Added Arrival","movie":{"id":1,"name":"Arrival","year":2016,"tmdb":"329865","imdb":"tt2543164"},"data":{}}}
```

- Read standard input line by line. Each line is one event. `id` is the event's number in HolyArrs's history: it only grows, survives restarts and stays the same when an event is sent again. Keep the highest `id` you handled and skip anything at or below it. `delivery` is unique per line.
- `payload` has the title it is about (`movie`, `show` with `episodes`, or `artist` with `tracks`) and, for most events, `text` and `data`. Links in them read `[link]` and full paths of the server are cut to their last name.
- Requests and reports carry their ids and the title only, never who asked, who decided or what anyone wrote.
- Events from different things can arrive out of order. A plugin that reads too slowly loses lines (up to 256 wait); HolyArrs never waits for it, and lost lines are not sent again. Delivery is at most once: if the plugin or HolyArrs stops suddenly, lines written but not yet read can be lost, and catch-up may skip an event that arrived just before a newer one. Use the `id` to skip any line you have already handled.
- **Catch-up:** when a plugin starts again (after a stop, an update or a HolyArrs restart), it first gets the events it missed, up to 1000, then the live ones. The very first start, and the first start after the owner approved other events, begin with what happens from then on. If more than 1000 were missed, a line `{"event":"missed","payload":{"after":<id>}}` follows the 1000: read what you need from scratch.
- Standard input ends (EOF) when HolyArrs stops the plugin.
- Plugins that list no events get nothing on standard input, as before.

| Event | When |
|---|---|
| `library.add` | A title is added |
| `library.delete` | A title, or some of its episodes, is deleted |
| `library.import` | Files are placed in the Library |
| `library.upgraded` | A file is replaced by a better one |
| `library.rename` | Files get new names |
| `library.artwork` | A title gets new cover art |
| `library.subtitle` | A subtitle is saved or upgraded, or could not be |
| `library.needs_decision` | Something needs the owner's decision |
| `library.edited` | A title's details are changed by hand |
| `library.refreshed` | A title's details are refreshed |
| `library.moved` | A title moves to another Library folder |
| `library.episodes_changed` | A show's episodes or an artist's albums are added or changed |
| `library.adopted` | A folder found on disk is added to the Library |
| `library.file_missing`, `library.file_back` | A file goes missing, or is back |
| `download.grab` | A Release is sent to a Downloader |
| `download.done` | A download finishes |
| `download.failed` | A download fails |
| `download.rejected` | A finished download is turned down at import |
| `download.vanished` | A download disappears from its Downloader |
| `search.done`, `search.failed` | A search finishes, or fails |
| `search.release_held` | A Release is held back |
| `request.made`, `request.approved`, `request.declined`, `request.available` | A request moves along (ids and title only) |
| `report.made`, `report.fixed` | A problem is reported or fixed (ids, kind and title only) |
| `system.health` | A health check breaks or recovers |
| `system.media_server_updated` | A media server is told about new files |
| `system.list_synced` | An Outside list is synced |
| `system.folder_unreachable` | A Library folder can't be reached |
| `system.disk_low` | An import was refused because the disk is too full |

## The sandbox

Plugins run only when HolyArrs runs on Linux, which includes the Docker image. Each running plugin gets:

- Memory: up to 512 MB.
- Processor: up to 1 CPU core.
- Processes: up to 256.
- Files: its own data folder, plus the folders the owner approved, read only or read and write as approved. A read-write folder can't get new symlinks, and a symlink in a folder that points outside it is not followed.
- Network: only the listed websites, through the proxy. HolyArrs resolves each name itself, refuses addresses written as numbers, and refuses a name that points at a private, loopback or link-local address. UDP, raw sockets, listening sockets and io_uring are refused; only hosts the owner adds may reach the LAN. A plugin with no websites gets no network sockets at all.

How the network lock works: a seccomp user-notify filter catches every `connect()`. HolyArrs opens its own socket to the plugin's proxy (or to the app's address, for a plugin handed it), puts it in place of the plugin's socket (`SECCOMP_IOCTL_NOTIF_ADDFD`) and reports success. The address the plugin passed is only read as a hint, so swapping it during the call changes nothing. This needs Linux 5.9 or newer and works in Docker's and Podman's default security profiles, rootless included, with no extra capabilities. Without it, plugins that list websites stay stopped unless the owner turns the network lock off on the Plugins page; UDP and raw sockets stay blocked even then.

The memory, processor and process caps need cgroups v2 on the host. When the plugin stops, everything it started stops too.

## Developer mode

Developer mode lets you run a plugin you're building on your own server, without publishing or signing it.

1. On the **Plugins** page, open **Add** (the + button), choose **Build with code**, and select **Turn on developer mode**. A banner stays on the page while it is on.
2. Build your plugin for Linux on the server's CPU type, for example `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o plugin .`.
3. Copy two files into `<config folder>/plugins/_dev/<plugin id>/`. In the Docker image the config folder is `/config`.
   - `holyarrs-plugin.json`: the same file you will publish with. Its `id` must match the folder name.
   - `plugin`: the built program.
   Copy real files; links are refused.
4. The plugin shows on the Plugins page with an **Unsigned** tag. Select **Install from folder**, then approve what it asks for, as for any plugin.
5. After each new build, copy the file again and select **Reload**. Nothing needs a restart or a new version number.
6. A plugin that lists `access.events` gets a **Send a test event** button while it runs. It sends one line for the first event it asks for, with `"test": true` in the payload and a made-up movie.

What stays the same: the plugin runs in the same sandbox, reaches only the websites, folders and events you approved, and is checked against the program's hash at each start. What changes: it is not on the signed list, it never updates on its own, and it starts only while developer mode is on. Turning developer mode off stops every developer plugin.

## Test versions

A test version lets a few people try a plugin before everyone gets it. It is signed like a normal version but sits on separate test lists, so only people who ask for it see it.

- To publish one, run the publish job with `--test` and a version such as `1.1.0-test.1`.
- On the Plugins page, an owner opens the plugin and selects **Get test versions**. From then on, its update offers the newest test version when it is newer than the newest normal one. **Stop test versions** goes back to normal versions at the next update.
- Apps before 1.3.1 never read the test lists.

## How publishing works

Plugins are served from `https://plugins.holyarrs.com`, the Cloudflare R2 bucket `holyarrs-plugins`. The `HolyArrs/HolyArrs-Plugins` repo holds the workflows that fill it. The bucket holds:

- `plugins.json`: the signed list of every installable version for linux/amd64.
- `plugins-linux-arm64.json` and `plugins-linux-arm-v7.json`: the same list for the other CPU types.
- `blocklist.json`: the signed list of blocked versions, shared by every CPU type.
- `plugins-test.json`, `plugins-test-linux-arm64.json` and `plugins-test-linux-arm-v7.json`: the signed lists of test versions, with their own counter. Only there after a `--test` publish.
- `plugins/<id>/<version>/manifest.json` and `plugins/<id>/<version>/plugin` (linux/amd64), and `plugins/<id>/<version>/<platform>/...` for `linux-arm64` and `linux-arm-v7`.
- `source/<id>/<version>.tar.xz`: the exact source each version was built from.

Every version is built for the same CPU types as the app image. HolyArrs reads the list for the machine it runs on and installs that build. The amd64 files keep the first layout, so older apps still find them.

Each list has a counter that goes up with every change; all the plugin lists of one run share the same counter. HolyArrs refuses a list older than one it has seen, and the plugin list names the oldest blocklist it accepts.

The `publish.yml` workflow in `HolyArrs/HolyArrs-Plugins`:

1. Checks out the app repo at a tag.
2. Builds `app/plugins/<id>` for linux/amd64, linux/arm64 and linux/arm/v7 (`publish.py build --src <dir> --out <dir>` does only this step, with nothing signed).
3. Runs `deploy/release/plugins/publish.py`, which writes the manifest, adds the version to the list and raises the counter. It starts from the live lists, and refuses when the version already exists.
4. Signs the lists with the signing key from the `plugin-signing` environment (it has no passphrase, so the run needs nobody), then runs `pluginkey check`, the same install check the app runs.
5. Uploads the new files to R2, the version files first and the lists last. A file already on R2 is never overwritten.
6. Fetches the live `plugins.json` and checks it again.

`block.yml` adds a version to the blocklist the same way. To run either one, see [Publish plugins](releasing.md#publish-plugins). For the keys, see `docs/plugin-signing-key.md`.

## Try it locally

To run your own plugin, use [Developer mode](#developer-mode).

To try the publishing job, `app/backend/tools/pluginkey/publishjob_test.go` runs the whole job with a throwaway key that has no passphrase, like the real one: first lists, two publishes, a test version, a block, a stale tree and a repeat publish.

```sh
cd app/backend
go test ./tools/pluginkey/
```

You should see `ok`.

## Related

- [Plugins](../run/plugins.md)
- [Releasing](releasing.md)
