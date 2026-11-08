# Metadata privacy

You want to know what your install tells the metadata server. This page lists all of it. You can also see a short version on **Settings > Metadata**, in the **What we send** card.

## What is sent

- **An install token.** On first start HolyArrs asks the server for one with an empty request. After that every lookup carries it. It identifies the install, not a person.
- **What you look up.** The words you type in Explore or Search, and the ids of the movies, shows, people, artists and albums a page shows (TMDB and MusicBrainz ids).
- **Display choices.** The language and region the app shows, so posters, trailers and "where to watch" match.
- **A few standard request headers.** These say what kind of answer HolyArrs wants, whether a stored list is still current, and whether HolyArrs is loading a page ahead of time.
- **Install count by country.** The server counts installs per country, by where requests come from.

Like any web request, the server also sees your public IP address.

## What is never sent

- Your files, folder paths or file names.
- Your library: what you have, what is downloading, your history.
- Accounts: user names, passwords, who asked for what.
- Keys and addresses of your downloaders, sources, media servers and lists.
- Crash reports. Apart from the per-country count, HolyArrs has no telemetry.

## Leave this install out of the count

1. Open **Settings > This app**.
2. In the **This server** card, turn off **Count this install**.

Requests then tell the server to leave your install out.

## Skip the metadata server for movies and shows

Set `HOLYARRS_TMDB_KEY` to your own TMDB read token. Movie and show lookups then go to TMDB directly. See [Metadata](metadata-service.md).

## Troubleshooting

**You turned off Count this install and want to check it.** The switch is saved for the whole install. Reload **Settings > This app** and check it is still off.
