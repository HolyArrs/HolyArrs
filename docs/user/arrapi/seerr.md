# Connect Seerr or Overseerr

You want requests made in Seerr or Overseerr to land in your HolyArrs Library. Seerr sends approved requests to a Sonarr and a Radarr server. You add HolyArrs as both.

First turn the feature on and make both keys. See [Turn it on](README.md#turn-it-on).

## Steps

1. In Seerr, open **Settings > Services**.
2. Press **Add Sonarr Server** and fill in the fields below with the Sonarr-side key.
3. Press **Test**. Seerr lists your HolyArrs Pick rules as quality profiles and your show Library folders as root folders. Pick the ones new requests should use.
4. Save.
5. Press **Add Radarr Server** and do the same with the Radarr-side key and `/radarr`.

| Field | Sonarr server | Radarr server |
| --- | --- | --- |
| Hostname or IP | Your HolyArrs host, for example `your-server`, or `holyarrs` on the same Docker network | same |
| Port | `6262`, or the port you use | same |
| Use SSL | Only if you reach HolyArrs over HTTPS | same |
| API key | The Sonarr-side key | The Radarr-side key |
| URL base | `/sonarr` | `/radarr` |

If HolyArrs runs under a URL base such as `/holyarrs`, put it in front: `/holyarrs/sonarr`.

Add each side only once. Do not also add the `/radarr` address as Seerr's 4K server. HolyArrs keeps every copy of a movie on one title, so removing the "4K" one in Seerr would remove the whole movie.

## What happens next

- **Requests.** An approved show request adds the show and keeps only the requested seasons up to date. Asking for another season later switches that season on. Anime shows are added with absolute episode numbers. An approved movie request adds the movie with the Release gate that matches Seerr's minimum availability.
- **Searching.** When the request asks for a search, HolyArrs first looks at the title's folder, then searches. Files you already have are not grabbed again. Seerr's "search now" starts a hand search in HolyArrs.
- **Progress.** Seerr's progress bars read HolyArrs Downloads, one line per episode for shows.
- **Available.** Seerr's scan reads the Library. A season counts as available once every episode in it has a file. A season with episodes still to air stays "partly available", the same as with Sonarr. A movie is available once it has a file.
- **Removing.** Seerr's "remove from Sonarr/Radarr" takes the title out of the Library. Whether files are deleted depends on **Let these apps delete files**, see [the settings](README.md#settings-on-the-card). If Seerr asks for an exclusion, the title goes on the Never-add list.
- **Tags.** Seerr's per-user tags are made and kept, so Seerr finds them again. They are not attached to titles.
- **Open in.** Seerr's "open in Sonarr/Radarr" link opens the title in HolyArrs.

## Troubleshooting

- **Test fails with 404.** The feature is off, or the URL base is wrong. Check the switch and copy the URL base from the card.
- **Test fails with 401 or 403.** The key is wrong, was replaced, or its owner lost permission to manage the Library. Make a new key and paste it in.
- **Changing the root folder in Seerr does nothing.** That is expected. Move titles in HolyArrs.

More in [Limits and troubleshooting](limits.md).
