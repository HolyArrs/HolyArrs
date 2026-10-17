# TV and music request fixtures

`sample-show-music-requests.py` adds six requests only to the disposable Intercom preview database under `%TEMP%/holyarrs-intercom-preview-18747/config/holyarrs.db` and its backend on port 18747.

| Type | Pending | Approved | Turned down |
| --- | --- | --- | --- |
| TV | Breaking Bad | Game of Thrones | Rick and Morty |
| Music | Hozier / Unreal Unearth | Queen / The Platinum Collection | Fleetwood Mac / Greatest Hits |

Names and HTTPS artwork come from the existing real catalog. Provider identifiers remain disposable test identifiers. No real media is downloaded.

The fixture initializes pending rows directly because owner requests automatically approve in this preview. Approval and rejection then use the normal API, including its messages. Search on approval is temporarily disabled and the exact original settings restored. Existing requests and downloads are checked for changes. Re-running preserves the six existing fixtures.

In Wishlist, choose Shows or Music and Everyone’s to see all three states; Waiting shows pending requests only. Headless checks covered both kinds at 1440, 834 and 390 pixels, with loaded artwork, no horizontal overflow, no page errors and no automated WCAG A/AA violations. Evidence is in the ignored `.omx/artifacts/visual-ralph/intercom/request-media-verification/` folder. These checks cover the preview, not a combined release build.
