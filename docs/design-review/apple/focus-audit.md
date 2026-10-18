# Focus audit (2026-10-18)

Owner report: clicking into the search field (and other places) showed a ring "like accessibility is enabled".

## What the audit found

- Every focus style in `web/app/src` already used `:focus-visible` (56 rules). There was no plain `:focus` ring. The only `:focus` rule is `routes/library/+page.svelte:503` (`.preview h2:focus { outline: none; }`), which removes a ring. The only `:focus-within` rule is `routes/library/+page.svelte:517`, which shows the preview button and is not a ring.
- No script adds focus classes.
- The cause: browsers always treat a **text field** as `:focus-visible`, even after a mouse click, because you are about to type. So the ring showed on a clicked search field, the ⌘K palette input (opened by clicking the top bar search), Sheet fields focused when a Sheet opens, and settings inputs.
- Buttons, links, chips, cards, tabs, the sidebar, Menu and Popover items already behaved: no ring after a mouse click, ring after Tab. Chrome's own heuristic covers programmatic focus (a Sheet focusing its first button, a Menu focusing its first item) after a pointer press.

## Change

- `web/app/src/lib/focus.ts` (new): `trackInputModality()` sets `<html data-input="keyboard|pointer">`. It becomes `keyboard` on Tab, ⌘K / Ctrl+K, and arrow, Home, End, Page, Enter or Space keys outside a text field. It becomes `pointer` on any mouse, pen or touch press. Typing never changes it. Unit tests are in `web/app/src/lib/focus.test.ts`.
- `web/app/src/routes/+layout.svelte:32,146`: calls it once on mount.
- `web/app/src/app.css:259-265`: while the last input was not the keyboard, a focused text field (input or textarea, not checkbox, radio, range or file) has no ring. It gets a quiet Apple-style change instead: a stronger border and a `--surface-1` fill. With the keyboard, the normal `--focus-ring` shows, so WCAG 2.4.7 still passes.

## Checked with Playwright on the live preview (1440, light, read-only)

| Step | Focused | Ring |
|---|---|---|
| Click the top bar search (palette opens) | palette input | none |
| ⌘K / Ctrl+K | palette input | ring |
| Click Settings "Search settings" | `#settings-find` | none |
| Tab from there | next button | ring |
| Shift+Tab back | `#settings-find` | ring |
| Click the Search page field | `#q` | none |
| Click a SegmentedControl choice | radio button | none |
| Click the Library Add menu | first menu item | none (items use the inset hover style) |
| Escape closes that menu | trigger | ring (keyboard action, expected) |

Screenshots: `releases/app-redesign-20261016/apple-direction/final/focus/`.
