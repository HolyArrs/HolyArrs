---
title: Kid accounts
description: Give a child an account that sees and asks for only titles rated for their age.
level: Start
sidebar: { order: 3 }
---

A Kid account is a member with an age-rating limit. They see and ask for only movies and shows rated at or below it.

## Make a Kid account

1. Turn off **Skip sign-in on my local network** in **Settings > Sign-in and security**. With it on, everyone at home is the owner, so a limit cannot be set. See [Sign-in and passwords](sign-in.md).
2. Open **Settings > Users and roles** and start an invite as a **Member**.
3. Pick an age limit, for example **PG**.
4. Keep only **Request titles**, plus **Requests approved without waiting** if you want.
5. Make the link and give it to the child. You should see the invite listed as open. See [Users and roles](roles.md#invite-someone).

To limit an existing member, pick a limit on their row in **Settings > Users and roles**. It applies from their next click, with no sign-out, and their API keys follow it.

## Choices

### Pick the limit

Movie ratings, lowest first: G, PG, PG-13, R, NC-17. Show ratings: TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA. A limit on one ladder maps onto the other:

| Movie limit | Shows up to | | Show limit | Movies up to |
| --- | --- | --- | --- | --- |
| G | TV-G | | TV-Y | G |
| PG | TV-PG | | TV-Y7 | G |
| PG-13 | TV-14 | | TV-G | G |
| R | TV-MA | | TV-PG | PG |
| NC-17 | TV-MA | | TV-14 | PG-13 |
| | | | TV-MA | R |

A PG limit shows movies rated G or PG and shows rated TV-Y to TV-PG. Only US ratings exist for now.

### What stays hidden

- Movies and shows with no rating, rated NR, or rated off their ladder.
- All music, because music has no age ratings.

### Permissions

A Kid account holds only the two ask permissions. Others open Library screens such as Downloads and History, which the limit does not cover. To give more, clear the limit in the same change. Only members can have a limit; making them an admin clears it for good.

## What the child sees

HolyArrs hides titles on the server, so every screen and app gets the same answer.

| Place | With a limit |
| --- | --- |
| Library, Home, search | Allowed titles only; Home counts only those |
| A Library title above the limit | Looks like it does not exist |
| Explore and add searches | Allowed titles only; a title with no rating found stays hidden |
| A TMDB title page above the limit | "This title is rated above what this account may see." |
| People pages, movie series | Allowed movies only |
| Trailers | Never play on their own |
| Searches and Explore loads | 30 a minute |

Asking above the limit is refused with their own limit named, for example "This account can ask only for titles rated up to PG." Music asks are refused too.

## If it does not work

**Symptom:**
> Turn off skipping sign-in on the local network first.

**Cause:** the shortcut is on.
**Fix:** turn it off, then set the limit. While anyone has a limit, the shortcut stays off.

**Symptom:**
> A Kid account can only ask for titles.

**Cause:** a limit combined with another permission.
**Fix:** remove the extra permission, or clear the limit.

**Symptom:**
> Only members can have an age-rating limit.

**Cause:** the person is an owner or admin.
**Fix:** make them a member first.

**Symptom:** the child got in with a second account.
**Cause:** Plex, Jellyfin and single sign-on can let new people join as ordinary members with no limit. **Settings > Users and roles** warns when a Kid account exists and joining is open.
**Fix:** link the child's outside accounts to their Kid account, or let only listed names join.

## For experts

<details class="ha-more"><summary>Limits of the hiding</summary>

Posters sit in one shared cache named after their address. Someone who knows a poster's address can tell whether it is cached. The picture shows no title or rating, and the title stays hidden everywhere else.

</details>

<details class="ha-more"><summary>API fields</summary>

The limit is `max_rating`; the country is `rating_country` (always `US`). Both appear on `POST /api/v1/users/invites`, `PUT /api/v1/users/{id}`, `GET /api/v1/users`, `GET /api/v1/users/invites`, `GET /api/v1/auth/invites/{token}` and `GET /api/v1/auth/me`.

- Empty `max_rating` means no limit.
- On a change, empty clears the limit; leaving it out keeps it.
- Too many lookups answer 429 with `Retry-After`.

</details>

## Related

- [Users and roles](roles.md)
- [Sign-in and passwords](sign-in.md)
- [Asking for titles](../requests/asking.md)
