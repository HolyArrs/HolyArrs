# Real metadata for disposable activity

`sample-real-activity.json` records public TVMaze episode metadata and Apple album/track metadata with source URLs and HTTPS artwork. It replaces the fictional labels in the existing activity examples:

| State | Episode | Music |
| --- | --- | --- |
| Completed | Severance — Good News About Hell | Daft Punk — Discovery |
| Needs a decision | Severance — Half Loop | Daft Punk — Random Access Memories |
| Paused | Severance — Hello, Ms. Cobel | The Weeknd — Dawn FM |
| Downloading | The Bear — System / Hands | twenty one pilots — Clancy / Overcompensate |

Run `python docs/design-review/intercom/sample-real-activity.py` only after the original lifecycle fixtures have been seeded. It is locked to the exact disposable preview database and known fake Source. One transaction updates descriptive title, episode, album, track and release metadata. It verifies that every download row, slot row and import record remains unchanged. The unused fictional special is hidden because it has no real episode counterpart.

Existing completed test files and import plans retain their original internal fixture paths so import evidence stays valid. No real media is downloaded. Album track lists are representative subsets in this preview; they are not complete music metadata coverage. Fake provider identifiers remain test identifiers, not production metadata identifiers. Do not use metadata-refresh/search on these renamed fixture identities to validate real providers.

The normal activity seeder recognizes both original and real labels and can verify the same records after this metadata pass. Its exact settings restoration still applies. The preview Vite configuration adds artwork only; download phases, names and episode/track details come from persisted records. There are no fabricated GET status responses.
