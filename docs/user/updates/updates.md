# Updates

You want to know when a new HolyArrs is out, update safely, and go back if something goes wrong. HolyArrs checks for new versions. It never installs one. You pull the new image when it suits you.

## See if a new version is out

1. Open the **Status** page (`http://your-server:6262/status`).
2. Look at **Updates**. It says one of these:
   - "HolyArrs 1.2.0 is out, newer than this one."
   - "This is the newest version."
   - "No HolyArrs release is published yet."
   - "The last check did not work: ..." (for example, no internet).
   - "The update check is off."
3. To ask right now, click **Check now** (or **Check again**). You need permission to change settings.

A newer version also shows as a warning in the health checks. A failed check does not. It only shows under Updates.

When there is a link to the release notes, it points to the HolyArrs release pages on GitHub. HolyArrs drops any other link and shows the version as plain text.

## Update

1. Pull the new image and restart the container, for example `docker compose pull` then `docker compose up -d`.
2. HolyArrs starts, makes a backup if the new version changes how the data is stored, and then updates the data. See [The backup before an update](#the-backup-before-an-update).

## What the check sends

On each check HolyArrs makes one request to `api.github.com` for the newest release. It sends only the header `User-Agent: HolyArrs/<version>`. No account, no key, no list of your media. When the check is off, HolyArrs makes no request at all.

## Settings

Go to **Settings > Health checks** (`/settings/health`). The last three settings are under the advanced part of the card.

| Setting | Default | What it does |
|---|---|---|
| Check for a newer HolyArrs | On | Off means no request at all. |
| Check for updates every | Day | How often to check: every day, 3, 7, 14 or 30 days. |
| Release feed | Empty (HolyArrs releases) | Another place to check. It can be a GitHub-style release answer, a list of releases, any JSON with `version`, or plain text holding the version. The address may not hold a name, password, query or fragment. |
| Feed may be on the local network | Off | Turn on if your feed is on your own network. Without it, only public addresses are allowed. Cloud metadata addresses are always refused. |
| Include test versions (like 1.2.0-rc1) | Off | A test version counts as older than the plain version and gives no notice unless this is on. |

The last answer is saved, so a restart does not check again before it is due. One check waits at most 15 seconds, reads at most 512 KB and follows at most 3 redirects. A redirect may not switch between http and https.

## The backup before an update

A new version may need to change how the data is stored. Before it changes anything, HolyArrs:

1. Checks there is room for a copy and for the update. If there is not, nothing changes and the page tells you. You can skip this check with `HOLYARRS_SKIP_SPACE_CHECK=yes`.
2. Copies each database to the `backups` folder, checks the copy and writes a sha256 file next to it. If the copy fails 3 times, nothing changes and the page tells you.
3. Runs each update step on its own and checks the result. If a step fails, that step is undone and HolyArrs stops on a page that says what happened. Steps that already finished stay. Start the same version again and it carries on from the failed step, with the same copy.

After the first good start, the copy becomes a pre-update backup, named like `holyarrs-backup-pre-update-<time>.zip` and described as "before updating from 1.1.0 to 1.2.0". You see it in **Settings > Backups** and with `holyarrs restore --list`. HolyArrs keeps the newest 3. The backup schedule never removes them.

A fresh install has nothing to copy, so there is no pre-update backup. An update that does not change the data makes no pre-update backup either.

## Go back to the older version

The **Go back** panel on the Status page is held back in this release. Going back still works with the steps below.

HolyArrs cannot change its own image, so going back takes two parts. You run the older image. HolyArrs puts the older data back.

1. Change the image in your compose file (or `docker run`) to the older tag, for example `ghcr.io/coderluii/holyarrs:<older version>`. If you pull from another registry, change only the tag.
2. Start the container with `docker compose up -d`.
3. If the update did not change the data, the older version just starts. You are done.
4. If it did, the older version shows a page instead of the app. Click **Put back the data from before the update**.
5. Type the one-time code. Find it with `docker logs <container>` and look for "one-time code".
6. HolyArrs checks the backup, puts it in place and stops. Docker's restart policy starts it again on that data. Without a restart policy, start the container yourself.

Without the page, run `docker exec <container> /holyarrs restore --pre-update`, then `docker restart <container>`. Without Docker, stop HolyArrs, run `holyarrs restore --pre-update`, then start it again.

Good to know:

- Anything you changed after the update (new titles, settings, history) is not in that backup.
- Nothing is deleted. The newer data moves to `backups/before-restore-<time>/`, so you can undo going back.
- If you go back more than one version, HolyArrs picks the newest pre-update backup that version can read.

## Troubleshooting

**"No HolyArrs release is published yet."** The release feed has nothing yet. Nothing is wrong.

**"The last check did not work."** HolyArrs could not reach the feed. Check that the server has internet. If you use your own feed on your network, turn on **Feed may be on the local network**.

**The release feed will not save.** Remove any name, password, `?query` or `#fragment` from the address. It must start with `http://` or `https://`.

**The update stopped because there is too little disk space.** Free up disk space where your config folder lives, then start again.

**The go-back page refuses my code.** Copy the code from the newest log line. After 10 wrong codes the page refuses until the next start. Restart the container and use the new code it writes to the log.
