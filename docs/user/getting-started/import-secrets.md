# Import your keys and passwords from one file

You have a lot of keys to type in: subtitle sites, Downloaders, media servers,
mail. Instead of entering each one in Settings, you can put them all in one
file, import it with one command, and delete the file.

## Steps

1. Copy the template,
   [holyarrs-secrets.template.yaml](holyarrs-secrets.template.yaml), to
   `holyarrs-secrets.yaml`.
2. Fill in what you use. Leave the rest empty. Empty means skip.
3. Make the file readable by you only:

   ```sh
   chmod 600 holyarrs-secrets.yaml
   ```

4. Mount it read-only into the container, next to the config:

   ```yaml
   volumes:
     - /srv/holyarrs/config:/config
     - /srv/holyarrs/holyarrs-secrets.yaml:/run/secrets/holyarrs.yaml:ro
   ```

5. While HolyArrs runs, the command talks to it through its API. Make an
   owner API key with the **admin** scope in
   **Settings > API > API keys**. Put it in the `holyarrs`
   block of the file, with the address `http://127.0.0.1:6262`.
6. Try a dry run. It changes nothing:

   ```sh
   docker exec holyarrs /holyarrs import-secrets -file /run/secrets/holyarrs.yaml -dry-run
   ```

7. Run it for real:

   ```sh
   docker exec holyarrs /holyarrs import-secrets -file /run/secrets/holyarrs.yaml
   ```

8. Check new Sources and Downloaders, then resume them. Test new media servers
   and Alerts, then turn them on.
9. Delete the file on the host (see [Delete the file](#delete-the-file)), and
   the API key if you like.

### With HolyArrs stopped

You can also run a one-off container on the same config. It saves straight
into the config folder and needs no `holyarrs` block:

```sh
docker stop holyarrs
docker run --rm -e PUID=1000 -e PGID=1000 \
  -v /srv/holyarrs/config:/config \
  -v /srv/holyarrs/holyarrs-secrets.yaml:/run/secrets/holyarrs.yaml:ro \
  holyarrs/holyarrs:beta import-secrets -file /run/secrets/holyarrs.yaml
docker start holyarrs
```

Use the same `PUID` and `PGID` as the app. Otherwise the config folder is
handed to 1000:1000. The `PUID` user must be able to read the file. If you
use `HOLYARRS_MASTER_KEY_FILE`, give the one-off container the same variable
and mount.

## What each block becomes

| In the file | Becomes |
| --- | --- |
| `trakt.client_id` | The Trakt app key (Settings > Outside lists). |
| `tmdb`, `tvdb` | Accepted from older files. Metadata needs no keys. |
| `opensubtitles`, `subdl` | A subtitle site with your account. |
| `addic7ed` | An Addic7ed subtitle site, for shows only. |
| `indexers` | One Source per entry, **paused** until you check and resume it. |
| `jackett`, `prowlarr` | Their key fills in any `indexers` entry on the same address that has no key. |
| `qbittorrent`, `sabnzbd` | A Downloader, **paused** until you check and resume it. |
| `nzbget`, `transmission`, `deluge` | Nothing. HolyArrs cannot use these Downloaders yet, so they are skipped. |
| `jellyfin`, `emby`, `plex` | A media server, **off** until you test it and turn it on. |
| `smtp`, `discord`, `webhook` | The Alerts "Email", "Discord" and "Webhook", **off** until you test them. |
| `radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr`, `seerr` | A key saved for the Move tool. |
| `holyarrs` | Nothing. Only used to reach HolyArrs while it runs. Never saved. |

Every key and password goes into the encrypted vault, the same as a key you
type into Settings.

## Settings explained

### The local network switch

Each block with an address can have `allow_local_network: true`. It lets that
one item reach this machine or your home network, such as `192.168.x.x`,
`10.x.x.x` or a container name. Without it, the item may only reach the
internet, so a Downloader at home will not answer. HolyArrs never guesses
this from the address. It is the same switch as **May reach the local
network** in Settings.

### Running it again

Running it again is safe. Items that match what HolyArrs has say
`unchanged`. Only changed items are saved again.

- Sources match by name. Downloaders and media servers match by kind and
  address. Alerts match by name and kind. Subtitle sites and old apps match
  by kind.
- A Source or Downloader whose address or key changed is paused again.
- An empty `api_key` on an `indexers` entry keeps the stored key. Add
  `clear_key: true` to remove it.
- Once you resume a Source or Downloader yourself, a re-run leaves it running.

### Special fields

- SABnzbd needs its full `api_key`. An `nzb_key`, user name or password is
  skipped and never stored.
- `tvdb.unique_user_key` and `trakt.redirect_urls` are accepted and skipped.
- `smtp.host` is the server name alone, without a scheme or port.
  `smtp.port` is 1 to 65535, 587 by default. `smtp.from` is one plain email
  address.

### Old apps and the Move tool

The `radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr` and `seerr` blocks are
saved for **Settings > Move from other apps**. When you pick one of those apps
there, choose **Use the saved key**. The key is never shown, only the owner
can use it, and it is deleted after one successful read. To delete all saved
keys without using them:

```sh
holyarrs import-secrets -forget-move-credentials
```

### The file format

The file uses a small, strict part of YAML:

- blocks by indentation with spaces, never tabs; lists with `- `,
- values plain, in `"double quotes"` or in `'single quotes'` (write a `'`
  inside single quotes twice),
- `#` starts a comment at the start of a line or after a space outside
  quotes, so quote a password that holds ` #`,
- quote a value that starts with `&`, `*`, `!`, `|`, `>`, `[`, `{`, `%`, `@`
  or a backquote, or that holds `: `.

An unknown key stops the whole import before anything is saved. The message
names the key and the line. So does a file over 256 KB. A link to the file is
refused. Give the file itself.

### Flags

| Flag | What it does |
| --- | --- |
| `-file <path>` | The secrets file. |
| `-dry-run` | Only say what would be set. |
| `-config <folder>` | The config folder. `/config` by default. |
| `-delete` | After a run with no failures, overwrite the file with zeros and delete it. |
| `-forget-move-credentials` | Delete every saved old-app key, then stop. |
| `-allow-http-lan` | Allow plain `http://` to a home-network address. Prints a warning. |
| `-allow-readable-file` | Accept a file other users may read. Docker Desktop on Windows shows every file as readable. |

The address in the `holyarrs` block must be `https://`, or plain `http://` to
this machine only (`127.0.0.1`, `::1` or `localhost`). Over plain http your
owner key and the whole file travel unencrypted. Running inside the container
keeps it on the same machine.

## What it prints

One line per item. Each line says one of:

- `set`, or `would set` in a dry run, sometimes with a short note,
- `unchanged`,
- `skipped (empty)` or `skipped (why)`,
- `failed: why`.

It never prints a key, password, token or address. History gets one Event
with counts only. If an error would show a value from your file, the value
is replaced with `[hidden]`.

Exit codes: `0` when every item went well, `1` when an item failed, `2` for a
bad command line or file. After a failure, fix the file and run it again.

## Delete the file

With `-delete`, the command overwrites the file with zeros and removes it,
but only after a run where nothing failed. A read-only mount cannot be
deleted from inside the container, so delete it on the host:

- Linux: `shred -u holyarrs-secrets.yaml`
- macOS: `rm -P holyarrs-secrets.yaml`
- Windows: `Remove-Item holyarrs-secrets.yaml`, then empty the Recycle Bin.

On SSDs and copy-on-write file systems, old blocks may stay on disk. The keys
now live in the encrypted vault.

## Troubleshooting

| You see | Do this |
| --- | --- |
| The file is readable by other users | Run `chmod 600` on it. On Windows, keep it in a folder only you can open. |
| An unknown key stopped the import | Fix the key on the named line. Check for a typo or a tab. |
| Plain http refused | Run the command inside the container with `http://127.0.0.1:6262`, or use https. |
| A Downloader at home does not answer | Set `allow_local_network: true` on that block. |
| The one-off container changed folder owners | Give it the same `PUID` and `PGID` as the app. |
| `-delete` did nothing | Something failed, or the mount is read-only. Delete it on the host. |
