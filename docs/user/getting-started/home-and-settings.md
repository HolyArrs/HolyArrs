# Home and Settings

Home is the working summary of this HolyArrs. Settings is the map to every
configuration screen the signed-in person may use.

## First setup

After the owner account is made, HolyArrs asks an owner or admin to:

1. Turn Movies, Shows, Music and Subtitles on or off, and say whether family
   or friends will use Requests.
2. Add one Library folder for movies, shows or music.

Metadata needs no keys; see [Metadata](metadata-keys.md).

The **Just movies** button turns on Movies and turns the other optional parts
off. Parts may be changed later under **Settings > Parts and controls**.

See [Install HolyArrs with Docker](install.md#finish-the-first-run),
[Metadata](metadata-keys.md), and [Library folders](library-folders.md).

## Home

Home changes as the Library and linked apps change. It shows:

- **Downloading now** for active, queued and stalled Downloads, with progress,
  speed and an estimated finish time when those facts are available.
- **Searches** for running and recent searches, with links to their details.
- **Missing** for the first eight wanted or searching slots.
- **Latest Events** for the latest twelve history Events.
- **Health** for the first five current problems, or that all checks passed.
- **This HolyArrs** with its version, uptime, Source count and Downloader count.

The page updates while downloads, searches, imports, health checks and other
background work run. Use the linked screen when a card needs more detail.

### Getting started

Until the basic setup is complete, Home lists the remaining steps. The current
checklist covers a Library folder, a Source, a Downloader and a
first movie. A music-only or shows-only setup can still use the relevant
Settings screens even when that movie-oriented checklist remains visible.

## Settings

The Settings landing page is a set of links. A link is shown only when its
Part is on and the current person may open it:

- The owner may open every settings screen.
- An admin or member with **Change settings** may open the ordinary settings
  screens.
- The owner and admins may open **Move from other apps**, **Parts and
  controls**, **Users**, and **Logs**.
- Everyone may open **My account**.

The complete screen-to-guide map is in the [documentation index](index.md).

### What needs attention

`GET /api/v1/settings/status` (Change settings) gives the Settings home one
answer: `{"items": [{"group", "page", "text", "severity"}], "unset": {"<group>":
["<page>"]}, "known": ["<page>"], "metadata_up": bool}`. Groups are
`general`, `library`, `copies`, `people`, `security`, `linked`, `maintenance`
and `account`. Severity follows what was found:

- a health check: its own state (`warning` or `error`);
- a failed stage, media server or subtitle site: `error`; a stuck one: `warning`;
- a Source or Downloader not answering: `error`;
- the metadata server not answering, no backup, or no backup in the last 7
  days: `warning`;
- a newer HolyArrs: `info`;
- a saved Movies and TV default whose Library folder or Pick rules are gone:
  `warning`, on `/settings/title-defaults` in `library`.

Pages with no Library folders, Sources or Downloaders are listed under
`unset`. The answer is kept for 30 seconds for everyone. `metadata_up` says whether
the metadata server answers (`GET /api/v1/metadata/health` gives the same,
cached about a minute, to anyone signed in).

### Movies and TV defaults

`GET /api/v1/settings/title-defaults` (Run the Library) and `PUT` (Change
settings) hold what a new movie or show gets when an add leaves a field out:
`{"movie": {"library_folder_id", "pick_rules_id", "keep_up_to_date",
"search_on_add"}, "show": {...}}`. `0` means the first folder of that kind and
the folder's own Pick rules. Built-in values are Keep up to date on for both,
Search when added on for movies and off for shows.

`POST /titles` and `POST /shows` use a saved value only for a field the
request leaves out, and only once that value has been saved; with nothing
saved, adds behave as before. A saved folder or Pick rules that are removed
later are skipped (the add uses the built-in choice) and Settings flags them.
`POST /shows` also takes `search_on_add`: after the folder look, the whole
show is searched the same way **Search** on a show does. Outside lists and
approved asks keep their own settings.

### Pick rules editing

`POST /api/v1/pick-rules`, `GET`/`PUT`/`DELETE /api/v1/pick-rules/{id}` make,
read, change and remove Pick rules (reading needs Run the Library, the rest
Change settings). A change bumps `revision`. Ready-made rules keep their name
and kinds and cannot be removed; their rules may change. Rules in use (by a
Version, a Library folder, an Outside list, a Request setting or a Movies and
TV default; `in_use` counts each) cannot be removed, drop a kind in use, or
switch between video and music: those answer 409 with `in_use`. A wrong field
answers 400 `{"error", "field"}` where `field` is a dotted path such as
`body.good_enough_at`.

## Settings for Explore and trailers

| Where | Setting | Default |
| --- | --- | --- |
| My account, Alerts for you | **New episodes of shows I asked for** | On |
| Settings > This app | **Autoplay trailers** | On |

See [Explore](explore.md#new-episode-notices) and [Trailers](trailers.md#settings).

## Appearance

The Settings landing page offers **Dark** and **Light**. The choice is kept in
the current browser. Reduced motion follows the browser or operating-system
preference.

