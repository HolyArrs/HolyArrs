---
title: Send downloads through a VPN
description: Put qBittorrent inside a gluetun VPN container so torrent traffic only leaves through the VPN, and connect HolyArrs to it.
level: Setup
sidebar: { order: 5 }
---

Keep torrent traffic inside a VPN. The Downloader is what connects to other peers, so it is what goes inside the VPN. HolyArrs itself can stay outside: its own traffic is searches, lookups and fetching the Release file, not the download.

The usual way is [gluetun](https://github.com/qdm12/gluetun), a container that connects to your VPN provider. Another container can share gluetun's network, so all of its traffic goes through the VPN. gluetun's firewall blocks anything else, except the VPN server itself and LAN addresses you allow.

## Before you start

- A VPN provider that gluetun supports, and its WireGuard or OpenVPN details. See [gluetun's provider list](https://github.com/qdm12/gluetun-wiki/tree/main/setup/providers).
- HolyArrs and qBittorrent in the same Docker Compose file.

## Set it up

1. Add gluetun, with the settings for your provider:

   ```yaml
   services:
     gluetun:
       image: qmcgaw/gluetun
       cap_add:
         - NET_ADMIN
       devices:
         - /dev/net/tun
       environment:
         - VPN_SERVICE_PROVIDER=your-provider
         - VPN_TYPE=wireguard
         - WIREGUARD_PRIVATE_KEY=your-key
       ports:
         - "8080:8080" # qBittorrent's web page, published here, not on qBittorrent
       restart: unless-stopped
   ```

2. Put qBittorrent inside gluetun's network. It has no `ports:` of its own. Run it as the same user as HolyArrs (`1000` unless you changed HolyArrs's `PUID`), so HolyArrs can manage its files. `depends_on` makes it wait until the VPN is up:

   ```yaml
     qbittorrent:
       image: lscr.io/linuxserver/qbittorrent
       network_mode: "service:gluetun"
       environment:
         - PUID=1000
         - PGID=1000
         - WEBUI_PORT=8080
       volumes:
         - /path/to/qbittorrent-config:/config
         - /path/to/data:/data
       depends_on:
         gluetun:
           condition: service_healthy
       restart: unless-stopped
   ```

3. Give HolyArrs the same `/data` folder. See [Folders](../../start/folders/).

   ```yaml
     holyarrs:
       image: holyarrs/holyarrs:latest
       ports:
         - "6262:6262"
       volumes:
         - /path/to/holyarrs-config:/config
         - /path/to/data:/data
   ```

4. Start them: `docker compose up -d`.
5. In qBittorrent, set **Default Save Path** to a folder under `/data`, such as `/data/downloads`.
6. In HolyArrs, open **Settings > Downloaders** and add qBittorrent. The **Address** is gluetun's name and qBittorrent's port: `http://gluetun:8080`. See [Downloaders](../downloaders/).

To check the VPN is in use, ask qBittorrent's container for its public address: `docker compose exec qbittorrent curl -s https://ipinfo.io/ip`. It should be the VPN's address, not your home's.

## Things to know

### Never recreate gluetun on its own

qBittorrent shares gluetun's network. `docker compose up -d` with no names recreates both together when gluetun changes. But `docker compose up -d gluetun`, or anything else that recreates gluetun alone, leaves qBittorrent tied to the old gluetun. It keeps running with no network and shows no error.

To fix it, remove and start both:

```sh
docker compose rm -sf gluetun qbittorrent && docker compose up -d gluetun qbittorrent
```

Not `docker compose down`, which also stops everything else in the file.

### A tunnel that is up but passes nothing

A VPN can fail quietly. gluetun stays up and qBittorrent's page still answers, but nothing downloads. HolyArrs handles this the safe way:

- A download with no progress for **Stalled after** (6 hours by default) counts as stalled. A stall alone never turns the Release down and never deletes its files, so the same Release resumes once the tunnel works again.
- After **Try the next Release after** (24 hours by default) HolyArrs searches again. A Release that keeps stalling waits longer before each re-send, so a dead tunnel does not get the same Release over and over.
- **Status** flags a Downloader that stopped answering.

One setting can still fail torrents during a long outage: **Give up on dead torrents sooner** fails a torrent that stays slow for the hours you set. Leave it off if your VPN can take hours to recover.

## If it does not work

**Symptom.** **Test** on the Downloader fails with a connection error.
**Cause.** HolyArrs is not on the same Docker network as gluetun, or the address uses `qbittorrent` as the host.
**Fix.** Put both in one Compose file and use `http://gluetun:<port>`. qBittorrent has no network name of its own.

**Symptom.** qBittorrent never starts, and gluetun shows as unhealthy.
**Cause.** The VPN did not connect: wrong key, provider or server.
**Fix.** Read `docker logs gluetun`. qBittorrent waits for gluetun on purpose.

**Symptom.** Downloads stopped, and nothing shows an error.
**Cause.** gluetun was recreated without qBittorrent.
**Fix.** Remove and start both. See [Never recreate gluetun on its own](#never-recreate-gluetun-on-its-own).
