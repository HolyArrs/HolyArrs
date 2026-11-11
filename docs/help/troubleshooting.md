---
title: Troubleshooting
description: Find what is wrong from what you see, then fix it with the log, the doctor and the support bundle.
level: Start
sidebar: { order: 1 }
---

Find your symptom, read the cause, follow the fix. The examples use the container name `holyarrs`; use yours if it differs.

## First 3 checks

1. Watch the log: `docker logs -f holyarrs`. Secrets are masked. Press Ctrl+C to stop following; the container keeps running.
2. Run the doctor: `docker exec holyarrs /holyarrs doctor`. Every line says `OK`, `WARN` or `FAIL`, and each `WARN` or `FAIL` says how to fix it.
3. Still stuck? Download a support bundle and ask. See [Get support](../help/get-support/).

## Can't reach the web page

**Symptom:** the browser can't connect to `http://your-server:6262`.

**Cause:** the container is stopped, the port is not mapped, or a proxy sits in the way.

**Fix:**

1. Run `docker ps`. You should see `holyarrs` with `0.0.0.0:6262->6262`.
2. If it keeps restarting, read `docker logs holyarrs` for the reason.
3. If you use a path such as `/holyarrs`, see [Reverse proxy and URL base](../run/url-base/).

## Permission denied

**Symptom:** the doctor says a folder is not writable, or imports fail with a permission error.

**Cause:** the folder's owner on the host doesn't match `PUID` and `PGID`.

**Fix:** run the `chown` the doctor suggests, or set `PUID` and `PGID` to the folder's owner. On a NAS, give that user read and write in the permissions screen. See [NAS, Synology and Unraid](../run/platforms/).

## Nothing downloads

**Symptom:** titles stay missing and nothing reaches your downloader.

**Cause:** no source answers, the downloader is not connected, or no release passes your pick rules.

**Fix:**

1. Open **Settings > Downloaders** and click **Test** on your downloader. It should pass.
2. Open **Settings > Sources** and click **Test** on each source.
3. On the title, open **Choose a release** to see every result and the reasons it was not picked.

See [Downloaders](../downloads/downloaders/) and [Sources](../downloads/sources/).

## Downloaded but not imported

**Symptom:** the download finished in your downloader, but the title is not in your Library.

**Cause:** HolyArrs can't see the file at the same path, can't write the Library folder, or the import needs a decision.

**Fix:**

1. Give HolyArrs and the downloader the same download folder at the same path, such as `/downloads`, or set **Downloader's path** and **HolyArrs' path** on the downloader.
2. Run the doctor and fix any `FAIL` on the download or Library folders.
3. Check the downloads list for items marked **Needs a decision**, and approve or discard the plan.

## Imports are copies, not hardlinks

**Symptom:** the doctor says hardlinks don't work, or disk use doubles after imports.

**Cause:** downloads and Library folders are on different disks or mounts, or the disk can't hardlink (DrivePool, many NAS and SMB shares, Docker Desktop on Windows). Each file is still written once. Finished Usenet downloads are moved when they share a disk with the Library.

**Fix:** mount one shared parent folder into the container instead of separate ones. See [Folders](../start/folders/).

## HolyArrs won't start

**Symptom:** the container stops or restarts at once.

**Cause:** the log names it. Common ones: `/config` on a network share or FUSE path, or a broken part.

**Fix:**

1. Move `/config` to a local disk if the log says so. HolyArrs won't run its database on a share.
2. Otherwise start in [safe mode](../run/parts-safe-mode/#start-in-safe-mode), fix it from **Settings**, and restart normally.

## A feature is missing

**Symptom:** a feature you read about is not in the app.

**Cause:** it may be held back in this release.

**Fix:** see [Held-back features](../run/updates.md#held-back-features).

## Pages have no names or pictures

**Symptom:** titles show without posters or details.

**Cause:** the metadata server can't be reached.

**Fix:** check your DNS and firewall. The doctor's Network section shows whether the server answers.

## For experts

<details class="ha-more"><summary>Doctor checks and options</summary>

The doctor runs inside the container as the same user HolyArrs runs as, so it hits what HolyArrs hits.

| Section | Shows |
|---|---|
| App | Version, user and group, umask, whether it runs in a container. |
| Config folder | Owner, rights, free space, whether it is on a local disk. |
| Library and download folders | Exists, owner, writable, free space, hardlinks between download and Library folders. |
| Network | Whether the metadata server's name resolves and it answers. |
| Clock | Time, time zone (`TZ`), a warning when the clock is clearly wrong. |

| Option | What it does |
|---|---|
| `-config <folder>` | Checks another HolyArrs config folder than `/config`. |
| `-json` | Prints the report as JSON. |

Exit code 0 means nothing failed; 1 means a line says `FAIL`. The report never shows keys, passwords or tokens. It only reads, except for a small test file it writes and removes. Started as root, it also hands the config folder to `PUID:PGID`, like a normal start.

If HolyArrs keeps restarting, `docker exec` fails. Run the doctor in a new container with the same folders:

```sh
docker run --rm --volumes-from holyarrs -e PUID=<puid> -e PGID=<pgid> -e TZ=<tz> holyarrs/holyarrs:<tag> doctor
```

With Compose: `docker compose run --rm holyarrs doctor`.

</details>

<details class="ha-more"><summary>Start-up folder check</summary>

At start, HolyArrs checks its folders and logs one summary line, such as `folders: 4 ok, 1 not writable`, then one line per problem folder with a fix. The check stops after 3 seconds so start never hangs. A folder too slow to answer counts as not checked; this is often a disk waking from sleep or a slow network share. The doctor checks it without the time limit. See [Logs](../run/logs/).

</details>

<details class="ha-more"><summary>No shell in the image</summary>

The image holds only the HolyArrs program, so it stays small and leaves less for an attacker. `docker exec holyarrs sh` fails by design. For a closer look, start a throwaway helper beside it:

```sh
docker run --rm -it --pid container:holyarrs --network container:holyarrs --volumes-from holyarrs busybox sh
```

It sees the HolyArrs process, shares its network and has the same folders at the same paths. It runs as root, so look but don't change: a file written as root may be unreadable for HolyArrs. To test as the HolyArrs user, add `--user <PUID>:<PGID>`.

</details>

<details class="ha-more"><summary>Find the version</summary>

```sh
docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' holyarrs
```

The doctor's App section shows the same.

</details>

## Related

- [Get support](../help/get-support/)
- [Logs](../run/logs/)
- [Health checks](../run/health/)
