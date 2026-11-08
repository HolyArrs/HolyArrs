# Run HolyArrs on Synology

You want HolyArrs in Container Manager on your Synology. Read [Install HolyArrs with Docker](../getting-started/install.md) and [Run HolyArrs on a NAS](nas.md) first.

## Steps

1. In File Station, make `docker/holyarrs/config`.
2. Make one shared folder for media, for example `media`, with `downloads`, `movies`, `tv` and `music` inside.
3. In Container Manager, open Project, then Create, and paste the Compose file from the Docker guide.
4. Set the volumes to `/volume1/docker/holyarrs/config:/config` and `/volume1/media:/data`.
5. Set `PUID` and `PGID` to your user. On Synology this is often `1026` and `100`. Check with `id <user>` over SSH.
6. Give that user read and write on the media folder: Control Panel, Shared Folder, Edit, Permissions.
7. Tick Enable auto-restart.
8. Open http://your-server:6262 and create the owner account. Add `/data/movies`, `/data/tv` and `/data/music` as Library folders.

## Troubleshooting

**HolyArrs stays stopped after a restart in the app.** Enable auto-restart is off. Turn it on.

**Imports fail with a permission error.** Your user needs read and write on the media shared folder.

More help: [Run HolyArrs on a NAS](nas.md#troubleshooting).
