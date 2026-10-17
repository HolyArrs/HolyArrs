# Disposable preview real-save verification

This pass exercises genuine UI writes against preview 18746 and its existing loopback backend on 18747. It follows the earlier interaction pass, whose writes were intercepted. This pass does not substitute successful API responses.

## Scope and safeguards

- Verify the owned worktree, branch, port ownership, preview proxy and pinned backend binary before any fixture mutation.
- Require the exact disposable database and three temporary Library folders, one loopback fake Source, one loopback fake Downloader, and disabled search on approval.
- Save a consistent private database baseline under the ignored run evidence directory. It is never copied to the served gallery.
- Bootstrap two one-use member invitations. The actual invitation forms create the new disposable members. A failed run may continue against its original baseline using fixture sessions for those same members; this is explicitly recorded as fixture authentication, not new login proof. Original users, passwords, keys, authentication settings and sessions are preserved.
- Allow only those new members' scoped request, alert-destination and message-read writes, plus owner decisions on registered new requests. Reject baseline request IDs and candidate reference collisions before decisions.
- Compare original sensitive rows, original requests and requester links, stable Library/download/import identities, and temporary media inventory after the flows.
- Disable only the two newly created test members and revoke only their sessions at the end. Keep the sample requests and approved Library titles for review.

## Scenarios

1. Save a new member's alert destination, refresh to confirm persistence, then clear and save it again.
2. Request a movie, join it as a second member, approve it as owner, and verify its Library relationship, approval badge and both requesters.
3. Request and approve a selected TV season; verify the selected scope survives.
4. Request and approve a selected album; verify the selected scope survives.
5. Turn down a request with a note; confirm the member sees that saved decision and the title does not enter Library.
6. Withdraw a pending request; confirm it and its requester link are removed and the title does not enter Library.
7. Mark the new member's lifecycle messages read, then refresh and check the saved read state.

Library cards and list rows are checked at 360px, 834px and 1440px. Screenshots retain the corresponding browser state. The test uses disposable metadata fixtures and no real media download.

## Reproduction

The lifecycle candidates must be absent from the preview before starting. This is an additive suite against the retained disposable preview; it refuses to overwrite baseline data. Keep exactly one headless browser suite active.

```powershell
$env:HOLYARRS_REAL_SAVE_ALLOW = 'preview-18746'
node docs/design-review/intercom/real-save-runner.cjs
Remove-Item Env:HOLYARRS_REAL_SAVE_ALLOW
```

Use the returned run ID with `node docs/design-review/intercom/real-save-gallery.cjs <run-id>` only after the receipt passes. Raw baselines and receipts stay under `.omx/artifacts/visual-ralph/intercom/real-save/<run-id>`; the gallery exports only sanitized results and PNGs.

An incomplete run can continue with `HOLYARRS_REAL_SAVE_RESUME=<run-id>` alongside the opt-in above. This requires verified cleanup with both test members disabled, the same frontend/backend process identities and source hash, and the same private baseline. It reactivates only those two run members, creates temporary fixture sessions, and preserves their previously saved requests and decisions. Completed flows are rechecked without creating duplicate requests. Finalization disables them and revokes their sessions again. Earlier receipts and diagnostic screenshots remain available.

## Evidence limits

The backend executable is the existing preview binary with SHA-256 `D1FA075167517E84FD834EC112C211EE84F27AA8BE8191E07DB9AE62E316C914`. This pass checks that binary together with the frozen frontend source recorded in its receipt. It does not certify a later combined integration build, live metadata providers, downloads/imports, the installed app on 18730, or the saved Air preview on 18745. No backend source, dependency, installed-app or real-media change is part of this work. Media preservation compares paths, sizes and modification times; it is not a byte-for-byte media audit. The TV fixture contains one season, so it proves selected-season persistence but cannot prove excluding another season. Music includes both a selected and an unselected album.

## Results, 2026-10-17

Run `30bd5412-2e9a-4240-9579-257d5a5f279b` passed all eight final named checks. Across the initial execution and its continuations, the runner recorded **22 accepted UI writes**, all passed through to the actual preview backend, with no substituted success response. There were no browser runtime errors or out-of-scope attempted writes.

| Request | Saved outcome | Library result |
| --- | --- | --- |
| Copper Lighthouse, request 27 | Two members joined one request; owner approved | Title 161; Approved, Missing and both requesters |
| Moss Parade, request 28 | Season 1 approved | Title 162; two missing episodes, Season 1 kept up to date |
| Copper Wren, request 29 | Low Orbit album approved | Title 163; selected album kept up to date; Wire left off |
| Velvet Tide, request 30 | Turned down with saved note | No Library title |
| Northern Kettle, request 31 | Member took back pending request | Request and requester link absent; no Library title |

The member alert destination saved and survived refresh, then was cleared and saved again. A genuine mark-all-read save returned 204; a retained continuation's passing message-read subcheck recorded four unread before the save and zero after refresh, and the final continuation confirmed zero remained saved. Member views lacked owner approval controls. Library cards, list rows, requesters and the requester filter were checked at 360, 834 and 1440 pixels. At 360 pixels the list intentionally scrolls horizontally: the initial viewport partly clips the State column, while card view shows the badges directly. DOM presence assertions do not prove full badge visibility before scrolling that list.

The independent database check found exactly four retained new requests and five requester links. Original users (including password fields), settings, secrets, API keys, requests and requester links were preserved. Stable original Library, slot, import and download identities were preserved. Existing sessions remained present, temporary media inventory was unchanged, and listener/process ownership plus the backend binary hash were identical before and after. Finalization disabled only member IDs 5 and 6 and revoked their two fixture sessions; reviewable requests and approved Library titles remain.

The checked frontend was commit `5685bf7188c753ca789a0e39b6954cd990d2b3fe`, branch `codex/mobbin-product-review`, source digest `7e39bb2cb51d787f61ae9f35e0995cf4fb12dd3150fd1c276e4cb7d01c387ed9`. This delivery adds the runner, fixture helper, lifecycle checks, gallery exporter and this report. It changes no product source or dependencies. No backend interface change is required.

### Review evidence

- Home network: [Screenshots and sanitized results](http://10.10.25.25:18746/preview-audit/real-save/index.html).
- Tailscale: [Screenshots and sanitized results](http://100.87.70.86:18746/preview-audit/real-save/index.html).
- Ninety screenshots are retained: 87 flow/diagnostic captures and three focused, read-only review captures after cleanup. The focused captures appear first. Earlier diagnostic captures are identified and are not presented as the final passing state.
- A fresh isolated headless gallery smoke check passed at 360, 834 and 1440 pixels: image loading, width filtering, no document overflow, no browser errors and no credential fields or private database/evidence paths in the results export. Raw baselines remain private and ignored.

### Harness repairs and retained diagnostics

The first fixture setup used a timestamp precision the backend rejected; that attempt created no members or requests. Subsequent execution created both members through real invitation forms and genuinely saved/joined/approved the movie. The initial artwork assertion assumed the fake metadata populated a title artwork array; it does not. The invalid assertion was removed without adding artwork or changing product behavior. A cleanup SQL call lacked its parameter binding; it was repaired and cleanup was rerun successfully before continuation.

The same baseline/run was retained for three continuations. The message-read test was corrected to await the asynchronously loaded control and its save response. The independent database check was corrected to compare stored album-object identifiers, treating the backend's non-music null album field as an empty selection. No saved request, approval or original credential was reset to accommodate these repairs. Each continuation used fresh fixture sessions for the same two disposable members; this is not a new password-login test. Private attempt receipts and diagnostic screenshots remain available.

The fake metadata fixtures return no new-title artwork, so the new titles use the existing poster fallback. This pass proves saves and lifecycle relationships, not artwork-provider population. A non-blocking backend wording issue is visible in TV/music history: the scan event says "Looked at the movie folder." Record wording should become neutral or media-specific in the backend integration lane; no new interface field is needed for the verified flows.
