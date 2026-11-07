param(
  [string]$Old = 'holyarrs-swaptest:n',
  [string]$Mid = 'holyarrs-swaptest:n1',
  [string]$New = 'holyarrs-swaptest:n2',
  [int]$Port = 18745,
  [string]$Lab = 'C:\HolyForge\coderluii\cl\holyarrs\research\lab\swaptest'
)
# Docker image swap test (plan item B2): one config folder goes through three
# images, old -> mid -> new, then new again, then old again. Each swap must
# start, run its migrations once (one "backup before the ... update" line),
# leave one new pre-update backup, and keep the owner's sign-in, the settings,
# the member and the Library folder. The second start of the new image must
# run nothing; the old image on the newer data must refuse and change nothing.
# Build the three images first, for example from worktrees of older commits:
#   docker build -t holyarrs-swaptest:n <worktree of an older commit>
# The owner is a throwaway made here; its password is kept only in $Lab.
# No restart policy: the container never starts at boot. Leaves the container
# stopped and removed, and the lab folder in place for a look afterwards.
$ErrorActionPreference = 'Stop'
$name = 'holyarrs-swaptest'
$base = "http://localhost:$Port"
$cfg = Join-Path $Lab 'config'
$media = Join-Path $Lab 'media\movies'
$vol = 'holyarrs-swaptest-config'
$results = New-Object System.Collections.Generic.List[object]

# Runs docker and ignores what it says (Windows PowerShell turns stderr into errors).
function Quiet {
  $e = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  try { & docker @args 2>&1 | Out-Null } finally { $ErrorActionPreference = $e }
}

function Note([string]$step, [bool]$ok, [string]$detail) {
  $results.Add([pscustomobject]@{ Step = $step; Result = $(if ($ok) { 'pass' } else { 'FAIL' }); Detail = $detail })
  "{0,-4} {1}  {2}" -f $(if ($ok) { 'ok' } else { 'FAIL' }), $step, $detail
}

function Start-Image([string]$image) {
  Quiet rm -f $name
  # /config is a named volume, as the docs ask on Docker Desktop (a C:\ share
  # is refused); a copy lands in $Lab\config at the end.
  docker run -d --name $name -p "${Port}:6262" -v "${vol}:/config" -v "${media}:/movies" $image | Out-Null
}

function Wait-Up([int]$seconds = 120) {
  $end = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $end) {
    $state = docker inspect -f '{{.State.Running}}' $name 2>$null
    if ($state -ne 'true') { return $false }
    try { Invoke-RestMethod "$base/api/v1/auth/state" -TimeoutSec 3 | Out-Null; return $true } catch { Start-Sleep -Milliseconds 700 }
  }
  $false
}

function Stop-Image { docker stop -t 30 $name | Out-Null }

function Call([string]$method, [string]$path, $body, $ws, [string]$csrf) {
  $h = @{ Origin = $base }
  if ($csrf) { $h['X-CSRF-Token'] = $csrf }
  $p = @{ Method = $method; Uri = "$base$path"; Headers = $h; WebSession = $ws; ContentType = 'application/json' }
  if ($null -ne $body) { $p.Body = ($body | ConvertTo-Json -Depth 6 -Compress) }
  Invoke-RestMethod @p
}

function Sign-In([string]$user, [string]$pass) {
  $ws = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $r = Call POST '/api/v1/auth/login' @{ name = $user; password = $pass } $ws $null
  @{ ws = $ws; csrf = $r.csrf }
}

# Copies /config out of the (running or stopped) container for a look.
function Snapshot {
  $d = Join-Path $env:TEMP "holyarrs-swaptest-$([guid]::NewGuid().ToString('n'))"
  Quiet cp "${name}:/config" $d
  $d
}
function Backups {
  $d = Snapshot
  $b = @(Get-ChildItem (Join-Path $d 'backups') -Filter 'holyarrs-backup-pre-update-*.zip' -ErrorAction SilentlyContinue | ForEach-Object Name)
  Remove-Item -Recurse -Force $d -ErrorAction SilentlyContinue
  $b
}
function DbSums {
  $d = Snapshot
  $s = (Get-ChildItem $d -Filter '*.db' | Sort-Object Name | ForEach-Object { $_.Name + '=' + (Get-FileHash $_.FullName).Hash }) -join ','
  Remove-Item -Recurse -Force $d -ErrorAction SilentlyContinue
  $s
}
function Logs {
  $e = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  try { & docker logs $name 2>&1 | ForEach-Object { "$_" } } finally { $ErrorActionPreference = $e }
}
function UpdateLines { @((Logs) | Where-Object { "$_" -match 'store: backup before the .* update \d+ -> \d+' } | ForEach-Object { "$_" }) }

function Check-Kept([string]$label) {
  try { $s = Sign-In $owner $pw; Note "$label sign-in" $true 'owner signs in with the same password' }
  catch { Note "$label sign-in" $false "$_"; return }
  $ui = Call GET '/api/v1/settings/ui' $null $s.ws $s.csrf
  Note "$label settings" ($ui.artwork_cache_gb -eq 2 -and $ui.history_routine_days -eq 14 -and -not $ui.autoplay_trailers) "cache $($ui.artwork_cache_gb) GB, routine $($ui.history_routine_days) d, autoplay $($ui.autoplay_trailers)"
  $users = (Call GET '/api/v1/users' $null $s.ws $s.csrf).users
  $m = @($users | Where-Object { $_.name -eq 'swap-member' -and $_.role -eq 'member' })
  Note "$label users" ($users.Count -eq 2 -and $m.Count -eq 1) "$($users.Count) people, member kept: $($m.Count -eq 1)"
  try { Sign-In 'swap-member' $memberPw | Out-Null; Note "$label member sign-in" $true 'member signs in' } catch { Note "$label member sign-in" $false "$_" }
  $f = @((Call GET '/api/v1/library-folders' $null $s.ws $s.csrf).library_folders)
  $hit = @($f | Where-Object { $_.path -eq '/movies' -and $_.label -eq 'Swap movies' })
  Note "$label folder" ($hit.Count -eq 1) "folders: $(($f | ForEach-Object { $_.path + ' ' + $_.label }) -join ', ')"
}

# Fresh lab folder and a throwaway owner kept only in the lab folder.
Quiet rm -f $name
Quiet volume rm $vol
if (Test-Path $cfg) { Remove-Item -Recurse -Force $cfg }
New-Item -ItemType Directory -Force $cfg, $media | Out-Null
$chars = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'.ToCharArray()
$pw = -join (1..24 | ForEach-Object { $chars | Get-Random })
$memberPw = -join (1..24 | ForEach-Object { $chars | Get-Random })
$owner = 'swap-owner'
"owner $owner $pw`r`nmember swap-member $memberPw" | Set-Content -Encoding ascii (Join-Path $Lab 'throwaway-accounts.txt')

try {
  # 1. Old image: first run and real-looking data through the API.
  Start-Image $Old
  Note 'old starts' (Wait-Up) $Old
  $ws = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $r = Call POST '/api/v1/auth/setup' @{ name = $owner; password = $pw } $ws $null
  $s = @{ ws = $ws; csrf = $r.csrf }
  try { Call POST '/api/v1/auth/setup/done' @{} $s.ws $s.csrf | Out-Null } catch { }
  $ui = Call GET '/api/v1/settings/ui' $null $s.ws $s.csrf
  $ui.artwork_cache_gb = 2; $ui.history_routine_days = 14; $ui.autoplay_trailers = $false; $ui.count_install = $false
  Call PUT '/api/v1/settings/ui' $ui $s.ws $s.csrf | Out-Null
  Call POST '/api/v1/library-folders' @{ path = '/movies'; kind = 'movie'; label = 'Swap movies' } $s.ws $s.csrf | Out-Null
  $inv = Call POST '/api/v1/users/invites' @{ role = 'member'; note = 'swap test'; days = 1 } $s.ws $s.csrf
  $tok = ($inv.path -split '/')[-1]
  $mws = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  Call POST "/api/v1/auth/invites/$tok" @{ name = 'swap-member'; password = $memberPw } $mws $null | Out-Null
  Check-Kept 'old'
  Stop-Image

  # 2. and 3. Swap to mid, then to new, on the same folder.
  foreach ($img in @(@('mid', $Mid), @('new', $New))) {
    $before = (Backups).Count # read from the stopped previous container
    Start-Image $img[1]
    Note "$($img[0]) starts" (Wait-Up) $img[1]
    $lines = UpdateLines
    Note "$($img[0]) migrations once" ($lines.Count -eq 1) ($lines -join ' | ')
    $after = (Backups).Count
    Note "$($img[0]) pre-update backup" ($after -eq $before + 1) "pre-update backups $before -> $after"
    Check-Kept $img[0]
    Stop-Image
  }

  # 4. New image again: nothing to update, no new backup.
  $before = (Backups).Count
  Start-Image $New
  Note 'new again starts' (Wait-Up) $New
  $lines = UpdateLines
  Note 'new again no migrations' ($lines.Count -eq 0) "update lines: $($lines.Count)"
  Note 'new again no backup' ((Backups).Count -eq $before) "pre-update backups $before -> $((Backups).Count)"
  Stop-Image

  # 5. Old image on the newer data: refused in plain words, data untouched.
  $sums = DbSums
  Start-Image $Old
  Start-Sleep -Seconds 8
  $log = ((Logs) | ForEach-Object { "$_" }) -join "`n"
  $page = ''
  try { $page = (Invoke-WebRequest "$base/" -UseBasicParsing -TimeoutSec 5).Content } catch { $page = "$_" }
  $plain = $log -match 'newer HolyArrs' -or $page -match 'newer HolyArrs'
  Note 'old on newer data refused' $plain (($log -split "`n" | Where-Object { $_ -match 'newer' } | Select-Object -First 1))
  Stop-Image
  $sums2 = DbSums
  Note 'old left data intact' ($sums -eq $sums2) 'database files unchanged after the refused start'

  # 6. New image after the refusal still has everything.
  Start-Image $New
  Note 'new after refusal starts' (Wait-Up) $New
  Check-Kept 'new after refusal'
  Stop-Image
  if (Test-Path $cfg) { Remove-Item -Recurse -Force $cfg }
  docker cp "${name}:/config" $cfg | Out-Null
}
finally {
  Quiet rm -f $name
  Quiet volume rm $vol
  ''
  $results | Format-Table -AutoSize | Out-String -Width 200
  $fails = @($results | Where-Object { $_.Result -ne 'pass' }).Count
  "$($results.Count) checks, $fails failed"
}
