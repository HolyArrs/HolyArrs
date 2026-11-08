# Apps that talk to Sonarr and Radarr

Some apps only know how to talk to Sonarr and Radarr. HolyArrs can answer them in their place. You give each app a HolyArrs address that ends in `/sonarr` or `/radarr`, plus a key, and the app treats HolyArrs as Sonarr or Radarr.

Pages in this section:

- [Connect Seerr or Overseerr](seerr.md): send requests from Seerr or Overseerr into your HolyArrs Library.
- [Connect Bazarr, Prowlarr, Unpackerr and Homepage](other-apps.md): the other apps that work this way.
- [Limits and troubleshooting](limits.md): what these apps cannot do with HolyArrs, and how to fix common problems.

This is a small API made only for those apps. If you want to write your own scripts, use [the HolyArrs API](../../dev/api.md) instead.

## Turn it on

It is off until you switch it on. While it is off, every address under `/sonarr/` and `/radarr/` answers 404.

1. Open **Settings > API** (`http://your-server:6262/settings/api`).
2. Find the card **Requests from Seerr and Overseerr**.
3. Switch on **Answer Seerr and Overseerr**.
4. Press **Make key** next to **Shows**. This is the Sonarr-side key. Copy it right away. It is shown only once.
5. Press **Make key** next to **Movies**. This is the Radarr-side key. Copy it too.

Making a key asks for your password again if you have not signed in with it recently.

The card also shows the URL base for each side, for example `/sonarr`. If HolyArrs runs under a [URL base](../getting-started/url-base.md) such as `/holyarrs`, the card shows `/holyarrs/sonarr` instead. Use what the card shows.

## Settings on the card

| Setting | What it does |
| --- | --- |
| Answer Seerr and Overseerr | Turns both `/sonarr` and `/radarr` on or off. |
| Make key / New key | Makes the key for that side. A new key stops the old one at once. |
| Remove key | Deletes the key for that side. Apps using it are refused from then on. |
| Let these apps delete files | Off by default. When off, removing a title from an app leaves the files on disk. When on, the files go to the holding folder first, like every delete in HolyArrs. |

For each key the card shows its last four characters, who made it and when it was last used.

## About the keys

- Each side has its own key, made only on this card. HolyArrs API keys (the ones starting with `hak_`) do not work on `/sonarr` or `/radarr`. These keys work nowhere else.
- A key acts as the person who made it. You need permission to manage the Library to make one.
- If that person's account is turned off, or loses that permission, apps are refused until someone makes a new key. The card warns you when this happens.
- Some apps send the key in the address (`?apikey=`). HolyArrs accepts that only on these addresses and hides the key in its own log. A reverse proxy in front of HolyArrs may still log the full address. Check its settings if that matters to you.
- Seerr stores keys in plain text in its settings file. That is why deleting files is off by default.

## Do it with the API

You can do the same with these calls, signed in with your password. The key calls need a recent password sign-in.

| What | Call |
| --- | --- |
| See the switches, addresses and keys | `GET /api/v1/settings/old-app-api` |
| Turn it on | `PUT /api/v1/settings/old-app-api` with `{"enabled": true}` |
| Let apps delete files | `PUT /api/v1/settings/old-app-api` with `{"delete_files": true}` |
| Make the Sonarr-side key | `POST /api/v1/settings/old-app-api/keys/sonarr` |
| Make the Radarr-side key | `POST /api/v1/settings/old-app-api/keys/radarr` |
| Remove a key | `DELETE /api/v1/settings/old-app-api/keys/sonarr` (or `radarr`) |
