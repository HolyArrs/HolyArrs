---
title: Folders
description: Lay out one shared data folder so HolyArrs moves finished downloads into your Library instantly with hardlinks.
level: Start
sidebar: { order: 2, label: "Step 2 of 5: Folders" }
---

Set up your folders once so finished downloads land in your Library instantly, without a second copy.

The [Install](install.mdx) example mounts `/movies`, `/tv`, `/music` and `/downloads` one by one. That works, but every import is a full copy. This page swaps those four mounts for one shared parent folder. It is optional.

## The rule

Put downloads and media under **one parent folder, on one disk**, and mount that parent into HolyArrs and your downloader **at the same path**.

A hardlink is a second name for the same file. It takes no extra space and is made instantly. It only works inside one filesystem, and Docker treats every mount as its own, so separate mounts force a full copy.

```
/srv/data                  mounted as /data in HolyArrs and the downloader
├── downloads
│   ├── torrents
│   └── usenet
└── media
    ├── movies
    ├── tv
    └── music
```

## Set it up

1. Make the folders on the host:

   ```sh
   mkdir -p /srv/data/downloads/{torrents,usenet} /srv/data/media/{movies,tv,music}
   ```

2. Give your `PUID` user write access:

   ```sh
   chown -R 1000:1000 /srv/data
   ```

3. Mount the parent in HolyArrs: `- /srv/data:/data`.
4. Mount it the same way in your downloader: `- /srv/data:/data`, and set its save folder to `/data/downloads/torrents` or `/data/downloads/usenet`.
5. In HolyArrs, use `/data/media/movies`, `/data/media/tv` and `/data/media/music` as Library folders during [First run](../first-run/).

   You should see no **No hardlink** warning in **Settings > Library folders**.

:::caution
Separate NAS shares or datasets under `/srv/data` are separate filesystems. Files still copy between them. Use folders inside one share.
:::

## Choices

### Separate mounts

`/movies`, `/tv`, `/music` and `/downloads` as four mounts still work. Every import then copies the file, which takes longer and uses double the space until the downloader deletes its copy. That holds for Usenet downloads too, even on one disk: a file can only move instantly within one mount, so use the shared parent folder above.

### Downloader in another container with other paths

If the downloader sees the folder under another name, fill in **Downloader's path** and **HolyArrs' path** on the downloader in **Settings > Downloaders**, such as `/data/torrents` and `/downloads`. Matching paths need no mapping.

## If it does not work

| You see | Cause | Fix |
|---|---|---|
| **No hardlink** on a Library folder | Downloads and media are on different mounts or filesystems | Mount one parent folder, as above |
| **Read only** or **Missing** | The path is wrong, or `PUID` cannot write | Use the path inside the container; fix ownership |
| A folder is not found | You typed the host path | Type the container path, such as `/data/media/movies` |
| Import says the path is not allowed | The download folder is not a known place | Set `HOLYARRS_SAVE_PATH`, or add it under **Settings > Library folders > Extra import folders** |

## For experts

<details class="ha-more"><summary>What the folder check tests</summary>

| Check | Fails as |
|---|---|
| Folder exists | **Missing** (folder is not used) |
| HolyArrs can write a test file | **Read only** (folder is not used) |
| Hardlink from each active downloader folder | **No hardlink** (warning, imports copy) |
| At least 100 MB free | Low space note |

Test files are removed after the check. **Check again** reruns it.

</details>

<details class="ha-more"><summary>Library folder rules</summary>

- Must exist, be a folder and be a full path.
- Cannot sit inside or around another Library folder. Each path is used once.
- **Rename** changes only the shown name. **Remove** leaves every file and is refused while titles still use the folder.
- **Extra import folders**: one full path per line, up to 32. A whole disk such as `/` or `C:\` is refused. Only the owner, or an admin signed in with a password, can change the list.
- Manual import also allows Library folders, `HOLYARRS_SAVE_PATH` and the HolyArrs side of each downloader path map. Paths with `..`, network share paths and links that lead outside are refused.

</details>

## Related

- [Install](../install/)
- [Library folders](../../library/folders/)
- [Platforms](../../run/platforms/)
