# Data packs

Data packs move selected Pick rules, naming patterns and release-name checks between HolyArrs installs. A pack is JSON data only. It cannot run code, fetch a URL, add a title, change a user, touch media files or carry secrets.

Importing a pack stores it as inactive. Nothing changes until the owner previews and explicitly applies a selection. Applying can create selected Pick rule definitions and replace the selected naming pattern keys. It does not assign the new rules to existing titles or rename existing files.

## Limits

Version 1 uses `format: "holyarrs-data-pack"` and `version: 1`. The pack and every export are limited to 256 KiB. A pack may hold at most 32 Pick rules, 8 naming sets and 256 release-name checks. IDs use 1 to 64 lowercase ASCII letters, digits, dots, dashes or underscores.

The decoder rejects duplicate or unknown JSON fields, trailing JSON values, unknown quality and trait names, invalid naming words, unsupported versions and values outside the documented limits. Release-name checks run the HolyArrs parser during validation and must match every expected fact.

Naming sets contain patterns and formatting choices only. They exclude rename-on-import, folder behavior, tag reading and scan limits. Applying a naming set updates only these keys:

- movie, show, season, episode, anime and music naming patterns
- colon and illegal-character formatting
- multi-episode style, air-date style and absolute-number digits

## API flow

All data-pack routes are owner-only and require the `admin` API-key scope. A `health_read`, `read` or `write` key cannot use them.

1. Send a pack to `POST /api/v1/data-packs/validate` to validate without storing it.
2. Send the same JSON to `POST /api/v1/data-packs` to store it as inactive. An existing pack ID returns a conflict and is never replaced.
3. List stored packs with `GET /api/v1/data-packs`, or export one with `GET /api/v1/data-packs/{id}`.
4. Preview a selection with `GET /api/v1/data-packs/{id}/preview`. Repeat `pick_rule_id` for each rule ID and optionally add one `naming_id`.
5. Send the same canonical selection and the returned `approval_hash` to `POST /api/v1/data-packs/{id}/apply`.

Example preview:

```text
GET /api/v1/data-packs/home-cinema/preview?pick_rule_id=balanced-web&naming_id=readable
```

Example apply body:

```json
{
  "pick_rule_ids": ["balanced-web"],
  "naming_id": "readable",
  "approval_hash": "the hash returned by preview"
}
```

The approval hash binds the stored pack, sorted selected IDs, target Pick-rule name occupancy and the exact current values of the naming keys that would change. Apply rereads all of them inside its single database transaction. A changed pack, conflicting rule name or changed selected naming key rejects the whole operation. Preview again after a stale result.

The original synthetic fixture used by the validator tests is in `internal/datapack/testdata/synthetic-pack.json`. It contains no community dataset, personal settings or library inventory.
