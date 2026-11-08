# Alerts

You want HolyArrs to tell you, your staff or the people who asked for something when it happens. An Alert does that. It sends a message to Discord or email when a request waits, a movie lands in the Library, a health check breaks, and more.

You need the settings permission to add or change Alerts.

## Add an Alert

1. Open **Settings**, then **Alerts**.
2. Click **Add an Alert**.
3. Give it a **Name** and pick a **Kind**.
4. Fill in the fields for that kind (see below).
5. Under **When to send**, tick the moments you want.
6. Leave **Send messages** on and click **Save**.
7. Click **Send a test** on the card to check that it arrives.

A new Alert starts with every moment ticked. Untick what you do not want, or use **All of these** and **None of these** for a whole group.

## Kinds

Discord and email are on in this release. Telegram, ntfy, Gotify, Pushover and webhooks are held back. They only show up when the `alert_channels_more` switch is on. See [Held back](../help/held-back.md).

### Discord

Paste the channel's webhook link into **Discord link**. In Discord, open the channel settings, then Integrations, Webhooks, and Copy link. The link starts with `https://discord.com/`.

If a person added their Discord user ID to their account, messages about their requests mention them by name.

### Email

- **Mail server** and **Port**.
- **Security**: STARTTLS (port 587), TLS (port 465), or None. Use None only for a mail server on your own network.
- **From address**.
- **Mail username** and **Mail password**.
- **Staff addresses**: one or more addresses, split by commas.

Messages about a request go to the people who asked, at the address in their account. Everything else goes to the staff addresses.

### Held-back kinds

These work the same way once the switch is on:

- **Telegram**: a **Bot token** from @BotFather and a **Chat ID** (a user, group or channel ID, or `@channel`). Message the bot or add it to the group first.
- **ntfy**: a **Server address** (empty means ntfy.sh), a **Topic**, and an optional **Access token**. Anyone who knows the topic name can read it.
- **Gotify**: a **Server address** and an **App token**.
- **Pushover**: your **User key** (group keys work too) and an **App token**.
- **Webhook (JSON)**: a **Webhook link**, an optional **Secret**, and **What the secret does**. See [What a webhook receives](#what-a-webhook-receives).

## Settings explained

### When to send

**Requests** moments:

- **New request**: to the staff, when a movie, show or artist waits for approval.
- **Approved**: to the people who requested it.
- **Declined**: to the people who requested it, with your note.
- **Ready**: to the people who requested it, once the files are in the Library.
- **New episodes**: to the people who requested or follow a show. At most one message per show per hour. Each person can turn these off in their account.
- **New albums**: to the people who follow an artist. At most one message per artist per hour.
- **Problem reported**: to the staff, when someone reports a problem with a title. Reports live at `/requests/reports`.
- **Problem fixed**: to the person who reported it, with your reply.

**Library** moments, all to the staff:

- **Sent to download**: a release went to a downloader.
- **In the Library**: files were placed. A better copy counts too.
- **Renamed**: files got new names.
- **Added**: a movie or show was added by hand, from a list, or from an approved request.
- **Deleted**: a movie, a show, or a season or episodes left the Library.
- **Health**: a health check broke, or is fixed again. See [Health checks](../health/health-checks.md).
- **Needs a decision**: a download or an import needs you.

### Season packs

Below the Alerts list, **Season packs** decides what happens when one download brings several episodes. Pick **One Alert for the pack**, which lists every episode on its own line, or **One Alert per episode**.

### Reach the local network

Off by default. While it is off, a Discord Alert only sends to discord.com, and other kinds only send to servers on the internet. A link to this machine or your home network is refused, also after the name is looked up. Turn it on for a mail server, webhook receiver or ntfy server at home. Redirects are never followed.

### Send messages

Untick it to pause an Alert without removing it. The card then shows **Off**.

### Passwords, tokens and secrets

They are stored encrypted and never shown again. The form shows **Stored** instead. Leave the field empty to keep what is stored. If you point an Alert at another server, type the secret again. HolyArrs does not send a stored secret to a new place.

## Sending and retrying

**Send a test** sends one message right away and tells you if it arrived. After that the card shows **Last sent** and when, or **Last try failed** with the reason.

When a real message fails because the server timed out, asked to slow down, or had an error, HolyArrs tries again after 5 seconds, 30 seconds and 2 minutes. If the server asks for a longer wait, HolyArrs waits that long, up to 5 minutes. When the server refuses the message, for example a wrong link, it does not try again. Each failure also adds an "Alert could not be sent" row to History.

## What a webhook receives

Webhooks are held back. This section is for when the switch is on.

HolyArrs sends one `POST` per message with `Content-Type: application/json` and these headers:

- `X-HolyArrs-Event`: the moment, such as `import` or `test`.
- `X-HolyArrs-Delivery`: a random ID for this message.
- With **Signs each message**: `X-HolyArrs-Timestamp` (seconds since 1970) and `X-HolyArrs-Signature`. The signature is `sha256=` followed by the hex HMAC-SHA256 of `<timestamp>.<delivery>.<body>`, keyed with your secret. Check it on your side. Refuse old timestamps and delivery IDs you have already seen, so a copied message cannot be replayed.
- With **Is sent as a bearer token**: `Authorization: Bearer <secret>`.
- With **Nothing**: no secret is sent.

Example body:

```json
{
  "moment": "import",
  "event_kind": "import_done",
  "subject": "In the Library: Arrival (2016)",
  "text": "Import done: 1 placed by hardlink",
  "at": "2025-01-01T18:04:11Z",
  "movie": { "id": 12, "name": "Arrival", "year": 2016, "tmdb": "329865", "imdb": "tt2543164" },
  "data": { "from": "/downloads/Arrival.2016.1080p" }
}
```

Other fields show up when they apply: `test`, `show` and `episodes`, `artist` and `tracks`, `report`, and `people`. `people` holds the names of the people who asked, never their addresses. `data` holds the reasons and paths from the History row behind the message. Any `2xx` answer counts as delivered.

## Troubleshooting

**"Paste the Discord link"**: the link must start with `https://discord.com/`. For a Discord-like server at home, turn on **Reach the local network**.

**"Points to the local network, which this Alert may not reach"**: the server is on this machine or your home network. Turn on **Reach the local network**.

**Last try failed: ... answered 401 or 404**: the link or token is wrong. Fix it and send a test. HolyArrs does not retry these.

**The person who asked got nothing**: check that they set an email address or Discord user ID in their account, and that the Alert has the moment ticked. For new episodes, check that they did not turn those off.

**I cannot pick Telegram, ntfy, Gotify, Pushover or Webhook**: those kinds are held back. See [Held back](../help/held-back.md).
