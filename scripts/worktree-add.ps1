param(
  [Parameter(Mandatory = $true, Position = 0)][string]$Name,
  [string]$Branch = '',   # new branch name; empty = detached at -From
  [string]$From = 'master'
)
# Makes a worktree under worktrees\ with its own node_modules (npm ci from the
# npm cache, about 15-25 s), never a junction to a shared one: removing a
# worktree with a junction emptied the shared folder four times on 2026-11-03.
# Remove it later with worktree-remove.ps1. Example:
#   & C:\HolyForge\coderluii\cl\holyarrs\repo\scripts\worktree-add.ps1 lib-42 -Branch lib-42
$ErrorActionPreference = 'Stop'
$repo = 'C:\HolyForge\coderluii\cl\holyarrs\repo'
$wt = Join-Path 'C:\HolyForge\coderluii\cl\holyarrs\worktrees' $Name
if (Test-Path $wt) { throw "$wt already exists" }
if ($Branch) { git -C $repo worktree add -q -b $Branch $wt $From } else { git -C $repo worktree add -q --detach $wt $From }
if ($LASTEXITCODE -ne 0) { throw 'git worktree add failed' }
Push-Location (Join-Path $wt 'web\app')
try { npm ci --offline --no-audit --no-fund 2>&1 | Select-Object -Last 1 } finally { Pop-Location }
if (-not (Test-Path (Join-Path $wt 'web\app\node_modules\.bin\vite.cmd'))) { throw "npm ci did not finish in $wt\web\app" }
"ready: $wt (own node_modules)"
