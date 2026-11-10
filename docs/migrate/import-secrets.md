---
title: Import keys from a file
description: Put every key and password in one YAML file, import it with one command, then delete the file.
level: Setup
sidebar: { order: 6 }
---

Fill in one file with your Sources, Downloaders, media servers, subtitle sites and mail, import it, and skip typing each key in Settings.

## Before you start

- HolyArrs runs in Docker with its config at `/config`.
- You can make an owner API key in **Settings > API > API keys**.

## Import the file

1. Copy [holyarrs-secrets.template.yaml](holyarrs-secrets.template.yaml) to `holyarrs-secrets.yaml`.
2. Fill in what you use. Leave the rest empty; empty means skip.
3. Make it readable by you only:

   ```sh
   chmod 600 holyarrs-secrets.yaml
   ```

4. Mount it read-only into the container:

   ```yaml
   volumes:
     - /srv/holyarrs/config:/config
     - /srv/holyarrs/holyarrs-secrets.yaml:/run/secrets/holyarrs.yaml:ro
   ```

5. Make an owner API key with the **admin** scope in **Settings > API > API keys**. Put it in the file's `holyarrs` block with the address `http://127.0.0.1:6262`.
6. Try a dry run. It changes nothing:

   ```sh
   docker exec holyarrs /holyarrs import-secrets -file /run/secrets/holyarrs.yaml -dry-run
   ```

   You should see one `would set` line per item.

7. Run it for real (same command without `-dry-run`). You should see `set` lines and exit code `0`.
8. Check new Sources and Downloaders, then resume them. Test new media servers and Alerts, then turn them on.
9. Delete the file on the host (see [Delete the file](#delete-the-file)). Delete the API key too if you no longer need it.

:::caution
The file holds every secret in plain text until you delete it. Delete it on the host as soon as the run succeeds.
:::

## What each block becomes

| In the file | Becomes |
| --- | --- |
| `trakt.client_id` | The Trakt app key (**Settings > Outside lists**). |
| `tmdb`, `tvdb` | Accepted from older files. Metadata needs no keys. |
| `opensubtitles`, `subdl` | A subtitle site with your account. |
| `addic7ed` | An Addic7ed subtitle site, shows only. |
| `indexers` | One Source per entry, **paused**. |
| `jackett`, `prowlarr` | Their key fills any `indexers` entry on the same address that has no key. |
| `qbittorrent`, `sabnzbd` | A Downloader, **paused**. |
| `nzbget`, `transmission`, `deluge` | Skipped. The import cannot set these up yet; add them in **Settings > Downloaders**. |
| `jellyfin`, `emby`, `plex` | A media server, **off** until you test it. |
| `smtp`, `discord`, `webhook` | The Alerts "Email", "Discord" and "Webhook", **off** until you test them. |
| `radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr`, `seerr` | A key saved for **Import from other apps**. |
| `holyarrs` | Only used to reach HolyArrs while it runs. Never saved. |

Every key goes into the encrypted vault, the same as one typed in Settings.

## Choices

### The local network switch

Each block with an address can take `allow_local_network: true`. It lets that item reach this machine or your home network (`192.168.x.x`, `10.x.x.x`, a container name). Without it the item reaches only the internet, so a Downloader at home will not answer. HolyArrs never guesses this from the address. It is the same switch as **May reach the local network** in Settings.

### With HolyArrs stopped

Run a one-off container on the same config. It saves straight into the config folder and needs no `holyarrs` block:

```sh
docker stop holyarrs
docker run --rm -e PUID=1000 -e PGID=1000 \
  -v /srv/holyarrs/config:/config \
  -v /srv/holyarrs/holyarrs-secrets.yaml:/run/secrets/holyarrs.yaml:ro \
  holyarrs/holyarrs:latest import-secrets -file /run/secrets/holyarrs.yaml
docker start holyarrs
```

Use the same `PUID` and `PGID` as the app, or the config folder is handed to `1000:1000`. The `PUID` user must be able to read the file. If you use `HOLYARRS_MASTER_KEY_FILE`, give the one-off container the same variable and mount.

### Running it again

Running it again is safe. Matching items say `unchanged`; only changed items are saved.

| Item | Matched by |
| --- | --- |
| Sources | name |
| Downloaders, media servers | kind and address |
| Alerts | name and kind |
| Subtitle sites, old apps | kind |

A Source or Downloader whose address or key changed is paused again. Once you resume one yourself, a rerun leaves it running. An empty `api_key` on an `indexers` entry keeps the stored key; add `clear_key: true` to remove it.

## Delete the file

With `-delete`, the command overwrites the file with zeros and removes it, only after a run where nothing failed. A read-only mount cannot be deleted from inside the container, so delete it on the host:

| System | Command |
| --- | --- |
| Linux | `shred -u holyarrs-secrets.yaml` |
| macOS | `rm -P holyarrs-secrets.yaml` |
| Windows | `Remove-Item holyarrs-secrets.yaml`, then empty the Recycle Bin. |

On SSDs and copy-on-write file systems, old blocks may stay on disk. The keys themselves now live in the encrypted vault.

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| The file is readable by other users | Permissions too open. | Run `chmod 600`. On Windows, keep it in a folder only you can open. |
| An unknown key stopped the import | Typo or a tab on the named line. | Fix that line. Nothing was saved. |
| Plain http refused | The `holyarrs` address is not on this machine. | Run inside the container with `http://127.0.0.1:6262`, or use https. |
| A Downloader at home does not answer | No local network permission. | Set `allow_local_network: true` on that block. |
| The one-off container changed folder owners | Different `PUID`/`PGID`. | Use the app's `PUID` and `PGID`. |
| `-delete` did nothing | Something failed, or the mount is read-only. | Delete it on the host. |

## For experts

<details class="ha-more"><summary>Flags and exit codes</summary>

| Flag | What it does | Default |
| --- | --- | --- |
| `-file <path>` | The secrets file. | none |
| `-dry-run` | Only say what would be set. | off |
| `-config <folder>` | The config folder. | `/config` |
| `-delete` | After a run with no failures, overwrite the file with zeros and delete it. | off |
| `-forget-move-credentials` | Delete every saved old-app key, then stop. | off |
| `-allow-http-lan` | Allow plain `http://` to a home-network address. Prints a warning. | off |
| `-allow-readable-file` | Accept a file other users may read. Docker Desktop on Windows shows every file as readable. | off |

The `holyarrs` address must be `https://`, or plain `http://` to this machine only (`127.0.0.1`, `::1`, `localhost`). Over plain http your owner key and the whole file travel unencrypted.

| Exit code | Meaning |
| --- | --- |
| `0` | Every item went well. |
| `1` | An item failed. Fix the file and run again. |
| `2` | Bad command line or file. |

</details>

<details class="ha-more"><summary>Output</summary>

One line per item: `set` (or `would set` in a dry run, sometimes with a note), `unchanged`, `skipped (empty)`, `skipped (why)` or `failed: why`. It never prints a key, password, token or address. History gets one Event with counts only. An error that would show a value from your file shows `[hidden]` instead.

</details>

<details class="ha-more"><summary>File format</summary>

A small, strict part of YAML:

- blocks by indentation with spaces, never tabs; lists with `- `,
- values plain, in `"double quotes"` or `'single quotes'` (write a `'` inside single quotes twice),
- `#` starts a comment at line start or after a space outside quotes, so quote a password that holds ` #`,
- quote a value that starts with `&`, `*`, `!`, `|`, `>`, `[`, `{`, `%`, `@` or a backquote, or that holds `: `.

An unknown key or a file over 256 KB stops the whole import before anything is saved; the message names the key and line. A link to the file is refused.

</details>

<details class="ha-more"><summary>Special fields</summary>

| Field | Rule |
| --- | --- |
| SABnzbd | Needs the full `api_key`. An `nzb_key`, user name or password is skipped and never stored. |
| `tvdb.api_key`, `tvdb.unique_user_key`, `trakt.redirect_urls` | Accepted and skipped. |
| `smtp.host` | Server name only, no scheme or port. |
| `smtp.port` | 1 to 65535. Default `587`. |
| `smtp.from` | One plain email address. |
| Old-app blocks | Used in **Import from other apps** with **Use the saved key**. Never shown, owner only, deleted after one successful read. |

</details>

## Related

- [Read your old apps](what-is-read.md)
- [Environment variables](../reference/env-vars.md)
