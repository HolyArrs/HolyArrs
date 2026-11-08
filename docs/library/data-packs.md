---
title: Data packs
description: Copy Pick rules, naming patterns and release-name checks between installs with a JSON pack through the API.
level: Expert
---

Copy your Pick rules, naming patterns or release-name checks from one install to another.

A data pack is a JSON file with data only. It cannot run code, fetch an address, add a title, change a user, touch media files or carry secrets.

## Before you start

- You are the owner.
- You have an API key with the `admin` scope. Keys with `health_read`, `read` or `write` are refused.
- You have the pack JSON. Use the pack you were given, or write one from the shape under **For experts**.

## Move a pack

All routes are on `http://your-server:6262`.

1. Send the pack to `POST /api/v1/data-packs/validate`. It checks without saving. You should see `"valid": true`.
2. Send the same JSON to `POST /api/v1/data-packs`. It saves the pack as inactive. Nothing changes yet.
3. Open `GET /api/v1/data-packs` to list saved packs. `GET /api/v1/data-packs/{id}` exports one.
4. Open `GET /api/v1/data-packs/{id}/preview` with one `pick_rule_id` per rule and at most one `naming_id`. You get an `approval_hash`.
5. Send the same choice and the hash to `POST /api/v1/data-packs/{id}/apply`. You should see the pack id and the rule ids you chose.

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

:::caution
A naming set replaces your current naming patterns. Existing files are not renamed.
:::

## What applying changes

- Each chosen Pick rule is created new. It is not assigned to any title.
- A chosen naming set replaces only these settings:
  - Movie file and folder patterns.
  - Show folder, season folder, specials folder, episode, daily and anime patterns.
  - Artist folder, album folder, disc folder, track and multi-disc track patterns.
  - Colon and illegal-character formatting.
  - Multi-episode style, air-date style and absolute-number digits.
- Rename on import, folder behavior, tag reading and scan limits are never in a pack.

## If it does not work

**Symptom:** Apply says the preview is stale.
**Cause:** The pack, a Pick rule name that would collide, or a naming setting it would replace changed after the preview. Nothing was applied.
**Fix:** Preview again and send the new hash.

**Symptom:** Import says the pack already exists.
**Cause:** Packs are never replaced.
**Fix:** Give the new pack another id.

**Symptom:** Permission error.
**Cause:** The key is not an owner-made `admin` key.
**Fix:** Create an `admin` key as the owner and use it.

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
| Ids | 1 to 64 characters: lowercase letters, digits, dots, dashes, underscores. Start and end with a letter or digit |
| Apply request body | 16 KiB |

A pack is refused for duplicate or unknown fields, extra JSON after the end, unknown quality or trait names, bad naming words, a version other than 1, or values past these limits. Each release-name check runs through the HolyArrs parser and must match every fact it expects.

</details>

<details class="ha-more"><summary>Pack shape</summary>

| Field | Holds |
|---|---|
| `format`, `version`, `id`, `name` | Required header |
| `pick_rules` | Each has `id`, `name`, `kinds` and `rules` (steps, traits, size limits, protocol, language, order) |
| `naming` | Each has `id`, `name` and `patterns` (the naming settings listed above) |
| `release_tests` | Each has `id`, `name`, `release_name`, `hints` and `expected` (kind, quality, resolution, source, codec, flags) |

</details>

<details class="ha-more"><summary>Routes and responses</summary>

| Route | Success | Refusal |
|---|---|---|
| `POST /api/v1/data-packs/validate` | 200 with `valid` and a summary | 400 invalid, 413 too large |
| `POST /api/v1/data-packs` | 201 with a summary, status `inactive` | 409 id exists, 400, 413 |
| `GET /api/v1/data-packs` | 200 with `data_packs` | |
| `GET /api/v1/data-packs/{id}` | 200 with the pack JSON | 404 |
| `GET /api/v1/data-packs/{id}/preview` | 200 with `approval_hash` | 400 for any query other than `pick_rule_id` and `naming_id`, 404 |
| `POST /api/v1/data-packs/{id}/apply` | 200 with `pack_id`, `pick_rule_ids`, `naming_id` | 409 stale, 400, 404 |

</details>

## Related

- [Metadata](metadata.md)
- [Library](index.md)
