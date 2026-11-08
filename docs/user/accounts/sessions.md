# Signed-in devices and passwords

## Your devices

My account > Signed-in devices lists every browser, phone or script where you are signed in:

- a short device name read from the browser ("Firefox on Windows", "Safari on iPhone"); the raw browser text is never shown,
- when it signed in and when it was last used,
- "This device" on the one you are looking from.

Last-use times can be up to an hour behind, for both signed-in devices and API keys.

**Sign out** ends one device at once; its next click goes to the sign-in page. **Sign out all other devices** ends every session but this one. Both need a password sign-in (or an API key with the admin scope); a local-network visit without a password cannot do it.

A session ends by itself after 7 days without use or 30 days after sign-in. Changing your password also signs out every other device.

## Other people's devices (owner and admins)

Settings > Users > **Devices** shows the same list for one person, with **Sign out** per device and **Sign (name) out everywhere**.

- The owner can see and end everyone's sessions.
- An admin can see and end members' sessions. Their own are in My account; the owner's and other admins' are the owner's call.

Turning a person off on the Users page also ends every session they have.

## Forgot the password

Members and admins ask the owner. The owner resets their own name and password on the machine that runs HolyArrs:

```
docker exec -i holyarrs /holyarrs reset-owner
```

Type the name on one line and the new password on the next, then press Ctrl+D. It works while HolyArrs runs, turns off the owner's two-factor sign-in and signs out every device and API key. The sign-in page shows this under "Forgot the password?".

## API

| Route | Who | What |
|---|---|---|
| `GET /api/v1/auth/sessions` | anyone signed in | your sessions: `id`, `device`, `created_at`, `last_used_at`, `expires_at`, `current` |
| `DELETE /api/v1/auth/sessions` | anyone signed in | sign out every other session; answers `{"ended": n}`. Refused (400) for an API key or the local network, which have no session of their own: use a signed-in browser session |
| `DELETE /api/v1/auth/sessions/{sid}` | anyone signed in | sign out one session |
| `GET /api/v1/users/{id}/sessions` | owner, admin | one person's sessions |
| `POST /api/v1/users/{id}/sessions/end-all` | owner, admin | sign one person out everywhere |
| `DELETE /api/v1/users/{id}/sessions/{sid}` | owner, admin | end one of their sessions |

The `id` of a session is a random public id; it is not the cookie and cannot be used to sign in. API keys need the admin scope for these routes.
