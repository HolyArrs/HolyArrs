# Contributing to HolyArrs

Thanks for wanting to help. This page gets you from a fresh clone to a pull request.

## Before you write code

1. Open an issue first, or comment on one that exists. Say what you want to change and why.
2. Wait for a reply, so you don't build something that won't be merged.
3. Keep the pull request small. One fix or one feature per pull request.

Questions go to [Discord](https://discord.com/invite/mthsvesKpC). Ideas go to [r/HolyArrs](https://www.reddit.com/r/HolyArrs/). Issues are for bugs and the roadmap.

## Setup

You need Go (version in `go.mod`), Node.js 22 and make.

    git clone https://github.com/HolyArrs/HolyArrs.git
    cd holyarrs
    make ui build

That builds the web app and the program into `bin/holyarrs`. On Windows without make, run the two steps by hand:

    cd app/frontend; npm ci; npm run build; cd ../..
    go build -o bin/holyarrs.exe ./app/backend/cmd/holyarrs

Start it with `bin/holyarrs` and open http://localhost:6262. The first person to open a fresh install becomes the owner.

For live reload while you work on pages, see [docs/dev/setup.md](../docs/dev/setup.md).

## Where the code lives

Every feature is a module, and a module has the same name everywhere:

- `app/backend/internal/<module>/` holds the server code: `routes.go`, `service.go`, `store.go`, `types.go` and their tests.
- `app/frontend/src/lib/<module>/` holds the page parts. `index.ts` is its front door.
- `app/tests/<module>/` holds the browser tests.
- `docs/<section>/` holds the user docs (a Starlight site, flat by section).

So if the Discord alert says the wrong thing, look in `alerts`.

Rules that keep it that way:

- A module talks to another module only through its service (Go) or its `index.ts` (frontend). Never through its store or its files.
- SQL lives only in a module's store files, over that module's own tables.
- A page part goes in `lib/ui` only when two or more modules use it.
- No `util`, `common`, `shared` or `helpers` folders.
- Every outside call goes through `core/outbound`.

The module list and the order modules may depend on each other are in [docs/dev/modules.md](../docs/dev/modules.md). Tests check these rules, so a wrong import fails the build.

## Tests to run

Before you open a pull request:

    go vet ./...
    go test ./...
    cd app/frontend
    npm run check
    npm test

If you changed a page, also run its browser tests:

    npm run e2e -- --project=<name>

More in [docs/dev/testing.md](../docs/dev/testing.md).

## Pull requests

- Link the issue.
- Say what changed and how you tested it.
- Add or update tests for what you changed.
- Write comments that explain why the code does what it does.
- Leave the app's wording alone unless the issue is about wording.

## Licence agreement

On your first pull request, a bot asks you to sign our contributor licence agreement ([CLA.md](CLA.md)). You reply with one sentence and you are done. You keep the copyright to your work. The agreement lets us offer HolyArrs under the AGPL and under a commercial licence. Anything we merge stays available under the AGPL too.

## Code of conduct

Be kind. Read the [code of conduct](CODE_OF_CONDUCT.md).
