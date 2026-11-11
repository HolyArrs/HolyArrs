---
title: Connect
description: Connect one downloader and one source so HolyArrs can find releases and send them to download.
level: Start
sidebar: { order: 4, label: "Step 4 of 5: Connect" }
---

Connect one downloader and one source. When both pass **Test**, HolyArrs can find and fetch titles.

HolyArrs does not download by itself. A **downloader** (qBittorrent, SABnzbd and others) fetches files. A **source** is a feed HolyArrs searches for releases, usually through Prowlarr or Jackett.

## Before you start

- A downloader running and reachable from HolyArrs, with its address and password or API key.
- Prowlarr or Jackett with at least one site set up, and its API key.

## Connect a downloader

1. Open **Settings > Downloaders** and press **Add a Downloader**.
2. Under **Which app**, pick your app and type a **Name**.
3. Type the **Address** of its web page, such as `http://192.168.1.10:8090`.
4. Fill in its sign-in:

   | App | Sign-in |
   |---|---|
   | qBittorrent | **API key** (5.2 or newer, in Options > Web UI) or **Username and password** |
   | SABnzbd | API key, in Config > General |
   | Transmission | Username and password, empty if it asks for none. Version 4 or newer |
   | Deluge | Web UI password, no username |
   | rTorrent | Username and password of the web server in front of it, empty if it asks for none. The address is its XML-RPC link, such as `http://192.168.1.10:8000/RPC2`. See [rTorrent address](../downloads/downloaders.md#rtorrent-address) |
   | NZBGet | Username and password |

5. Press **Save**, then **Test** on its row.

   You should see **Working** next to its name.

## Link Prowlarr or Jackett

1. Open **Settings > Sources** and find the **Prowlarr or Jackett** card.
2. Press **Link** and pick **Prowlarr** or **Jackett**.
3. Type the **Address**. Usual ports: `9696` for Prowlarr, `9117` for Jackett. For Jackett, include its base path if you use one.
4. Paste the **API key**. Prowlarr: **Settings > General**. Jackett: its main page.
5. Press **Test**, then **Link**.

   You should see your sites listed as sources. From Prowlarr they arrive within 1 minute.

:::note
Linking Prowlarr needs a HolyArrs password on your account. The card links to your account page if you have none.
:::

## Choices

### Category

HolyArrs puts its downloads in one category and only watches that one. qBittorrent makes it if missing. SABnzbd needs it made first, under Config > Categories.

### Downloader in another container

If the downloader sees the download folder under another name, set **Downloader's path** and **HolyArrs' path**, such as `/data/torrents` and `/downloads`. With the shared layout from [Folders](../folders/) you need neither.

### Prowlarr or Jackett

Prowlarr pushes its sites to HolyArrs and keeps them current. HolyArrs pulls from Jackett every 6 hours and when you press **Sync**. Both work; use the one you already run.

## If it does not work

| You see | Cause | Fix |
|---|---|---|
| **Not answering** | Wrong address, or the containers cannot reach each other | Use the IP and port you open in your browser, not `localhost` |
| **Sign-in refused** | Wrong key or password | Copy it again from the app, save, press **Test** |
| SABnzbd test fails on category | Category does not exist | Make it in SABnzbd under Config > Categories |
| Prowlarr links but no sources arrive | Prowlarr cannot reach HolyArrs | Press **Change** on the card and give an address Prowlarr can open |

## For experts

<details class="ha-more"><summary>Downloader settings</summary>

| Setting | What it does | Default |
|---|---|---|
| **Category** | Label HolyArrs uses and watches | set per app |
| **Priority** | 1 to 50, lowest used first; equal numbers take turns | |
| **Tags** | Only titles with a matching tag use it; untagged titles use untagged downloaders | none |
| **Reach the local network** | Off for a downloader on the internet | on |
| **HolyArrs can change this Downloader** | Off: new downloads still go in, nothing existing is paused or removed | on |
| **Try the next Downloader** | Sends to the next one of the same kind if one refuses | on |
| Send spacing | Gap between sends to one downloader | 2 s |

Stored keys are encrypted and never shown again. Moving a downloader to a new host needs the key typed again. Full page: [Downloaders](../../downloads/downloaders/).

</details>

<details class="ha-more"><summary>Sources by hand</summary>

HolyArrs reads Torznab (torrents), Newznab (Usenet) and RSS (newest only, no search) feeds itself. **Settings > Sources > Add a Source**, pick **Kind of feed**, paste **Address** and **Key**, tick **Reach the local network** for a feed on your home network, **Save**. It tests right away. Full page: [Sources](../../downloads/sources/).

</details>

## Related

- [Folders](../folders/)
- [First title](../first-title/)
- [Downloaders](../../downloads/downloaders/)
- [Sources](../../downloads/sources/)
