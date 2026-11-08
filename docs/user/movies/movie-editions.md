# Movie editions

You want a certain cut of a movie, such as the Director's Cut, or you want more than one cut side by side. Each Version of a movie can ask for its own edition. For example, one movie can have a Theatrical HD Version, a Theatrical 4K Version and a Director's Cut HD Version.

## Ask for an edition

1. Open the movie.
2. Add or edit a Version.
3. Set its edition.

Searches for that Version look for the edition you set. Imports check it again before the file is placed. A Version with no edition takes any cut.

When you upgrade HolyArrs, your existing Versions and files are not relabelled as theatrical.

## When the edition is unclear

If a finished download has an unknown or conflicting edition, it waits for you on [Downloads](downloads-movies.md) under **Review**. Check the file yourself, then use **Set edition** and **Save and try again**. Your choice is recorded. It does not skip any other check.

## Names for your media server

New files with a known edition get the Plex edition tag in the name, for example `{edition-Director's Cut}`. Existing files keep their names until you rename them. See [Organise your movies](organise-movies.md#rename-files).

Plex has its own rules for when it shows editions. Jellyfin and Emby group versions differently and may not show the tag as a separate edition.

## Troubleshooting

- **A download keeps stopping for an edition.** The release name does not say which cut it is. Set the edition yourself on the Review row.
- **My media server shows one movie, not two cuts.** Check your media server's own edition support.
