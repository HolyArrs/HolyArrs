---
title: Parts and safe mode
description: Turn parts of HolyArrs on or off, start in safe mode, restart from the browser, and read health states.
level: Setup
sidebar: { order: 7 }
---

Turn off the parts you don't use, start in safe mode when something breaks, and restart HolyArrs from the browser.

## Turn a part on or off

HolyArrs has 5 parts: **Movies**, **TV shows**, **Music**, **Subtitles** and **Requests**. You first pick them in the first-run wizard.

1. Open **Settings > Parts**.
2. Click **Turn on** or **Turn off** next to a part. You should see its menu items appear or go.

Nothing is deleted. Turn a part on again and it picks up where it left off.

## Start in safe mode

Safe mode starts only the basics, for when something keeps HolyArrs from starting or working. You can sign in, look at your library, change settings and make or restore a backup. Searching, adding, refreshing, downloading and importing wait for the next normal start.

1. Open **Settings > Danger zone**.
2. Click **Restart in safe mode**. You should see a banner on every page.
3. Fix the problem, then click **Restart HolyArrs**. The next start is normal.

If you can't reach the page, set `HOLYARRS_SAFE_MODE=1` in Compose or start with `-safe-mode`. Remove it after the fix: both keep every start in safe mode.

## Choices

### What stops when a part is off

| Part | Stops | Keeps going |
|---|---|---|
| Movies | Movie pages and menu items, adding, searching, refreshing | Downloads already in your downloader are still followed. Files stay. |
| TV shows | Show pages, adding, searching, season searches, refreshes | Same as Movies. |
| Music | Artist and album pages, music settings, artist refreshes, album searches | Same as Movies. |
| Subtitles | Subtitle settings, the missing subtitles page, searches and checks | Subtitle files beside your videos stay. |
| Requests | Request pages and asking for titles | Existing requests stay and return when it is back on. |

Always on: sign-in and accounts, library folders, sources, downloaders and downloads, alerts, history, backups, health checks, settings and the API.

- Subtitles need Movies or TV shows. Requests need Movies, TV shows or Music. HolyArrs refuses a change that breaks this and says what to change first.
- A queued job for a part that is off waits and runs once the part is on.
- Turning a part off does not save memory. Everything runs in one program.

### Restart, pause and shut down

**Settings > Danger zone** (owner and admins) has **Restart HolyArrs**, **Restart in safe mode** and **Shut down**, plus **Pause**, **Resume** and **Restart** for each background task, such as backups or download sync.

- Controls that stop something ask you to confirm within 60 s.
- Every control used is written to History with your name.
- You need a password sign-in. A no-password sign-in on your local network is refused.

Restart stops HolyArrs cleanly and lets Docker start it again, so set `restart: unless-stopped`.

| Restart policy | Restart | Shut down |
|---|---|---|
| `unless-stopped` or `always` | Comes back | Comes back too |
| `on-failure` | Comes back | Stays down |
| `no` | Stays down | Stays down |

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| HolyArrs can't turn a part off | Another part needs it | Turn that part off first, as the message says. |
| Restart shut HolyArrs down for good | Restart policy is `no` | Start the container and set `restart: unless-stopped`. |
| Controls are refused | No-password local sign-in, or the key lacks `control` | Sign in with your password. |
| A task stays failed | It needs you | Read its reason on `/health`, fix it, click **Restart** for that task. |

More fixes: [Troubleshooting](../../help/troubleshooting/).

## For experts

<details class="ha-more"><summary>Health states</summary>

`/health`, **Settings > Parts** and `GET /api/v1/health/states` list each background task, part and linked app with a state and a reason.

| State | Meaning |
|---|---|
| off | Turned off, paused, or not started in safe mode. |
| starting | Starting, or not checked yet. |
| running | Works. |
| retrying | Failed; HolyArrs tries again, waiting longer each time. |
| failed | Crashed, stuck or needs you. The reason says what to do. |

A crashed task shows failed, then retrying, then starts again. Sign-in, the library and backups keep working meanwhile. A stuck task clears on a full restart.

</details>

<details class="ha-more"><summary id="low-memory-mode">Memory and low-memory mode</summary>

HolyArrs aims to stay under 384 MiB, which fits a 500 MB container limit. To set your own target, set `GOMEMLIMIT` to about 80 percent of the container's memory.

Low-memory mode keeps less in memory and reads more from the database. Some pages take a moment longer, and background work such as the music catalog copy runs less often.

| Setting | Effect |
|---|---|
| (default, Linux) | Turns on by itself at about 2.5 GiB of memory or less. |
| `HOLYARRS_LOW_MEMORY=1` | Always on. |
| `HOLYARRS_LOW_MEMORY=0` | Always off. |

The first log line says whether it is on and why.

</details>

<details class="ha-more"><summary>API key scopes</summary>

Make a key in **Settings > API > API keys** and send it in the `X-Api-Key` header.

| Scope | Reaches |
|---|---|
| `read` | Library, jobs, downloads, history, health, settings. Secrets masked. |
| `health_read` | Health states only. For monitoring tools. |
| `write` | Every change except admin routes and controls. |
| `admin` | People, invites, request limits, API keys, password and two-factor, sign-in settings, backups. |
| `control` | The controls on this page. Only the owner and admins can grant it. |
| `all` | `read`, `write` and `admin`. Not `control`. |

A key never does more than the person who made it. Scopes can't be changed later; make a new key. Every route and its scope: `/api/v1/openapi.json`.

</details>

## Related

- [Health checks](../../run/health/)
- [Backups](../../run/backups/)
- [API routes](../../reference/api-routes/)
