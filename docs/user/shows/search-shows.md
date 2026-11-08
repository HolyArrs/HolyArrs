# Find episodes for your shows

You want the episodes of your shows to show up without chasing them. HolyArrs looks for them on its own, and you can also search or pick a Release yourself. Movies work the same way, see [Search for movies](../movies/search-movies.md). Only the owner and admins see these screens.

## How HolyArrs thinks about episodes

Every episode has its own place in each Version. So one episode can be in the Library while the next one is still missing.

A Release can hold:

- one episode, like `Copper.Valley.S01E02.1080p.WEB-DL`
- several episodes, like `S01E02E03` or `S01E02-E04`
- a whole season, like `Copper.Valley.S01.1080p.BluRay`. This is a season pack.
- several seasons, like `S01-S03`
- an air date, for talk shows and news: `The.Late.Lamp.Show.2025.03.14`
- a running number, for anime: `[Group] Starfall Kitchen - 112 [1080p]`
- a special: `S00E03`, `S04E00` or `OVA 02`

HolyArrs matches a Release to the show by its name, or by another name the show is known by. Then it matches the episodes by season and episode, air date or running number. A show Release never matches a movie, and a movie Release never matches a show.

A season pack can fill many episodes. It goes to your Downloader once, as one download.

## Let HolyArrs look by itself

An episode is only looked for when the show, its season, the episode and the Version all have **Keep up to date** on.

- **Feed check.** HolyArrs reads the TV categories of your Sources. A new Release that fits a missing episode, or one that wants a better copy, is judged by the show's Pick rules.
- **Nightly search.** When several episodes of one season are missing, HolyArrs searches the whole season. A single missing episode gets its own search. Episodes that have not aired are never searched.
- **Better copies.** An episode whose copy is below **Good enough** keeps looking.

## Search from the show page

1. Open the show.
2. Press **Search now** at the top to search every season that still needs episodes. HolyArrs picks the best Releases.
3. Or open one season and press its **Search now**. If the whole season is missing, it takes a season pack. Otherwise each episode is searched on its own.
4. Or press **Search now** on one episode. It says **Find a better copy** when the copy you have is below Good enough.

## Choose a Release yourself

1. Press **Choose a release** on a season or an episode.
2. HolyArrs searches every Source and picks nothing. A card opens with every Release found, best first.
3. Each Release shows its verdict (**Would be chosen**, **Passed over**, **Turned away by the rules**, **Turned down**), the episodes it holds, its Source, size, seeders, points and rank, and the reasons.
4. Press **Choose** and confirm. For a season pack, the box says which episodes it fills.

The box also warns you when the Release fails the rules (the button then says **Choose anyway**), when the episode has not aired, or when you already have a good enough copy.

**Turn down** on an episode's list turns the Release down for that episode only. It can still be judged for other episodes. Turn down on a season's list turns it down for every episode it holds.

## Shows with several Versions

A show kept in HD and 4K (see [Several Versions](#shows-with-several-versions)) is searched once per Version, each with its own Pick rules. An HD season pack fills only HD episodes. The 4K Version needs its own 4K pack.

**Search now** on an episode looks for every Version. **Choose a release** on an episode lists Releases for the main Version. A season's list names the Version next to each Release.

## Season pack rules

- A season pack is only taken once every episode of the season has aired, or airs within a day. Until then it is turned away with a reason like "not every episode of season 4 has aired".
- By default a season pack is only taken when every episode in it would be new or better. If not, the missing episodes are searched one by one.
- When a season pack and single episodes are equally good, the season pack wins.
- Releases with more than one season are turned away unless you allow them.

## Settings

Open **Settings > Search**.

| Setting | What it does | Default |
| --- | --- | --- |
| Search for missing episodes too | The nightly search and **Search all missing** also cover aired episodes, a season at a time when several are missing. | On |
| A season pack is taken when | **Every episode in it would be new or better**, or **at least one would be**. With the second, the pack fills only the episodes it improves. | Every episode |
| Season pack or single episodes | **Best copy first** (a pack wins a tie), or **a season pack whenever one passes the Pick rules**. | Best copy first |
| Take Releases with several seasons | On lets a Release like seasons 1 to 3 fill every missing episode it holds. | Off |

The feed check interval and the number of nightly searches are shared with movies, see [Search for movies](../movies/search-movies.md#settings).

## Where to see what is going on

- **Library > Held** and **Library > Turned down** name the show and episode of each Release, like "Copper Valley S01E02". **Take off the list** lets HolyArrs pick a turned-down Release again. **Stop holding** sends the episode back to looking.
- The **Searches** block on Home names the show and the episode or season, like "Searching for Copper Valley S01E02".

## Troubleshooting

**An episode is never searched.** Check that Keep up to date is on for the show, the season, the episode and the Version. Check that the episode has aired.

**A season pack is turned away.** Open **Choose a release** on the season and read the reasons. Usually not every episode has aired, or some episodes already have a copy as good. Change **A season pack is taken when** if you want packs to fill only the gaps.

**A Release is turned away as early or fake.** A Release posted before the last episode it holds aired, or far too small for its length, is refused. See [Early and fake Releases](../movies/search-movies.md#fake-and-early-releases).
