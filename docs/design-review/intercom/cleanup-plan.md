# Scoped cleanup plan

Scope: only files changed on `codex/mobbin-product-review` since the saved Air baseline. Preserve APIs, request lifecycles, permission checks, URL-base routing, dialogs, navigation expansion, forms and error handling.

Behavior lock: strict Svelte check passes with no warnings. The existing nine formatting/token regression tests pass. Read-only responsive captures and an independent architect review establish the remaining visual and behavior boundaries before cleanup.

Order:
1. Inspect changed code for dead styles, stale comments, duplicate wrappers and masking fallbacks. Classify compatibility branches before touching them.
2. Delete proven dead additions and normalize whitespace only where needed. Do not extract new abstractions merely to reduce repeated route-scoped styles.
3. Preserve semantic status colors, the colored HolyArrs mark and accessibility corrections even where screenshot colors differ.
4. Re-run strict diagnostics, targeted regression checks and responsive checks after any source change.

No dependency, backend, API, database or unrelated cleanup is authorized by this pass. Existing unknown font/motion values remain explicit evidence gaps. A no-change cleanup result is valid when deletion would weaken function or reviewability.
