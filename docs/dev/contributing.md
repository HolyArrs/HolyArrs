---
title: Contributing
description: Go from a fresh clone to a pull request. Pick an issue, make a small change in the right module and run the checks.
level: Setup
sidebar: { order: 1 }
---

Make one small change in the right place and send it as a pull request.

## Before you start

- Open an issue first, or comment on one that exists. Say what you want to change and why.
- Wait for a reply, so you do not build something that will not be merged.
- Finish [Setup](setup.md) so you can build and run HolyArrs.

Questions go to [Discord](https://discord.com/invite/mthsvesKpC). Ideas go to [r/HolyArrs](https://www.reddit.com/r/HolyArrs/). Issues are for bugs and the roadmap.

## Send your first pull request

1. Find the module your change belongs in. The names in [Modules](modules.md#find-the-right-module) match folder names, so a wrong Discord alert is a change in `alerts`.
2. Make the change in `app/backend/internal/<module>/` and `app/frontend/src/lib/<module>/`. Keep it to one fix or one feature.
3. Build the web app, then the program, and try the change in the browser. See [Setup](setup.md#build-and-run).
4. Run the checks in [Testing](testing.md#run-the-everyday-checks).

   You should see no failure.

5. If you changed a page, run its end-to-end project, for example `npm --prefix app/frontend run e2e -- --project=movies`.
6. Open the pull request. Link the issue, say what changed and how you tested it, and add or update tests for the change.
7. On your first pull request, a bot asks you to sign the contributor licence agreement. Reply with the one sentence it asks for.

:::note
You keep the copyright to your work. The agreement lets HolyArrs be offered under the AGPL-3.0 and under a commercial licence. Anything merged stays available under the AGPL-3.0 too. The text is in `.github/CLA.md`.
:::

## Choices

### Where a change goes

| Change | Go to |
|---|---|
| A new route | [Modules](modules.md#new-route) |
| A new module | [Modules](modules.md#new-module). Open an issue first |
| A held-back feature | [Setup](setup.md#turn-on-a-held-back-feature) |
| A release | [Releasing](releasing.md). Maintainers only |

### What a good pull request looks like

- Write comments that say why the code does what it does. No roadmap numbers, dates, names or em dashes in comments.
- Leave the app's wording alone unless the issue is about wording.
- Keep released API paths, field names, settings keys, table names and job names as they are. Add, never rename.

## If it does not work

**Symptom.** A structure check fails.
**Cause.** The message names the file and the rule.
**Fix.** Move the code, or pass plain data in from the higher module. See [Modules](modules.md#the-checks).

**Symptom.** The web app looks old.
**Cause.** The program serves the build it embedded.
**Fix.** Build the web app again before the program.

**Symptom.** End-to-end tests cannot start.
**Cause.** The test browser is not installed.
**Fix.** Run `npx --prefix app/frontend playwright install chromium`.

## For experts

<details class="ha-more"><summary>Rules a pull request must keep</summary>

- A module talks to another module only through its service (Go) or its `index.ts` (web). Never through its store or its files.
- SQL lives only in a module's `store*.go` files, over that module's own tables.
- A web part goes in `lib/ui` only when 2 or more modules use it.
- No `util`, `common`, `shared` or `helpers` packages.
- Every outside call goes through `core/outbound`.

Tests check these rules, so a wrong import fails the build. The full list and the check names are in [Modules](modules.md#for-experts).

</details>

<details class="ha-more"><summary>What CI runs</summary>

| Job | Runs |
|---|---|
| Web | `check`, `lint`, `test`, `build` and the end-to-end suite |
| Go | `go build`, `go vet` and `go test -timeout 60m ./...` on `amd64` and `arm64`, natively and under emulation |
| Race | `go test -race ./...` |
| Contract | The `openapi.json` match, scope checks on every route, the Downloader suite |
| Fault and scale | Crash, slow write, bad outside service and memory runs |
| Image | A development image builds, starts and reports its version |

Details of the long runs are in [Testing](testing.md#for-experts).

</details>

## Related

- [Setup](setup.md)
- [Testing](testing.md)
- [Architecture](architecture.md)
