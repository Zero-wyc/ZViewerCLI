#!/usr/bin/env bash
# ZViewer CLI 跨平台构建脚本（Bash）
# 用法: ./build.sh [platform]
# platform 可选: win, macos-x64, macos-arm64, linux-x64, all（默认）

set -euo pipefail

TARGET="${1:-all}"
VERSION="0.1.0"
LDFLAGS="-s -w -X main.version=$VERSION"
GOPROXY="https://goproxy.cn,direct"

mkdir -p release

build_target() {
    local goos="$1"
    local goarch="$2"
    local output="$3"
    local ext="${4:-}"
    local out="release/${output}${ext}"

    echo "Building $goos/$goarch -> $out"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 GOPROXY="$GOPROXY" \
        go build -trimpath -ldflags "$LDFLAGS" -o "$out" .

    local size
    size=$(du -h "$out" | cut -f1)
    echo "  Size: $size"

    if command -v upx >/dev/null 2>&1; then
        echo "  Compressing with UPX..."
        upx --best --lzma "$out" >/dev/null 2>&1 || true
        size=$(du -h "$out" | cut -f1)
        echo "  Compressed size: $size"
    fi
}

case "${TARGET,,}" in
    win)
        build_target windows amd64 zviewer-cli-win .exe
        ;;
    macos-x64)
        build_target darwin amd64 zviewer-cli-macos-x64
        ;;
    macos-arm64)
        build_target darwin arm64 zviewer-cli-macos-arm64
        ;;
    linux-x64)
        build_target linux amd64 zviewer-cli-linux-x64
        ;;
    all)
        build_target windows amd64 zviewer-cli-win .exe
        build_target darwin amd64 zviewer-cli-macos-x64
        build_target darwin arm64 zviewer-cli-macos-arm64
        build_target linux amd64 zviewer-cli-linux-x64
        ;;
    *)
        echo "Unknown target: $TARGET"
        echo "Valid targets: win, macos-x64, macos-arm64, linux-x64, all"
        exit 1
        ;;
esac

cp -f start-cli.bat release/start-cli.bat
cp -f start-cli.sh release/start-cli.sh
chmod +x release/start-cli.sh

echo "Build complete."
