# Install or upgrade athena-proxy from the latest GitHub release.
#
#   irm https://raw.githubusercontent.com/astraqcd/athena-proxy/main/install.ps1 | iex
#
# Optional env:
#   ATHENA_PROXY_VERSION      Pin a release tag (e.g. v1.0.0). Default: latest.
#   ATHENA_PROXY_INSTALL_DIR  Install directory. Default: %LOCALAPPDATA%\Programs\athena-proxy

$ErrorActionPreference = 'Stop'

$Repo = 'astraqcd/athena-proxy'
$Binary = 'athena-proxy.exe'

function Write-Info([string]$Message) {
    Write-Host $Message
}

function Die([string]$Message) {
    Write-Error "error: $Message"
    exit 1
}

$InstallDir = if ($env:ATHENA_PROXY_INSTALL_DIR -and $env:ATHENA_PROXY_INSTALL_DIR.Trim() -ne '') {
    $env:ATHENA_PROXY_INSTALL_DIR.Trim()
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\athena-proxy'
}

$Version = if ($env:ATHENA_PROXY_VERSION -and $env:ATHENA_PROXY_VERSION.Trim() -ne '') {
    $env:ATHENA_PROXY_VERSION.Trim()
} else {
    ''
}

$Arch = $env:PROCESSOR_ARCHITECTURE
switch -Regex ($Arch) {
    '^(AMD64|X64)$' { $Arch = 'amd64' }
    '^(ARM64)$' { $Arch = 'arm64' }
    default { Die "unsupported architecture: $Arch" }
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    Write-Info 'Resolving latest release…'
    try {
        $headers = @{
            Accept                 = 'application/vnd.github+json'
            'User-Agent'           = 'athena-proxy-install'
            'X-GitHub-Api-Version' = '2022-11-28'
        }
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
        $Version = [string]$release.tag_name
    } catch {
        Die "could not resolve latest release; set ATHENA_PROXY_VERSION ($($_.Exception.Message))"
    }
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    Die 'could not resolve latest release; set ATHENA_PROXY_VERSION'
}

if (-not $Version.StartsWith('v')) {
    $Version = "v$Version"
}

$Asset = "athena-proxy_${Version}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
$Dest = Join-Path $InstallDir $Binary

Write-Info "Installing athena-proxy $Version (windows/$Arch) → $Dest"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$stage = Join-Path ([System.IO.Path]::GetTempPath()) ('athena-proxy-' + [guid]::NewGuid().ToString('N'))
$archive = "$stage.zip"
try {
    Invoke-WebRequest -Uri $Url -OutFile $archive -UseBasicParsing
    Expand-Archive -LiteralPath $archive -DestinationPath $stage -Force

    $unpacked = Join-Path $stage $Binary
    if (-not (Test-Path -LiteralPath $unpacked)) {
        Die "$Asset holds no $Binary"
    }

    $replaced = $false
    for ($i = 0; $i -lt 5; $i++) {
        try {
            Move-Item -Force -LiteralPath $unpacked -Destination $Dest
            $replaced = $true
            break
        } catch {
            Start-Sleep -Milliseconds 200
        }
    }
    if (-not $replaced) {
        Die "could not write $Dest (is athena-proxy running?)"
    }
} finally {
    foreach ($path in @($archive, $stage)) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -Force -Recurse -LiteralPath $path -ErrorAction SilentlyContinue
        }
    }
}

$onPath = ($env:PATH -split ';' | Where-Object { $_ -and ($_ -ieq $InstallDir) }).Count -gt 0
if (-not $onPath) {
    Write-Info "Note: $InstallDir is not on PATH. Add it for this session:"
    Write-Info "  `$env:PATH = `"$InstallDir;`$env:PATH`""
    Write-Info 'Or permanently (User PATH):'
    Write-Info "  [Environment]::SetEnvironmentVariable('Path', `"$InstallDir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
}

try {
    $ver = & $Dest --version 2>$null
    if ($ver) {
        Write-Info "Installed $ver"
    } else {
        Write-Info "Installed $Dest"
    }
} catch {
    Write-Info "Installed $Dest"
}
