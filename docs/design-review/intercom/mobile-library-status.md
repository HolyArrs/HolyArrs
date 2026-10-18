# Mobile Library list statuses

Preview 18746 only, branch `codex/mobbin-product-review`. The phone list now stacks each title, its requester and its status badges in one full-width row. Approved and Missing remain visible without scrolling sideways. Long titles, requester names and multiple or long status labels wrap. At widths above 600px the existing table layout remains.

The native table, caption and column headers remain available to assistive technology. Explicit header associations connect the title and state cells. Title links, requester labels, filters, saved views and return highlighting retain their existing behavior. No API, backend, database, dependency or authentication change is required.

## Verification

Before run `before-0ed4fd72-bbb4-4006-b35d-081375056423` reproduced the failure: at 360px the list content was 369px wide inside a 313px region; both status badges extended beyond its right edge.

Final run `after-ee2c6c39-f278-4a64-be5f-03a1b7dfe363` passed **54 checks** and retained **27 screenshots**. Its frontend source SHA256 is `5391def226d2a4675a2b24951fd6273c80abad8d484051d054e136247d9fefd9`. The checked files were uncommitted on parent commit `320e82a62314091e1043fa314834068efbec619a`; the final delivery commit is recorded in the shared integration handoff.

- Actual preview movie, TV and music Library rows at 320, 360, 390, 430, 600, 601, 834 and 1440 pixels: all badges fit inside the list and viewport, with no horizontal list or document overflow. The 360px content now measures 313px inside the same 313px region.
- Rows stack through 600px; tablet and desktop retain table rows. The real approved requests and requesters remain visible for all three media types.
- Keyboard activation opens the movie, and returning preserves the query, list view and last-opened marker.
- Scoped automated accessibility scan of the Library list reports zero WCAG A/AA violations; table caption and State header remain exposed to role queries.
- Browser-only long-title, long-requester and multiple-status stress cases wrap at 320 and 360px. These temporary DOM changes never reach an API or database.
- No browser runtime errors, attempted API writes or out-of-scope network requests were recorded. The frontend source stayed unchanged during verification.
- `npm run check`: zero Svelte errors and warnings. Independent source review found no blocking issue. Representative phone, TV, desktop and long-text screenshots were inspected.

The runner allows only GET/HEAD requests to preview 18746 and image GETs to the existing artwork hosts. It creates an isolated headless context for each width and never supplies credentials, creates sessions, changes data or visits the installed app or saved Air preview. Listener identities remained 18746/PID6760 and loopback backend 18747/PID56444, with the saved and installed app processes untouched.

## Reproduce and review

```powershell
node docs/design-review/intercom/mobile-library-status.cjs
```

Use `--before` only against the unfixed source to record the original geometry failure; it records rather than throws on that expected failed assertion. Raw receipts and PNGs are retained under ignored `.omx/artifacts/visual-ralph/intercom/mobile-library-status/<run-id>`. Two earlier after-attempts retain a failed test assumption about TV's label: TV/music use labels such as `2 missing`; the final assertion accepts that actual wording.

Review the [before-and-after screenshots](http://10.10.25.25:18746/preview-audit/mobile-library-status/index.html), or use [Tailscale](http://100.87.70.86:18746/preview-audit/mobile-library-status/index.html).

This targeted pass supersedes the narrow-list scrolling limitation documented in the earlier real-save report. It does not repeat the earlier lifecycle writes or certify provider downloads, a later combined build, physical devices, all browsers or exact Intercom parity. It changes no password, real media, installed app or saved preview. No backend dependency remains for this layout fix.
