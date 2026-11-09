---
title: Releasing
description: Write release notes, check a build, publish an image and take a bad release back. For maintainers.
level: Expert
sidebar: { order: 6 }
---

Turn a tested build into a release. This page is for maintainers.

:::caution
Only the maintainer starts a release. Do not tag, publish or move an image tag on your own.
:::

## Before you start

- Every check in [Testing](testing.md) passes, including a full end-to-end run and the long and fault runs.
- `CHANGELOG.md` has the new version.
- Python 3 is installed for the release scripts.

## Prepare a release

1. Write `releases/notes-<version>.toml`, for example `releases/notes-1.0.0.toml`. See [Release notes file](#release-notes-file).
2. Check it:

   ```sh
   python deploy/release/notes.py 1.0.0 --check
   ```

   You should see no errors. A banned word, an em dash, a date, launch talk or a repeated highlight issue exits 1.

3. Build the release texts:

   ```sh
   python deploy/release/notes.py 1.0.0
   ```

   You should find 7 files in `releases/out/1.0.0/`.

4. Check the third-party notices:

   ```sh
   python3 deploy/release/notices.py --check
   ```

5. Build the image and start it on a fresh config folder:

   ```sh
   docker build --file deploy/Dockerfile .
   ```

   You should see the first-run wizard.

6. Hand the version to the maintainer. A release starts by hand on the main branch; the run makes the tag.

## Choices

### Version and tag names

| Thing | Name |
|---|---|
| Git tag (made by the release run) | `v1.0.0`, or `v1.0.0-rc.1` for a candidate |
| Image tag and notes `version` | `1.0.0` |

The workflow refuses any other version shape, a version that differs from the `VERSION` file, and a version that already has a tag.

### Beta first, stable later

A new version lands on `:beta` and on its version tag. `:beta` only moves to a newer version. `:stable`, `:latest` and the short tags `X` and `X.Y` move only when the maintainer starts the promote run for a version that ran well on beta. That run takes the image digest from the release run, and it never moves `:stable` back to an older version.

### Take a release back

1. Move `:beta` to the previous version.
2. Mark the bad version in the update list so installs do not offer it.
3. Keep the tag.

## If it does not work

**Symptom.** `notes.py --check` exits 1.
**Cause.** The notes file breaks a rule. The message names the line.
**Fix.** Edit the notes file, never the files in `releases/out`, and run it again.

**Symptom.** The workflow stops with a message about the tag.
**Cause.** The tag is not `vX.Y.Z` or `vX.Y.Z-rc.N`.
**Fix.** Make a tag with that shape.

**Symptom.** The beta run says `beta stays on` a version.
**Cause.** You tagged an older version than the current beta.
**Fix.** Tag a newer one. Beta never moves back.

## For experts

<details class="ha-more"><summary id="release-notes-file">Release notes file</summary>

Every release text comes from one file. Never hand-edit what it builds.

| Field | Content |
|---|---|
| `version` | Plain `X.Y.Z` |
| `theme` | One line |
| `images` | List of `{registry, name}`. Docker Hub `holyarrs/holyarrs` only |
| `docs_url` | Optional. Empty means no Docs link |
| `[[highlights]]` | Exactly 3, each with `line`, `issues` and `shot`. Their issues appear in no group line |
| `[groups]` | Lines per place. New is the default. Others start `Changed: `, `Fixed: ` or `Docs: `. Each ends with its issue, such as `(#46)`. A place with under 3 lines goes under Also new |
| Optional | `[before_you_update]` (`breaking`, `manual_steps`, `backup`, `rollback`), `security`, `known_issues`, `credits`, `[stats]`. Empty parts are hidden |

Use a GitHub login only when it is known, in the notes file or in `releases/credits-map.json`.

Body order: theme, Highlights, places (Requests, Library, Downloads, Indexers, Accounts, Health, Setup, Install), Also new, Security, Known issues, Before you update, Image, Thanks, stats line.

| Output in `releases/out/<version>/` | Used for |
|---|---|
| `changelog.md` | The changelog entry |
| `github.md` | The GitHub release body |
| `update-notes.json` | The notes the app shows |
| `reddit.txt` | The Reddit post |
| `discord-dm.txt`, `discord-announce.md` | Discord. Each stays under 1,000 characters |
| `roadmap-card.json` | The roadmap card |

`notes.py` takes `--file` (another notes file), `--map` (another credits map), `--out` (another output folder) and `--date`. `--check` warns on long lines and near repeats.

</details>

<details class="ha-more"><summary>The image workflow</summary>

The `release` workflow runs from main only, for the version in `VERSION`, and only when the maintainer ticks confirm_release. It:

1. Runs `go test ./...` before building.
2. Builds one image for `linux/amd64`, `linux/arm64` and `linux/arm/v7`.
3. Pushes `holyarrs/holyarrs` with the version tag, and `beta` when the version is newer than the current beta.
4. Checks that every platform and the digest match.
5. Signs the image with cosign and attaches an SBOM.

The `promote` workflow starts by hand, runs as a dry run unless you switch that off, and moves the stable tags by digest.

</details>

<details class="ha-more"><summary>Update list</summary>

The app reads a signed update list. Each release adds its entry and signs it. The signing key never goes in the repo. HolyArrs refuses a list it cannot verify.

After a release, check that the image's version label matches the tag, and watch new issues for the first day:

```sh
docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' holyarrs
```

</details>

<details class="ha-more"><summary>Unraid test template</summary>

`deploy/release/nas-template.py` writes an Unraid user template for one exact image. It pulls, installs and publishes nothing.

```sh
python deploy/release/nas-template.py --image 'holyarrs/holyarrs@sha256:<digest>' --config '/mnt/<pool>/appdata/holyarrs-test' --movies /mnt/user/holyarrs-test-data/movies --downloads /mnt/user/holyarrs-test-data/downloads --port 6262 --output holyarrs-test.xml
```

Replace `<digest>` and `<pool>`.

| Flag | Notes |
|---|---|
| `--image` | The registry manifest digest, not a Docker image ID |
| `--config` | A pool path. The tool refuses `/mnt/user` and `/mnt/user0` |
| `--movies`, `--tv`, `--music`, `--downloads` | Optional. Separate paths are separate mounts, so imports copy |
| `--port` | Defaults to `6262` |
| `--allowed-hosts` | Optional, comma-separated host names |
| `--output` | The XML file to write |

The template sets `PUID=99`, `PGID=100` and `UMASK=022`. Give that account write access to the test media. Put the file in `/boot/config/plugins/dockerMan/templates-user/` and pick it under **Docker > Add Container > Template**.

</details>

## Related

- [Testing](testing.md)
- [Contributing](contributing.md)
- [Updates](../run/updates.md)
