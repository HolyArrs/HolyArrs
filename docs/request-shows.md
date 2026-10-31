# Adding and asking for shows

This page covers how shows get into HolyArrs: adding them yourself, keeping seasons and episodes up to date, and asking for them on the Requests page.

Searching for episodes comes in a later version. Until then HolyArrs adds shows, tracks every season and episode with its air date, and shows what is missing; it does not fetch episodes yet.

## A folder for shows

Shows live in their own Library folder. Open **Settings, Library folders**, press **Add a folder**, set **Holds** to **Shows** and type the path, for example `/shows`.

## Add a show

1. Open **Search** and press **Shows** above the search box (the page remembers it as `?kind=show`).
2. Type a show name, or paste an id: `tmdb:`, `tvdb:`, `tvmaze:` or `imdb:`.
3. Press **Add** on the show you want.
4. Pick:
   - **Library folder** and **Pick rules**.
   - **Seasons to keep up to date**: every season, only new episodes, the latest season, or none for now. Specials stay off unless you turn them on.
   - **New seasons**: keep seasons that come out later up to date, or leave them off.
   - **Episode order**: season and episode (most shows), by air day (talk shows and news) or one running number (anime).
   - **Details from**: TMDB or TVMaze. Neither needs a key.
   - **Keep up to date** for the whole show.
5. Want every episode in HD and in 4K? Tick **Also keep a second Version**. It suggests the name 4K and the 4K Pick rules. The 4K Version gets its own Pick rules and its own folder: in the same Library folder, the show folder gets `[4K]` after its name.
6. Press **Add to Library**.

## Several Versions (HD and 4K)

A Version is one copy of the show you want to keep. Every episode has its own place (a Slot) in each Version, so the HD and the 4K copy of an episode never replace each other.

- The **Versions** card on the show page lists each Version with its Pick rules and folder. **Add a Version** adds one more; **Change** renames it, gives it other Pick rules or turns its Keep up to date off; **Remove** stops keeping it (files on disk are not touched).
- A Version is not removed while it still has files, while a download or import for it is under way, or while a finished download for it still waits to be imported (also one waiting in Needs a decision).
- A Version's name also names its folder, so it is plain text of up to 40 characters, without `/ \ : * ? " < > | [ ]`, invisible characters or a final dot, and not a name Windows keeps such as `CON`. A name whose folder another show in the same Library folder already uses is refused.
- A Version takes Pick rules for movies and shows; music Pick rules are refused.
- The main Version cannot be removed. **Edit** and moving the show to another Library folder change the main Version only.
- Each episode shows one state per Version, for example "Main: In library" and "4K: Missing".
- An episode that airs later gets a Slot in every Version, each wanted from its air date.

## The show page

Open a show from the Library. It shows:

- How many aired episodes are in the Library, and the next air date.
- The **Why** line: each episode counts as missing once it has aired.
- **Seasons**, each with a line such as "2 of 3 aired in the Library · 1 missing · next 4 Jan". Open a season to see its episodes, with their air dates and state (Missing, Not out yet, In library), and why an episode is not looked for (for example "Keep up to date is off for this season").
- A **Keep up to date** switch for the show, for each season and for each episode. Turning a season on turns its episodes on; turning the show off keeps the season and episode switches as they were.
- **Edit** changes the Pick rules (of the main Version), the episode order, new seasons and the details source. Switching the details source finds the show there; episodes and their files stay.
- **Refresh** reads seasons, episodes and air dates again. HolyArrs also does this by itself every 12 hours.

Members can open a show page too, but only see it; the switches and buttons are for the owner and admins.

## The Library

The Library holds movies and shows together. Use **Everything**, **Movies** or **Shows** at the top, then **Missing**, **Not out yet**, **Better copy wanted** or **In library**. A show counts under a filter when any episode it keeps up to date is in that state.

## Ask for a show (members)

1. Open **Search**, press **Shows** and find the show.
2. Press **Ask**. Pick **The whole show** (every season but specials, and seasons that come later) or **Only some seasons** and tick the seasons you want.
3. The show lands on your requests as **Waiting for approval**, with the seasons you asked for.

If someone already asked for the same seasons, you join their ask and hear the same news. If their ask is still waiting and you want more seasons, the ask grows to include yours. A show already in the Library shows **Ask for more**: you can ask only for seasons it does not keep up to date yet.

**Weekly limit.** Shows have their own weekly limit, apart from movies and music. A show ask counts as one, whatever its seasons; asking for more seasons of a waiting ask is one more. The default is in **Settings, Requests** (**Show asks**, 0 is no limit); one person's own show limit is on the **Users** page. Joining someone else's ask never counts.

## Approve or turn down (owner and admins)

On the **Requests**, show asks carry a **Show** chip and say which seasons were asked for. Press **Approve**, pick a show Library folder and Pick rules (or keep the Request defaults), add a note if you like, and press **Approve and add**. HolyArrs adds the show and keeps exactly the asked seasons up to date; for a show already in the Library it turns those seasons on. **Turn down** asks for a short reason, which the people who asked see.

People who may auto-approve get their show asks approved straight away.

The default show folder and Pick rules for approved asks, and the default show limit, live in **Settings, Requests**.

## Being told

Messages on the Requests page tell the people who asked when their show is approved or turned down, and when it is ready: every aired episode of the seasons they asked for is in the Library. Alerts (Settings, Alerts) can send the same to Discord, email or a webhook.

## Who asked

The show page has a Requests card naming who asked, for which seasons, and who approved. **Downloads, History** lists the same, for example "max asked for Lantern Street (2023), seasons 1 and 2".
