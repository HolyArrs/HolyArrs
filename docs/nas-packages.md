# NAS package preparation

No NAS package is published or accepted yet. The current prepared format is
an Unraid local Docker user template. Synology, QNAP and TrueNAS native
packages are unimplemented and unverified. An ordinary Docker host or a
Windows media share does not prove NAS catalog acceptance.

## Make a private Unraid candidate

First obtain the immutable registry manifest digest of the exact image you
have independently verified. A Docker image ID is not a registry manifest
digest. Do not invent a public tag or use a digest from another source build.
The renderer does not pull, install or publish anything:

```sh
python scripts/nas-template.py --image 'ghcr.io/coderluii/holyarrs@sha256:<verified-manifest-digest>' --config /mnt/cache/appdata/holyarrs-candidate --data /mnt/user/holyarrs-test-data --port 6262 --output holyarrs-candidate.xml
```

Replace `cache` with your actual local pool name and the digest placeholder
with 64 hexadecimal digits. The output must not already exist. The image
must be available to the target; private registry access is a separate
requirement. The template is for local testing, not a Community Applications
submission. Current submission requirements are controlled by Unraid's
[Applications documentation](https://docs.unraid.net/unraid-os/manual/applications/).

On an isolated authorized Unraid target, local user templates belong under
`/boot/config/plugins/dockerMan/templates-user/` and can be selected through
Docker > Add Container > Template > User templates. See Unraid's
[Community Applications documentation](https://docs.unraid.net/community-applications/).
That manual install does not publish a Community Applications catalog entry.

Config uses a **direct local pool path** such as
`/mnt/cache/appdata/holyarrs-candidate`. Do not use
`/mnt/user/appdata/holyarrs`: that is a FUSE path, which HolyArrs refuses for
SQLite WAL. `/mnt/user0`, remote mounts and overlapping config/data paths are
also refused by the renderer. Path spelling alone does not prove the
filesystem is local; check the target's actual mount before starting.

The container runs as numeric user/group `1000:1000`, without privilege or
a Docker socket. Give that account write access to the isolated config and
test data. No `PUID`/`PGID` support is implied. The template mounts one data
tree at `/data`; downloads and Library folders must share a filesystem for
hardlinks. Never run two instances against one config directory.

## Required actual-target checks

Use new task-owned config/data paths and an unused host port. Preserve
existing services. Record NAS model, CPU, OS/version, Docker version, actual
mount type, numeric account and ACLs. Then:

1. Install the entry through the NAS interface and finish first-run setup.
2. Verify the embedded web app, sign-in, permissions and synthetic file work.
3. Restart and confirm owner, encrypted settings and Library folders persist.
4. Take a checked backup and retain the matching master key separately.
5. Replace with a different verified candidate without changing mounts.
6. Restore in a disposable config, restart, and verify recovered settings.
7. Remove only the owned test installation after confirming health and
   preserving its sanitized receipt.

Keep the previous image and complete compatible config for rollback.
Archive passwords, keys and databases only in protected private storage,
never the catalog or a published receipt. [Portable recovery](portable.md#update-and-recover)
and [Install](install.md) explain the key and database constraints.

The roadmap 8.1.3 acceptance gate stays open until a real lab NAS installs
from an entry and these checks pass. Offline XML tests prove only template
structure and input validation.
