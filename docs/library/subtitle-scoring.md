---
title: Subtitle scoring and sites
description: How subtitles are scored against the Pass mark, when HolyArrs searches, and how Subtitle sites rest and sign in.
level: Expert
sidebar: { order: 4 }
---

How HolyArrs scores a subtitle, when it searches, and how each Subtitle site behaves.

For setup, see [Subtitles](subtitles.md). Subtitle sites are held back in the first release, see [Held-back features](../run/updates.md#held-back-features).

## Change the Pass mark

A search saves a subtitle by itself only at or above the Pass mark. A hand choice may have any score.

1. Open **Settings > Subtitles**.
2. Set the Pass mark for episodes and for movies, each from 0 to 100.
3. Click **Save**. You should see the new marks used the next time a search runs.

A lower mark takes looser matches, which are more often out of time. A higher mark waits longer for a close match. Defaults are 90 for episodes and 70 for movies.

## What the score counts

Each subtitle gets 0 to 100 points for:

- the right name and year, and for an episode the right season and episode,
- the same release group, source, resolution, video codec, audio and edition as your file,
- the hearing-impaired and forced choice you asked for,
- a trusted uploader,
- a frame rate hint, worth 1 point. HolyArrs does not measure your video's frame rate, so this rarely adds anything.

A subtitle for another title, year, season or episode scores 0.

## Subtitle sites

Sites are asked lowest priority number first.

| State | Meaning |
|---|---|
| **On** | Asked on every search. |
| **Paused** | You paused it. Click **Turn on** to bring it back. |
| **Resting until** | It failed. HolyArrs tries again after that time. |
| **Needs your account** | It will not run until you fill in your details. |

**Test** runs one search for a known title. A failed test never pauses the site.

:::caution
Some sites only work with your own account. Account details are stored encrypted and never shown again. Leave the fields empty when you edit the site to keep the stored ones. Moving the site to another address asks for them again. Removing the site forgets them. They never appear in the log, in Events or in error messages.
:::

## If it does not work

**Symptom:** a site says **Resting until**.
**Cause:** it failed. The rest time depends on why, see How long a site rests below.
**Fix:** wait, or fix your account details and save. Saving turns the site back on.

**Symptom:** a site says **Needs your account**.
**Fix:** open the site and fill in the details it asks for.

**Symptom:** a test fails.
**Cause:** the account details are wrong or the site is down. A failed test never pauses the site.

## For experts

<details class="ha-more"><summary>When HolyArrs searches</summary>

| Trigger | Default | Range |
|---|---|---|
| New file, from an import or a better copy | searched at once | |
| Files still missing a subtitle | every 6 hours | 1 to 168 hours |
| **Files per run, at most** | 100 | 1 to 1000 |
| **Slow down after three weeks** | On: not found in 3 weeks, then searched once a week | On or Off |
| **Look for better subtitles** | On: every 12 hours, subtitles saved in the last 7 days that scored under 99 | every 6 to 168 hours, age 0 to 30 days |

A better subtitle must score higher than the one it replaces.

</details>

<details class="ha-more"><summary>Other settings</summary>

| Setting | What it does | Default | Range |
|---|---|---|---|
| **Count subtitles inside the video** | A language already inside the file counts as there. | On | |
| **Leave out picture subtitles** | Picture tracks from discs do not count, since many players can't show them. | Off | |
| Hearing-impaired file ending | Ending on saved files. | `.hi` | `.hi`, `.cc`, `.sdh` |
| **Sync fix** starting step | Step the Sync fix dialog starts with. | 500 ms | 10 to 60000 ms |
| **Sync fix** frame rates | Rates the dialog starts with. | 25 to 23.976 | 1 to 120 each |

</details>

<details class="ha-more"><summary>Site limits and Reach the local network</summary>

**Limits** set the wait between calls, the most calls in a window, and downloads a day.

| Limit | Default | Range |
|---|---|---|
| Wait between calls | 1000 ms | 0 to 60000 ms |
| Calls per window | 60 | 1 to 10000 |
| Window length | 3600 s | 60 to 86400 s |
| Downloads a day | no cap (`0`) | 0 to 100000 |

**Reach the local network** is only for a site running on your own network.

</details>

<details class="ha-more"><summary>How long a site rests</summary>

| Why | Rest |
|---|---|
| Did not answer | 20 minutes, doubling up to 6 hours |
| Asked HolyArrs to slow down | the time the site gives, up to a day, or 1 hour |
| Reached its download limit | until the site's reset, or midnight UTC |
| Sent an answer HolyArrs could not read | 6 hours |
| Refused your account details, or showed an anti-bot check | 12 hours |

</details>

<details class="ha-more"><summary>Sites and accounts</summary>

| Site | Covers | Account | Notes |
|---|---|---|---|
| OpenSubtitles.com | movies and shows | API key, user name and password | Searches use the key; downloads sign in. Matches by file hash, then IMDb or TMDB id, then name. Machine translations are left out. |
| subdl | movies and shows | API key | Searches by TMDB id, IMDb id or name. The download count resets at midnight UTC. |
| Gestdown | shows | none | Show subtitles from Addic7ed. |
| Podnapisi | movies and shows | none | Starts turned off because the site does not answer now. Turn it on when it does. |
| Addic7ed (direct) | shows | user name and password | At least 3 s between calls and 100 calls a day by default. Unfinished subtitles are left out. HolyArrs never tries to get past a captcha. |

The account sites have only been tested against stand-in servers. Run **Test** before your first real search.

</details>

<details class="ha-more"><summary>API</summary>

Every subtitle route is in `http://your-server:6262/api/v1/openapi.json`, under the Subtitles tag. Searches answer with the ids of their Jobs. See [API routes](../reference/api-routes.md).

</details>

## Related

- [Subtitles](subtitles.md)
- [Held-back features](../run/updates.md#held-back-features)
