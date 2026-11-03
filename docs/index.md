# HolyArrs documentation

Start with [Install HolyArrs with Docker](install.md), or [Install on a NAS](nas.md). The current image is a
local build and has not been published to a registry.

## Explore and trailers

- [Explore](explore.md): rows, the type switch, Trending rows, badges, smart row
  titles, new-episode notices and Asked by.
- [Trailers](trailers.md): how trailers play, the settings and privacy.

## Find a guide by screen

Every current web route is listed below. A changing number or token is shown
in square brackets.

| Screen | Route | Guide |
| --- | --- | --- |
| Home | `/` | [Home and Settings](home-and-settings.md), [History and the calendar](history-and-calendar.md), and [Alerts, health checks and media servers](alerts-and-health.md). |
| My account | `/account`, `/settings/account` | [Signed-in devices and passwords](sessions.md), [Sign-in and passwords](sign-in.md), and API keys in [Parts, safe mode, controls and API keys](parts-and-safe-mode.md#api-keys-and-scopes). |
| First-run setup | `/login`, `/welcome` | [Install HolyArrs with Docker](install.md#finish-the-first-run), [Sign-in and passwords](sign-in.md), and [Parts and safe mode](parts-and-safe-mode.md#the-first-run-step-what-do-you-use). |
| Join by invite | `/invite/[token]` | [Users, roles and two-factor sign-in](users-and-roles.md#make-and-use-an-invite) and [Sign-in and passwords](sign-in.md). |
| Downloads | `/downloads` | [Movies](downloads-movies.md), [shows](downloads-shows.md), [music](downloads-music.md), and [Downloaders](downloaders.md). |
| History | `/downloads/history` | [History and the calendar](history-and-calendar.md#history). |
| Needs a decision | `/downloads/decide` | [Movies](downloads-movies.md), [shows](downloads-shows.md), and [music](downloads-music.md). |
| Library | `/library` | [Find titles in the Library](library-search.md), then organise [movies](organise-movies.md), [shows](organise-shows.md), or [music](organise-music.md). |
| Movie, show or artist | `/library/[id]` | Request, search, download and organise guides for [movies](request-movies.md), [shows](request-shows.md), and [music](request-music.md). |
| Album | `/library/[id]/albums/[album]` | [Organise music](organise-music.md) and [Downloads for music](downloads-music.md). |
| Held Releases | `/library/held` | [Searching for movies](search-movies.md), [shows](search-shows.md), and [music](search-music.md). |
| Turned-down Releases | `/library/turned-down` | [Searching for movies](search-movies.md), [shows](search-shows.md), and [music](search-music.md). |
| Import a library | `/library/import` | [Organise movies](organise-movies.md), [shows](organise-shows.md), and [music](organise-music.md). |
| Rename files | `/library/rename` | [Organise movies](organise-movies.md), [shows](organise-shows.md), and [music](organise-music.md). |
| Missing subtitles | `/library/missing-subtitles` | [Subtitles](subtitles.md). |
| Search | `/search` | [Searching for movies](search-movies.md), [shows](search-shows.md), and [music](search-music.md). |
| Calendar | `/calendar` | [History and the calendar](history-and-calendar.md#calendar). |
| Requests | `/requests` | Adding and asking guides for [movies](request-movies.md), [shows](request-shows.md), and [music](request-music.md). |
| Settings | `/settings` | [Home and Settings](home-and-settings.md#settings), plus this screen-to-guide map. |
| Alerts | `/settings/alerts` | [Alerts, health checks and media servers](alerts-and-health.md). |
| Backups | `/settings/backups` | [Backups](backups.md), including schedules, the master key and restore. |
| Move from other apps | `/settings/move` | [Move from other apps](move-from-other-apps.md), [reading their setup](move-reading.md), and [import secrets](import-secrets.md). |
| Downloaders | `/settings/downloaders` | [Downloaders](downloaders.md) and the shared `/data` layout in [Install](install.md#plan-the-two-host-locations). |
| Library folders | `/settings/library-folders` | [Library folders](library-folders.md) and the shared `/data` layout in [Install](install.md). |
| Media servers | `/settings/media-servers` | [Alerts, health checks and media servers](alerts-and-health.md#media-servers). |
| Parts and controls | `/settings/parts` | [Parts, safe mode, controls and API keys](parts-and-safe-mode.md). |
| Sign-in | `/settings/sign-in` | [Sign-in and passwords](sign-in.md), [signed-in devices](sessions.md), and safe sign-in behavior in [Parts and safe mode](parts-and-safe-mode.md). |
| Sources | `/settings/sources`, `/settings/sources/[id]` | [Sources, and Jackett or Prowlarr as a bridge](sources-bridge.md). |
| This app | `/settings/this-app` | [Autoplay trailers and Keep yt-dlp up to date](trailers.md#settings), [Logs and storage](logs.md), [updates](updates.md), [health](alerts-and-health.md#health-checks), and [URL base](url-base.md). |
| Logs | `/settings/logs` | [Logs and storage](logs.md). |
| Schedules | `/settings/schedules` | [Schedules](schedules.md). |
| API for other apps | `/settings/api` | [The API for other apps](api.md) and [API keys and scopes](parts-and-safe-mode.md#api-keys-and-scopes). |
| Metadata | `/settings/tmdb` | [Metadata](metadata-keys.md): no keys needed, and whether the metadata server answers. |
| Users | `/settings/users` | [Users, roles and two-factor sign-in](users-and-roles.md), [Kid accounts](kid-accounts.md) and [Signed-in devices](sessions.md). |
| Naming and files | `/settings/naming` | [Organise movies](organise-movies.md), [shows](organise-shows.md), and [music](organise-music.md). |
| Outside lists | `/settings/outside-lists` | [Adding and asking for movies](request-movies.md#outside-lists), [Metadata](metadata-keys.md#trakt), [Schedules](schedules.md), and [Import secrets](import-secrets.md). |
| Search settings | `/settings/search` | [Searching for movies](search-movies.md), [shows](search-shows.md), and [music](search-music.md). |
| Music settings | `/settings/music` | [Adding and asking for music](request-music.md), [searching for music](search-music.md), and [organising music](organise-music.md). |
| Subtitles | `/settings/subtitles` | [Subtitles](subtitles.md). |
| Request settings | `/settings/wishlist` | Adding and asking guides for [movies](request-movies.md), [shows](request-shows.md), and [music](request-music.md). |

## Guides without their own screen

- [Import keys and passwords from one file](import-secrets.md)
- [Update check](updates.md)
- [Running under a path behind a reverse proxy](url-base.md)
- [Word check for contributors](word-check.md)

## Coverage note

The route map covers all current web routes. These guides are checked against
the local pre-release source. They do not claim a published image or completed
manual install acceptance on another person's machine.
