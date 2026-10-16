# HolyArrs app redesign: direction review

Owner: redesign chat `01a146e0-0468-7e42-a35b-dab4c289493a`.
State: direction preparation; Luis review required before product rollout.
Source: `4b660863ab8c73977e9e2a58aa24811a4d64660b`.
Working copy: `worktrees/app-redesign-review`, branch `codex/app-redesign-review`.

## Scope and stop condition

Prompt 2 owns app frontend across desktop, tablet and phone. This first milestone produces a source-based screen inventory, design choices, responsive review artifact and screenshots. It does not change the implemented product. The installed app and its data stay untouched. Review examples are fictional disposable fixtures and never enter production flows.

## Inventory

| Family | Implemented screens and functions | Design checks |
| --- | --- | --- |
| App shell | Capability-filtered navigation, parts, live connection state, safe mode, account, theme, sign out | Desktop groups; tablet rail; five phone destinations with More; skip link; focused menu return |
| Home | Setup steps, downloads, searches, missing titles, events and health | Prioritize decisions and failures; setup replaces empty content; keep real status visible |
| Library | Kind/state filters, name search, poster/list view; held, turned down, subtitles, import, rename, file codecs, movie series and people | Group tools; retain direct actions; long names; large library; grid/list parity |
| Title | Movie/show/artist; versions, seasons, episodes, albums, releases, subtitles, files, edit/remove | Clear identity and current state; reveal detailed actions without losing them; preserve confirmation scope |
| Search and people | Movie/show/music search, add/ask dialogs, people and filmography | Keyboard results; appropriate role actions; loading/empty/error; selection return |
| Downloads | Active/finished/import rows, nested episodes, progress, remove and mark failed | Text state plus color; full release names; touch actions; removal consequences |
| Decisions and history | Retry, choose release, version, turn down, stop; filters, details, pagination | Recovery adjacent to cause; no destructive one-click shortcut |
| Calendar | Month/list, month controls, kind filters, phone feed | Desktop month; tablet readable cells; phone agenda; preserve feed function |
| Wishlist | Personal asks or approval inbox, filters, messages, approve/turn down | Member/admin variants; long names; badges; accessible decision dialog |
| Library operations | Import scan/adopt, rename plans, copy selection and bulk search | Staged preview/review/apply; preserve protection, warnings, cancellation and exact file scope |
| Settings | Account, people/users, linked apps, folders, pick rules, naming, music, subtitles, Wishlist, lists, schedules, parts, sign-in, alerts, health/logs, backups, migration, API | Searchable section index; persistent section selection on tablet/phone; credentials remain masked; separate test/save feedback |
| Access | Sign-in/OTP, SSO/Plex/proxy, invitation, first-run welcome | Actual available options only; forms, validation, password reveal, recovery and keyboard access |

Source inspection identifies a 232px desktop sidebar and a seven-item possible phone bar; settings section navigation disappears below 900px. These are source findings, not live-browser failure claims.

## Direction choices

1. **Media workbench (recommended).** Keep the established red mark, charcoal surfaces, Sora headings and Geist body. Put media identity first, with disciplined operational rows and restrained state color. A narrow tablet rail and five-destination phone bar protect useful space. Fits both browsing and frequent administration.
2. **Cinema-led.** Larger artwork, prominent title backdrops and fewer visible details. More atmospheric, but slower to scan decisions, paths, downloads and large libraries. Would need a distinct compact operations treatment.
3. **Compact utility.** Smaller type, mostly tables and little artwork. Efficient for mouse and keyboard administration, but weak for family browsing and touch use. Would need a roomier touch mode.

## Proposed system

- Color: charcoal canvas `#111316`, panels `#191c20`, warm-white text `#f0eee9`, readable secondary text `#b1b2b5`, red action `#cc342b`. Green/amber/red indicate actual state alongside words/icons. No decorative gradient panels.
- Type: existing bundled Sora for title identity, Geist for controls/body, tabular figures for progress and counts. Headings 28–36px desktop, 26px phone. Controls/body 14–16px, supporting text at least 12px.
- Shape and space: 4px rhythm, 6px controls, 10px surfaces; borders for grouping; shadows reserved for popovers/dialogs. Avoid putting every fact inside a card.
- Desktop from 1180px: 216px grouped sidebar; 64px context header; content follows screen purpose, allowing dense tables to use width and forms to stay readable.
- Tablet 720–1179px: labelled 80px rail and full section selector; two-column details only when both columns fit. No desktop sidebar squeezed into tablet content.
- Phone below 720px: compact header, five bottom destinations (Home, Library, Search, Downloads, More for staff); More contains Calendar, Wishlist, History, Decisions, Alerts and Settings/account as allowed. Member destinations remain capability-derived. A labelled menu shows the current location when it lives under More.
- Forms: visible labels, help next to the field, validation next to the cause, bounded form width, explicit save/test outcomes respecting the existing save-then-test sequence and retained input on error. Settings section selector remains present at every size.
- Lists/tables: poster/list parity; consistent sorting/filter placement. Wide comparison tables keep headers and get a named local scroll region; phone tasks use stacked rows where relationships survive. Do not hide meaningful columns without an accessible detail path.
- Media: preserve the existing poster assets in actual implementation. Review art is original geometric cover placeholders, labelled fictional fixtures; no artwork downloads or real library access.
- States: skeleton matches content; empty state distinguishes no library from no filter matches; error retains previous data and offers retry; stale/live state stays readable. Disabled part and safe mode remain explicit.
- Input: minimum 44px hit targets; visible focus; native scroll; reduced motion; Escape and focus return for dialogs/menus; no hover-only actions. Current capability checks, part checks and URL-base handling remain authoritative.

## Review artifact and validation

Review artifact: `worktrees/app-redesign-review/docs/design-review/index.html`. It demonstrates Library, title detail, Downloads and settings with an explicit fixture label and local-only navigation/filter/theme controls. Product network calls are absent. Planned review snapshots cover desktop 1440px, tablet 834px and phone 390px; narrow 320px and text/keyboard/error variants are checked when the bounded browser slot is available.

Use a single isolated headless browser and one page, no build or simulation in parallel. Preview port 18744 was free at intake; check again before binding. Disposable assets and captures belong to this milestone. This is design validation only; no product build, product interaction regression or combined-source acceptance is implied.

## Integration milestones

1. Direction review: this artifact and source inventory; stop for Luis's decision.
2. After approval: implement tokens/shell and representative real routes; run targeted role/part/base-path/keyboard checks; provide exact commit for backend integration.
3. Roll through remaining screen families; preserve real contracts and test state variants; record exact frontend commits and backend needs.
4. Backend combines completed source and reruns final packages, affected workflows and installation. The uninterrupted 24-hour run remains last and belongs to the backend chat.

No API or schema change is currently requested. A new backend need must name endpoint, fields, compatibility and owner in the shared interfaces file. No backend files belong to this redesign branch.
