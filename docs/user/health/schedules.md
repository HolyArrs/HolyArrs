# Schedules

You want to see what HolyArrs does on a timer, run something now, or change how often it runs. Open **Settings > Schedules**.

## Run a job now

1. Find the row.
2. Click **Run now**.

The job is queued at once. The next timed run does not move. If the job is already waiting or running, HolyArrs tells you and does not start a second one.

Run now is refused while the job's part is off in **Settings > Parts**, and in safe mode for anything outside the core. If a schedule is turned off, only people who may change settings can run it by hand.

## Change how often it runs

1. Click **Change** on the row. You need permission to change settings.
2. Pick a new interval within the allowed range.

The next run moves to now plus the new interval.

## What each row shows

- What the job does, in one line.
- How often it runs, when it last ran, whether that failed and why, and when it runs next.
- Its state: Waiting, Queued, Running, Turned off, Part is off, or Safe mode.
- A link to another settings page when that page also changes it.

## Jobs and their ranges

| Job | Allowed range | Also set on |
|---|---|---|
| Feed check | 10 minutes to 2 hours | Settings > Search |
| Try Held releases again | 1 minute to 1 hour | |
| Read Plex watchlists | 3 minutes to 1 hour | Settings > Request rules |
| Read Outside lists | 15 minutes to 1 day, in 5-minute steps | |
| Health checks | 1 minute to 1 day | Settings > This app |
| Check Library folders | 1 hour to 1 day | |
| Find artist pictures | 1 hour to 7 days | |
| Look for missing subtitles | 1 hour to 7 days | Settings > Subtitles |
| Look for better subtitles | 6 hours to 7 days | Settings > Subtitles |
| Check Jackett | 6 hours to 7 days | Settings > Sources |
| Refresh movie, show and artist details | 6 hours to 7 days | |
| Search for missing | 6 hours to 7 days | Settings > Search |
| Rescan the Library | 6 hours to 7 days | |
| Clean the holding folder | 6 hours to 7 days | |
| Clean up History | 6 hours to 7 days | Settings > This app |

## Turn a schedule off

Two schedules can be turned off: **Refresh movie details** and **Rescan the Library**. This helps when NAS disks should sleep. Off is kept across restarts and does not stop other jobs.

- With **Rescan the Library** off, files you add by hand are found only with Run now.
- With **Refresh movie details** off, queued runs that have not started are cancelled. Names, dates and artwork can go stale. Manual refresh still works.

Turning a schedule back on plans one run at now plus its interval. Missed runs are not replayed.

## Background loops

Below the jobs you see the work that runs on its own, such as Download sync, Backups and Log masking. Each shows its state, how often it runs, its last and next pass, and why it is retrying. Pause, resume or restart a loop in **Settings > Parts**.

## API

| Route | Who | What |
|---|---|---|
| `GET /api/v1/schedules` | People who run the Library (read scope) | Schedules and loops |
| `POST /api/v1/schedules/{kind}/run` | People who run the Library (write scope) | Queue it now. Answers 202, or 409 when its part is off or in safe mode |
| `PUT /api/v1/schedules/{kind}` | People who may change settings (write scope) | `{"every_s": 1800}` for any schedule. `{"enabled": false}` only for `refresh_titles` and `rescan_library`. Answers 400 outside the range |

## Troubleshooting

**Run now says the part is off.** Turn the part on in **Settings > Parts**, then try again.

**A job failed.** Check the row's last run for the reason. Reason text is masked like the log.
