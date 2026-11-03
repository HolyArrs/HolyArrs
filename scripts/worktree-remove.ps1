param([Parameter(Mandatory = $true, Position = 0)][string]$Path, [switch]$Force)
# Removes a git worktree safely. Some worktrees link node_modules to a shared
# folder (a junction); `git worktree remove --force` deletes through that link
# and empties the shared folder for every session (it happened three times on
# 2026-11-03). This script unlinks every junction under the worktree first,
# leaving their targets alone, then removes the worktree. Example:
#   & C:\HolyForge\coderluii\cl\holyarrs\repo\scripts\worktree-remove.ps1 C:\HolyForge\coderluii\cl\holyarrs\worktrees\my-check
$ErrorActionPreference = 'Stop'
$wt = (Resolve-Path $Path).Path
$repo = 'C:\HolyForge\coderluii\cl\holyarrs\repo'
# Junctions are found without walking into them, so nothing behind a link is touched.
function Find-Links([string]$dir) {
  foreach ($d in [System.IO.Directory]::GetDirectories($dir)) {
    $attr = [System.IO.File]::GetAttributes($d)
    if ($attr -band [System.IO.FileAttributes]::ReparsePoint) { $d } else { Find-Links $d }
  }
}
$links = @(Find-Links $wt)
foreach ($l in $links) {
  [System.IO.Directory]::Delete($l, $false) # removes the link only, never its target
  "unlinked $l"
}
$args = @('-C', $repo, 'worktree', 'remove')
if ($Force) { $args += '--force' }
& git @args $wt
if ($LASTEXITCODE -ne 0) { "git worktree remove failed (exit $LASTEXITCODE); the links are already gone, so --force is now safe"; exit $LASTEXITCODE }
"removed $wt"
