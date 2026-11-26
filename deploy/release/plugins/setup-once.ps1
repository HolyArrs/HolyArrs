# One-time setup of the plugin publishing secrets in HolyArrs/HolyArrs-Plugins.
#
# Run it once, by hand, from this repo's root:
#   powershell -ExecutionPolicy Bypass -File deploy\release\plugins\setup-once.ps1
#
# 1. Makes a new plugin signing key and pipes the private half straight into
#    the PLUGIN_SIGNING_KEY secret. It is never shown or saved; only the
#    public key is written, to deploy/release/plugins/keys/main.pub, and
#    pinned in app/backend/cmd/holyarrs/plugins_wiring.go.
# 2. Asks for the Cloudflare values the workflows need (typing is hidden)
#    and sets each as a secret. Press Enter on one to skip it.
#
# Nothing else is changed. Commit the two changed public files afterwards.

$ErrorActionPreference = 'Stop'
$Repo = 'HolyArrs/HolyArrs-Plugins'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$Pub = Join-Path $Root 'deploy\release\plugins\keys\main.pub'
$Wiring = Join-Path $Root 'app\backend\cmd\holyarrs\plugins_wiring.go'

function Set-Secret([string]$Name, [string]$Env, [string]$Value) {
    $Value | gh secret set $Name --env $Env -R $Repo
    if ($LASTEXITCODE -ne 0) { throw "setting $Name in $Env failed" }
    Write-Host "  set $Name ($Env)"
}

function Read-Hidden([string]$Prompt) {
    $s = Read-Host -AsSecureString $Prompt
    $p = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($s)
    try { return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($p) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($p) }
}

gh auth status *> $null
if ($LASTEXITCODE -ne 0) { throw 'sign in to gh first (gh auth login)' }

# 1. Signing key.
$tool = Join-Path ([IO.Path]::GetTempPath()) 'holyarrs-pluginkey.exe'
Push-Location (Join-Path $Root 'app\backend')
try { go build -o $tool ./tools/pluginkey; if ($LASTEXITCODE -ne 0) { throw 'building pluginkey failed' } }
finally { Pop-Location }

if (Test-Path $Pub) { Move-Item -Force $Pub "$Pub.old" }
Write-Host 'Making the plugin signing key (it goes straight into the secret)...'
& $tool keygen -plain -pipe -pub $Pub | gh secret set PLUGIN_SIGNING_KEY --env plugin-signing -R $Repo
if ($LASTEXITCODE -ne 0) {
    if (Test-Path "$Pub.old") { Move-Item -Force "$Pub.old" $Pub }
    throw 'setting PLUGIN_SIGNING_KEY failed; nothing changed'
}
Remove-Item -Force "$Pub.old" -ErrorAction SilentlyContinue
Remove-Item -Force $tool -ErrorAction SilentlyContinue

$newKey = (Get-Content $Pub -Raw).Trim()
$go = Get-Content $Wiring -Raw
$go = [regex]::Replace($go, '"[A-Za-z0-9+/=]{44}", // main', "`"$newKey`", // main")
[IO.File]::WriteAllText($Wiring, $go)
Write-Host "  new public key $newKey pinned in plugins_wiring.go"

# 2. Cloudflare values (R2 bucket holyarrs-plugins, zone holyarrs.com).
Write-Host ''
Write-Host 'Cloudflare values (hidden; Enter skips one):'
$asks = @(
    @('R2_ACCOUNT_ID', 'Cloudflare account ID'),
    @('R2_ACCESS_KEY_ID', 'R2 access key ID (bucket holyarrs-plugins, read and write)'),
    @('R2_SECRET_ACCESS_KEY', 'R2 secret access key'),
    @('CF_API_TOKEN', 'Cloudflare API token (Analytics read, zone holyarrs.com)'),
    @('CF_ZONE_ID', 'Zone ID of holyarrs.com')
)
$got = @{}
foreach ($a in $asks) {
    $v = Read-Hidden $a[1]
    if ($v) { $got[$a[0]] = $v }
}
foreach ($k in 'R2_ACCOUNT_ID', 'R2_ACCESS_KEY_ID', 'R2_SECRET_ACCESS_KEY') {
    if ($got[$k]) { Set-Secret $k 'plugin-signing' $got[$k] }
}
foreach ($k in 'R2_ACCESS_KEY_ID', 'R2_SECRET_ACCESS_KEY', 'CF_API_TOKEN', 'CF_ZONE_ID') {
    if ($got[$k]) { Set-Secret $k 'plugin-stats' $got[$k] }
}
if ($got['R2_ACCOUNT_ID']) { Set-Secret 'CF_ACCOUNT_ID' 'plugin-stats' $got['R2_ACCOUNT_ID'] }
$got.Clear()

Write-Host ''
Write-Host 'Done. Commit the new public key and the pin.'
