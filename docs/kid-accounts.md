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
  the Library, then from the details it keeps for 24 hours, then from one
  TMDB lookup (at most four at a time). An item whose rating cannot be found
  stays hidden.
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

A Kid account is meant to hold only the Ask permission. A Kid who is also
given Run the Library can open the Library-work screens, such as Downloads
and History, and the live updates, which still show every title.

## US ratings only for now

HolyArrs keeps only the US rating of each title, so `rating_country` is `US`
and any other country is refused with "Only US age ratings are known for now."
Other errors are just as plain: an unknown rating lists the choices, and a
limit on an admin answers "Only members can have an age-rating limit."
