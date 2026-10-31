# Mobile Requests and Downloads lists

Preview 18746 only, branch `codex/mobbin-product-review`. Requests rows use full-width phone action buttons rather than squeezing Review, Approve and Turn down beside one another. Phone artwork is 44px wide, leaving more room for the title and status. Both lists wrap long status labels inside their rows. Download episode/track statuses move below their labels, and import rows can wrap. Existing actions and larger-screen layouts remain.

No API, backend, data, dependency or authentication changes are needed. Requests changes apply through 600px; Downloads changes use the existing 620px breakpoint.

## Evidence

Before run `before-841b4b18-8f59-46ff-a867-512a000e562e` retained eight screenshots and four failed assertions. At 320px, Requests buttons broke words into several short lines. The original geometry-only assertion missed this because the text technically wrapped inside its button; the strengthened check also rejects excessively tall short-label controls. Normal Downloads rows already fit, while a browser-only long-status case exposed overflow that the new wrapping rules prevent.

Final run `after-3d26f3a4-a513-4ef5-95f9-0664c7f58bf4` passed **63 checks**, with **32 screenshots**, against frontend SHA256 `43fc2da6e276f68d58177579e4144a77925fb23a20fbec14d08d8a68f890970b`. The checked source was uncommitted on parent `c86429d4e0feb3a00ad542dcb2b044299e0407cf`; the final delivery commit is named in the shared integration handoff.

- At 320, 360, 390, 430, 600, 620, 621, 834 and 1440px: eight waiting requests, all 30 requests, and 37 download/import rows have fitting statuses/buttons and no horizontal document scrolling.
- Browser-only long unbroken names and long decision statuses fit at 320px. These temporary DOM changes never reach a backend or database.
- At 360px, Requests Review and Approve dialogs open and close/cancel; Downloads Details and Remove dialogs open and cancel. No confirmation is submitted. Three actual episode/track disclosures open, with their nested status badges included in geometry checks.
- Zero browser runtime errors or blocked out-of-scope/write attempts. One isolated headless browser with sequential disposable contexts; no credentials or sessions supplied. Only preview-origin GET/HEAD and explicit artwork-host image GETs are allowed.
- `npm run check`: zero Svelte errors and warnings. Node syntax, diff whitespace and secret checks pass. Independent source/harness review approved. Representative Wishlist, Downloads, expanded-track and long-label screenshots were visually inspected.

## Reproduce and review

```powershell
node docs/design-review/intercom/mobile-activity-fit.cjs
```

`--before` records failures without throwing, for comparison against the original source. Raw receipts/screenshots stay under ignored `.omx/artifacts/visual-ralph/intercom/mobile-activity-fit/<run-id>`. The [screenshot gallery](http://10.10.25.25:18746/preview-audit/mobile-activity-fit/index.html) and [Tailscale mirror](http://100.87.70.86:18746/preview-audit/mobile-activity-fit/index.html) export only images and a static index. Stress-case captions identify browser-only content.

This is a layout and non-saving interaction regression, not another lifecycle-save test or combined integration certification. No password, real media, installed app, saved preview or backend was altered. No heavy build, installation, push or publication occurred. Integrate the exact commit and recheck the combined app.
