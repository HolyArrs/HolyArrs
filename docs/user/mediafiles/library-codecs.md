# Video codecs

You want to find movies in an old or large codec and look for better copies. The **Video codecs** page in the Library does this. You need permission to manage the Library.

## Find copies by codec

1. Open the **Video codecs** page from the Library.
2. Type in **Search movies** to match the start of a title.
3. Pick a codec in **Codec**.
4. Pick an order in **Sort**: title, codec or recently added.

The list shows main movie files that are present, with the Version each one belongs to. Pages hold up to 100 rows. Your selection is kept while you move between pages.

The codec comes only from what HolyArrs read inside the file. A codec in the release name never counts. If the file was not read, could not be read, or has a codec HolyArrs does not know, it shows as **Unknown**. See [Media inspection](media-inspection.md).

## Search again for better copies

1. Tick the copies you want.
2. Select **Review**. HolyArrs checks each copy again and shows what it will search.
3. Confirm the search.

HolyArrs searches each chosen Version with its current Pick rules and saves the results for you to look at. It does not pick, grab, replace or delete anything, and it does not change your rules. A better release may not exist.

## For scripts

These routes need the manage Library permission:

- `GET /api/v1/library/copies?q=&codec=&sort=&limit=&offset=`
- `POST /api/v1/library/copies/search/preview`
- `POST /api/v1/library/copies/search`

Preview body:

```json
{"selections":[{"file_id":12,"version_id":7}]}
```

Send the same selections and the `review_token` from preview to the search route.

## Troubleshooting

**A copy cannot be ticked.** It is protected. See [Protected copies](protected-copies.md).

**Review refuses a copy.** The file was moved, is missing, is not a main movie file, no longer belongs to that Version, or is protected.

**The search says it is out of date.** Something changed after you reviewed, and the answer is `409 Conflict`. Select **Review** again.
