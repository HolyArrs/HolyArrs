---
title: Remote access
description: Reach HolyArrs from your phone, TV or laptop anywhere with the Tailscale plugin.
level: Setup
sidebar: { order: 13 }
---

Open HolyArrs away from home on your own devices, without opening a port on your router. It works through [Tailscale](https://tailscale.com), a private network between your devices.

## Before you start

- You need a Tailscale account, or your own Headscale server (see [Headscale](#headscale)).
- Only the owner sets this up.
- Every phone, TV or laptop that should reach HolyArrs needs the Tailscale app, signed in to the same account.
- Plugins run only on Linux, which includes the Docker image.

## Set it up

1. Open **System > Plugins** and select **Install** on the **Tailscale** card. See [Plugins](plugins.md).
2. Select **Open** on the card, or open **Settings > Remote access**.
3. Select **Connect to Tailscale**. While HolyArrs fetches your link you see **Getting your sign-in link…**.
4. Tailscale's sign-in opens in a new tab. Sign in and add the device. On a phone, you can scan the QR code under **Or scan this with your phone** instead.
5. Back in HolyArrs you should see **Connected** and the **Address** HolyArrs has on your tailnet.
6. On your phone or TV, open the Tailscale app, then open that address in a browser.

Every visit through Tailscale asks you to sign in to HolyArrs, even from a computer that skips sign-in at home.

## What each state means

| State | What to do |
|---|---|
| Starting… | Wait a few seconds. |
| Not connected yet | Select **Connect to Tailscale**. |
| Waiting for sign-in | Select **Connect to Tailscale** or scan the QR code. |
| Waiting for your tailnet admin to approve this device | Approve the device in Tailscale's admin page, or ask whoever runs your tailnet. |
| Connected | Nothing. |
| Connected · login expires within 14 days | Sign in again before the date under **Login expires**, or turn off key expiry for this device in Tailscale's admin page. |
| Login expired. Sign in again. | Select **Connect to Tailscale** and sign in. |
| Removed in Tailscale. Sign in again. | Someone removed the device in Tailscale. Select **Connect to Tailscale** to add it back. |
| Disconnected (still signed in) | Select **Reconnect**. |

## Buttons

- **Connect to Tailscale** opens the sign-in, or starts Tailscale again if it's stopped.
- **Disconnect** (when connected) leaves the tailnet for now but keeps the login, so **Reconnect** needs no new sign-in.
- **Sign out** (when connected) leaves the tailnet and wipes HolyArrs' Tailscale login. If HolyArrs says "Signed out here. Remove the device in Tailscale's admin page too.", open that page and remove the HolyArrs device.
- **Change address** (when connected) sets the first part of the address. The rest is your tailnet's name. HolyArrs reconnects to apply it.

If HTTPS certificates are off for your tailnet, HolyArrs is reached over plain `http://` on the tailnet instead. Turn on HTTPS in Tailscale's admin page to get `https://`.

## Relayed connections

**Connection** shows **Relayed**: HolyArrs blocks UDP for every plugin, so Tailscale always goes through Tailscale's encrypted relays over HTTPS. It works from anywhere but is slower than a direct connection. Opening UDP ports changes nothing.

## Headscale

To use your own Headscale server, set its address on HolyArrs and restart:

```yaml
environment:
  - HOLYARRS_TAILSCALE_LOGIN_SERVER=https://headscale.example.com
```

Then add the server's name on the Tailscale plugin under **Plugins** > **Also allow your own server**, for example `headscale.example.com` (or `headscale.example.com:8080` for another port), and select **Save**. The plugin reaches only Tailscale's servers and the ones you add. Then follow [Set it up](#set-it-up). The sign-in link opens your Headscale server.

You can also change the device name with `HOLYARRS_TAILSCALE_HOSTNAME`. The default is `holyarrs`.

## Troubleshooting

| Problem | Fix |
|---|---|
| "Remote access uses the Tailscale plugin." | Select **Go to Plugins** and install Tailscale. |
| No **Remote access** in Settings | Sign in as the owner. |
| Stuck on **Getting your sign-in link…** | Check the [log](logs.md) for lines starting with `plugin tailscale:`. HolyArrs needs to reach `controlplane.tailscale.com`. |
| The address won't open on my phone | Open the Tailscale app and check it's on and signed in to the same account. |
| The sign-in link stopped working | Links expire. Select **Connect to Tailscale** again for a new one. |
| "Couldn't read the Tailscale status" | Check that the plugin shows **Running** on the Plugins page, then select **Start** if not. |
| Always **Relayed** | Expected: plugins can't use UDP, see [Relayed connections](#relayed-connections). |
