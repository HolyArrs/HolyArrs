# Releasing

This page is for maintainers who cut a release.

## Versions

Release tags are plain numbers: `1.0.0`, `1.1.0`, `1.1.1`. Every release is built from a tag on the main branch.

## Image tags

The release workflow builds one image for `linux/amd64`, `linux/arm64` and `linux/arm/v7` and pushes it to Docker Hub as `holyarrs/holyarrs`.

- A new version lands on `:beta` and on its own version tag.
- `:latest` and `:stable` move only after that version has run well for a while.

## Before you tag

1. Every check in [Testing](testing.md) passes, including a full end-to-end run.
2. The `overnight` tests pass.
3. `CHANGELOG.md` has the new version.
4. `THIRD_PARTY_NOTICES.md` is current. Check it with `python3 deploy/release/notices.py --check`.
5. The image builds with `docker build --file deploy/Dockerfile .` and starts on a fresh config folder.

## Release notes

Every release text is built from one file, `releases/notes-<version>.toml` (TOML, read with Python's built-in `tomllib`). Write only that file; never hand-edit what it builds.

- `version` (plain `X.Y.Z`), `theme` (one line), `images` (a list of `{registry, name}`; Docker Hub `holyarrs/holyarrs` only), optional `docs_url` (empty = no Docs link) and exactly 3 `[[highlights]]`, each with `line`, `issues` (list of issue numbers) and `shot`. A highlight shows once, at the top; its issues appear in no group line.
- `[groups]`: one list of lines per place (Requests, Library, Downloads...). Lines are new by default; only non-new lines start `Changed: `, `Fixed: ` or `Docs: `. Every line ends with its issue, for example `(#46)`. Places with fewer than 3 lines are shown together under Also new.
- Optional: `[before_you_update]` (`breaking`, `manual_steps`, `backup`, `rollback`), `security`, `known_issues`, `credits` (`name`, `link`, `github`, `issues`) and `[stats]`. Leave anything empty that has nothing real in it; empty parts are hidden everywhere.
- GitHub logins only when known, either in the notes file or in `releases/credits-map.json` (keyed by the bare or the `u/` name).
- Release body order: theme, Highlights, places (Requests, Library, Downloads, Indexers, Accounts, Health, Setup, Install), Also new, Security, Known issues, Before you update, Image, Thanks, stats line. Assets stay in GitHub's own box below the body.

Check it, then build:

```
python deploy/release/notes.py 1.0.0 --check
python deploy/release/notes.py 1.0.0
```

The second command writes `changelog.md`, `github.md`, `update-notes.json`, `reddit.txt`, `discord-dm.txt`, `discord-announce.md` and `roadmap-card.json` to `releases/out/1.0.0/`. `--check` exits 1 on any error (banned words, em dashes, dates or launch talk, a highlight issue repeated in a group, a Discord text over 1,000 characters) and prints warnings for long lines and near-repeats.

## Update list

The update check in the app reads a signed update list. Each release adds its entry and signs the list. The signing key never goes in the repo. HolyArrs refuses a list it can't verify.

## After a release

- Check that the new image starts and its version label matches the tag:
  `docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' holyarrs`
- Watch new issues for the first day.

## Taking a release back

If a release is bad, move `:beta` back to the previous version and mark the bad one in the update list, so installs don't offer it. Keep the tag.

## Unraid test template

No NAS package is published yet. `deploy/release/nas-template.py` writes an Unraid user template for testing one exact image on your own box. It does not pull, install or publish anything.

```sh
python deploy/release/nas-template.py --image 'holyarrs/holyarrs@sha256:<digest>' --config '/mnt/<pool>/appdata/holyarrs-test' --movies /mnt/user/holyarrs-test-data/movies --downloads /mnt/user/holyarrs-test-data/downloads --port 6262 --output holyarrs-test.xml
```

- Use the registry manifest digest of the image you checked, not a Docker image ID.
- `--config` must be a pool path such as `/mnt/<pool>/appdata/holyarrs-test`. `/mnt/user` and `/mnt/user0` are FUSE paths, and the tool refuses them.
- `--movies`, `--tv`, `--music` and `--downloads` are optional. Separate media paths are separate mounts, so imports between them are copied.
- The template sets `PUID=99`, `PGID=100` and `UMASK=022`. Give that account write access to the test media.
- Put the file in `/boot/config/plugins/dockerMan/templates-user/` and pick it under Docker > Add Container > Template.
