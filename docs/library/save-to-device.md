---
title: Save to device
description: Save a movie or episode file from the Library to the phone or computer you are using, and choose who may do it.
level: Setup
---

Take a copy of a Library file with you. The browser saves the file as it is, with no conversion.

## Before you start

- The owner can always save files. Anyone else needs the **Save files to their device** permission. Nobody has it until the owner or an admin gives it. See [Users and roles](../users/roles.md).
- A Kid account can never save files, whatever is ticked.

## Save a movie

1. Open the movie and go to its **Library** tab.
2. Under **File** or **Versions**, select **Save to device** on the copy you want.

You should see your browser start the save, named after the file in the Library.

## Save an episode

1. Open the show and go to its **Library** tab.
2. Select the episode. Its sheet opens.
3. Under **File**, select **Save to device**.

## Choices

### Who may save files

Tick or clear **Save files to their device** for a person in **Settings > Users and roles**. It counts from their next request. An admin can give it only if the admin has it.

### A save that stops

The save can carry on where it stopped, if your browser offers to resume it. If the file changed in the Library in between, the save starts again from the start.

## If it does not work

**Symptom:** there is no **Save to device** button.
**Cause:** you do not have the permission, your account has an age limit, or the copy has no file yet.
**Fix:** ask the owner for the permission, or wait until the file is in the Library.

**Symptom:** the save fails with "not found".
**Cause:** the file is missing on disk, sits outside its Library folder, or is not the main video file.
**Fix:** run a rescan of the title, then try again.

## For experts

<details class="ha-more"><summary>The route and what it checks</summary>

`GET /api/v1/titles/{id}/files/{fid}/save` sends the file as an attachment. HEAD works, and `Range` and `If-Range` let a save resume. The file is found from the Library's own records only, opened inside its Library folder, and must be a regular file; a link out of the folder answers 404.

- It needs `save_files`. An API key needs the `read` scope and acts within its person's permissions. Plugins can't use it.
- A title above a Kid's limit, a title whose part is off, a removed title or a file of another title answers 404.
- Each save that starts from the first byte writes a `file_saved` event on the audit trail, with the title and file ids only.

</details>

## Related

- [Users and roles](../users/roles.md)
- [Kid accounts](../users/kids.md)
- [Protected copies](protected-copies.md)
