---
title: Sonarr and Radarr
description: What comes over from Sonarr and Radarr, how folders are matched, and what to check after the import.
level: Setup
sidebar: { order: 2 }
---

Move your shows from Sonarr and movies from Radarr, with their folders, files and quality choices, without searching again.

## Before you start

- Have each app's address and API key (in that app's settings).
- Map your media folders into HolyArrs. The old app's folder names can differ; you match them in **What to import**.

## Import Sonarr and Radarr

1. Open **Settings > Import from other apps** and click **Add an app**.
2. Pick **Radarr** or **Sonarr**, type its address and API key, and click **Check connection**. You should see the app's name and version.
3. Start the read. Repeat for the other app.
4. In **What to import**, match each old folder to a Library folder here. You should see **Suggested** next to folders HolyArrs filled in.
5. Go to **Import**, check the counts HolyArrs shows, then click **Import**.

:::tip
Old apps often run in other containers and see folders under other names. A rule like `/data/media/movies` to `/movies` fixes every title in that folder at once.
:::

## What comes over

| From Sonarr or Radarr | In HolyArrs |
| --- | --- |
| Library folders | Library folders for movies and shows. |
| Movies and shows | Titles, matched by TMDB or TVDB id. Folders and files stay where they are. A watched title gets **Keep up to date**; seasons that were not watched stay off. Nothing is searched for during the move. |
| Quality profiles | Each one matched to the closest **Pick rules** preset (Standard, 4K). The **Import** step shows why. Titles use the preset of their profile. |
| Exclusions | The **Never-add** list. |
| Blocklist | The **Turned down** list of each title that comes over, so those releases are never taken again. A show's release is turned down for the exact episodes Sonarr kept for it; when those can't be read, for the episodes its name gives (season and episode, anime number or air date). A name that gives no episode is listed as not moved instead of turning down the whole show. Each one is kept under the Source with the old indexer's name, or else the first Source of its protocol, so add or link your Sources first. |
| Radarr's release stage per movie, and its availability delay | Each movie's **Release gate** and gate delay, up to 365 days. Your default for new movies stays. |
| The untagged default wait rule | The preferred protocol and Usenet and torrent waits on the matched Pick rules, up to 7 days. See [Waits](#waits). |
| Radarr TMDB lists, Trakt lists and Trakt watchlists | **Outside lists** for movies, added paused, with folder, Pick rules, Keep up to date, search on add, Release gate and read interval (6 to 168 hours). A Trakt list needs your Trakt key; a private one also needs **Sign in with Trakt**. |
| Sonarr Trakt lists and Trakt watchlists | **Outside lists** for shows, added paused, with folder, Pick rules, seasons, new seasons, search on add and read interval. A Trakt list needs your Trakt key; a private one also needs **Sign in with Trakt**. **Pilot** keeps the first season. A list that both apps read becomes two lists, one for movies and one for shows; the Sonarr one gets " (sonarr)" after its name. |
| Torznab, Newznab and RSS indexers | **Sources**, added paused and blocked from your home network. |
| Download clients (qBittorrent, SABnzbd, Transmission, Deluge, rTorrent, aria2, NZBGet) | **Downloaders**, added paused and blocked from your home network. They use the `holyarrs` category, so they never pick up the old apps' downloads. For rTorrent, the old directory, priority, add-stopped and post-import label are not used. For aria2, the RPC path is read and switched to `/jsonrpc`, the secret token must be typed again, and the old directory is not used. |
| Remote path mappings | **Different folder names** on the Downloader at that host, up to 100 pairs. A Downloader that was already here is left alone, and its pairs are listed for you to add by hand. A pair whose folder HolyArrs cannot see yet is still added and marked "not visible here yet (check your mounts)". |
| Naming | HolyArrs naming, where a pattern word has a match. Nothing on disk is renamed. |
| "Unmonitor deleted episodes" and "Unmonitor deleted movies" | **Stop looking for deleted episodes** and **Stop looking for deleted movies** under **Settings > Naming and files**, listed with Naming. |
| Discord, webhook and email notifications | **Alerts**, off until you type the link, address or mail password. |

An indexer is a site that lists releases; in HolyArrs it is a **Source**.

### The same indexer in more than one app

Each site comes over once, however many apps had it:

- The same Torznab, Newznab or RSS address in Sonarr and Radarr is one Source for shows and movies, with each app's categories. Letter case, the default port and a trailing `/` do not matter.
- An indexer that Prowlarr put in Sonarr or Radarr (its address is Prowlarr's, like `http://prowlarr:9696/5/api`, and its name ends in "(Prowlarr)") is the same Source as that indexer in Prowlarr, when you import Prowlarr too and the indexer there is a Torznab, Newznab or RSS one. The Source is the one from Prowlarr, at the feed's own address. It is still the same when Sonarr or Radarr reach Prowlarr under another host name, as long as the number and name match.
- Prowlarr can be imported in the same import, before or after. A Source already here at the same address, or named like the indexer with or without "(Prowlarr)", is listed as already here. Nothing is added twice.

The **Import** step shows it on the Source: "the same as Nyaa.si (Prowlarr) in Sonarr and Radarr: one Source".

## Choices

### Folders

In **What to import**, each old folder that is not a Library folder here gets a row. Pick the matching Library folder, or **Other path…** and type one. When the old folder is missing here but a Library folder with the same last name exists, HolyArrs fills it in as **Suggested**.

Under **Advanced**, add path rules. The longest matching rule wins. Titles whose folder is still not found are listed with the reason; fix the rule and import again. Nothing is added twice.

### Waits

The old default wait rule comes over only when it differs from that app's own default, and only while those Pick rules keep the waits they came with. "Skip the wait at the top quality" turns on **Skip the wait once good enough**. If two apps set different waits for the same Pick rules, neither is used. The **Import** step shows the change and how many versions use those Pick rules.

## What stays behind

Custom formats, tag-scoped wait rules (listed with their waits, so you can set them under Pick rules), release word rules, language profiles, tags, list kinds HolyArrs does not read (IMDb, Plex, another Radarr or Sonarr, Trakt popular, watched or collection lists), MDBList lists (add them again under **Settings > Outside lists**), metadata writers, download categories and notification kinds other than Discord, webhook and email. Each is on the **not moved** list with its reason. Auto-tag rules and history are not read.

## If it does not work

| Symptom | Cause | Fix |
| --- | --- | --- |
| **Check connection** fails | The address works from your computer but not from the HolyArrs server. | Use the address the server can reach. For a home-network app, turn on the local network switch. |
| Titles show as not found | No path rule matches their folder. | Add a rule under **Advanced** in **What to import**, then import again. |
| A path pair says "not visible here yet (check your mounts)" | HolyArrs cannot see that folder. | Add the mount to the container. The pair is already saved. |

## For experts

<details class="ha-more"><summary>Remote path mappings in detail</summary>

A Downloader that was already here is left alone; its pairs are listed for you to add by hand. A pair whose folder HolyArrs cannot see yet is still added and marked "not visible here yet (check your mounts)".

</details>

## Related

- [Import from other apps](index.md)
- [Read your old apps](what-is-read.md)
- [Lidarr, Bazarr and Prowlarr](lidarr-bazarr-prowlarr.md)
