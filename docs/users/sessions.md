---
title: Sessions and passwords
description: See where an account is signed in, sign out a lost device, and reset a forgotten owner password.
level: Setup
sidebar: { order: 4 }
---

## Sign out a device

1. Open **My account > Signed-in devices**. Each row shows a device name such as "Firefox on Windows", when it signed in and when it was last used. Yours says **This device**.
2. Select **Sign out** on a row. You should see the row disappear; that device's next click goes to sign-in.
3. Or select **Sign out all other devices** to keep only this one.

Both need a password sign-in, or an API key with the admin scope. The local network shortcut is not enough.

## Forgot your password

**Members and admins:** ask the owner or an admin. They set a new password in **Settings > Users**.

**The owner, from the sign-in page:** press **Forgot password?**, then **Make a code**. HolyArrs writes a one-time code to its log and to `reset-owner-code.txt` in your config folder. Open the log (`docker logs holyarrs`, or the Logs button in Docker Desktop, Unraid, TrueNAS, Synology or Portainer), paste the code, then pick the owner name and a new password. The code works once, for 15 minutes, and five wrong tries end it. No shell is needed. You can turn the link off in **Settings > Sign-in**.

If the page can't be reached, set `HOLYARRS_RESET_OWNER=1` and restart: a code shows in the log for 30 minutes and the sign-in page opens at the code step. Remove the variable once you are back in. `holyarrs reset-owner --code` prints a fresh code too.

**The owner, from a terminal:** while HolyArrs runs, open a terminal on the machine that runs Docker and run:

```sh
docker exec -it holyarrs /holyarrs reset-owner
```

It asks for the name, then the new password twice; the password does not show while you type. You should see `Done. Sign in as "<name>" with the new password.` Without Docker, run `holyarrs reset-owner` as the user HolyArrs runs as.

:::caution
Both ways turn off the owner's two-factor sign-in, sign out every device and remove every owner API key.
:::

If it does not work:

| You see | What to do |
| --- | --- |
| `OCI runtime exec failed` or `executable file not found` | The image has no shell. Keep `/holyarrs` in the command, and don't use the app's **Shell** or **Console** button in TrueNAS, Unraid, Synology or Portainer: those start a shell. Use the host's terminal: **System > Shell** on TrueNAS, the terminal icon at the top of Unraid, or SSH on Synology. |
| `No such container` | Your container has another name. `docker ps` lists them; use that name in place of `holyarrs`, such as `ix-holyarrs-holyarrs-1` on TrueNAS. |
| `nothing was typed in` | `-it` is missing. Run the command exactly as shown above. |
| Nothing happens after `Name:` | Docker got `-t` without `-i`. Press Ctrl+C and run it with `-it`. |

The sign-in page shows the same command under **Can't sign in?**. `holyarrs help` lists every command.

## Choices

### How long you stay signed in

| Event | Result |
| --- | --- |
| 7 days without use | Device signed out |
| 30 days after sign-in | Device signed out, used or not |
| You change your password | Every other device signed out |

Last-used times for devices and API keys can lag by up to 1 hour.

### Other people's devices

Open **Settings > Users and roles**, then **Devices** beside the person. Select **Sign out** on one device, or **Sign (name) out everywhere**. The owner can do this for anyone; an admin for members. Turning a person off also signs them out everywhere.

## If it does not work

**Symptom:** **Sign out all other devices** is refused.
**Cause:** you are in by API key or the local network shortcut.
**Fix:** sign in with a password in a browser and try again.

## For experts

<details class="ha-more"><summary>Sessions API</summary>

| Route | Who | What |
|---|---|---|
| `GET /api/v1/auth/sessions` | Anyone signed in | Your devices: `id`, `device`, `created_at`, `last_used_at`, `expires_at`, `current` |
| `DELETE /api/v1/auth/sessions` | Anyone signed in | End every other device. Answers `{"ended": n}`; 400 for an API key or the local network shortcut |
| `DELETE /api/v1/auth/sessions/{sid}` | Anyone signed in | End one device |
| `GET /api/v1/users/{id}/sessions` | Owner, admin | One person's devices |
| `POST /api/v1/users/{id}/sessions/end-all` | Owner, admin | Sign one person out everywhere |
| `DELETE /api/v1/users/{id}/sessions/{sid}` | Owner, admin | End one of their devices |

The `id` is a random public value and cannot sign anyone in. API keys need the admin scope.

</details>

## Related

- [Sign-in and passwords](sign-in.md)
- [Users and roles](roles.md)
