---
title: Plugins
description: Install, start, stop and remove HolyArrs plugins, and what the notices on the Plugins page mean.
level: Setup
sidebar: { order: 12 }
---

Plugins add extra features to HolyArrs, such as [remote access with Tailscale](remote-access.md).

No plugins can be installed yet. Until the first one is listed, the page shows **No plugins yet.**

## Before you start

- Only the owner sees and manages **Plugins**.
- Plugins run only when HolyArrs runs on Linux. The Docker image counts.
- HolyArrs installs only plugins signed by HolyArrs. You can't add a plugin from anywhere else.
- Each plugin runs locked into its own folder. It reaches only the websites, folders and HolyArrs actions it lists. Each card on the page shows what that plugin can access.

## Limits

Every running plugin is held to these limits:

- Memory: up to 512 MB.
- Processor: up to 1 CPU core.
- Processes: up to 256.
- Open files: up to 1,024 at once.
- Files and folders: only its own folder.
- When it stops, everything it started stops too.

The memory, processor and process caps need cgroups v2 on the host; without them those three aren't applied.

## Install a plugin

1. Open **Plugins** under **System** in the menu.
2. Select **Install** next to the plugin.
3. Read what it can reach: **Websites**, **Folders** and **HolyArrs actions**. Anything not listed is out of its reach.
4. Confirm. You should see **Running** next to it.

## Start, stop and remove

- **Stop** shuts the plugin down. It stays installed and shows **Stopped**.
- **Start** runs it again.
- **Remove** asks what to do with its settings:
  - **Keep for later** deletes the plugin but saves its settings, so a new install picks them up.
  - **Sign out and delete** signs the plugin out of any account it uses and deletes everything it saved.

## Notices on the page

| Notice | What it means | What to do |
|---|---|---|
| Plugins can't be installed yet. This HolyArrs build doesn't have the plugin signing key. | This build can't check signatures, so it installs nothing. | Wait for a release that has the key. |
| Plugins run only on Linux. | HolyArrs runs on Windows or macOS here. | Run HolyArrs on Linux or in Docker to use plugins. |
| Couldn't check for blocked plugins for over a week. | HolyArrs couldn't fetch the list of blocked plugins for 7 days. Plugins keep running. | Check that HolyArrs can reach the internet. See [Outside addresses](outside-hosts.md). |
| Turned off: ... | HolyArrs stopped the plugin, for example because it was blocked. | Read the reason. Remove the plugin if it was blocked. |

## Troubleshooting

| Problem | Fix |
|---|---|
| No **Plugins** in the menu | Sign in as the owner. |
| "Plugins aren't available on this server." | Plugins are off on this server. Run HolyArrs on Linux or in Docker. |
| "Couldn't install ..." or "Couldn't start ..." | Try again. If it keeps failing, check the [log](logs.md) for the plugin's name. |
| A plugin stopped by itself | It hit one of its **Limits**, or it was blocked. Select **Start**, then check the log. |
