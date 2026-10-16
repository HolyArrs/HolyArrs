# Isolated Pi-like and NAS-like test environments

Luis has no physical Raspberry Pi or NAS available. These environments provide
reproducible emulation and filesystem-contract coverage with synthetic data and
task-owned Docker resources. They never use the workstation's `X:` drive, the
media server, host port 445, firewall rules or network-adapter settings.

The standard-library CLI is `tools/device-simulation/device_simulation.py`.
Its tests and machine-readable scenario contract live beside it.

## Pi-like environment

`prepare-arm` requires a `holyarrs.qemu-build.v2` receipt for each architecture.
That receipt binds the source commit/tree/manifest/archive, preparation tool,
build command and environment, test binary, explicit userspace-QEMU executable,
immutable local test image and three-case child-process proof. The proof binds
the exact helper source, binary, image, test arguments and parent entrypoint. It
records either an actual exec-format failure or actual implicit-handler success
with the executor omitted, a required not-found failure for a fixed impossible
executor path, and a required PASS through the exact hashed userspace executor.
All three raw child logs are retained beside the preparation receipt under the
contract's fixed names. Preparation checks their bounded byte counts and raw
hashes, then reads the files to confirm the recorded exec-format, not-found or
exact test PASS result.

Version 2 also requires independently hash-pinned target-graph and module-cache
inputs. The target graph contains both `linux-arm64` and `linux-arm-v7`.
Preparation rereads both bounded Go package JSON logs, rejects error-bearing or
incomplete package records, requires the main target's `testexec_test.go`, and
derives each architecture's module set plus their exact union. The offline cache
must bind that graph hash and contain exactly `.info`, `.mod`, `.zip` and
`.ziphash` for every union pair. The receipt selects the exact architecture set,
retains all five build-step logs, and binds the same cache counts.
The workspace root, every ancestor and every evidence leaf must remain an
ordinary non-link/non-reparse path. Preparation validates the full path before
and after each exact held-file read; the bounded cache archive is hashed and
parsed through the same open handle.

The producer's five `container-<owned-name>.json` files are not fields covered
by its receipt. The evidence-root owner must create and independently hash a
`holyarrs.qemu-container-states.v1` manifest. It binds the exact source commit,
selected variant, preparation-receipt hash, five fixed owned names, unique full
container IDs, file lengths and raw hashes. Preparation rereads every safe
regular non-reparse state file and requires its exact `id` and `name`. This is
an explicit external evidence-root trust binding rather than producer receipt
provenance. The producer receipt must still say its containers were cleaned and
identify the one retained scratch image.
It observes host handler behavior without installing or changing handlers. The existing AMD64 runner
receipt describes a tool image only and cannot satisfy this contract. No
final-source ARM receipt exists yet, so preparation fails closed until its
producer emits the agreed fields.

```powershell
python tools/device-simulation/device_simulation.py prepare-arm `
  --workspace . `
  --source-commit <final-40-hex-commit> `
  --source-tree <final-40-hex-tree> `
  --source-manifest <final-source-manifest.json> `
  --source-manifest-sha256 <raw-sha256> `
  --prepare-receipt <final-arm64-prepare-receipt.json> `
  --prepare-receipt-sha256 <raw-sha256> `
  --preparation-tool-sha256 <raw-sha256> `
  --target-graph-manifest <target-graph.json> `
  --target-graph-manifest-sha256 <raw-sha256> `
  --module-cache-manifest <module-cache.json> `
  --module-cache-manifest-sha256 <raw-sha256> `
  --module-cache-archive <module-cache.tar> `
  --module-cache-archive-sha256 <raw-sha256> `
  --container-states-manifest <container-states.json> `
  --container-states-manifest-sha256 <raw-sha256> `
  --architecture arm64 --idle 30m --run-id arm64-30m-<unique> `
  --output <new-run-directory>/plan.json
```

Repeat with `--architecture armv7`. An explicitly requested historical harness
may add `--allow-historical-harness`; its plan retains the historical source
identity, sets `final_source_gate` false and cannot satisfy final-source
acceptance.

The separate day-long run uses `--idle 24h`; it does not replace the original
30-minute scale gate. A 24-hour receipt passes only when HolyArrs reports exactly
86,400 idle seconds and container timestamps prove at least 86,400 real elapsed
seconds. `watch-arm` observes the same full container ID at the selected 30–300
second interval. Its record limit is derived from the duration and interval.
A gap over the interval plus 30 seconds, a non-increasing monotonic timestamp,
or disagreement between monotonic span, wall span and container duration leaves
an incomplete run. An interrupted watcher, host interruption or Docker
interruption cannot be resumed as a continuous 24-hour pass.

```powershell
$planHash = (Get-FileHash <run>/plan.json -Algorithm SHA256).Hash.ToLower()
python tools/device-simulation/device_simulation.py start-arm --plan <run>/plan.json --plan-sha256 $planHash --state <run>/state.json
$stateHash = (Get-FileHash <run>/state.json -Algorithm SHA256).Hash.ToLower()
python tools/device-simulation/device_simulation.py watch-arm --plan <run>/plan.json --plan-sha256 $planHash --state <run>/state.json --state-sha256 $stateHash --poll-seconds 60
$stateHash = (Get-FileHash <run>/state.json -Algorithm SHA256).Hash.ToLower()
python tools/device-simulation/device_simulation.py status-arm --plan <run>/plan.json --plan-sha256 $planHash --state <run>/state.json --state-sha256 $stateHash --receipt <run>/receipt.json --log <run>/arm-runtime.log
```

`status-arm` accepts only the fixed receipt-relative `arm-runtime.log`. The
receipt records its raw byte count and SHA-256, and `verify-arm` independently
requires a bounded regular non-link/non-reparse file, checks those values and
parses the retained `SCALERESULT` back into the exact receipt result.

For a failed or interrupted run, recompute the state hash and run `cleanup-arm`
with the same plan and state arguments.

The reconstructed Docker command preserves the four-CPU, 1-GiB memory/no-extra-
swap profile, `GOMEMLIMIT=384MiB`, user `1000:1000`, no network, read-only root,
bounded tmpfs and 256 PIDs. Before start and again before collection, the CLI
compares exact command, entrypoint, full environment, mounts, tmpfs, security
options, capabilities, network, user and resources against the immutable plan.
The combined workload remains 10,000 titles, 55,000 items, 20 fake feeds, two
Library folders, 29,000 files, two found files, zero unreachable folders and ten
in-process live clients receiving ten rescan events. Those clients are not ten
rendered browser tabs.

Every container follows create, full-ID persistence, inspect, start/wait and
exact cleanup. The creation intent is durable before the daemon call; an
ambiguous create with no visible ID stays unresolved until later reconciliation
finds and removes the exact named/labeled container. Cleanup proves a known
labeled full ID absent. The prepared image
invokes its pinned QEMU executable directly as its entrypoint; this suite does
not install, remove or change global `binfmt`/QEMU handlers.

## NAS-like environment

Create a plan after the final candidate image exists. Local image IDs are valid
for this local simulation; they are not registry manifest digests.

```powershell
python tools/device-simulation/device_simulation.py prepare-nas `
  --workspace . `
  --source-commit <final-40-hex-commit> `
  --source-tree <final-40-hex-tree> `
  --source-manifest <final-source-manifest.json> `
  --source-manifest-sha256 <raw-sha256> `
  --candidate-image sha256:<64-hex> `
  --candidate-revision <final-40-hex-commit> `
  --candidate-receipt <candidate-image-receipt.json> `
  --candidate-receipt-sha256 <raw-sha256> `
  --baseline-image sha256:<64-hex> `
  --baseline-revision <baseline-40-hex-commit> `
  --app-images-receipt <nas-app-images-receipt.json> `
  --app-images-receipt-sha256 <raw-sha256> `
  --helper-image sha256:<64-hex> `
  --helper-receipt <helper-image-receipt.json> `
  --helper-receipt-sha256 <raw-sha256> `
  --port 18746 --run-id nas-sim-<unique> `
  --output <new-run-directory>/nas-plan.json
```

The reviewed `holyarrs.nas-image.v1` candidate receipt binds its immutable local
image ID to the actual source commit/tree/manifest, tested binary and passing
test log. Preparation re-hashes the manifest, binary, test log and receipt. The
candidate OCI revision must equal that receipt's actual source commit. An
explicit `--allow-historical-harness` plan retains the receipt's historical
source and binary hashes and sets `final_source_gate` false. The baseline uses
its independently supplied revision. A separate hash-bound
`holyarrs.nas-app-images.v1` receipt comes from read-only inspection of the
exact immutable candidate and baseline IDs. It records each image's exact
revision, user, command, entrypoint and allowlisted public environment defaults
(`PATH`, `GOMEMLIMIT`, `GOGC` and `TZ`), plus the separately proven
`/holyarrs` binary entrypoint. The plan derives one exact resulting container
contract per full image ID by applying the explicit config path, address,
memory limit and synthetic save path. The two explicit environment values stay
first in CLI order, followed by inherited defaults whose names were not
overridden. Unexpected or duplicate inherited environment names are rejected.
If candidate and baseline are the same image,
their revisions and one stored contract must also be the same. The helper is
bound by immutable ID,
Dockerfile hash, user and entrypoint from its reviewed receipt; no HolyArrs
revision is fabricated for that external helper.

The runtime uses labeled volumes for config, data, stopped backup, disposable
restore and an empty-data case. Config is local, writable and owned by numeric
UID/GID 1000. Data contains only synthetic downloads, movies, shows and music,
and is mounted read-only into the app. Receipts retain sanitized manifest
records and raw-byte digests. Entry records include type, mode, UID, GID, size
and relative path; content records include file hashes and relative paths.

The suite creates a synthetic owner, signs in, stores a dummy encrypted setting,
proves owner and setting persistence through restart and candidate update,
backs up stopped config, restores into a disposable volume, and starts from the
restored copy. The generated password and dummy setting remain in process
memory and never enter state, harness output or receipts. The setup code is
read through a task-owned helper with the config volume mounted read-only; it
is never copied into state or receipts. App and helper containers are created
with unique names, persisted by full ID, inspected before start, and removed in
`finally` cleanup paths. Before their first start, inspection also requires the
returned full ID, exact slash-prefixed name, created/not-running state, PID
zero, zero start/finish timestamps and zero restart count. App inspection
compares the full command, entrypoint, environment and exact volume sources;
helper inspection compares each exact source, destination, type and access
mode and rejects duplicate or extra destinations. Volume and network inspection records
exact names, labels, drivers, scope, the full network ID and the network's
internal flag. Create recovery polls for delayed daemon visibility; cleanup
discovers and removes labeled owned orphans before proving stable absence. A
create intent is written before every daemon call. If an ambiguous call exposes
no ID within the bounded poll, that intent remains `outcome-unknown`; negative
polls cannot complete cleanup. A later cleanup reconciles the same exact name
and label, persists the full ID if it appears, removes it and only then closes
the intent.

Initialization mounts all five task-owned volumes. Its root helper drops every
capability and adds back only `CHOWN`; inspection rejects a missing, duplicate
or extra added capability. It records the synthetic hardlink and copy facts
before its final recursive ownership change to UID/GID 1000. Every later
filesystem helper, including manifest reads, setup-code read, UID-write and
read-only checks, stopped backup, interrupted copy and restore, runs explicitly
as `1000:1000` with no added capabilities. The stopped source config is
manifested before backup, and verification requires exact source, backup and
restore entry/content equality while the data before/after equality remains
independent.

Storage checks are deliberately separate:

- The existing Linux store source contract rejects network-backed `/config`.
- Actual local-config plus network-data behavior is not covered.
- A same-filesystem operation proves equal device/inode identity.
- A two-filesystem copy proves content-hash equality and source preservation.
- A disposable interrupted copy creates and removes a partial restore file,
  proves its source unchanged, then completes and validates a clean restore.
- Read-only data rejects an unprivileged UID-1000 write and remains byte/entry
  identical before and after the app lifecycle.

The filesystem operations are platform checks. Final Go tests separately cover
HolyArrs importer behavior.

```powershell
$planHash = (Get-FileHash <run>/nas-plan.json -Algorithm SHA256).Hash.ToLower()
python tools/device-simulation/device_simulation.py run-nas --plan <run>/nas-plan.json --plan-sha256 $planHash --state <run>/nas-state.json --receipt <run>/nas-receipt.json
$stateHash = (Get-FileHash <run>/nas-state.json -Algorithm SHA256).Hash.ToLower()
$receiptHash = (Get-FileHash <run>/nas-receipt.json -Algorithm SHA256).Hash.ToLower()
python tools/device-simulation/device_simulation.py cleanup-nas --plan <run>/nas-plan.json --plan-sha256 $planHash --state <run>/nas-state.json --state-sha256 $stateHash --receipt <run>/nas-receipt.json --receipt-sha256 $receiptHash
```

SMB and NFS are **not tested** because no reviewed, cached, isolated user-space
server exists in this lane. `network-config-refused` is `source-contract`, not
a runtime protocol pass. `local-config-network-data` is `not-covered`. The
verifier rejects promotion of either gap.

## Verification and remaining inputs

```powershell
python -m unittest -v tools/device-simulation/test_device_simulation.py
```

Runtime still requires the final clean source commit/tree/manifest, producer
implementation of the agreed ARM receipt, fresh ARM64 and ARMv7 test images,
final candidate and baseline image IDs/revisions, and a reviewed cached helper
receipt. The soak requires a full uninterrupted real day. Nothing here claims
native Raspberry Pi, NAS appliance, thermal, power, vendor-package, SMB/NFS,
final-source ARM, or completed runtime acceptance.
