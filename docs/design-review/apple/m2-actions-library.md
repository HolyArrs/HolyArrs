# M2 actions: Library, Search and People (before → after)

Every action, confirmation, filter, permission check, `to()` link and `data-testid` below stays. "Before" is the pre-M2 markup; "after" is the kit piece that now carries it.

## Library (`routes/library/+page.svelte`)

| Action | Before | After |
|---|---|---|
| Page title | "Media collection" eyebrow + h1 + "N titles in this view" + second "N titles" in the command bar | `PageHeader` "Library", subtitle "N titles" (once; "N titles match" while filtering) |
| Find in the Library (debounced `/titles?q=` index search, URL `q`) | search band input + "Clear filters" button | same `#find` input (label "Find in the Library"), search icon, tokens |
| Kind filter (all/movie/show/artist, counts, URL `kind`) | Type `<details>` menu, `group "Kind"` buttons | `Chip`s in `role=group "Kind"` with counts (aria-pressed) |
| Status filter (all/missing/not_out_yet/better/have, counts, URL `show`) | Status `<details>` with a select | `Chip`s in `role=group "Status"` with counts |
| Requested by (anyone/me/person, counts, "Me" disabled without id, URL `requester`) | `<details>` + select | kit `Menu` "Requested by" with checked items |
| Sort (name / newest year, URL `sort`) | bare select | kit `Menu` "Sort titles" |
| Layout (Posters / List, remembered in localStorage) | `.seg` group "Layout" | `SegmentedControl` "Layout" (radiogroup) |
| Remove one filter (requester, search), focus moves to next | "Active filters" pill buttons for all four | `Chip` with remove for requester and search; kind/status are cleared by picking "Everything"/"All" |
| Clear all filters | "Clear filters" always visible | one `Button` "Clear", only while a filter is active (also the action of the "Nothing matches" empty state) |
| Add a movie / show / artist (manage_library) | `+ Add` `<details>` nav "Library tools" | kit `Menu` "Add and Library tools"; each Add item shows only when its part (movies/shows/music) is on |
| Held files, Turned down releases, Missing subtitles, Rename files, Import a library | same `<details>` nav | same Menu, under a "Library tools" heading; Missing subtitles only when the subtitles part is on |
| Movie series, People, Subtitles, File codecs links (movies part on; last two manage_library) | filter-bar links | kit `Menu` "Browse" |
| Poster grid → title (`?returnTo=` current URL) | `.grid .item` link + `Poster` | `PosterCard` with `vtName=title-art-<id>` (poster grows into the title hero), state as `StatusPill` |
| List view | `<table class="list-table">` | `ul.list-table` of kit `ListRow`s (title, year · kind · progress, out-on date · keep up to date, state pills) |
| Preview panel (≥1280px), focus to heading, Close returns focus | text "Preview" buttons | `IconButton` "Preview <name>" (aria-pressed), `IconButton` "Close preview", kit `StatusPill`/`Button` inside |
| Scroll restore + Last opened (sessionStorage per kind and URL) | unchanged logic | unchanged; `data-testid="last-opened"` label kept on card and row |
| Requested by names | raw names joined | `peopleNames()` from `lib/plain.ts` (id-like names read "Someone in your household") |
| Approved request marker | `.chip` | `StatusPill kind=approved` |
| Load error / search error | notice + "Try again" | `LoadError` (full, retry); search error stays an inline `role=alert` |
| Empty Library (search show/artist, import, Library folders by permission) | old EmptyState with five buttons | kit `EmptyState` (Search for a movie) + the other buttons beneath, same permissions |
| Loading | grey blocks | `Skeleton shape=poster` |
| Live refresh on title/slot/resync | unchanged | unchanged |

Kept testids: `library-card` (now on the card `li.item` inside `ul.grid`), `library-row`, `requested-by`, `request-approved`, `last-opened`.

## Library tools

| Page | Before | After |
|---|---|---|
| Movie series list | eyebrow + panel links with "Source identity <id>" | `PageHeader` with back, `ListRow` per series with a Badge count, `EmptyState`, `LoadError` with retry, `Skeleton`; the raw source id is no longer shown |
| Movie series detail: tick movies, file and Never-add choices, protected acknowledgement, **Review N selected → Confirm this exact selection → Remove reviewed movies / Cancel**, stale review shows the alert and re-review, results + "Review remaining movies" | eyebrow + `.btn`s | `PageHeader` with back; same staged preview/review/apply and same labels; Remove is a destructive `Button`; Version/File ids moved into a "Details" disclosure (settings permission) |
| Held / Turned down (ReleaseList): remove one, **Clear all with confirm()**, pager, links between lists | eyebrow + `.btn`s | `PageHeader` with back and a Held/Turned down link switch (aria-current), kit `Button`s, `EmptyState`, `LoadError`; `listed-release`, `listed-for` kept |
| Import a library: pick folder, **Scan (nothing is moved)**, Scan the rest, tick/choose/look up per folder, **Adopt N**, results | eyebrow + `.btn` + `<progress>` | `PageHeader`, kit `Button`s (busy), `ProgressBar`, `Badge` score, `EmptyState`; all testids (`nothing-moved`, `adopt-plan`, `scan-warning`, `scan-more`, `group-*`, `music-albums`, `music-album`, `show-files`, `adopt-results`) kept |
| Missing subtitles: filters (language/kind/title, URL), **Search all**, search one, pager, progress | eyebrow + `.btn` + `<progress>` | `PageHeader` with Search all in actions, kit `Button`s, `ProgressBar`, `EmptyState`, `LoadError`; testids kept |
| Rename files: preview, **Rename N files** | eyebrow + back link | `PageHeader` with back; `RenamePreview` uses kit `Button`/`ProgressBar`/`Skeleton`; testids kept |
| Video codecs: find/codec/sort, select across pages, **Review searches → Start searches / Cancel**, pager | eyebrow + `.btn` | `PageHeader` with back, kit `Button`s, `Badge` Protected, `EmptyState`, `Skeleton`; staged review unchanged |

## Search (`routes/search/+page.svelte`) and its dialogs

| Action | Before | After |
|---|---|---|
| Page title + how-to line per kind | `.page-head` with mono id hints | `PageHeader` "Search", subtitle per kind |
| Look for Movies / Shows / Music (URL `kind`) | `.seg` group "Look for" (aria-pressed) | `Chip`s with icons in group "Look for" (aria-pressed kept) |
| Search field (labels "Movie name or id" / "Show name or id" / "Artist name or id") + Search | custom bar + `.btn` | same input and labels, kit `Button` primary (busy) |
| Ask limit line, ask problem alert | unchanged | unchanged (`ask-limit` testid kept; problem text via `plainReason`) |
| Result rows `li.hit` (`show-hit`, `artist-hit` testids): In your Library link, Never-add, Add (manage_library), You asked · state link, Ask / Ask too / Ask for more (ask), no-permission line | `.btn` / `.chip` | kit `Button`s (same names), `StatusPill` "On the Never-add list"; same permission order |
| Add a movie: folder, Pick rules, Release gate, Keep up to date, Search now, second Version, Cancel / **Add to Library** | hand-made `<dialog class="modal">` | kit `Sheet` titled "Add <name>"; buttons in the Sheet footer submit the same form |
| AddShowDialog, AddArtistDialog, AskShowDialog, AskArtistDialog (`open()` API unchanged, all fields and checks unchanged) | hand-made `<dialog>` | kit `Sheet` (native dialog, Escape/backdrop close), kit `Button`s in the footer; `no-show-search` and other testids kept |
| Loading / empty / no TMDB key | grey blocks, old EmptyState | `Skeleton` rows, kit `EmptyState` (TMDB key action keeps the settings link) |
| CopyControls: save checked edition, protect / remove protection (acknowledge), **Keep this copy and add another Version**, **Use an existing video** | `.btn` + two `<dialog>`s | kit `Button`s, `Badge` Protected, both dialogs are kit `Sheet`s with the same titles and forms |
| WishEditor: add / take off a language | `.btn` | kit `Button` "Add a language", `IconButton` "Take … off"; `wish-editor`, `wish-row` kept |
| EditionDecision: edition, confirm checkbox, Save edition and try import again | `.btn` + "01" badge | kit `Button` (busy), icon heading; `edition-decision` kept |
| RenamePreview: Rename N files, progress | `.btn` + `<progress>` | kit `Button`, `ProgressBar`, `Skeleton`; tracked-caps "Now/New" labels are sentence case |

WishEditor and EditionDecision stay inline forms: they live inside pages and sheets owned by other areas (Settings > Subtitles, the subtitle panel, Decide), so turning them into their own sheets would change those flows.

## People

| Action | Before | After |
|---|---|---|
| Person search (URL `q`), pager, attribution | `.page-head`, `.btn` | `PageHeader` with back to Library, search field with icon, kit `Button`s, `Skeleton`, `EmptyState` |
| Person cards (`a.person-card`, Known for …) | bordered tiles | token card with lift on hover (press action), same link |
| Person page: role filter (All / Actor / Director) | `.seg` group "Credit role" | `SegmentedControl` "Credit role" |
| Select missing movies, **Review N selected → Add/Ask for reviewed movies**, results list, ask limit | `.btn` | kit `Button`s; same staged review; results text unchanged |
| Filmography states: In Library (link), Never-add, ask state, Missing | coloured text | `StatusPill`s with the same words (In Library stays a link) |
| Load error | notice | `LoadError` (full) or inline alert once data exists |
