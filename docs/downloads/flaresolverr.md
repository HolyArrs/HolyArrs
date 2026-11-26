---
title: Get past bot checks with FlareSolverr
description: Use the FlareSolverr plugin so a Source behind a Cloudflare or DDoS-Guard bot check works again.
level: Setup
sidebar: { order: 6 }
---

Some sites put a bot check, such as Cloudflare's "Just a moment...", in front of their feed. HolyArrs runs no browser, so such a Source shows **Blocked**. The FlareSolverr plugin hands that page to [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr), which you run next to HolyArrs. FlareSolverr opens it in a real browser and waits out the check. HolyArrs then keeps the cookie it got for that site and calls the site straight again, until the site checks again.

## Before you start

- Only the owner sets this up.
- You need HolyArrs 1.3.2 or newer. The plugin installs from the Plugins page like any other, signed by HolyArrs. Plugins run only on Linux, which includes the Docker image. See [Plugins](../run/plugins.md).
- You need FlareSolverr running where HolyArrs can reach it by name, such as a container named `flaresolverr` on the same Docker network. An address like `192.168.1.5` can't be used; use a name.

```yaml
services:
  flaresolverr:
    image: ghcr.io/flaresolverr/flaresolverr:latest
    container_name: flaresolverr
    restart: unless-stopped
```

Put it on the same Docker network as HolyArrs. It listens on port `8191`.

## Set it up

1. Open **System > Plugins** and select **Install** on the **FlareSolverr** card. It asks for the bot-check action and shows "Sees everything sent through it": it is handed each page it opens, the Source's key included.
2. Open the plugin. Under **Your choices**, type `flaresolverr:8191` in **Also allow your own server**, and select **Save**. The plugin reaches nothing else.
3. Select **Settings** and set **FlareSolverr address** to `http://flaresolverr:8191`. That is the default, so leave it if your container has that name. Select **Save**.
4. Open **Settings > Sources**. On each Source with a key, type the key (and the proxy password, if it has one) again and save. HolyArrs hands a key only to the bot-check plugin it was typed for.
5. On a **Blocked** Source, select **Try now**. You should see **Working**.

## What it does and does not do

- Only a plain feed call that meets a bot check goes to FlareSolverr. Every other call goes straight to the site, with the cookie FlareSolverr got.
- When the Source has a proxy, FlareSolverr is told to use it too, so the site sees one address.
- The page must be on the Source's own site, and the Source's **Reach the local network** setting still holds. HolyArrs checks this before and after FlareSolverr opens the page.
- FlareSolverr gets 60 seconds per page.
- The plugin never writes a page's address, its key or a cookie in the log.

## If it does not work

The reason shows on the Source and in the [log](../run/logs.md), on lines starting with `plugin flaresolverr:`.

| Reason | Fix |
|---|---|
| The FlareSolverr address is not set | Set **FlareSolverr address** in the plugin's **Settings** |
| The FlareSolverr address must look like http://flaresolverr:8191 | Type the address as `http://name:port`, with no path other than `/v1` |
| FlareSolverr did not answer | Check FlareSolverr runs, the address and port are right, and the same name and port are under **Also allow your own server**. After a miss, the plugin waits up to 2 minutes before it asks FlareSolverr again |
| FlareSolverr took too long to answer | FlareSolverr is busy or the check is hard. Select **Try now** later |
| FlareSolverr could not pass the bot check | FlareSolverr's own reason follows. Update FlareSolverr; some checks it can't pass |
| FlareSolverr's browser was still on the bot check page | Same as above |
| Something that is not FlareSolverr answered | The address leads to another program. Check the port |
| The bot-check plugin changed since this Source's key was typed | Type the Source's key again and save |
| HolyArrs refused the plugin (in the log) | Open the plugin on the Plugins page and approve what it asks for |

## Related

- [Sources](sources.md)
- [Plugins](../run/plugins.md)
