# Parts, safe mode and controls

You want to turn off the parts of HolyArrs you don't use, start it in safe mode when something breaks, or restart it from the browser. This page covers all three, plus the health states and API key scopes.

## Pick your parts

HolyArrs has five parts you can turn on and off: **Movies**, **TV shows**, **Music**, **Subtitles** and **Requests**.

Right after you make the owner account, HolyArrs asks "What do you use?". Tick the parts you want. The question "Will family or friends ask for titles?" turns Requests on or leaves it off. You can change all of this later.

To change it:

1. Open **Settings > Parts**.
2. Click **Turn on** or **Turn off** next to a part.

Everything else is always on: sign-in and accounts, library folders, sources, downloaders and downloads, alerts, history, backups, health checks, settings and the API.

### What happens when a part is off

| Part | What stops | What keeps going |
|---|---|---|
| Movies | Movie pages and menu items, adding, searching and refreshing movies. | Downloads already in your downloader are still followed. Your movie files stay. |
| TV shows | Show pages, adding and searching shows, season searches, show refreshes. | Same as Movies. |
| Music | Artist and album pages, music settings, artist refreshes, album searches. | Same as Movies. |
| Subtitles | Subtitle settings, the missing subtitles page, subtitle searches and checks. | Subtitle files already beside your videos stay. |
| Requests | The requests pages and asking for titles. | Requests already made stay and come back when you turn it on. |

A few rules:

- **Nothing is deleted.** Turn a part on again and it picks up where it left off.
- **Queued work waits.** A job for a part that is off stays in the queue and runs once the part is on again.
- **Some parts need others.** Subtitles need Movies or TV shows. Requests needs Movies, TV shows or Music. HolyArrs refuses a change that breaks this and tells you what to turn on or off first.
- **It doesn't save memory.** Parts that are off still run in the same program.

## Start in safe mode

Use safe mode when something keeps HolyArrs from starting or working. It starts only the basics. You can sign in, look at your library, change settings and make or restore a backup. Searching, adding, refreshing, downloading and importing stay off. Anything you ask for waits until the next normal start.

Turn it on in one of three ways:

- set `HOLYARRS_SAFE_MODE=1` in your Compose file,
- start the program with `-safe-mode`,
- click **Restart in safe mode** in **Settings > Danger zone**. Only the next start is in safe mode. The one after is normal.

A banner shows on every page while safe mode is on. Fix the problem, then restart normally.

## Restart, pause and shut down

The owner and admins find these in **Settings > Danger zone**:

- **Restart HolyArrs**, **Restart in safe mode** and **Shut down**.
- **Pause**, **Resume** and **Restart** for each background task, such as backups or download sync.

Controls that stop something ask you to confirm first. You have 60 seconds to say yes. Every control you use is written to History with your name.

You need a password sign-in to use the controls. Signing in without a password on your local network is not enough.

### Restart needs Docker's restart policy

HolyArrs stops cleanly and Docker starts it again. Use `restart: unless-stopped` in your Compose file.

| Restart policy | Restart | Shut down |
|---|---|---|
| `unless-stopped` or `always` | Comes back. | Comes back too. |
| `on-failure` | Comes back. | Stays down. |
| `no` | Stays down. Start it yourself. | Stays down. |

If HolyArrs crashes, the same restart policy brings it back.

## Health states

The `/health` page and **Settings > Parts** list each background task, part and linked app with a state and a short reason. The same list is at `GET /api/v1/health/states`.

| State | Meaning |
|---|---|
| off | You turned it off or paused it, or safe mode didn't start it. |
| starting | It is starting or hasn't been checked yet. |
| running | It works. |
| retrying | It failed and HolyArrs tries again by itself, waiting longer each time. |
| failed | It crashed, got stuck or needs you. The reason says what to do. |

When one background task crashes, it shows failed, then retrying, and starts again. Sign-in, your library and backups keep working meanwhile. A task that is stuck clears when you restart the whole app.

## Memory

HolyArrs aims to stay under 384 MiB, which fits a 500 MB container limit. To set your own target, set `GOMEMLIMIT` to about 80 percent of the memory you give the container.

### Low-memory mode

Low-memory mode is for a small NAS or a container with little memory. HolyArrs keeps less in memory and reads more from its database, so some pages take a moment longer. Background work like the music catalog copy runs less often.

- It turns on by itself when the machine or container has about 2.5 GiB or less (Linux).
- `HOLYARRS_LOW_MEMORY=1` turns it on, `HOLYARRS_LOW_MEMORY=0` turns it off.
- The first log line says whether it is on and why.

## API keys and scopes

Make a key in **My account** and send it in the `X-Api-Key` header. Each key holds one or more scopes:

| Scope | What it reaches |
|---|---|
| `read` | Looking: library, jobs, downloads, history, health and settings. Secrets are masked. |
| `health_read` | Only the health states. Good for a monitoring tool. |
| `write` | Every change, except admin routes and the controls. |
| `admin` | People, invites, request limits, API keys, password and two-factor, sign-in settings and backups. |
| `control` | The controls on this page. |
| `all` | `read`, `write` and `admin` together. It does not include `control`. |

A key can never do more than the person who made it. You can't change a key's scopes later, so make a new key instead. Only the owner and admins can give the `control` scope.

Every route, with its scope, is listed at `/api/v1/openapi.json`.

## Troubleshooting

**HolyArrs says it can't turn a part off.** Another part needs it. Read the message, turn that part off first, then try again.

**Restart shut HolyArrs down and it didn't come back.** Your restart policy is `no`. Start the container yourself and set `restart: unless-stopped`.

**The controls are greyed out or refused.** You signed in without a password on the local network, or your API key lacks the `control` scope. Sign in with your password.

**A task shows failed and stays failed.** Read its reason on the `/health` page. Fix what it says, then click **Restart** for that task, or restart the whole app.
