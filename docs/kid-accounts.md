# Kid accounts

A Kid account is a member with an age-rating limit. HolyArrs then shows that
person only the movies and shows rated at or below the limit, and lets them ask
only for those.

## Who may set a limit

- The owner and admins set a limit when they make a member invite, or later
  when they change a member on the Users screen.
- Only members can have a limit. An owner or admin never has one. Making a
  member an admin clears their limit, and making them a member again does not
  bring the old limit back.
- A changed limit holds from the person's next request. They do not need to
  sign out, and their API keys follow the same limit.

In the API the limit is `max_rating` and the country is `rating_country`, on
`POST /api/v1/users/invites`, `PUT /api/v1/users/{id}`, `GET /api/v1/users`,
`GET /api/v1/users/invites`, `GET /api/v1/auth/invites/{token}` and
`GET /api/v1/auth/me`. An empty `max_rating` means no limit. On a change, an
empty `max_rating` clears the limit and leaving it out keeps it.

## The ladders

The limit is one value from either ladder, lowest first:

| Kind | Ladder |
| --- | --- |
| Movies | G, PG, PG-13, R, NC-17 |
| Shows | TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA |

A limit picked from one ladder is matched onto the other one with this table,
then each title is compared with the limit on its own ladder:

| Movie limit | Shows up to | | Show limit | Movies up to |
| --- | --- | --- | --- | --- |
| G | TV-G | | TV-Y | G |
| PG | TV-PG | | TV-Y7 | G |
| PG-13 | TV-14 | | TV-G | G |
| R | TV-MA | | TV-PG | PG |
| NC-17 | TV-MA | | TV-14 | PG-13 |
| | | | TV-MA | R |

So a Kid with the limit PG sees movies rated G or PG and shows rated TV-Y,
TV-Y7, TV-G or TV-PG.

## Titles without a rating and music stay hidden

With a limit set, these are never shown and cannot be asked for:

- a movie or show with no rating, rated NR, or with a rating that is not on
  the ladder for its kind;
- every music artist, album and track, because music has no age ratings.

## What a Kid sees

The server does the hiding, so every screen and every API client gets the
same answer:

- **Library, Home and search** list only the titles the limit allows, and
  Home's counts count only those.
- **A title page** for a Library title above the limit answers "not found",
  exactly like a title that is not there, on every page of that title
  (seasons, episodes, people and the rest).
- **Explore rows, movie search and the add searches** show only allowed
  movies and shows. A row item carries no rating, so HolyArrs takes it from
  the Library, then from the ratings and details it keeps for 24 hours, then
  from one small TMDB rating lookup (at most four at a time). One answer
  makes at most 24 such lookups, and each Kid at most 120 a minute; an item
  whose rating cannot be found stays hidden. A Kid's searches, add searches,
  TMDB title pages and Explore rows together are limited to 30 a minute
  (429 with Retry-After after that).
- **A TMDB title page** above the limit answers "This title is rated above
  what this account may see.", and its "more like this" list keeps only
  allowed titles.
- **People pages** list only the person's movies that are in the Library
  and allowed. **Movie series** show only their allowed movies, and a series
  with none is not shown.
- **Calendar** (for a Kid who may run the Library) lists only allowed titles.
- **Asking** for a title above the limit is refused with "This account can
  ask only for titles rated up to PG." (with the Kid's own limit). Asking for
  music is refused with "This account cannot ask for music, because music
  has no age ratings." A Kid also cannot join someone else's ask for a title
  above the limit.

A Kid account may hold only the Ask permission and, if the owner wants, the
permission to have asks approved without waiting (`ask` and `auto_approve`).
Any other permission would open Library-work screens such as Downloads,
History, jobs and the live updates, which the limit does not cover. So an
invite or a change on `PUT /users/{id}` that combines a limit with any other
permission answers 400 "A Kid account can only ask for titles." To give such
a person more, clear the limit in the same change.

## Sign-in and Kid accounts

- **Skipping sign-in on the local network** makes everyone at home the
  owner, so a limit would mean nothing. While it is on, setting a limit
  answers 400 "Turn off skipping sign-in on the local network first: with it
  on, anyone at home signs in as the owner." While anyone has a limit,
  turning it on answers 400 too. If both are on anyway (for example a Kid
  joined from an older invite), Settings shows an error on the sign-in page.
- **Plex, Jellyfin, Emby and single sign-on** can let new people join by
  signing in there. Such a newcomer is an ordinary member with no age limit,
  even when it is a child. While a Kid account exists and any of these lets
  new people join, Settings shows a warning on the Users page. Link a Kid's
  outside accounts to their Kid account, or let only listed names join, so
  the child cannot come in through a second, unlimited account.

## Pictures

Posters are kept in one picture cache shared by everyone, and a picture's
cache name comes from its address. Someone who knows a title's poster
address can therefore tell whether that poster is cached, which hints that
someone in the house looked at the title. The picture itself shows no
title or rating, and the Kid limit still hides the title everywhere else.

## US ratings only for now

HolyArrs keeps only the US rating of each title, so `rating_country` is `US`
and any other country is refused with "Only US age ratings are known for now."
Other errors are just as plain: an unknown rating lists the choices, and a
limit on an admin answers "Only members can have an age-rating limit."
