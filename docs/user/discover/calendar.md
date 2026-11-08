# Calendar

Use the Calendar to see when the movies, shows and music in your Library come out. Open it at `http://your-server:6262/calendar`.

## See what is coming

1. Open **Calendar** from the menu.
2. Pick **Movies**, **TV shows**, **Music** or **All** at the top.
3. Use the arrows to move through the weeks. Press **Today** to come back.
4. Press a title to open its page.

To jump to a day, press **Pick a day** and choose a date. You can also link to a month: `http://your-server:6262/calendar?month=YYYY-MM` opens on that month. Use the year and the month number, such as 03 for March.

## Pick which dates you see

The **Which dates** menu decides what fills the calendar.

- **My follows** shows what you follow. Asking for a title follows it for you.
- **Everyone's follows** shows what anyone on this server follows, and each row says who. It only appears when more than one person uses the server.
- **Whole Library** shows every date in the Library, followed or not.

If you follow nothing yet, the calendar opens on **Whole Library** and tells you how to follow.

To follow a title, press **Follow** on its row. Press **Unfollow** to stop. In **My follows** the row leaves the list.

## Settings on this page

- **Cards** or **List** changes how each day is drawn. Your pick is kept in the address, so a shared link opens the same way.
- **Show month** and **Hide month** turn the small month at the side on and off.
- A movie shows one date: the day you can watch it at home, or its cinema date if that is all there is.
- Each row has a coloured dot: **In the Library**, **Missing** or **Coming up**.

## Troubleshooting

- **Nothing coming up.** Your Library has no dates yet. Add a movie, show or artist and dates show up.
- **Nothing you follow here.** You follow nothing with a date in this stretch. Press **Show the Whole Library**, or follow more titles.
- **Calendar could not be loaded.** HolyArrs could not answer. Reload the page. If it keeps happening, open `http://your-server:6262/status` to see what is wrong.
