---
title: Media servers
description: Connect Plex, Jellyfin or Emby so new imports, renames and deletes show up without waiting for a full scan.
level: Setup
---

Make new movies, shows and music appear in Plex, Jellyfin or Emby right after import.

HolyArrs tells the server to look at a folder again after it imports, renames or deletes files there. It only sends these updates; it never reads your library back.

## Add a media server

1. Open **Settings > Media servers**.
2. Click **Add a media server**.
3. Pick the **Kind**: Plex, Jellyfin or Emby.
4. Fill in a **Name**, unique, up to 80 characters.
5. Type the **Address** with `http` or `https`, such as `http://192.168.1.10:32400` for Plex or `http://192.168.1.10:8096` for Jellyfin and Emby.
6. Paste the **Plex token**, or the **API key** for Jellyfin and Emby (in the server: Dashboard, API keys).
7. Click **Save**, then **Test** on the new card. You should see the library names (Plex) or the server name and version (Jellyfin, Emby).

:::caution
If the server sees your files under another path, for example in another container, scans find nothing new. Set the path pair under [Paths](#paths).
:::

## Choices

### Update it after

Pick which changes the server hears about: **An import**, **A rename**, **A delete**. All 3 are on for a new server. A delete counts only when files left the disk.

### Paths

Fill **HolyArrs path** and **Same folder on the server** only when the two see files under different paths. If HolyArrs sees `/movies` and the server sees `/media/movies`, put `/movies` first and `/media/movies` second. Fill both or neither.

### Reach the local network

On by default, since media servers are almost always at home. Turn it off for a server on the internet. Link-local and cloud metadata addresses stay blocked either way.

### Send updates

Off pauses a server without removing it. The card shows **Off**.

The token or key is stored encrypted and never shown. Leave the box empty to keep it. After an address change, paste the token again, so it never goes to a different server.

## Check that it works

The card shows the last check or last error. Every update also lands in [History](../downloads/history.md) as **Told the media server about it** or **Could not reach the media server** with the reason. The [Status](../health/health-checks.md) page lists each server: off, failed, not told yet, or working.

## If it does not work

| Message or symptom | Fix |
|---|---|
| "the server turned down the token" | Click **Change** and paste a new token or key |
| "the server did not answer" | Check the address, the port, and that HolyArrs can reach that machine |
| "the server is on the local network, which it may not reach" | Turn on **Reach the local network** |
| "the server has no movie, TV or music library" | Add a library in Plex, then test again |
| "type the token again when the server address changes" | Paste the token again and save |
| "give both sides of the path map, or neither" | Fill both path boxes, or clear both |
| The server scans, but nothing new shows | Paths differ. Set the path pair |

## For experts

<details class="ha-more"><summary>How updates are sent</summary>

HolyArrs waits 10 s and gathers every changed folder before sending, so renaming a whole show is one update.

| Server | What HolyArrs sends |
|---|---|
| Plex | Finds the movie, TV or music library holding the folder and scans that folder only. If none holds it, scans every library of that type |
| Jellyfin | The list of created, changed or deleted folders, then one library refresh |
| Emby | The list of created, changed or deleted folders |

</details>

## Related

- [Library folders](folders.md)
- [History](../downloads/history.md)
