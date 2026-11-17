# Publishing a plugin

A checklist for the maintainer. Plugins are built, signed and hosted by us
only. The workflow **HolyArrs - Plugins** (`.github/workflows/plugins.yml`)
does all of it.

## Where things live

- Plugin code: `app/plugins/<id>/`, with its own `go.mod`.
- What users see on the Plugins page: `app/plugins/<id>/plugin.json` (name,
  author, description, icon, access, min_app_version). Leave `id`, `version`
  and `sha256` out; the workflow fills them in.
- The plugin files: a GitHub release on this repo per version, tagged
  `plugin-<id>-v<version>`, with one file per platform (`<id>-linux-amd64`,
  `<id>-linux-arm64`, `<id>-linux-armv7`). Never marked as the latest release.
- `https://plugins.holyarrs.com/`, a small Cloudflare Pages site
  (`holyarrs-plugins`) holding:
  - `plugins.json` and `blocklist.json`, the two signed lists
  - `plugins/<id>/<version>/<platform>/manifest.json`
  - `plugins/<id>/<version>/<platform>/plugin`, a redirect to the file on its
    release. The app trusts a file only when its hash matches the signed list,
    wherever it was downloaded from.

## Publish a new version

1. Merge the plugin change.
2. Tag the commit and push the tag:

   ```powershell
   git tag plugin-tailscale-v1.0.1
   git push origin plugin-tailscale-v1.0.1
   ```

3. The workflow builds the three platforms, puts them on the release, adds
   them to the list, signs both lists with the main key and deploys the site. Every HolyArrs install sees the new
   version the next time it reads the list.

A version can be published once. To change it, publish the next version.

## Try it first (dry run)

Actions > HolyArrs - Plugins > Run workflow, action `publish`, leave **Dry run**
ticked. It builds and signs but uploads nothing; the signed files are kept as a
download on the run for 30 days.

## Pull a plugin (blocklist)

Actions > HolyArrs - Plugins > Run workflow:

- action `block`, the plugin id, and the version (leave empty to block every
  version)
- untick **Dry run**

The blocked versions leave the list, so no one can install them, and installs
that already have them switch them off at their next blocklist check. Publish a
fixed version the normal way when it is ready.

## Counters

Each list has a counter that only goes up; the app refuses a list with a lower
counter than the last one it saw. The workflow always starts from the live lists
(checked against the keys the app pins), so you never set a counter by hand. If
plugins.holyarrs.com cannot be reached, the workflow stops instead of guessing.

## One-time setup

- Repository secrets: `PLUGIN_SIGNING_KEY` (the key file's contents) and
  `PLUGIN_SIGNING_PASSPHRASE`, `PLUGINS_CLOUDFLARE_TOKEN` (an API token that may
  only edit Cloudflare Pages) and `PLUGINS_CLOUDFLARE_ACCOUNT_ID`.
- A GitHub environment named `plugins`. Add yourself as a required reviewer, so
  nothing is published without your click.
- In Cloudflare: a Pages project `holyarrs-plugins` (direct upload), with
  `plugins.holyarrs.com` added as its custom domain.

## Signing by hand

The same tool runs on your own PC, with the key from your password manager. See
`docs/plugin-signing-key.md`.

```powershell
go build -o pluginkey.exe ./app/backend/tools/pluginkey
.\pluginkey.exe block -id tailscale -version 1.0.0 -current live -out out
.\pluginkey.exe sign -key <key file> -kind blocklist -in out\blocklist.payload.json -out out\blocklist.json -min-counter <BLOCK_MIN>
.\pluginkey.exe sign -key <key file> -kind plugins -in out\plugins.payload.json -out out\plugins.json -min-counter <LIST_MIN>
.\pluginkey.exe check -kind plugins -in out\plugins.json
```

`live` holds the current `plugins.json` and `blocklist.json`; `out\counters.env`
gives the two `-min-counter` values.
