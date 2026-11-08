---
title: Data packs
description: Copy Pick rules, naming patterns and release-name checks between installs with a JSON pack through the API.
level: Expert
---

Copy your Pick rules, naming patterns or release-name checks from one install to another.

A data pack is a JSON file with data only. It cannot run code, fetch an address, add a title, change a user, touch media files or carry secrets.

## Before you start

- You are the owner.
- You have an API key with the `admin` scope. `health_read`, `read` and `write` keys are refused.

## Move a pack

All routes are on `http://your-server:6262`.

1. `POST /api/v1/data-packs/validate` with the pack. It checks without saving.
2. `POST /api/v1/data-packs` with the same JSON. It saves the pack as inactive. Nothing changes yet.
3. `GET /api/v1/data-packs` lists saved packs. `GET /api/v1/data-packs/{id}` exports one.
4. `GET /api/v1/data-packs/{id}/preview` with one `pick_rule_id` per rule and at most one `naming_id`. You get an `approval_hash`.
5. `POST /api/v1/data-packs/{id}/apply` with the same choice and the hash. You should see the new rules in Settings.

Preview:

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

- Each chosen Pick rule is created new. It is not assigned to any title.
- A chosen naming set replaces only: the movie, show, season, episode, anime and music naming patterns; colon and illegal-character formatting; multi-episode style, air-date style and absolute-number digits.
- Existing files are not renamed. Rename on import, folder behaviour, tag reading and scan limits are never in a pack.

## If it does not work

**Symptom:** Apply says the preview is stale.
**Cause:** The pack, a colliding Pick rule name, or a naming setting it would replace changed. Nothing was applied.
**Fix:** Preview again and send the new hash.

**Symptom:** Import says the id already exists.
**Cause:** Packs are never replaced.
**Fix:** Give the new pack another id.

**Symptom:** Permission error.
**Fix:** Use an owner-made key with the `admin` scope.

## For experts

<details class="ha-more"><summary>Limits and refusals</summary>

| Limit | Value |
|---|---|
| `format` | `holyarrs-data-pack` |
| `version` | `1` |
| Size of a pack or an export | 256 KiB |
| Pick rules | 32 |
| Naming sets | 8 |
| Release-name checks | 256 |
| Ids | 1 to 64 characters: lowercase letters, digits, dots, dashes, underscores |

A pack is refused for duplicate or unknown fields, extra JSON after the end, unknown quality or trait names, bad naming words, a version other than 1, or values past these limits. Each release-name check runs through the HolyArrs parser and must match every fact it expects.

</details>

## Related

- [Metadata](metadata.md)
