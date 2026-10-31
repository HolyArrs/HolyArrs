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
