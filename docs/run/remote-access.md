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

1. Open **Plugins** and select **Install** next to **Tailscale**. See [Plugins](plugins.md).
2. Open **Settings > Remote access**. You should see **Waiting for sign-in**.
3. Select **Open Tailscale sign-in**, or scan the QR code with your phone.
4. Sign in to Tailscale and add the device. You should see **Connected** and the **Address** HolyArrs has on your tailnet.
5. On your phone or TV, open the Tailscale app, then open that address in a browser.

Every visit through Tailscale asks you to sign in to HolyArrs, even from a computer that skips sign-in at home.

## What each state means

| State | What to do |
|---|---|
| Starting… | Wait a few seconds. |
| Not connected yet | Select **Reconnect**, then sign in. |
| Waiting for sign-in | Select **Open Tailscale sign-in** or scan the QR code. |
| Waiting for your tailnet admin to approve this device | Approve the device in Tailscale's admin page, or ask whoever runs your tailnet. |
| Connected | Nothing. |
| Connected · login expires within 14 days | Sign in again before the date under **Login expires**, or turn off key expiry for this device in Tailscale's admin page. |
| Login expired. Sign in again. | Sign in again with the link or QR code. |
| Removed in Tailscale. Sign in again. | Someone removed the device in Tailscale. Sign in again to add it back. |
| Disconnected (still signed in) | Select **Reconnect**. |

## Buttons

- **Reconnect** joins the tailnet again.
- **Disconnect** leaves the tailnet for now but keeps the login, so **Reconnect** needs no new sign-in.
- **Sign out** leaves the tailnet and wipes HolyArrs' Tailscale login. If HolyArrs says "Remove the device in Tailscale's admin page too.", open that page and remove the HolyArrs device.

## Faster connections

**Connection** shows **Direct** or **Relayed**. Relayed works, but it's slower. For direct connections, let UDP port `41641` reach the HolyArrs server. This is optional. With Docker, add it to your ports:

```yaml
ports:
  - "6262:6262"
  - "41641:41641/udp"
```

If your router blocks it, forward UDP `41641` to the server too.

## Headscale

To use your own Headscale server, set its address on HolyArrs and restart:

```yaml
environment:
  - HOLYARRS_TAILSCALE_LOGIN_SERVER=https://headscale.example.com
```

Then follow [Set it up](#set-it-up). The sign-in link opens your Headscale server.

You can also change the device name with `HOLYARRS_TAILSCALE_HOSTNAME`. The default is `holyarrs`.

## Troubleshooting

| Problem | Fix |
|---|---|
| "Remote access uses the Tailscale plugin." | Select **Go to Plugins** and install Tailscale. |
| No **Remote access** in Settings | Sign in as the owner. |
| The address won't open on my phone | Open the Tailscale app and check it's on and signed in to the same account. |
| Stuck on **Waiting for sign-in** | Open the link again. Links expire, so select **Reconnect** for a new one. |
| "Couldn't read the Tailscale status" | Check that the plugin shows **Running** on the Plugins page, then select **Start** if not. |
| Always **Relayed** | Open UDP `41641`, see [Faster connections](#faster-connections). |
