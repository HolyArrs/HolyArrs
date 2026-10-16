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
