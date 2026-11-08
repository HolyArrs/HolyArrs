# Signed-in devices and passwords

You want to see where your account is signed in, sign out a lost phone, or get
back in after forgetting a password. This page covers all three.

## See and sign out your devices

1. Open **My account > Signed-in devices**.
2. Each row shows a short device name, such as "Firefox on Windows", when it
   signed in and when it was last used. Your current one says
   **This device**.
3. Select **Sign out** on a row to end that device. Its next click goes to the
   sign-in page.
4. Or select **Sign out all other devices** to keep only this one.

Both need a password sign-in, or an API key with the admin scope. A visit let
in by the local network shortcut cannot do it.

## How long you stay signed in

- A device is signed out after 7 days without use.
- It is signed out 30 days after sign-in, used or not.
- Changing your password signs out every other device.
- Last-used times can be up to an hour behind. The same goes for API keys.

## Other people's devices

The owner and admins can do this from **Settings > Users and roles**, then **Devices** beside the person.

1. Pick the person.
2. Select **Sign out** on one device, or **Sign (name) out everywhere**.

The owner can see and end anyone's devices. An admin can do it for members.
Admins manage their own devices in My account. Turning a person off on the
Users page also signs them out everywhere.

## Forgot your password

**Members and admins:** ask the owner.

**The owner:** run this on the machine that runs HolyArrs, while it runs:

```
docker exec -i holyarrs /holyarrs reset-owner
```

Type the name on one line and the new password on the next, then press
Ctrl+D. Without Docker, run `holyarrs reset-owner`. It turns off the owner's
two-factor sign-in and signs out every device and API key. The sign-in page
shows the same steps under **Forgot the password?**.

## Troubleshooting

**Sign out all other devices is refused.** You are signed in by API key or by
the local network shortcut. Sign in with a password in a browser and try
again.

## For scripts

| Route | Who | What |
|---|---|---|
| `GET /api/v1/auth/sessions` | anyone signed in | your devices: `id`, `device`, `created_at`, `last_used_at`, `expires_at`, `current` |
| `DELETE /api/v1/auth/sessions` | anyone signed in | sign out every other device. Answers `{"ended": n}`. 400 for an API key or the local network shortcut. |
| `DELETE /api/v1/auth/sessions/{sid}` | anyone signed in | sign out one device |
| `GET /api/v1/users/{id}/sessions` | owner, admin | one person's devices |
| `POST /api/v1/users/{id}/sessions/end-all` | owner, admin | sign one person out everywhere |
| `DELETE /api/v1/users/{id}/sessions/{sid}` | owner, admin | end one of their devices |

The `id` is a random public value. It cannot be used to sign in. API keys need
the admin scope for these routes.
