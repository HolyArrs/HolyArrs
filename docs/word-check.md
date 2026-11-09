# Word check (for people working on HolyArrs)

HolyArrs uses its own words (see `glossary.md` beside the repo). A few upstream coined words must never appear on screens, in the docs, in the API file or in the plans.

## What checks it

- `go test ./app/backend/cmd/holyarrs -run TestNoBannedWords`: the web app source, `docs/`, `README.md`, every name and description in `openapi.json`, and the `plans` folder when it sits next to the repo.
- `go test ./web -run TestNoBannedWords`: the built app in `web/dist`.
- `npm run build` in `web/app` runs `scripts/check-words.mjs` over the source and the built app.

CI runs all of them, but CI has only the repo, so the plans part is skipped there.

## Run it locally, plans included

```
sh scripts/word-check.sh
```

It sets `HOLYARRS_REQUIRE_PLANS=1`, so the check fails if the plans folder is missing instead of skipping it, and prints `file:line` for each word found. The few allowed places (rule lines that name the words to avoid, and the research-side run plan) are listed with their reason in `wordExceptions` in `app/backend/cmd/holyarrs/words_test.go`; add one only with a logged decision.
