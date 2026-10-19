# Parts, safe mode, controls and API keys

HolyArrs is one program, but it is built from **parts** you can turn on and off, and every piece of background work is watched on its own. This page says what each part does when it is off, how safe mode works, which controls the owner and admins have, how API keys and their scopes work, what the health states mean, and what happens when HolyArrs crashes or is restarted.

The part switches and the controls are on **Settings**, **Parts and controls** (owner and admins). Everything on this page is also available through the API (see [API keys and scopes](#api-keys-and-scopes)); the health states are on **Settings**, **This app** too.

## The first-run step: What do you use?

Right after the owner is made, HolyArrs asks once which parts to use. **Movies**, **Shows** and **Subtitles** are ticked, **Music** is not, and one question, "Will family or friends ask for titles?", turns **the Wishlist** on (Yes) or leaves it off (No, the default). Each part has one line saying what stops while it is off. **Continue** saves the choice; **Just movies** keeps only Movies on in one click. Change it any time in **Settings**, **Parts and controls**.

## Parts

There are five parts: **Movies**, **Shows**, **Music**, **Subtitles** and **the Wishlist**. All five are on in a new install until the first-run step (above) turns the unpicked ones off.

Everything else is the **core** and is always on: sign-in and accounts, Library folders, Sources, Downloaders and Downloads, Alerts, History, backups, health checks, settings and the API.

### What stops when a part is off

| Part | What stops | What does not stop |
|---|---|---|
| Movies | The movie pages and their menu entries, adding and searching movies, the Release gate check, movie refreshes. Movie routes answer "this part is off". | Downloads already in a Downloader are still followed; movie files and rows stay. |
| Shows | The show pages, adding and searching shows, season searches, show refreshes. Show routes answer "this part is off". | As for movies. |
| Music | The artist and album pages, Settings > Music, artist refreshes and album searches. | As for movies. |
| Subtitles | Settings > Subtitles, the missing-subtitles page, every subtitle search and check. | Subtitle files already placed stay beside their videos. |
| The Wishlist | The Wishlist page, asking for titles, the Wishlist ready check, messages about asks. | Asks already made stay and come back when it is on. |

When Movies, Shows and Music are all off, the shared Library pages (Library, Search, Calendar, Rename, Import) and the shared scheduled work (searching for missing items, feed checks, list syncs, rescans) stop too.

A few rules apply:

- **Nothing is deleted.** Turning a part off keeps every row and file. Turning it on again picks up where it was.
- **Its data stays readable through the core.** History (Events), the Jobs list, Downloads, Imports and the health states are core, so entries about a part that is off still show there (for example a finished download of a movie while Movies is off, or a subtitle Event).
- **Work in the queue waits.** A Job of a part that is off stays in the queue and runs once the part is on again; it is not lost and not run twice. A scheduled Job of a part that is off is not queued at all.
- **Some parts need others.** Subtitles need Movies or Shows. The Wishlist needs Movies, Shows or Music. HolyArrs refuses a switch that would break this and says which part to turn on or off first.
- **A member cannot ask for a title of a part that is off.**
- **It is not a memory saving.** Parts that are off share the same process, database connections and worker queue. The measured numbers are in the release report; see [Memory](#memory).

## Safe mode

Safe mode starts only the core: you can sign in, see the Library, change settings and make or restore a backup. The Library list and each title's page stay open, and a title can be edited or removed, although every part counts as off; searching, adding, refreshing, downloading and importing stay off (anything asked for waits in the queue for the next normal start). Every optional part and stage stays off and the health states say "Safe mode: not started". Use it when something keeps HolyArrs from starting or working, fix it from the screens, then restart normally.

Turn it on in one of three ways:

- the environment variable `HOLYARRS_SAFE_MODE=1` (for Docker, in the compose file),
- the command-line flag `-safe-mode`,
- the **restart in safe mode** control (below): the next start is in safe mode once, and the start after that is normal.

Every page and the sign-in page show a banner while safe mode is on.

## Controls

The owner and admins have these controls, on **Settings**, **Parts and controls**: each part with its state, what stops when it is off, **Turn on**, **Turn off** and **Restart**; each stage with **Pause**, **Resume** and **Restart**; each linked app with **Reconnect** (a Downloader) or **Test now** (a Source, media server or subtitle site); **Restart HolyArrs**, **Restart in safe mode** and **Shut down**; and the last controls used (their Events). A control with a confirm step shows its question on the page with **Yes** and **Cancel**; the question lasts 60 seconds. In safe mode the page says so at the top, and the switches the server refuses say why. Members do not. An API key needs the `control` scope for them. They need a password sign-in: the sign-in skip on the local network does not run them. Reconnecting a linked app also needs the settings permission, like testing it from its own page.

| Control | API | Confirm step |
|---|---|---|
| Turn a part on | `POST /api/v1/parts/{part}/on` | no |
| Turn a part off | `POST /api/v1/parts/{part}/off` | yes |
| Restart a part (its scheduled work stops and starts again) | `POST /api/v1/parts/{part}/restart` | yes |
| Pause, resume or restart one stage (backups, download sync, schedules, session clean-up, stranded imports, Release gates, Wishlist ready check) | `POST /api/v1/stages/{name}/pause`, `/resume`, `/restart` | no |
| Reconnect one linked app (a Downloader, a Source, a media server or a subtitle site) | `POST /api/v1/links/{kind}/{id}/restart` | no |
| Restart the whole app | `POST /api/v1/app/restart` | yes |
| Restart in safe mode | `POST /api/v1/app/safe-mode` | yes |
| Shut down | `POST /api/v1/app/shutdown` | yes |

**The confirm step:** send `{}` first. HolyArrs answers 202 with a confirm id. Send `{"confirm": "<id>"}` within 60 seconds, as the same person, to the same control. Every control writes an Event to History with who did it.

**Restart and shut down use Docker.** HolyArrs stops cleanly and the process ends; Docker's restart policy starts it again. With the policy `unless-stopped` or `always` that happens for restart, restart in safe mode and shut down. With `on-failure`, restart and restart in safe mode still come back (they end with a non-zero code) and shut down stays down. With `no`, nothing starts it again: start the container yourself. Sign-in, the database and the API are shared by everything and only restart with the whole app.

A stage that is stuck (one pass that never returns) cannot be stopped from inside the program. It shows as stuck, is never started a second time, and clears with a whole-app restart.

## Health states

**Settings**, **This app** lists every stage, every part and every linked app with one of five states and a reason in plain words. The same list is `GET /api/v1/health/states`.

| State | Means |
|---|---|
| off | Turned off by you, paused, or not started in safe mode. |
| starting | Starting, or not checked yet. |
| running | Working. |
| retrying | It failed or did not answer, and HolyArrs tries again on its own, waiting longer each time (1 s, doubling, at most 5 minutes for stages). |
| failed | It crashed, is stuck, or needs you (a refused sign-in, a broken answer). The reason says what to do. |

Reasons are fixed plain words. Addresses, paths and raw error text go only to the log, never to the health states (a `read` key can see those). A pass that runs past its time limit gets a short grace to stop on its own before it is shown as stuck.

A crash in one stage is caught: the stage shows failed, then retrying, and starts again by itself. Sign-in, the Library and backups keep working while it does; the fault runs in the release report check that on purpose.

## Restart after a crash

If HolyArrs itself crashes, Docker's restart policy starts it again. Use `restart: unless-stopped` in your compose file. The measured time from a crash to a working sign-in page is in the release report (see below). A crash in the shared core (sign-in, the database, the web server) stops everything until that restart; that is the honest limit of a single program.

## Memory

The Docker image sets `GOMEMLIMIT`, the point where Go starts working harder to stay small, just under a 500 MB container limit. The SQLite database keeps its page cache outside what `GOMEMLIMIT` sees, so the limit leaves room for it. If you give the container a different memory limit, set `GOMEMLIMIT` to about 80 percent of it.

## API keys and scopes

Make a key in **My account** (or `POST /api/v1/auth/keys`), send it in the `X-Api-Key` header. Each key holds one or more scopes:

| Scope | Reaches |
|---|---|
| `read` | The GET routes marked read: looking at the Library, Jobs, Downloads, History, health and settings (secrets in them are masked). |
| `write` | Every route that changes something, except the admin ones and the controls. |
| `admin` | People, invites, ask limits, API keys, password and two-factor sign-in, sign-in settings, backups (list, download, restore), the Trakt and TVDB key settings, the calendar feed key, and any GET route not marked read. |
| `control` | The controls above. |
| `all` | `read`, `write` and `admin` together. Keys made before scopes existed hold this one and keep working. It does not include `control`. |

A key reaches a route only when it holds the route's scope, and it can never do more than the person who made it may do. A key cannot make a key with a scope it does not hold, a key's scopes never change after it is made (make a new one instead), and only the owner and admins may give the `control` scope. `admin` does not include `control`: inviting or promoting an admin with a key also needs the `control` scope, because the new admin's sign-in holds every scope.

The API description at `/api/v1/openapi.json` lists every route with its permission (`x-holyarrs-permission`), scope (`x-holyarrs-scope`) and part (`x-holyarrs-part`). It is generated from the code, so it always matches what the server does.

### The API promise

Once 1.0 is out, a route outside apps use stays at least **12 months** after it is marked deprecated, and a screen or setting stays at least **6 months**. A deprecated route says so in the API description, in its answers and in the release notes, with what to use instead. Adding routes, optional fields and answer fields can happen in any release, so tools should ignore fields they do not know. The full promise is in `design/system/api-promise.md` in the HolyArrs design notes.

## Release reports

Each measured run (the fault runs, the memory runs with all parts on next to Movies and Shows only, and the crash-to-sign-in time) is written to the release report for that stage, and the automated checks run them again on every change.
