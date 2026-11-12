---
title: First run
description: Make the owner account and answer the setup steps so HolyArrs knows what you collect and where it lives.
level: Start
sidebar: { order: 3, label: "Step 3 of 5: First run" }
---

Make the owner account and tell HolyArrs what you collect. Nobody can use the app until the owner finishes this.

## Make the owner

1. Open `http://your-server:6262`.
2. Pick an owner name and a password of at least 8 characters.
3. Sign in.

   You should see **Coming from other apps?**

## Answer the setup steps

1. **Coming from other apps?** Pick **Start fresh** for a new setup. Pick **Bring my apps** to copy titles, folders and settings from Sonarr, Radarr, Lidarr, Overseerr and others. HolyArrs only reads them; your files and old apps stay as they are. See [Moving from other apps](../migrate/index.md).
2. **Where's your media?** Turn on what you collect: movies, TV shows, music. Type a folder for each, as HolyArrs sees it inside the container, such as `/data/media/movies`. Press **Continue**.

   Every part you leave ticked needs a folder. TV shows starts ticked; untick it if you only want movies.

   Keep these folders in the same mount as your downloads, such as `/data/media/movies` next to `/data/downloads`. Then imports are instant and take no extra space. Folders on separate mounts, such as `/movies` and `/downloads`, make every import a full copy. See [Folders](folders.md).
3. **Which app downloads?** Connect your downloader, or press **Skip for now**. [Connect](../connect/) walks through it.
4. **Where should we search?** Link Prowlarr or Jackett, or press **Skip for now**. Also covered in [Connect](../connect/).
5. **HolyArrs is ready.** Press **Open Home**, or **Invite family** to turn on Requests and add users.

If **Bring my apps** found a downloader or a source, HolyArrs skips that step.

## Choices

### Skip for now

The folder, downloader and source steps each have it. Skipped steps show as **Not set** on the last screen; each opens the matching Settings page.

### Change your answers later

Open `/welcome` any time to change what you use: movies, TV shows, music, subtitles, and whether family asks for titles. Saying yes to family adds a **Requests** page.

## If it does not work

| You see | Fix |
|---|---|
| Someone else became the owner | On the server, run `docker exec -i holyarrs /holyarrs reset-owner`. Everyone then signs in again. |
| Only the owner or an admin can finish setup | Sign in as the owner, or as an admin with Settings access. |
| A folder is refused | Each folder is used once and cannot sit inside another one you added. Pick another path. |
| HolyArrs is not answering | Wait a few seconds and retry. If it repeats, check `docker compose ps`. |

## For experts

<details class="ha-more"><summary>Setup details</summary>

| Item | Value |
|---|---|
| Password minimum | 8 characters by default, 4 to 64 under **Settings > Sign-in and security** |
| Setup sign-in | Stays fresh for up to 24 hours while setup runs |
| Owner on a fresh install | First visitor, no setup code |
| Navigation | Browser Back and Forward move between steps; reload keeps your place |
| Who can finish | Owner, or an admin with Settings access |

</details>

## Related

- [Folders](../folders/)
- [Connect](../connect/)
- [Users and roles](../../users/roles/)
