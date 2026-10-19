# Intercom-derived visual system for HolyArrs

Status: implementation specification from the approved Mobbin Intercom web reference, version `6e269961-628f-4a8b-b490-a34aea334a69`.

This document defines how to translate the observed Intercom product system into the real HolyArrs app. It does not copy Intercom branding, text, customer data, or product-specific illustrations. HolyArrs keeps its own mark, media imagery, vocabulary, routes, permissions, and working API behavior.

## Evidence and confidence

The local corpus contains 563 exported PNG screens under `.omx/artifacts/visual-ralph/intercom/full-export/`, all at 1920 x 1320, plus 56 connector images under `.omx/artifacts/visual-ralph/intercom/screens/`. The lower 128 px of the exported PNGs is Mobbin attribution, so the observed application frame is approximately 1920 x 1192.

The corpus is strong evidence for desktop composition, spacing, surface hierarchy, controls, forms, tables, overlays, empty states, and light/dark palettes. It is not direct evidence for tablet or phone layouts, exact font files, animation duration, easing curves, focus appearance, or touch behavior. Those values must not be described as exact until a flow recording or live computed-style inspection proves them.

Representative evidence used for this specification:

| Export | Pattern observed |
| --- | --- |
| `40.png` | Dense four-pane workspace, centered scheduler overlay, control sizing |
| `80.png` | Modal, scrim, grouped form fields, fixed action footer |
| `120.png` | Rail plus expandable secondary navigation, empty state, option cards |
| `160.png` | Editor workspace, right inspector, tabs, data table, floating tool bar |
| `200.png` | Secondary navigation, tabs, search/filter row, editable cards, preview pane |
| `240.png` | Large right-side sheet, form grouping, fixed bottom action bar |
| `280.png` | Dashboard canvas, chart card grid, filter strip, metric hierarchy |
| `320.png` | Right-side sheet, contextual popover, form controls and scrim |
| `360.png` | Large dotted work canvas, node cards, left navigation and right palette |
| `400.png` | Contacts/detail layout, tabs, data card, chips and bulk actions |
| `440.png` | Dark theme shell, chart cards, dark surface and divider hierarchy |
| `480.png` | Settings tree, segmented control, form cards, preview pane, warning notice |
| `520.png` | Settings page under scrim with a wide creation sheet |
| `560.png` | Minimal authentication/reset form |

## Product direction

HolyArrs should feel like a media operations workspace: calm, dense, direct, and fast. The memorable behavior is progressive depth. A person starts in a small global rail, enters a media or operations area through a contextual sidebar, and works in a large white canvas that can split into a list, primary detail, and inspector without losing place.

Media art supplies identity inside the workspace. It should not replace the operational hierarchy. Posters, backdrops, albums, and avatars sit inside bounded regions; controls, state, and ownership remain legible on plain surfaces.

## Measured desktop shell

Measurements are taken from the 1920 px-wide reference captures and should be treated as target ranges because screenshots include raster scaling and antialiasing.

| Element | Observed target |
| --- | --- |
| Global product rail | 56 px wide |
| Context sidebar | 290-300 px wide |
| Gap between major shell surfaces | 8-12 px |
| Outer shell inset | 8-12 px |
| Top account/trial strip | 62-66 px high, 14-18 px radius |
| Workspace top/header row | 72-80 px high |
| Context nav row | 38-44 px high |
| Dense list row | 72-96 px high depending on preview lines |
| Inspector/detail pane | 360-485 px wide |
| Primary content pane | flexible, minimum about 560 px |
| Major panel radius | 14-18 px |
| Standard card radius | 10-14 px |
| Compact control radius | 8-12 px or fully pill-shaped |
| Divider | 1 px |

The Inbox references frequently resolve to approximately 56 px global rail, 300 px navigation, 340 px result list, 710 px primary conversation, and 480 px inspector. HolyArrs should use this as a desktop multi-pane template, with pane removal in reverse importance when space shrinks.

### Shell rules

1. Keep the 56 px global rail visually quiet and icon-led. It selects a major product area; it does not contain every route label.
2. Put the current area's name, expandable groups, badges, and local destinations in the 290-300 px contextual sidebar.
3. Give the workspace its own rounded surface with an independent header, rather than rendering pages directly on the app background.
4. Separate panes with 1 px dividers. Do not wrap every row in a floating card.
5. Preserve spatial context. Open media details, decision inspectors, and settings editors beside the list when desktop width allows.
6. Keep the approved Library parent expandable, without a disclosure arrow. Movies, TV Shows, and Music appear as indented children with a slim guide line.

## Spacing and density

Use a 4 px base rhythm.

| Token intent | Value |
| --- | --- |
| Hairline separation | 1 px |
| Tight inline gap | 4 px |
| Icon/text or compact control gap | 8 px |
| Standard inner gap | 12 px |
| Control/card inset | 16 px |
| Section inset | 20-24 px |
| Major section separation | 32 px |
| Spacious empty-state separation | 48-64 px |

Desktop navigation and data screens are intentionally dense. Density comes from alignment and shared dividers, not tiny text. Default pointer targets should remain at least 40 px high and become at least 44 px on coarse pointers.

## Typography

The exact Intercom font family is not established by the exported raster images. Do not claim a font-name match. The observed form is a compact neutral grotesk with a high x-height, restrained tracking, and clear weight contrast.

The public Intercom sign-in CSS currently declares Graphik Regular, Semibold and Bold. That is a current public-shell signal, not proof of the authenticated November2025 workspace font. HolyArrs retains its bundled Geist Variable; no proprietary font files are shipped. The preview type pass uses15px/1.5 body,14px controls/secondary copy,12px captions,28px page titles, and16px phone inputs. Re-evaluate line breaks and control widths if licensed, verified font files become available. See `type-motion.md` for evidence and limitations.

| Role | Target size / line height | Weight | Notes |
| --- | --- | --- | --- |
| Workspace title | 26-30 / 32-36 px | 600-650 | Tight tracking, one line where possible |
| Section title | 20-24 / 28-32 px | 600 | No decorative display face |
| Card/list title | 16-18 / 22-26 px | 600 | Primary scan anchor |
| Body | 15-16 / 22-24 px | 400-450 | Operational copy |
| Navigation | 15-16 / 20-24 px | 500-600 | Selected row becomes semibold |
| Metadata | 13-14 / 18-20 px | 400-500 | Muted, never low-contrast |
| Label/caption | 12-13 / 16-18 px | 500-600 | Uppercase only for short grouping labels |
| Numeric metric | 32-40 / 38-44 px | 500-600 | Tabular numerals |

Use sentence case. Avoid oversized marketing headings inside the signed-in product.

## Palette and surfaces

Point samples below come from the exported screenshots. They are implementation targets, not claims about Intercom's source tokens.

### Light

| Role | Candidate | Evidence |
| --- | --- | --- |
| App/global rail | `#efefeb` | blank rail sample in `120.png` |
| Context sidebar | `#fafbf8` | blank sidebar sample in `120.png` |
| Workspace/card | `#ffffff` | repeated primary panels |
| Dashboard canvas | `#f7f8f1` | blank report canvas sample in `280.png` |
| Divider | `#ebece9` | pane boundary sample in `120.png` |
| Trial/info tint | `#e7e2fd` | top strip sample in `120.png` |
| Primary text | `#1f201e` target | visually observed; verify from live styles |
| Secondary text | `#666863` target | visually observed; verify from live styles |
| Filled action | `#20211f` target | black/charcoal primary controls |

Selected navigation rows are white or a slightly darker neutral than their parent surface, with a fine border and very soft shadow. Do not use saturated color as the default selection fill.

### Dark

| Role | Candidate | Evidence |
| --- | --- | --- |
| Global rail | `#080808` | blank rail sample in `440.png` |
| Context sidebar | `#13171c` | blank sidebar sample in `440.png` |
| Workspace | `#181b21` | blank workspace sample in `440.png` |
| Elevated card | `#1c2025` | chart card sample in `440.png` |
| Top info tint | `#332e4d` | top strip sample in `440.png` |
| Divider | `#343941` target | visually observed; verify from live styles |
| Primary text | `#f5f6f7` target | visually observed; verify from live styles |
| Muted text | `#b6bac0` target | visually observed; verify from live styles |

Dark mode is a true surface inversion, not a black background with unchanged light cards. Charts, empty states, borders, menus, and overlays all need dark tokens.

### Semantic color

Color is reserved for meaning:

- Orange underline/accent: current tab, current nested settings route, or focused product context.
- Green: successful save, publish/live action, healthy or available status.
- Yellow: attention, waiting, question, or installation warning.
- Red: failure, destructive action, or negative metric.
- Blue: selected conversational/media item and informational data series.
- Lavender: plan/trial or neutral product notice.

Do not make all primary actions the HolyArrs brand color. The common primary action is charcoal; semantic actions earn color.

## Elevation, borders, and corners

Most hierarchy comes from surface color and 1 px borders. Shadows are limited to floating menus, dialogs, side sheets, selected rows, and bottom toolbars.

- Static page/card: 1 px neutral border, 10-14 px radius, no shadow or an almost imperceptible one.
- Selected list row: 1 px border plus a soft 0-2 px ambient shadow.
- Menu/popover: 10-12 px radius, visible ambient shadow, no heavy border.
- Center modal: 12-16 px radius, medium shadow, neutral scrim.
- Side sheet: rounded 18-22 px on exposed left corners; fills full application height inside the shell.
- Circular icon control: 36-44 px diameter.
- Pill button/filter: 36-44 px height, full radius.

Avoid stacked shadows, glossy highlights, gradients on controls, and decorative glass effects.

## Components

### Navigation

- The global rail uses 20-22 px monochrome icons in 40-44 px targets.
- Selected rail destinations sit on a white/light well or a darker well in dark mode.
- The contextual sidebar starts with a 26-30 px area title and optional search/create controls.
- Expandable groups use a 1 px vertical guide and indented children. The Library parent remains arrowless by prior decision; its expanded state is communicated by visible children and `aria-expanded`.
- Counts align to the far edge and use tabular numbers.
- Account, theme, settings, and live state stay near the bottom.

### Buttons

- Primary: charcoal fill, white label, 38-44 px high, 10-12 px or pill radius.
- Semantic primary: green only for publish/live/enable; red only for destructive confirmation.
- Secondary: light neutral fill or plain white with 1 px border.
- Quiet: label/icon on transparent surface; gain a neutral well on hover/focus.
- Split button: primary label plus narrow chevron segment with a divider.
- Disabled: reduced contrast and no shadow; retain readable label.
- Icon-only: 36-44 px circular or rounded-square target, always with an accessible name.

### Tabs and segmented controls

Tabs are text-led and use a 2-3 px underline. A compact orange underline is the dominant light-theme treatment. Segmented controls use a neutral rail and a white selected segment with subtle inner shadow. They should not look like large pills floating in empty space.

### Fields

- Standard field height: 44-48 px.
- Large command/search field: 52-64 px.
- Border: 1 px neutral; focused border/ring must be visible but is not established by still references.
- Labels sit above fields at 13-14 px semibold.
- Descriptions and validation follow the field, aligned with its text edge.
- Textareas use 12-16 px inner padding and preserve user resizing where safe.
- Checkbox/radio targets include the label row, not only the glyph.

### Lists and tables

- Prefer shared rows with 1 px dividers for dense operational lists.
- Keep the first column visually strongest; put status, time, and actions on the trailing edge.
- Selected rows use a pale blue or neutral tint and a clear border.
- Long names truncate only where a nearby detail view exposes the full value.
- Table headers are muted and sticky when lists are long.
- Bulk actions appear only after selection and live in the list header.

### Cards

Use cards for independent units: metrics, configuration groups, onboarding choices, and media facts. Do not turn ordinary navigation or every setting row into a separate card.

For media:

- Poster cards keep the media art dominant but place status and actions in stable, plain regions.
- Backdrops belong in detail headers or preview modules; they do not sit behind long settings forms.
- Status chips use semantic tints and short labels.
- Availability, request, download, file, and subtitle states can coexist in one detail inspector rather than becoming separate decorative panels.

### Overlays

- Command menu: centered, roughly 760-820 px wide on desktop, 10-14 px radius, 52-60 px rows, keyboard hints in a separated footer.
- Dialog: centered, about 760-820 px wide for forms; header and action footer remain visible.
- Side sheet: 46-52% of the desktop viewport for complex creation/editing; 360-480 px for an inspector.
- Popover: anchored to its trigger, 280-360 px wide for filters or conditions.
- Scrim: neutral black at roughly 25-35% opacity in the reference. Preserve underlying structure without competing with the overlay.
- Escape, close button, focus trap, focus return, and background inertness are required.

### Toasts and notices

- Toast: top-center, about 680-720 px wide on desktop, 48-52 px high, strong semantic fill, close action at right.
- Inline notice: pale semantic surface, 12-16 px radius, title plus concise next action.
- Global safe mode: use the trial-strip pattern across the workspace top, with persistent status and one clear recovery path.

### Empty, loading, error, and offline states

- Empty state stays inside the pane it describes. Use one simple icon, a specific sentence, and at most one primary action.
- Loading keeps the final pane geometry stable; use subtle skeleton blocks without dramatic shimmer.
- Error retains successfully loaded surrounding panes and places retry near the failed unit.
- Offline/reconnecting state remains visible in the shell and does not erase the current screen.
- Long operations surface progress and allow navigation away when the backend supports it.

## Motion and interaction

The still-image corpus does not prove exact duration or easing. Until flow recordings are captured, retain restrained provisional values and label them provisional in implementation notes:

- Hover/focus surface change: existing 120 ms token.
- Menu/popover entry: existing 160 ms token.
- Dialog/sheet entry: existing 200 ms token.
- Pane transition: opacity plus no more than 8 px translation; avoid scale-heavy page motion.
- Library expansion: height/grid reveal plus opacity, with no arrow rotation.
- Press response: subtle 0.99 scale only on pointer activation.

Every transition must stop under `prefers-reduced-motion`. Do not invent spring constants or claim exact Intercom motion until video evidence exists.

## Responsive adaptation

The approved Mobbin set is desktop-only. Responsive behavior below preserves HolyArrs functionality and translates the hierarchy; it is not claimed as an Intercom match.

### Desktop, 1180 px and wider

- Show global rail and contextual sidebar.
- Use list + detail + inspector for queues and media detail when useful.
- Keep settings tree visible.
- Let inspector collapse before the result list.

### Tablet, 768-1179 px

- Keep a 64-76 px global rail with accessible tooltips.
- Replace the contextual sidebar with a drawer or a compact local header where content needs width.
- Show at most two panes. Inspector becomes an overlay sheet.
- Preserve 44 px touch targets and never depend on hover.

### Phone, below 768 px

- Keep the existing top bar and bottom destinations as the reliable navigation frame.
- Use full-screen drill-in: list -> detail -> inspector, with an explicit back path and retained scroll state.
- Put secondary actions in a labeled sheet, not a row of tiny icons.
- Forms use full width and a sticky action footer where submit context would otherwise scroll away.
- Library remains expandable in the full navigation menu; its children navigate directly to Movies, TV Shows, and Music.

## Route-to-pattern mapping

| HolyArrs route/family | Intercom-derived pattern | Adaptation |
| --- | --- | --- |
| Root `+layout` | Global rail + contextual sidebar + rounded workspace | Major areas in rail; route-local destinations and badges in contextual sidebar |
| `/` | Reports dashboard (`280`, `440`) | Health, activity, queue, storage, and schedule metrics; no fake analytics |
| `/library` | Content/Contacts list (`160`, `400`) | Media grid/list toggle, filters, bulk actions, actual posters and state |
| `/library/[id]` and album/collection detail | Inbox/editor multi-pane (`40`, `160`) | Library list, media detail, operational inspector containing files, downloads, subtitles, history |
| `/library/held`, `/turned-down`, `/copies`, `/missing-subtitles` | Filtered Inbox or Contacts queue | Shared queue shell, clear selection, inspector-based resolution |
| `/library/rename`, `/import` | Editor plus side inspector (`160`, `320`) | Preview changes in the main pane, options/actions in inspector |
| `/search` | Search/list workspace and command pattern | Search field remains primary; results use media art with operational metadata |
| `/wishlist` | Inbox queue | Wishlist list + selected request detail + approval/status inspector; member permissions preserved |
| `/downloads` | Inbox queue | Active queue with compact status, progress, source, and actions |
| `/downloads/history` | Content/table pattern | Dense sortable history table with filters and detail drawer |
| `/downloads/decide` | Inbox + right inspector | Decision queue, release evidence in detail, resolution controls in inspector |
| `/calendar` | Reports canvas | Date controls and event density; list fallback for narrow screens |
| `/people` and `/people/[id]` | Contacts (`400`) | Person list/profile, filmography and related library items |
| `/settings` | Settings tree (`480`, `520`) | Context sidebar grouped by HolyArrs domains; overview in workspace |
| `/settings/*` | Settings form + preview/editor sheets | Shared headers, grouped fields, sticky actions; retain all real settings and permissions |
| `/account` | Personal settings | Small focused form inside the same system; members do not see admin-only groups |
| `/login`, `/invite/[token]`, `/welcome` | Minimal auth (`560`) | HolyArrs mark, centered form, clear validation; welcome can add a short setup sequence |

## Permission and lifecycle rules

- Admin and household/member views use the same visual system. Permission changes destinations and actions, not the entire product shell.
- Requested and approved media can appear both in Wishlist (your own asks) and Library. The item detail communicates availability, approval, search, download, file, and subtitle state in one lifecycle.
- A missing approved title must remain visibly requested/approved. Do not reduce it to a generic missing state.
- Hide actions the current user cannot perform. Do not show decorative controls that fail later.
- Preserve part-off, safe-mode, URL-base, live status, session, and direct-route gates from the actual app.

## Implementation guardrails

1. Build the shell and tokens first; they affect every route and are easy to compare.
2. Establish one dense queue (`/downloads/decide` or `/wishlist`), one media detail, one settings form, and one dashboard as fidelity anchors.
3. Reuse those components throughout the route families. Do not restyle each page independently.
4. Keep API calls, permissions, and actions intact while changing composition.
5. Use TMDB/media-server imagery already supplied by real APIs. Do not bundle Intercom/Mobbin imagery or customer avatars.
6. Validate at desktop, tablet, and phone widths, but score visual fidelity against desktop references only until responsive references exist.
7. Record exact unknowns in each visual verdict. A high score cannot convert unverified font or motion values into facts.

## Open evidence gaps

- Exact authenticated November2025 Intercom font family, licensed files, axes, tracking, and rendered metrics; current public app CSS declares Graphik.
- Exact animation durations, easing functions, stagger, sheet transitions, and focus choreography.
- Native tablet and phone behavior for this Intercom web version.
- Computed focus ring colors/offsets and high-contrast mode behavior.
- Exact source palette values; screenshot pixel samples may include compression and color-profile changes.
- Drag, resize, and keyboard behavior of multi-pane layouts.

These gaps do not block the shell and component build. They do block any claim of pixel- and motion-exact reproduction.
