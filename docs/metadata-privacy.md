# What HolyArrs sends to the metadata server

HolyArrs looks up posters, titles, music and trailers through the HolyArrs
metadata server (`https://holyarrs-api.coderluii.dev` unless
`HOLYARRS_METADATA_URL` points somewhere else). This page lists everything an
install sends there.

## What is sent

- **An install token.** On first start HolyArrs asks the server for one with an
  empty request. After that every lookup carries it as
  `Authorization: Bearer <token>`. It identifies the install, not a person.
- **What you look up.** Search words you type in Explore or Search, and the IDs
  of the movies, shows, people, artists and albums a page shows (TMDB IDs and
  MusicBrainz IDs).
- **Display choices.** The language and region the app shows, so posters,
  trailers and "where to watch" match.
- **Small request headers.** `Accept`, `Cache-Control`, `If-None-Match` for
  lists already stored, and `X-Fetch: ahead` when HolyArrs is loading a page
  ahead of time instead of because someone opened it.

Like any web request, the server sees the public IP address the request comes
from.

## What is never sent

- Your files, folder paths or file names.
- Your library: which titles you have, what is downloading, your history.
- Accounts: user names, passwords, sign-in sessions, who asked for what.
- Keys and addresses of your downloaders, sources, media servers and lists.
- Usage counts or crash reports. HolyArrs has no telemetry.

## Using your own key instead

Set `HOLYARRS_TMDB_KEY` to your own TMDB read token and movie and show lookups
go to TMDB directly instead of the metadata server.

The test `TestMetadataRequestsCarryOnlyAllowedHeaders` in
`internal/metadata/privacy_test.go` fails if a request to the metadata server
ever carries a header beyond the ones listed here.
