# Preview music approval to Library

This supplements the consolidated audit; it closes its fresh disposable music-lifecycle gap only. Product frontend digest remains `45144ef19d962c8cd785e124d352c7f00f42821ea359558b919415073e6f2928`. No frontend or backend product code changed.

## Actual saved scenario

Run `6d60338b-2a0f-4b57-859b-460851b88fb5` on preview 18746 / disposable backend 18747:

- New fake artist **Amber Satellites**, MBID `a4444444-0000-4000-8000-000000000004`.
- A new household member requested **Evening Signals** only. Request **34** was genuinely saved pending, and the member had no approval button.
- Owner approval genuinely saved and mapped to Library artist **166**. The request retained its selected album; Evening Signals is kept up to date and **Quiet Orbit** remains off.
- Library cards and lists at 360/834/1440 show **Approved**, **3 missing**, and the requester. Refresh preserves the request-to-title link.
- Four authorized browser writes: two disposable invitation acceptances, one request and one approval. No download search was started; search-on-approval was guarded off.

The initial attempt successfully saved and approved the request but failed a card-status assertion before background track counts became ready. Its receipt/screenshots are retained. A bounded readiness check now waits for real track counts. Continuation resumed only the same two run-owned members through fresh disposable sessions, without changing any password or recreating the artist/request. It passed the remaining lifecycle checks. Historical creation writes remain recorded; continuation did not repeat them.

Cleanup verifies both new members disabled and sessions revoked, one approved sample request retained, original rows/media preserved. Pre/post restart fingerprints also confirmed existing users, keys, secrets, settings, asks, requester links and titles unchanged. The same preview database/config and pinned backend binary were reused; only the owned preview launcher/backend/fakes restarted. Installed 18730, saved 18745 and their process identities were preserved.

## Evidence and reproducibility

- Genuine save receipt and earlier attempt: `.omx/artifacts/visual-ralph/intercom/real-save/6d60338b-2a0f-4b57-859b-460851b88fb5/receipt.json` and `attempt-1.json`.
- Fourteen lifecycle screenshots include retained diagnostic images; they are not all accepted final views.
- Final committed-checkout read-only verification: `music-lifecycle-verify.cjs` writes `final-checkout.json` alongside the receipt, binding six card/list checks and screenshots to its exact commit and frontend digest.
- Local gallery: `/preview-audit/music-lifecycle-20261017/`.
- Provider setup: `music-lifecycle-sample.mjs`, imported after the existing sample catalog by the ignored owned `resume-preview.mjs` launcher. It adds metadata only, never seeds stored titles or overwrites existing identities.
- Candidate profile: `HOLYARRS_REAL_SAVE_PROFILE=music20261017`. A new ordinary run deliberately refuses occupied candidates. Do not reset/reuse this artist or rerun a completed continuation.

All artist/album/track identities are synthetic disposable data. The artwork fallback is intentional; no external artwork identity is fabricated. This proves preview request/approval persistence and frontend presentation, not real provider lookup, download, filesystem import or music playback. Other consolidated-audit limitations remain open, including the Music settings permission contract and exact Intercom font/timing/native zoom.
