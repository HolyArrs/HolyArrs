---
title: Subtitle scoring and sites
description: How subtitles are scored against the Pass mark, when HolyArrs searches, and how Subtitle sites rest and sign in.
level: Expert
sidebar: { order: 4 }
---

How HolyArrs scores a subtitle, when it searches, and how each Subtitle site behaves.

For setup, see [Subtitles](subtitles.md). Subtitle sites are held back in the first release, see [Held back](../help/held-back.md).

## The Pass mark

Each subtitle gets a score from 0 to 100. A search saves one by itself only at or above the Pass mark. A hand choice may have any score.

| Setting | Default | Range |
|---|---|---|
| Pass mark, episodes | 90 | 0 to 100 |
| Pass mark, movies | 70 | 0 to 100 |

A lower mark takes looser matches, which are more often out of time. A higher mark waits longer for a close match.

Points come from:

- the right name and year, and for an episode the right season and episode,
- the same release group, source, resolution, video codec, audio and edition as your file,
- the hearing-impaired and forced choice you asked for,
- a trusted uploader,
- a frame rate hint, worth 1 point. HolyArrs does not measure your video's frame rate, so this rarely adds anything.

A subtitle for another title, year, season or episode scores 0.

## When HolyArrs searches

| Trigger | Default | Range |
|---|---|---|
| New file, from an import or a better copy | searched at once | |
| Files still missing a subtitle | every 6 hours | 1 to 168 hours |
| **Files per run, at most** | 100 | 1 to 1000 |
| **Slow down after three weeks** | not found in 3 weeks, then searched once a week | |
| **Look for better subtitles** | every 12 hours, subtitles saved in the last 7 days that scored under 99 | |

A better subtitle must score higher than the one it replaces.

## Other settings

| Setting | What it does | Default |
|---|---|---|
| **Count subtitles inside the video** | A language already inside the file counts as there. | On |
| **Leave out picture subtitles** | Picture tracks from discs do not count, since many players can't show them. | Off |
| Hearing-impaired file ending | `.hi`, `.cc` or `.sdh` | `.hi` |
| **Sync fix** starting step | Step the Sync fix dialog starts with. | 500 ms |
| **Sync fix** frame rates | Rates the dialog starts with. | 25 to 23.976 |

## Subtitle sites

Sites are asked lowest priority number first.

| State | Meaning |
|---|---|
| **On** | Asked on every search. |
| **Paused** | You paused it. Click **Turn on** to bring it back. |
| **Resting until** | It failed. HolyArrs tries again after that time. |
| **Needs your account** | It will not run until you fill in your details. |

**Test** runs one search for a known title. A failed test never pauses the site.

**Limits** set the wait between calls, the most calls in a window, and downloads a day (`0` means no cap). **Reach the local network** is only for a site running on your own network.

### How long a site rests

| Why | Rest |
|---|---|
| Did not answer | 20 minutes, doubling up to 6 hours |
| Asked HolyArrs to slow down | the time the site gives, up to a day, or 1 hour |
| Reached its download limit | until the site's reset, or midnight UTC |
| Sent an answer HolyArrs could not read | 6 hours |
| Refused your account details, or showed an anti-bot check | 12 hours |

If it refused your details, fix them and save. That turns it back on.

### Sites that need your account

Some sites only work with your own account: a user name and password, or an API key. The fields are masked and stored encrypted, and never shown again. Leave them empty when you edit the site to keep the stored ones. Moving the site to another address asks for them again. Removing the site forgets them. Account details never appear in the log, in Events or in error messages.

| Site | Covers | Account | Notes |
|---|---|---|---|
| OpenSubtitles.com | movies and shows | API key, user name and password | Searches use the key; downloads sign in. Matches by file hash, then IMDb or TMDB id, then name. Machine translations are left out. |
| subdl | movies and shows | API key | Searches by TMDB id, IMDb id or name. The download count resets at midnight UTC. |
| Gestdown | shows | none | Show subtitles from Addic7ed. |
| Podnapisi | movies and shows | none | Starts turned off because the site does not answer now. Turn it on when it does. |
| Addic7ed (direct) | shows | user name and password | At least 3 s between calls and 100 calls a day by default. Unfinished subtitles are left out. HolyArrs never tries to get past a captcha. |

The account sites have only been tested against stand-in servers. Run **Test** before your first real search.

## For scripts

Every subtitle route is in `http://your-server:6262/api/v1/openapi.json`, under the Subtitles tag. Searches answer with the ids of their Jobs, so a script can follow them. See [API routes](../../reference/api-routes.md).

## Related

- [Subtitles](subtitles.md)
- [Held back](../help/held-back.md)
