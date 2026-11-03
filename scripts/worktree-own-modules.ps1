param([string[]]$Only = @(), [switch]$WhatIf)
# Gives every worktree its own node_modules instead of a junction to a shared
# one. A junction lets `git worktree remove` empty the shared folder for every
# session (four times on 2026-11-03); an own copy from the npm cache takes
# about 15-25 s and 164 MB. A worktree with a run in progress (an e2e slot
# held in it, or a node or go process started from it) is skipped; run the
# script again later for those. Example:
#   & C:\HolyForge\coderluii\cl\holyarrs\repo\scripts\worktree-own-modules.ps1
$ErrorActionPreference = 'Stop'
$root = 'C:\HolyForge\coderluii\cl\holyarrs\worktrees'
$state = 'C:\HolyForge\coderluii\cl\holyarrs\.omc\state'
$busyDirs = @(Get-ChildItem $state -Filter 'e2e.lock*' -Directory -ErrorAction SilentlyContinue | ForEach-Object {
  $l = Get-Content (Join-Path $_.FullName 'holder.txt') -ErrorAction SilentlyContinue; if ($l.Count -gt 4) { $l[4] } })
$procs = @(Get-CimInstance Win32_Process | Where-Object { $_.Name -in 'node.exe', 'go.exe', 'holyarrs.exe' } | ForEach-Object { "$($_.CommandLine) $($_.ExecutablePath)" })
$done = 0; $skipped = @()
foreach ($wt in Get-ChildItem $root -Directory) {
  if ($Only -and $wt.Name -notin $Only) { continue }
  $app = Join-Path $wt.FullName 'web\app'
  $nm = Join-Path $app 'node_modules'
  $item = Get-Item $nm -Force -ErrorAction SilentlyContinue
  if (-not $item -or $item.LinkType -ne 'Junction') { continue }
  $busy = ($busyDirs | Where-Object { $_ -and $_.StartsWith($wt.FullName, [StringComparison]::OrdinalIgnoreCase) }) -or ($procs | Where-Object { $_ -like "*$($wt.FullName)*" })
  if ($busy) { $skipped += $wt.Name; "skip $($wt.Name): a run is in progress there"; continue }
  if (-not (Test-Path (Join-Path $app 'package-lock.json'))) { $skipped += $wt.Name; "skip $($wt.Name): no package-lock.json"; continue }
  if ($WhatIf) { "would convert $($wt.Name) (now -> $($item.Target))"; continue }
  [System.IO.Directory]::Delete($nm, $false) # the link only, never its target
  Push-Location $app
  try { $out = npm ci --offline --no-audit --no-fund 2>&1 | Select-Object -Last 1 } finally { Pop-Location }
  $ok = Test-Path (Join-Path $nm '.bin\vite.cmd')
  "$(if ($ok) { 'converted' } else { 'FAILED' }) $($wt.Name): $out"
  if ($ok) { $done++ } else { $skipped += $wt.Name }
}
"done: $done converted; skipped: $($skipped -join ', ')"
