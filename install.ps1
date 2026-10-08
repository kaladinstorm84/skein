# Skein installer for Windows (PowerShell 5.1 or later).
#
# One-line install:
#   powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/kaladinstorm84/skein/main/install.ps1 | iex"
#
# Options (environment variables, set before running):
#   SKEIN_VERSION      Release tag to install, e.g. v0.7.0 (default: latest)
#   SKEIN_INSTALL_DIR  Target directory (default: %LOCALAPPDATA%\Programs\skein)
#
# Downloads skein-windows-amd64.exe from GitHub Releases, verifies it against
# the release SHA256SUMS.txt, installs it as skein.exe, and adds the install
# directory to the user PATH if needed.

$ErrorActionPreference = 'Stop'

$Repo = 'kaladinstorm84/skein'
$Asset = 'skein-windows-amd64.exe'

$Version = if ($env:SKEIN_VERSION) { $env:SKEIN_VERSION } else { 'latest' }
$InstallDir = if ($env:SKEIN_INSTALL_DIR) { $env:SKEIN_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\skein' }

if ($Version -eq 'latest') {
    $Base = "https://github.com/$Repo/releases/latest/download"
} else {
    $Base = "https://github.com/$Repo/releases/download/$Version"
}

# TLS 1.2 for Windows PowerShell 5.1; harmless on PowerShell 7+.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Tmp = Join-Path ([IO.Path]::GetTempPath()) ("skein-install-" + [IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
    Write-Host "Downloading $Asset ($Version) from github.com/$Repo ..."
    $ExePath = Join-Path $Tmp $Asset
    $SumsPath = Join-Path $Tmp 'SHA256SUMS.txt'
    Invoke-WebRequest -UseBasicParsing -Uri "$Base/$Asset" -OutFile $ExePath
    Invoke-WebRequest -UseBasicParsing -Uri "$Base/SHA256SUMS.txt" -OutFile $SumsPath

    $SumLine = Get-Content $SumsPath | Where-Object { $_ -match "^\s*([0-9a-fA-F]{64})\s+\*?$([regex]::Escape($Asset))\s*$" }
    if (-not $SumLine) {
        throw "No entry for $Asset in SHA256SUMS.txt"
    }
    $Expected = ($SumLine -split '\s+')[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ExePath).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) {
        throw "Checksum mismatch for $Asset (expected $Expected, got $Actual)"
    }
    Write-Host "Checksum verified: $Actual"

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $Target = Join-Path $InstallDir 'skein.exe'
    Copy-Item -Path $ExePath -Destination $Target -Force
    Write-Host "Installed: $Target"

    $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $OnPath = ($UserPath -split ';') -contains $InstallDir
    if (-not $OnPath) {
        [Environment]::SetEnvironmentVariable('Path', "$UserPath;$InstallDir", 'User')
        Write-Host "Added $InstallDir to your user PATH."
    }
    if (($env:Path -split ';') -notcontains $InstallDir) {
        $env:Path = "$env:Path;$InstallDir"
        Write-Host "PATH updated for this session; new terminals pick it up automatically."
    }
} finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
