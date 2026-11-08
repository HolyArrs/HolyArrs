# Explore

Use Explore to find something to ask for or add. The page is built for you, from what is in your Library and what you asked for. Open it at `http://your-server:6262/explore`.

Explore never plays anything. It reads your Library, your asks and the charts from the [metadata server](../metadata/metadata-service.md). It does not track what anyone watches.

## Find something to ask for

1. Open **Explore** from the menu.
2. Pick **All**, **Movies**, **Shows** or **Music** at the top.
3. Scroll. More rows load as you go.
4. Press a card to open the title. Press **Ask** or add it from there.

Your type pick is kept in the address (`?type=`) and remembered on this device. **Music** only shows when the Music part is on.

## How your page is built

Each person gets their own page. Two people in the same house see different rows in a different order.

- **The first rows** open fast and show each title once. These include the charts, **Top picks for you**, **Because you have** a title, **Best new releases**, **Because you asked for** a title and **Popular in your household**.
- **More rows** load as you scroll. The more titles you have and ask for, the more rows you get: up to 8 with fewer than 3, up to 20 with 3 to 9, and up to 40 with 10 or more.
- **The order** changes a little each day and stays the same all day.
- **A new server** still gets a full page. With an empty Library and no asks, you see the charts and popular genre and decade rows.

Some rows need something before they show:

- **Top picks for you** needs at least 3 titles in your Library or asks.
- **Because you asked for** needs an open ask of yours.
- **Popular in your household** needs at least 2 people on the server who asked for things lately.

## Other rows

- **On your services.** Titles on the streaming services you pick in the row's header. The pick is kept on this device.
- **Finish the collection.** Movies missing from a collection you started.
- **Returning soon.** Shows in your Library with a new episode coming in the next two months.
- **Browse by genre.** Wide genre cards. The genres your house has and asks for most come first. A card opens that genre's page.
- **Hidden gems**, decade rows, and rows about the people in your favourite titles fill the rest.

The **Music** type has its own rows, such as new albums from artists you have, trending artists and top albums. Albums are square and artists are round.

## Row titles

Lower rows can get a short title that fits the moment, such as the time of day or a holiday. A row only gets a title that is true for it. A Kid account only gets titles made for kids. The first rows keep their plain titles.

The table that shows the owner how these titles do is held back. It is off in this release.

## Cards

- A strip on the card can say **New Episode**, **New Season**, **Recently Added**, **Trending**, **Coming** or **New Release**.
- A round mark at the top right says if the title is in the Library, partly there, downloading, asked for or turned down.
- Under **All**, a small label says Movie, Show or Music.

On a computer, rest the pointer on a card and it grows to show more and play the trailer. Keyboard focus does the same, and Esc closes it. On a phone, a tap opens the title page. With Reduce motion on, cards do not grow. See [Trailers](../metadata/trailers.md).

## Who asked

A title page can say who asked for it, like "You asked" or "Asked by Ana and 2 others". Press the line to see everyone and when. Cards never show names.

## Kid accounts

Kid accounts see fewer rows, and every row is checked against their age limit. A title with no rating for your region is hidden. See [Kid accounts](../accounts/kid-accounts.md).

## New episodes

If you asked for a show, you get a notice when new episodes arrive, at most once an hour per show. Turn it off under **My account**. The owner can also send it through [Alerts](../alerts/alerts.md).

## Troubleshooting

- **The page has few rows.** Add or ask for a few titles. Rows grow with what HolyArrs knows about you.
- **A row I saw yesterday is gone.** Rows only show when they have enough titles. It comes back when it fills again.
- **Music is missing.** Turn the Music part on in Settings.
- **Cards show only names.** The metadata server could not match the title. It still counts in the row.

For new releases and what is popular on streaming, see [New & Popular](popular.md). For dates, see [Calendar](calendar.md). For actors and directors, see [People](people.md).
