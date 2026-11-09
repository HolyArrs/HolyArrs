---
title: Alerts
description: Send Discord or email messages when a request waits, a title lands in the Library or a health check breaks.
level: Setup
sidebar: { order: 5 }
---

An Alert sends a Discord or email message when something happens: a request waits, a movie lands, a health check breaks. You need the **Change settings** permission.

## Add an Alert

1. Open **Settings > Alerts** and select **Add an Alert**.
2. Give it a **Name** and pick a **Kind**: Discord or email.
3. Fill in the kind's fields (see [Kinds](#kinds)).
4. Under **When to send**, untick the moments you do not want. All start ticked; **All of these** and **None of these** work on a whole group.
5. Leave **Send messages** on and select **Save**.
6. Select **Send a test** on the card. You should see the message arrive and the card show **Last sent**.

## Kinds

### Discord

In Discord, open the channel settings, then **Integrations > Webhooks**, and copy the link. Paste it into **Discord link**; it starts with `https://discord.com/`. People who added their Discord user ID to their account are mentioned in messages about their requests.

### Email

| Field | What to enter |
| --- | --- |
| **Mail server**, **Port** | Your mail server |
| **Security** | STARTTLS (port 587), TLS (port 465) or None. None only for a mail server on your own network |
| **From address** | The sender address |
| **Mail username**, **Mail password** | The mail account |
| **Staff addresses** | One or more, split by commas |

Messages about a request go to the people who asked, at their account address. Everything else goes to the staff addresses.

Telegram, ntfy, Gotify, Pushover and webhooks are held back in this release. See [Held-back features](../run/updates.md#held-back-features).

## Choices

### When to send

| Moment | Goes to | When |
| --- | --- | --- |
| **New request** | Staff | A movie, show or artist waits for approval |
| **Approved** | Requesters | Their request was approved |
| **Declined** | Requesters | Declined, with your note |
| **Ready** | Requesters | Files are in the Library |
| **New episodes** | Requesters and followers | At most 1 per show per hour. Each person can turn these off |
| **New albums** | Artist followers | At most 1 per artist per hour |
| **Problem reported** | Staff | Someone reported a title. Reports are at `/requests/reports` |
| **Problem fixed** | Reporter | With your reply |
| **Sent to download** | Staff | A release went to a downloader |
| **In the Library** | Staff | Files were placed, including a better copy |
| **Renamed** | Staff | Files got new names |
| **Added** | Staff | A title was added by hand, from a list or from a request |
| **Deleted** | Staff | A movie, show, season or episodes left the Library |
| **Health** | Staff | A [health check](../run/health.md) broke or is fixed |
| **Needs a decision** | Staff | A download or import needs you |

### Season packs

Below the list, **Season packs** decides what one download with many episodes sends: **One Alert for the pack**, with a line per episode, or **One Alert per episode**.

### Reach the local network

Off by default. While off, Discord Alerts go only to discord.com and other kinds only to internet servers; links to this machine or your home network are refused, even after a name lookup. Turn it on for a mail server or receiver at home. Redirects are never followed.

### Pause an Alert

Untick **Send messages**. The card shows **Off** and the Alert stays.

### Passwords and tokens

Stored encrypted, then shown as **Stored**. Leave the field empty to keep it. Point an Alert at another server and you type the secret again; a stored secret never goes to a new place.

## If it does not work

**Symptom:**
> Paste the Discord link

**Cause:** the link does not start with `https://discord.com/`.
**Fix:** copy the webhook link again. For a Discord-like server at home, turn on **Reach the local network**.

**Symptom:**
> Points to the local network, which this Alert may not reach

**Cause:** the server is on this machine or your home network.
**Fix:** turn on **Reach the local network**.

**Symptom:** **Last try failed** with an answer of 401 or 404.
**Cause:** the link or token is wrong. These are not retried.
**Fix:** correct it and select **Send a test**.

**Symptom:** the person who asked got nothing.
**Cause:** no email address or Discord user ID on their account, the moment is unticked, or they turned off new episodes.
**Fix:** check those three.

## For experts

<details class="ha-more"><summary>Retries</summary>

A timeout, a slow-down answer or a server error is retried after 5 s, 30 s and 2 min. A longer wait asked by the server is honoured, up to 5 min. A refusal, such as a wrong link, is not retried. Each failure adds an "Alert could not be sent" row to History.

</details>

<details class="ha-more"><summary>Held-back kinds and their fields</summary>

Shown only with the `alert_channels_more` switch on.

| Kind | Fields |
| --- | --- |
| Telegram | **Bot token** from @BotFather, **Chat ID** (user, group or channel ID, or `@channel`). Message the bot or add it to the group first |
| ntfy | **Server address** (empty means ntfy.sh), **Topic**, optional **Access token**. Anyone who knows the topic can read it |
| Gotify | **Server address**, **App token** |
| Pushover | **User key** (group keys work), **App token** |
| Webhook (JSON) | **Webhook link**, optional **Secret**, **What the secret does** |

</details>

<details class="ha-more"><summary>What a webhook receives</summary>

One `POST` per message, `Content-Type: application/json`, with these headers:

| Header | Value |
| --- | --- |
| `X-HolyArrs-Event` | The moment, such as `import` or `test` |
| `X-HolyArrs-Delivery` | Random ID for this message |
| `X-HolyArrs-Timestamp` | With **Signs each message**: seconds since 1970 |
| `X-HolyArrs-Signature` | With **Signs each message**: `sha256=` plus hex HMAC-SHA256 of `<timestamp>.<delivery>.<body>`, keyed with your secret |
| `Authorization` | With **Is sent as a bearer token**: `Bearer <secret>` |

With **Nothing**, no secret is sent. When signing, refuse old timestamps and delivery IDs you have seen, so a copied message cannot be replayed.

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

Other fields appear when they apply: `test`, `show` and `episodes`, `artist` and `tracks`, `report`, `people`. `people` holds requester names, never addresses. `data` holds the reasons and paths from the History row behind the message. Any `2xx` answer counts as delivered.

</details>

## Related

- [Health checks](../run/health.md)
- [Approving requests](../requests/approving.md)
- [Users and roles](roles.md)
