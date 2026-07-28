# ZViewer CLI cross-platform build script (PowerShell)
# Usage: .\build.ps1 [target]
# Valid targets: win, macos-x64, macos-arm64, linux-x64, all (default)

param(
    [string]$Target = "all",
    [switch]$Compress
)

$ErrorActionPreference = "Stop"
$GOPROXY = "https://goproxy.cn,direct"
$version = "0.1.0"
$ldflags = "-s -w -X main.version=$version"

# Locate Go binary
$goBin = Get-Command go -ErrorAction SilentlyContinue
if (-not $goBin) {
    $candidates = @(
        "C:\Program Files\Go\bin\go.exe",
        "C:\Go\bin\go.exe",
        "$env:LOCALAPPDATA\Programs\Go\bin\go.exe"
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) {
            $env:PATH = "$env:PATH;$(Split-Path $c)"
            $goBin = $c
            break
        }
    }
}
if (-not $goBin) {
    throw "Go is not installed or not in PATH. Please install Go from https://go.dev/dl/"
}

function Build-Target {
    param(
        [string]$GOOS,
        [string]$GOARCH,
        [string]$OutputName,
        [string]$Ext = ""
    )

    $out = "release\$OutputName$Ext"
    Write-Host "Building $GOOS/$GOARCH -> $out" -ForegroundColor Cyan

    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH
    $env:CGO_ENABLED = "0"
    $env:GOPROXY = $GOPROXY

    & go build -trimpath -ldflags "$ldflags" -o $out .
    if ($LASTEXITCODE -ne 0) { throw "Build failed for $GOOS/$GOARCH" }

    $info = Get-Item $out
    Write-Host "  Size: $([math]::Round($info.Length / 1MB, 2)) MB" -ForegroundColor Gray

    if ($Compress -and (Get-Command upx -ErrorAction SilentlyContinue)) {
        Write-Host "  Compressing with UPX..." -ForegroundColor DarkGray
        upx --best --lzma $out | Out-Null
        $info = Get-Item $out
        Write-Host "  Compressed size: $([math]::Round($info.Length / 1MB, 2)) MB" -ForegroundColor Gray
    }
}

New-Item -ItemType Directory -Force -Path "release" | Out-Null

if (-not (Test-Path "page.html")) {
    throw "page.html not found. Please keep it alongside main.go."
}

switch ($Target.ToLower()) {
    "win" { Build-Target -GOOS "windows" -GOARCH "amd64" -OutputName "zviewer-cli-win" -Ext ".exe" }
    "macos-x64" { Build-Target -GOOS "darwin" -GOARCH "amd64" -OutputName "zviewer-cli-macos-x64" }
    "macos-arm64" { Build-Target -GOOS "darwin" -GOARCH "arm64" -OutputName "zviewer-cli-macos-arm64" }
    "linux-x64" { Build-Target -GOOS "linux" -GOARCH "amd64" -OutputName "zviewer-cli-linux-x64" }
    "all" {
        Build-Target -GOOS "windows" -GOARCH "amd64" -OutputName "zviewer-cli-win" -Ext ".exe"
        Build-Target -GOOS "darwin" -GOARCH "amd64" -OutputName "zviewer-cli-macos-x64"
        Build-Target -GOOS "darwin" -GOARCH "arm64" -OutputName "zviewer-cli-macos-arm64"
        Build-Target -GOOS "linux" -GOARCH "amd64" -OutputName "zviewer-cli-linux-x64"
    }
    default {
        Write-Host "Unknown target: $Target" -ForegroundColor Red
        Write-Host "Valid targets: win, macos-x64, macos-arm64, linux-x64, all"
        exit 1
    }
}

Copy-Item -Force "start-cli.bat" "release\start-cli.bat" | Out-Null
Copy-Item -Force "start-cli.sh" "release\start-cli.sh" | Out-Null

Write-Host "Build complete." -ForegroundColor Green
