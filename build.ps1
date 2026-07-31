# ZViewer CLI 一键编译脚本
# 自动交叉编译所有平台：
#   Windows → UPX --best --lzma（极限压缩）
#   Linux   → UPX --lzma（正常压缩）
#   macOS   → 跳过 UPX（不支持）
#
# 用法: .\build.ps1

param(
    [string]$UPXPath = ""  # 留空则自动查找 PATH 或默认路径
)

$C_GO = "C:\Program Files\Go\bin\go.exe"
$C_DIST = Join-Path $env:TEMP "zviewer-cli-dist"
$C_VER = "0.1.0"

$L_INFO = "[INFO]"
$L_OK = "[OK]"
$L_ERR = "[ERROR]"

function Write-Info  { Write-Host "$L_INFO $args" -ForegroundColor Cyan }
function Write-Ok    { Write-Host "$L_OK   $args" -ForegroundColor Green }
function Write-Error { Write-Host "$L_ERR $args" -ForegroundColor Red }

# 清理并创建输出目录
if (Test-Path $C_DIST) { Remove-Item -Recurse -Force $C_DIST }
New-Item -ItemType Directory -Path $C_DIST -Force | Out-Null

# 检查 Go
if (-not (Test-Path $C_GO)) {
    Write-Error "Go not found at: $C_GO`nInstall Go from https://go.dev/dl/ or update `$C_GO in this script."
    exit 1
}
$goVer = & $C_GO version
Write-Info "Go version: $goVer"

# 查找 UPX
$C_UPX = ""
if ($UPXPath -and (Test-Path $UPXPath)) {
    $C_UPX = $UPXPath
} elseif (Get-Command "upx" -ErrorAction SilentlyContinue) {
    $C_UPX = (Get-Command "upx").Source
} else {
    # 常见默认路径
    $defaultPaths = @(
        "$env:USERPROFILE\AppData\Local\Temp\upx\upx-5.0.0-win64\upx.exe",
        "$env:USERPROFILE\scoop\shims\upx.exe",
        "C:\ProgramData\chocolatey\bin\upx.exe"
    )
    foreach ($p in $defaultPaths) {
        if (Test-Path $p) { $C_UPX = $p; break }
    }
}

$hasUPX = ($C_UPX -ne "" -and (Test-Path $C_UPX))
if ($hasUPX) {
    $uv = & $C_UPX --version | Select-String "UPX" | Select-Object -First 1
    Write-Info "UPX: $C_UPX"
    Write-Info "UPX version: $uv"
} else {
    Write-Info "UPX not found, skip compression. Install from: https://github.com/upx/upx/releases"
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

    # macOS 不支持 UPX
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