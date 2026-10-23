# Users, roles and two-factor sign-in

Open **Settings > Users** to invite people and decide what each person may do.
The owner and admins may open this screen, with the limits described below.

## Roles and permissions

There are three roles:

| Role | What it means |
| --- | --- |
| Owner | The first account. It can do everything and cannot be changed or turned off from the Users screen. |
| Admin | May manage members and the Library according to its permissions. |
| Member | Usually asks for titles and sees their own Wishlist. |

The five assignable permissions are:

| Permission | Allows |
| --- | --- |
| Ask for titles | Make asks through the Wishlist. |
| Asks approved without waiting | Add an ask without waiting for another person. |
| Approve or turn down asks | Decide other people's asks. |
| Run the Library | Search, organise and manage Library work. |
| Change settings | Open and change ordinary settings. |

A new admin starts with Ask, automatic approval, approval, and Library
management. A new member starts with Ask. The owner may change those defaults
for each invite or person. An admin may invite members and grant only
permissions that admin has. Only the owner may invite or promote an admin.

## Make and use an invite

1. Choose **Member** or, as the owner, **Admin**.
2. Choose the permissions.
3. Optionally add a private note that identifies the invite.
4. Choose how long the link lasts, from 1 to 30 days. The default is 7 days.
5. Select **Make an invite link** and copy it then. The full link is shown only
   once and works once.

The invited person opens `/invite/[token]`, chooses a unique name and a
password that meets the current minimum length, and is signed in. An invite no
longer works after it is used, expires, is taken back, or its maker is no
longer an active owner or admin.

The **Invite links** list shows whether each link is open, used or expired.
The owner may take back any open link. An admin may take back only a link they
made.

## Change or turn off a person

The owner may change an admin or member. An admin may change members, and may
grant only permissions the admin holds. Nobody changes their own role or
permissions from this screen.

Changing a role selects that role's default permissions before the change is
saved. Weekly movie, show and music ask limits accept a whole number from 0 to
1000. Empty uses the default from **Settings > Wishlist**; 0 means no limit.
People who may approve asks have no weekly ask limit.

**Turn off** signs the person out everywhere, removes their API keys, and
prevents another sign-in until they are turned on. It does not delete their
account. The owner sees every person's devices; an admin sees member devices.
Each person manages their own devices under **My account**.

These changes require a password sign-in. A session admitted by the local
network shortcut must sign in with a password first.

## Profile pictures

Everyone can set a picture for their own account on **My account > Profile
picture**. Choose a JPEG, PNG or WebP file up to 5 MB, check the round preview,
then select **Save picture**. HolyArrs crops the middle square, shrinks it to
256 by 256 pixels and keeps it as a JPEG in its database. **Remove** takes it
away. The picture replaces the coloured initials in the sidebar, on My account
and on **Settings > Users**; anyone without a picture keeps the initials.

For scripts:

| Route | What it does |
| --- | --- |
| `PUT /api/v1/account/avatar` | Send the image file as the request body. Answers `{"avatar": "<address>"}`. 413 over 5 MB, 415 when it is not a JPEG, PNG or WebP picture. |
| `DELETE /api/v1/account/avatar` | Removes your picture. |
| `GET /api/v1/users/{id}/avatar` | The picture of a signed-in person, cached, or 404. |

`GET /api/v1/auth/me` and `GET /api/v1/users` carry an `avatar` address, or
`null` when there is no picture. Changes need the usual CSRF header, and an
API key needs the write scope.

## Set up two-factor sign-in

Each person manages their own two-factor sign-in under **My account**:

1. Select **Set up two-factor sign-in**.
2. Add the shown account to an authenticator app, either through its link or by
   typing the shown key.
3. Enter the current six-digit code.
4. Store the ten backup codes somewhere safe. Each code works once and the
   list is shown only when it is made.

The person may make a new set of backup codes or turn two-factor sign-in off
by entering a current app code or an unused backup code. Making a new set
invalidates the old set.

Under **Settings > Sign-in**, the owner may require two-factor sign-in for all
admins. An admin without it may then open only **My account** until setup is
complete. This switch does not require members to use two-factor sign-in.

## Recovery

The owner may reset two-factor sign-in for an admin or member. An admin may
reset it for a member. The reset signs that person out everywhere; they may
then sign in with their password and set it up again.

If the owner loses both the authenticator and every backup code, run:

```powershell
docker exec -i holyarrs /holyarrs reset-owner
```

Run it on the machine that runs the documented Docker container. Native
installs use `holyarrs reset-owner`. It chooses a new owner name and password,
turns off the owner's two-factor sign-in, ends every session, and removes every
owner API key. See [Sign-in and passwords](sign-in.md) and
[Signed-in devices and passwords](sessions.md).

