# Milestone 2 actions: admin (Approvals / Your requests, Settings, Account, access screens)

Every action below existed before the restyle and stays reachable after it. "Before" is the old markup, "after" is the kit version. No handler, API call, confirmation, validation, permission check, `to()` link or `data-testid` was removed; where markup changed the testid sits on the equivalent element.

## Everywhere in this area

| Before | After |
|---|---|
| `<header class="page-head">` with h1 + intro + action buttons | `PageHeader` (one h1 = page or section name, subtitle, actions slot). Source health page uses PageHeader's back link "Sources" instead of a separate back link. |
| `.btn btn-red / btn-ghost / btn-quiet / btn-danger` buttons (~180) | kit `Button` primary / secondary / plain / destructive, same `type`, `onclick`, `disabled`, `aria-*`, `form`. Buttons that had no `type` keep submit behaviour (`type="submit"`). |
| `<a class="btn">` links (OpenAPI file, Download the log, report download, Health, Open My account, Read the log) | `Button href` for plain links; new `lib/ui/AdminLinkButton.svelte` (same look) for links that need `download` / `target` / `rel` / `style`. |
| `icon-btn` delete buttons | kit `IconButton` with the same accessible name. |
| `section.card` / `form.card` groups with an h2 | `SettingsCard` (title, description). Forms wrap the card (`<form class="sc-form" aria-label=…>`), same `onsubmit`. Sections that carried a `data-testid` keep it on a wrapping `div`. Remaining list cards (`ul.card`, tiles) get the SettingsCard look from the settings layout. |

## Settings layout (`routes/settings/+layout.svelte`)

| Before | After |
|---|---|
| Full-bleed grey frame, 288px nav panel "Settings" + Home link + grouped section links (`nav aria-label="Settings sections"`, `aria-current`) | Inside normal shell width: quiet 236px section list, same `nav aria-label="Settings sections"`, "All settings" + four groups, `aria-current="page"`, base-path `to()` links. Content column max 820px. |
| Below 1000px: native `<select>` "Settings section" that `goto()`s | Below 1100px: kit `Menu` button (label "Settings sections", shows current section) with the same groups as headings; items are links with `to()`. Hidden on the index page, where the grid is the list. |

## Settings index (`routes/settings/+page.svelte`)

| Before | After |
|---|---|
| h1 Settings + Appearance radios Light / Dark (`setTheme`) | PageHeader; Appearance SettingsCard with SegmentedControl Light / Dark / Automatic (`setTheme`, adds the existing `system` choice). |
| Grouped section link lists (Library, Linked apps, Access, System) filtered by permission and parts | Grid of SettingsCards, one per group, same filtering (`visible`, `offPartName`). |
| — | New: "Search settings" field filters sections by name and description; no match shows EmptyState with "Clear the search". |

## Section pages (all of `routes/settings/*`)

Library folders (add, recheck, remove, kind), Naming and files (save, add word tokens, clean now, examples), Sources (add, change, test, pause, remove, Health link; save-then-test order unchanged), Source health (tiles, calls), Search (save, run), Subtitles (languages, sites add/change/test/pause/remove), Downloaders (add/change/test/pause/remove, path maps, abandon attempt, "When downloads go wrong" save; save-then-test unchanged), Outside lists (add/change/sync/remove, Trakt, Never-add add/remove), Music (save), Request defaults (save), Alerts (add/change/test/remove, moment groups All/None, season packs), Media servers (add/change/test/remove; save-then-test unchanged), TMDB key (masked password field, save), Backups (make now, download, delete, restore from file label), Sign-in (local network, two-factor rule, minimum password, Plex/Jellyfin/Emby save, API keys make/copy/revoke), Move from other apps (read, plan, try, move, undo, forget, report download, saved key), Parts and controls (switch, restart part/stage/app, reconnect, confirm), Users (invite with role presets Admin/Manager/Member/Kid "coming soon", copy link, change, devices, reset two-factor, suspend/take back), My account, This app (check again, check update, health settings, storage, logs links), Schedules (run now, change, enable/disable refresh), API (copy, scopes, routes), Logs (filter, newer/older, download, clear).
All of the above keep their handlers, confirmations, validation messages (`form-error`, `aria-invalid`), separate test result rows (`*-test`, `*-result`) vs save toasts, and permission checks. Only header, buttons and card containers changed.

## Requests / Your requests (`routes/requests/+page.svelte`)

| Before | After |
|---|---|
| Eyebrow "Asks" + h1 "Requests" | PageHeader "Requests" (approvers) or "Your requests" (members); same subtitles. Ask for a movie / show / music links kept in the actions slot (`session.can('ask')`). |
| Messages card at the top (5 latest, `message` testid, unread chip, Mark all read) | Secondary "Messages" card in the side column (after the list on tablets/phones); unread count badge; Mark all read in its header; the feed is a collapsed `<details>` ("Show the latest N"). Message texts read id-like names as "someone in your household". |
| Weekly limit lines (`ask-limit`) | Same, soft grey box. |
| Pills Waiting n / Everyone's n / Mine and Everything / Movies / Shows / Music (`aria-pressed`) | kit Chips with counts (`aria-pressed`), same filters; request count on the right. |
| Ask row: poster, title link, state chip (`ask-state`), kind chip (`ask-kind`), seasons/albums, `asked-by`, `decided-by`, note, approved line, Review / Approve / Turn down / Take back (confirm) | Same row in one list card: StatusPill with the same words (`ask-state`), kind tag (`ask-kind`), plain names; Approve (primary), Turn down, Take back (destructive, same confirm), Review (pressed state, `aria-controls="wishlist-review"`). |
| Review aside (heading focus, Close on phones, facts, note, Approve / Turn down) | Same aside as a card in the side column; phone keeps the open/close behaviour and heading focus. |
| Native `<dialog class="modal">` approve / turn down: Library folder, Pick rules, note / Why, hint, error, Cancel, Approve and add / Turn down | kit `Sheet` (native dialog, titled "Approve <name>" / "Turn down <name>") with the same fields, ids, labels, hint, error and buttons; submit calls the same `decide`. |
| Loading skeleton, load error with Try again, empty state with Search | Skeleton, LoadError (Try again → `load`), kit EmptyState with Search. |

## Account (`lib/Account.svelte`, `routes/account`, `routes/settings/account`) and `lib/Sessions.svelte`

| Before | After |
|---|---|
| Cards: Signed in, Confirm local sign-in, linked ways, Two-factor (set up, enable, backup codes, turn off), Alerts for you (Save), Password (change), Signed-in devices | Same order as SettingsCards with one gap; account name passes through `personName`. Every form and handler unchanged. |
| Sessions rows (`sessions`, `session`, `data-current`), This device chip, Sign out (per row), Sign out all other devices | Same rows; StatusPill "This device"; kit Buttons with the same accessible names. |

## Access screens

| Before | After |
|---|---|
| Sign-in (`routes/login`): left-aligned panel; setup code, name, password (+ again on setup), error with wait seconds, Sign in / Make the owner; two-factor code + Start again; outside ways (HolyArrs password, Plex, Jellyfin, Emby, single sign-on, trusted proxy) only when offered; "Forgot the password?" (`reset-owner-help`) | Centred card on a soft accent wash, brand mark, large centred title, same fields and order; **Show / Hide password** toggle (`aria-pressed`); centred numeric code field for two-factor; kit Buttons (`busy` spinner); "or" divider; outside ways still only when the server lists them; recovery disclosure kept. Wrapped in `main`. |
| Invite (`routes/invite/[token]`): name, password, again, Join and sign in, gone message with sign-in link | Same centred card, password reveal, kit Button. Wrapped in `main`. |
| Welcome (`routes/welcome`): sticky side header, "Step n of m" in mono, parts form (`welcome`, `welcome-update-privacy`), family radios, Continue / Just movies, TMDB key (masked), Library folder form | Centred friendly step flow: brand, step dots (`aria-label="Step n of m"`), large centred title, same three forms and handlers; Continue / Just movies, Save and continue, Add folder and open Home as kit Buttons. |

## Not done

- Settings section pages kept their inner field markup (labels, hints, checklines) and list cards; only containers, headers and buttons moved to the kit.
- Password reveal uses a "Show/Hide" text button because the icon set has no eye icon.
