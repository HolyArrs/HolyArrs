# HolyArrs

HolyArrs is one self-hosted app for your movies, shows and music. You or the people you share it with ask for something. HolyArrs finds a release on your Sources, chooses one with your Pick rules, hands it to your Downloader, and files the finished download in your Library.

Downloaders: qBittorrent, SABnzbd, Transmission, NZBGet and Deluge. Sources: Jackett or Prowlarr. Media servers: Plex, Jellyfin and Emby get told when something new arrives.

Already using Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Seerr or Overseerr? HolyArrs can read your old setup and bring it over. Your old apps keep running until you switch them off.

Website: [HolyArrs.com](https://holyarrs.com)

## Install

Add this to your Docker Compose file:

```yaml
services:
  holyarrs:
    image: holyarrs/holyarrs:beta
    container_name: holyarrs
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - ./config:/config
      - /path/to/media:/data
    ports:
      - 6262:6262
    restart: unless-stopped
```

1. Set `PUID` and `PGID` to the user that owns your media folders.
2. Start it with `docker compose up -d`.
3. Open `http://your-server:6262`.
4. Make the owner account right away. The first person to open a fresh HolyArrs becomes the owner.

Keep `/config` on a local disk. HolyArrs refuses to run its database from a network share.

Full guide: [Install HolyArrs with Docker](docs/start/install.mdx). NAS guides live in [docs/run/platforms.md](docs/run/platforms.md).

## Keep your master key safe

HolyArrs encrypts stored passwords, keys and tokens with the file `holyarrs.key` in your config folder. Backups do not include it. Copy it somewhere safe. Without it, a restored backup can't open your stored secrets. See [Backups](docs/run/backups.md).

## Metadata

You don't need any API keys. Titles, artwork and charts come from the HolyArrs metadata server. See [Metadata](docs/library/metadata.md).

## When something goes wrong

- Follow the log: `docker logs -f holyarrs`
- Check your setup: `docker exec holyarrs /holyarrs doctor`
- Locked out of the owner account: `docker exec -i holyarrs /holyarrs reset-owner`
- More help: [docs/help](docs/help/)

Ask questions on [Discord](https://discord.com/invite/mthsvesKpC). Share ideas on [r/HolyArrs](https://www.reddit.com/r/HolyArrs/). Report bugs in Issues.

## All docs

Start at [docs/README.md](docs/README.md).

## Contributing

Read [CONTRIBUTING](.github/CONTRIBUTING.md) first, then [Architecture](docs/dev/architecture.md) and [Setup](docs/dev/setup.md). The code lives in `app/backend`, `app/frontend` and `app/tests`. Docs are in `docs`, and Docker and release files are in `deploy`. Open an issue before you start a big change.

## Security

Report security problems privately. See [SECURITY](.github/SECURITY.md).

## Licence

HolyArrs is free software under the [GNU AGPL-3.0](LICENSE) (AGPL-3.0-only).

Running it at home is free, forever. You can read the code, change it and share it. If you share a changed version, or let other people use it over a network, you have to offer them your changed source under the same licence.

If you want to build HolyArrs into a closed product, or run a changed version without sharing your changes, you can buy a commercial licence instead. See [COMMERCIAL.md](COMMERCIAL.md).

Contributors agree to a short licence agreement on their first pull request. See [CLA.md](.github/CLA.md).

"HolyArrs" and the HolyArrs logo are trademarks of Luis (CoderLuii). The AGPL covers the code, not the name. If you ship a changed version, give it a different name.
