---
title: FAQ
description: Short answers to the questions people ask most about HolyArrs.
level: Start
sidebar: { order: 2 }
---

Short answers, each with a link to the full page.

## What does HolyArrs do?

It runs movies, shows and music in one app: requests, searching your sources, picking a release, sending it to your downloader, importing the file, subtitles and alerts. It works with qBittorrent, SABnzbd, Transmission, NZBGet, Deluge and rTorrent. It does not download by itself.

## Do I need API keys for names and pictures?

No. Names, pictures and charts come from the HolyArrs.com metadata server. See [Metadata](../library/metadata/).

## Can I keep my old apps while I try it?

Yes. Moving over only reads your old apps; they keep running until you turn them off. See [Moving from other apps](../migrate/index.md).

## Will Seerr still work?

Yes. HolyArrs can answer Seerr and similar apps as if it were Sonarr and Radarr. It is off until you turn it on. See [Old-app API](../reference/old-app-api/).

## What port does it use?

`6262`. Open `http://your-server:6262`.

## Where are my settings stored?

In `/config`. Keep it on a local disk; HolyArrs refuses to run its database from a network share.

## I lost the master key. Can I get my passwords back?

No. Stored passwords, keys and tokens are locked with `holyarrs.key`, and backups don't hold it. Keep a copy somewhere safe. See [Backups](../run/backups/).

## I forgot the owner password.

Run `docker exec -i holyarrs /holyarrs reset-owner`. It asks for a new name and password and signs everyone out. The same command fixes an owner account someone else made. See [First run](../start/first-run/).

## Why can't I open a shell in the container?

The image holds only the HolyArrs program, so there is less for an attacker to use. [Troubleshooting](../help/troubleshooting/) shows a throwaway helper container for a closer look.

## Does HolyArrs send anything home?

Only what the metadata server needs to answer a lookup. See [Metadata](../library/metadata/).

## Related

- [Troubleshooting](../help/troubleshooting/)
- [Get support](../help/get-support/)
- [Glossary](../help/glossary/)
