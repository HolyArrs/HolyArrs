# TV and music request fixtures

`sample-show-music-requests.py` adds six requests only to the disposable Intercom preview database under `%TEMP%/holyarrs-intercom-preview-18747/config/holyarrs.db` and its backend on port 18747.

| Type | Pending | Approved | Turned down |
| --- | --- | --- | --- |
| TV | Breaking Bad | Game of Thrones | Rick and Morty |
| Music | Hozier / Unreal Unearth | Queen / The Platinum Collection | Fleetwood Mac / Greatest Hits |

Names and HTTPS artwork come from the existing real catalog. Provider identifiers remain disposable test identifiers. No real media is downloaded.

The fixture initializes pending rows directly because owner requests automatically approve in this preview. Approval and rejection then use the normal API, including its messages. Search on approval is temporarily disabled and the exact original settings restored. Existing requests and downloads are checked for changes. Re-running preserves the six existing fixtures.

In Wishlist, choose Shows or Music and Everyone’s to see all three states; Waiting shows pending requests only. Headless checks covered both kinds at 1440, 834 and 390 pixels, with loaded artwork, no horizontal overflow, no page errors and no automated WCAG A/AA violations. Evidence is in the ignored `.omx/artifacts/visual-ralph/intercom/request-media-verification/` folder. These checks cover the preview, not a combined release build.

Run sample-approved-library.py to verify the approved request-to-title links and persist their request artwork in the native title artwork field. It is guarded to the exact disposable preview database and fake source, and preserves requests, downloads, slots and imports. Game of Thrones remains 8 missing episodes; Queen remains 40 missing tracks in the existing partial test catalog. Approval is visible in each title’s Wishlist section; neither is marked downloaded. Grid and detail checks passed at 1440/834/390 with artwork, missing status, approval details, no overflow or page errors. Evidence: .omx/artifacts/visual-ralph/intercom/approved-library-check/.
