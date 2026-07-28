@echo off
chcp 65001 >nul
cd /d "%~dp0"

echo [ZViewer CLI] 正在启动本地高画质代理...

if exist "zviewer-cli-win.exe" (
    start "" "zviewer-cli-win.exe" --setup
) else if exist "zviewer-cli.exe" (
    start "" "zviewer-cli.exe" --setup
) else (
    echo [错误] 找不到 zviewer-cli-win.exe 或 zviewer-cli.exe
    pause
    exit /b 1
)

echo [ZViewer CLI] 本地配置页将在浏览器打开，地址: http://127.0.0.1:9333
pause
