# Library codec view

The codec view lists present main movie files with the exact Version each file belongs to. Its codec filter reads only `media_info.video_codec` when `media_info.inspection_status` is `complete` or `partial`. Missing data, failed or unsupported inspections, and unrecognized codec values appear as **Unknown**. The parsed release-name value in `quality.codec` never counts as inspected media.

The list supports title prefix search, codec filtering, and title, codec, or recently-added sorting. Pages contain at most 100 rows and use title, Title ID, Version ID, and File ID as deterministic tie breakers. A selection contains both the File ID and Version ID, so it remains exact while a person moves between pages.

## Reviewed searches

Searching is a two-step action:

1. `POST /api/v1/library/copies/search/preview` re-reads every selected File and Version. It rejects a missing, moved, non-movie, non-main, non-present, mismatched, or protected copy and returns a review token.
2. `POST /api/v1/library/copies/search` revalidates the same records and token, then queues the existing search for each exact movie Slot in the selected Versions. The queued search has `by_hand: true` and `choose: false`.

The search applies each Version's current Pick rules and stores candidates for a person to review. It does not choose, grab, replace, delete, or edit rules, and it does not guarantee that a replacement exists.

All three routes require `manage_library`:

- `GET /api/v1/library/copies?q=&codec=&sort=&limit=&offset=`
- `POST /api/v1/library/copies/search/preview`
- `POST /api/v1/library/copies/search`

The preview body is:

```json
{"selections":[{"file_id":12,"version_id":7}]}
```

The final body includes the unchanged selections and the `review_token` returned by preview. A stale token returns `409 Conflict` and requires another review.
