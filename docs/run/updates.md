---
title: Updates
description: See when a new HolyArrs is out, update by pulling the image, and go back to the older version.
level: Start
sidebar: { order: 1 }
---

HolyArrs tells you when a new version is out and never installs it. You pull the new image when it suits you.

## See if a new version is out

1. Open the **Status** page (`http://your-server:6262/health`).
2. Read **Updates**. You should see one of these:

| Message | Meaning |
| --- | --- |
| "HolyArrs 1.2.0 is out, newer than this one." | Update when ready |
| "This is the newest version." | Nothing to do |
| "No HolyArrs release is published yet." | The feed is empty. Nothing is wrong |
| "The last check did not work: ..." | Feed not reached, often no internet |
| "The update check is off: ..." | Turned off in Settings |

3. To ask now, select **Check now** (or **Check again**). It needs the **Change settings** permission.

A newer version also shows as a health warning. A failed check shows only under **Updates**.

## Update

1. Pull the new image and restart: `docker compose pull`, then `docker compose up -d`.
2. HolyArrs starts. If the new version changes how data is stored, it makes a backup first, then updates the data. You should see the app open on the new version.

:::tip
A failed update step is undone and HolyArrs stops on a page that says what happened. Your data from before the update stays as a backup.
:::

## Go back to the older version

HolyArrs cannot change its own image, so you run the older image and HolyArrs puts the older data back.

:::caution
Anything changed after the update, such as new titles, settings and history, is not in the pre-update backup.
:::

1. Change the image tag in your compose file (or `docker run`) to the older one, for example `holyarrs/holyarrs:<older version>`.
2. Run `docker compose up -d`.
3. If the update did not change the data, the older version starts. You are done.
4. Otherwise the older version shows a page instead of the app. Select **Put back the data from before the update**.
5. Type the one-time code. Find it with `docker logs <container>`; look for "one-time code".
6. HolyArrs checks the backup, puts it in place and stops. You should see the container start again on that data through its restart policy. Without one, start it yourself.

Without the page: `docker exec <container> /holyarrs restore --pre-update`, then `docker restart <container>`. Without Docker: stop HolyArrs, run `holyarrs restore --pre-update`, start it.

Nothing is deleted. The newer data moves to `backups/before-restore-<time>/`, so you can undo. Going back more than one version picks the newest pre-update backup that version can read.

## Choices

All in **Settings > Health checks**. The last three sit under the card's advanced part.

| Setting | Default | Change it when |
|---|---|---|
| **Check for a newer HolyArrs** | On | You want no outside request at all |
| **Check for updates every** | day | You want fewer checks: 3, 7, 14 or 30 days |
| **Release feed** | Empty (HolyArrs releases) | You host your own feed |
| **Feed may be on the local network** | Off | Your feed is on your own network |
| **Include test versions (like 1.2.0-rc1)** | Off | You want notices for test versions |

## If it does not work

**Symptom:** "The last check did not work."
**Cause:** the server has no internet, or your own feed is on the local network.
**Fix:** check internet access, or turn on **Feed may be on the local network**.

**Symptom:** the release feed will not save.
**Cause:** the address holds a name, password, `?query` or `#fragment`, or does not start with `http://` or `https://`.
**Fix:** remove those parts.

**Symptom:** the update stopped for lack of disk space.
**Cause:** no room for the copy and the update.
**Fix:** free space where the config folder lives, then start again.

**Symptom:** the go-back page refuses your code.
**Cause:** an old code, or 10 wrong codes since the last start.
**Fix:** copy the code from the newest log line. After 10 wrong tries, restart the container and use the new code.

## Held-back features

Some features are off by default in this release. Turn them on at your own risk. While held back, their pages and menu items are hidden, their API routes answer "not found" and their jobs do not run. Data you saved for them stays.

To turn one on, add its switch to `HOLYARRS_FLAGS`, comma-separated, and restart the container:

```yaml
environment:
  HOLYARRS_FLAGS: "subtitle_sites,trakt_lists"
```

Remove the name and restart to turn it off. Hidden settings come back when you turn it on again.

| Switch | Feature |
|---|---|
| `ai_apps` | Connect AI apps: **Settings > AI apps**, the `/ai` page and its routes |
| `row_title_stats` | Explore row title statistics in Appearance |
| `trakt_lists` | Trakt key, Trakt lists and Trakt reviews. TMDB lists stay on |
| `subtitle_sites` | Subtitle sites and subtitle search. Your subtitle language stays on |
| `emby_signin` | Sign in with Emby. Emby library refresh still works |
| `go_back` | The **Go back** panel on the Status page. The steps in [Go back](#go-back-to-the-older-version) work without it |

Nothing changed after adding a switch? Check the spelling (lowercase, underscores) and restart the container, not only the page.

## For experts

<details class="ha-more"><summary>What the check sends</summary>

One request to `api.github.com` for the newest release, with only `User-Agent: HolyArrs/<version>`. No account, key or media list. With the check off, no request at all.

The last answer is saved, so a restart does not check before it is due. One check waits at most 15 s, reads at most 512 KB and follows at most 3 redirects, never switching between http and https. A release-notes link shows only when it points to the HolyArrs release pages on GitHub; otherwise the version shows as plain text.

**Release feed** accepts a GitHub-style release answer, a list of releases, any JSON with `version`, or plain text holding the version. Cloud metadata addresses are always refused. A test version counts as older than the plain version.

</details>

<details class="ha-more"><summary id="the-backup-before-an-update">The backup before an update</summary>

Before changing stored data, HolyArrs:

1. Checks there is room for a copy and the update. If not, nothing changes. Skip with `HOLYARRS_SKIP_SPACE_CHECK=yes`.
2. Copies each database to `backups`, checks the copy and writes a sha256 file beside it. After 3 failed copies, nothing changes.
3. Runs each update step on its own and checks it. A failed step is undone; finished steps stay. Start the same version again to carry on from the failed step with the same copy.

After the first good start the copy becomes `holyarrs-backup-pre-update-<time>.zip`, described like "before updating from 1.1.0 to 1.2.0". It shows in **Settings > Backups** and `holyarrs restore --list`. The newest 3 are kept; the backup schedule never removes them. A fresh install or an update that leaves the data alone makes none. This copy is made even when **Settings > Backups > How often** is **Off**.

</details>

<details class="ha-more"><summary id="verify-the-image">Verify the image</summary>

From 1.0.3 on, every image on Docker Hub is signed by the release run on GitHub. There is no key to download. Check an image with [cosign](https://docs.sigstore.dev/cosign/system_config/installation/). Put the version you run, such as `1.0.3`, where it says `<version>`:

```bash
cosign verify holyarrs/holyarrs:<version> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/HolyArrs/HolyArrs/\.github/workflows/sign\.yml@refs/(tags/[0-9.]+|heads/main)$'
```

You should see `The cosign claims were validated`. Anything else means the image did not come from the HolyArrs release run: don't run it.

The image also carries a record of how it was built and a list of what is inside it (an SBOM). Check them with the GitHub CLI:

```bash
gh attestation verify oci://docker.io/holyarrs/holyarrs:<version> --repo HolyArrs/HolyArrs
```

The same files are on each [GitHub release](https://github.com/HolyArrs/HolyArrs/releases): `.sigstore.json` (signature), `.intoto.jsonl` (build record) and `.sbom.spdx.json` (contents).

</details>

## Related

- [Backups](backups.md)
- [Health checks](health.md)
- [Parts and safe mode](parts-safe-mode.md)
