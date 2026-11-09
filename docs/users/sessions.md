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

**Members and admins:** ask the owner.

**The owner:** while HolyArrs runs, run this on its machine:

```sh
docker exec -i holyarrs /holyarrs reset-owner
```

Type the name on one line and the new password on the next, then press Ctrl+D. Without Docker, run `holyarrs reset-owner`. You should be able to sign in with the new password.

:::caution
`reset-owner` turns off the owner's two-factor sign-in, signs out every device and removes every owner API key.
:::

The sign-in page shows the same steps under **Forgot the password?**.

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
