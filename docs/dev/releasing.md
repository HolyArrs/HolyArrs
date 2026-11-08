# Releasing

This page is for maintainers who cut a release.

## Versions

Release tags are plain numbers: `1.0.0`, `1.1.0`, `1.1.1`. Every release is built from a tag on the main branch.

## Image tags

The release workflow builds one image for `linux/amd64`, `linux/arm64` and `linux/arm/v7` and pushes it to GHCR and Docker Hub.

- A new version lands on `:beta` and on its own version tag.
- `:latest` and `:stable` move only after that version has run well for a while.

## Before you tag

1. Every check in [Testing](testing.md) passes, including a full end-to-end run.
2. The `overnight` tests pass.
3. `CHANGELOG.md` has the new version.
4. `THIRD_PARTY_NOTICES.md` is current. Check it with `python3 deploy/release/notices.py --check`.
5. The image builds with `docker build --file deploy/Dockerfile .` and starts on a fresh config folder.

## Release notes

Every release text is built from one file, `releases/notes-<version>.yml`. Write only that file; never hand-edit what it builds.

- `version` (plain `X.Y.Z`), `theme` (one line), and exactly 3 `highlights`, each with `line`, `issue` and `shot`.
- `groups`: one entry per place (Setup, Library, Downloads...), each with `added`, `fixed` and `changed` lists. Every line ends with its issue, for example `(#46)`.
- Optional: `before_you_update` (`breaking`, `manual_steps`, `backup`, `rollback`), `security`, `known_issues`, `credits` (`name`, `link`, `github`, `issues`) and `stats`. Leave anything empty that has nothing real in it; empty parts are hidden everywhere.
- GitHub logins only when known, either in the yml or in `releases/credits-map.json`.

Check it, then build:

```
python deploy/release/notes.py 1.0.0 --check
python deploy/release/notes.py 1.0.0
```

The second command writes `changelog.md`, `github.md`, `update-notes.json`, `reddit.txt`, `discord-dm.txt`, `discord-announce.md` and `roadmap-card.json` to `releases/out/1.0.0/`. `--check` exits 1 on any error and prints warnings for long lines and banned words.

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
python deploy/release/nas-template.py --image 'ghcr.io/coderluii/holyarrs@sha256:<digest>' --config '/mnt/<pool>/appdata/holyarrs-test' --movies /mnt/user/holyarrs-test-data/movies --downloads /mnt/user/holyarrs-test-data/downloads --port 6262 --output holyarrs-test.xml
```

- Use the registry manifest digest of the image you checked, not a Docker image ID.
- `--config` must be a pool path such as `/mnt/<pool>/appdata/holyarrs-test`. `/mnt/user` and `/mnt/user0` are FUSE paths, and the tool refuses them.
- `--movies`, `--tv`, `--music` and `--downloads` are optional. Separate media paths are separate mounts, so imports between them are copied.
- The template sets `PUID=99`, `PGID=100` and `UMASK=022`. Give that account write access to the test media.
- Put the file in `/boot/config/plugins/dockerMan/templates-user/` and pick it under Docker > Add Container > Template.
