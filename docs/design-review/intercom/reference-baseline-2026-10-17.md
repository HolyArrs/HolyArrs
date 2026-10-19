# Measured Intercom reference baseline

Audit date: 2026-10-17. Approved source: [Intercom November 2025](https://mobbin.com/apps/intercom-web-e2b63d2e-e001-426a-93f3-b5bd11346744/6e269961-628f-4a8b-b490-a34aea334a69/screens). This is a comparison baseline, not a claim of exact reproduction.

## Provenance and measurable limits

The local acquisition manifest dates the flow archive export to `2026-10-17T08:22:49.8514634Z`. Original product capture dates are not supplied; November 2025 is Mobbin's version label. Acquisition time is not original capture time.

Ignored reference corpus under `.omx/artifacts/visual-ralph/intercom/`:

| Corpus | Confirmed inventory | Limits |
| --- | --- | --- |
| full-export | 563 PNGs, each 1920×1320 raster pixels, one frame | No computed styles, viewport, DPR or zoom metadata |
| screens | 56 WebPs, each 1920×1320, one frame | 60 ordered positions in 10 flows; four duplicate positions |
| flow-exports | 118 ZIPs, 681 PNG entries | No timed video, GIF, CSS, JS, font or prototype assets |

The visible Mobbin footer occupies approximately 128 raster pixels; the product frame is approximately 1920×1192. These are **raster dimensions**, not verified CSS viewport dimensions. Comparison captures use a documented 1920×1192 CSS viewport at DPR 1 and 100% browser zoom; their equivalence to the source capture scale remains unknown.

The canonical registry of exact ordered screen IDs is `inbox-flows.json` in that ignored corpus. The other 118 ZIP manifest entries have names/hashes, not Mobbin UUID mappings. Do not invent missing IDs.

## Component and flow mapping

| HolyArrs component | Reference files / exact flow IDs | Comparison scope |
| --- | --- | --- |
| Shell, navigation, expandable Library | full-export 40,120,480; Inbox `57e3c9bd-7065-4cfc-9584-a248c4aaeea6` | Rail, sidebar, hierarchy, selected state, workspace |
| Wishlist, Downloads, decisions, title detail | 40; Inbox; Sending a message `27b1d962-dee6-4cbb-ba51-f3d667477e9d`, screen `7d888da9-c41c-4130-aa8e-1d48326b910d` | List / inspector relationship, stable state progression |
| Search and filters | Searching inbox `0e25f6bb-b36c-409b-a7f7-35687f1de248`; Filtering and sorting `6211d06a-abe1-4112-92c9-a37a676704f4`; Searching Intercom `1edce39d-654d-4597-8b5a-7d69a13708ce` | Filter hierarchy, query controls, selection |
| Home and Calendar | 280,440; Team inbox performance `4005ac5d-5051-4d93-9121-5e3a46bb9cc8` | Metrics, restrained hierarchy, surfaces |
| Settings, grouped forms, sheets | 240,480; Adding a team inbox `670d481a-5adb-4c2e-ae5d-9f48210d3c4f` | Fields, dividers, footer actions, feedback |
| Welcome, login, invite | Inbox screen `d72f1b2c-5fd0-4ebb-bc3b-bd66c43cec5e` | Onboarding shell and hierarchy |

Additional recorded flows: Marking as favorite `0fcdc6cf-ef54-4528-aab6-02eff0d3ab62`, Help desk `dc3d8590-28fd-4161-9287-9a1ed177cff9`, Creating a conversation `0c5e35be-e5e0-428b-8c22-0033c723f7d3`. Adding a macro was previously probed at `0b2bd43c-00fc-4cd6-a962-43f85c8a96bc`.

## Observed targets and classification

Read-only reference review inspected exports 40,120,240,280,440,480 and the two exact screen IDs above. Inventory checks enumerated all images; this does not mean every reference image received visual review.

Approximate screenshot measurements: rail 56 raster px, contextual sidebar 290–300, major gaps 8–12, top strip 62–66 high, header 72–80, list rows 72–96, inspector 360–485, major radii 14–18, cards 10–14, divider 1. Screenshot samples: light `#efefeb`, `#fafbf8`, `#ffffff`, `#f7f8f1`, `#ebece9`; dark `#080808`, `#13171c`, `#181b21`, `#1c2025`. Color profile/export processing and content affect these samples; they are not authenticated source tokens.

Confirmed implementation correspondences: 56 CSS px rail; 280–300 sidebar; shared spacing/radius/divider tokens; layered light/dark surfaces; semantic feedback; list/inspector and contextual navigation patterns. A shared number alone does not prove perceptual or computed-style equality.

Intentional adaptations: HolyArrs branding, terms, artwork, operational states and permissions; expandable Movies/TV Shows/Music; household versus administrator capabilities; responsive pane removal; accessible keyboard/touch behavior; no irrelevant Intercom controls or Mobbin footer. Media content cannot have a meaningful whole-image pixel identity with support conversations.

Known deviation: existing licensed Geist Variable is a substitute. Public Intercom sign-in CSS previously referenced Graphik, but that does not identify this authenticated November 2025 UI. No unlicensed font has been acquired.

Missing evidence: authenticated font identity/metrics, source CSS tokens, source viewport/DPR/zoom, timed duration/easing/stagger/exit choreography, computed focus styles, native tablet/phone/TV behavior, drag/resize and keyboard details. Flow still sequences establish order and continuity only. Exact parity stays unclosed for these properties. Current HolyArrs motion tests prove its behavior and reduced-motion handling, not Intercom timing equality.

## Fresh flow inspection and component classifications

Public Sending a message, Filtering and sorting inbox, and Adding a team inbox pages were inspected headlessly on `2026-10-18T00:42:45Z` through `00:42:49Z` (October 17 local time), at **1440×1000 CSS px, DPR 1**. Each returned HTTP 200 and rendered ordered stills. No video/audio element or timed player control was available. The receipt is `audit20261017/flows/receipt.json`; `timedMotionVerified` is false for all three. This is current inspection evidence, not a source animation measurement.

| HolyArrs implementation | Intercom visual counterpart | Classification and boundary |
| --- | --- | --- |
| Global rail/context sidebar, Library expansion | Global product rail and expanded Train/Inbox hierarchy (120/40) | Corresponding hierarchy and layered surfaces; HolyArrs destinations and chooser behavior are adaptations |
| `Icon.svelte` | Navigation/action glyphs (40/120/240) | Own 24px-grid line icons; silhouettes and stroke are an intentional difference, not copied source assets |
| Shared buttons, textboxes, selects, labels/help/validation | Inbox controls and team-inbox settings forms (40/240/480) | Rounded compact controls and restrained borders correspond; native selects and accessible validation remain platform adaptations |
| Filter popovers, chips and selected rows | Inbox filtering/sorting flow | Ordered filter/selection continuity corresponds; exact hover/focus/disabled computed styles unavailable |
| Poster, PersonCard, media cards | Contact avatar/content thumbnail treatment | Artwork, poster proportions, fallback initials and media metadata are intentional media-manager additions |
| Request and download list/inspector | Inbox list/conversation/details panes (40) | Multi-pane relationship corresponds; progress, approval, file and service states are HolyArrs-specific |
| Dialogs, release chooser, rename/delete previews | Inbox modal/sheet overlays (40/240) | Surface, corner, divider and action hierarchy correspond; domain actions differ |
| SubtitlePanel, ShowPage, ArtistPage, album/track views | Grouped details/forms and expandable data sections | Domain-specific capabilities, not one-to-one Intercom screens |
| EmptyState, loading placeholders, ToastList | Empty work panel, transient feedback (120/240) | Visual grammar corresponds; local loading, success/error/recovery and reduced-motion tests do not establish source timing parity |
| Sessions, Account, login/invite/welcome | Sign-in/onboarding shell | Functional accessibility tested locally; authenticated Intercom security interactions are unavailable |

Desktop Settings deliberately bounds readable row/form width. Library search deliberately caps at 760 CSS px on wide screens. These differ from some reference compositions and are retained as explicit reading-density adaptations rather than filled with irrelevant controls. Selected request summaries may also appear in their list row and inspector; that repetition preserves the master/detail relationship.
