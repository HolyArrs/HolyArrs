# Kid accounts

You want a child to use HolyArrs and see only titles that fit their age. A Kid
account is a member with an age-rating limit. HolyArrs shows that person only
movies and shows rated at or below the limit, and lets them ask only for
those.

## Make a Kid account

1. Turn off **Skip sign-in on the local network** first, under
   **Settings > Sign-in and security**. A limit cannot be set while it is on. See
   [Sign-in and passwords](sign-in.md).
2. Open **Settings > Users and roles** and start an invite as a **Member**.
3. Pick an age limit from either ladder below.
4. Keep only the Ask permission, and Asks approved without waiting if you
   want.
5. Make the link and give it to the child. See
   [Users, roles and two-factor sign-in](users-and-roles.md).

To limit an existing member, change them on **Settings > Users and roles** and pick a
limit. The new limit applies from their next click. They do not need to sign
out, and their API keys follow it too.

## Settings explained

### The ladders

Pick one value, lowest first:

| Kind | Ladder |
| --- | --- |
| Movies | G, PG, PG-13, R, NC-17 |
| Shows | TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA |

A limit from one ladder is matched onto the other:

| Movie limit | Shows up to | | Show limit | Movies up to |
| --- | --- | --- | --- | --- |
| G | TV-G | | TV-Y | G |
| PG | TV-PG | | TV-Y7 | G |
| PG-13 | TV-14 | | TV-G | G |
| R | TV-MA | | TV-PG | PG |
| NC-17 | TV-MA | | TV-14 | PG-13 |
| | | | TV-MA | R |

So a limit of PG shows movies rated G or PG, and shows rated TV-Y up to
TV-PG.

Only US ratings are known for now.

### What stays hidden

With a limit set, these never show and cannot be asked for:

- movies and shows with no rating, rated NR, or with a rating not on their
  ladder,
- all music, because music has no age ratings.

### Permissions

A Kid account can hold only Ask and Asks approved without waiting. Other
permissions open Library screens such as Downloads and History, which the
limit does not cover. To give the person more, clear the limit in the same
change.

Only members can have a limit. Making a member an admin clears it, and making
them a member again does not bring it back.

## What the child sees

HolyArrs hides titles on the server, so every screen and app gets the same
answer.

- **Library, Home and search** list only allowed titles. Home's counts count
  only those.
- **A Library title above the limit** looks like it does not exist.
- **Explore and the add searches** show only allowed titles. A title whose
  rating cannot be found stays hidden.
- **A title page from TMDB** above the limit says "This title is rated above
  what this account may see."
- **People pages and movie series** show only allowed movies.
- **Trailers** never play on their own.
- **Asking** for a title above the limit is refused with "This account can
  ask only for titles rated up to PG." (with their own limit). Music is
  refused with "This account cannot ask for music, because music has no age
  ratings."

A Kid account's searches and Explore loads are limited to 30 a minute.

## Troubleshooting

**"Turn off skipping sign-in on the local network first."** With that shortcut
on, everyone at home signs in as the owner, so a limit means nothing. Turn it
off, then set the limit. While anyone has a limit, the shortcut cannot be
turned on. If both end up on, Settings shows an error on the sign-in page.

**"A Kid account can only ask for titles."** You combined a limit with a
permission other than Ask or Asks approved without waiting. Remove the extra
permission, or clear the limit.

**"Only members can have an age-rating limit."** You tried to limit an owner
or admin. Make them a member first.

**"Only US age ratings are known for now."** Only the US country works.

**The child got in with a second account.** Plex, Jellyfin and single sign-on
can let new people join by signing in there. A newcomer that way is an
ordinary member with no limit. Settings warns on the Users page when a Kid
account exists and joining is open. Link the child's outside accounts to
their Kid account, or let only listed names join. Sign in with Emby is held
back for now.

**Someone could guess what a child looked at.** Posters sit in one shared
cache named after their address. Someone who knows a poster's address can
tell whether it is cached. The picture shows no title or rating, and the
title stays hidden everywhere else.

## For scripts

The limit is `max_rating` and the country is `rating_country` (always `US`).
They appear on `POST /api/v1/users/invites`, `PUT /api/v1/users/{id}`,
`GET /api/v1/users`, `GET /api/v1/users/invites`,
`GET /api/v1/auth/invites/{token}` and `GET /api/v1/auth/me`. An empty
`max_rating` means no limit. On a change, empty clears the limit and leaving
it out keeps it. Too many lookups answer 429 with Retry-After.
