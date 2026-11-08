# Update check

HolyArrs can tell you when a newer version is out. It never updates by itself: you pull the new image when it suits you.

## What it sends

Once a day, when the check is on, HolyArrs makes one GET request to `api.github.com` (the default feed). The only thing it says about itself is the header `User-Agent: HolyArrs/<version>`. No account, no key, no list of your media. Off means no request at all. The welcome page and Settings > This app say the same.

## How it works

- Once a day (with the health checks) HolyArrs asks the release feed for the newest release. The answer is kept, so a restart does not ask again within the day.
- The default feed is the HolyArrs project's releases address (`https://api.github.com/repos/CoderLuii/holyarrs/releases/latest`). Until a release is published it answers "not found"; HolyArrs says "No HolyArrs release is published yet" and raises nothing.
- A newer release shows on Settings > This app > Updates and as a warning in the health checks. The "What is new" link is shown only when the release page is under `https://github.com/CoderLuii/holyarrs/releases/` (no other host, port, user, query or `..`); any other link is dropped and the version is shown as text.
- A pre-release (a version with a suffix, such as `v1.2.0-rc1`) counts as older than the plain version and gives no notice unless "Tell me about test versions too" is ticked (off by default).
- Redirects: at most 3, and never to another scheme (http to https or back).
- A failed ask (no internet, the feed down) is shown on This app only; it is not a health warning.
- One ask takes at most 15 seconds and reads at most 512 KB.
- The ask goes through the same network rule as other links: public addresses only, unless "The release feed may be on the local network" is ticked. Cloud metadata addresses are always refused.

## Settings (This app > Health check settings)

| Setting | Default | What |
|---|---|---|
| Check once a day for a newer HolyArrs | on | Off: HolyArrs makes no request at all |
| Release feed | empty (the project's releases); never holds a secret: a name, password, query or fragment is refused | Another feed: a GitHub-style release JSON (`tag_name`, `html_url`), a list of releases, any JSON with `version`, or plain text holding the version |
| The release feed may be on the local network | off | For a feed on your own network |
| Tell me about test versions too | off | A pre-release may give a notice |

"Check for updates now" on This app asks at once (people who may change settings).

## API

| Route | Who | What |
|---|---|---|
| `GET /api/v1/app/update` | run the Library (read scope) | `on`, `feed` (only for people who may change settings, as scheme, host and path), `checked_at`, `latest`, `newer`, `link`, `no_release`, `problem`, `version`; never asks the feed |
| `POST /api/v1/app/update/check` | settings (write scope) | asks now (nothing when off) |
| `PUT /api/v1/settings/health` | settings | `update_check`, `update_check_url`, `update_check_local`, `update_prerelease` |

## A backup before every update of the data

A new version may need to change how the data is stored. Before it changes anything, HolyArrs:

1. Checks there is room for a copy and for the update (`HOLYARRS_SKIP_SPACE_CHECK=yes` skips this). Too little room: nothing is changed and the page says so.
2. Copies each database to `backups/`, checks the copy and writes its sha256 next to it. If the copy fails three times, nothing is changed and the page says so.
3. Runs each step of the update in its own transaction and checks the result. A step that fails is rolled back and the app stops on a page that says what happened. Steps that finished before it stay; the copy from before the whole update is kept, and starting the same version again carries on from the failed step with that same copy.

On the first good start the copy becomes a pre-update backup (`holyarrs-backup-pre-update-<time>.zip`, "before updating from 1.1.0 to 1.2.0"). It shows in Settings > Backups and restore and in `holyarrs restore --list`. The newest 3 are kept; the backup schedule never removes them and older ones go only after a good start. A fresh install has nothing to copy, so it makes none. An update that does not change the data makes no pre-update backup (the usual update backup is still made).

## Going back

HolyArrs cannot change its own image, so going back is two steps: you run the older image, and HolyArrs puts the older data back.

1. Settings > This app > Updates shows "Go back to <version>" when there is a pre-update backup for the update to the version you run. It shows the image to use (`ghcr.io/coderluii/holyarrs:<version>`, with a Copy button). If you pull from another registry, change only the tag.
2. Change the image in your compose file (or `docker run`) to that tag and start the container (`docker compose up -d`).
3. If the update did not change the data, the older version just starts. If it did, the older version sees newer data and shows a page instead of the app. That page offers "Put back the data from before the update". To confirm, type the one-time code it wrote to the log (`docker logs <container>`, look for "one-time code").
4. HolyArrs checks the backup, puts it in place, and stops; Docker's restart policy starts it again on that data. Without a restart policy, start the container.

Notes:

- Anything changed after the update (new titles, settings, history) is not in that backup.
- Nothing is deleted. The newer data moves to `backups/before-restore-<time>/`, so going back can be undone.
- After going back more than one version (1.2.0 to 1.0.0), HolyArrs picks the newest pre-update backup that version can read: the one taken before the update from 1.0.0.
- The page is open to anyone who can reach HolyArrs, and no accounts can be read while the data is refused, so the button needs the code from the log. After 10 wrong codes it refuses until the next start, which writes a new code.
- Without the page: `docker exec <container> /holyarrs restore --pre-update`, then `docker restart <container>`. Without Docker: stop HolyArrs, run `holyarrs restore --pre-update`, start it again.
