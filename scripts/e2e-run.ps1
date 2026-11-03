param(
  [Parameter(Mandatory = $true, Position = 0)][string]$Command,
  [string]$Who = (Split-Path -Leaf (Get-Location)),
  [int]$StaleMinutes = 45,
  [int]$WaitMinutes = 120
)
# Runs one e2e command while holding the shared e2e lock, so sessions in any
# worktree take turns on the e2e harness. The lock is one folder at a fixed
# path; making a folder either works or fails at once, so two runs never both
# get it. A lock whose holder is gone, or that is older than StaleMinutes, is
# taken over. Example:
#   & C:\HolyForge\coderluii\cl\holyarrs\repo\scripts\e2e-run.ps1 'npx playwright test e2e/library.spec.ts' -Who library
$ErrorActionPreference = 'Stop'
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
try {
  # Encoded, so quotes inside the command reach the child shell unchanged.
  & powershell -NoProfile -EncodedCommand ([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($Command)))
  $code = $LASTEXITCODE
} finally {
  $h = Read-Holder
  if ($h -and $h.Pid -eq $PID) { Remove-Item -Recurse -Force $lock -ErrorAction SilentlyContinue }
  "e2e lock: released"
}
exit $code
