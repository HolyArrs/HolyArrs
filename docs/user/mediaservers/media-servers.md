# Media servers

You want new movies, shows and music to show up in Plex, Jellyfin or Emby right away, without waiting for the server's own scan. Add your server once and HolyArrs tells it to look at a folder again after it imports, renames or deletes files there.

HolyArrs only sends these updates. It does not read your library back from the server.

## Add a media server

1. Open **Settings**, then **Media servers** (the page is at `http://your-server:6262/settings/media-servers`).
2. Click **Add a media server**.
3. Pick the **Kind**: Plex, Jellyfin or Emby.
4. Give it a **Name**. Each server needs its own name, up to 80 characters.
5. Type the **Address**, for example `http://192.168.1.10:32400` for Plex or `http://192.168.1.10:8096` for Jellyfin and Emby. Use an `http` or `https` link.
6. Paste the **Plex token**, or the **API key** for Jellyfin and Emby. You make an API key in the server under Dashboard, API keys.
7. Click **Save**, then **Test** on the new card.

## What happens after a change

HolyArrs waits 10 seconds and gathers every changed folder before it sends anything. Renaming a whole show is one update, not dozens.

- **Plex**: HolyArrs finds the movie, TV or music library that holds the folder and scans only that folder. If no library holds it, it scans every library of that type.
- **Jellyfin**: HolyArrs sends the list of folders that were created, changed or deleted, then asks for one library refresh.
- **Emby**: HolyArrs sends the list of folders that were created, changed or deleted.

## Settings

- **Update it after**: pick which changes the server hears about. **An import**, **A rename** and **A delete**. A delete only counts when files were removed from disk. All three are on for a new server.
- **HolyArrs path** and **Same folder on the server**: fill these only when the server sees your files under another path, for example when it runs in another container. If HolyArrs sees `/movies` and the server sees `/media/movies`, put `/movies` in the first box and `/media/movies` in the second. Fill both or neither.
- **Reach the local network**: on by default, since media servers are almost always at home. Turn it off for a server out on the internet. Even when it is on, HolyArrs never calls link-local or cloud metadata addresses.
- **Send updates**: turn this off to pause a server without removing it. Its card then shows **Off**.

Your token or key is kept encrypted and never shown again. Leave the box empty when you change other settings to keep the stored one. If you change the address, you must type the token again, so a token never goes to a different server.

## Check that it works

**Test** checks the address and the token:

- Plex must list at least one movie, TV or music library. The test shows the library names.
- Jellyfin and Emby must answer with their name and version.

The card shows when the server was last checked, or the last error.

Every update also lands in History: **Told the media server about it**, or **Could not reach the media server** with the reason. The [Status](../health/health-checks.md) page lists each media server too: off, failed, not told about a change yet, or working.

## Troubleshooting

- **"the server turned down the token"**: the token or API key is wrong or was revoked. Click **Change** and paste a new one.
- **"the server did not answer"**: check the address and port, and that HolyArrs can reach that machine.
- **"the server is on the local network, which it may not reach"**: turn on **Reach the local network** for that server.
- **"the server has no movie, TV or music library"**: add a library in Plex first, then test again.
- **"type the token again when the server address changes"**: you changed the address. Paste the token again and save.
- **"give both sides of the path map, or neither"**: fill both path boxes, or clear both.
- **The server scans, but nothing new shows up**: the paths probably differ. Set **HolyArrs path** and **Same folder on the server**.
