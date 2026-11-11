---
title: TRaSH Guides scores
description: Apply a TRaSH Guides profile to Pick rules so its release scores become points, and sync again when the guides change.
level: Expert
---

Use the scores from a [TRaSH Guides](https://trash-guides.info) profile as points in your Pick rules. Each Release gets the sum of the scores that fit its name, and the higher total wins.

## Before you start

- You can change settings.
- The Pick rules set is for movies or shows, not music.
- HolyArrs can reach `github.com`. The profiles are read from the TRaSH Guides repository each time you open the list (kept for an hour).

## Apply a profile

1. Open **Settings > Pick rules** and open a set. Save any changes first.
2. In **TRaSH Guides**, pick **Shows** or **Movies**, then **Show TRaSH profiles**.
3. Pick a profile and select **Apply profile**.

HolyArrs then:

- makes one trait for each scored TRaSH entry it can test, listed under **Your traits > From TRaSH Guides**,
- puts those traits in **Traits and points** with the TRaSH score as points,
- sets **Fewest points a Release needs**, the good enough points and **Points a better copy must add** to the profile's numbers.

Points you gave built-in traits or your own traits stay as they are. Your quality steps and **Good enough at** stay too; set them to match the profile yourself.

## Sync again

Select **Sync again** on the set. HolyArrs reads the profile again, updates the traits it made earlier in place and replaces the points from the last sync. Nothing is made twice. A trait synced from TRaSH Guides can't be changed by hand; remove it if you don't want it.

## Check the scores

Open **Choose a release** on a title. Each Release shows its points and which traits gave them, including the synced ones.

## What is left out

After applying, **left out** lists every TRaSH entry HolyArrs didn't take, and why. An entry is taken whole or not at all, so a left-out entry never scores half.

| TRaSH test | Taken? |
|---|---|
| Release name pattern | Yes |
| Release group pattern | Yes |
| Source (TV, WEB-DL, WEBRip, DVD, Blu-ray, remux) | Yes, read from the name |
| Resolution | Yes, read from the name |
| Remux or full disc (movies) | Yes, as a source |
| Language | Not yet |
| Indexer flags | Not yet |
| Release type (single episode or season pack) | Not yet |
| Size (min and max in GB) | Yes, on the whole Release's size from the search result; a Release with no size given doesn't fit |
| Year and other kinds | Not yet |

TRaSH entries with no score in the profile are skipped quietly, since they change nothing. Optional TRaSH groups (the ones a profile doesn't include by default) aren't added; give those traits points yourself after a sync if you want them.

Pick rules hold at most 64 traits. If a sync would go over, the strongest scores are kept and the rest are listed.

## Problems

| You see | Do this |
|---|---|
| TRaSH Guides could not be reached | Check that HolyArrs can reach `github.com`, then try again. |
| TRaSH Guides answered 403 | GitHub limits how often one address may list files. Wait an hour and try again. |
| **Apply profile** is greyed out | Save or undo your changes to the set first. |

## For experts

| Route | What it does |
|---|---|
| `GET /api/v1/trash/profiles?app=sonarr` | Lists profiles (`sonarr` for shows, `radarr` for movies). |
| `POST /api/v1/pick-rules/{id}/trash` | Applies `{"app": "sonarr", "profile_id": "<TRaSH id>"}`. Send it again to re-sync. |

The answer lists `scored` (trait key, name, points), `skipped` (name, test kinds, reason), and how many traits were `added` and `updated`. The set's `body.synced` holds the profile last applied.

A synced trait keeps its TRaSH id, so a sync finds it again even after you rename the profile. Its patterns are read the way TRaSH writes them, look-arounds included, with a time limit per test.

TRaSH Guides is MIT licensed. HolyArrs reads it at the moment you ask; nothing from it ships inside HolyArrs.
