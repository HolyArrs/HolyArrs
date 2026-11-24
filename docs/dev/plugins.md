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
| `min_app_version` | Oldest HolyArrs that may install it |
| `sha256` | Hash of the plugin file. Added by the release tools |
| `description` | Optional. One short line |
| `icon` | Optional. A PNG or SVG data URL under 48 KB. In `holyarrs-plugin.json`, set `icon_file` to a `.png` or `.svg` next to it instead |
| `support_email`, `support_url` | How people reach the author for help. A listed plugin needs at least one; the URL must be https. The Plugins page shows them under Contact |
| `size` | Size of the plugin file in bytes for that CPU type. Added by the release tools |

The Plugins page shows all three lists to the owner before install. Installing approves the websites; each folder gets a real folder only once the owner picks one. An update that asks for more (a new website or folder, or `read_write` where `read` was approved) is installed but held until the owner approves it. An update that asks for less starts at once.

### What a plugin gets

- `HTTPS_PROXY`, `HTTP_PROXY` and `ALL_PROXY` point at the plugin's own proxy in HolyArrs. Use your HTTP library's standard proxy support. A connect that ignores them still ends at the proxy, which then reads the TLS server name.
- `HOLYARRS_PLUGIN_FOLDER_<NAME>` for each approved folder (the name in capitals, other characters as `_`), and `HOLYARRS_PLUGIN_FOLDERS` as a JSON map of name to path.
- `HOLYARRS_PLUGIN_NO_UDP=1`: UDP is not available. Only TCP to the listed websites on ports 443 and 80 works.

## The sandbox

Plugins run only when HolyArrs runs on Linux, which includes the Docker image. Each running plugin gets:

- Memory: up to 512 MB.
- Processor: up to 1 CPU core.
- Processes: up to 256.
- Files: its own data folder, plus the folders the owner approved, read only or read and write as approved. A read-write folder can't get new symlinks, and a symlink in a folder that points outside it is not followed.
- Network: only the listed websites, through the proxy. HolyArrs resolves each name itself, refuses addresses written as numbers, and refuses a name that points at a private, loopback or link-local address. UDP, raw sockets, listening sockets and io_uring are refused; only hosts the owner adds may reach the LAN. A plugin with no websites gets no network sockets at all.

How the network lock works: a seccomp user-notify filter catches every `connect()`. HolyArrs opens its own socket to the plugin's proxy (or to the app's address, for a plugin handed it), puts it in place of the plugin's socket (`SECCOMP_IOCTL_NOTIF_ADDFD`) and reports success. The address the plugin passed is only read as a hint, so swapping it during the call changes nothing. This needs Linux 5.9 or newer and works in Docker's and Podman's default security profiles, rootless included, with no extra capabilities. Without it, plugins that list websites stay stopped unless the owner turns the network lock off on the Plugins page; UDP and raw sockets stay blocked even then.

The memory, processor and process caps need cgroups v2 on the host. When the plugin stops, everything it started stops too.

## How publishing works

Plugins are served from `https://plugins.holyarrs.com`, the Cloudflare R2 bucket `holyarrs-plugins`. The `HolyArrs/HolyArrs-Plugins` repo holds the workflows that fill it. The bucket holds:

- `plugins.json`: the signed list of every installable version for linux/amd64.
- `plugins-linux-arm64.json` and `plugins-linux-arm-v7.json`: the same list for the other CPU types.
- `blocklist.json`: the signed list of blocked versions, shared by every CPU type.
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

`app/backend/tools/pluginkey/publishjob_test.go` runs the whole job with a throwaway key that has no passphrase, like the real one: first lists, two publishes, a block, a stale tree and a repeat publish.

```sh
cd app/backend
go test ./tools/pluginkey/
```

You should see `ok`.

## Related

- [Plugins](../run/plugins.md)
- [Releasing](releasing.md)
