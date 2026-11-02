# Standalone metadata private validation

This service is independent of the HolyArrs application. It does not implement the Sonarr, Radarr, Lidarr, Skyhook or MusicBrainz server protocols. Its own versioned API supplies bounded search and detail replies. Current validation uses synthetic sources only. Public-provider permission, credentials and real response compatibility remain separate checks.

## Oracle layout

Everything for this standalone service lives below `/home/home/holyarrs-metadata-server`. Release IDs remain unchanged in commands and Docker tags. Human-readable folders map `20261015a` through `20261015h` to `2026-10-15-build-1` through `2026-10-15-build-8`.

| Path | Purpose |
|---|---|
| `application/versions/<build>/source` | Frozen, sanitized source; no application checkout, existing secrets or Git directory |
| `application/builds/<build>` | Capped native compilation, binaries and exported root filesystems |
| `application/build-cache/compiler` and `dependencies` | Dedicated build caches, separate from shared BuildKit |
| `settings/versions/<build>` | Private validation configuration |
| `settings/private/<build>` | Newly generated validation credential and future provider credentials; never print or commit |
| `data/test-runs/<build>` | SQLite cache and separate hashed-token access database |
| `backups/database-copies/<build>` | Consistent paired snapshots |
| `backups/encrypted` | Private encrypted release and restore bundles |
| `results/history/<build>` | Native tests, exact image identities and measurements |
| `results/latest` | Plain-English current results and pointers to the latest tested build |

The private network is internal-only. Neither container publishes a host port. There is no new Cloudflare connector, DNS record, public ingress, boot startup or automatic image updater. Both runtime containers have no Linux capabilities, a read-only root filesystem, a non-root user, bounded temporary storage and bounded logs. API limits are one CPU and 1 GiB; fixture limits are 0.25 CPU and 256 MiB. The native compiler uses two CPUs and 4 GiB. Heavy compilation, storage population and load tests run sequentially.

The compiler image is pinned by registry digest and checked for ARM64. Compilation and root-filesystem assembly happen inside the capped service-owned compiler container. Docker imports those root filesystems as scratch images; no shared build service is used. The Dockerfiles describe the equivalent scratch contents. Run containers using the recorded image IDs, with pull disabled. A frozen build directory cannot be reused for another compilation.

## Operator commands

Run `deploy/metadata/private-ops.py` on Oracle with root privileges, a matching frozen release and its image-identity evidence. `setup` creates only that build's settings, private credential, test data and database-copy folders, plus the private network and fixture; `start` starts the API. `paced`, `fast` and `quota` restart only the owned API using explicit test configurations. `snapshot` stops only the owned API and creates `backups/database-copies/<build>/initial` once per release; a second invocation refuses to overwrite it. `stop` stops the two owned validation containers after checking their ownership labels. Emergency stop works without image evidence, disk reserve or healthy unrelated services. Read-only `status` also works without those gates. Creation, restart and snapshot actions compare the six pre-existing containers against their baseline, verify image identity and refuse writes below 25 GiB available space.

The default provider intervals are TMDB 200 ms, TVmaze 1,000 ms and MusicBrainz 1,100 ms. Accelerated mode is explicitly synthetic and uses 5 ms intervals. High request allowances in load tests measure the service; they are not the proposed public installation allowance. `quota` uses 60 requests/minute, burst 10 and four simultaneous requests per credential.

Global admission is 100 simultaneous requests, with 200 pending provider jobs and two connections per provider. One provider request is active at a time under its pacing gate. Lookup deadline is 15 seconds; provider work has a 10-second timeout and at most two attempts within that budget. Decoded provider bodies and saved result data are limited to 2 MiB; request headers to 16 KiB and provider response headers to 32 KiB. A thousand installations does not define traffic: distinct uncached work is constrained to at most roughly five TMDB starts/second, one TVmaze start/second and 0.9 MusicBrainz starts/second under these conservative settings. Real network delays can lower those rates. Shared answers and repeated in-flight lookups reduce provider work.

The API supports `/v1/search`, movie/show details, TMDB seasons including season zero, and MusicBrainz artist/release-group/release/recording details. Responses include schema version, source identity, timestamps, stale status and complete validated data. Provider-native pages are preserved. Only MusicBrainz search honors a requested result limit; other providers keep their native page size. Unused detail parameters are rejected rather than creating duplicate saved answers.

TTL is 24 hours by default, with provider and provider-operation overrides. Empty complete searches expire after at most five minutes. Serving stale data is disabled in private deployment; enabling it later requires an explicit retention policy. A cache write failure can return a fresh valid reply with `X-Metadata-Cache: bypass`; incomplete or invalid source data cannot replace a valid saved answer. Authentication remains mandatory on warm hits and metrics. Health/readiness expose only a boolean. Readiness uses bounded connection pings, while full integrity checks run at startup and on operator request.

## Credentials, snapshots and rollback

Operator commands operate on the stopped service's data directory, protected by an exclusive owner lock. `token-issue --data-dir ... --token-file ...` generates a private file without printing the token. If a crash leaves that file before registration, `token-register` safely registers that exact file. Registration cannot undo a revoked token; issue a new credential instead. `token-revoke`, `check` and `backup --destination <new-directory>` also operate offline. Pass `--config` with the same storage limits when operating a larger deployment.

The offline backup command allows five minutes for its paired snapshot and integrity checks; other administration commands keep a one-minute budget. The earlier one-minute backup budget failed against the approximately 952 MB private test database and was replaced before the final release. Backup CPU and elapsed time are recorded in the measured results.

Backups contain both databases. Cache eviction never deletes access/revocation records. Restoring an old access database can resurrect credentials revoked after its snapshot; production recovery must replay a separately protected revocation record or rotate all installation credentials before reopening traffic. This private test proves revocation present in the snapshot survives restoration; it does not claim protection against restoring an older pre-revocation snapshot.

`tests/recovery.py` works only on new, disjoint copied data directories and launches isolated containers without network access or host ports. It tests paired integrity, hashed credentials, durable revocation, backup restoration, unknown future schema refusal, damaged-database refusal and rollback to the untouched pair. Initial schema version is one; there is no production schema-upgrade history yet. Future migrations need their own upgrade/downgrade rehearsal against a real prior release.

There is no live restore action in `private-ops.py`. Its snapshot and recovery commands create evidence and isolated copies. Production restoration remains a separate implementation task: stop the owned API, preserve its current database directory, stage both databases from one snapshot in a new directory on the same local filesystem, set owner 10001 and restrictive modes, run offline `check` with the matching image/configuration, then atomically switch the directory and start. Verify readiness and authorization before exposing traffic; preserve an atomic rollback to the original pair. Never mix database or WAL files from different generations. The private off-host rehearsal is recorded separately and does not turn this into a supported production restore command.

The validation backup uses OpenSSL CMS authenticated encryption with a newly generated recipient certificate. The decryption key stays off Oracle. The encrypted bundle includes the exact exported ARM64 image, source, configuration and paired snapshot, but no bearer-token file or existing-service credentials. This is a manually verified off-host copy; scheduled retention, a remote object-store destination and disaster recovery onto a replacement VM are not configured by this private build.

## Storage and upgrade signals

Keep live SQLite files on a local Oracle block filesystem. A shared drive or network filesystem is suitable for exported backup objects, not the live WAL databases. A full MusicBrainz mirror, artwork hosting and media files are outside this cache's storage model. Free storage eligibility and permitted use need provider/account checks before adoption.

The private test uses a 2 GiB logical cache allowance and 100,000-entry ceiling. Physical SQLite pages have a separate ceiling: three times the logical allowance, plus about 1 KiB per allowed entry and 1 MiB schema reserve. WAL writes stop at an 8 MiB observed threshold; one transaction may temporarily add to it. These limits are not a hard host filesystem quota. Keep an independent disk alert and do not use all available root space. Testing refuses new operations below 25 GiB free; cache writes reserve 20 GiB.

Reassess the host when cache p95 exceeds 200 ms at normal demand, API memory stays above 75% of its cap, provider queue waits approach the 15-second lookup deadline, or free storage approaches 25 GiB. Fix cache-hit rate, request fan-out and refresh timing before buying CPU. Provider throttling requires better reuse, permitted provider capacity or a mirror; a faster VM alone does not increase an upstream allowance. One VM remains one failure point, even behind a future Cloudflare Tunnel.

Before public use, finish live-provider compatibility/permissions, OCI account/network/billing checks, an installation-enrollment policy, independent monitoring, scheduled encrypted off-host backup and replacement-VM recovery. Public Cloudflare routing and its outside latency remain untested. The app's existing direct-provider behavior stays unchanged until separate integration work.

Live checks must cover provider terms and credentials, payload drift, TVmaze specials and episode completeness, TMDB artwork references and appended credits/translations/release dates/content ratings, large MusicBrainz catalog pagination, and refresh/update behavior. Scheduled refresh, artwork binary proxying, TVDB data access and scene-numbering mappings are not implemented. The provider coverage report records field-level gaps against the clean-room Sonarr and Radarr information requirements.

The current private test has no user-facing route and uses only synthetic HTTP providers. Future live providers will see Oracle's outgoing IP. A future Cloudflare Tunnel can keep the metadata origin address out of client-facing DNS and close that direct ingress path; it does not hide Oracle's outgoing address from those providers or erase addresses exposed by other services.
