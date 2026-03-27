param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$GoTestArgs = @('./...')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Find-GccPath {
    $gccCommand = Get-Command gcc -ErrorAction SilentlyContinue
    if ($gccCommand -and $gccCommand.Source) {
        return $gccCommand.Source
    }

    if (-not $env:LOCALAPPDATA) {
        return $null
    }

    $wingetPackages = Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages'
    if (-not (Test-Path $wingetPackages)) {
        return $null
    }

    $candidates = Get-ChildItem $wingetPackages -Directory -ErrorAction SilentlyContinue |
        ForEach-Object {
            @(
                (Join-Path $_.FullName 'mingw64\bin\gcc.exe')
                (Join-Path $_.FullName 'bin\gcc.exe')
            )
        } |
        Where-Object { Test-Path $_ }

    return $candidates | Select-Object -First 1
}

$gccPath = Find-GccPath
if (-not $gccPath) {
    throw @"
gcc.exe was not found.

Install a Windows C toolchain first, for example:
  winget install --id BrechtSanders.WinLibs.POSIX.UCRT --exact --accept-source-agreements --accept-package-agreements
"@
}

$gccDir = Split-Path -Parent $gccPath
$env:PATH = "$gccDir;$env:PATH"
$env:CC = $gccPath
$env:CXX = Join-Path $gccDir 'g++.exe'
$env:CGO_ENABLED = '1'

Write-Host "Using gcc: $gccPath"
Write-Host "Running: go test -race $($GoTestArgs -join ' ')"

& go test -race @GoTestArgs
