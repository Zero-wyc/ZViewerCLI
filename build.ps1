# ZViewer CLI build script
# Windows: extreme UPX compression (--best --lzma)
# Linux: normal UPX compression (--lzma)
# macOS: no UPX (not supported)

$C_GO = "C:\Program Files\Go\bin\go.exe"
$C_UPX = "C:\Users\Zero_\AppData\Local\Temp\upx\upx-5.0.0-win64\upx.exe"
$C_DIST = Join-Path $env:TEMP "zviewer-cli-dist"
$C_VER = "0.1.0"

$L_INFO = "[INFO]"
$L_OK = "[OK]"
$L_ERR = "[ERROR]"

function Write-Info  { Write-Host "$L_INFO $args" -ForegroundColor Cyan }
function Write-Ok    { Write-Host "$L_OK   $args" -ForegroundColor Green }
function Write-Error { Write-Host "$L_ERR $args" -ForegroundColor Red }

if (Test-Path $C_DIST) { Remove-Item -Recurse -Force $C_DIST }
New-Item -ItemType Directory -Path $C_DIST -Force | Out-Null

if (-not (Test-Path $C_GO)) {
    Write-Error "Go not found: $C_GO"
    exit 1
}

$goVer = & $C_GO version
Write-Info "Go version: $goVer"

$hasUPX = Test-Path $C_UPX
if ($hasUPX) {
    $uv = & $C_UPX --version | Select-String "UPX" | Select-Object -First 1
    Write-Info "UPX version: $uv"
} else {
    Write-Info "UPX not found, skip compression"
}

function Build-Binary {
    param($TargetOS, $Arch, $Name, $CompressMode)

    $binaryName = if ($TargetOS -eq "windows") { "$Name.exe" } else { $Name }
    $outputPath = Join-Path $C_DIST $binaryName

    Write-Info "Building $TargetOS/$Arch -> $binaryName"

    $env:GOOS = $TargetOS
    $env:GOARCH = $Arch
    $env:CGO_ENABLED = 0

    & $C_GO build -ldflags="-s -w" -trimpath -o $outputPath .

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Build failed for $TargetOS/$Arch"
        Remove-Item env:GOOS, env:GOARCH, env:CGO_ENABLED -ErrorAction SilentlyContinue
        return
    }

    $beforeSize = (Get-Item $outputPath).Length
    Write-Info "  built: $([math]::Round($beforeSize / 1KB)) KB"

    if ($TargetOS -eq "darwin") {
        Write-Ok ("  ${binaryName}: $([math]::Round($beforeSize / 1KB)) KB (macOS skip UPX)")
        Remove-Item env:GOOS, env:GOARCH, env:CGO_ENABLED -ErrorAction SilentlyContinue
        return
    }

    if ($hasUPX) {
        $upxArgs = if ($CompressMode -eq "extreme") {
            @("--best", "--lzma", $outputPath)
        } else {
            @("--lzma", $outputPath)
        }

        Write-Info "  compressing ($CompressMode)..."
        $result = & $C_UPX @upxArgs 2>&1

        if ($LASTEXITCODE -eq 0) {
            $afterSize = (Get-Item $outputPath).Length
            $ratio = [math]::Round($afterSize / $beforeSize * 100, 1)
            Write-Ok ("  ${binaryName}: $([math]::Round($beforeSize / 1KB)) KB -> $([math]::Round($afterSize / 1KB)) KB ($ratio%)")
        } else {
            Write-Error "  UPX failed: $result"
        }
    } else {
        Write-Ok ("  ${binaryName}: $([math]::Round($beforeSize / 1KB)) KB (uncompressed)")
    }

    Remove-Item env:GOOS, env:GOARCH, env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Info "========== ZViewer CLI v$C_VER build start =========="
$startTime = Get-Date

Build-Binary -TargetOS "windows" -Arch "amd64" -Name "zviewer-cli-windows-amd64" -CompressMode "extreme"
Build-Binary -TargetOS "linux" -Arch "amd64" -Name "zviewer-cli-linux-amd64" -CompressMode "normal"
Build-Binary -TargetOS "linux" -Arch "arm64" -Name "zviewer-cli-linux-arm64" -CompressMode "normal"
Build-Binary -TargetOS "darwin" -Arch "amd64" -Name "zviewer-cli-darwin-amd64" -CompressMode "normal"
Build-Binary -TargetOS "darwin" -Arch "arm64" -Name "zviewer-cli-darwin-arm64" -CompressMode "normal"

$elapsed = (Get-Date) - $startTime
Write-Info "========== Build complete, $([math]::Round($elapsed.TotalSeconds, 1))s =========="
Write-Info "Output: $C_DIST"

Get-ChildItem $C_DIST | ForEach-Object {
    Write-Host ("  " + $_.Name + " - " + [math]::Round($_.Length / 1KB) + " KB")
}