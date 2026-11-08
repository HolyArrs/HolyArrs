# Metadata

You want names, dates, posters and ids for your movies, shows and music. HolyArrs gets them for you from the HolyArrs metadata server. You need no keys, and a new install works without typing anything.

## Check that it works

1. Open **Settings > Metadata**.
2. Look at the **Metadata server** card. It says whether the server answers.

That is all there is to set up. On its first lookup, your install registers with the server once and gets its own credential. HolyArrs keeps it encrypted and never shows it.

## Where each kind of data comes from

- **Movies and shows.** Searches, details, seasons, Explore rows, people, age ratings and TMDB lists come from the metadata server. Show details can also come from TVMaze. Neither needs a key.
- **Posters and backdrops.** These load from TMDB's picture host in TMDB's own sizes. HolyArrs falls back to the metadata server only when TMDB can't be reached.
- **Music.** Details come from MusicBrainz. There is no key field. Its rate and contact settings are under **Settings > Music defaults**.
- **Music pictures.** Album covers, artist photos, backgrounds and logos come from the metadata server, through your install's artwork cache. HolyArrs never asks a picture site itself.
- **Artist top songs.** The top songs on an artist page, and their explicit marks, come from Deezer's public API. It needs no key.

What the server learns about your install is on [Metadata privacy](metadata-privacy.md).

## Settings

| Setting | Where | What it does |
| --- | --- | --- |
| `HOLYARRS_METADATA_URL` | environment | Points HolyArrs at another metadata server. The default is `https://holyarrs-api.coderluii.dev`. |
| `HOLYARRS_TMDB_KEY` | environment | Your own TMDB read token. Movie and show lookups then go to TMDB directly. |
| `HOLYARRS_DEEZER_URL=off` | environment | Turns off artist top songs. |
| **Trakt key** | **Settings > Outside lists** | Held back. See below. |

### Trakt

Trakt lists are held back in this release. They stay off until the `trakt_lists` switch is on, see [Held back](../../run/updates.md#held-back-features). With the switch on, open **Settings > Outside lists**, find **Trakt key**, and paste the Client ID of a Trakt app. After you save, you only see a masked value.

### Keys from other apps

[Import secrets](../getting-started/import-secrets.md) reads app keys from one local file. The preview masks the values, and the import stores them in the encrypted vault. Never put real keys in a tracked file.

## Troubleshooting

**Home, Search or Explore says the metadata server is down.** Searches and new artwork come back when the server answers again. HolyArrs checks at most once a minute. Your Library, downloads and everything already saved keep working meanwhile.

**You set `HOLYARRS_METADATA_URL` and nothing loads.** Check the address from the HolyArrs machine, then restart HolyArrs so it reads the new value.

**Artist pages show no top songs.** Check that `HOLYARRS_DEEZER_URL` is not set to `off`.
