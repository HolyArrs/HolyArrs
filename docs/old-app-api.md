# Connect Seerr or Overseerr

Seerr and Overseerr send approved requests to Sonarr and Radarr. HolyArrs can answer them in their place: point Seerr's Sonarr server at HolyArrs's `/sonarr` address and its Radarr server at `/radarr`, and requests land in your HolyArrs Library and show up as available once the files are in.

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
| Port | HolyArrs's port, for example `18730` | same |
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

## What does not work (yet)

- Only Seerr and Overseerr are supported. Prowlarr, Bazarr, Recyclarr, Unpackerr, Maintainerr and other apps that talk to Sonarr or Radarr are not.
- These apps see one copy per title: the main Version. A 4K Version next to it is invisible to them.
- Choosing another Library folder for a title in Seerr is ignored; move titles in HolyArrs. Choosing other Pick rules in Seerr switches the main Version's Pick rules, which can start a search for a better copy.
- Changing a show's type (Seerr's series type) for a show already in the Library is ignored.
- Settings-type calls (Pick rules edits, scoring rules, naming, indexers, notifications, backups) are not answered. Anything not listed here answers an error, never a pretend success.
- The status answer gives HolyArrs's own name and reports a version of 4.0.99.0 (Sonarr side) or 5.0.99.0 (Radarr side), because these apps decide which calls to make from the version number.
