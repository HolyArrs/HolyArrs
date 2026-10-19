# Metadata

HolyArrs gets names, dates, pictures and identifiers from the HolyArrs
metadata server. Metadata needs no keys: a new install works without typing
anything.

## The metadata server

On its first lookup, each install registers itself with the server once and
gets its own credential, which HolyArrs keeps encrypted and never shows. The
server answers movie and show searches and details, seasons, Explore rows,
people, age ratings, TMDB lists and music lookups. Pictures load straight
from TMDB's picture host in TMDB's own sizes.

The server's address is `https://holyarrs-api.coderluii.dev`. An operator can
point HolyArrs elsewhere with `HOLYARRS_METADATA_URL`.

**Settings > Metadata** shows whether the server answers. When it does not,
Home, Search, Explore and the add screens show "Metadata server is down —
searches and new artwork will return when it's back". HolyArrs checks at most
once a minute. The Library, downloads and everything already saved keep
working.

## Trakt

Trakt Outside lists need the Client ID of a Trakt app. Open
**Settings > Outside lists**, find **Trakt key**, and paste or replace the
Client ID. The screen shows only a masked value after saving it. A Trakt list
cannot be read until this key is set.

## Show details

Shows take their details from TMDB (through the metadata server) or TVMaze.
Neither needs a key.

## MusicBrainz

MusicBrainz is used for music details and does not have a key field in
HolyArrs. Its rate and contact settings are under **Settings > Music**.

## Artist pictures (Deezer)

MusicBrainz has no pictures, so an artist's picture comes from Deezer's
public artist search (`https://api.deezer.com/search/artist?q=<name>`), which
needs no key. HolyArrs looks once when an artist is added, and again on a
refresh while the artist still has no picture. Only an artist with the very
same name (case aside) counts. The picture is kept in the artwork cache like
a movie poster; answers are kept for 24 hours. When Deezer does not answer,
nothing fails: the artist simply has no picture until a later refresh.

`HOLYARRS_DEEZER_URL` names a stand-in (tests), and `off` turns the source
off. When `HOLYARRS_MUSICBRAINZ_URL` names a MusicBrainz stand-in and no
Deezer stand-in is named, the source is off, so tests never reach Deezer.

## Moving keys from other apps

[Import secrets](import-secrets.md) accepts the Trakt key and other app keys
from one local file. The preview masks values, and applying the import stores
them in the encrypted vault. Do not put real keys in a tracked file.

