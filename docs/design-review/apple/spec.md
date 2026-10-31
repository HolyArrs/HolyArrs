# HolyArrs "Apple" direction: design spec and template rules

Status: approved direction from Luis, 2026-10-18. Branch `codex/apple-direction` (from `3300dd4`). Preview on port 18749, backed by the disposable backend on 18747.

Luis's brief, in short: beautiful, polished, iOS/macOS feel, super clean, super easy for all ages. One template that every screen follows. Beautiful animated buttons and motion. A side menu that collapses and expands.

References: Luis's screenshots (a DVR app's sidebar and Apple TV-like title page, trakt's poster rows, Perplexity's quiet sidebar) plus the Mobbin survey in `releases/app-redesign-20261016/mobbin-200/`. These are references for ideas. **Never copy anyone's layout pixel for pixel, their icons, artwork, words or brand.** HolyArrs keeps its own mark, glossary words (`glossary.md`) and red accent.

## 1. Principles

1. **Media first, work second.** Browsing screens (Home, Explore, title pages) are artwork-led and airy. Work screens (Downloads, Requests, Settings) are calm and dense. Both use the same shell, type, colours, buttons and motion.
2. **One way to do each thing.** One page header, one button set, one card, one shelf, one sheet, one status pill. No page invents its own.
3. **Plain words, big targets.** At least 44px touch targets. Words a non-technical family member understands. No jargon in member views.
4. **Calm motion.** Motion explains (where did this come from, did my tap register). Never decorative loops. Everything respects reduced motion.
5. **Light by default, dark follows the system.** The user can override in Account.

## 2. Tokens (single source: `web/app/src/lib/tokens.css`)

Replace the Intercom token values but **keep every existing variable name** so old components keep working. Add new names where listed.

### Colour

| Token | Light | Dark |
|---|---|---|
| `--bg` (page) | `#ffffff` | `#0b0b0c` |
| `--sidebar-bg` | `#f6f6f7` (translucent `rgb(246 246 247 / 86%)` + `backdrop-filter: saturate(180%) blur(20px)` where supported) | `rgb(22 22 24 / 86%)` + same blur |
| `--surface-1` (cards) | `#ffffff` | `#161618` |
| `--surface-2` (raised, inputs) | `#f2f2f4` | `#1e1e21` |
| `--surface-3` (hover, selected row) | `#e9e9ec` | `#2a2a2e` |
| `--line` | `rgb(0 0 0 / 8%)` | `rgb(255 255 255 / 9%)` |
| `--line-strong` | `rgb(0 0 0 / 16%)` | `rgb(255 255 255 / 18%)` |
| `--text` | `#1d1d1f` | `#f5f5f7` |
| `--text-muted` | `#6e6e73` | `#a1a1a6` |
| `--text-faint` | `#8e8e93` (only for ≥15px or non-essential) | `#7c7c80` |
| `--accent` (HolyArrs red) | `#e5352b` | `#ff453a` |
| `--accent-soft` | `rgb(229 53 43 / 10%)` | `rgb(255 69 58 / 16%)` |
| `--ok` / `--wait` / `--fail` / `--info` | `#248a3d` / `#b25000` / `#d70015` / `#0071e3` | `#30d158` / `#ffb340` / `#ff6961` / `#409cff` |
| `--scrim` | `rgb(0 0 0 / 40%)` | `rgb(0 0 0 / 60%)` |

All text pairs must pass WCAG AA (4.5:1 body, 3:1 for ≥18px or bold ≥14px). Status is never colour alone: always dot or icon **plus a word**.

### Type
- Font: `Inter Variable` is not installed; use the bundled **Geist Variable** with `font-feature-settings: "ss01","cv11"` off, letter-spacing tuned to feel like SF Pro. System stack fallback `-apple-system, BlinkMacSystemFont, "SF Pro Text"`.
- Scale (new names, keep old as aliases): `--t-large-title: 34px/1.12 700, -0.022em` (page titles like "Home") · `--t-title1: 28px/1.15 700` · `--t-title2: 22px/1.2 700` (shelf titles) · `--t-title3: 19px/1.25 600` · `--t-headline: 16px/1.35 600` · `--t-body: 15px/1.5 400` · `--t-callout: 14px/1.4 400` · `--t-subhead: 13px/1.35 500` · `--t-footnote: 12px/1.3 500` · `--t-caption: 11px/1.25 600`. Sidebar section labels are sentence case, 12px 600, muted. No tracked-caps eyebrows anywhere.
- Numbers in tables and progress use `font-variant-numeric: tabular-nums`.

### Space, shape, depth
- Spacing stays on the 4px grid (`--s-1`…`--s-16`). Page gutter `--gutter: clamp(16px, 3vw, 40px)`. Shelf gap 16px desktop, 12px phone.
- Radius: `--r-sm 6px` (chips), `--r-btn 10px`, `--r-card 12px` (posters, cards), `--r-panel 16px`, `--r-modal 20px`, `--r-pill 999px`. Round action buttons are circles.
- Shadows: soft and layered, never harsh. `--shadow-card: 0 1px 2px rgb(0 0 0/6%), 0 4px 12px rgb(0 0 0/6%)`; `--shadow-lift: 0 2px 4px rgb(0 0 0/8%), 0 12px 28px rgb(0 0 0/14%)`; `--shadow-modal: 0 24px 64px rgb(0 0 0/24%)`. Dark mode raises opacity and adds a 1px `--line` ring.

### Motion
- Durations: `--dur-instant 90ms`, `--dur-fast 150ms`, `--dur 240ms`, `--dur-slow 360ms`, `--dur-page 300ms`.
- Easing: `--ease-out cubic-bezier(.22,1,.36,1)`, `--ease-in-out cubic-bezier(.65,0,.35,1)`, `--ease-spring linear(...)` (a CSS `linear()` spring approximation with slight overshoot, fallback `--ease-out`).
- `@media (prefers-reduced-motion: reduce)`: all durations become `1ms`, no transforms, no view transitions, no stagger; opacity changes allowed at 1ms.

## 3. Motion rules (every interactive element)

| Element | Rest → hover | Press | Other |
|---|---|---|---|
| Button (all kinds) | background/colour ease `--dur-fast` | `scale(.97)` `--dur-instant`, release springs back `--dur` `--ease-spring` | focus ring fades in |
| Round icon button | soft fill appears | `scale(.92)` | tooltip after 500ms |
| Poster / card | `translateY(-3px)` + `--shadow-lift`, artwork `scale(1.03)` inside its frame | `scale(.98)` | |
| Sidebar row | fill fades in | `scale(.98)` | selected indicator slides between rows (shared element, `--dur` spring) |
| Sidebar collapse | width animates `--dur` `--ease-out`, labels fade out first then width shrinks; expand reverses | | state saved in localStorage |
| Shelf | native horizontal scroll with `scroll-snap-type: x mandatory`, arrow buttons appear on hover (desktop) | | arrows fade in |
| Page change | View Transitions API: cross-fade + 8px rise `--dur-page`; poster → title hero uses `view-transition-name: title-art-<id>` so the poster grows into the backdrop | | feature-detect, fallback plain |
| Sheets, menus, popovers | open: fade + `scale(.96→1)` from the trigger with `--ease-spring`; close faster `--dur-fast` | | backdrop fades |
| Toast | slide up 12px + fade, auto-dismiss | | |
| Progress bar | width animates `--dur-slow` `--ease-out`; finished → green ✓ pulse once (`scale 1→1.15→1`) | | |
| Lists loading | skeleton shimmer (one subtle sweep, 1.4s), content fades in with 30ms stagger, max 8 items | | |

Implement as: CSS in `tokens.css` + a small `lib/motion.ts` (Svelte actions: `press`, `reveal`, `viewTransitionName`, helper `navigateWithTransition`). **No new dependencies.** Svelte built-in transitions are allowed.

## 4. Shell: collapsible macOS sidebar

### Desktop (≥ 1024px)
- Left sidebar **expanded 260px**, **collapsed 72px** (icons only, labels become tooltips and accessible names stay).
- Toggle: a sidebar icon button at the top of the sidebar **and** in the page top bar when collapsed; keyboard `⌘\` / `Ctrl+\`. State persisted in localStorage (`holyarrs.sidebar`), wrapped in try/catch.
- Top of sidebar: HolyArrs mark + "HolyArrs". Then a **Search** row showing `⌘K`.
- Sections (sentence case 12px muted labels, hidden when collapsed, replaced by a thin divider):
  - (no label) **Home**, **Explore**
  - **Library**: Movies, TV shows, Music, Movie series (only parts that are on)
  - **Your stuff** (members): My requests, Calendar
  - **Manage** (only for people with manage/approve/settings permissions): Downloads, Requests (Wishlist inbox, with count badge), History, Library tools, Settings
- Bottom: account row (avatar, name, role) with a ⌃⌄ menu: Account, Appearance (Light / Dark / System), Sign out.
- Selected row: soft fill `--surface-3`, accent-coloured icon, semibold label. Counts are right-aligned pills.
- Top bar inside the content area (56px, sticky, translucent blur): collapse button (when collapsed), back/forward chevrons on detail pages, search field `Search… ⌘K` opening a command palette sheet, live connection dot, and an activity button showing current download count.

### Tablet (768–1023px)
- Sidebar starts collapsed (72px). Expanding overlays the content as a sheet with scrim, closes on Escape/outside tap/selection.

### Phone (< 768px)
- No sidebar. Bottom tab bar (translucent blur, safe-area aware): **Home, Explore, Library, Search**, and **Manage** (admins) or **Requests** (members). A "More" sheet holds the rest. Large-title page headers that shrink on scroll.

### Accessibility
- `<nav aria-label="Main">`, `aria-current="page"`, the toggle has `aria-expanded` and `aria-controls`. Collapsed rows keep text via `aria-label`. Tooltips are not the only label. Skip link stays first. Focus returns to the toggle after closing the overlay sheet.

## 5. Component kit (`web/app/src/lib/ui/`)

Each component: typed props, light/dark, keyboard, focus ring, reduced motion, and an entry on the catalog page. Existing pages migrate to these; do not create one-offs.

`AppShell`, `Sidebar`, `SidebarItem`, `TopBar`, `TabBar` (phone), `CommandPalette`, `PageHeader` (large title, optional subtitle, actions slot, back link), `Shelf` (title + "See all ›" + horizontal scroller with arrows), `PosterCard` (2:3) and `WideCard` (16:9, with progress line and overlay badges), `PersonCircle`, `Hero` (backdrop, logo/title, meta line, badges, description with "Show more", action row), `Badge` (quality: 4K, HDR, Dolby Vision, Atmos, CC, rating; neutral outlined), `StatusPill` (dot + word; kinds: in library, downloading %, wanted, requested, approved, turned down, failed, coming soon), `Button` (variants: primary filled accent, secondary tinted, plain text, destructive; sizes: lg 44, md 36, sm 30 (sm never on phone)), `IconButton` (round), `SegmentedControl`, `Chip`, `Menu`/`Popover` (closes on pick, Escape, outside click, returns focus), `Sheet` (bottom on phone, centred on desktop, native `<dialog>`), `ListRow` (for work screens: leading art, title, subtitle, trailing status/actions), `Table` (dense, sticky header), `SettingsCard`, `EmptyState` (icon, one sentence, one action), `LoadError` (inline once data exists, full only when nothing loaded), `Skeleton`, `ProgressBar`, `Toast`.

Catalog page: route `/ui` (signed-in only, not in navigation), shows every component in every state, in light and dark side by side, with a width switcher.

## 6. Screens in this milestone

### Home
Large title "Home" (+ greeting line using the person's display name, never a raw username or id). Shelves, each hidden when empty:
1. **Downloading now** (WideCard with progress line, percent, time left; admins and the person who asked).
2. **Up next** / **Coming soon** (next episodes and releases from calendar data).
3. **Recently added** (PosterCard).
4. **Your requests** (members) or **Waiting for approval** (approvers, with count).
5. **Needs attention** (admins: failed downloads, decisions; one tap goes to the fix).
A setup card replaces shelves when the library is empty.

### Explore (route `/explore`)
Behaviour as built (user guide: [Explore](../../explore.md), [Trailers](../../trailers.md)). The first draft of this section described rows from local data and a `GET /discover` fallback; the server now builds the page.
- **Switch:** All, Movies, Shows, Music (`?type=`, remembered per device; Music only when the Music part is on).
- **Head, one request** (`GET /api/v1/explore/head`): a static hero (up to 6 titles not in the Library, Ask as the main button), Just arrived (Library adds in 14 days, newest first), Trending on Netflix (rank cards, movies or shows, region), Your asks on the way (only with open asks), Top picks for you (3+ seeds), Trending this week everywhere, Coming soon (wide cards), then the best tail row. Seven rows at most.
- **Tail** (`GET /api/v1/explore/tail?page=n`): generated rows, 4 per page as the page scrolls, capped by signal (8 under 3 seeds, 20 from 3, 40 from 10; Kids 20). On your services (in-row picker), Best new releases, Because you have X, More from a person, Genre picks, Popular in your household, decades and Hidden gems. A daily shuffle of up to 5 percent; the same row type never twice in a row.
- **Smart row titles:** 585 templates in five layers (occasion, time of day, genre mix, taste, row reason), true to the row's items, no repeat for 14 days; fixed head rows keep their title and get a factual subtitle.
- **Music:** its own head (genre chips, album hero, Just arrived, New from artists you have, Your asks, charts) and a longer tail; square albums, round artists.
- **Cards:** movie and show rows use wide 16:9 cards with the title logo; the ranked row uses rank cards. One freshness strip (New Episode, New Season, Recently Added, Trending, Coming or Premieres, Season N coming, New Release) and one solid round Library mark top right with a download ring; under All a type pill top left. Badge text states facts only.
- **Hover:** a wide card grows about 1.5 times in place after 0.5 s, then shows an info panel and the trailer; a poster opens its card after 0.8 s. Pointer: fine only; focus opens, Esc closes, tap opens on touch; no grow with reduced motion; never mid-drag. Rows scroll by mouse drag with a glide.
- **Trailers:** muted, one at a time, hero after 1.5 s in view; never for Kid accounts, with reduced motion, on slow or data-saving connections, or when **Autoplay trailers** is off. Sound button when the stream has sound.
- **Signals:** Library, asks and the metadata server's charts only. No play history.
- Explore row contracts for the older `GET /discover` are in `backend-needs.md`.

### Title page (movie and show; artist uses the same hero)
- **Hero**: full-bleed backdrop (fallback: blurred poster), gradient for legibility, title (logo art if available, else large title type), meta line (year · runtime · genres), badges row (rating, 4K/HDR/Dolby/Atmos/CC from files when known), StatusPill, description clamped to 3 lines with "Show more", action row of round buttons.
- Actions (by permission): admins — Search now, Monitor on/off, Edit, Library ▾ (move/rename/remove), ⋯ (all existing actions remain reachable). Members — Ask / "Requested" / "In your library". "Open in Jellyfin/Plex" only when that integration is configured. **Every existing title action and confirmation must stay reachable.**
- Below: Seasons (SegmentedControl or menu → episode WideCards with status per episode), Cast & crew (PersonCircle shelf), Versions & files (ListRow), Subtitles, Similar in your library. Breadcrumb "Movies › Title (Year)" at top-left of hero.
- **Asked by** line under the title (library title pages): "You asked", "Asked by Ana", "Asked by Ana and 2 others". The server trims it per reader (owner, admins and managers see all; members see themselves and askers who opted in when the household allows it; Kids see only themselves). Settings: **Show who asked** (household) and **Show my name on my asks**.
- The hero backdrops move slowly (up to four pictures) and can play a muted trailer; see Explore.

### Card system: three card templates

1. **Wide title card** (movies and shows on Explore and Home). The title is always on the art, never under it. The server picks the art once per title and language and keeps it 30 days (`card_art`, `card_logo`, `art_has_title` in the Explore head/tail and Home answers; the Explore pool job fetches the image lists, paced). In order: a backdrop that already shows the title in the user's language or English; else a textless backdrop with the title logo in a fixed box bottom-left; else a textless backdrop with the name in the display font in that same box over a scrim. Under the card, on every wide card alike: the name (one line, ellipsis), then the muted meta line ("Show · 2024 · TV-14"). On hover (after about 400 ms) the card grows in about 220 ms (ease-out cubic-bezier(.2,.8,.2,1)), shrinks back in about 180 ms, and its panel fades in about 120 ms after the grow starts; transform and opacity only, no transform at rest. The grown card keeps the logo or name over the trailer, with no description. Cards are at least 240 CSS px wide, sized by container queries on the row (about 5.5 visible on a wide row, fewer on narrow ones). The status mark is 22px under 280px of card width and 26px above; the freshness strip and Trending tag scale with it, at whole-pixel sizes.
2. **Ranked poster**: the tall poster with its big rank number; keeps its own template.
3. **Music card**: square album cover or round artist photo, name and one reason line under it. Album covers come from the metadata server's cover chain (Cover Art Archive, then Deezer, then iTunes), else the Cover Art Archive alone. Release wording: "Out Sep 25" ahead, "Out now" up to 14 days after, "Released Sep 25" after that. The Music hero only shows an album whose cover loads, else the blurred artist photo behind the words.
## 7. Admins and members: one app, permission-driven

- Navigation, actions and shelves appear by `session.can(...)` permissions that already exist: `ask`, `auto_approve`, `approve`, `manage_library`, `settings`.
- **Manage** section shows only if the person has `manage_library`, `approve` or `settings`; each item has its own permission.
- Same title page for everyone; admin-only buttons hidden (not disabled) for members.
- **Role presets** (display + invite UI only; they map onto existing permission sets, no backend change): Admin (all), Manager (manage_library + approve), Member (ask), Kid (ask, plus an age-rating limit — the rating limit needs backend support; record in backend-needs.md, show the preset as "coming soon" until then).
- Member words: "Get it", "Requested", "Downloading", "Ready to watch". Admin words may be more precise, but always glossary terms.

## 8. Not in this milestone (later, same template)
Downloads, Requests, History, Library list/grid, Search results, Calendar, People, Settings and Access screens move onto the kit in the next milestone. Until then they render inside the new shell with their current content (they must still work).

## 9. Done means
- Svelte check 0/0, unit tests pass, word check passes.
- Catalog `/ui` shows every component, light and dark.
- Home, Explore and a movie and a show title page work at 1440, 1024, 834, 390 and 320, light and dark, with real preview data.
- Sidebar collapses/expands with mouse, keyboard shortcut and touch; state persists; tablet overlay and phone tab bar work.
- axe: 0 violations on the new screens; keyboard walk passes; reduced motion verified.
- Screenshots saved under `releases/app-redesign-20261016/apple-direction/`.
