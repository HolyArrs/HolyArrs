# Alerts, health checks and media servers

This page covers how HolyArrs tells you and your tools what happened: Alerts (Discord, email and webhooks), the health checks that watch HolyArrs itself, and the link to Plex, Jellyfin or Emby. Only people with the settings permission change these; the owner and admins see the results.

## Alerts

Open **Settings**, **Alerts**, then **Add an Alert**. An Alert has a kind, where it sends, and the moments it speaks for.

### Kinds

- **Discord**: paste a channel's webhook link (in Discord: channel settings, Integrations, Webhooks, Copy link). People who asked for a movie are named in the message when they added their Discord user id in My account.
- **Email**: your mail server, port, security (STARTTLS, TLS or none), a From address, a sign-in name and password, and the staff addresses. Messages about asks go to the people who asked, at the address they set in My account. Everything else goes to the staff addresses.
- **Webhook**: a link on your own server. HolyArrs posts JSON to it (see below). Add a secret if your server should check that the message really came from HolyArrs.

Links, passwords and secrets are kept encrypted and are never shown again: the page shows them masked. Leave a masked field as it is to keep what is stored. When you point an Alert at another server, type the password or secret again: HolyArrs never sends a stored secret to a new place.

### Moments

Each Alert picks what it speaks for. New Alerts speak for every moment; untick what you do not want, or use **All of these** and **None of these** per group.

**Wishlist**

- **Someone asks**: to the staff, when a movie waits for approval.
- **Approved**, **Turned down**, **Ready to watch**: to the people who asked.

**Library** (to the staff)

- **Sent to download**: a Release went to a Downloader.
- **In the Library**: files were placed. A better copy that replaced an old one counts too.
- **Renamed**: files got new names.
- **Movie added**: added by hand, from an Outside list or from an approved ask. Adopting a whole library does not send one per movie.
- **Movie deleted**.
- **Health**: a health check broke, or is fixed again (see below).
- **Needs a decision**: a download or an import needs you (Downloads, Needs a decision).

### Sending, retrying and the test button

**Send a test** sends one message straight away and says whether it arrived. The card then shows **Last sent** or **Last try failed** with the reason.

When a real message fails because the server did not answer, was busy or had an error, HolyArrs tries again after 5 seconds, 30 seconds and 2 minutes. When the server refuses the message (a wrong link, a bad request), it does not try again. Every failure also writes an **alert failed** row in History.

### May reach the local network

Off by default. Then a Discord Alert only sends to discord.com, and email and webhooks only to servers on the internet: a link to this machine or your home network is refused, also after the name is looked up. Turn it on for a mail catcher, a webhook receiver or a Discord-like server at home. Redirects are never followed.

### What a webhook receives

One `POST` per message, with `Content-Type: application/json` and these headers:

- `X-HolyArrs-Event`: the moment, such as `import` or `test`.
- `X-HolyArrs-Delivery`: a random id for this message.
- With **What the secret does** set to **Signs each message** (the default): `X-HolyArrs-Timestamp` (seconds since 1970) and `X-HolyArrs-Signature`, which is `sha256=` followed by the hex HMAC-SHA256 of the timestamp, a dot, the delivery id, a dot and the body, keyed with your secret. Check it on your side, refuse timestamps more than a few minutes old, and refuse a delivery id you have already seen, so a copied message cannot be sent to you again.
- With **Is sent as a bearer token**: `Authorization: Bearer <secret>`.

The body:

```json
{
  "moment": "import",
  "event_kind": "import_done",
  "subject": "In the Library: Arrival (2016)",
  "text": "Import done: 1 placed by hardlink",
  "at": "2026-10-13T18:04:11Z",
  "movie": { "id": 12, "name": "Arrival", "year": 2016, "tmdb": "329865", "imdb": "tt2543164" },
  "data": { "from": "/downloads/Arrival.2016.1080p", "paths": [{ "from": "...", "to": "/movies/Arrival (2016)/Arrival (2016).mkv", "method": "hardlink" }] }
}
```

`data` holds the reasons and paths of the History row behind the message. Messages about asks name the people who asked, never their addresses. Any `2xx` answer counts as delivered.

## Health checks

HolyArrs checks itself every 5 minutes, when it starts, and straight after an import was refused for lack of space. **Home** shows anything that needs a look; **Settings**, **This app** shows every check, when it last changed, and **Check again** runs them now.

| Check | Fine | Warning | Broken |
|---|---|---|---|
| Library folder | there and writable | on another disk than the downloads (imports are copied) | gone (disk or share not mounted) or not writable |
| Free space | room to spare | under 1 GB free (a setting) | under the free-space floor, or an import was just refused for space (until the room is back) |
| Source | working, or paused by you | cannot be reached, or turned down its key | |
| Downloader | working, or paused by you | does not answer, or turned down the sign-in | |
| Backups | a recent backup | no backup yet, or the newest is older than two backup periods | the last scheduled backup failed |
| Update | this is the newest version | a newer HolyArrs is out | |

The update check only runs when **Update check link** is set (on This app). It reads that link at most every 6 hours.

### Alerts for health

An Alert with the **Health** moment hears when a check breaks and when it is fixed. To keep a Downloader that blips once from sending messages, a new result must hold for 2 runs in a row first (**Alert after this many runs in a row**, 1 to 20). Each change is told once: a Downloader that stays down sends one message, and one more when it is back.

### Settings (This app)

- **Check every**: 1 to 1440 minutes (5 by default).
- **Alert after this many runs in a row**: the flap guard above (2 by default).
- **Warn when free space is under**: in MB (1024 by default).
- **Update check link**: empty turns the update check off.

## Media servers

Open **Settings**, **Media servers**, then **Add a media server**. After HolyArrs imports, renames or deletes a movie's files, it tells the server to look at that movie's folder again, so the change shows up without waiting for the server's own scan. Nothing is read back from the server.

- **Plex**: the server address (for example `http://192.168.1.10:32400`) and the server's token. HolyArrs asks Plex to scan only that movie's folder, in the movie library that holds it. If no movie library holds the folder, every movie library is scanned.
- **Jellyfin** and **Emby**: the address (for example `http://192.168.1.10:8096`) and an API key (in the server: Dashboard, API keys). HolyArrs tells the server which folders were created, changed or deleted.

Changes are gathered for 10 seconds, so renaming a whole movie is one call. A delete is only told when it removed files.

**Tell it after** turns imports, renames and deletes on or off per server. When the server sees your movies under another path (for example in another container), set **HolyArrs path** and **Same folder on the server**: `/data/movies` here may be `/movies` there.

**Test** checks the address and the key: Plex must list at least one movie library; Jellyfin and Emby must answer with their name and version. A wrong key says "the server turned down the token". Every call is also written to History (**media server told** or **media server failed**).

Tokens and keys are kept encrypted, are never shown again, and must be typed again when the address moves to another server. Media servers may reach the local network by default, because they are almost always at home.
