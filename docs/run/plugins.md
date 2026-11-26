---
title: Plugins
description: Install, start, stop and remove HolyArrs plugins, and what the notices on the Plugins page mean.
level: Setup
sidebar: { order: 12 }
---

Plugins add extra features to HolyArrs, such as [remote access with Tailscale](remote-access.md) or [getting Sources past bot checks with FlareSolverr](../downloads/flaresolverr.md).

## Before you start

- Only the owner sees and manages **Plugins**.
- Plugins run only when HolyArrs runs on Linux. The Docker image counts.
- HolyArrs installs only plugins signed by HolyArrs. You can't add a plugin from anywhere else.
- Each plugin runs locked into its own data folder and can reach only the websites it lists. Each card shows the websites, folders and HolyArrs actions the plugin lists.

## Limits

Every running plugin is held to these limits:

- Memory: up to 512 MB.
- Processor: up to 1 CPU core.
- Processes: up to 256.
- Open files: up to 1,024 at once.
- Files and folders: its own data folder, plus the folders you choose for it.
- Websites: only the ones it lists. Everything else on the internet and on your network is blocked, and so are UDP and listening for connections.
- How many connections it tried that were blocked shows under **Your choices** on the plugin.
- When it stops, everything it started stops too.

The memory, processor and process caps need cgroups v2 on the host; without them those three aren't applied.

## Available plugins

| Plugin | What it does |
|---|---|
| Tailscale | Reach HolyArrs from your own devices anywhere. See [Remote access](remote-access.md). |
| FlareSolverr | Gets Sources past a site's bot check with the FlareSolverr you run. See [Get past bot checks with FlareSolverr](../downloads/flaresolverr.md). |

## Install a plugin

To install Tailscale, follow these steps and pick **Tailscale**, then finish its sign-in in [Remote access](remote-access.md).

1. Open **Plugins** under **System** in the menu.
2. Select **Install** next to the plugin.
3. Read what it lists: **Websites**, **Folders** and **HolyArrs actions**.
4. Confirm. You should see **Running** next to it.

### Folders and updates

- A plugin that asks for folders gets none until you choose them. Open the plugin, type the folder on this server under **Your choices**, and select **Save**. Each folder shows whether the plugin may only read it or also write to it.
- System folders and HolyArrs's own config folder can't be chosen.
- An update that asks for more (a new website or folder, or writing where it could only read) is installed but stays stopped. Open the plugin, check the list, and select **Approve new access**.
- If a chosen folder is removed or replaced, the plugin stops with "a folder this plugin uses moved or changed". Choose the folder again.

### Your own server (Tailscale with Headscale, FlareSolverr)

Tailscale reaches only Tailscale's own servers. For a self-hosted Headscale, add its name under **Also allow your own server**, for example `headscale.example.com:8080`. The FlareSolverr plugin reaches only what you add there, such as `flaresolverr:8191`. Up to 100; names only, not addresses. These may reach devices on your network.

### Test versions

Some plugins publish test versions before a release. Open the plugin and select **Get test versions** to get them at the next update. They are signed and checked like normal versions, but not finished. Select **Stop test versions** to go back to normal versions at the next update.

### Developer mode

Developer mode is for people building their own plugin. It runs unsigned plugins from a folder on your server, in the same sandbox. Turn it on under **Add** > **Build with code**. A banner shows while it is on. Leave it off unless you are building a plugin. See [Developer mode](../dev/plugins.md#developer-mode).

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
| Network lock isn't available | This system can't keep plugins to the websites they list. Plugins that use the internet stay stopped. | See **Network lock requirements** below. Or select **Start them anyway**: they can then reach any address, but UDP stays blocked. |

## Network lock requirements

The network lock needs Linux 5.9 or newer. It works in Docker's and Podman's default security settings, rootless too, with no extra capabilities. The card says why it's missing:

- **Too old**: the host's Linux is older than 5.9. Update the host (Unraid 6.12, TrueNAS 24.10 and Debian 12 are new enough).
- **Blocked by the container's security settings**: a custom seccomp profile blocks `seccomp` or `process_vm_readv`, the container runs under gVisor, or the host sets `kernel.yama.ptrace_scope` to 2 or 3. Use the default profile, or set ptrace_scope to 1.
- **Seccomp is turned off**: the host's kernel was built without seccomp.

## Tailscale without direct connections

UDP is blocked for every plugin, so Tailscale always connects through Tailscale's encrypted relays (DERP over HTTPS). It works from anywhere but can be slower than a direct connection. The Remote access page shows **Relayed**.

## Troubleshooting

| Problem | Fix |
|---|---|
| No **Plugins** in the menu | Sign in as the owner. |
| "Plugins aren't available on this server." | Plugins are off on this server. Run HolyArrs on Linux or in Docker. |
| "Couldn't install ..." or "Couldn't start ..." | Try again. If it keeps failing, check the [log](logs.md) for the plugin's name. |
| A plugin stopped by itself | It hit one of its **Limits**, or it was blocked. Select **Start**, then check the log. |
