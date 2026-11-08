---
title: Protected copies
description: Stop upgrades from replacing a movie or show file you want to keep, and add a second Version beside it.
level: Setup
---

Keep a file you like. A protected copy is never replaced by an upgrade.

## Protect a copy

1. Open the title page.
2. Go to **Copies**.
3. In the Version's group, select **Protect this copy**. You should see a **Protected** badge.

Protection survives restarts and rescans, even while the file is missing for a while. HolyArrs checks it again before using a search result and before an import places a file, so a search or download already queued cannot replace it.

## Keep a copy and add another Version

Use this for a second copy beside this one, such as 4K next to HD.

1. Select **Keep this copy and add another Version**.
2. Choose a label, a Library folder and Pick rules. Add an edition and audio languages if you want.
3. Select **Protect and add Version**.

Both happen in one step. If it fails, nothing changes. It does not start a download or copy the file into the new Version.

## Remove protection

1. In the copy's group, tick the confirm box.
2. Select **Remove protection**.

## What happens on delete

- Removing a title or a movie series skips protected titles and keeps them.
- Removing files by hand can include protected copies if you confirm that separately. Only files HolyArrs imported or took over go to the holding folder; others stay put.
- When that move works, protection is cleared and holding cleanup can remove the file later. When it fails, the copy stays protected.
- Holding cleanup skips protected files.

## If it does not work

**Symptom:** **Remove protection** is greyed out.
**Fix:** Tick the confirm box first.

**Symptom:** A protected file changed anyway.
**Cause:** Protection covers what HolyArrs does. Another program or a person on the disk can still change it.

**Symptom:** The **Video codecs** page will not let you tick a copy.
**Cause:** Protected copies cannot be searched again. See [Codecs and media inspection](codecs-inspection.md).

## For experts

<details class="ha-more"><summary>Edge cases</summary>

- A show file that covers several episodes cannot be partly replaced by a season pack while protected.
- Protection does not change other titles, Versions or Pick rules.

</details>

## Related

- [Codecs and media inspection](codecs-inspection.md)
- [Library folders](folders.md)
