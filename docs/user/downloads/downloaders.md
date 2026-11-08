# Downloaders

You want HolyArrs to send what it finds to your download app and bring the finished files into your Library. HolyArrs does not download anything itself. It hands each Release to a Downloader, follows the download, and imports the files when they are ready.

HolyArrs works with:

- **qBittorrent**, **Transmission** and **Deluge** for torrents.
- **SABnzbd** and **NZBGet** for Usenet.

Only the owner and admins can see or change Downloaders.

## Add a Downloader

1. Open **Settings**, then **Downloaders**.
2. Press **Add a Downloader**.
3. Under **Which app**, pick your app.
4. Fill in **Name**. Any name works.
5. Fill in **Address**. This is the address of the app's web page, such as `http://192.168.1.10:8090`.
6. Fill in the sign-in for your app. See [Sign-in for each app](#sign-in-for-each-app).
7. Check the other settings. See [The Downloader settings](#the-downloader-settings).
8. Press **Save**.

To check it later, press **Test** on its row. The result shows next to the name.

## Sign-in for each app

- **qBittorrent**: under **Sign in with**, pick **API key** or **Username and password**. The API key needs qBittorrent 5.2 or newer. You find it in qBittorrent under Options, Web UI.
- **Transmission**: a username and password. Leave them empty when Transmission does not ask for them from your network. Transmission 4 or newer is needed.
- **Deluge**: the Deluge Web UI password. There is no username.
- **SABnzbd**: the API key. You find it in SABnzbd under Config, General.
- **NZBGet**: a username and password. Leave them empty only when NZBGet lets HolyArrs in without them.

Keys and passwords are stored encrypted and never shown again. To keep the stored one, leave the field alone. When you move a Downloader to another host, type the key or password again. HolyArrs never sends a stored key to a new host by itself.

## The Downloader settings

For qBittorrent these sit under **More options**.

- **Category**: HolyArrs puts its downloads in this category and only looks at that category.
  - qBittorrent makes it if it is missing.
  - SABnzbd needs it to exist already, under Config, Categories.
  - Transmission and Deluge store it as a label.
  - NZBGet gets it as the download category.
- **Priority**: 1 to 50. The lowest number is used first.
- **Tags**: only titles with one of these tags use this Downloader.
- **Downloader's path** and **HolyArrs' path**: fill these in only when the Downloader sees the download folder under a different name than HolyArrs does. This is common when they run in separate containers. For example, `/data/downloads` for the Downloader and `/downloads` for HolyArrs.
- **Reach the local network**: on for most setups, since Downloaders usually run at home. Turn it off for a Downloader on the internet.
- **HolyArrs can change this Downloader**: when off, new downloads still go to it, but HolyArrs never pauses, changes or removes anything already in it. See [Keep a Downloader read only](#keep-a-downloader-read-only).

## What the states mean

Each Downloader row shows one state.

- **Working**: it answered the last check.
- **Paused**: you paused it. HolyArrs sends nothing new, but still follows downloads already there.
- **Resting**: it did not answer, so HolyArrs waits before sending to it again. The row shows until when.
- **Not answering**: the last check got no answer.
- **Sign-in refused**: it turned down the key or the sign-in. Check them and test again.

Use **Pause** and **Resume** on the row to stop and start sending.

## How HolyArrs picks a Downloader

- Torrent Releases go to torrent Downloaders. Usenet Releases go to Usenet Downloaders.
- The lowest priority number goes first. Downloaders with the same number take turns.
- Paused and resting Downloaders are skipped.
- If one cannot take a Release, the next one of the same kind is tried. This is the **Try the next Downloader** setting, on by default.
- Sends to the same Downloader are spaced out, 2 seconds by default.
- With tags, a title goes to the Downloaders that share one of its tags. An untagged title goes to untagged Downloaders.

## Follow a download

While a title downloads, its page shows how it is going: waiting its turn, downloading with progress, speed and time left, stalled, or failed with the reason. It updates by itself.

Press **Mark failed** to end a download you do not want, for example a wrong copy or one that is too slow. HolyArrs then:

1. turns that Release down for the title so it is not picked again. It shows on the **Turned down** list.
2. removes it from the Downloader, with its files.
3. searches again and sends the next best Release.

If nothing else passes your rules, the title shows **Needs a decision**.

## When downloads go wrong

HolyArrs handles these the same way as **Mark failed**:

- **The Downloader gives up**, for example when parts are missing or a repair failed. The Downloader's reason is shown.
- **Programs in the download.** A movie never needs files like `.exe`, `.bat`, `.scr` or `.lnk`, even when named `movie.mkv.exe`.
- **Fakes in the download.** A password note, a torrent with only archives and no video or audio, or a web shortcut with no video or audio beside it. For torrents, HolyArrs reads the file list as soon as it is there, so a fake fails before it downloads.
- **Files that do not match.** When the files are not what the Release name said, the import is refused and the download is removed.
- **Stalled.** With no progress for 6 hours, a download is stalled. After 24 hours with no progress, HolyArrs gives up on it. Waiting its turn, paused in the Downloader, and repairing or unpacking at 100% do not count.
- **Removed outside HolyArrs.** When a download disappears from the Downloader, HolyArrs replaces it with the next Release.

## Settings for failed downloads

These sit lower on the same Settings page.

| Setting | Default | What it does |
|---|---|---|
| Stalled after (hours) | 6 | When a download counts as stalled. |
| Give up and try the next Release after | 24 hours | When a stalled download is failed and replaced. |
| Remove failed downloads from the Downloader | on | Off keeps them in the Downloader so you can look at them. |
| Fail downloads that hold programs or fakes | on | Off lets them through to the import. |
| Replace downloads removed outside HolyArrs | on | Off makes the title wait for your decision. |
| Try the next Downloader | on | Tries another Downloader of the same kind. |
| Seconds between sends to one Downloader | 2 | 0 turns the spacing off. |

## Keep a Downloader read only

Turn off **HolyArrs can change this Downloader** on a Downloader. New downloads still go there, but nothing already in it is paused, changed or removed.

To lock this from the server, set `HOLYARRS_READONLY_DOWNLOADERS` to a comma list of Downloader ids or addresses:

```
HOLYARRS_READONLY_DOWNLOADERS=http://your-server:8080
```

A Downloader on that list stays read only whatever its switch says, even if you remove and add it again. The switch shows as locked, and the Downloads page hides the controls on those rows.

## Remove a Downloader

Press **Remove** on its row. HolyArrs refuses while the Downloader still has active downloads. Pause it, let them finish, then remove it. Downloads already in the app stay there.

## Troubleshooting

**SABnzbd test fails with a host name.** SABnzbd refuses calls that use a host name it does not know. Use its IP address in **Address**, or add HolyArrs' host name to `host_whitelist` in SABnzbd under Config, Special.

**SABnzbd says the category is missing.** Make the category in SABnzbd under Config, Categories, then test again.

**Deluge does not work.** Connect the Deluge Web UI to your daemon yourself first, and turn on the daemon's Label plugin. HolyArrs does not pick a daemon for you.

**A Release from a Source with a proxy is refused.** Transmission, Deluge and NZBGet cannot take a `.torrent` or `.nzb` file from a Source that uses a proxy yet. Magnet links work.

**NZBGet will not keep files on removal.** NZBGet deletes the files when a download or its history entry is removed, so HolyArrs refuses a removal that asks to keep them. Leave the download in NZBGet if you need the original files.

**A row says "Check whether this download arrived".** The connection dropped while Transmission, Deluge or NZBGet was taking a Release, so HolyArrs cannot tell if it arrived. It will not send that Release again on its own. You cannot change or remove the Downloader while this is open.

1. Open the Downloader app and look for the download.
2. In HolyArrs, press **Give up on it** on that row.

HolyArrs will not retry that Release for the title. The download in your Downloader stays where it is. You can then pick a different Release by hand under **Needs a decision**.

**Sign-in refused.** Check the key or password in your Downloader, type it again in HolyArrs, and press **Test**.

See also [History](history.md) to see every step a download went through.

For your own tools, every Downloader route is in the API description at `http://your-server:6262/api/v1/openapi.json`.
