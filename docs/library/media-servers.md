---
title: Media servers
description: Connect Plex, Jellyfin or Emby so new imports, renames and deletes show up without waiting for a full scan.
level: Setup
---

Make new movies, shows and music appear in Plex, Jellyfin or Emby right after import.

HolyArrs tells the server to look at a folder again after it imports, renames or deletes files there.

## Before you start

- You can open the server's address from the machine that runs HolyArrs.
- You have a Plex token, or an API key for Jellyfin or Emby (make one in the server under Dashboard, API keys).
- You know where the server sees your media folders. In Docker, that is the path inside the server's container.

## Add a media server

1. Open **Settings > Media servers**.
2. Select **Add a media server**.
3. Enter a **Name**. Each server needs its own, up to 80 characters.
4. Pick the **Kind**: Plex, Jellyfin or Emby. You cannot change it later.
5. Enter the **Address** with `http` or `https`, such as `http://192.168.1.10:32400` for Plex or `http://192.168.1.10:8096` for Jellyfin and Emby.
6. Paste the **Plex token**, or the **API key** for Jellyfin and Emby.
7. Select **Save**, then **Test** on the new card. You should see the library names (Plex) or the server name and version (Jellyfin, Emby).

:::caution
If the server sees your files under another path, for example in another container, scans find nothing new. Set the path pair under [Paths](#paths).
:::

## Choices

### Update it after

Pick which changes the server hears about: **An import**, **A rename**, **A delete**. All 3 are on for a new server. A delete counts only when files left the disk.

### Paths

Fill **HolyArrs path** and **Same folder on the server** only when the two see files under different paths. If HolyArrs sees `/movies` and the server sees `/media/movies`, enter `/movies` first and `/media/movies` second. Fill both or neither.

### Reach the local network

On by default, since media servers are almost always at home. Turn it off for a server on the internet. HolyArrs never calls link-local or cloud metadata addresses, whatever this says.

### Send updates

Turn it off to pause a server without removing it. The card shows **Off**.

The token or key is stored encrypted and never shown. Leave the box empty to keep it. After an address change, paste it again, so it never goes to a different server.

## Check that it works

The card shows the last check or the last error. Each update also lands in [History](../downloads/history.md) as **Told the media server about it** or **Could not reach the media server**, with the reason. [Health checks](../run/health.md) lists each server and its state.

## If it does not work

| Message or symptom | Fix |
|---|---|
| "the server turned down the token" | Select **Change** and paste a new token or key |
| "the server did not answer" | Check the address and port, and that HolyArrs can reach that machine |
| "the server is on the local network, which it may not reach" | Turn on **Reach the local network** |
| "the server has no movie, TV or music library" | Add a library in Plex, then test again |
| "type the token again when the server address changes" | Paste the token again and save |
| "give both sides of the path map, or neither" | Fill both path boxes, or clear both |
| The server scans, but nothing new shows | The paths differ. Set the path pair |

## For experts

<details class="ha-more"><summary>How updates are sent</summary>

HolyArrs waits 10 s and gathers every changed folder before it sends, so renaming a whole show is one update.

| Server | What HolyArrs sends |
|---|---|
| Plex | Finds the movie, TV or music library that holds the folder and scans that folder only. If none holds it, scans every library of that type |
| Jellyfin | The list of created, changed or deleted folders, then one library refresh |
| Emby | The list of created, changed or deleted folders |

A request to a server times out after 20 s, and **Test** after 30 s.

</details>

## Related

- [Library folders](folders.md)
- [Folders and hardlinks](../start/folders.md)
- [Health checks](../run/health.md)
