---
title: Profilarr
description: Let Profilarr push its formats, profiles and settings into HolyArrs, or import your Sonarr and Radarr formats one to one.
level: Setup
sidebar: { order: 7 }
---

Keep using Profilarr to manage how releases are picked. HolyArrs takes what it pushes as if it were Sonarr and Radarr.

You can also skip Profilarr and bring the formats and profiles straight from Sonarr and Radarr. See [Import formats from Sonarr and Radarr](#import-formats-from-sonarr-and-radarr).

## What things are called here

| In Profilarr, Sonarr and Radarr | In HolyArrs |
| --- | --- |
| Custom format | A trait (**Your traits**) |
| Quality profile | Pick rules |
| Format score | Points for a trait in Pick rules |
| Delay profile | The wait and preferred protocol in Pick rules |
| Quality size limits | Size limits per quality step in Pick rules |

## Before you start

- Sign in as the owner or an admin.
- Profilarr must reach HolyArrs on your network.

## Connect Profilarr

1. Open **Settings > API**. On the **Requests from Seerr and Overseerr** card, switch on **Answer Seerr and Overseerr**. You should see a row for **Shows** and one for **Movies**.
2. Click **Make key** next to **Shows**. Copy the key now; it is shown once.
3. Click **Make key** next to **Movies**. Copy that key too.
4. In Profilarr, add a Sonarr server. Use HolyArrs's address and port with `/sonarr` at the end, such as `http://your-server:6262/sonarr`, and the **Shows** key.
5. In Profilarr, add a Radarr server with `/radarr` at the end and the **Movies** key.
6. Push from Profilarr. You should see new Pick rules named like the profiles, and new traits under **Your traits**.

Use the address the card shows. Under a URL base it looks like `/holyarrs/sonarr`.

The key acts as the person who made it. That person must be allowed to change settings, or every push is refused.

## What Profilarr can push

| Profilarr sends | What HolyArrs does |
| --- | --- |
| Custom formats | Keeps each one as sent and turns it into a trait. A format testing something HolyArrs cannot check is kept but becomes no trait. |
| Quality profiles | Makes Pick rules for each one: allowed qualities, the cutoff, scores and language. Sonarr profiles carry no language, so every language is allowed. |
| Default delay profile | Sets the preferred protocol, both waits and the skip-wait switch on the Pick rules sent to that side. |
| Quality sizes | Sets size limits per quality step on those Pick rules. When several old qualities share one step, the widest limits win. |
| Media management | Only the repack choice is used. It sets the re-release rule. |
| Naming | Sets the rename switch and the colon, odd-character and multi-episode choices, for shows and movies. |

Profilarr reads back exactly what it sent, so it sees no changes to push again.

## What is kept and what changes

- **Pick rules you made in HolyArrs**, and the ready-made ones, are never changed or removed by Profilarr. Profilarr can see them, but trying to change one is refused.
- **Pushed Pick rules** take the profile's name. If that name is taken, HolyArrs adds the side or a number.
- **Removing a profile** in Profilarr removes its Pick rules, unless titles, Library folders, lists or settings still use them. Then they stay.
- **Only formats sent through Profilarr** are listed or removed. Traits you made in HolyArrs stay.

## Import formats from Sonarr and Radarr

No Profilarr? Bring the formats and profiles over once.

1. Open **Settings > Import from other apps** and read Sonarr or Radarr. See [Sonarr and Radarr](sonarr-radarr.md).
2. At **What to import**, leave **Pick rules** on. You should see **Bring formats and scores** under it.
3. Leave **Bring formats and scores** on. Each profile that scores formats becomes its own Pick rules, with those formats as traits.
4. Import. Each set of Pick rules lists **Traits** it got and what was **Left out**.

## Limits

| Limit | What happens |
| --- | --- |
| A format tests something HolyArrs cannot check | It is kept but becomes no trait. |
| More than 64 scored formats in one profile | Pick rules hold at most 64 traits. The strongest scores are kept and the weakest are left out. |
| Very large scores | Kept exactly, over the same range Sonarr and Radarr allow. |
| Extra delay profiles | Kept and listed back, but they change nothing. Only the default one is used. |
| Rename patterns | Kept and listed back, but HolyArrs names files its own way. |
| Other media management settings | Kept and listed back, but they change nothing. |
| Music | Profilarr's Lidarr side is not answered. |

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| Profilarr gets 404 | **Answer Seerr and Overseerr** is off, or the address lacks `/sonarr` or `/radarr`. | Switch it on and copy the address from the card. |
| Every push is refused | The key's maker may not change settings, or their account is off. | Make a new key as the owner or an admin. |
| A profile is not removed | Titles, Library folders, lists or settings still use its Pick rules. | Move them to other Pick rules, then push again. |
| A profile cannot be changed | Those Pick rules were made in HolyArrs. | Change them in HolyArrs, or push a profile with a new name. |
| A format has no effect | It tests something HolyArrs cannot check. | Make a trait for it under **Your traits** instead. |

## Related

- [Old-app API](../reference/old-app-api.md)
- [Sonarr and Radarr](sonarr-radarr.md)
- [Import from other apps](index.md)
