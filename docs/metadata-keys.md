# Metadata keys

HolyArrs uses outside catalogues for names, dates, pictures and identifiers.
It ships without personal TMDB, TVDB or Trakt credentials. Settings that store
a key require **Change settings** and a password sign-in.

## TMDB

Movies and TMDB-backed shows need the long **API Read Access Token** from a
TMDB account. During first setup, HolyArrs asks for it when Movies or Shows is
on. Later, open **Settings > TMDB key**.

The screen shows whether a token is saved and only a masked form of the current
value. Paste a token to add or replace it. HolyArrs stores it encrypted and
does not show the full value again.

An operator may instead set `HOLYARRS_TMDB_TOKEN_FILE` to a file containing the
token. When that setting is active, the screen says the key comes from a file;
change that file rather than the form.

## Trakt

Trakt Outside lists need the Client ID of a Trakt app. Open
**Settings > Outside lists**, find **Trakt key**, and paste or replace the
Client ID. The screen shows only a masked value after saving it. A Trakt list
cannot be read until this key is set.

## TVDB

TVDB is an optional show-details source and needs a personal TVDB API key.
HolyArrs ships no key. The current web app has no TVDB-key form. Import it from
a local secrets file as described in [Import secrets](import-secrets.md), or
use the settings route in [the API reference](api.md). Sending an empty value
through that API stops using TVDB.

TMDB and TVMaze remain available show-details choices; TVMaze does not need a
key.

## MusicBrainz

MusicBrainz is used for music details and does not have a key field in
HolyArrs. Its rate and contact settings are under **Settings > Music**.

## Moving keys from other apps

[Import secrets](import-secrets.md) accepts the supported TMDB, TVDB and Trakt
fields from one local file. The preview masks values, and applying the import
stores them in the encrypted vault. Do not put real keys in a tracked file.

