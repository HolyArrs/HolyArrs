# Users, roles and two-factor sign-in

You want other people in your home to use HolyArrs, each with their own
account and only the rights they need. This page shows you how to invite them,
set what they may do, and protect accounts with two-factor sign-in.

Open **Settings > Users and roles** for all of this. The owner and admins can open it.

## Invite someone

1. Open **Settings > Users and roles**.
2. Choose **Member**. If you are the owner, you can choose **Admin** too.
3. Pick the permissions. The defaults for the role are already ticked.
4. Add a private note if you want to remember who the link is for.
5. Choose how long the link lasts, from 1 to 30 days. The default is 7 days.
6. Select **Make an invite link** and copy it right away. The full link is
   shown only once, and it works only once.

Send the link to the person. They open it, pick a name nobody else has, and
choose a password that meets the current minimum length. Then they are signed
in.

The **Invite links** list shows whether each link is open, used or expired.
The owner can take back any open link. An admin can take back only the links
they made. A link stops working when it is used, expires, is taken back, or
when the person who made it is no longer an active owner or admin.

To invite a child with an age limit, see [Kid accounts](kid-accounts.md).

## Roles

| Role | What it means |
| --- | --- |
| Owner | The first account. It can do everything. Nobody can change or turn it off from the Users screen. |
| Admin | Manages members and the Library, within its permissions. |
| Member | Usually asks for titles and sees their own requests. |

## Permissions

| Permission | What it allows |
| --- | --- |
| Ask for titles | Make requests. |
| Asks approved without waiting | A request goes through without someone approving it. |
| Approve or turn down asks | Decide other people's requests. |
| Run the Library | Search, organise and manage Library work. |
| Change settings | Open and change ordinary settings. |

A new admin starts with Ask, Asks approved without waiting, Approve, and Run
the Library. A new member starts with Ask. You can change these for each
invite or person.

Who can grant what:

- Only the owner can invite an admin or make someone an admin.
- An admin can invite and change members, and can grant only permissions the
  admin has.
- Nobody changes their own role or permissions here.

## Change a person

1. Open **Settings > Users and roles** and pick the person.
2. Change the role or permissions. Changing the role ticks that role's
   defaults before you save.
3. Set weekly limits for movie, show and music requests if you want. Each one
   takes a whole number from 0 to 1000. Leave it empty to use the default from
   the request settings at `/settings/requests`. 0 means no limit.
   People who can approve requests have no weekly limit.
4. Save.

**Turn off** signs the person out everywhere, removes their API keys, and
blocks new sign-ins until you turn them on again. Their account stays.

These changes need a password sign-in. If you came in through the local
network shortcut, sign in with your password first.

## Profile picture

Everyone can set their own picture.

1. Open **My account > Profile picture**.
2. Choose a JPEG, PNG or WebP file up to 5 MB.
3. Check the round preview and select **Save picture**.

HolyArrs crops the middle square and keeps it at 256 by 256 pixels. The
picture shows in the sidebar, on My account and on **Settings > Users and roles**.
Select **Remove** to go back to the coloured initials.

## Set up two-factor sign-in

Each person sets this up for their own account.

1. Open **My account** and select **Set up two-factor sign-in**.
2. Add the account to an authenticator app, with the link or by typing the
   key shown.
3. Enter the six-digit code from the app.
4. Save the ten backup codes somewhere safe. Each one works once, and you see
   the list only now.

To make new backup codes or turn two-factor sign-in off, enter a current app
code or an unused backup code. New backup codes replace the old ones.

The owner can require two-factor sign-in for all admins under
**Settings > Sign-in and security**. An admin without it can then open only **My account**
until they set it up. This does not apply to members.

## Troubleshooting

**Someone lost their authenticator and backup codes.** The owner can reset
two-factor sign-in for an admin or member. An admin can reset it for a member.
The person is signed out everywhere, then signs in with their password and
sets it up again.

**The owner lost their authenticator and backup codes.** Run this on the
machine that runs HolyArrs:

```
docker exec -i holyarrs /holyarrs reset-owner
```

Without Docker, run `holyarrs reset-owner`. It sets a new owner name and
password, turns off the owner's two-factor sign-in, signs out every device and
removes every owner API key. See
[Signed-in devices and passwords](sessions.md).

**An invite link does not work.** It was used, expired, taken back, or its
maker lost their admin role. Make a new one.

**An admin cannot tick a permission.** Admins can only grant permissions they
have themselves. Ask the owner.

## For scripts

| Route | What it does |
| --- | --- |
| `PUT /api/v1/account/avatar` | Send the image as the body. Answers `{"avatar": "<address>"}`. 413 over 5 MB, 415 when it is not JPEG, PNG or WebP. |
| `DELETE /api/v1/account/avatar` | Removes your picture. |
| `GET /api/v1/users/{id}/avatar` | A person's picture, or 404. |

`GET /api/v1/auth/me` and `GET /api/v1/users` include `avatar`, or `null`.
Changes need the usual CSRF header. An API key needs the write scope.
