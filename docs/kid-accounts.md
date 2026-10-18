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

## US ratings only for now

HolyArrs keeps only the US rating of each title, so `rating_country` is `US`
and any other country is refused with "Only US age ratings are known for now."
Other errors are just as plain: an unknown rating lists the choices, and a
limit on an admin answers "Only members can have an age-rating limit."
