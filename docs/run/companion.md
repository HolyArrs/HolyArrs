---
title: Health companion
description: Check HolyArrs health from another container with a key that can read health states and nothing else.
level: Setup
sidebar: { order: 10 }
---

Let an outside tool check whether HolyArrs is healthy, without giving it access to anything else. The companion reads the health states once, prints how many parts are off, starting, running, retrying or failed, and exits.

## Run the companion

1. Build it from `app/backend/cmd/holyarrs-companion` in the source.
2. As the owner, open **Settings > API > API keys** and make an API key with **health_read** as its only scope.
3. Save the key, shown once, in a private file outside the source folder.
4. Mount that file read-only at `/run/secrets/holyarrs-key` in the companion's container.
5. Set `HOLYARRS_URL` to your HolyArrs address, such as `http://your-server:6262`. Add your URL base if you use one.
6. Run the companion. You should see 5 counts printed.

:::caution
Never pass the key as an argument, or put it in an image, an address, a log, a Compose file or source control.
:::

## If it does not work

The companion stops with a short fixed message.

| Message | Cause | Fix |
|---|---|---|
| "mount the companion key at /run/secrets/holyarrs-key" | Key file missing | Mount it at that path. |
| "set HOLYARRS_URL to the app address, without credentials or a query" | Address empty or wrong | Set a plain address. |
| "the app refused the companion health read" | Key wrong, revoked or without the scope | Make a new **health_read** key. |
| "the app did not answer within the deadline" | HolyArrs unreachable | Check the address and that HolyArrs runs. |

## For experts

<details class="ha-more"><summary>What the key and companion can do</summary>

- A **health_read** key reads health states only. It can't see titles, settings or other keys, can't change anything and can't widen its own scope. Do not give the companion a broader key.
- The companion follows no redirects, waits 10 s at most and reads 1 MiB at most.
- It prints counts only. Labels and raw error text stay out.
- Use HTTPS on any network you don't trust.
- When you are done, revoke the key and delete its file. For ongoing use, give it its own key and swap it through the same mount.

</details>

## Related

- [Health checks](../run/health/)
- [Parts and safe mode](../run/parts-safe-mode/)
