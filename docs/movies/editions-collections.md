---
title: Editions and collections
description: Ask for a certain cut of a movie, keep several cuts side by side and see a collection together.
level: Start
---

Get the cut you want, keep more than one, or see a movie collection as a group.

Each Version of a movie asks for its own edition. One movie can have a Theatrical HD, a Theatrical 4K and a Director's Cut HD Version.

## Ask for an edition

1. Open the movie.
2. Add or edit a Version.
3. Set its edition and save.

You should see the edition on the Version. Searches for that Version look for that cut, and imports check it again before HolyArrs places the file. A Version with no edition takes any cut.

Upgrading HolyArrs does not relabel existing Versions or files as theatrical.

## See a collection

A collection is a movie series, such as every Dune film. HolyArrs groups movies by the collection their movie details list, never by names or folders.

1. Open **Library** and pick **Movies**.
2. Open the sort and filter panel and turn on **Group by collection**.

You should see each collection with its movies under it. To see one collection, pick it under **Collection** in the same panel.

## Remove a collection

:::caution
Deleted files move to the holding folder. HolyArrs removes them for good after the holding days (7 by default).
:::

1. Open the collection.
2. Pick the movies to remove and choose whether to delete their files.
3. Press **Review removal**. It lists every movie and file HolyArrs will touch.
4. Press **Confirm this exact selection**.

You should see each movie marked removed, skipped or failed.

Files stay by default. Files HolyArrs imported or adopted move to the holding folder, as when you [delete one movie](organise.md#delete-a-movie). HolyArrs never touches files it does not own, and never deletes the collection folder.

## Choices

### When the edition is unclear

A finished download with an unknown or conflicting edition waits under **Review** on [Movie downloads](downloads.md#settle-a-movie-in-review).

1. Check the file yourself.
2. Press **Set edition** and pick the cut.
3. Press **Save and try again**.

HolyArrs records your choice. It skips no other check.

### Names for your media server

New files with a known edition get the Plex edition tag in the name, for example `{edition-Director's Cut}`. Existing files keep their names until you [rename them](organise.md#rename-files).

Plex has its own rules for showing editions. Jellyfin and Emby group versions differently and may not show the tag as a separate edition.

### What a collection removal does

- **Something changed after the review**, such as a new file or a protected copy: HolyArrs refuses and asks you to review again.
- **A movie with a protected copy** is skipped, unless you also delete files and confirm the protected copy.
- **A failure** does not undo the others. Press **Review remaining movies** to try the rest again.
- **Outside lists** may add a removed movie again later, unless you add it to the Never-add list.

## If it does not work

**A download keeps stopping for an edition.**
Cause: the release name does not say which cut it is.
Fix: set the edition yourself on the Review row.

**My media server shows one movie, not two cuts.**
Cause: the media server's own edition support.
Fix: check its documentation for editions or versions.

**A collection removal says to review again.**
Cause: a file or protected copy changed after the review.
Fix: press **Review removal** again and confirm the new list.

## Related

- [Organise movies](organise.md)
- [Movie downloads](downloads.md)
- [Search for movies](search.md)
