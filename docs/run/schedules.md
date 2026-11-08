---
title: Schedules
description: See what HolyArrs runs on a timer, run a job now, and change how often each job runs.
level: Setup
sidebar: { order: 5 }
---

Every timed job lives in **Settings > Schedules**.

## Run a job now

1. Find the job's row.
2. Select **Run now**. You should see its state change to **Queued**.

The next timed run does not move. A job already waiting or running is not started twice; HolyArrs tells you.

## Change how often a job runs

1. Select **Change** on the row. It needs the **Change settings** permission.
2. Pick an interval inside the allowed range and save. You should see the next run move to now plus the new interval.

## What a row shows

- What the job does, in one line.
- How often it runs, its last run and any failure reason, and its next run.
- Its state: Waiting, Queued, Running, Turned off, Part is off or Safe mode.
- A link to another settings page that also sets it.

## Choices

### Default ranges

| Job | Allowed range | Also set on |
|---|---|---|
| Feed check | 10 min to 2 h | Settings > Search |
| Try Held releases again | 1 min to 1 h | |
| Read Plex watchlists | 3 min to 1 h | Settings > Request rules |
| Read Outside lists | 15 min to 1 day, 5-min steps | |
| Health checks | 1 min to 1 day | Settings > This app |
| Check Library folders | 1 h to 1 day | |
| Find artist pictures | 1 h to 7 days | |
| Look for missing subtitles | 1 h to 7 days | Settings > Subtitles |
| Look for better subtitles | 6 h to 7 days | Settings > Subtitles |
| Check Jackett | 6 h to 7 days | Settings > Sources |
| Refresh movie details | 6 h to 7 days | |
| Refresh show details | 6 h to 7 days | |
| Refresh artist details | 6 h to 7 days | |
| Search for missing | 6 h to 7 days | Settings > Search |
| Rescan the Library | 6 h to 7 days | |
| Clean the holding folder | 6 h to 7 days | |
| Clean up History | 6 h to 7 days | Settings > This app |

### Turn a schedule off

Two schedules can be turned off: **Refresh movie details** and **Rescan the Library**, for example to let NAS disks sleep. Off survives restarts and leaves other jobs running.

| Off | Cost |
| --- | --- |
| Rescan the Library | Files you add by hand show up only after **Run now** |
| Refresh movie details | Queued runs that have not started are cancelled. Names, dates and artwork can go stale. Manual refresh still works |

Turning one back on plans a run at now plus its interval. Missed runs are not replayed.

### Background loops

Below the jobs sit loops that run on their own, such as Download sync, Backups and Log masking. Each shows its state, interval, last and next pass, and why it retries. Pause, resume or restart a loop in **Settings > Parts**.

## If it does not work

**Symptom:** **Run now** says the part is off.
**Cause:** the job's part is off in **Settings > Parts**, or HolyArrs is in safe mode and the job is outside the core.
**Fix:** turn the part on, or leave safe mode. See [Parts and safe mode](parts-safe-mode.md).

**Symptom:** **Run now** is missing on a turned-off schedule.
**Cause:** only people with **Change settings** run a turned-off schedule by hand.
**Fix:** ask someone who has it.

**Symptom:** a job failed.
**Cause:** shown on the row's last run, masked like the log.
**Fix:** act on that reason, then select **Run now**.

## For experts

<details class="ha-more"><summary>API</summary>

| Route | Who | What |
|---|---|---|
| `GET /api/v1/schedules` | **Run the Library**, read scope | Schedules and loops |
| `POST /api/v1/schedules/{kind}/run` | **Run the Library**, write scope | Queue now. 202, or 409 when its part is off or in safe mode |
| `PUT /api/v1/schedules/{kind}` | **Change settings**, write scope | `{"every_s": 1800}` for any schedule; `{"enabled": false}` only for `refresh_titles` and `rescan_library`. 400 outside the range |

</details>

## Related

- [Parts and safe mode](parts-safe-mode.md)
- [Health checks](health.md)
- [Logs and storage](logs.md)
