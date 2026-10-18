# Preview downgrade protection checks

Preview 18746; branch `codex/mobbin-product-review`. No product/frontend/backend code changes. The retained disposable artist166 / album144 remains lossless: slots1183–1185 reference FLAC files10–12, all in_library, 3/3 present, better_wanted=0. Request34 stays available.

## Runtime checks that pass

- Automatic album search completes with “no track … is looking for a copy”; no download is created.
- Fresh album results include MP3 320 with passed=false, failing reason match.album_gain and zero eligible slots. A normal choice with force=false returns HTTP400 / matched none.
- At 360×800, 834×1112 and 1440×1000 CSS pixels, the release warning explains that tracks already have copies as good. The check cancels the dialog instead of forcing a choice. Album status remains In library; cards/lists show 3 tracks in library.
- Before/after slot IDs, file rows, complete file hashes, downloads, imports, fake torrent records and unrelated sample records are equal. Original account/password hashes compare equal; disposable access is revoked. Nonvolatile Library-folder configuration is now fingerprinted separately from health refreshes.

Receipts and 12 screenshots per successful run live under `.omx/artifacts/visual-ralph/intercom/music-downgrade/`. Receipts bind checks to source commit and capture timestamps. The first run stopped on a test assumption that omitted slot_ids would be an array; the assertion now treats omission as an empty list. No download/import or media operation resulted.

## Backend-owned risk: stale manual choices

Source inspection of internal/source/choose.go identified an unverified runtime path that prevents an unconditional “cannot downgrade” claim. Stale MP3 release129 is still chosen with matched_slot_ids [1183,1184,1185] from before the FLAC replacement. When packSlots rejects every current track because none would gain, Choose can fall through to the first old matched slot. Its accepted stale state can bypass NeedsForceError; the in_library warning alone does not block the write. Explicit slot_id selection similarly needs current-quality review.

This was not exercised against the populated preview because it could initiate an unwanted replacement. It is a source finding, not a reproduced runtime downgrade. No force=true or explicit slot override was sent. Fresh-result protection and automatic-search protection are verified; stale-result protection remains open.

Backend integration requirement: revalidate current gain/protection inside the final choice transaction for album/track choices, handle zero eligible pack slots without falling back, and cover stale accepted MP3 plus explicit slot_id with force=false after FLAC import. Preserve intentional override semantics with explicit confirmation if supported. Recheck on the exact combined preview binary after the backend fix; these receipts cannot certify a future fix.
