# Portable candidates

Portable archives are development candidates. They are not a signed release.
The v1 public delivery plan remains Docker. NAS installation is still
unverified; no NAS platform support is claimed.

## Build a committed candidate

The packaging command uses Python 3, Git, Docker and the pinned Go 1.27.2
builder already used by the project. It never pulls an image or fetches a
module. Supply a complete module cache path or Docker volume with
`--module-cache`; it is mounted read-only. Populate it with the locked
modules through the normal project build first. The installed frontend dependencies must
match `web/app/package-lock.json`; they supply their license notices.

From the checkout:

```sh
python scripts/package.py --ref HEAD --version 0.0.0-candidate.1 --output /srv/holyarrs-candidates/candidate-1 --web-node-modules web/app/node_modules --module-cache /home/you/go/pkg/mod
```

Use a new output directory outside the checkout. An existing output is
refused. The command exports the committed source, including `web/dist`,
and builds there with networking disabled. Uncommitted edits and runtime
state are excluded. Rebuild and commit the web app before packaging a UI
change. Go modules are verified and vendored for an offline source rebuild.

For a constrained machine, add `--low-resource`. Each builder container is
limited to one CPU, 2 GiB memory with no additional swap, and 256 PIDs. Go uses
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `GOMEMLIMIT=768MiB` and `GOGC=50`. The Go memory
setting is a target within the container's hard limit. The option leaves normal
build defaults unchanged. Python assembles the archives on the host, outside
these container limits; apply host process limits separately when needed.

The default matrix is Linux amd64, arm64, armv7 (`GOARM=7`), 386 and Windows
amd64. Every selected target must build or the command fails without
publishing the output directory. To select a subset, pass `--targets` with
comma-separated names. Linux amd64 uses `GOAMD64=v1`; arm64 uses `GOARM64=v8.0`.
Compilation alone is not native platform acceptance.

The resolved commit is appended to the supplied version as build metadata
and embedded into the executable. The running app and its backups report
that candidate identity. It does not create a release tag.

Each archive includes the program, license, third-party notices, guides,
source manifest and package identity. `SHA256SUMS` covers the archives.
The source archive includes the project source and vendored Go dependencies.
Frontend dependencies are identified by the lockfile, with notices for the
bundled runtime and fonts; their complete source is not included. Keep the
source archive beside its binaries. The manifest records the Git commit,
tree and file hashes. `package.json` records the target, compiler, builder,
source archive hash and executable hash. These checksums are not signatures.

Archives use fixed ordering and normalized timestamps and ownership. Repeat
the command with the same commit, version, locked inputs and toolchain to
check reproducibility. A failed build leaves its requested output absent.

## Run without installing a service

Verify `SHA256SUMS`, then extract the archive into a new directory. Keep the
program and config in separate directories. Choose explicit local paths:

```sh
GOMEMLIMIT=384MiB ./holyarrs -config /srv/holyarrs/config -addr :18730
```

On Windows PowerShell:

```powershell
$env:GOMEMLIMIT = '384MiB'
.\holyarrs.exe -config 'C:\HolyArrs\config' -addr ':18730'
```

The default config path is `/config`; always supply your own. Node is not
needed at runtime. Linux needs readable system CA certificates for HTTPS;
the static binary does not embed a CA trust store. Run under a dedicated
account with write access only to config and the intended data tree. On
Windows use an NTFS ACL restricted to that account; Unix file modes do not
prove Windows ACL protection.

Without `GOMEMLIMIT` the program uses a 384 MiB memory target, the same
as in the Docker image; set `GOMEMLIMIT` to change it (about 80 percent
of the memory you give HolyArrs). This is a Go memory target, not a hard
process limit. Use your service manager for resource
limits and restart handling; packaging does not create a service.

On a machine with about 2.5 GiB of memory or less, HolyArrs turns on its
low-memory mode by itself (Linux). `HOLYARRS_LOW_MEMORY=1` or `=0` decides it
yourself. See [Low-memory mode](parts-and-safe-mode.md#low-memory-mode).

Open port 18730 at your host's network address, read `setup-code.txt` only
from the private config directory, and create the owner account. See
[Install](install.md#finish-the-first-run) for the remaining setup and data
permissions. Config must stay on local disk. Keep downloads and Library
folders in one filesystem when hardlinks matter.

## Update and recover

1. Verify the new archive and its source identity before stopping the app.
2. Stop it gracefully and take a protected copy of the complete config,
   including the master key. Copy only while the app is stopped, or use a
   checked in-app backup and keep its matching key separately.
3. Extract the new program into a different directory. Keep the previous
   program and its matching pre-update config. Start the new program against
   the intended config and verify sign-in, health and saved settings.
4. A restore through Settings > Backups stages the databases and exits 75.
   Restart the program to apply it. A standalone process has no automatic
   restart policy unless you set one in your service manager.

Rollback requires the old program **and its matching pre-update config**.
Never point an older program at databases migrated by a newer one. Preserve
the new config separately before rollback so later changes are not silently
lost. Test recovery with a disposable config copy first. See [Backups](backups.md).

## Evidence classes

| Platform | What packaging can establish | Remaining acceptance |
| --- | --- | --- |
| Windows amd64 | Native disposable first-run, restart and backup/recovery when a receipt records them | Service installation, ACL hardening and a separate user's experience |
| Linux amd64 | Cross-build plus native Linux runtime only when a receipt records it | NAS OS and catalog installation |
| Linux arm64 | Cross-build | Native ARM64, Pi hardware and day-long resource gates |
| Linux armv7 and 386 | Mandatory selected-target cross-build for these candidates | Native runtime and platform installation; historical support policy remains in decision 0002 |
| Unraid, Synology, QNAP, TrueNAS | Unraid template preparation only | Every actual NAS installation check |

Exact tested versions and hashes belong in the candidate's acceptance receipt.
