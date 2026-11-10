# HolyArrs docs

New here? Start with [Install HolyArrs with Docker](start/install.mdx), then [First run](start/first-run.md). On a NAS, see [Platforms](#platforms).

Writing code for HolyArrs? Go to [For developers](#for-developers).

## Getting started

- [Install HolyArrs with Docker](start/install.mdx)
- [First run](start/first-run.md)
- [Home and Settings](start/home-and-settings.md)
- [Run under a path behind a reverse proxy](run/url-base.md)
- [Import keys and passwords from one file](migrate/import-secrets.md)

## Guides by area

| Area | Guides |
|---|---|
| Requests | [Asking](requests/asking.md), [approving](requests/approving.md), [reports](requests/reports.md), [limits and Request rules](requests/settings.md) |
| Movies | [Searching](movies/search.md), [downloads](movies/downloads.md), [organising](movies/organise.md), [editions and collections](movies/editions-collections.md) |
| Shows | [Searching](shows/search.md), [downloads](shows/downloads.md), [organising](shows/organise.md) |
| Music | [Searching](music/search.md), [downloads](music/downloads.md), [organising](music/organise.md) |
| Explore and the calendar | [Explore](discover/explore.md), [people](discover/popular-people.md), [calendar](discover/calendar.md) |
| Sources | [Sources, Jackett and Prowlarr](downloads/sources.md), [find titles in the Library](downloads/library-search.md) |
| Downloads | [Downloaders](downloads/downloaders.md), [history](downloads/history.md) |
| Subtitles | [Subtitles](library/subtitles.md) |
| Files and folders | [Library folders](library/folders.md), [codecs and media inspection](library/codecs-inspection.md), [audio versions](library/audio-versions.md), [protected copies](library/protected-copies.md) |
| Media servers | [Plex, Jellyfin and Emby](library/media-servers.md) |
| Metadata | [Metadata and privacy](library/metadata.md), [trailers](library/trailers.md), [data packs](library/data-packs.md), [TRaSH Guides scores](library/trash-guides.md) |
| Accounts | [Sign-in and passwords](users/sign-in.md), [users and roles](users/roles.md), [kid accounts](users/kids.md), [signed-in devices](users/sessions.md) |
| Alerts | [Alerts](users/alerts.md) |
| Importing from other apps | [Import from other apps](migrate/index.md), [how HolyArrs reads them](migrate/what-is-read.md) |
| Older apps' API | [Connect Seerr, Overseerr, Bazarr and others](reference/old-app-api.md) |
| AI apps (held back) | [Connect AI apps](reference/ai.md) |
| Backups | [Backups](run/backups.md) |
| Health | [Health checks](run/health.md), [logs](run/logs.md), [schedules](run/schedules.md) |
| Updates | [Updates](run/updates.md) |

## Platforms

- [NAS, Synology and Unraid](run/platforms.md)
- [Portable archives (no Docker)](run/portable.md)

## Help

- [Troubleshooting](help/troubleshooting.md): the log, the doctor check, the support bundle, the version
- [FAQ](help/faq.md)
- [Parts, safe mode, controls and API keys](run/parts-safe-mode.md)
- [Held back in the first release](run/updates.md#held-back-features)

## Find a guide by page

A changing number or token is in square brackets.

| Page | Address | Guide |
|---|---|---|
| Home | `/` | [Home and Settings](start/home-and-settings.md) |
| New & Popular | `/popular` | [New & Popular](discover/popular-people.md) |
| Explore | `/explore` | [Explore](discover/explore.md) |
| Search | `/search` | Searching for [movies](movies/search.md), [shows](shows/search.md), [music](music/search.md) |
| People | `/people/[id]` | [People](discover/popular-people.md) |
| Calendar | `/calendar` | [Calendar](discover/calendar.md) |
| Title page | `/title/...` | [Ask for a title](requests/asking.md) |
| Requests | `/requests` | [Ask for a title](requests/asking.md), [approve or decline](requests/approving.md) |
| Reports | `/requests/reports` | [Report a problem](requests/reports.md) |
| Status | `/health` | [Health checks](run/health.md), [updates](run/updates.md) |
| My account | `/account`, `/settings/account` | [Signed-in devices](users/sessions.md), [sign-in](users/sign-in.md) |
| First run | `/login`, `/welcome` | [First run](start/first-run.md) |
| Join by invite | `/invite/[token]` | [Users and roles](users/roles.md) |
| Downloads | `/downloads` | Downloads for [movies](movies/downloads.md), [shows](shows/downloads.md), [music](music/downloads.md) |
| History | `/downloads/history` | [History](downloads/history.md) |
| Needs a decision | `/downloads/decide` | Downloads for [movies](movies/downloads.md), [shows](shows/downloads.md), [music](music/downloads.md) |
| Library | `/library` | [Find titles in the Library](downloads/library-search.md) |
| Movie, show or artist | `/library/[id]` | Organising [movies](movies/organise.md), [shows](shows/organise.md), [music](music/organise.md) |
| Held and turned-down releases | `/library/held`, `/library/turned-down` | Searching for [movies](movies/search.md), [shows](shows/search.md), [music](music/search.md) |
| Import a library, Rename files | `/library/import`, `/library/rename` | Organising [movies](movies/organise.md), [shows](shows/organise.md), [music](music/organise.md) |
| Missing subtitles | `/library/missing-subtitles` | [Subtitles](library/subtitles.md) |
| Connect an AI app | `/ai` | [Connect AI apps](reference/ai.md) (held back) |
| Settings | `/settings` | [Home and Settings](start/home-and-settings.md#settings) |
| Request rules | `/settings/requests` | [Weekly limits and Request rules](requests/settings.md) |
| AI apps | `/settings/ai` | [Connect AI apps](reference/ai.md) (held back) |
| Alerts | `/settings/alerts` | [Alerts](users/alerts.md) |
| Backups | `/settings/backups` | [Backups](run/backups.md) |
| Import from other apps | `/settings/move` | [Import from other apps](migrate/index.md) |
| Downloaders | `/settings/downloaders` | [Downloaders](downloads/downloaders.md) |
| Library folders | `/settings/library-folders` | [Library folders](library/folders.md) |
| Media servers | `/settings/media-servers` | [Media servers](library/media-servers.md) |
| Parts | `/settings/parts` | [Parts and safe mode](run/parts-safe-mode.md) |
| Sign-in and security | `/settings/sign-in` | [Sign-in and passwords](users/sign-in.md) |
| Sources | `/settings/sources` | [Sources](downloads/sources.md) |
| This app | `/settings/this-app` | [Logs](run/logs.md), [updates](run/updates.md), [URL base](run/url-base.md) |
| Logs | `/settings/logs` | [Logs](run/logs.md) |
| Schedules | `/settings/schedules` | [Schedules](run/schedules.md) |
| API | `/settings/api` | [The HolyArrs API](dev/api.md) |
| Metadata | `/settings/tmdb` | [Metadata](library/metadata.md) |
| Users and roles | `/settings/users` | [Users and roles](users/roles.md), [kid accounts](users/kids.md) |
| Naming and files | `/settings/naming` | Organising [movies](movies/organise.md), [shows](shows/organise.md), [music](music/organise.md) |
| Outside lists | `/settings/outside-lists` | [Ask for a title](requests/asking.md) (no Outside lists guide yet) |
| Search settings | `/settings/search` | Searching for [movies](movies/search.md), [shows](shows/search.md), [music](music/search.md) |
| Music defaults | `/settings/music` | [Organising music](music/organise.md) |
| Subtitles | `/settings/subtitles` | [Subtitles](library/subtitles.md) |

## For developers

- [Architecture](dev/architecture.md): the code map
- [Setup](dev/setup.md): build and run from source
- [Modules](dev/modules.md): the 20 modules, the layer order and the rules between them
- [Testing](dev/testing.md)
- [The HolyArrs API](dev/api.md)
- [Releasing](dev/releasing.md)
