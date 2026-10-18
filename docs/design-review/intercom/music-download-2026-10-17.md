# Disposable music approval through availability

Scope: preview 18746 only, branch `codex/mobbin-product-review`. No backend source changes.

The retained approved request 34 for Amber Satellites / Evening Signals (album 144, artist 166) was exercised through the real search, choose, download and import pipeline. Download 35 finished; import 14 placed three distinct files (7–9) into the disposable music folder. The album reports complete, 3/3 present; the request reports available / Ready to listen. Quiet Orbit was not requested or imported.

The files are locally synthesized MP3 tones with fixture metadata, not commercial music. Their imported hashes match the generated source files. The slots correctly retain Better copy sought because the configured preference wants a lossless upgrade; this does not mean the present files are unavailable. Artist cards and lists now say “3 tracks in library” when only part of an artist's catalog is present.

## Evidence and limits

Real-write source commits: `e81bf6d` (download creation), `946f919` (successful import continuation). Frontend availability correction: `192680b`. Final read-only receipts record the exact HEAD checked and capture timestamps under `.omx/artifacts/visual-ralph/intercom/music-download/final-*/receipt.json`.

The final verifier checks the completed download, album, Library posters, Library list and request at 360×800, 834×1112 and 1440×1000 CSS pixels, DPR 1, default zoom. It asserts status text and absence of page horizontal overflow and rejects browser writes. Screenshots include matching detail crops. This is targeted lifecycle verification, not certification of the entire Intercom redesign or physical devices.

Preservation checks compare original users/password hashes, keys, settings, sample records and file hashes against the pre-task snapshot. They compare original downloader configuration excluding volatile health timestamps, verify original fake torrents, and require all five temporary fixture accounts disabled with zero sessions. The temporary size-reporting proxy and downloader were removed; the original connection was restored. Installed app, credentials and real media were outside the exercised paths.

Earlier attempts are retained, not counted as passes: a tiny placeholder FLAC was correctly rejected by the importer; a later valid MP3 import succeeded but an overly strict test initially rejected its legitimate upgrade-wanted state. The fake downloader required a disposable loopback proxy to report actual generated file sizes. This proves local import and UI state propagation, not external torrent transfer or playback quality.

Verification: music helper regression suite 6 passed; Svelte check 0 errors and 0 warnings. Final responsive receipt is the authority for browser results.
