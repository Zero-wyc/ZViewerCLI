@echo off
chcp 65001 >nul
title ZViewer CLI 启动器

rem 切换到脚本所在目录，避免从其他位置运行时路径错误
cd /d "%~dp0"

set EXE_NAME=zviewer-cli-win.exe
set EXE_PATH=
if exist "%~dp0\%EXE_NAME%" set EXE_PATH=%~dp0\%EXE_NAME%
if exist "%~dp0\release\%EXE_NAME%" set EXE_PATH=%~dp0\release\%EXE_NAME%

if defined EXE_PATH (
    echo [ZViewer CLI] 检测到可执行文件，正在启动本地配置页...
    "%EXE_PATH%" setup
    echo.
    echo [ZViewer CLI] 服务已退出。
    pause
    exit /b 0
)

echo [ZViewer CLI] 未检测到可执行文件，正在检查 Node.js 环境...

where node >nul 2>nul
if %errorlevel% neq 0 (
    echo [错误] 未检测到 Node.js，请先安装 Node.js 后再运行。
    pause
    exit /b 1
)

where npm >nul 2>nul
if %errorlevel% neq 0 (
    echo [错误] 未检测到 npm，请确认 Node.js 安装完整。
    pause
    exit /b 1
)

if not exist "node_modules" (
    echo [ZViewer CLI] 检测到依赖未安装，正在执行 npm install...
    call npm install
    if %errorlevel% neq 0 (
        echo [错误] 依赖安装失败，请检查网络或手动运行 npm install。
        pause
        exit /b 1
    )
)

echo [ZViewer CLI] 正在启动本地配置页...

rem 优先使用构建产物，release 包中 tsx 等 dev 依赖可能未安装
if exist "dist\index.js" (
    node dist\index.js setup
) else (
    rem 源码开发模式：使用 tsx 直接运行 TypeScript
    call npm run dev -- setup
)

echo.
echo [ZViewer CLI] 服务已退出。
pause
