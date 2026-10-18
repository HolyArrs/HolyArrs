# Title page actions: before → after

Every action, dialog and confirmation on the movie, show and artist pages, and where it lives on the Apple title page. Dialogs, confirmations and API calls are unchanged; only the place you press moved. "Hero" means the round action row under the title. Admin = `manage_library`.

## Movie (`routes/library/[id]/+page.svelte`)

| Before | After |
|---|---|
| Search now (red button) | Hero: **Search now** (primary, white) |
| Choose a release (button) | Hero ⋯ **More › Choose a release**; also a button in the empty Releases section |
| Refresh | Hero ⋯ **More › Refresh from TMDB** |
| Edit → edit dialog (Pick rules, gate, extra days, Keep up to date) | Hero: **Edit** (same dialog) |
| Rename files → rename preview dialog | Hero **Library › Rename files** (same dialog, RenamePreview) |
| Delete → delete dialog with plan, files / Never-add options | Hero **Library › Delete movie** (same dialog and plan) |
| Keep up to date switch | Hero: **Keep up to date** on/off button (same API call) |
| Add a Version → dialog | Hero **Library › Add a Version**, and the button in **Versions and files** |
| Remove a Version (confirm) | **Versions and files** row, same confirm |
| Mark failed (confirm) on an active download | Download card under the hero actions, same confirm |
| Release Choose → confirm / Choose anyway | **Releases** section, same dialog |
| Release Turn down → reason dialog | **Releases** section, same dialog |
| Version picker and Back / More pages for Releases | **Releases** section |
| Copy controls (checked edition, protect / unprotect, keep and add a Version, use an existing video) | **Copies and Version criteria** card (CopyControls, unchanged) |
| Subtitles (look for, languages, sync fix, turn down, choose) | **Subtitles** card (SubtitlePanel, unchanged) |
| History: All N rows link | **History** in the side column; also Hero ⋯ **More › All history** |
| Cast and directors: Show credits button, links to people | **Cast and crew** shelf (loads by itself), each person links to the people page |
| Read more on the description | Hero **Show more** |
| Back to Library link | Hero breadcrumb **Movies › Title (Year)** |

## Show (`lib/ShowPage.svelte`)

| Before | After |
|---|---|
| Search now / Refresh / Edit (dialog) / Keep up to date switch | Hero: **Search now**, **Keep up to date**, **Edit**; ⋯ **More › Refresh seasons and episodes**, **All history** |
| Rename files (dialog) | Hero **Library › Rename files** |
| Delete show (ShowDeleteDialog) | Hero **Library › Delete show** |
| Season: open by expanding a row | **Seasons**: SegmentedControl (up to 4 seasons) or a season menu |
| Season: Search now / Choose a release / Delete season / Keep season switch | Row under the season picker, same calls and dialogs |
| Episode: Search now or Look for a better copy, Choose a release, Subtitles (dialog), History link, Delete (dialog), Keep switch | Each episode card's ⋯ menu (Keep is a checked item, same call) |
| Choose a release list (ShowReleases, Choose / Turn down) | Opens above Seasons, unchanged |
| Versions (add, change, remove with confirm) | **Versions** card (ShowVersions, unchanged) |
| Copy controls, Subtitles, Wishlist, History | Same components, in the main and side columns |

## Artist (`lib/ArtistPage.svelte`)

| Before | After |
|---|---|
| Search now / Refresh / Edit album kinds / Keep up to date | Hero: **Search now**, **Keep up to date**, **Edit**; ⋯ **More › Refresh albums**, **All history** |
| Rename files / Delete artist | Hero **Library ›** menu, same dialogs |
| Ask for albums (members) | Hero: **Ask for albums** (same AskArtistDialog) |
| Albums list: filter pills, per-album Search and Keep switch, album links | **Albums** card, unchanged |

## Members

Same page, admin buttons hidden. Movie and show heroes show the status in member words ("Ready to watch", "Requested", "Missing", "Downloading n%"). Artist keeps **Ask for albums**.

## Not shown

- **Open in Jellyfin/Plex**: the title API has no per-title media server link, so it is left out.
- Not on these pages before, so not moved: EditionDecision (Downloads › Decide), ReleaseList (Held / Turned down), FilmographyList (people pages), WishEditor (inside SubtitlePanel and Settings).
