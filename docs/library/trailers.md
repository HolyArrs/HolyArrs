---
title: Trailers
description: Turn trailers on or off, see where they play, and fix trailers that do not play.
level: Start
---

Turn trailers on or off over the art on Explore and title pages, and fix trailers that do not play.

## Turn trailers on or off

1. Open **Settings > Appearance**.
2. In the **For everyone** card, switch **Play trailers by themselves**.

It is on by default and applies to everyone. Off means nothing plays by itself and HolyArrs stops looking trailers up.

Trailers never play by themselves for Kid accounts, with **Reduce motion** on (hover growth and slow zoom stop too), on 2G or 3G connections, or when the browser asks to save data.

## Where trailers play

| Place | When |
|---|---|
| Explore hero | After 1.5 s in view. The hero also zooms through up to 4 backdrops |
| Poster cards | 0.8 s after the pointer rests on a card, or on keyboard focus. Esc closes |
| Artist pages | One official music video, the same way |

One video plays at a time, muted. The sound button at the bottom right turns sound on until you close the browser. If the browser blocks sound without a recent click, it stays muted.

## If it does not work

**Symptom:** nothing plays anywhere.
**Fix:** check **Play trailers by themselves** is on, the account is not a Kid account and **Reduce motion** is off. Then check **Health** for a trailer warning.

**Symptom:** one trailer never plays.
**Cause:** the video needs a sign-in or age check, is removed, private, blocked in your country, an unstarted premiere or behind a content warning. HolyArrs does not get around these. **Fix:** none needed. HolyArrs skips the video for 7 days and tries up to 4 more trailers for the title. Blocked videos and premieres are retried within the hour. With none left, the moving backdrop shows.

**Symptom:** the full-screen player says it can't play.
**Cause:** it shows the reason when known, for example that YouTube plays it only for signed-in adults.

**Symptom:** trailers stopped after working.
**Fix:** open **Health**. If it says YouTube changed how its videos are reached, update HolyArrs.

## For experts

<details class="ha-more"><summary>Where a trailer comes from</summary>

1. The metadata server lists the title's YouTube trailers: official trailer first, then teaser, your language before English. Clips never play.
2. HolyArrs reads the video's direct address from YouTube. It downloads nothing and never signs in.
3. Your browser plays that address. When it cannot, or needs it for sound, HolyArrs passes the stream through without storing it.
4. If that fails, the privacy-enhanced YouTube embed plays, then the moving backdrops.

</details>

<details class="ha-more"><summary>Lookup schedule</summary>

| Item | Value |
|---|---|
| First round after start | about 3 min |
| Round interval | 5 min |
| Lookups per round | 20, about 2 s apart |
| Retry for a title with no playable trailer | 2 h |
| Artist page kept ready | 1 week after anyone last looked |

Order in a round: addresses about to expire, then titles people opened that had nothing, then the rest of Explore for each non-Kid account. Lookups never run when you open a page. The one exception is pressing play in an artist's **Music videos** row. On a new setup the first full pass can take a few hours; until a round reaches a title, the embed plays instead.

Music videos start at the most replayed moment, ignoring the opening. If the picture barely moves there, a nearby moving moment is used. A video behind a sign-in or age check starts 30% in.

</details>

<details class="ha-more"><summary>Health warnings</summary>

**Status > Updates** and **Health** show the last good lookup. After YouTube turns away 5 different titles in a row, Health warns with a reason. The count survives restarts. With trailers off, Health leaves this out. Age-checked, sign-in or content-warning videos do not count.

| Reason | Meaning |
|---|---|
| YouTube asks to confirm you're not a bot | YouTube is checking this server |
| YouTube changed how its videos are reached | A HolyArrs update fixes it |
| YouTube turned the lookups away | YouTube answered with an error |
| YouTube could not be reached | No connection |
| Trailers are not being found | Trailers that played stopped. An update may be needed |

</details>

<details class="ha-more"><summary>Privacy</summary>

- Lookups go from your server to YouTube. The browser loads video from YouTube's servers, so YouTube sees the viewer's address.
- The embed uses `youtube-nocookie.com`.
- HolyArrs sends no names, requests or Library data to YouTube, and keeps only the video address.

</details>

## Related

- [Explore](../discover/explore.md)
- [Health checks](../run/health.md)
