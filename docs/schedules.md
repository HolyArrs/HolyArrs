# Schedules

Settings > Schedules lists everything HolyArrs does on a timer.

## Scheduled work

One row per scheduled Job: refresh of movie, show and artist details, the Library folder check, Outside lists, the feed check, Held releases, the search for missing titles, the Library rescan, the holding folder clean-up, the health checks and the two subtitle searches. Each row shows:

- what it does, in one line,
- how often it runs, when it last ran (and whether that run failed, with the reason) and when it runs next,
- its state: Waiting, Queued, Running, Turned off (its own setting is off, such as the daily search with "missing search count" 0), Part is off (Settings > Parts and controls) or Safe mode,
- "Its settings" when another settings page changes it too (Search, This app, Subtitles).

**Run now** (people who run the Library) queues the Job at once. It does not move the timer: the next run stays when it was. When the Job is already waiting or running, HolyArrs says so instead of starting a second one. Run now is refused while the Job's part is off, and in safe mode for anything outside the core. A schedule that is turned off runs by hand only for people who may change settings (others get "this schedule is turned off"). Error texts and stage reasons on the page are masked like the log.

**Change** (people who may change settings) sets how often it runs, within its bounds; the next run moves to now plus the new interval.

People who may change settings can turn two schedules off: **Refresh movie details** and **Rescan the Library** (useful when NAS disks should sleep; files you add by hand are then found only with Run now). Turning one off does not stop any other scheduled work, and off is kept across restarts.

For **Refresh movie details**, any queued automatic movie-details run that has not started is cancelled; if one provider request is already in flight, that request may finish before the run stops. Movie names, dates and artwork can become stale while it is off. **Run now** and the Library's manual refresh still work with their usual permissions and provider pacing.

Turning automatic movie refresh on plans one run for the current time plus its interval. It does not replay missed runs.

| Job | Default | Allowed |
|---|---|---|
| Feed check | 15 minutes | 10 minutes to 2 hours (same as Settings > Search) |
| Try Held releases again | 1 minute | 1 minute to 1 hour |
| Read Outside lists | 15 minutes | 15 minutes to 1 day, 5-minute steps |
| Health checks | 5 minutes | 1 minute to 1 day (same as This app) |
| Check Library folders | 6 hours | 1 hour to 1 day |
| Look for missing subtitles | 6 hours | 1 hour to 7 days (same as Settings > Subtitles) |
| Look for better subtitles | 12 hours | 6 hours to 7 days |
| Refresh movie, show and artist details | 1 day or 12 hours | 6 hours to 7 days |
| Search for missing, Rescan the Library, Clean the holding folder | 1 day | 6 hours to 7 days |

## Background loops

The work that keeps running on its own (Download sync, Schedules, Backups, Release gates, Session clean-up, Log masking and the rest) with its state, how often it runs, its last and next pass, and why it is retrying when it is. Pause, resume or restart one in Settings > Parts and controls.

## API

| Route | Who | What |
|---|---|---|
| `GET /api/v1/schedules` | run the Library (read scope) | `schedules` and `loops` as above |
| `POST /api/v1/schedules/{kind}/run` | run the Library (write scope) | queue it now; `202 {"job_id": n, "already": false}`; 409 when its part is off or in safe mode |
| `PUT /api/v1/schedules/{kind}` | settings (write scope) | `{"every_s": 1800}` for any listed schedule; `{"enabled": false}` or `{"enabled": true}` for `refresh_titles` and `rescan_library`; fields may be combined; 400 outside interval bounds |
