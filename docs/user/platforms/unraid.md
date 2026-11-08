# Run HolyArrs on Unraid

You want HolyArrs on Unraid. Read [Install HolyArrs with Docker](../getting-started/install.md) and [Run HolyArrs on a NAS](nas.md) first.

There is no HolyArrs template in Community Applications yet, so you add the container by hand.

## Steps

1. In the Docker tab, add a container with the HolyArrs image.
2. Map port `6262`.
3. Map `/config` to a pool path, such as `/mnt/<pool>/appdata/holyarrs`. `<pool>` is your pool's name.
4. Map your media folders to `/movies`, `/tv`, `/music` and `/downloads`. Use disk or pool paths for them.
5. Set `PUID=99`, `PGID=100` (Unraid's `nobody:users`) and `UMASK=022`.
6. Make sure the container restarts on its own.
7. Open http://your-server:6262 and create the owner account.

## Settings explained

**Why not /mnt/user/appdata.** `/mnt/user` is a FUSE path. The database is not safe there, so HolyArrs refuses to start on it. Use a pool path. Or make the appdata share Exclusive: first turn on Permit exclusive shares in Settings > Global Share Settings, then set Shares > appdata to Primary storage on one pool and Secondary storage to none.

**Pool path must exist.** If you point `/config` at a pool that does not exist, the folder ends up in memory and is lost on reboot. HolyArrs refuses it.

**Hardlinks.** Separate `/movies`, `/tv`, `/music` and `/downloads` mappings work, but imports become copies. For hardlinks, map one share holding all of them. Avoid `/mnt/user` paths for media too, since hardlinks across them can fail.

## Troubleshooting

**HolyArrs will not start and mentions an Unraid user share.** `/config` points at `/mnt/user`. Change it to `/mnt/<pool>/appdata/holyarrs`.

More help: [Run HolyArrs on a NAS](nas.md#troubleshooting).
