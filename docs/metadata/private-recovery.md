# Private metadata recovery

This operation creates a bounded encrypted recovery package while the private metadata API and fixture are stopped, pulls that ciphertext to the restricted Windows secrets tree, authenticates and verifies it, and prepares database copies with every installation token revoked. It never starts a service or replaces a live database.

## Fixed identities and paths

The Oracle root is `/home/home/holyarrs-metadata-server`. The runtime is `2026-10-15-build-7`; the operator revision is `c9ddc1deabd41c33e5cb58729a4255bc436ddaf7`. The API and fixture image IDs are fixed in `private-recovery.py`. The recipient at `settings/private/backup-recipient.pem` must contain a valid certificate and no private key.

The fixed Windows project root is `C:/Users/Luis/holyarrs-metadata-backup`. The only command destination is its `backups` child. The private key is `keys/backup-key.pem`, the recipient certificate is `keys/recipient.pem`, task state is under `task`, and frozen tools are installed under `tools/versions/<commit>`. The command rejects `O:`, parent traversal, and symlink or reparse ancestors. Before task registration, the deployment gate must apply and verify protected Windows ACLs on the complete project path and individual sensitive files, allowing only the signed-in user, SYSTEM, and Administrators. POSIX mode bits do not enforce a Windows ACL.

This profile-root layout supersedes the original HolyForge secrets-tree proposal. The old empty `C:/HolyForge/secrets/holyarrs-metadata-backups` directory is preserved and unused. Installation copies the already provisioned key and recipient certificate into the new `keys` directory, verifies byte equality without printing their contents, and leaves the originals unchanged.

## Root-owned recovery manifest

Install `settings/recovery-manifest.json` as a root-owned regular file. Its complete schema is:

```json
{
  "schema_version": 1,
  "identities": {
    "schema_version": 1,
    "runtime_build": "2026-10-15-build-7",
    "operator_revision": "c9ddc1deabd41c33e5cb58729a4255bc436ddaf7",
    "api_image": "sha256:3f4e2fbe701e70ec502a6ab2958629c47d63b3cc82cbe510d144d31795e82af5",
    "fixture_image": "sha256:1cbba147d02d9bff2ccbfcf0c63155e8efca0eae57361d91f20f02aced150def"
  },
  "existing_services": [
    {
      "name": "holycore",
      "id": "<exact-container-id>",
      "started": "<exact-StartedAt>",
      "restarts": 0,
      "running": true,
      "health": "healthy"
    }
  ],
  "files": {
    "config/private.json": {"path": "settings/versions/2026-10-15-build-7/private.json", "bytes": 0, "sha256": "<64 lowercase hex>"},
    "source/runtime.tar.gz": {"path": "application/source-packages/2026-10-15-build-7/source.tar.gz", "bytes": 0, "sha256": "<64 lowercase hex>"},
    "source/recovery-tools.tar.gz": {"path": "<current frozen recovery-tools archive relative to root>", "bytes": 0, "sha256": "<64 lowercase hex>"},
    "operator/private-ops.py": {"path": "application/versions/2026-10-15-build-8/source/deploy/metadata/private-ops.py", "bytes": 0, "sha256": "<64 lowercase hex>"},
    "operator/current-build.json": {"path": "settings/current-build.json", "bytes": 0, "sha256": "<64 lowercase hex>"}
  }
}
```

`existing_services` must contain exactly these names in order: `holycore`, `sacris`, `cloudflared`, `watchtower`, `portainer_agent`, and `buildx_buildkit_cw-pretag150-arm-node`. Copy the exact `id`, `started`, `restarts`, `running`, and `health` values from the verified build-7 baseline receipt. Every `bytes` value is the exact current file size, including zero only when the file is actually empty. Every hash is calculated from the named regular file. The five package member names are fixed; source paths can change without changing the encrypted package layout. The recovery-tools archive must contain the reviewed recovery source, including `private-recovery.py`, `offhost-backup.py`, and `scheduled-backup.py`.

The hash-bound `settings/current-build.json` must also equal exactly `{"runtime_folder":"2026-10-15-build-7","operator_folder":"2026-10-15-build-8","source_commit":"c9ddc1deabd41c33e5cb58729a4255bc436ddaf7","public_deployment":false}`. Recovery tools are frozen separately and do not change the selected runtime or operator.

## Oracle command

Install the reviewed Oracle operator at `application/tools/private-recovery.py`, then run:

```text
sudo python3 -B /home/home/holyarrs-metadata-server/application/tools/private-recovery.py backup
sudo python3 -B /home/home/holyarrs-metadata-server/application/tools/private-recovery.py status
```

`backup` refuses a non-root caller, root-path aliases, overlapping runs, source or image drift, changes to the six existing services, running or unowned private containers, published ports, an active private-test marker, a colliding maintenance container, unsafe manifest inputs, and less than 25 GiB free after its conservative peak estimate. It creates a root-only failed staging directory and refuses another attempt until that one owned directory is diagnosed and safely cleared. The native snapshot destination is initially nonexistent at `/backup/snapshot`; only its parent is owned by UID 10001.

The existing Oracle root and `backups` directory already provide traversal to their `ghost` owner; the operator never changes their owner, mode or ACLs. It creates `backups/scheduled` as `ghost:ghost` mode `0700` when absent, or refuses reuse unless it is a real directory with that exact owner and mode. Each published run directory is also `ghost:ghost` mode `0700`; its encrypted CMS file and sanitized receipt are mode `0600`. No plaintext enters `backups/scheduled`. Plaintext stays under the root-only staging tree and is deleted by an exact owned-file cleanup after ciphertext publication. The receipt says `offhost_copy_verified: false`; Oracle creation does not prove a Windows copy.

Oracle recovery points are not pruned by this first operator. They accumulate until the 25 GiB post-staging reserve stops new work. Windows retention remains bounded, but remote retention needs an independent review before automation can safely delete Oracle history.

## Windows pull and verification

Run from the signed-in task context:

```text
python deploy/metadata/offhost-backup.py --destination C:/Users/Luis/holyarrs-metadata-backup/backups
python deploy/metadata/offhost-backup.py --destination C:/Users/Luis/holyarrs-metadata-backup/backups --status
```

The command has no remote-command or remote-path option. It always connects to `coderluii-vm` with batch mode, strict host-key checking and host-key updates disabled, then runs the fixed absolute sudo command. It downloads only the exact receipt-bound ciphertext path. The ciphertext cap is 512 MiB for the current synthetic dataset, not a real-data compression promise.

The local Docker decryptor is fixed to `sha256:007a316059246276cb214df93950be77837545dc78007a73e0291a4963d5d3bf` on `amd64`, with pull disabled, no network, one CPU, 512 MiB, a read-only root, all capabilities dropped, no-new-privileges and 64 PIDs. Authenticated decryption publishes nothing until the archive allowlist, sizes, hashes, 3 GiB expansion cap, exact SQLite schemas and integrity, synthetic configuration, and both Docker image config hashes and `arm64` architecture pass.

Only verified run directories participate in retention. The operation keeps at most seven verified points within a 2 GiB ciphertext-and-receipt budget and always preserves the latest and prior points. Unknown, linked, unverified or mismatched content stops retention before any deletion. A failed run stays visible and blocks a later download rather than accumulating another incomplete directory. `latest.json` survives failures; `failure.json` records the last sanitized failure separately.

## Preparing a restore

After verification, decrypt and inspect a recovery point through the same bounded verification path. Then prepare new database copies only:

```text
python deploy/metadata/private-recovery.py prepare-restore --source <verified-data-directory> --destination <new-empty-path>
```

The destination must not exist. Both databases must have schema version 1, exact tables, columns and indexes, no extra user objects, and a passing integrity check. The command copies only `cache.sqlite` and `access.sqlite`, then revokes every recovered token in one transaction. Cache rows stay unchanged. Re-enroll installations before any recovered service is allowed to accept traffic.

## Scheduling and limits

Install `scheduled-backup.py` beside `offhost-backup.py` in the reviewed, ACL-protected stable tools directory. Register two task actions against that same absolute Python file and the fixed restricted destination:

```text
<python.exe> -I -S -B C:/Users/Luis/holyarrs-metadata-backup/tools/versions/<commit>/scheduled-backup.py --mode daily --destination C:/Users/Luis/holyarrs-metadata-backup/backups
<python.exe> -I -S -B C:/Users/Luis/holyarrs-metadata-backup/tools/versions/<commit>/scheduled-backup.py --mode logon --destination C:/Users/Luis/holyarrs-metadata-backup/backups
```

Source completion does not authorize unattended registration. Registration remains blocked until the actual host passes the full ancestor, interpreter, directory, and individual-file owner/ACL gate. Any unexpected identity with replace or write access, including a capability SID on an ancestor, must refuse registration. If that gate cannot be met without changing shared profile ancestors, retain manual execution against explicitly protected existing paths and record the scheduled acceptance as blocked.

Before launching a child, the wrapper atomically writes `task/in-progress.json`. A preexisting valid, malformed, linked or reparse guard blocks both daily and logon without automatic expiry. Normal success removes the exact verified guard. Child failure, timeout, failed `taskkill /T`, ambiguous process-tree state, crypto cleanup failure, or abrupt wrapper termination retains it even after the OS lock is released, so another scheduled run cannot overlap possible descendants.

Clearing the guard is an explicit manual recovery action. First verify that the recorded attempt has no surviving task-owned Python, SSH or SCP process and no crypto container matching its schedule label, the fixed owner, and the fixed image. Then remove only the exact regular `in-progress.json` from the protected task directory. Never automatically remove a malformed, linked or reparse guard. Manual recovery through the reviewed offhost tool must likewise finish or be inspected before another attempt.

The daily task triggers at local 02:15, runs every time, uses `StartWhenAvailable`, and does not apply a rolling 24-hour skip. The logon task is only a catch-up: it runs when no valid verified point covers the latest local calendar 02:15 slot. The comparison converts the UTC verification time into local wall time, including a daylight-saving transition where 02:15 does not exist. A failed, malformed, unverified or older point never suppresses catch-up.

Both modes share the same nonblocking OS file lock and durable guard. The wrapper launches only its sibling `offhost-backup.py` through the current Python executable with `-I -S -B`, captures its output without copying it into status or console diagnostics, and enforces a 1,800-second timeout. The task actions do not accept a remote command or tool path.

Create and ACL-gate the dedicated state sibling `C:/Users/Luis/holyarrs-metadata-backup/task` during installation. It must remain separate from the backup destination and contains the cross-mode lock, atomically replaced `schedule-status.json`, and durable `in-progress.json` guard. The guard schema is:

```json
{
  "schema_version": 1,
  "schedule_id": "<32 lowercase hex>",
  "started_at": "<UTC timestamp>",
  "mode": "<daily or logon>"
}
```

The status schema is:

```json
{
  "schema_version": 1,
  "last_attempt_at": "<UTC timestamp or null>",
  "last_mode": "<daily, logon, or null>",
  "last_result": "<never, success, failed, or skipped>",
  "last_success_at": "<UTC timestamp or null>",
  "last_success_mode": "<daily, logon, or null>",
  "error": "<scheduled backup failed or null>"
}
```

Failure replaces only the attempt/result/error fields, preserves the prior successful timestamp and mode, and retains the run guard. The state and backup paths must remain under the fixed profile project root with no symlink or reparse ancestor. On timeout, the wrapper attempts to terminate the exact local process tree and remove only crypto containers carrying its unique schedule label, the fixed owner label, and the fixed image. An already accepted remote Oracle operation may continue after the local SSH tree is terminated; Oracle's own lock and bounded operation prevent another Oracle backup from overlapping it, and the durable local guard blocks all scheduled retries until manual recovery. Windows must be awake with SSH, Docker and the signed-in task context available. This is not independent always-on remote storage and cannot guarantee a 24-hour recovery point while the PC is offline. Configure the task account without saving its password.

The status commands return nonzero when the latest verified copy is older than 24 hours. Scheduler failures remain visible separately from the last success. No command logs subprocess output, database content, token hashes, real configuration, certificate keys or search content.
