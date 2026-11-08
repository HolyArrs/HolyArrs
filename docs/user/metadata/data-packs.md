# Data packs

You want to copy your Pick rules, naming patterns or release-name checks from one HolyArrs install to another. A data pack does that. It is a JSON file with data only. It can't run code, fetch an address, add a title, change a user, touch your media files or carry secrets.

Only the owner can use data packs, and only through the API with an `admin` key. A `health_read`, `read` or `write` key gets refused.

## Move a pack to another install

1. Send the pack to `POST /api/v1/data-packs/validate`. This checks it without saving it.
2. Send the same JSON to `POST /api/v1/data-packs`. This saves it as inactive. Nothing changes yet. If a pack with that id already exists, you get a conflict and the old one stays.
3. List saved packs with `GET /api/v1/data-packs`. Export one with `GET /api/v1/data-packs/{id}`.
4. Preview what you want to apply with `GET /api/v1/data-packs/{id}/preview`. Add `pick_rule_id` once for each rule, and at most one `naming_id`.
5. Send the same choice and the `approval_hash` from the preview to `POST /api/v1/data-packs/{id}/apply`.

Preview example, against `http://your-server:6262`:

```text
GET /api/v1/data-packs/home-cinema/preview?pick_rule_id=balanced-web&naming_id=readable
```

Apply body:

```json
{
  "pick_rule_ids": ["balanced-web"],
  "naming_id": "readable",
  "approval_hash": "the hash returned by preview"
}
```

## What applying changes

- Each chosen Pick rule is created as a new rule. It isn't assigned to any title you already have.
- A chosen naming set replaces only these settings:
  - the movie, show, season, episode, anime and music naming patterns
  - colon and illegal-character formatting
  - multi-episode style, air-date style and absolute-number digits
- Existing files are not renamed. Rename on import, folder behavior, tag reading and scan limits are never part of a pack.

## Limits

| Limit | Value |
| --- | --- |
| `format` | `holyarrs-data-pack` |
| `version` | `1` |
| Size of a pack or an export | 256 KiB |
| Pick rules | 32 |
| Naming sets | 8 |
| Release-name checks | 256 |
| Ids | 1 to 64 characters: lowercase letters, digits, dots, dashes, underscores |

A pack is refused when it has duplicate or unknown fields, extra JSON after the end, unknown quality or trait names, bad naming words, a version other than 1, or values past these limits. Each release-name check runs through the HolyArrs parser and must match every fact it expects.

## Troubleshooting

**Apply says the preview is stale.** Something changed since you previewed: the pack, a Pick rule name you would collide with, or one of the naming settings it would replace. Nothing was applied. Preview again and send the new hash.

**Import says the id already exists.** Packs are never replaced. Give the new pack another id.

**You get a permission error.** Use an API key with the `admin` scope, created by the owner.
