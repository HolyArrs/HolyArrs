# Executed Intercom preview audit coverage

Status: **covered-checks-clear-with-limitations**
Frontend SHA-256: 45144ef19d962c8cd785e124d352c7f00f42821ea359558b919415073e6f2928

| Suite | Expected | Observed | Result |
| --- | ---: | ---: | --- |
| responsive | 744 | 744 | checks-clear |
| interaction | 243 | 243 | checks-clear |
| touch | 8 | 8 | checks-clear |
| environment | 164 | 164 | checks-clear |
| comparison | 8 | 8 | checks-clear |
| motion | 6 | 6 | checks-clear |
| realSave | 7 | 7 | checks-clear |

Native zoom: unavailable; exact scope in JSON.

## Routes

| Template | Cases | CSS widths | States | Fixture cases | Findings |
| --- | ---: | --- | --- | ---: | ---: |
| / | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /account | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /calendar | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /downloads | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /downloads/decide | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /downloads/history | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /invite/[token] | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 12 | 0 |
| /library | 60 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, requester-menu, list | 0 | 0 |
| /library/collections | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/collections/[id] | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 12 | 0 |
| /library/copies | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/held | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/import | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/missing-subtitles | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/rename | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/turned-down | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /library/[id] | 48 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, edit | 0 | 0 |
| /library/[id]/albums/[album] | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /login | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 12 | 0 |
| /people | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /people/[id] | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 12 | 0 |
| /search | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/account | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/alerts | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, add | 0 | 0 |
| /settings/api | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/backups | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/downloaders | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, add | 0 | 0 |
| /settings/library-folders | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, add | 0 | 0 |
| /settings/logs | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/media-servers | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, add | 0 | 0 |
| /settings/move | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/music | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/naming | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/outside-lists | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, add | 12 | 0 |
| /settings/parts | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/schedules | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/search | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/sign-in | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/sources | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, add | 0 | 0 |
| /settings/sources/[id] | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 12 | 0 |
| /settings/subtitles | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/this-app | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/tmdb | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /settings/users | 24 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base, edit-user | 12 | 0 |
| /settings/wishlist | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /welcome | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |
| /wishlist | 12 | 360, 390, 430, 768, 834, 1024, 1280, 1440, 1920, 2560, 3440, 3840 | base | 0 | 0 |

## Limits

- Intercom source CSS viewport/DPR/zoom, authenticated font and timed easing/duration are unavailable. Geist is an explicit substitute.
- Native mobile/tablet/TV references and physical 4K hardware are unverified.
- Interaction writes are browser-intercepted; separate real-save coverage exercises only the disposable backend.
- Fresh music real-save needs a fourth fake artist identity; all three existing identities are occupied by preserved samples. Music sample states and fixture interactions are covered.
- Settings-only shell exposes Music but API GET requires manage_library; this permission contract remains a backend integration dependency. Role fixtures certify shell behavior only.
- External downloader/indexer/metadata/subtitle/notification delivery remains fake-provider or fixture bounded. No production build or combined backend release is certified.
- Native zoom, if proven, covers measured Chromium zoom and document overflow on four sampled routes; component clipping and complete keyboard behavior at native zoom are excluded.

All prior partial, failed and source-mismatched batches are excluded from final certification.
