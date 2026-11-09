---
title: Get support
description: What to collect, what to hide, and where to ask for help.
level: Start
sidebar: { order: 3 }
---

Collect the right details once, hide what is private, and ask in the right place.

## Before you ask

1. Read [Troubleshooting](../help/troubleshooting/) for your symptom.
2. Run the doctor: `docker exec holyarrs /holyarrs doctor`. Fix any `FAIL` line it explains.

## Collect a support bundle

A support bundle is one zip with what a helper asks for first. Owners and admins can download it.

1. Open the `/health` page.
2. Click **Download support bundle**. You should get a `.zip` file.

| File | Holds |
|---|---|
| `log.txt` | The newest log, with keys, tokens and passwords removed. |
| `about.json` | Version and setup details. |
| `doctor.txt` | The doctor report. |
| `health.json` | Current health check results. |

If you can't open the web page, send the doctor output and the last 100 log lines instead: `docker logs --tail 100 holyarrs`.

## Hide what is private

Secrets are removed, but the bundle can still show folder paths and host names. Before you post anything:

- Replace host names, domains and public IP addresses with `example`.
- Remove folder names you don't want public, such as your name in a path.
- Never post `holyarrs.key`, an API key, a password or a Compose file with secrets in it.

## Where to ask

| Place | Use it for |
|---|---|
| [Discord](https://discord.com/invite/mthsvesKpC) | Questions and setup help. |
| [GitHub Issues](https://github.com/HolyArrs/HolyArrs/issues) | Bugs you can repeat. |
| [r/HolyArrs](https://www.reddit.com/r/HolyArrs/) | Ideas. |

In your message, say what you did, what you expected and what happened. Attach the bundle, or paste the doctor output.

## For experts

<details class="ha-more"><summary>Get the bundle from the API</summary>

`GET /api/v1/support/bundle` with an owner or admin key holding the `admin` scope, sent in `X-Api-Key`, returns the same zip.

</details>

## Related

- [Troubleshooting](../help/troubleshooting/)
- [Logs](../run/logs/)
- [FAQ](../help/faq/)
