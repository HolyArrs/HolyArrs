# Adding and asking for music

This page covers how music gets into HolyArrs: adding an artist yourself, choosing which albums and which Pressing of each album to keep up to date, and asking for an artist or a few albums on the Requests page.

Searching for music Releases comes in a later version. Until then HolyArrs adds artists, fills in every album, Pressing and track from MusicBrainz, and shows what is missing; it does not fetch music yet, so a music ask is never Ready to listen until that version.

## A folder for music

Music lives in its own Library folder. Open **Settings, Library folders**, press **Add a folder**, set **Holds** to **Music** and type the path, for example `/music`.

## Settings, Music

- **Albums to keep up to date**, **New albums**, **Album kinds**, **Extra kinds** and **Pressings that count** are what a new artist starts with. The add dialog can change them for one artist.
- **Album kinds**: the main kinds an artist includes (Album, EP, Single, Broadcast, Other). The default is Album and EP.
- **Extra kinds**: an album with an extra kind, such as Live or Compilation, is only included when that kind is ticked. **No extra kind (studio)** covers ordinary albums. The default is that one alone.
- **Pressings that count**: an album is included when at least one of its Pressings has a ticked status. The default is Official.
- **Which Pressing an album follows**: a Pressing is one edition of an album (a CD, a vinyl, a digital edition, a deluxe edition with bonus tracks). The tracks HolyArrs keeps are the tracks of one Pressing. **The usual one** (default) picks the track count most Pressings share, then a digital one, then the earliest; **The most tracks** and **The earliest** are the other rules.
- **MusicBrainz address**: leave it empty for musicbrainz.org, or point it at a mirror of your own. HolyArrs asks MusicBrainz at most once a second and keeps answers for the hours you set.

## Add an artist

1. Open **Search** and press **Music** above the search box (the page remembers it as `?kind=music`).
2. Type an artist name, or paste `mbid:` with the artist's MusicBrainz id.
3. Press **Add** on the artist you want.
4. Pick:
   - **Library folder** and **Pick rules**.
   - **Albums to keep up to date**: every album, albums still to come, albums without files, the latest album, the first album, or none for now.
   - **New albums**: keep albums that come out later up to date, only those released after today, or leave them off.
   - **Album kinds**, **Extra kinds** and **Pressings that count** for this artist.
   - **Keep up to date** for the whole artist.
5. Press **Add to Library**. HolyArrs reads the albums straight away; the Pressings and tracks of the included albums follow within a few seconds per album.

## The artist page

Open an artist from the Library. It shows:

- How many tracks are in the Library, how many are missing and how many are not out yet.
- The **Why** line: each album is missing once its release date has passed.
- **Albums**, newest first, each with its year, its kinds (for example "Album · Live"), its state (Missing, Some tracks in, In library, Not out yet, Not kept up to date, Not included) and a line such as "3 of 12 tracks in the Library · 9 missing". **Not included** lists the albums the album kinds leave out; they are kept as rows so changing the kinds brings them in.
- A **Keep up to date** switch for the artist and for each included album.
- **Edit album kinds** changes new albums, album kinds, extra kinds and Pressings that count; a refresh then brings in the albums the new kinds include.
- **Refresh** reads albums, Pressings and tracks again. HolyArrs also does this by itself every 12 hours.

## The album page

Open an album from the artist page. It lists its tracks (disc and number, name, length and state) and every Pressing MusicBrainz lists, and says which Pressing the tracks follow and whether the rule or a person picked it. The owner and admins can pick another Pressing and press **Use this Pressing**: tracks on both Pressings keep their files, new tracks are added, and tracks the new Pressing lacks are removed unless a file holds them. A Pressing picked by hand stays picked for as long as MusicBrainz lists it.

Members can open artist and album pages too, but only see them; the switches and buttons are for the owner and admins.

## The Library

The Library holds movies, shows and artists together. Use **Everything**, **Movies**, **Shows** or **Music** at the top, then **Missing**, **Not out yet**, **Better copy wanted** or **In library**. An artist counts under a filter when any of its tracks is in that state.

## Ask for music (members)

1. Open **Search**, press **Music** and find the artist.
2. Press **Ask**. Pick **Every album** (every album of the usual kinds, and albums that come out later) or **Only some albums** and tick the albums you want. Albums already in the Library, and albums the artist in the Library leaves out, cannot be ticked.
3. The ask lands on your requests as **Waiting for approval**, with a **Music** chip and the albums you asked for.

If someone already asked for the same albums, you join their ask and hear the same news. If their ask is still waiting and you want more albums, the ask grows to include yours. An artist already in the Library shows **Ask for more**, and its page has **Ask for albums**: you can ask only for albums it does not keep up to date yet.

**Weekly limit.** Music has its own weekly limit, apart from movies and shows. A whole-artist or album ask counts as one, whatever its albums; asking for more albums of a waiting ask is one more. The default is in **Settings, Requests** (**Music asks**, 0 is no limit); one person's own music limit is on the **Users** page. Joining someone else's ask never counts.

## Approve or turn down (owner and admins)

On the **Requests**, music asks say which albums were asked for. Press **Approve**, pick a music Library folder and Pick rules (or keep the Request defaults), add a note if you like, and press **Approve and add**. HolyArrs adds the artist with only the asked albums kept up to date. An asked album of a kind the defaults leave out (a single, a live album) is included for that artist so it can be kept. For an artist already in the Library, HolyArrs turns the asked albums on. **Turn down** asks for a short reason, which the people who asked see.

People who may auto-approve get their music asks approved straight away.

The default music folder and Pick rules for approved asks, and the default music limit, live in **Settings, Requests**.

## Being told

Messages on the Requests page tell the people who asked when their music is approved or turned down, and when it is ready to listen to: every track of every album they asked for is in the Library. Alerts (Settings, Alerts) can send the same to Discord, email or a webhook.

## Who asked

The artist page has a Requests card naming who asked, for which albums, and who approved. **Downloads, History** lists the same, for example "ivy asked for Moss Choir, Fern Hymns".
