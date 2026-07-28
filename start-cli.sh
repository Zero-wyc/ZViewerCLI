#!/usr/bin/env bash
# ZViewer CLI 一键启动脚本（macOS / Linux）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "[ZViewer CLI] 正在启动本地高画质代理..."

OS="$(uname -s)"
ARCH="$(uname -m)"
BINARY=""

case "$OS" in
    Linux)
        BINARY="zviewer-cli-linux-x64"
        ;;
    Darwin)
        if [ "$ARCH" = "arm64" ]; then
            BINARY="zviewer-cli-macos-arm64"
        else
            BINARY="zviewer-cli-macos-x64"
        fi
        ;;
    *)
        echo "[错误] 不支持的操作系统: $OS"
        exit 1
        ;;
esac

if [ ! -f "$BINARY" ]; then
    echo "[错误] 找不到可执行文件: $BINARY"
    exit 1
fi

chmod +x "$BINARY"
"./$BINARY" --setup &

echo "[ZViewer CLI] 本地配置页将在浏览器打开，地址: http://127.0.0.1:9333"
wait
