# Searching for shows

This page covers how HolyArrs finds Releases for the episodes of your shows: by itself, when you press a button, and when you choose a release yourself. Movies work the same way and are covered in [Searching for movies](search-movies.md). Only the owner and admins (people who may manage the Library) see any of this; members never do.

## Episodes, seasons and season packs

Each episode of a show has its own place in each Version, so HolyArrs looks for every episode on its own: one can be in the Library while the next is still missing.

A Release can hold:

- **one episode**, like `Copper.Valley.S01E02.1080p.WEB-DL`;
- **several episodes**, like `S01E02E03` or `S01E02-E04`;
- **a whole season**, like `Copper.Valley.S01.1080p.BluRay` (a season pack);
- **several seasons** or the whole show, like `S01-S03`;
- **an air date**, for talk shows and news: `The.Late.Lamp.Show.2026.03.14`;
- **a running number**, for anime: `[Group] Starfall Kitchen - 112 [1080p]`;
- **a special**: `S00E03`, `S04E00` or `OVA 02`.

HolyArrs reads these names and matches each Release to the show by its name (or another name the show is known by), then to the episodes by season and episode, air date or running number. A Release for a show never matches a movie, and a movie Release never matches a show.

One season pack can fill many episodes. It is sent to your Downloader once, as one download for all the episodes it fills.

## How HolyArrs looks by itself

- **Feed checks.** The same feed check that serves movies also reads the TV categories of your Sources. A new episode Release that fits an episode still missing (or wanting a better copy) is judged by the show's Pick rules and chosen or held.
- **The nightly search.** It now covers episodes too. When several episodes of one season are missing, HolyArrs searches the season as a whole; a single missing episode gets its own search. Episodes that have not aired yet are never searched.
- **Find a better copy.** An episode whose copy is below **Good enough** keeps looking, like a movie.

An episode is only looked for when the show, its season, the episode and the Version all have **Keep up to date** on.

## Season packs: the rules

- A season pack is only taken once **every episode** of the season has aired (or airs within a day). Until then it is turned away with the reason "not every episode of season 4 has aired".
- By default a season pack is only taken when **every episode in it would be new or better**. If some episodes already have a copy as good, it is turned away and the missing episodes are searched one by one. You can change this in Settings > Search.
- When a season pack and single episodes are equally good, the season pack wins. A better single episode still beats a worse season pack, unless you change this in Settings > Search.
- Releases holding **more than one season** are turned away unless you turn them on in Settings > Search.

## The show page

- **Search now** at the top searches every season that has an episode kept up to date and still looking. HolyArrs picks the best Releases by itself.
- Open a season to see its **Search now** and **Choose a release** buttons. A season search takes a season pack when the whole season is missing; otherwise, and for the episodes the season search found nothing for, each episode is searched on its own.
- Each episode that can use a copy has **Search now** (or **Find a better copy**, when its copy is below Good enough) and **Choose a release**.

**Choose a release** searches every Source but picks nothing. A card opens above the seasons with every Release found, best first, each with its verdict (**Would be chosen**, **Passed over**, **Turned away by the rules**, **Turned down**), the episodes it holds, a short line (Source, size, seeders, points and rank) and the reasons that decided it.

**Choose** asks you to confirm first. For a season pack chosen from a season's list, the box says which episodes it fills ("One download fills S01E01, S01E02"). The box also warns when the Release failed the rules (the button then says **Choose anyway**), when the episode has not aired yet, or when the episode already has a good enough copy.

**Turn down** on an episode's list turns the Release down **for that episode only**. HolyArrs never picks it again for that episode. A season pack turned down for one episode stays on the Turned down list for that episode, and is still judged for the other episodes. Turn down on a season's list turns the Release down for every episode it holds.

## Shows with several Versions

A show kept in HD and 4K (see [Several Versions](request-shows.md#several-versions-hd-and-4k)) is searched once per Version, each with its own Pick rules. A season search runs for each Version, and the nightly search queues one per Version. A season pack chosen for the HD Version fills only HD episodes; the 4K Version needs its own 4K pack. **Search now** on an episode looks for every Version's copy; **Choose a release** on an episode lists Releases for the main Version, and a season's list names the Version beside each Release.

## Early and fake Releases

A Release posted before the last episode it holds aired, or far too small for it, is turned away like for movies; see [Early and fake Releases](search-movies.md#early-and-fake-releases).

## Held and Turned down

The **Held** and **Turned down** lists under **Library** show the show and the episode for each Release, for example "Copper Valley S01E02". **Take off the list** lets HolyArrs pick a turned-down Release again; **Stop holding** sends the episode back to looking.

## Searches on Home

The **Searches** block on Home names the show and the episode or season of each search, for example "Searching for Copper Valley S01E02" or "Copper Valley S02: searching episode by episode".

## Settings > Search

| Setting | What it does | Default |
| --- | --- | --- |
| Search for missing episodes too | The nightly search and **Search every missing one** also cover aired episodes, a whole season at a time when several are missing. | On |
| A season pack is taken when | **Every episode in it would be new or better**, or **at least one would be**. With the second, a pack fills only the episodes it improves; your other copies stay. | Every episode |
| Season pack or single episodes | **Best copy first; a season pack wins when they are equal**, or **a season pack whenever one passes the Pick rules**. | Best copy first |
| Take Releases with several seasons | Off: a Release with seasons 1 to 3, or a whole show, is turned away. On: it fills every missing episode it holds. | Off |

The feed check interval and the number of searches each night are shared with movies; see [Searching for movies](search-movies.md).
