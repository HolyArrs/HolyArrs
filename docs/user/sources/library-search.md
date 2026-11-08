# Find a title in the Library

Open Library and type in **Find in the Library**. This searches the titles
already added to HolyArrs. To find something to add, use the Search page.

The name box finds movie, show and artist names, their other names from
metadata, and years. Latin accents are optional: `amelie` finds `Amélie`.
You can type the beginning of a word, and several words narrow the match.
The existing name-substring matches also work. A short pause while typing
lets HolyArrs finish the search.

The kind and state buttons still apply. For example, choose Shows and
Missing, then type part of a show's name. The button counts describe the
Library before the name filter. Clear the name box to see that view again.

Only enabled parts are shown, except in safe mode. Removed titles never
appear in the results. Searching does not add a title, start a download or
change a file.

The name box accepts at most 256 characters and 16 words. Search symbols
such as `OR` and `*` do not act as commands. If a search fails, its message
appears on the page; clear or change the name and try again.

For scripts, `GET /api/v1/titles?q=amelie` returns matching titles with their
Versions and Slots. This API matches word prefixes in local names, aliases
and years; every word must match. Empty input lists the visible Library.
See [The API for other apps](api.md) for sign-in and keys.
