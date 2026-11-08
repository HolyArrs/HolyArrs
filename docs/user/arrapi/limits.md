# Limits and troubleshooting

You want to know what these apps cannot do with HolyArrs, or why one of them fails.

## Supported apps

Seerr, Overseerr, Bazarr, Unpackerr, Prowlarr and the Homepage Sonarr widget. Recyclarr, Maintainerr, Cleanuparr and other apps that talk to Sonarr or Radarr are not supported.

## What these apps cannot do

- They see one copy per title, the main Version. A 4K Version next to it is invisible to them.
- Choosing another Library folder for a title is ignored. Move titles in HolyArrs.
- Choosing other Pick rules switches the main Version's Pick rules. That can start a search for a better copy.
- Changing the series type of a show already in the Library is ignored.
- Settings calls, such as Pick rules edits, naming, notifications and backups, are not answered. Indexers are answered only the way Prowlarr writes them, one at a time.
- Anything not answered returns an error. HolyArrs never pretends a call worked.
- Tags are not attached to titles, so lists filtered by tag are not narrowed.
- Languages carry only their name. A title with no known original language says "Unknown".
- The status call reports version 4.0.99.0 on the Sonarr side and 5.0.99.0 on the Radarr side, because these apps pick which calls to make from the version number.

## Troubleshooting

| Problem | Fix |
| --- | --- |
| Every call answers 404 | Switch on **Answer Seerr and Overseerr** in **Settings > API**. Check the URL base, including your HolyArrs URL base if you use one. |
| 401 | The key is wrong or was replaced. Make a new key and paste it into the app. |
| 403 | The key's owner is turned off or lost permission to manage the Library. For Prowlarr, the owner also needs permission to change settings. Make a new key as someone who has both. |
| A HolyArrs `hak_` key is refused | Expected. Use the keys from the **Requests from Seerr and Overseerr** card. |
| Files stay on disk after removing in Seerr | Expected while **Let these apps delete files** is off. |
| You deleted a Prowlarr Source by hand | Run **Sync App Indexers** in Prowlarr so it forgets the old one. |
| Bazarr keeps logging connection errors | HolyArrs has no live feed. Turn Bazarr's live feed off. Its scheduled sync still works. |

Back to [Apps that talk to Sonarr and Radarr](README.md).
