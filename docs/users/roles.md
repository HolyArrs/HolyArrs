---
title: Users and roles
description: Invite people, set their role and permissions, and turn on two-factor sign-in.
level: Setup
sidebar: { order: 2 }
---

Give each person in your home their own account with only the rights they need. The owner and admins work in **Settings > Users and roles**.

## Invite someone

1. Open **Settings > Users and roles**.
2. Choose **Member**. The owner can also choose **Admin**.
3. Check the permissions. The role's defaults are already ticked.
4. Add a private note if you want to remember who the link is for.
5. Pick how long the link lasts: 1 to 30 days, default 7.
6. Select **Make an invite link** and copy it now. You should see the full link once; it works once.

The person opens the link, picks a unique name and a password that meets the [current minimum](sign-in.md#set-the-shortest-password). They are then signed in.

For a child with an age limit, see [Kid accounts](kids.md).

## Roles and permissions

| Role | What it means |
| --- | --- |
| Owner | The first account. Can do everything. Nobody can change or turn it off on this screen |
| Admin | Manages members and the Library, within its permissions |
| Member | Usually asks for titles and sees their own requests |

| Permission | What it allows | New admin | New member |
| --- | --- | --- | --- |
| Request titles | Make requests | Yes | Yes |
| Requests approved without waiting | Requests skip approval | Yes | No |
| Approve or turn down requests | Decide other people's requests | Yes | No |
| Run the Library | Search, organise and manage Library work | Yes | No |
| Change settings | Open and change ordinary settings | No | No |

Who can grant what:

- Only the owner invites an admin or makes someone one.
- An admin invites and changes members, and grants only permissions the admin holds.
- Nobody changes their own role or permissions.

## Change a person

1. Open **Settings > Users and roles** and pick the person.
2. Change the role or permissions. A new role ticks its defaults before you save.
3. Optional: set weekly request limits for movies, shows and music, 0 to 1000 each. Empty uses the default from **Settings > Request rules**; 0 means no limit. People who can approve have no limit.
4. Select **Save**.

**Turn off** signs the person out everywhere, removes their API keys and blocks sign-in until you turn them on. The account stays.

These changes need a password sign-in. If you came in through the local network shortcut, sign in with your password first.

## Set up two-factor sign-in

Each person does this for their own account.

1. Open **My account**, find **Two-factor** and select **Set up**.
2. Add the account to an authenticator app, with the link or the key shown.
3. Enter the 6-digit code from the app.
4. Save the 10 backup codes somewhere safe. You see them only now; each works once.

To make new backup codes or turn two-factor off, enter a current app code or an unused backup code. New codes replace the old ones.

## Choices

### Require two-factor for admins

The owner turns this on under **Settings > Sign-in and security**. An admin without two-factor can then open only **My account** until they set it up. Members are not affected.

### Profile picture

Anyone can set one in **My account > Profile picture**: JPEG, PNG or WebP, up to 5 MB. HolyArrs crops the middle square to 256 by 256 pixels. **Remove** goes back to coloured initials.

## If it does not work

**Symptom:** an invite link does not work.
**Cause:** it was used, expired or taken back, or its maker is no longer an active owner or admin.
**Fix:** make a new one. The **Invite links** list shows each link's state; the owner can take back any open link, an admin only their own.

**Symptom:** an admin cannot tick a permission.
**Cause:** admins grant only permissions they hold.
**Fix:** ask the owner.

**Symptom:** someone lost their authenticator and backup codes.
**Cause:** no code left to sign in.
**Fix:** the owner resets two-factor for an admin or member; an admin resets it for a member. The person is signed out everywhere, signs in with their password and sets it up again. For the owner, see [Forgot your password](sessions.md#forgot-your-password); `reset-owner` also turns off the owner's two-factor.

## For experts

<details class="ha-more"><summary>Picture API</summary>

| Route | What it does |
| --- | --- |
| `PUT /api/v1/account/avatar` | Image as the body. Answers `{"avatar": "<address>"}`. 413 over 5 MB, 415 when not JPEG, PNG or WebP |
| `DELETE /api/v1/account/avatar` | Removes your picture |
| `GET /api/v1/users/{id}/avatar` | A person's picture, or 404 |

`GET /api/v1/auth/me` and `GET /api/v1/users` include `avatar`, or `null`. Changes need the usual CSRF header; an API key needs the write scope.

</details>

## Related

- [Sign-in and passwords](sign-in.md)
- [Kid accounts](kids.md)
- [Sessions and passwords](sessions.md)
