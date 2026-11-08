# Health checks

You want to know if something is wrong before it costs you a download. HolyArrs checks itself on a timer and tells you.

## See the checks

1. Open **Home**. Anything that needs a look shows there.
2. For the full list, open **Settings > This app**. Each check shows its state and when it last changed.
3. Click **Check again** to run all checks now.

Checks run every 5 minutes, when HolyArrs starts, and right after an import is refused for lack of space. The **Status** page at `/status` shows free space for each Library folder.

## What each check means

| Check | Fine | Warning | Broken |
|---|---|---|---|
| Config folder | `/config` is mapped to a host folder or named volume | | Not mapped. Everything is lost when the container is removed. See [Forgot to map /config?](../getting-started/install.md#forgot-to-map-config) |
| Library folder | There and writable | On another disk than the downloads, so imports are copied | Gone (disk or share not mounted) or not writable |
| Free space | Room to spare | Under the warning size (1 GB by default) | An import was just refused for space |
| Source | Working, or paused by you | Cannot be reached, or refused its key | |
| Downloader | Working, or paused by you | Does not answer, or refused the sign-in | |
| Waiting downloads | No download waits for its files | A download waits because the Downloader cannot use its files, often a share that mounted late | One has waited about 3 days |
| Backups | A recent backup exists | No backup yet, or the newest is old | The last scheduled backup failed |
| Key kit | A key kit was saved for the current master key | No key kit saved yet | |
| Update | You run the newest version | A newer HolyArrs is out | |

## Settings

These live on **Settings > This app**.

- **Check every**: 1 to 1440 minutes. Default 5.
- **Alert after this many runs in a row**: 1 to 20. Default 2. A new result must hold this many runs before an Alert goes out, so a Downloader that blips once stays quiet.
- **Warn when free space is under**: in MB. Default 1024.
- **Update check**: on or off. When off, HolyArrs sends no request at all.
- **Update check link**: the release feed to ask. Leave it empty to use the default HolyArrs.com feed.
- **Check for updates every**: 1, 3, 7, 14 or 30 days. Default 1.
- **Pre-releases**: off by default. Turn it on to hear about test versions too.

## Alerts for health

Create an Alert with the **Health** moment. You get one message when a check breaks and one when it is fixed. A Downloader that stays down sends one message, not one per run.

## Troubleshooting

**A download waits for its files after a share came back.** qBittorrent, Deluge and Transmission keep the error until the files are checked again. Open the Downloads page and click **Check again** on that row, or recheck it in the Downloader.

**The key kit warning stays.** Backups never hold the master key that opens saved passwords and keys. The key kit is that key as a file. Only the owner can download it, from **Settings > Backups > Download key kit**. Others see a note to ask the owner. Keep the kit apart from your backups. To restore on a new machine, see [Backups](../backups/backups.md).

**A Library folder shows broken after a reboot.** The disk or share was not mounted when HolyArrs started. Mount it, then click **Check again**.
