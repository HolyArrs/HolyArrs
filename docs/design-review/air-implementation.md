# Air media baseline

Approved reference: https://mobbin.com/screens/746740ff-0fe1-4ec3-bcb3-350951c7c424

Implemented in the actual app on branch `codex/app-redesign-review`, currently uncommitted. Scope: shared light tokens, application sidebar/workspace, Library search/filter/header/poster layout. Library remains expandable with real Movies, TV Shows and Music routes. Original colored logo and role/part gates remain. No backend interface changes.

The reference baseline is 1920 by 1200, excluding the Mobbin footer. Cool gray canvas, flat sidebar, white rounded workspace, centered search, compact filter toolbar and blue primary action follow Air. Real media posters preserve 2:3 proportions. Unsupported Save filters and decorative controls were omitted. This is an Air media adaptation, not a pixel-identical reproduction.

## Verification

Independent visual score: 91/100, pass. Independent architecture gate: clear after repairing desktop search overlap. Responsive headless checks at 390, 834, 1024, 1100, 1200, 1366 and 1920: zero page errors, no document overflow or search/sidebar overlap. Axe: zero violations. Search empty/recovery, status URL, sort and list switching passed. Svelte check: zero errors/warnings. Unit suite: 223 passed. Production build was not launched: resource reservation expired during preflight; fresh allocation is required. Older build receipts do not certify this source.

Evidence: `.omx/artifacts/visual-ralph/air-media/` contains final captures, reference, overlay/diff, visual verdict, source manifest, interaction results and accessibility results. Capture script is run from `web/app` via Node stdin. No production data/configuration changed.

## Cleanup review

Bounded to the three implementation files. Existing regression checks lock behavior. Removed misleading reset wording and retained mobile recovery, deleted dead prior styles, resolved whitespace. No masking fallbacks or new abstractions/dependencies introduced. Remaining routes still need consistent application of the baseline; full app redesign and combined integration acceptance remain open.
