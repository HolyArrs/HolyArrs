---
title: Downloaders
description: Connect qBittorrent, Transmission, Deluge, SABnzbd or NZBGet so HolyArrs can send Releases and import the finished files.
level: Setup
sidebar: { order: 2 }
---

Connect your download app so HolyArrs can send it Releases and import what finishes.

HolyArrs does not download by itself. It hands each Release to a Downloader, follows it, and imports the files.

| Kind | Apps |
|---|---|
| Torrent | qBittorrent, Transmission 4 or newer, Deluge |
| Usenet | SABnzbd, NZBGet |

## Before you start

- You are the owner or an admin.
- Your download app is running and you can open its web page.

## Add a Downloader

1. Open **Settings > Downloaders**.
2. Press **Add a Downloader**.
3. Under **Which app**, pick your app.
4. Type a **Name**.
5. Type the **Address**, the app's web page, such as `http://192.168.1.10:8090`.
6. Fill in the sign-in. See [Sign-in for each app](#sign-in-for-each-app).
7. Press **Save**, then **Test** on the row.

   You should see **Working** next to the name.

:::caution
If HolyArrs and the Downloader run in separate containers and see the download folder under different paths, imports fail. Fill in the path pair under [Paths](#paths) before the first download.
:::

## Sign-in for each app

| App | What to enter | Where to find it |
|---|---|---|
| qBittorrent | **API key** (qBittorrent 5.2 or newer) or **Username and password** | Options > Web UI |
| Transmission | Username and password, empty if it asks for none | Transmission settings |
| Deluge | Web UI password, no username | Deluge Web UI |
| SABnzbd | API key | Config > General |
| NZBGet | Username and password, empty if NZBGet lets HolyArrs in without them | NZBGet settings |

HolyArrs stores keys and passwords encrypted and never shows them again. Leave the field alone to keep the stored one. When you change the address to another host, type the key again. HolyArrs never sends a stored key to a new host.

## Choices

For qBittorrent these sit under **More options**.

### Category

HolyArrs files its downloads under this category and only follows that category.

| App | How the category works |
|---|---|
| qBittorrent | Made for you if missing |
| SABnzbd | Must exist first, under Config > Categories |
| Transmission, Deluge | Stored as a label |
| NZBGet | Sent as the download category |

### Priority and tags

**Priority** runs 1 to 50. The lowest number is used first, and equal numbers take turns. With **Tags**, only titles that share a tag use this Downloader. Untagged titles use untagged Downloaders.

### Paths

Fill **Downloader's path** and **HolyArrs' path** under **Different folder names** only when the two apps see the download folder under different names. Example: the Downloader saves to `/data/torrents` and HolyArrs sees it as `/downloads`. **Add a path pair** adds another row.

The better fix is one shared parent folder mounted at the same path in both containers. See [Folders](../../start/folders/).

### Reach the local network

On by default, since Downloaders usually run at home. Turn it off for a Downloader on the internet.

### HolyArrs can change this Downloader

Off means new downloads still go there, but HolyArrs never pauses, changes or removes anything already in it. Use it for a Downloader you share with other apps.

## Follow a download

A title's page shows each download: waiting its turn, downloading with speed and time left, stalled, or failed with the reason.

Press **Mark failed** to end a download you do not want. HolyArrs then:

1. Adds that Release to the title's **Turned down** list so it is not picked again.
2. Removes it from the Downloader, with its files.
3. Searches again and sends the next best Release.

If nothing else passes your rules, the title shows **Needs a decision**.

## Row states

| State | Meaning |
|---|---|
| **Working** | Answered the last check |
| **Paused** | You paused it. Nothing new is sent; current downloads are still followed |
| **Resting** | It did not answer, so HolyArrs waits until the time shown |
| **Not answering** | The last check got no answer |
| **Sign-in refused** | The key or sign-in was turned down |

**Pause** and **Resume** on the row stop and start sending. A Downloader that stops answering is skipped for up to 30 minutes.

## If it does not work

**Symptom:** The SABnzbd test fails when **Address** uses a host name.
**Cause:** SABnzbd refuses host names it does not know.
**Fix:** Use its IP address, or add HolyArrs' host name to `host_whitelist` in SABnzbd under Config > Special.

**Symptom:** SABnzbd says the category is missing.
**Fix:** Make the category in SABnzbd under Config > Categories, then press **Test**.

**Symptom:** Deluge does not work.
**Fix:** In the Deluge Web UI, connect to your daemon yourself and turn on the Label plugin. HolyArrs does not pick a daemon.

**Symptom:** A `.torrent` or `.nzb` from a Source with a proxy is refused.
**Cause:** Transmission, Deluge and NZBGet cannot take those files through a proxied Source yet. Magnet links work.

**Symptom:** A row says **Check whether this download arrived**.
**Cause:** The connection dropped while Transmission, Deluge or NZBGet was taking a Release. HolyArrs does not send it again by itself.
**Fix:**
1. Look for the download in the Downloader app.
2. In HolyArrs, press **Give up on it**. The download stays in the Downloader. Pick another Release under **Needs a decision** if you want one.

**Symptom:** **Sign-in refused**.
**Fix:** Copy the key or password from the Downloader, type it again, press **Test**.

More in [Troubleshooting](../../help/troubleshooting/).

## For experts

<details class="ha-more"><summary>How HolyArrs picks a Downloader</summary>

- Torrent Releases go to torrent Downloaders, Usenet Releases to Usenet.
- Lowest priority number first. Equal numbers take turns.
- Paused and resting Downloaders are skipped.
- If one refuses a Release, the next of the same kind is tried (**Try the next Downloader**).
- Sends to one Downloader are spaced 2 s apart by default.
- Tags must match. Untagged titles go to untagged Downloaders.

</details>

<details class="ha-more"><summary>When downloads go wrong: what counts, and the settings</summary>

HolyArrs treats these like **Mark failed**:

- The Downloader gives up (missing parts, failed repair). Its reason is shown.
- The download holds programs such as `.exe`, `.bat`, `.scr` or `.lnk`, including `movie.mkv.exe`.
- The download looks fake: a password note, only archives with no video or audio, or a web shortcut with no media beside it. Torrent file lists are read as soon as they arrive, so a fake fails before it downloads.
- The files do not match the Release name. The import is refused and the download removed.
- No progress for the give-up time. Waiting its turn, paused in the Downloader, and repairing or unpacking at 100% do not count.
- The download was removed outside HolyArrs. It is replaced with the next Release.

These settings sit under **When downloads go wrong** on **Settings > Downloaders**. Press **Save** in the bar that appears after a change.

| Setting | Default | Change it when |
|---|---|---|
| Remove failed downloads from the Downloader | on | You want to inspect failures (off). On also deletes their files |
| Add matched titles to your Library | off | You want a sure match already in a Downloader added without searching |
| When you remove a download: **Delete its files** | on | You want the files kept |
| When you remove a download: **Turn down its Release** | off | You never want that Release picked again for the title |
| When you remove a download: **Search again** | on | Off if you pick the next Release yourself. Needs **Turn down its Release** |
| Stalled after (hours) | 6 | Your downloads are slow by nature. Range 1 to 168 |
| Try the next Release after (hours) | 24 | You prefer waiting over replacing. Counted from the last progress. Range: the stalled time up to 720 |
| Fail downloads that hold programs or fakes | on | Rarely. Off lets them reach the import |
| Replace downloads removed outside HolyArrs | on | You remove downloads on purpose. Off makes the title wait for you |
| Try the next Downloader | on | You want one Downloader only |
| Seconds between sends to one Downloader | 2 | The Downloader chokes on bursts. 0 turns spacing off. Range 0 to 60 |

</details>

<details class="ha-more"><summary>Lock a Downloader read only from the server</summary>

Set `HOLYARRS_READONLY_DOWNLOADERS` to a comma list of Downloader ids or addresses:

```
HOLYARRS_READONLY_DOWNLOADERS=http://your-server:8080
```

A listed Downloader stays read only whatever its switch says, even after remove and re-add. The switch shows as locked, and the Downloads page hides controls on its rows.

</details>

<details class="ha-more"><summary>Removing, NZBGet removals, API</summary>

- **Remove** is refused while the Downloader has active downloads. Pause it, let them finish, then remove.
- NZBGet deletes files whenever a download or its history entry is removed, so HolyArrs refuses removals that ask to keep files.
- Every Downloader route is in `http://your-server:6262/api/v1/openapi.json`.

</details>

## Related

- [Sources](../sources/)
- [History](../history/)
- [Folders](../../start/folders/)
