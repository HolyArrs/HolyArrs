param(
  [Parameter(Mandatory = $true, Position = 0)][string]$Command,
  [string]$Who = (Split-Path -Leaf (Get-Location)),
  [ValidateSet('agent', 'owner')][string]$As = 'agent',
  [int]$StaleMinutes = 3,
  [int]$WaitMinutes = 120,
  # GhostPC rule (coord 2026-11-04 05:20): at most 2 e2e runs at once. After any
  # stall or reset it is one heavy job at a time again: run with -Exclusive.
  [int]$Slots = 2,
  [switch]$Exclusive,
  [string]$StateDir = 'C:\HolyForge\coderluii\cl\holyarrs\.omc\state'
)
# Runs one e2e command while holding one of the shared e2e lock slots, so
# sessions in any worktree take turns on the e2e harness without waiting for
# unrelated runs. Example (from your worktree's web\app folder):
#   & C:\HolyForge\coderluii\cl\holyarrs\repo\scripts\e2e-run.ps1 'npx playwright test --project=library --reporter=line' -Who library
#
# - Slots: slot 0 uses the harness's usual ports (18732-18749, 18797); slots 1-3
#   each get their own 16 ports from 20800 up, handed to the command in the
#   HOLYARRS_E2E_*_PORT variables the harness reads. A run with its own
#   --config (live-push, phone-audit, status, csp-check, ...) only takes slot 0,
#   since those configs use fixed ports. Two runs never share a worktree folder,
#   since each run's servers keep their files in e2e\.tmp there.
# - A slot is one folder; making a folder either works or fails at once. The
#   holder touches its holder file every 30 s; a slot silent for StaleMinutes
#   with its ports free (its holder died) is taken over, by renaming it away
#   first so two waiters never both take it. Process ids are not checked: a
#   session cannot always see another session's processes.
# - -As picks which 18729 showcase login a check reads (never printed): agent,
#   the admin agent-check (default), or owner, for owner-only pages. The
#   command gets that file's path in HOLYARRS_SHOWCASE_LOGIN.
# - The 18729 build and that login's role are written down at the start and the
#   end; if they changed during the run, checks against 18729 in it are invalid.
# - The command runs as its own process with its output in a log file, so piping
#   or redirecting this script never breaks a run; its exit code passes through.
# - A run waiting for a slot restarts itself when this script is updated.
# - -Exclusive holds every slot (slot 0's ports) so nothing else runs beside
#   it: for timing-sensitive checks only (explore-memory, frame times, AC6, load
#   numbers). Pass/fail chains run without it, so a second run can go beside.
# - A second run beside another needs C: with 50 GB free and CPU under 60%;
#   Docker builds and pulls, installers, a full go test ./... and night soaks
#   still run alone (coord 2026-11-04 05:20).
$ErrorActionPreference = 'Stop'
$logins = @{ agent = 'C:\HolyForge\coderluii\cl\holyarrs\secrets\showcase-18729-agent.txt'; owner = 'C:\HolyForge\coderluii\cl\holyarrs\secrets\showcase-18729-home.txt' }
$env:HOLYARRS_SHOWCASE_LOGIN = $logins[$As]
$state = $StateDir
New-Item -ItemType Directory -Force $state | Out-Null
$here = (Get-Location).Path
$selfHash = (Get-FileHash -LiteralPath $PSCommandPath).Hash

# Each slot's ports, in the order the harness numbers them from 18734.
$portNames = 'BASE_APP', 'BASE_PROXY', 'OLD_RADARR', 'OLD_SEERR', 'SUBS', 'MB', 'TVMAZE', '', 'PLEX', 'JELLYFIN', 'TMDB', 'TRAKT', 'DISCORD', 'INDEXER', 'QBIT', 'SAB'
function Slot-Ports([int]$k) {
  if ($k -eq 0) { return @(18732..18749) + 18797 }
  $base = 20800 + 16 * ($k - 1)
  return @($base..($base + 15))
}
function Slot-Lock([int]$k) { if ($k -eq 0) { Join-Path $state 'e2e.lock' } else { Join-Path $state "e2e.lock.$k" } }
function Read-Holder([string]$lock) {
  try {
    $l = Get-Content (Join-Path $lock 'holder.txt') -ErrorAction Stop
    return @{ Pid = [int]$l[0]; Who = $l[1]; At = [datetime]::Parse($l[2]); Dir = $(if ($l.Count -gt 4) { $l[4] } else { '' }) }
  } catch { return $null }
}
function Ports-Taken([int[]]$ports) {
  # Tried by opening them: a listing of connections can miss other sessions' ports.
  @($ports | Where-Object { $probe = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $_); try { $probe.Start(); $false } catch { $true } finally { $probe.Stop() } })
}
# The 18729 build (backend and web process ids, the site's build time) and the
# chosen login's role there; '-' where 18729 does not answer.
function Preview-Stamp {
  $rt = Join-Path $env:LOCALAPPDATA 'Temp\holyarrs-showcase-join'
  $ids = foreach ($f in 'backend.pid', 'web.pid') { try { (Get-Content (Join-Path $rt $f) -ErrorAction Stop | Select-Object -First 1).Trim() } catch { '-' } }
  $site = 'C:\HolyForge\coderluii\cl\holyarrs\repo\web\app\.svelte-kit\showcase\site\index.html'
  $built = try { (Get-Item $site -ErrorAction Stop).LastWriteTime.ToString('HH:mm:ss') } catch { '-' }
  $role = '-'
  try {
    $t = Get-Content $env:HOLYARRS_SHOWCASE_LOGIN -Raw -ErrorAction Stop
    $n = [regex]::Match($t, '(?im)^\s*(?:name|HOLYARRS_SHOWCASE_USER)\s*[:=]\s*(.+?)\s*$').Groups[1].Value
    $pw = [regex]::Match($t, '(?im)^\s*(?:password|HOLYARRS_SHOWCASE_PASSWORD)\s*[:=]\s*(.+?)\s*$').Groups[1].Value
    $body = @{ name = $n; password = $pw } | ConvertTo-Json -Compress
    $null = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18729/api/v1/auth/login' -Body $body -ContentType 'application/json' -SessionVariable s -TimeoutSec 10
    $me = Invoke-RestMethod -Uri 'http://127.0.0.1:18729/api/v1/auth/me' -WebSession $s -TimeoutSec 10
    $role = if ($me.role) { $me.role } elseif ($me.user.role) { $me.user.role } else { '?' }
  } catch {}
  "backend $($ids[0]), web $($ids[1]), site built $built, $As role $role"
}

# A run with its own config uses fixed ports, so only slot 0 fits it.
$ownConfig = $Command -match '(^|\s)(-c|--config)(\s|=)'
$choices = if ($ownConfig -or $Exclusive) { @(0) } else { @(0..($Slots - 1)) }
$deadline = (Get-Date).AddMinutes($WaitMinutes)
$said = ''
$slot = -1
while ($slot -lt 0) {
  if ((Get-FileHash -LiteralPath $PSCommandPath).Hash -ne $selfHash) {
    'e2e lock: e2e-run.ps1 was updated while waiting; starting again with the new one'
    & $PSCommandPath @PSBoundParameters
    exit $LASTEXITCODE
  }
  # Another run in this same folder shares e2e\.tmp: wait for it. A holder from
  # an older script names no folder, so it counts as this one.
  $holders = @(0..($Slots - 1) | ForEach-Object { $h = Read-Holder (Slot-Lock $_); if ($h) { $h } })
  $sameDir = @($holders | Where-Object { -not $_.Dir -or $_.Dir -eq $here })
  if (-not $sameDir) {
    foreach ($k in $choices) {
      $lock = Slot-Lock $k
      try {
        New-Item -ItemType Directory $lock -ErrorAction Stop | Out-Null
        Set-Content -Path (Join-Path $lock 'holder.txt') -Encoding ascii -Value @($PID, $Who, (Get-Date).ToString('o'), $Command, $here)
        $slot = $k
        break
      } catch {}
    }
    if ($slot -ge 0) { break }
  }
  foreach ($k in $choices) {
    $lock = Slot-Lock $k
    $info = Join-Path $lock 'holder.txt'
    # Last sign of life: the holder file's last touch (folder creation times are
    # not used: Windows can hand a new folder an old one's creation time). A
    # holder file that cannot be read is never stale, nor a slot whose ports are taken.
    $seen = $null
    try { $seen = (Get-Item $info -ErrorAction Stop).LastWriteTime } catch {}
    if (-not $seen -or ((Get-Date) - $seen).TotalMinutes -le $StaleMinutes) { continue }
    if (Ports-Taken (Slot-Ports $k)) { continue }
    $h = Read-Holder $lock
    "e2e lock: taking over stale slot $k ($(if ($h) { "$($h.Who), pid $($h.Pid), since $($h.At)" } else { 'no holder file' }))"
    $tomb = "$lock.stale-$PID"
    try { Rename-Item $lock $tomb -ErrorAction Stop } catch { continue }
    $was = $null
    try { $was = (Get-Item (Join-Path $tomb 'holder.txt') -ErrorAction Stop).LastWriteTime } catch {}
    if ($was -ne $seen) { try { Rename-Item $tomb $lock -ErrorAction Stop } catch {}; continue }
    Remove-Item -Recurse -Force $tomb -ErrorAction SilentlyContinue
  }
  if ((Get-Date) -gt $deadline) { throw "e2e lock: no free slot after $WaitMinutes minutes" }
  $now = ($holders | ForEach-Object { "$($_.Who) since $($_.At.ToString('HH:mm'))" }) -join '; '
  if ($now -ne $said) {
    if ($sameDir) { "e2e lock: waiting for the run in this folder ($(($sameDir | ForEach-Object { $_.Who }) -join ', ')) to end" }
    else { "e2e lock: all slots busy ($now)" }
    $said = $now
  }
  Start-Sleep -Seconds 5
}

$lock = Slot-Lock $slot
$info = Join-Path $lock 'holder.txt'
$ports = Slot-Ports $slot
# Every slot this run holds; -Exclusive takes the others too, one by one as
# their runs end, touching what it already holds so none looks stale.
$held = @($lock)
if ($Exclusive) {
  "e2e lock: -Exclusive: waiting for every other slot to be free"
  foreach ($k in 1..($Slots - 1)) {
    $l = Slot-Lock $k
    while ($true) {
      try {
        New-Item -ItemType Directory $l -ErrorAction Stop | Out-Null
        Set-Content -Path (Join-Path $l 'holder.txt') -Encoding ascii -Value @($PID, "$Who (exclusive)", (Get-Date).ToString('o'), $Command, $here)
        $held += $l
        break
      } catch {}
      if ((Get-Date) -gt $deadline) {
        foreach ($x in $held) { Remove-Item -Recurse -Force $x -ErrorAction SilentlyContinue }
        throw "e2e lock: could not hold every slot within $WaitMinutes minutes"
      }
      foreach ($x in $held) { try { (Get-Item (Join-Path $x 'holder.txt')).LastWriteTime = Get-Date } catch {} }
      Start-Sleep -Seconds 5
    }
  }
  "e2e lock: holding all $Slots slots"
}
if ($slot -gt 0) {
  for ($i = 0; $i -lt 16; $i++) { if ($portNames[$i]) { Set-Item "env:HOLYARRS_E2E_$($portNames[$i])_PORT" $ports[$i] } }
  $env:HOLYARRS_E2E_PORT = $ports[7]
}
"e2e lock: slot $slot held by $Who (ports $($ports[0])-$($ports[-1]))"
@(
  'e2e notice:',
  '  - Rebuild web\dist (npm run build) first if the app changed: the harness serves the built app.',
  '  - Your servers use this slot''s ports (HOLYARRS_E2E_*_PORT); never start the harness outside e2e-run.ps1.',
  '  - Checks against 18729 count only if the stamps below match at start and end.'
) | ForEach-Object { $_ }
# The last holder's servers can take a few seconds to let go of their ports
# after it releases the slot; start only once this slot's ports are free.
for ($i = 0; $i -lt 60; $i++) {
  $taken = Ports-Taken $ports
  if (-not $taken) { break }
  if ($i -eq 0) { "e2e lock: waiting for ports $($taken -join ', ') to be free" }
  try { (Get-Item $info).LastWriteTime = Get-Date } catch {}
  Start-Sleep -Seconds 2
}
$stampStart = Preview-Stamp
"e2e stamp at start: $stampStart"
# Two traps that make a run test the wrong thing; said up front and in the log.
$warn = @()
$dist = Join-Path $here '..\dist\index.html'
$src = Join-Path $here 'src'
if ((Test-Path $dist) -and (Test-Path $src)) {
  $newest = Get-ChildItem $src -Recurse -File | Sort-Object LastWriteTime | Select-Object -Last 1
  if ($newest.LastWriteTime -gt (Get-Item $dist).LastWriteTime) { $warn += "WARNING: web\dist is older than $($newest.Name) ($($newest.LastWriteTime.ToString('HH:mm'))): the harness serves the built app, so this run tests old pages. Run npm run build first." }
}
$link = Get-Item (Join-Path $here 'node_modules') -Force -ErrorAction SilentlyContinue
if ($link -and $link.LinkType -eq 'Junction') { $warn += "WARNING: node_modules here is a junction to $($link.Target); removing this worktree can empty that shared folder for everyone. Give it its own copy: scripts\worktree-own-modules.ps1 -Only $(Split-Path -Leaf (Split-Path -Parent (Split-Path -Parent $here)))" }
if ($Command -match '--no-deps') { $warn += 'WARNING: --no-deps skips the setup project, so the fresh e2e server has no owner and no folders yet: tests that sign in fail. Leave it out; a project now depends only on setup plus what it needs.' }
$warn | ForEach-Object { $_ }
$logDir = Join-Path $state 'e2e-logs'
New-Item -ItemType Directory -Force $logDir | Out-Null
$log = Join-Path $logDir ("{0}-{1}.log" -f ($Who -replace '[^\w.-]', '_'), (Get-Date -Format 'yyyyMMdd-HHmmss'))
$code = 1
# Every log starts with what ran and where, whatever the command does after.
@("e2e run by $Who in $here", "command: $Command", "slot $slot (ports $($ports[0])-$($ports[-1])), login $As", "stamp at start: $stampStart") + $warn + @('') | Set-Content -Path $log -Encoding utf8
# The command goes into its own script file, run with both of its output
# streams sent to files: quotes inside it reach PowerShell unchanged, and even
# a command that fails to parse leaves its error in the log.
$run = "$log.ps1"; $out = "$log.out"; $err = "$log.err"
# A command that fails inside PowerShell (a typo, a missing program) runs no
# program to set an exit code, so its recorded errors make the exit 1.
Set-Content -Path $run -Encoding utf8 -Value @('$Error.Clear()', $Command, 'if ($LASTEXITCODE) { exit $LASTEXITCODE } elseif ($Error.Count) { exit 1 } else { exit 0 }')
try {
  $p = Start-Process -FilePath powershell -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $run -WorkingDirectory $here -NoNewWindow -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
  $null = $p.Handle # keeps the exit code readable after the process ends
  while (-not $p.WaitForExit(30000)) { foreach ($x in $held) { try { (Get-Item (Join-Path $x 'holder.txt')).LastWriteTime = Get-Date } catch {} } }
  $code = $p.ExitCode
  if ($null -eq $code) { $code = 1 }
} finally {
  foreach ($x in $held) {
    $h = Read-Holder $x
    if ($h -and $h.Pid -eq $PID) { Remove-Item -Recurse -Force $x -ErrorAction SilentlyContinue }
  }
}
$stampEnd = Preview-Stamp
foreach ($f in $out, $err) { if (Test-Path $f) { Get-Content $f | Add-Content -Path $log -Encoding utf8; Remove-Item $f -ErrorAction SilentlyContinue } }
Remove-Item $run -ErrorAction SilentlyContinue
$tail = @('', "stamp at end: $stampEnd")
if ($stampEnd -ne $stampStart) { $tail += 'stamp CHANGED during the run: 18729 was rebuilt or the role moved, so any check in this run against 18729 is invalid; run it again.' }
$tail += "exit $code"
$tail | Add-Content -Path $log -Encoding utf8
Get-Content $log -Tail 40 | ForEach-Object { "  $_" }
"e2e lock: slot $slot released; exit $code; full log $log"
exit $code
