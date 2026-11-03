param(
  [Parameter(Mandatory = $true, Position = 0)][string]$Command,
  [string]$Who = (Split-Path -Leaf (Get-Location)),
  [ValidateSet('agent', 'owner')][string]$As = 'agent',
  [int]$StaleMinutes = 45,
  [int]$WaitMinutes = 120
)
# Runs one e2e command while holding the shared e2e lock, so sessions in any
# worktree take turns on the e2e harness. The lock is one folder at a fixed
# path; making a folder either works or fails at once, so two runs never both
# get it. A lock whose holder is gone, or that is older than StaleMinutes, is
# taken over. Example:
#   & C:\HolyForge\coderluii\cl\holyarrs\repo\scripts\e2e-run.ps1 'npx playwright test e2e/library.spec.ts' -Who library
# -As picks which 18729 showcase login a check reads (never printed): agent,
# the admin agent-check (default), or owner, for owner-only pages. The
# command gets that file's path in HOLYARRS_SHOWCASE_LOGIN.
$ErrorActionPreference = 'Stop'
$logins = @{ agent = 'C:\HolyForge\coderluii\cl\holyarrs\secrets\showcase-18729-agent.txt'; owner = 'C:\HolyForge\coderluii\cl\holyarrs\secrets\showcase-18729-home.txt' }
$env:HOLYARRS_SHOWCASE_LOGIN = $logins[$As]
$state = 'C:\HolyForge\coderluii\cl\holyarrs\.omc\state'
$lock = Join-Path $state 'e2e.lock'
$info = Join-Path $lock 'holder.txt'
New-Item -ItemType Directory -Force $state | Out-Null

function Read-Holder {
  try { $l = Get-Content $info -ErrorAction Stop; return @{ Pid = [int]$l[0]; Who = $l[1]; At = [datetime]::Parse($l[2]) } } catch { return $null }
}

$deadline = (Get-Date).AddMinutes($WaitMinutes)
$said = $false
while ($true) {
  try {
    New-Item -ItemType Directory $lock -ErrorAction Stop | Out-Null
    Set-Content -Path $info -Encoding ascii -Value @($PID, $Who, (Get-Date).ToString('o'), $Command)
    break
  } catch {}
  $h = Read-Holder
  $gone = $h -and -not (Get-Process -Id $h.Pid -ErrorAction SilentlyContinue)
  $old = $h -and ((Get-Date) - $h.At).TotalMinutes -gt $StaleMinutes
  $noInfo = (-not $h) -and (Test-Path $lock) -and ((Get-Date) - (Get-Item $lock).CreationTime).TotalMinutes -gt 1
  if ($gone -or $old -or $noInfo) {
    "e2e lock: taking over a stale lock ($(if ($h) { "$($h.Who), pid $($h.Pid), since $($h.At)" } else { 'no holder file' }))"
    Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue
    continue
  }
  if ((Get-Date) -gt $deadline) { throw "e2e lock: still held by $($h.Who) after $WaitMinutes minutes" }
  if (-not $said -and $h) { "e2e lock: waiting for $($h.Who) (pid $($h.Pid), since $($h.At))"; $said = $true }
  Start-Sleep -Seconds 5
}

"e2e lock: held by $Who"
# The command runs as its own process with its output in a log file, so a
# caller piping or redirecting this script can never break the run, and its
# stderr lines are never turned into errors here.
$logDir = Join-Path $state 'e2e-logs'
New-Item -ItemType Directory -Force $logDir | Out-Null
$log = Join-Path $logDir ("{0}-{1}.log" -f ($Who -replace '[^\w.-]', '_'), (Get-Date -Format 'yyyyMMdd-HHmmss'))
$code = 1
try {
  # Encoded, so quotes inside the command reach the child shell unchanged.
  $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("& { $Command } *>&1 | Out-File -FilePath '$log' -Encoding utf8; exit `$LASTEXITCODE"))
  $p = Start-Process -FilePath powershell -ArgumentList '-NoProfile', '-EncodedCommand', $enc -WorkingDirectory (Get-Location).Path -NoNewWindow -PassThru -Wait
  $code = $p.ExitCode
} finally {
  $h = Read-Holder
  if ($h -and $h.Pid -eq $PID) { Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue }
}
if (Test-Path $log) { Get-Content $log -Tail 40 | ForEach-Object { "  $_" } }
"e2e lock: released; exit $code; full log $log"
exit $code
