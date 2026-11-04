# Connect Seerr or Overseerr

Seerr and Overseerr send approved requests to Sonarr and Radarr. HolyArrs can answer them in their place: point Seerr's Sonarr server at HolyArrs's `/sonarr` address and its Radarr server at `/radarr`, and requests land in your HolyArrs Library and show up as available once the files are in.

The same addresses also serve Bazarr (subtitles), Unpackerr, Prowlarr (it fills in your Sources) and the Sonarr widget of the Homepage start page. See [Other apps](#other-apps) below.

This is a small, separate API made only for those apps. HolyArrs's own API stays [the API for other apps](api.md).

## Turn it on

It is off until you switch it on. While it is off, `/sonarr/` and `/radarr/` answer 404.

Open Settings > API for other apps > Requests from Seerr and Overseerr. Switch on "Answer Seerr and Overseerr", then make a key for each side: "Shows" for the Sonarr server and "Movies" for the Radarr server. Each key is shown once; copy it then. Making a key asks for your password again.

The same can be done with these calls, signed in with your password (the key calls need a recent password sign-in, like API keys):

| What | Call |
| --- | --- |
| See the switch, the addresses and the keys | `GET /api/v1/settings/old-app-api` |
| Turn it on | `PUT /api/v1/settings/old-app-api` with `{"enabled": true}` |
| Make the Sonarr-side key | `POST /api/v1/settings/old-app-api/keys/sonarr` |
| Make the Radarr-side key | `POST /api/v1/settings/old-app-api/keys/radarr` |
| Remove a key | `DELETE /api/v1/settings/old-app-api/keys/sonarr` (or `radarr`) |
| Let these apps delete files too | `PUT /api/v1/settings/old-app-api` with `{"delete_files": true}` |

Each key is shown once, when it is made. Making a new key for a side stops the old one at once. The settings answer shows the last four characters of each key, who made it and when it was last used.

## About the keys

- Each side has its own key, made only here. HolyArrs API keys (the ones starting `hak_`) do not work on `/sonarr` or `/radarr`, and these keys work nowhere else.
- A key acts as the person who made it. You need the "manage the Library" permission to make one. If that person's account is turned off or loses the permission, the apps get refused (401 or 403) until someone makes a new key.
- Seerr and Overseerr send the key in the address (`?apikey=`). HolyArrs accepts that only here, and masks the key in its own log. A reverse proxy in front of HolyArrs may still write the full address to its own log; check its settings if that matters to you.
- Seerr keeps the key in plain text in its settings file. That is why deleting files is off by default (see below).

## Add HolyArrs in Seerr or Overseerr

In Seerr: Settings > Services > Add Sonarr Server (and Add Radarr Server).

| Field | Sonarr side | Radarr side |
| --- | --- | --- |
| Hostname or IP | HolyArrs's address, for example `10.10.25.25`, or `holyarrs` on the same Docker network | same |
| Port | HolyArrs's port, for example `6262` | same |
| Use SSL | only if you reach HolyArrs over HTTPS | same |
| API key | the Sonarr-side key | the Radarr-side key |
| URL base | `/sonarr` | `/radarr` |

If HolyArrs runs under a [URL base](url-base.md) such as `/holyarrs`, put it in front: `/holyarrs/sonarr`.

Press Test. Seerr then lists HolyArrs's Pick rules as quality profiles and its show or movie Library folders as root folders. Pick the ones new requests should use and save.

Use one HolyArrs side per Seerr server. Do not add the same `/radarr` address as Seerr's 4K server as well: HolyArrs keeps all copies of a movie on one title, so removing the "4K" one from Seerr would remove the whole movie.

## What works

- **Requests.** An approved show request adds the show with only the asked-for seasons kept up to date. Asking for another season later switches that season on. Anime shows (Seerr's "anime" series type) are added with absolute episode numbers. An approved movie request adds the movie with the chosen Release gate (Seerr's minimum availability).
- **Searching.** When the request asks for a search, HolyArrs searches after it has looked at the show or movie folder, so files you already have are not grabbed again. Seerr's and Overseerr's "search now" start HolyArrs hand searches.
- **Progress.** Seerr's download progress bars read HolyArrs's Downloads: what is downloading and what finished but is not imported yet, one line per episode for shows.
- **Available.** Seerr's daily scan reads the Library. A season counts as available once every episode in it has a file, so a season with episodes still to air stays "partly available", the same as with Sonarr. A movie is available once it has a file.
- **Removing.** Seerr's "remove from Sonarr/Radarr" takes the title out of the HolyArrs Library. With the delete-files switch off (the default) the files stay on disk; with it on they go to the holding folder first, like every delete in HolyArrs. If Seerr asks for an exclusion, the title goes on the Never-add list.
- **Tags.** Seerr's per-user tags are made and kept, so Seerr finds them again. They are not attached to titles yet.
- **Open in.** Seerr's "open in Sonarr/Radarr" link for admins opens the title in HolyArrs: a show opens its Library page, a movie its title page.

## Other apps

Every app below uses the same two keys as Seerr: the Sonarr-side key on `/sonarr` and the Radarr-side key on `/radarr`. They only read, except Bazarr's rescan and Prowlarr, which writes Sources.

### Bazarr

In Bazarr: Settings > Sonarr (and Settings > Radarr). Fill in HolyArrs's address and port, `/sonarr` (or `/radarr`) as the URL base, and the key for that side. If HolyArrs runs under a URL base such as `/holyarrs`, put it in front: `/holyarrs/sonarr`. Press Test.

What works:

- **Library sync.** Bazarr copies every show with its episodes, and every movie, with the file of the main Version: its path, size, release name, languages, quality and video codec. Like with Sonarr, an episode or movie without a file is skipped.
- **Pictures.** Posters and wide pictures come from HolyArrs through `/sonarr/MediaCover/...` and `/radarr/MediaCover/...`, with the key. Bazarr's smaller poster copies are made once and kept.
- **Rescans.** After a subtitle change Bazarr asks for a rescan of the show or movie. HolyArrs looks at that title's folder again.
- **Folders and tags.** Bazarr reads the Library folders (for its health checks) and the tag list.

What does not work:

- **Live updates.** HolyArrs has no live feed for these apps, so Bazarr's connection to it is refused. Bazarr still finds new files through its scheduled full sync (every 60 minutes by default) and its scheduled subtitle search. Bazarr keeps trying to connect; if that fills its log, start Bazarr with its option that turns the live feed off.
- **The folder browser** in Bazarr's path mapping editor. HolyArrs does not list its folders to apps; type the paths by hand.
- **Grab history.** HolyArrs does not keep the indexer page link of a grab, so the AvistaZ and CinemaZ subtitle sites cannot use it.
- **Webhooks into Bazarr.** HolyArrs does not send Sonarr- or Radarr-style webhooks; use the scheduled sync.

### Unpackerr

Unpackerr is not needed with HolyArrs. HolyArrs unpacks RAR sets itself during the import, and it imports only the files the download listed, so files Unpackerr unpacks next to them are never imported.

If you run it anyway, point its Sonarr and Radarr entries at `/sonarr` and `/radarr` with the keys. It polls the download list: a finished download waiting for its import shows as completed, with the download's own folder (or its single file) as the place to unpack, mapped through the downloader's path map the way HolyArrs sees it. Once HolyArrs has imported it, it leaves the list. Unpackerr changes nothing in HolyArrs.

### Prowlarr

Prowlarr can fill in HolyArrs's Sources from its own indexers, the way it does for Sonarr and Radarr. Each Prowlarr indexer becomes one Newznab or Torznab Source that points at Prowlarr.

In Prowlarr: Settings > Apps, add a Sonarr app for shows and a Radarr app for movies (one is enough if you only want one kind).

| Field | Sonarr app | Radarr app |
| --- | --- | --- |
| Prowlarr server | Prowlarr's address as HolyArrs reaches it, for example `http://prowlarr:9696` on the same Docker network or `http://10.10.25.25:9696` | same |
| Sonarr server or Radarr server | HolyArrs's address with the side on the end, for example `http://10.10.25.25:6262/sonarr` | `http://10.10.25.25:6262/radarr` |
| API key | the Sonarr-side key | the Radarr-side key |

If HolyArrs runs under a URL base such as `/holyarrs`, put it in front: `http://10.10.25.25:6262/holyarrs/sonarr`. Press Test, then Save. Prowlarr then writes its indexers in.

What happens:

- **New Sources arrive paused.** Look them over on the Sources page and resume the ones you want. When Prowlarr moves a Source to another address, or turns the local network back on after you turned it off, it is paused again; other changes (priority, categories, key) leave it on or off as it was.
- **The local network.** These Sources may reach the local network, because Prowlarr usually sits there. Only Sources Prowlarr writes with a valid key get this; every other Source still needs its own "May reach the local network" tick.
- **What HolyArrs uses:** the address and feed path, the key (kept encrypted like every Source key and only ever shown masked), the categories (on the shows side the anime ones too), the least seeders, the priority, and the three switches for the feed, automatic searches and hand searches. Shows-side Sources search for shows, movies-side ones for movies.
- **What is only kept:** seed ratio and seed times, extra address values, tags and the other values Prowlarr sends are stored as sent, so its next sync sees no difference and does not write again. HolyArrs does not use them; seeding follows the downloader's seed goal.
- **Updates.** Prowlarr reads each Source back and writes only when something differs. If you change a value Prowlarr manages (the name, address, categories or priority) in HolyArrs, Prowlarr puts its own back at its next full sync, as it does with Sonarr.
- **Removing.** Removing an indexer in Prowlarr removes its Source. A removed Source's number is never given to another, so Prowlarr never mixes two indexers up. Prowlarr only sees, changes and removes the Sources it wrote, each side its own, never the ones you added by hand.
- **Both sides.** The same indexer sent to both sides makes two Sources. When the name is taken, the second one gets "(shows)" or "(movies)" after it; Prowlarr still sees the name it sent.
- **Testing.** Prowlarr's Test, and each save, make HolyArrs ask Prowlarr's feed for its capabilities once. If that fails, Prowlarr saves again with its force option and the Source is made anyway, paused.
- **Who may.** The key acts as the person who made it, who must be allowed to change settings (Sources) as well as manage the Library. Otherwise Prowlarr gets 403 and nothing is written.

The calls answered on each side, under `/api/v3`: `GET /indexer`, `GET /indexer/{id}`, `GET /indexer/schema` (Newznab and Torznab only), `POST /indexer`, `PUT /indexer/{id}` (both with `forceSave`), `DELETE /indexer/{id}` and `POST /indexer/test`.

### Homepage

Homepage's Sonarr widget works: the show count, the download list and its unpaged details, the list of episodes that have aired but have no file (kept up to date ones by default), and the calendar (today to two days on by default, only kept up to date episodes unless it asks for all). Lists are paged the way Sonarr pages them: newest first when no direction is sent, 10 a page.

Homepage's Radarr widget is not supported yet: its missing list and calendar answer 404.

## What does not work (yet)

- If you delete a Source that Prowlarr made by hand, sync it again from Prowlarr (Sync App Indexers) so Prowlarr forgets the old one.

- Supported apps: Seerr, Overseerr, Bazarr, Unpackerr, Prowlarr and Homepage's Sonarr widget. Recyclarr, Maintainerr, Cleanuparr and other apps that talk to Sonarr or Radarr are not.
- These apps see one copy per title: the main Version. A 4K Version next to it is invisible to them.
- Choosing another Library folder for a title in Seerr is ignored; move titles in HolyArrs. Choosing other Pick rules in Seerr switches the main Version's Pick rules, which can start a search for a better copy.
- Changing a show's type (Seerr's series type) for a show already in the Library is ignored.
- Settings-type calls (Pick rules edits, scoring rules, naming, notifications, backups) are not answered. Indexers are answered only as Prowlarr writes them: Newznab and Torznab, one at a time (no bulk edits). Anything not listed here answers an error, never a pretend success.
- Tags are not attached to titles, so a calendar or list asked for by tag is not narrowed.
- Languages carry only their name (their number is always 0), and a title with no known original language says "Unknown".
- The status answer gives HolyArrs's own name and reports a version of 4.0.99.0 (Sonarr side) or 5.0.99.0 (Radarr side), because these apps decide which calls to make from the version number.
