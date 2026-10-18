# Milestone 2 actions: operations (Downloads, Needs a decision, History, Calendar)

Every action below existed before the restyle and stays reachable after it. "Before" is the old markup, "after" is the kit version.
`ReleaseList.svelte` and `SubtitlePanel.svelte` are also used by title pages, so they are not touched here.

## Downloads area navigation (`routes/downloads/+layout.svelte`)

| Before | After |
|---|---|
| Sub-navigation links Downloads / Needs a decision (count `decide-count`) / History, `aria-current` | `OpsSegments` (new kit piece, looks like SegmentedControl) under each page's PageHeader: **Now** / **Needs a decision** (count badge, `decide-count`) / **History**. Still real links (`to()` base path), `aria-current="page"`, inside `main`. Not repeated as buttons elsewhere on the page. |

## Downloads (`routes/downloads/+page.svelte`)

| Before | After |
|---|---|
| Eyebrow + h1 "Downloads" + intro | PageHeader "Downloads" + subtitle (no eyebrow) |
| Row: poster, name link to title (`to('/library/id')`), covers (`row-covers`), release name in mono, state chip (`row-state`), progress bar, mono meta (`row-percent`, size, `row-speed`, `row-eta`, version, downloader, ago), note (`row-text`), episode list disclosure (`row-episodes`, `row-episode`) | Same data in one calm row: art, title link, covers, plain StatusPill (`row-state`), one ProgressBar with percent (`row-percent`) and time left (`row-eta`), speed (`row-speed`) in a quiet meta line, release name as secondary text (not mono), note, episode disclosure kept. |
| Details button (select into side panel, focus heading) | Tapping the row's Details button selects it. Desktop (≥ 1024px): side panel. Phones/tablets: a Sheet opens with the same details (list first). Heading focus kept on desktop. |
| Mark failed button (confirm, POST mark-failed, toast) | Kept as a visible row button (e2e asserts it is visible in the row) with the same confirmation. Also in the detail panel/Sheet. |
| Remove button → Remove dialog (delete files / turn down / search again, error, Cancel, Remove) | Kept as a visible row button; dialog is now the kit Sheet "Remove this download?" with the same three checkboxes, rules (Search again needs Turned down), error and buttons. |
| "Finished and stopped" section, "Imports" section | Same two sections with kit section headers; imports use ListRow with StatusPill. |
| Loading skeleton, load error with Try again, empty state with Add media + Downloaders links | Skeleton; LoadError (full, Try again); kit EmptyState "No downloads yet" with Add media, plus a Downloaders link. |
| Live updates (download/slot/import/resync, throttled) | Unchanged. |

Not done as briefed: Mark failed and Remove are **not** moved into a ⋯ Menu, because the e2e suite (`downloads.spec.ts`) needs them as visible `button`s inside `download-row`. They are styled as quiet plain buttons; Details is the main row action.

## Needs a decision (`routes/downloads/decide/+page.svelte`)

| Before | After |
|---|---|
| Eyebrow + h1 + intro + "Back to downloads" button | PageHeader "Needs a decision" + subtitle; the segment nav replaces the back button. |
| Card per decision (`decision`): art, name link, Version + waiting, why (`decision-why`), reason (`decision-reason`), last Release (mono), has-copy note | Card per decision: art, title, waiting line, the cause boxed in plain words (`decision-why`), reason via `plainReason` with raw text in an admin Details disclosure (`decision-reason` stays on the plain sentence), last Release in Details. |
| Choices: Search again (primary), Choose a release by hand (link `#h-rel`), Try/Add another Version (link `#h-versions`), Add to Turned down (confirm), Stop looking (confirm), EditionDecision | Same, placed directly under the cause. Search again primary; links secondary; Add to Turned down and Stop looking plain, both keep their confirmations. |
| Pack card (`decision-pack`): pack line, release name, episode list, pack actions (Search again for these…, Add the pack to Turned down, Stop looking for these…), per-episode sub cards | Same structure and button names. |
| Loading, error, empty "Nothing needs you" with Downloads link | Skeleton, LoadError, EmptyState with Downloads action. |

## History (`routes/downloads/history/+page.svelte`)

| Before | After |
|---|---|
| Eyebrow + h1 + intro + Back to downloads | PageHeader "History" + subtitle; segment nav. |
| Filter form: Movie/show/artist select, From/To dates with range validation, "What happened" checkboxes, episode filter + "All of the show", Show everything | Desktop: same controls in a quiet bar; kinds are `OpsCheckChip` (checkboxes that look like Chips). Phones (< 768px): a "Filters" button with the active count opens a Sheet holding the same form. Filters still live in the URL. |
| Event rows (`history`, `history-row`, kind chip, title link, `history-episode`, `history-version`, summary, time), "Why and where" toggle with facts, Newer/Older paging, `history-count` | Same rows in one panel, kind as StatusPill-style word, Why and where kept (facts stay mono, they are the admin detail), paging Buttons. |

## Calendar (`routes/calendar/+page.svelte`)

| Before | After |
|---|---|
| Eyebrow + h1 + intro | PageHeader "Calendar" + subtitle; Today button in its actions |
| Month before / after icon buttons, month name (`cal-month`), Today | IconButtons with the same names, month title, Today button |
| Month / List view buttons (aria-pressed) | `OpsSegments` (buttons, aria-pressed), same names. Phones still open on List (agenda). |
| Everything / Movies / Shows / Music (`cal-titles`) | `OpsSegments` with the same names and testid |
| In cinemas / Digital / Disc checkboxes | `OpsCheckChip` checkboxes with a colour dot |
| Month grid (`cal-grid`, `data-day`, `cal-entry` links) | Quiet empty cells, today ringed, readable event pills; same testids and links |
| List (`cal-list`, `cal-item`) | Agenda list grouped by day; same testids |
| Phone calendar feed (`cal-feed`, `feed-state`, Make a feed link / Make a new link with confirm, link field, Copy link, Open in a calendar app, Turn the feed off with confirm, proof-needed card, feed error) | Same in a kit card with Buttons; permission checks unchanged. |
| Empty "Nothing coming up" with Add media | Kit EmptyState with Add media |
