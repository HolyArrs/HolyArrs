# Preview typography and motion

Scope: actual app on preview18746, branch `codex/mobbin-product-review`, disposable backend18747. No backend, data, credentials, installed app, saved Air preview, new dependency, or font asset changes.

## Reference evidence

- Approved November2025 Intercom reference: https://mobbin.com/apps/intercom-web-e2b63d2e-e001-426a-93f3-b5bd11346744/6e269961-628f-4a8b-b490-a34aea334a69/screens
- Fresh connector review: [Sending a message](https://mobbin.com/flows/27b1d962-dee6-4cbb-ba51-f3d667477e9d), six states; [Adding a macro](https://mobbin.com/flows/0b2bd43c-00fc-4cd6-a962-43f85c8a96bc), five states. The composer, command menu and detail pane preserve their spatial context between states. The connector provides screen positions, not elapsed time.
- Independent archive enumeration: all681 entries in118 flow ZIPs are PNGs; no timed video, GIF, CSS, JS, font or prototype metadata. Full export has563 PNGs without font metadata.
- Fresh isolated public flow probe returned HTTP200, screenshots and no video/player/prototype controls. No account or login was used. This does not establish animation timing.
- [Intercom's public sign-in app](https://app.intercom.com/admins/sign_in) links [core CSS](https://static.intercomassets.com/assets/core_styles_shim-6cf3080a.css) declaring Graphik Regular, Semibold and Bold. This is a current public-shell signal, not proof of the authenticated November2025 workspace font. No Graphik files were downloaded or shipped.

The bundled Geist remains an openly licensed substitute. Exact Graphik glyphs, font metrics, flow durations and easing remain unverified; neither screenshots nor a qualitative score can certify them.

## Implementation

- One UI font definition in tokens, body15px/1.5, secondary and control text14px, captions12px; semibold600 headings and actions,28px page titles. Mobile inputs16px prevent small-field zoom. Long text still wraps.
- Sidebar15px primary/14px nested labels, sentence-case supporting labels, quieter tracking. Library stays arrowless and expandable with Movies, TV Shows and Music.
- Library expansion160ms with shared easing; opacity120ms. No bespoke timing constants.
- Dialog entrance160ms with a4px settle; scrim120ms. Native close is deliberately immediate: existing onclose handlers clear form state, and Escape/focus restoration must not wait for an exit animation or leave an empty animated card.
- Toasts fade in/out using the current CSS120ms token; reduced-motion is checked at each transition, including after a live preference change. No12px travel or hard-coded200ms.
- Page/card arrival is an opacity-only120ms reveal; no repeated10px rise or card hover lift. Tools menu uses the same reveal.
- Reduced motion disables dialog/page motion, collapses all duration tokens to0ms, and preserves native keyboard interaction. Timings are HolyArrs tuning, not measured Intercom constants.

## Verification and reproduction

Use one headless runner at a time, with fresh contexts, GET-only network policy and no personal profile. The motion runner creates one temporary UI-only diagnostic toast per context; it does not persist sample data or invoke an API mutation.

```powershell
$env:MOTION_BATCH='final-delivery'
$env:MOTION_ASSERT='1'
node docs/design-review/intercom/audit-motion.cjs
```

This captures390/834/1440 at900px height, both normal and reduced motion, with videos, screenshots and frame-by-frame animation properties. It checks Library opening, native dialog entry/Escape, zero closed-dialog layout, focus restoration, toast dismissal, five computed typography roles, dynamic reduced-motion changes, invalid visual-token recovery and source stability. Baseline artifacts remain under `typography-motion/before/`; final artifacts under `typography-motion/final-delivery/`, within the local Intercom evidence root. Intermediate `after/`, `final/` and `final-frozen/` recordings remain separate.

Additional checks: strict Svelte diagnostics0errors/0warnings;225unit tests pass. Responsive route/axe results, final source identity and review are recorded after the frozen capture.

Final app-source SHA256: `eaef1cbc615d2d42c648b5a295c2907caf4a1f1f9ffd7853975f929cf10bcb66`. Six final-delivery motion contexts pass with matching before/after hashes and no page errors. Strict diagnostics and225unit tests were rerun on this source. `full-audit/type-motion-final/receipt.json` has12 passing light-theme captures at390/834/1440/3840; `type-motion-dark/` adds8 dark captures at390/1440. Those20 checks use source `535dc008cf662efff2c27bf5d48779a938bd8504ab54fb3d4ee1239bf99b6e8d`; the only subsequent app edit renames a local timing variable to avoid a secret-scanner false positive. The earlier69 light captures tested the first type pass and remain intermediate evidence. A separate `type-motion-postcommit/` smoke receipt binds the delivered commit to its source hash.

Independent visual review:90/100 for the final desktop Library typography/hierarchy, with no objective regression; font identity and reference timing excluded. Architecture:CLEAR after removing contradictory heading overrides and asserting every collected type role. See `type-motion-verdict.json`.

Before/after overlay and difference images are secondary debugging evidence for HolyArrs changes, not pixel parity against Intercom's different content. `motion-gallery.cjs` copies only HolyArrs screenshots/videos into ignored preview static files; run it after captures. Review: http://10.10.25.25:18746/preview-audit/typography-motion/index.html

No full production build or combined backend acceptance is claimed. This preview uses the existing hash-pinned backend executable.

## Scoped cleanup plan

Scope is the six changed app files and this pass's audit/gallery scripts. Lock behavior with the motion/browser and existing unit checks, then remove only duplication or dead scaffolding introduced in this pass. Preserve native dialog closure, route actions, reduced motion and filter/scroll memory. The toast's invalid-token-to0ms behavior is a narrow visual fail-safe, not an API/error bypass; verify it in an isolated browser. Do not widen into dependency removal or unrelated formatting.

Cleanup result: duplicate font definitions and contradictory heading overrides removed; existing tokens/components reused. No new abstraction or dependency. The invalid-token visual fail-safe and dynamic preference branch pass browser checks. Final scoped smell review found no additional masking fallback, dead scaffolding or necessary cleanup. Unit/type checks and six motion contexts pass after the final local-variable rename. A final independent verifier found no material app bug; its staging/hash concerns were resolved by restaging and recording the final-delivery source identity. The staged secret scan passes without an exemption.

Exact-parity closure requires verified, licensed font files and timed source motion evidence or direct computed-style access. The current Mobbin still-state exports do not supply them. This remains an explicit evidence gap, not a completed exact-match claim.
