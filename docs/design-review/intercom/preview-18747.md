# Disposable Intercom preview backend

This harness prepares an independent backend for the redesign source tree. It does not build HolyArrs and does not certify the reused executable as matching the current source.

## Bound executable

The existing review backend was identified read-only from PID `62268`:

- Path: `C:\HolyForge\coderluii\cl\holyarrs\worktrees\app-redesign-review\captures\gumloop-verification-v7\holyarrs.exe`
- Size: `30,926,848` bytes
- SHA-256: `D1FA075167517E84FD834EC112C211EE84F27AA8BE8191E07DB9AE62E316C914`
- Observed listener: `127.0.0.1:18741`

The preview launcher reuses this executable and the adjacent `holyarrs-fakesubs.exe`. This is a speed optimization for UI review only. A later combined build and verification must use an executable built from the final source.

## Isolation contract

- Backend: `127.0.0.1:18747`.
- Frontend: `0.0.0.0:18746`, using `vite.intercom-preview.config.ts`.
- Disposable runtime: a fresh `%TEMP%\holyarrs-intercom-preview-18747-*` directory by default. The launcher never deletes or reuses an earlier runtime. An explicit `HOLYARRS_PREVIEW_RUNTIME` must name a path that does not exist.
- Fake services: loopback ports `28760` through `28772`. Ports `28773` and `28774` remain reserved and are checked too.
- The launcher checks every requested listener before starting anything and verifies the backend executable against the pinned SHA-256 above.
- It never reads the installed app config, database, owner password, keys, media, or another preview's config.
- The fresh database begins at the normal welcome flow. No owner credential is predefined or copied.
- Fake providers use only the repository's existing test fixtures. Sources, downloaders, accounts and lifecycle state can be changed later only inside this disposable database.

The session cookie is host-scoped rather than port-scoped. A browser visiting two HolyArrs ports on `10.10.25.25` would otherwise send the same `holyarrs_session` cookie to both. The preview Vite proxy therefore:

1. stores backend session and OIDC cookies in the browser under preview-specific names;
2. strips any normal HolyArrs session/OIDC cookie from requests bound for `18747`;
3. maps only the preview-specific cookies back to the names expected by the backend;
4. rewrites `Set-Cookie` responses back to the preview-specific names.

This prevents the `18746` UI from replacing or presenting the session used by another HolyArrs port on the same host. Lifecycle checks should still use a fresh isolated browser context.

## Ports

| Purpose | Port |
| --- | ---: |
| Backend | 18747 |
| MusicBrainz fake | 28760 |
| TVMaze fake | 28761 |
| Plex fake | 28762 |
| Jellyfin fake | 28763 |
| TMDB fake | 28764 |
| Trakt fake | 28765 |
| Discord fake | 28766 |
| Source fake | 28767 |
| qBittorrent fake | 28768 |
| SABnzbd fake | 28769 |
| Subtitle fake | 28770 |
| Old Radarr fake | 28771 |
| Old Seerr fake | 28772 |
| Reserved | 28773-28774 |

At preparation time, `18747` and `28760-28774` had no listeners. Existing listeners on `18741`, `18744`, `18745`, and `18746` were left untouched.

## Proposed launch

From the isolated worktree root, start the backend in one PowerShell terminal:

```powershell
$env:HOLYARRS_PREVIEW_BIN = 'C:\HolyForge\coderluii\cl\holyarrs\worktrees\app-redesign-review\captures\gumloop-verification-v7\holyarrs.exe'
node docs/design-review/intercom/backend-preview-18747.mjs
```

After the launcher reports readiness, use a second terminal for the frontend. Port `18746` is currently owned by the existing preview, so stop only that known preview process before this step; never stop an unidentified listener.

```powershell
Set-Location web/app
npm run dev -- --config vite.intercom-preview.config.ts
```

Review URLs:

- Home network: `http://10.10.25.25:18746`
- Tailscale: `http://100.87.70.86:18746`

Stopping the backend launcher stops only its child backend, subtitle fake and fake HTTP servers. Its unique disposable runtime remains available for inspection; later launches use a different directory.
