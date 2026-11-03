# Import your keys and passwords from one file

Setting up HolyArrs means typing a lot of keys: subtitle sites, your Downloaders, media servers, mail. Instead of typing each one into Settings, you can put them all in one file and import it with one command. Then you delete the file.

## What it does

`holyarrs import-secrets` reads the file and, for each item you filled in, does the same thing the Settings screens do when you save:

| In the file | Becomes |
|---|---|
| `trakt.client_id` | the Trakt app key (Settings > Outside lists). `tmdb.read_token` and `tvdb.api_key` are still accepted from older files; metadata needs no keys. |
| `opensubtitles`, `subdl` | a Subtitle site with your account |
| `indexers` (one entry per feed) | a Source, **paused** until you check and resume it |
| `jackett`, `prowlarr` | their key fills in any `indexers` entry on the same address that has no key of its own |
| `qbittorrent`, `sabnzbd` | a Downloader, **paused** until you check and resume it |
| `jellyfin`, `emby`, `plex` | a media server, **off** until you test it and turn it on |
| `smtp`, `discord`, `webhook` | the Alerts "Email", "Discord" and "Webhook", **off** until you test them |
| `radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr`, `seerr` | a key saved for the Move tool (see below) |
| `addic7ed` | an Addic7ed subtitle site with your user name and password (shows only) |
| `nzbget`, `transmission`, `deluge` | nothing yet: HolyArrs cannot use them yet, so they are skipped |
| `holyarrs` | nothing: only used to reach HolyArrs while it runs |

Every key and password goes into the encrypted vault, like a key you type into Settings.

SABnzbd needs its full `api_key`. If your file also includes `sabnzbd.nzb_key`, `sabnzbd.username` or `sabnzbd.password`, those add-only or web login credentials are accepted but skipped and never stored. They cannot replace the full API key.

The optional account fields `tvdb.unique_user_key` and `trakt.redirect_urls` are also accepted but skipped without storing them. HolyArrs uses `tvdb.api_key` and `trakt.client_id` for those services.

For email, `smtp.host` is the server name alone, without a scheme or port, `smtp.port` must be between 1 and 65535, and `smtp.from` must be one plain email address. A dry run checks these fields before reporting that email settings can be saved.

Running it again is safe. Items that match what HolyArrs already has say `unchanged`; only items you changed in the file are saved again. Items are matched by name (Sources), by kind and address (Downloaders, media servers), by name and kind (Alerts) and by kind (subtitle sites, old apps).

A Source or Downloader whose address or key changes is paused again and says `set (changed, paused until you check and resume it)`. An empty `api_key` on an indexers entry keeps the key HolyArrs already has; to remove it, add `clear_key: true` to that entry. A Source or Downloader the import made that is somehow running without you resuming it is paused again on the next run; once you resume one yourself, a re-run leaves it running.

The file must be the file itself: a link to it is refused.

### The local network switch

Each block with an address can have `allow_local_network: true`. That lets that one item reach this machine or your home network (a `192.168.x.x` or `10.x.x.x` address, or a container name). Without it, the item may only reach the internet, and a Downloader on your home network will not answer. HolyArrs never guesses this from the address; you say it per item. It is the same switch as **May reach the local network** in Settings.

### Old apps and the Move tool

The `radarr`, `sonarr`, `lidarr`, `prowlarr`, `bazarr` and `seerr` blocks are not turned into anything. Their address and key are saved, encrypted, for **Settings > Move from other apps**. When you pick one of those apps under **Add an app** there, the sheet offers **Use the saved key** instead of asking you to type it; the address is the saved one and cannot be changed. The key is never shown, and only the owner can use it. After one successful read the saved key is deleted. To delete all saved keys without using them:

```
holyarrs import-secrets -forget-move-credentials
```

## What it never prints

The command prints one line per item, and the line only ever says one of:

- `set` (or `would set` in a dry run), sometimes with a short note such as "paused until you check and resume it",
- `unchanged`,
- `skipped (empty)` or `skipped (why)`,
- `failed: why`, in plain words.

It never prints a key, a password, a token, an address, or any part of one. The same goes for the log and for History: the import records one Event with counts only ("3 set, 12 unchanged, 0 failed"). If an error message from somewhere inside would hold a value from your file, that value is replaced with `[hidden]`.

## The file

Start from the template, [holyarrs-secrets.template.yaml](holyarrs-secrets.template.yaml), with every value empty. Fill in what you use; leave the rest empty. Empty means skip.

The file is a small, strict part of YAML:

- blocks by indentation (spaces, never tabs), lists with `- `,
- values plain, in `"double quotes"` or `'single quotes'` (write a `'` inside single quotes twice),
- `#` starts a comment when it begins the line or follows a space outside quotes, so quote a password that holds ` #`,
- quote any value that starts with `&`, `*`, `!`, `|`, `>`, `[`, `{`, `%`, `@` or a backquote, or holds `: `.

A key HolyArrs does not know (a typo, or a block that is not in the template) stops the whole import before anything is saved, with the key's name and the line number. So does a file over 256 KB.

On Linux and macOS the file must be readable by you only: `chmod 600 holyarrs-secrets.yaml`. HolyArrs refuses a file that other users may read. On Windows it cannot check this and prints a warning: keep the file in a folder only you can open.

## How to run it

Always try a dry run first, over https or inside HolyArrs's container (loopback). It says what would be set and changes nothing:

```
holyarrs import-secrets -file holyarrs-secrets.yaml -dry-run
holyarrs import-secrets -file holyarrs-secrets.yaml
```

| Flag | What it does |
|---|---|
| `-file <path>` | the secrets file |
| `-dry-run` | only say what would be set |
| `-config <folder>` | HolyArrs's config folder (default `/config`) |
| `-delete` | after a run where nothing failed, overwrite the file with zeros and delete it (best effort: SSDs and copy-on-write file systems may keep old blocks) |
| `-forget-move-credentials` | delete every old-app key saved for the Move tool, then stop |
| `-allow-http-lan` | while HolyArrs runs, allow plain `http://` to a home-network address (a warning is printed; prefer https or running inside the container) |
| `-allow-readable-file` | accept a file other users may read (Docker Desktop on Windows shows every file as readable) |

The command picks how to save by itself:

- **HolyArrs is stopped**: it opens the config folder directly and saves there.
- **HolyArrs is running** (it holds the config folder): it sends the file to HolyArrs's API. For this, fill in the `holyarrs` block: the address HolyArrs answers on, and an owner API key with the **admin** scope (make one in **Settings > Sign-in > API keys**; delete it afterwards if you like). The key is sent in a request header, never in the address. The address must be `https://`, or plain `http://` to this machine only (`127.0.0.1`, `::1` or `localhost`): over plain http the owner key and the whole file travel unencrypted. The easiest safe route is to run the command inside HolyArrs's container (see **In Docker** below), where `http://127.0.0.1:<port>` never leaves the container. Plain http to another address on your home network is refused unless you pass `-allow-http-lan`, and then the command prints a warning. A cloud metadata or link-local address is always refused. The `holyarrs` block is never saved.

It ends with exit code 0 when every item went well, 1 when an item failed (fix it in the file and run again: what was set already says `unchanged`), and 2 for a bad command line or file.

### In Docker

Mount the file **read-only**, next to the config volume.

This is the default route. While HolyArrs runs, run the command inside its container with `docker exec`. The app holds the config folder, so the command goes through the API: set `holyarrs.address` to `http://127.0.0.1:18730` (the app as seen from inside its own container) and `holyarrs.owner_api_key` to your owner key.

```
docker run ... -v /srv/holyarrs/holyarrs-secrets.yaml:/run/secrets/holyarrs.yaml:ro ... holyarrs-image
docker exec holyarrs /holyarrs import-secrets -file /run/secrets/holyarrs.yaml -dry-run
docker exec holyarrs /holyarrs import-secrets -file /run/secrets/holyarrs.yaml
```

With HolyArrs stopped, run a one-off container on the same config volume. It saves straight into the config folder, and the `holyarrs` block is not needed:

```
docker stop holyarrs
docker run --rm -v holyarrs-config:/config -v /srv/holyarrs/holyarrs-secrets.yaml:/run/secrets/holyarrs.yaml:ro holyarrs-image import-secrets -file /run/secrets/holyarrs.yaml
docker start holyarrs
```

The container runs as user 1000, so the file must be readable by user 1000 (`chown 1000 holyarrs-secrets.yaml; chmod 600 holyarrs-secrets.yaml`). If your master key lives in its own file (`HOLYARRS_MASTER_KEY_FILE`), give the one-off container the same variable and mount.

## Deleting the file

When everything says `set` or `unchanged`, delete the file. With `-delete` the command does it: it overwrites the file with zeros, writes that to disk, then removes it. It only does this after a run where nothing failed.

A read-only mount cannot be deleted from inside the container. Delete the file on the host instead:

- Linux: `shred -u holyarrs-secrets.yaml`
- macOS: `rm -P holyarrs-secrets.yaml`
- Windows (PowerShell): `Remove-Item holyarrs-secrets.yaml`, then empty the Recycle Bin if it went there.

On SSDs and copy-on-write file systems, overwriting cannot promise the old blocks are gone; the encrypted vault is where the keys now live.
