---
title: Movie editions
description: Ask for a certain cut of a movie, such as the Director's Cut, or keep several cuts side by side.
level: Start
---

Get a certain cut of a movie, or keep more than one. Each Version of a movie asks for its own edition, so one movie can have a Theatrical HD, a Theatrical 4K and a Director's Cut HD Version.

## Ask for an edition

1. Open the movie.
2. Add or edit a Version.
3. Set its edition and save.

You should see the edition on the Version. Searches for that Version look for that cut, and imports check it again before the file is placed. A Version with no edition takes any cut.

Upgrading HolyArrs does not relabel existing Versions or files as theatrical.

## When the edition is unclear

A finished download with an unknown or conflicting edition waits on [Downloads](downloads-movies.md#settle-a-movie-in-review) under **Review**.

1. Check the file yourself.
2. Press **Set edition** and pick the cut.
3. Press **Save and try again**.

Your choice is recorded. It skips no other check.

## Names for your media server

New files with a known edition get the Plex edition tag in the name, for example `{edition-Director's Cut}`. Existing files keep their names until you [rename them](organise-movies.md#rename-files).

Plex has its own rules for showing editions. Jellyfin and Emby group versions differently and may not show the tag as a separate edition.

## If it does not work

**A download keeps stopping for an edition.**
Cause: the release name does not say which cut it is.
Fix: set the edition yourself on the Review row.

**My media server shows one movie, not two cuts.**
Cause: the media server's own edition support.
Fix: check its documentation for editions or versions.

## Related

- [Movie series](collections.md)
- [Organise movies](organise-movies.md)
- [Movie downloads](downloads-movies.md)
