# Connect Bazarr, Prowlarr, Unpackerr and Homepage

You want another app that normally talks to Sonarr or Radarr to work with HolyArrs. Every app here uses the same two keys as Seerr: the Sonarr-side key on `/sonarr` and the Radarr-side key on `/radarr`. Make them first, see [Turn it on](README.md#turn-it-on).

In every example, if HolyArrs runs under a URL base such as `/holyarrs`, put it in front: `/holyarrs/sonarr`.

## Bazarr

Bazarr finds subtitles for the files in your Library.

1. In Bazarr, open **Settings > Sonarr**.
2. Fill in your HolyArrs host and port, `/sonarr` as the URL base, and the Sonarr-side key.
3. Press **Test**, then save.
4. Do the same in **Settings > Radarr** with `/radarr` and the Radarr-side key.

What works:

- **Library sync.** Bazarr copies every show with its episodes, and every movie, with the file of the main Version: path, size, release name, languages, quality and video codec. Episodes and movies without a file are skipped, like with Sonarr.
- **Pictures.** Posters and wide pictures come from HolyArrs.
- **Rescans.** After a subtitle change Bazarr asks for a rescan. HolyArrs looks at that title's folder again.
- **Folders and tags.** Bazarr reads the Library folders for its health checks, and the tag list.

What does not work:

- **Live updates.** HolyArrs has no live feed for these apps, so Bazarr's live connection is refused. Bazarr still finds new files through its scheduled full sync and its scheduled subtitle search. If the retries fill Bazarr's log, start Bazarr with its option that turns the live feed off.
- **The folder browser** in Bazarr's path mapping editor. Type the paths by hand.
- **Grab history.** HolyArrs does not keep the indexer page link of a grab, so subtitle sites that need it cannot use it.
- **Webhooks into Bazarr.** HolyArrs does not send Sonarr or Radarr style webhooks. Use the scheduled sync.

HolyArrs also has its own subtitle sites and subtitle searching. That feature is held back in this release, so Bazarr is the way to get subtitles for now.

## Prowlarr

Prowlarr can fill in your HolyArrs Sources from its indexers. Each Prowlarr indexer becomes one Newznab or Torznab Source that points at Prowlarr.

1. In Prowlarr, open **Settings > Apps**.
2. Add a **Sonarr** app for shows. Add a **Radarr** app for movies. One is enough if you only want one kind.
3. Fill in the fields below.
4. Press **Test**, then **Save**. Prowlarr writes its indexers into HolyArrs.
5. Open **Sources** in HolyArrs. New Sources arrive paused. Look them over and resume the ones you want.

| Field | Sonarr app | Radarr app |
| --- | --- | --- |
| Prowlarr server | Prowlarr's address as HolyArrs reaches it, for example `http://prowlarr:9696` | same |
| Sonarr server / Radarr server | `http://your-server:6262/sonarr` | `http://your-server:6262/radarr` |
| API key | The Sonarr-side key | The Radarr-side key |

The person who made the key must be allowed to change settings as well as manage the Library. Otherwise Prowlarr gets 403 and nothing is written.

How it behaves:

- **Paused again.** When Prowlarr moves a Source to another address, or turns local network access back on after you turned it off, the Source is paused again. Other changes leave it as it was.
- **Local network.** Sources Prowlarr writes may reach the local network, because Prowlarr usually sits there. Every other Source still needs its own "May reach the local network" tick.
- **What HolyArrs uses.** The address and feed path, the key (stored encrypted and only shown masked), the categories, the least seeders, the priority, and the switches for the feed, automatic searches and hand searches.
- **What is only stored.** Seed ratio, seed time, tags and other values Prowlarr sends are kept as sent, so its next sync sees no change. HolyArrs does not use them. Seeding follows your downloader's seed goal.
- **Your edits.** If you change a value Prowlarr manages, such as the name, address, categories or priority, Prowlarr puts its own back at its next full sync.
- **Removing.** Removing an indexer in Prowlarr removes its Source. Prowlarr only sees and changes the Sources it wrote, never the ones you added by hand.
- **Both sides.** The same indexer sent to both sides makes two Sources. If the name is taken, the second gets "(shows)" or "(movies)" added.
- **Testing.** Each Test and save makes HolyArrs ask Prowlarr's feed for its capabilities. If that fails, Prowlarr can save with its force option and the Source is made anyway, paused.

## Unpackerr

You do not need Unpackerr. HolyArrs unpacks RAR sets itself during import. It imports only the files the download listed, so files Unpackerr unpacks next to them are never imported.

If you run it anyway, point its Sonarr and Radarr entries at `/sonarr` and `/radarr` with the keys. It reads the download list. A finished download waiting for import shows as completed, with its folder as the place to unpack. After HolyArrs imports it, it leaves the list. Unpackerr changes nothing in HolyArrs.

## Homepage

The Sonarr widget of the Homepage start page works. Add it with `http://your-server:6262/sonarr` and the Sonarr-side key. It shows the show count, the download list, aired episodes with no file, and the calendar. Lists come 10 to a page, newest first.

The Radarr widget does not work yet. Its missing list and calendar answer 404.
