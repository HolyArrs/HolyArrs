# Disposable show and music activity

`sample-show-music-activity.mjs` adds real TV episode and album download lifecycles to the disposable Intercom preview. It uses the HolyArrs API, the preview fake Source, and the preview fake Downloader. It does not write the database directly.

The script creates, for both shows and artists:

- at least one active download;
- one paused download;
- one completed download backed by a genuine `placed` import and real tiny fixture files;
- one finished download whose missing fixture files produce real **Needs a decision** records.

The fixture files stay inside these disposable temp roots:

- downloads: `%TEMP%\holyarrs-intercom-preview-18747\downloads`;
- show library: `%TEMP%\holyarrs-intercom-preview-media-18747\shows`;
- music library: `%TEMP%\holyarrs-intercom-preview-media-18747\music`.

The script refuses any other paths, validates every generated basename, creates files without overwriting, and reuses an existing file only when its content is the exact expected fixture. Search settings are copied before the run, temporarily changed to accept tiny fixtures, restored in `finally`, and compared byte-for-byte as JSON after restoration.

The disposable Source must already advertise `tv` and `music` in addition to `movie`, with Torznab categories `5000` for TV and `3000` for music. The script checks this before any mutation and stops with a clear error if the preview launcher has not prepared those capabilities. It never edits Source settings because that endpoint requires password-strength authentication.

For this exact disposable preview, `preview-source-kinds.py` reproducibly prepares that Source without changing authentication or any other setting. It refuses every database except `%TEMP%\holyarrs-intercom-preview-18747\config\holyarrs.db`, verifies that port `28767` is the expected fake Source, requires Source id `1` at the exact loopback URL, and accepts only the original movie-only state or the exact all-kinds preview state. Its one guarded transaction updates only `kinds` and `categories`.

```powershell
python docs/design-review/intercom/preview-source-kinds.py --check
python docs/design-review/intercom/preview-source-kinds.py
```

Run it only while the isolated preview backend and fake providers are running:

```powershell
node docs/design-review/intercom/sample-show-music-activity.mjs
```

Success output lists every seeded record, the real import IDs and placed paths for completed examples, decision counts for blocked imports, the final phase counts by media kind, and `settings_restored: true`.

The larger ten-track sample album deliberately exercises normal Pick rules. Its tiny fake FLAC release falls below the album size floor even though import fixture checks are relaxed. Prepare that one active example through HolyArrs' normal **Choose anyway** flow for the matching Clancy FLAC release before rerunning the seeder. The script accepts a representative active track from that release, while completed and decision examples still require every target slot. The stored Pick rules stay unchanged.
