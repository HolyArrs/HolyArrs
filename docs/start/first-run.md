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

   Every part you leave ticked needs a folder. TV shows start ticked; untick it if you only want movies.

   Keep these folders in the same mount as your downloads, such as `/data/media/movies` next to `/data/downloads`. Then imports are instant and take no extra space. Folders on separate mounts, such as `/movies` and `/downloads`, make every import a full copy. See [Folders](folders.md).
3. **Which app downloads?** Connect your downloader, or press **Skip for now**. [Connect](../connect/) walks through it.
4. **Where should we search?** Link Prowlarr or Jackett, or press **Skip for now** and add a built-in site later in **Settings > Sources**. Also covered in [Connect](../connect/).
5. **HolyArrs is ready.** Press **Open Home**, or **Invite family** to turn on Requests and add users.

If **Bring my apps** found a downloader or a source, HolyArrs skips that step.

## Choices

### Skip for now

The folder, downloader and source steps each have it. Skipped steps show as **Not set** on the last screen; each opens the matching Settings page.

### Change your answers later

Open `/welcome` any time to change what you use: movies, TV shows, music, subtitles, and whether family asks for titles. Saying yes to family adds a **Requests** page.

### Start without the wizard

A tool such as NixOS, Ansible or Kubernetes can make the owner and an API key at start, so no browser is needed and nobody can claim a fresh install first.

1. Make the files, readable only by you. The second line needs `openssl`; skip it if your tool needs no API key:

   ```sh
   umask 077
   printf '%s\n' 'your owner password' > holyarrs_owner
   echo "hak_$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=')" > holyarrs_api_key
   ```

   HolyArrs reads them as the `PUID` user, so hand them to that user if it is not you: `sudo chown 1000:1000 holyarrs_owner holyarrs_api_key`. HolyArrs reads the password file only while there is no owner, so you may remove it after the first start.

2. Hand the files to the container and name them, then start HolyArrs:

   ```yaml
   services:
     holyarrs:
       environment:
         - HOLYARRS_OWNER_NAME=admin
         - HOLYARRS_OWNER_PASSWORD_FILE=/run/secrets/holyarrs_owner
         - HOLYARRS_API_KEY_FILE=/run/secrets/holyarrs_api_key
       secrets:
         - holyarrs_owner
         - holyarrs_api_key
   secrets:
     holyarrs_owner:
       file: ./holyarrs_owner
     holyarrs_api_key:
       file: ./holyarrs_api_key
   ```

   The log says `first-run setup made the owner`. Send the key in the `X-Api-Key` header.

3. Sign in as the owner to answer the setup steps, or leave them for later.

How each start treats them:

- An owner who exists keeps their name and password. Change those in **My account**, or with `reset-owner`. Changing the password in **My account** removes the owner's API keys, the file's key too, so put a new key in the file before the next start.
- The same key file keeps the same key. To change the key, put a new one in the file: the old key stops working on the next start.
- `HOLYARRS_API_KEY_SCOPES` changes the key's scopes on the next start. It is `all` when unset.
- Remove `HOLYARRS_API_KEY_FILE` and the key stays. Delete it in **Settings > API > API keys** if you no longer use it.
- A removed key never comes back: one deleted in **Settings > API > API keys** or through the API, or removed by a password change. While the file still holds it, HolyArrs does not start; put a new key in the file.
- `reset-owner` removes every API key. The next start makes the file's key again, unless that key was removed earlier; then put a new key in the file.
- Restoring a backup also restores the list of removed keys as it was then, so a key removed after that backup works again. If a key leaked, put a new key in the file after a restore.
- When a value has to be fixed, HolyArrs shows a plain page saying why instead of starting, and keeps it up until you fix the value and restart.
- A key file with no owner stops the start. Set the owner variables too, or finish setup in the browser first.

Every variable is in [Environment variables](../reference/env-vars.md#owner-and-api-key-at-start).

## If it does not work

| You see | Fix |
|---|---|
| Someone else became the owner | On the server, run `docker exec -it holyarrs /holyarrs reset-owner`. Everyone then signs in again. |
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
