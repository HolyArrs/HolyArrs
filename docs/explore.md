# Explore

Explore is the page for finding something to ask for or add. It never plays
anything: HolyArrs is where people ask for titles, so Explore reads only your
Library, your asks and the charts from the
[metadata server](metadata-keys.md). Nothing about what people watch is
collected.

## The type switch

A switch at the top picks **All**, **Movies**, **Shows** or **Music**. The pick
is kept in the address (`?type=`) and remembered on this device. **Music**
shows only when the Music part is on; if it is turned off while picked, the
page falls back to All.

## How the page is built

The server builds each person's page, so two people see different rows in a
different order.

- **Head.** The first rows come in one answer, from a stored pool, so the page
  opens fast. Every title appears once in the head.
- **Tail.** Below the head, generated rows load four at a time as the page
  scrolls. How many there are grows with how much HolyArrs knows about the
  person (their Library titles and asks, called seeds): fewer than 3 seeds
  give at most 8 rows, 3 to 9 give at most 20, and 10 or more give at most 40.
  A Kid account stops at 20.
- **Order.** Tail rows are ordered by a score that is personal through those
  seeds. A small daily shuffle (up to 5 percent) keeps the page the same all
  day and different tomorrow. The same kind of row never appears twice in a
  row while another kind is left.
- **Cold start.** With an empty Library and no asks, the page still fills:
  the head holds the charts, and the tail holds up to 8 "Popular" genre and
  decade rows that name no person and no seed.

Counts of rows shown, cards opened and asks started are kept on the server to
measure only. They never change what Explore shows.

## Head rows

Each tab has its own head order. A slot without data is skipped, and the best
tail rows fill the head up to 12 rows. Just arrived, your asks and Coming
soon are Home rows, not Explore rows.

| Tab | Head order |
| --- | --- |
| All | Trending on streaming, Top picks for you, Trending this week everywhere, Because you have X, Best new releases, Because you asked for X, Popular in your household |
| Movies | Trending movies on streaming, Top picks for you, Finish the collection, Trending movies this week, Because you have X, Best new releases, Coming to cinemas and streaming, Popular in your household |
| Shows | Trending shows on streaming, Top picks for you, New seasons and premieres, Trending shows this week, Because you have X, Returning soon, Popular in your household |
| Under 3 seeds | Trending on streaming, Trending this week, Best new releases, the tab's own rows, Popular genre rows, Classics |

- **Hero.** Up to 6 titles not in your Library yet, from Trending this week
  and Trending on streaming, with **Ask** as the main button.
- **Top picks for you** needs 3 seeds.
- **Because you asked for X** needs an open ask of yours (your two newest).
- **Popular in your household** needs at least 2 active members and asks from
  the last 30 days. Without them these rows do not show.
- A title shows once in the first 8 rows, counting the hero and Trending on
  your services. A row keeps repeats rather than drop under its minimum.

When a row cannot fill, the next candidate moves up.

The ranked row keeps every rank. A title the metadata server cannot match to a card
shows as a name tile, and both seasons of a show are listed.

## Other rows

- **On your services.** Titles on the streaming services you pick, with a
  picker in the row's header. The pick is kept on this device.
- **Best new releases.** Well-rated recent titles. It was measured against
  Trending (movies 5 percent shared, shows 15 percent), so it stays as its own
  row.
- **Because you have X**, **More from a person**, **Genre picks for you**,
  **New on your services**, **Popular in your household**, decade rows and
  **Hidden gems** fill the tail.
- **Browse by genre** (All, Movies and Shows; not Music). One row of wide genre
  cards: each shows a backdrop from a popular title of that genre on your page,
  with the genre name in the corner. The genres your household has and asks
  for most come first, then the most popular ones, up to 16. No two cards share
  a backdrop, and Kid accounts see only kid-safe genres. Under All a genre shows
  once, as the kind with more of it. A card opens the same genre page as the
  **Genres** picker. The row comes from the page Explore already built (no new
  outside calls) and loads after the first rows, after row 6, never next to
  another genre row. API: `GET /api/v1/explore/genre-row?type=`.

## Smart row titles

Tail rows take their title from a library of 585 short titles, in five layers.
The most specific layer that is true wins:

1. **Occasion.** Halloween, Valentine's, the holiday season, New Year, summer,
   Friday night, the weekend, or a row's release day.
2. **Time of day** in the person's time zone (early morning, morning,
   afternoon, evening, late night).
3. **Genre mix.**
4. **Taste.**
5. **Row reason**, the plain "why this row".

If none fits, the row keeps its plain title. Titles stay true to the row: a
genre word needs that genre on at least half the items, and "new" needs at
least half released in the last 90 days. A Kid account only gets titles flagged
for Kids. A person does not see the same title again for 14 days, and a row
keeps its title for the whole local day. The fixed head rows (the charts, Top
picks, Best new releases, Because you have X and Because you asked for X, and
the tab's own rows) keep their title and get a short factual subtitle instead.

The occasion and time of day also nudge the order a little (a late-night
thriller row moves up a place or two, never more, and never for Kid
accounts). They never change which titles a row holds.

The owner sees how often each layer was shown, opened and asked from under
**Settings > This app** (`GET /api/v1/explore/title-stats`).

## Music

The **Music** switch has its own rows: a new-album hero and genre chips, then
Just arrived, New from artists you have (singles count too), Your asks, Trending
artists, Top albums, New this week, Rising artists, Classic albums, Popular in
your country, anniversaries, live albums and EPs, debuts, collaborators,
decades, moods that follow the hour, soundtracks for the Library's movies and
shows, Because you have an artist, Fans of an artist, and Missing albums.
Albums are square and artists are round. Small Libraries get at least 4 rows,
and a cold start keeps 10 or more.

## Cards and badges

Each card carries at most two marks.

- **Freshness strip.** One strip from the server, picked in this order: New
  Episode (a Library show with an episode in the last 7 days), New Season
  (started in the last 14 days), Recently Added (in the Library for under a
  week), Trending, Coming (this week) or Premieres (a date), Season N coming, and
  New Release (a movie or album out in the last 30 days). Music cards show
  only Recently Added and New Release. The ranked row already shows its rank,
  so it drops its own Trending strip.
- **Library icon.** A solid round mark at the top right says whether the
  title is in the Library, partly there, downloading (with a progress ring),
  asked for or turned down. On narrow cards it is only the icon, with the full
  sentence as its tooltip and accessible name.
- Under **All**, a small pill at the top left says Movie, Show or Music.

Movie and show rows on Explore and Home use wide 16:9 cards with the title's
logo (or name) over the backdrop. The ranked row uses tall cards with the rank, and
albums and artists keep their square and round shapes.

### Hover and touch

- On a wide card, resting the pointer for 0.4 seconds grows the card in place
  (inward at the edge of a row), then shows an info panel and the trailer.
  Every grown card has the same width (414 px), whatever row it is in.
- Keyboard focus opens the card after the same delay, and Esc closes it.
- On touch, a tap opens the title page. This follows Netflix on phones, where
  a tap on a poster goes straight to the title's details; there is no hover
  to grow a card from.
- With Reduce motion on, cards do not grow. Nothing opens while a row is being
  dragged.
- Rows also scroll by dragging with the mouse, with a glide on release.
- Trailers in these cards are covered in [Trailers](trailers.md).

## New-episode notices

People who asked for a show get one notice per show at most once an hour
(for example "3 new episodes of ..."). It is on by default and is a person's
own choice under **My account > Alerts for you > New episodes of shows I asked
for**. It reaches them on their Wishlist, and by email or Discord when the
owner has set those up; the owner can also route the `new_episodes` moment
through [Alerts](alerts-and-health.md).

## Asked by

A title page can say who asked for the title: "You asked", "Asked by Ana",
"Asked by Ana and Leo" or "Asked by Ana and 2 others". Cards stay factual and
show no names. The server trims the list before it is sent:

| Reader | Sees |
| --- | --- |
| Owner, admins and managers | Everyone who asked. |
| A member | Themselves, plus askers who chose to show their name, when the household allows it. |
| A Kid account | Only themselves. |

Two settings control it:

- **Show who asked** (household) under **Settings > Request rules**: **Owner,
  admins and managers only** (the default) or **Everyone in the household**.
- **Show my name on my asks** (personal) under **My account**, for people who may
  ask. It only has an effect when the household chose "Everyone in the
  household". The owner, admins and managers always see your name.

## Kid accounts

Kid accounts get the same rows, filtered twice by their age limit: when the
rows are built and again when the answer is sent. A title with no rating for
the region is hidden. See [Kid accounts](kid-accounts.md).
