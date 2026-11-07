#!/bin/sh
# Word check over plans\ and the screens (3.6.6), run beside the repo.
# CI cannot see the plans folder (it is not in the repo), so this runs the
# same check locally with the plans folder required: the web app source,
# docs, README, openapi.json and ../plans must use no word glossary.md
# lists under Avoid. Exit 0 clean, non-zero with file:line for each hit.
set -e
cd "$(dirname "$0")/.."
HOLYARRS_REQUIRE_PLANS=1 go test -count=1 -run 'TestNoBannedWords$' -v ./internal/api ./web | grep -E '^(=== RUN|--- |ok|FAIL|PASS)|uses "' || true
HOLYARRS_REQUIRE_PLANS=1 go test -count=1 -run 'TestNoBannedWords$' ./internal/api ./web >/dev/null
(cd web/app && node scripts/check-words.mjs)
echo "word check over plans and screens: clean"
