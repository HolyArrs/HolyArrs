---
title: Soulseek with slskd
description: "Find and download music from Soulseek through your own slskd: one slskd Source to search and one slskd Downloader to fetch."
level: Setup
sidebar: { order: 5 }
---

HolyArrs can find and download music on the Soulseek network through [slskd](https://github.com/slskd/slskd). slskd searches and downloads by itself, so you add it twice: once as a Source and once as a Downloader. Both use the same slskd.

slskd is for music only. Movies and shows still need a torrent or Usenet Downloader.

## Before you start

- slskd runs and is signed in to Soulseek. Its web page opens, for example at `http://192.168.1.10:5030`.
- slskd has an API key with the `readwrite` role. A `readonly` key can search nothing and download nothing. Add one in `slskd.yml` and restart slskd:

  ```yaml
  web:
    authentication:
      api_keys:
        holyarrs:
          key: <a long random key>
          role: readwrite
          cidr: 192.168.1.0/24
  ```

- HolyArrs can read slskd's download folder. With Docker, mount the same folder into both apps.

## Add the Source

1. Open **Settings > Sources** and press **Add a Source**.
2. Set **Kind of feed** to **Built-in site** and pick **slskd**.
3. Type slskd's **Address**, such as `http://192.168.1.10:5030`.
4. Paste the key into **API key**.
5. Tick **Reach the local network**.
6. Press **Save**.

   You should see **Working** under it. The test only asks slskd whether it is signed in; it never searches.

## Add the Downloader

1. Open **Settings > Downloaders** and press **Add a Downloader**.
2. Under **Which app**, pick **slskd** under **Soulseek**.
3. Type the same **Address** and paste the same key into **API key**.
4. Leave **Category** as `holyarrs`.
5. Add a path pair from slskd's download folder to the same folder as HolyArrs sees it. For example, slskd's `/app/downloads` is HolyArrs's `/slskd/downloads`. Recent slskd versions show where they save only to administrator keys, so with a `readwrite` key HolyArrs takes the folder from this pair.
6. Press **Save**.

   You should see **Working** and slskd's version.

## How it works

A search asks slskd for the artist and album and waits up to 45 seconds for answers. Each user's folder of audio files becomes one Release. Covers, logs and locked files are left out. The name comes from the folder, with the artist in front when the folder holds only the album, and the format at the end, such as `Artist - Album (2024) [FLAC 24bit]`. HolyArrs then judges it like any other Release.

The Downloader asks slskd to download the files of the chosen folder from that user. HolyArrs follows the files in slskd's download list and imports the folder once every file is done. A file slskd will try again counts as waiting. When every file stopped and one of them failed, the download failed and HolyArrs picks another Release.

A user with a free upload slot counts as one seeder, and the user's queue counts as peers, so busy users rank lower.

## If it does not work

| You see | Try |
|---|---|
| slskd's API key may only read | Give the key `role: readwrite` in `slskd.yml` and restart slskd |
| slskd is not signed in to the Soulseek network | Check slskd's own page. Its Soulseek user name and password live in `slskd.yml` |
| The Source says the address is on the local network | Tick **Reach the local network** on the Source |
| A finished download is never imported | HolyArrs cannot see the folder. Mount slskd's download folder into HolyArrs and add a path pair |
| slskd does not show this key where it saves downloads | Add the path pair from step 5 of [Add the Downloader](#add-the-downloader) |
| slskd saves into another folder pattern | HolyArrs expects slskd's default, each folder under its own name in the download folder. Set `transfers.download.destination.subdirectory` back to `${SOURCE_DIRECTORY}` |

<details class="ha-more">
<summary>For experts</summary>

The Source starts with a 5 second gap between searches and 60 searches an hour. Soulseek users see every search, so keep them low.

The Release link is `slskd:` followed by the user and the file list, so it never points anywhere else. Only a slskd Downloader takes it, because Soulseek is a protocol of its own next to torrent and Usenet.

A slskd download's id comes from the user and the folder. A send whose answer was lost is looked for by that id, so it is never sent twice.

Removing a download cancels and clears its files in slskd's list. slskd keeps the files it already saved.

</details>

## Related

- [Built-in sites](built-in-sites.md)
- [Downloaders](downloaders.md)
- [Sources](sources.md)
