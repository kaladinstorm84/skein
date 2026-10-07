#Requires -Version 5.1
<#
.SYNOPSIS
  End-to-end Phase 1 demo: init → patch → test → approve → land → why.
.EXAMPLE
  powershell -NoProfile -File .\demo.ps1
#>
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$Skein = Join-Path $RepoRoot "skein.exe"
$Work = Join-Path $env:TEMP ("skein-demo-" + [guid]::NewGuid().ToString("n"))
$Checkout = Join-Path $Work "co"

function Invoke-Skein {
    param(
        [Parameter(Mandatory)][string[]]$SkeinArgs,
        [switch]$AllowFail
    )
    $all = @("--dir", $Work) + $SkeinArgs
    Write-Host ""
    Write-Host (">>> skein " + ($all -join " ")) -ForegroundColor Cyan
    $stdout = & $Skein @all 2>&1 | Out-String
    Write-Host $stdout.TrimEnd()
    $obj = $stdout | ConvertFrom-Json
    if (-not $AllowFail) {
        if ($LASTEXITCODE -ne 0 -or -not $obj.ok) {
            throw "skein failed (exit $LASTEXITCODE): $stdout"
        }
    }
    return $obj
}

if (-not (Test-Path -LiteralPath $Skein)) {
    Write-Host "Building skein.exe..." -ForegroundColor Yellow
    Push-Location $RepoRoot
    try {
        go build -trimpath -ldflags="-s -w" -o skein.exe ./cmd/skein
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    }
    finally { Pop-Location }
}

New-Item -ItemType Directory -Path $Work | Out-Null
Write-Host "Demo repository: $Work" -ForegroundColor Green

try {
    $null = Invoke-Skein @("init", "--path-mode", "case-sensitive")
    $thread = Invoke-Skein @("open-thread", "--title", "hello from demo", "--scale", "trivial")
    $null = Invoke-Skein @("threads")
    $null = Invoke-Skein @("checkout", "--thread", "hello-from-demo", "--out", $Checkout)

    [System.IO.File]::WriteAllText((Join-Path $Checkout "hello.txt"), "hello skein`n")
    $null = Invoke-Skein @("sync", "--checkout", $Checkout)
    $null = Invoke-Skein @("test", "--thread", "hello-from-demo")

    $prop = Invoke-Skein @("propose-land", "--thread", "hello-from-demo")
    $wpShort = [string]$prop.weave_proposal_id_short
    $null = Invoke-Skein @("approve", "--weave-proposal", $wpShort)
    $land = Invoke-Skein @("land", "--weave-proposal", $wpShort)
    $null = Invoke-Skein @("why", "--path", "hello.txt")
    $null = Invoke-Skein @("log")

    $stale = Invoke-Skein @("land", "--weave-proposal", $wpShort) -AllowFail
    if ($stale.ok -or $stale.error -ne "stale") {
        throw "expected stale reland, got $($stale | ConvertTo-Json -Compress)"
    }

    Write-Host ""
    Write-Host '>>> skein agent  {"verb":"port"}  (expect policy)' -ForegroundColor Cyan
    $portRaw = '{"verb":"port"}' | & $Skein --dir $Work agent 2>&1 | Out-String
    Write-Host $portRaw.TrimEnd()
    $port = $portRaw | ConvertFrom-Json
    if ($port.ok -or $port.error -ne "policy") {
        throw "expected policy on port, got $portRaw"
    }

    Write-Host ""
    Write-Host "Landed. state_root=$($land.state_root)" -ForegroundColor Green
    Write-Host "Repo left at $Work"
}
catch {
    Write-Host $_ -ForegroundColor Red
    exit 1
}
