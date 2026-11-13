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

4. Build the image and start it on a fresh config folder:

   ```sh
   docker build --file deploy/Dockerfile .
   ```

   You should see the first-run wizard.

5. Run every check in [Testing](testing.md) on your machine. Tests run locally, not on GitHub.
6. Hand the version to the maintainer. The maintainer pushes the version tag on main, and that starts the release.

## Choices

### Version and tag names

| Thing | Name |
|---|---|
| Git tag (pushed by the maintainer) | `1.0.0` (plain numbers, no v) |
| Image tag and notes `version` | `1.0.0` |

The workflow starts only for an `X.Y.Z` tag and stops when the tag differs from the `VERSION` file.

### Image tags

Each release pushes its version tag and `:latest`.

### Take a release back

1. Push the next version with the fix.
2. Mark the bad version in the update list so installs do not offer it.
3. Keep the tag.

## If it does not work

**Symptom.** `notes.py --check` exits 1.
**Cause.** The notes file breaks a rule. The message names the line.
**Fix.** Edit the notes file, never the files in `releases/out`, and run it again.

**Symptom.** The workflow stops with `VERSION says`.
**Cause.** The tag and the `VERSION` file differ.
**Fix.** Fix `VERSION` on main and push the next version tag.

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

The `HolyArrs - Release` workflow starts when a version tag is pushed. It has four steps:

1. Build and push the image for `linux/amd64`, `linux/arm64` and `linux/arm/v7`, with the version tag and `latest`.
2. Build the portable archives for `linux-amd64`, `linux-arm64`, `linux-armv7` and `windows-amd64`, including the source archive and `SHA256SUMS`.
3. Update the Docker Hub description from `README.md`.
4. Publish the GitHub release with this version's `CHANGELOG.md` section and the portable archives as assets.

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
