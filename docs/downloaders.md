# Downloaders

HolyArrs does not download by itself. When it chooses a Release, it hands it to a **Downloader**: qBittorrent for torrents, SABnzbd for Usenet. It then watches the download until the files are ready and puts them in your Library. This page covers adding Downloaders, how HolyArrs picks one, and what happens when a download goes wrong. Only the owner and admins see any of this.

## Adding a Downloader

Open **Settings**, then **Downloaders**, and press **Add a Downloader**.

1. Pick the app: **qBittorrent** or **SABnzbd**.
2. **Name**: any name you like.
3. **Address**: the address of its web page, such as `http://192.168.1.10:8090`.
4. The sign-in:
   - **qBittorrent**: its sign-in name and password. Leave both empty when qBittorrent skips sign-in for your network.
   - **SABnzbd**: its API key, found in SABnzbd under Config, General.
5. **Category**: HolyArrs puts its downloads in this category and only ever looks at that category. qBittorrent makes it if it is missing. In SABnzbd the category must already exist (Config, Categories); `movies` exists out of the box.
6. **Priority**: 1 to 50. 1 is used first.
7. **Different folder names**: only when the Downloader sees the download folder under another name than HolyArrs does, for example when they run in different containers. Give both names, such as `/data/downloads` (the Downloader's) and `/downloads` (HolyArrs').
8. **May reach the local network**: on by default, because Downloaders usually run at home. Turn it off for a Downloader on the internet.

Press **Save Downloader**. HolyArrs tests it straight away and says the result under it.

Keys and passwords are kept encrypted and always shown masked. To keep the stored one, leave the field as it is. When you move a Downloader to another host, type the key or password again: HolyArrs never sends a stored key to a new host by itself.

### SABnzbd and host names

SABnzbd refuses calls that use a host name it does not know. Use its IP address in the **Address**, or add the name HolyArrs uses to SABnzbd's list of allowed host names (Config, Special, `host_whitelist`). The test says so when this is the problem.

## What the states mean

Each Downloader shows one state:

- **Working**: it answered the last check.
- **Paused**: you paused it. HolyArrs sends nothing new to it, but still follows downloads already there.
- **Resting**: it did not answer, so HolyArrs waits before sending to it again: one minute after the first failure, then twice as long each time, up to 30 minutes.
- **Not answering**: the last check got no answer.
- **Sign-in refused**: it turned down the key or the sign-in.

HolyArrs checks every Downloader once a minute. A working answer ends a rest. **Test** checks one now.

## How HolyArrs picks a Downloader

- A torrent Release goes to a torrent Downloader, a Usenet Release to a Usenet one.
- Lower priority numbers go first. Downloaders with the same priority take turns.
- Paused and resting Downloaders are skipped.
- When one cannot take a Release, the next one of the same kind is tried (**Try the next Downloader**, on by default).
- Two sends to the same Downloader are at least 2 seconds apart.

## On the Movie page

While a movie downloads, its page shows the download in plain words: waiting its turn, downloading with how far it is, the speed and the time left, stalled, or failed with the reason. It updates by itself.

**Mark failed** ends a download you do not want, for example a copy that is wrong or too slow. HolyArrs then:

1. turns that Release down for the movie, so it is never picked again (it shows on the **Turned down** list with "marked failed by" and your name);
2. removes it from the Downloader, with its files;
3. searches again and sends the next best Release.

When nothing else passes your Pick rules, the movie shows **Needs a decision** instead.

## When downloads go wrong

HolyArrs treats these the same way as Mark failed:

- **The Downloader gives up**, for example because parts are missing or a repair failed. The reason the Downloader gives is shown.
- **Programs inside the download.** A movie never needs files such as `.exe`, `.bat`, `.scr` or `.lnk`, also when hidden as `movie.mkv.exe`. Such a download fails.
- **Fakes inside the download.** A password note (`password.txt`), a torrent holding archives and no video or audio, or a web shortcut (`.url`) with no video or audio beside it. Such a download fails too. A web shortcut beside the video or tracks is common and only noted in the Events. For torrents HolyArrs reads the file list as soon as the Downloader has it, so a fake fails before it downloads.
- **Lying files.** When the files are not what the Release name said, the import is refused and the download is removed from the Downloader.
- **Stalled.** A download with no progress for 6 hours is **Stalled**. When it moves again, it is back to downloading. After 24 hours with no progress HolyArrs gives up on it. Waiting its turn, paused in the Downloader, and repairing or unpacking at 100% never count.
- **Removed outside HolyArrs.** When a download disappears from the Downloader, HolyArrs replaces it with the next Release.

The stall timers count from the last time the download moved forward, and survive a restart of HolyArrs.

## The settings

Under **When downloads go wrong** on the same page:

| Setting | Default | What it does |
|---|---|---|
| Stalled after (hours without progress) | 6 | When a download counts as stalled. |
| Give up and try the next Release after (hours) | 24 | When a stalled download is failed and replaced. At least the stalled time. |
| Remove failed downloads from the Downloader | on | Off keeps them in the Downloader for you to look at. |
| Fail downloads that hold programs or fakes | on | Off lets them through to the import. |
| Replace downloads removed outside HolyArrs | on | Off: the movie waits for your decision instead. |
| Try the next Downloader | on | Also on Settings, Search. |
| Seconds between sends to one Downloader | 2 | 0 turns the spacing off. |

## Removing a Downloader

**Remove** is refused while the Downloader still has active downloads: pause it, let them finish, then remove it. Downloads already in the app stay there.

## For developers

Every route is in the OpenAPI file (`/api/v1/openapi.json`, tag Downloaders): listing, adding, changing, testing, pausing and removing Downloaders, `POST /api/v1/downloads/{id}/mark-failed`, and `GET`/`PUT /api/v1/settings/downloaders`.
