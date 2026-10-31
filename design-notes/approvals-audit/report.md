# Approvals audit (31 Oct 2026)

Fix commit: 148da715. svelte-check 0 errors; vitest src/lib 1244/1244. CLS 0, no long tasks, no sideways scroll at 1920, 1440, 1024, 768, 390, 360 (dark) and 1440/390 (light).

| # | Issue | Severity | Status |
|---|---|---|---|
| 1 | Failed chip drawn as a red blob (tab dot style leaked) | High | Fixed |
| 2 | Phone: Failed status strip collapsed, Decline off screen | High | Fixed (buttons on own row, icon Decline) |
| 3 | ← → did nothing on Pending window | High | Fixed |
| 4 | Declined window said "What was approved" | Medium | Fixed ("What was asked") |
| 5 | Touch targets under 44px on coarse pointers | Medium | Fixed |
| 6 | Read-only list opened on an album nobody asked for | Medium | Fixed |
| 7 | Ready hint said "4 episodes available" | Medium | Fixed ("4 more available") |
| 8 | Stray period before the hint | Low | Fixed |
| 9 | Card said "Declined by showcase-owner" | Low | Fixed (card: Declined; window names who) |
| 10 | Failed always "Couldn't find it." | Low | Fixed (download failure: "The download failed.") |
| 11 | Screen-reader step message one ahead | Low | Fixed |
| 12 | Sentence shows "Leo (Kid)" with the account tag | Low | Open |

Not verified: measured contrast ratios, real screen reader, a partly-kept season/album on screen.
Test data trimmed to 13 asks (backup: holyarrs-before-trim.db).
