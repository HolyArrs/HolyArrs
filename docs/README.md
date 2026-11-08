# HolyArrs docs

New here? Start with [Install HolyArrs with Docker](user/getting-started/install.md), then [First run](user/setup/first-run.md). On a NAS, see [Platforms](#platforms).

Writing code for HolyArrs? Go to [For developers](#for-developers).

## Getting started

- [Install HolyArrs with Docker](user/getting-started/install.md)
- [First run](user/setup/first-run.md)
- [Home and Settings](user/getting-started/home-and-settings.md)
- [Run under a path behind a reverse proxy](user/getting-started/url-base.md)
- [Import keys and passwords from one file](user/getting-started/import-secrets.md)

## Guides by area

| Area | Guides |
|---|---|
| Requests | [Asking](requests/asking.md), [approving](requests/approving.md), [reports](requests/reports.md), [limits and Request rules](requests/settings.md) |
| Movies | [Searching](user/movies/search-movies.md), [downloads](user/movies/downloads-movies.md), [organising](user/movies/organise-movies.md), [editions](user/movies/movie-editions.md), [movie sets](user/movies/collections.md) |
| Shows | [Searching](user/shows/search-shows.md), [downloads](user/shows/downloads-shows.md), [organising](user/shows/organise-shows.md) |
| Music | [Searching](user/music/search.md), [downloads](user/music/downloads.md), [organising](user/music/organise.md) |
| Explore and the calendar | [Explore](discover/explore.md), [people](discover/popular-people.md), [calendar](discover/calendar.md) |
| Sources | [Sources, Jackett and Prowlarr](user/sources/sources-bridge.md), [find titles in the Library](user/sources/library-search.md) |
| Downloads | [Downloaders](user/downloads/downloaders.md), [history](user/downloads/history.md) |
| Subtitles | [Subtitles](user/subtitles/subtitles.md) |
| Files and folders | [Library folders](user/mediafiles/library-folders.md), [codecs and media inspection](user/library/codecs-inspection.md), [audio versions](user/library/audio-versions.md), [protected copies](user/library/protected-copies.md) |
| Media servers | [Plex, Jellyfin and Emby](library/media-servers.md) |
| Metadata | [Metadata](user/metadata/metadata-service.md), [privacy](user/metadata/metadata-privacy.md), [trailers](library/trailers.md), [data packs](user/metadata/data-packs.md) |
| Accounts | [Sign-in and passwords](users/sign-in.md), [users and roles](users/roles.md), [kid accounts](users/kids.md), [signed-in devices](users/sessions.md) |
| Alerts | [Alerts](users/alerts.md) |
| Moving from other apps | [Move from other apps](user/migrate/move-from-other-apps.md), [how HolyArrs reads them](user/migrate/move-reading.md) |
| Older apps' API | [Connect Seerr, Overseerr, Bazarr and others](user/arrapi/README.md) |
| AI apps (held back) | [Connect AI apps](reference/ai.md) |
| Backups | [Backups](run/backups.md) |
| Health | [Health checks](run/health.md), [logs](run/logs.md), [schedules](run/schedules.md) |
| Updates | [Updates](run/updates.md) |

## Platforms

- [NAS, Synology and Unraid](run/platforms.md)
- [Portable archives (no Docker)](user/platforms/portable.md)
- [Health companion](user/platforms/companion.md)

## Help

- [Troubleshooting](user/help/troubleshooting.md): the log, the doctor check, the support bundle, the version
- [FAQ](user/help/faq.md)
- [Parts, safe mode, controls and API keys](user/help/parts-and-safe-mode.md)
- [Held back in the first release](run/updates.md#held-back-features)

## Find a guide by page

A changing number or token is in square brackets.

| Page | Address | Guide |
|---|---|---|
| Home | `/` | [Home and Settings](user/getting-started/home-and-settings.md) |
| New & Popular | `/popular` | [New & Popular](discover/popular-people.md) |
| Explore | `/explore` | [Explore](discover/explore.md) |
| Search | `/search` | Searching for [movies](user/movies/search-movies.md), [shows](user/shows/search-shows.md), [music](user/music/search.md) |
| People | `/people/[id]` | [People](discover/popular-people.md) |
| Calendar | `/calendar` | [Calendar](discover/calendar.md) |
| Title page | `/title/...` | [Ask for a title](requests/asking.md) |
| Requests | `/requests` | [Ask for a title](requests/asking.md), [approve or decline](requests/approving.md) |
| Reports | `/requests/reports` | [Report a problem](requests/reports.md) |
| Status | `/health` | [Health checks](run/health.md), [updates](run/updates.md) |
| My account | `/account`, `/settings/account` | [Signed-in devices](users/sessions.md), [sign-in](users/sign-in.md) |
| First run | `/login`, `/welcome` | [First run](user/setup/first-run.md) |
| Join by invite | `/invite/[token]` | [Users and roles](users/roles.md) |
| Downloads | `/downloads` | Downloads for [movies](user/movies/downloads-movies.md), [shows](user/shows/downloads-shows.md), [music](user/music/downloads.md) |
| History | `/downloads/history` | [History](user/downloads/history.md) |
| Needs a decision | `/downloads/decide` | Downloads for [movies](user/movies/downloads-movies.md), [shows](user/shows/downloads-shows.md), [music](user/music/downloads.md) |
| Library | `/library` | [Find titles in the Library](user/sources/library-search.md) |
| Movie, show or artist | `/library/[id]` | Organising [movies](user/movies/organise-movies.md), [shows](user/shows/organise-shows.md), [music](user/music/organise.md) |
| Held and turned-down releases | `/library/held`, `/library/turned-down` | Searching for [movies](user/movies/search-movies.md), [shows](user/shows/search-shows.md), [music](user/music/search.md) |
| Import a library, Rename files | `/library/import`, `/library/rename` | Organising [movies](user/movies/organise-movies.md), [shows](user/shows/organise-shows.md), [music](user/music/organise.md) |
| Missing subtitles | `/library/missing-subtitles` | [Subtitles](user/subtitles/subtitles.md) |
| Connect an AI app | `/ai` | [Connect AI apps](reference/ai.md) (held back) |
| Settings | `/settings` | [Home and Settings](user/getting-started/home-and-settings.md#settings) |
| Request rules | `/settings/requests` | [Weekly limits and Request rules](requests/settings.md) |
| AI apps | `/settings/ai` | [Connect AI apps](reference/ai.md) (held back) |
| Alerts | `/settings/alerts` | [Alerts](users/alerts.md) |
| Backups | `/settings/backups` | [Backups](run/backups.md) |
| Move from other apps | `/settings/move` | [Move from other apps](user/migrate/move-from-other-apps.md) |
| Downloaders | `/settings/downloaders` | [Downloaders](user/downloads/downloaders.md) |
| Library folders | `/settings/library-folders` | [Library folders](user/mediafiles/library-folders.md) |
| Media servers | `/settings/media-servers` | [Media servers](library/media-servers.md) |
| Parts | `/settings/parts` | [Parts and safe mode](user/help/parts-and-safe-mode.md) |
| Sign-in and security | `/settings/sign-in` | [Sign-in and passwords](users/sign-in.md) |
| Sources | `/settings/sources` | [Sources](user/sources/sources-bridge.md) |
| This app | `/settings/this-app` | [Logs](run/logs.md), [updates](run/updates.md), [URL base](user/getting-started/url-base.md) |
| Logs | `/settings/logs` | [Logs](run/logs.md) |
| Schedules | `/settings/schedules` | [Schedules](run/schedules.md) |
| API | `/settings/api` | [The HolyArrs API](dev/api.md) |
| Metadata | `/settings/tmdb` | [Metadata](user/metadata/metadata-service.md) |
| Users and roles | `/settings/users` | [Users and roles](users/roles.md), [kid accounts](users/kids.md) |
| Naming and files | `/settings/naming` | Organising [movies](user/movies/organise-movies.md), [shows](user/shows/organise-shows.md), [music](user/music/organise.md) |
| Outside lists | `/settings/outside-lists` | [Ask for a title](requests/asking.md) (no Outside lists guide yet) |
| Search settings | `/settings/search` | Searching for [movies](user/movies/search-movies.md), [shows](user/shows/search-shows.md), [music](user/music/search.md) |
| Music defaults | `/settings/music` | [Organising music](user/music/organise.md) |
| Subtitles | `/settings/subtitles` | [Subtitles](user/subtitles/subtitles.md) |

## For developers

- [Architecture](dev/architecture.md): the code map
- [Setup](dev/setup.md): build and run from source
- [Modules](dev/modules.md): the 20 modules, the layer order and the rules between them
- [Testing](dev/testing.md)
- [The HolyArrs API](dev/api.md)
- [Releasing](dev/releasing.md)
