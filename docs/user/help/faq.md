# Questions people often ask

## What does HolyArrs do?

It handles movies, shows and music in one app: requests, searching your sites, picking a release, sending it to your downloader, importing the file, subtitles and alerts. It works with qBittorrent, SABnzbd, Transmission, NZBGet and Deluge. It does not download anything by itself.

## Do I need API keys for names and pictures?

No. Names, pictures and charts come from the HolyArrs.com metadata server. See [Metadata](../metadata/metadata-service.md).

## Can I keep my old apps while I try it?

Yes. Moving your data over only reads your old apps. They keep running until you turn them off. See [Move from other apps](../migrate/move-from-other-apps.md).

## Will Seerr still work?

Yes. HolyArrs can answer Seerr and similar apps as if it were Sonarr and Radarr. This is off until you turn it on. See [Apps that talk to Sonarr and Radarr](../arrapi/README.md).

## What port does it use?

6262. Open `http://your-server:6262` in your browser.

## Where are my settings stored?

In the `/config` folder. Keep it on a local disk. HolyArrs refuses to run its database from a network share.

## I lost the master key. Can I get my passwords back?

No. Passwords, keys and tokens are locked with that key, and backups don't hold it. Keep a copy of `holyarrs.key` somewhere safe. See [Backups](../backups/backups.md).

## I forgot the owner password, or someone else made the owner account.

Run `docker exec -i holyarrs /holyarrs reset-owner`. It reads a new name and password and signs everyone out. See [First run](../setup/first-run.md).

## Why can't I open a shell in the container?

The image holds only the HolyArrs program, so there is less for an attacker to use. [Troubleshooting](troubleshooting.md#why-there-is-no-shell) shows how to look around with a throwaway helper container.

## A feature I read about is missing.

It may be held back in the first release. See [Features held back](held-back.md).

## Does HolyArrs send anything home?

Only what the metadata server needs to answer a lookup. See [Metadata privacy](../metadata/metadata-privacy.md).
