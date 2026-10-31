# Approvals page: final audit (2026-10-31)

Page: `/wishlist` for approvers. Files: `lib/approvals/ApprovalsDesktop.svelte`, `ApprovalsStage.svelte`, `routes/wishlist/+page.svelte`.

## Round 1 findings

| # | Issue | Sev | Area | Result |
|---|---|---|---|---|
| 1 | After the Undo time, the ask flicked back to Pending until the server answered; a second click posted twice | High | Lifecycle | Fixed: the ask keeps its new tab (`sending`) until the answer lands; Undo can't cancel a send in flight |
| 2 | A list load started before an answer could land after it and bring back the old state | Med | Lifecycle | Fixed: only the newest load applies; an answer drops older loads |
| 3 | Music video and the "how much is in the Library" read restarted on every live event | Med | Window | Fixed: keyed on artist / ask+title+state only |
| 4 | Focus went to the wrong card (or body) after stepping and closing | Med | Keyboard | Fixed: focus returns to the card last shown |
| 5 | Esc needed two presses when the window opened into What to add | High | Keyboard | Fixed: one Esc closes the window |
| 6 | Window closed in one frame (open took ~550ms) | Med | Motion | Fixed: 320ms eased scale-down and fade, backdrop fades |
| 7 | Filter menu opened in ~170ms, closed instantly | Low/Med | Motion | Fixed: 280ms eased rise, 180ms fade out; phone sheet 420ms in, 260ms out |
| 8 | Decline still clickable during the Approved beat | Low | Cards | Fixed: card answers are inert during the beat |
| 9 | "Approve all N" counted cards already in their beat | Low | Toolbar | Fixed |
| 10 | ← / → during the advance after an answer jumped to the first card | Low | Window | Fixed |
| 11 | Phones: Approve all 48px vs Filter 40px | Low | Consistency | Fixed: both 40px |
| 12 | Unreachable "Nothing here with this status" branch; dead Esc handler in the Stage | Low | Code | Removed |
| 13 | Approve stays on with nothing ticked | Med | What to add | Not a bug: Approve is disabled when the pick is empty (`pick.empty`) |
| 14 | Toast shows ~600ms after Approve | Low | Motion | Kept: it follows the card's Approved beat by design |
| 15 | Failed-download card without Decline vs failed ask with Decline | Low | Cards | Kept: only asks the server can still decline get the button (seen with fake demo data only) |
| 16 | "Play trailer full screen" labels | Low | Copy | Kept: shared Billboard trailer control (used on Explore), not media playback |

## Round 2

All fixes above verified live at 1440, 390, 360 on real data. No horizontal overflow, no page console errors.
New: Done window reads "Declined by showcase-owner." That is the test account's name of whoever declined; Low (test data).

## Measured motion

Window open 532–581ms · close ~320ms · sheet reveal ~460ms · sheet collapse ~550ms · filter menu 283–295ms in, 167–201ms out · phone filter sheet ~445ms · toast ~320ms of movement. Reduced motion: all 0ms.

## States now

- Pending: card Decline / Approve with asker; window opens on What to add with stepper.
- Upcoming: card "Upcoming · date"; window "Getting it for … It isn't out yet."
- Processing: card "Processing"; window "Processing · Getting it for …" with What was approved.
- Failed: red "Failed" + Decline; window "Failed · Couldn't add it to the library." with Decline.
- Ready: card "Ready"; window "Ready · n seasons/albums" + "n more … available".
- Declined: card "Declined"; window "Declined by …".

## Preview-only code

`?demo` (live-demo.ts) removed. Nothing else preview-only remains on this page.
Searching / Downloading / Importing can't be checked on real data until something downloads.

## Skill review round

Four reviewers on real data (1920, 1440, 1024, 768, 390 touch, 360 touch; dark and light), then a critic pass that checked every finding against the code.

| Skill | Top findings | Result |
|---|---|---|
| design-critique | Phone: Filter and Approve all on two full-width rows push the art down; Decline a small icon square next to a wide Approve; lifecycle line under every card | Fixed: one row, equal halves; equal icon-only answers on phones. Line kept (it is the lifecycle line by design) |
| visual-verdict | Score 82 (revise): phone toolbar, uneven answers, empty space in What to add at 1440 | Phone items fixed; empty panel space kept for now |
| high-end-visual-design | Light cards ring + hard shadow; meta text weak over bright art; chip sizes differ in the window | Not changed (Low/unchecked by critic) |
| ux-copy | Approve toasts had a second sentence; "The download failed." vs "Couldn't add it to the library."; "Approved straight away (… may auto-approve)"; "Available" tab label in the inbox layout | Fixed: "Approved X.", "Couldn't download it." / "Couldn't add it to the Library.", "Auto-approved", "Ready" |
| kill-ai-slop | Glow and kicker in FocusView (not on this page's list); scrims are for legibility | No change |
| web-design-guidelines | Window close and backdrop still animated under reduced motion; press scale not removed; nested live region; progress fill animated width | Fixed all four (fill now scaleX) |
| animation-vocabulary | Close used a different curve from the open; endless shimmer on the window's progress line | Close now uses the open's ease; shimmer kept (off under reduced motion) |
| accessibility-scan / inspect | Approve buttons all named "Approve"; amber Pending count 3.84:1 in dark; phone tabs 32px tall; desktop Tab gets stuck in the search overlay | Fixed: "Approve {title}". Others left (below) |
| critic | Rejected "Available" as an approver-page issue (inbox layout only); downgraded phone toolbar and Downloading % to Med | Followed |

Also: "Declined by …" now shows your display name when you were the one who decided. The server only shares your own display name, so other approvers still show their account name until the asks view sends display names (server change).
"Play trailer full screen" is the shared TrailerPlayer label (Explore too); left.

Left open: Searching / Downloading · % / Importing words on Processing cards (Seerr shows one word; your call); amber count contrast; 44px phone tabs; search field opening on focus (shared shell); phone filter sheet still wraps a menu role; light card shadow; window panel height.

## Round 3: icons + skill audit

### Icons

New `web/app/src/lib/approvals/StatusIcon.svelte`, one glyph per status, used in the Done chip, the progress capsule and the Filter status list.
Path data is hand-copied from Lucide (ISC License, Copyright (c) Lucide Contributors): search, download, package, loader-circle, calendar, circle-check, circle-alert, circle-x, clock. It is inlined, with no runtime CDN and no new package. The licence note is also in the file header.

| Status | Glyph | Motion (off under reduced motion) |
|---|---|---|
| Searching | magnifier | slow sweep, fill moves with the lens |
| Downloading | arrow into tray | arrow nudges down |
| Importing | package | none |
| Processing | loader arc | slow spin (1.6s) |
| Upcoming | calendar | none |
| Ready | check circle | check draws in once when the card mounts |
| Failed | alert circle | none |
| Declined | x circle | none |

Chip: 28px tile, 16px glyph, duotone (20% tinted fill of the shape plus a 2.4-unit stroke, about 1.6px on screen), and a faint top inner highlight that matches the glass. Measured glyph contrast on the capsule is 5.3 to 7.8:1 and the word 6.0 to 9.5:1. Close-ups at 2x: `icons-light.png` and `icons-dark.png` in the round3 scratch folder.

### Skills, findings, outcome

| Skill | Top findings | Outcome |
|---|---|---|
| design-critique | Idle Processing capsules all look alike; Failed beside Decline reads as a button; empty space in the window's What to add | Kept: the capsule and the equal halves are the agreed design. The window height is left. |
| visual-verdict | Score 84 (revise): same three points | As above |
| high-end-visual-design | Light glass muddy over warm art (saturate 1.9) | Fixed: saturate 1.3 and brightness 1.14 |
| ui-ux-pro-max | Touch targets: Approve all and Filter at 40px, ticks at 18px; 11px phone text | Fixed: 44px on touch, a 32px tick label (44px on touch), 12px minimum |
| kill-ai-slop | Search fill and lens split during the sweep; six fast spinners at once | Fixed: the fill moves with the lens; the spin slowed to 1.6s |
| animation-vocabulary | The hover shimmer loops | Kept: it is the chosen hover, and it is off under reduced motion |
| web-design-guidelines | Window focus lands on the dialog; Filter groups not announced | Kept: the dialog is named by its title. The group roles are Low and left. |
| ux-copy | "7 d ago" in the window vs "7 days ago" on cards | Fixed: shared `agoLong` in format.ts, with a test |
| accessibility-scan / inspect | High: no focus ring on the card's Decline and Approve. High: the window's "Failed" was 2.74:1 in light. Tab out of Filter jumped to a card. Heading order skipped a level. | All fixed: `.b:focus-visible` ring; the dark status band keeps dark tones (#ff6961 about 6.9:1); Tab returns to Filter; card titles are h2 |
| code-review (high) | High: an approved album taken in by IncludeAlbums could be dropped by a refresh before it was kept up to date. Medium: failed downloads not sorted or counted as failed. Medium: the member Stage branch was dead code. Low: live reads capped at 100 and dropped events during a read. | Fixed: kept up to date in the same write, with a test; failed sorts and counts from the live stage; StageViews, AskPanel, FocusView and AskSummary removed; 500 rows and a reread after a busy read |
| verification-before-completion | svelte-check (0 errors), `vitest run src/lib` (164 files, 1270 tests), `go test ./internal/asks/ ./internal/library/` | All passing |
| critic | Accepted with reservations. One Medium: the failed count depends on live rows. | Raised to 500 rows. A server-side count of failed asks is still needed for libraries with more than 500 recent downloads. |

### Left, and why

- Cards and the window can word a partly-had show differently (the window has the per-season facts, the card does not). This needs one shared status helper and the facts cached in ctx. It is not a quick fix.
- Lifecycle `skipped` and `reason` are never filled, so "n more available" and the failure reason don't show yet. They need the server to send them.
- A failed count on the server for libraries with more than 500 recent downloads. That is the downloads API, which another session owns.
- An "asked" flag on album parts, so turning keep-up-to-date off later still keeps an asked album. This is a data-model change.
- Searching, Downloading and Importing were only checked in code and on the icon sheet. The test data has no live downloads, and Downloads must not get fake data.
