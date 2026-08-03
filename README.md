# ZViewerCLI

> ZViewer 的本地高画质代理客户端。

ZViewerCLI 是一个运行在用户本地的 Go 程序，用于解决浏览器端无法直接使用用户 Bilibili Cookie 与高画质地址的问题。它使用用户自己的 Cookie 在本地解析 Bilibili 视频，并代理视频流请求，从而让 ZViewer 房间中的所有人都能稳定播放大会员等高画质内容。

---

## 目录

- [为什么需要 ZViewerCLI](#为什么需要-zviewercli)
- [功能特性](#功能特性)
- [工作原理](#工作原理)
- [快速开始](#快速开始)
- [命令行参数](#命令行参数)
- [本地 HTTP API](#本地-http-api)
- [构建](#构建)
- [项目结构](#项目结构)
- [与 ZViewer 的关系](#与-zviewer-的关系)
- [常见问题](#常见问题)

---

## 为什么需要 ZViewerCLI

在 ZViewer 中直接解析 Bilibili 视频时，存在以下限制：

1. **Cookie 安全**：浏览器无法安全地将用户的 Bilibili Cookie 发送到服务端。
2. **CORS 与防盗链**：Bilibili CDN 对请求头（Referer、Origin、User-Agent）有严格要求，浏览器直接请求容易被拦截。
3. **画质限制**：服务端配置的 Bilibili Cookie 可能不是大会员，无法获取 1080P+/4K 等高画质。

ZViewerCLI 通过本地代理的方式解决这些问题：Cookie 只留在用户本机，视频解析和流请求都在本地完成，再绕过浏览器的安全限制。

---

## 功能特性

- **本地 Cookie 解析**：使用用户自己的 Bilibili Cookie 解析视频，支持大会员高画质。
- **视频流代理**：代理 Bilibili CDN 视频/音频流，注入正确的请求头，绕过 CORS 与防盗链。
- **CDN 自动切换**：单个 CDN 连接失败时，自动尝试备用 CDN 地址。
- **WebSocket 房间注册**：自动向 ZViewer 房间注册，前端可自动发现并启用本地代理。
- **二维码登录**：内置 Bilibili 二维码登录页面，无需手动复制 Cookie。
- **本地配置页面**：启动后自动打开浏览器配置页，填写后端地址、房间 ID、Cookie 即可连接。
- **断线自动重连**：与 ZViewer 后端断开连接后，使用指数退避策略自动重连。

---

## 工作原理

```text
┌─────────────────┐      HTTP / WebSocket      ┌─────────────────┐
│   ZViewer 后端   │  ◄──────────────────────►  │  ZViewerCLI     │
│  (房间状态同步)  │                            │  (用户本地运行)  │
└─────────────────┘                            └────────┬────────┘
       ▲                                                │
       │ Socket.IO 同步播放状态                          │ 本地解析/代理
       │                                                ▼
┌──────┴──────────┐                           ┌─────────────────┐
│   用户浏览器      │  ◄── 本地代理 URL ───────  │  Bilibili CDN   │
│  (ZViewer 前端)   │     (绕过 CORS)          │  (视频/音频流)   │
└─────────────────┘                           └─────────────────┘
```

核心流程：

1. 用户在本地启动 ZViewerCLI，配置 ZViewer 后端地址、房间 ID 和 Bilibili Cookie。
2. ZViewerCLI 通过 WebSocket 连接到 ZViewer 后端，向房间注册自己的代理地址。
3. 当前端检测到房间内有 CLI 代理可用时，优先将 Bilibili 视频解析请求发送给本地 CLI。
4. CLI 使用本地 Cookie 解析视频，返回代理后的视频/音频 URL。
5. 浏览器请求本地代理地址，CLI 再向上游 Bilibili CDN 请求真实数据并转发给浏览器。

---

## 快速开始

### 1. 获取可执行文件

从 Release 下载对应平台的二进制文件，或参考 [构建](#构建) 自行编译。

```text
dist/
├── zviewer-cli-windows-amd64.exe
├── zviewer-cli-darwin-amd64
├── zviewer-cli-darwin-arm64
├── zviewer-cli-linux-amd64
└── zviewer-cli-linux-arm64
```

### 2. 启动本地代理

#### Windows

```powershell
.\zviewer-cli-windows-amd64.exe
```

#### macOS / Linux

```bash
chmod +x zviewer-cli-darwin-arm64
./zviewer-cli-darwin-arm64
```

默认行为：

- 本地 HTTP 服务监听 `http://127.0.0.1:9333`
- 仅启动命令行，不自动打开浏览器（如需自动打开，添加 `-no-open=false`）

手动访问 `http://127.0.0.1:9333` 进入配置页面。

### 3. 配置并连接

在配置页面填写：

- **ZViewer 后端地址**：例如 `http://localhost:3333` 或 `https://your-domain.com`
- **房间 ID**：要加入的房间号
- **Bilibili Cookie**：可通过二维码登录自动获取，或手动粘贴

点击连接后，CLI 会验证 Cookie 并注册到房间。

### 4. 在房间中启用

进入 ZViewer 房间，在「Bilibili 解析设置」中开启「CLI 本地高画质代理」。此时播放器将通过本地 CLI 加载视频。

---

## 命令行参数

```text
用法:
  zviewer-cli [选项]

选项:
  -port int
        本地 HTTP 服务端口 (默认 9333)
  -server string
        ZViewer 后端地址
  -room string
        房间 ID
  -cookie string
        Bilibili Cookie
  -setup
        启动本地配置页面 (默认 true)
  -no-open
        不自动打开浏览器
  -help
        显示帮助
```

### 示例

```bash
# 仅启动本地配置页
./zviewer-cli

# 启动并自动连接指定房间
./zviewer-cli -server http://localhost:3333 -room abc123 -cookie "SESSDATA=xxx"

# 指定端口，不自动打开浏览器
./zviewer-cli -port 8080 -no-open
```

---

## 本地 HTTP API

CLI 启动后在本地监听 HTTP 请求，主要接口如下：

### 健康检查

```http
GET /health
```

### 获取当前配置与连接状态

```http
GET /api/config
```

### 配置并连接

```http
POST /api/connect
Content-Type: application/json

{
  "serverUrl": "http://localhost:3333",
  "roomId": "abc123",
  "cookie": "SESSDATA=xxx"
}
```

### 解析 Bilibili 视频

```http
GET /resolve?bvid=BVxxx&cid=123456&qn=120&preferMp4=false&forceDash=true
```

返回包含代理后的 `videoUrl` / `audioUrl` 以及原始 CDN 地址。

### 代理视频流

```http
GET /proxy?url=<url-encoded-bilibili-cdn-url>
```

支持 `Range` 请求头，适合 DASH 分段加载。

### 获取 Bilibili 视频信息

```http
GET /api/bili-info?bvid=BVxxx
```

### 生成 DASH MPD

```http
GET /api/dash-mpd?bvid=BVxxx&cid=123456&qn=120
```

### 二维码登录

```http
GET /api/qr
GET /api/qr/poll?qrcode_key=xxx
```

---

## 构建

### 环境要求

- Go 1.22.5 或更高版本
- （可选）[UPX](https://upx.github.io/) — 用于压缩可执行文件，减小体积

### 快速编译（不压缩）

```bash
# 克隆代码
cd ZViewerCLI

# 编译当前平台（调试/开发用）
go build -o zviewer-cli .

# 运行
./zviewer-cli
```

### 优化编译（推荐）

使用 `-ldflags="-s -w"` 和 `-trimpath` 移除调试信息与路径信息，显著减小体积：

```bash
go build -ldflags="-s -w" -trimpath -o zviewer-cli .
```

### 使用 UPX 压缩

[UPX](https://upx.github.io/) 是一个可执行文件压缩工具，能将二进制体积压缩至原来的 30% 左右，适合分发场景。

#### 安装 UPX

**Windows（推荐）**：从 [UPX Releases](https://github.com/upx/upx/releases) 下载 `upx-5.0.0-win64.zip`，解压后即可使用。

```powershell
# 下载并解压 UPX
curl -LJO https://github.com/upx/upx/releases/download/v5.0.0/upx-5.0.0-win64.zip
Expand-Archive -Path upx-5.0.0-win64.zip -DestinationPath upx
```

**macOS**：
```bash
brew install upx
```

**Linux**：
```bash
sudo apt install upx       # Debian/Ubuntu
sudo pacman -S upx          # Arch Linux
```

#### 压缩命令

基本压缩（Linux/macOS 即可）：
```bash
upx --lzma zviewer-cli
```

极限压缩（推荐 Windows 使用）：
```bash
upx --best --lzma zviewer-cli.exe
```

> 注意：UPX 不支持压缩 macOS 可执行文件，macOS 版本跳过 UPX 步骤。

### 交叉编译

使用 `GOOS`/`GOARCH` 环境变量为其他平台编译：

```bash
# Windows amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o zviewer-cli-windows-amd64.exe .

# Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o zviewer-cli-linux-amd64 .

# Linux arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o zviewer-cli-linux-arm64 .

# macOS Intel
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o zviewer-cli-darwin-amd64 .

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o zviewer-cli-darwin-arm64 .
```

交叉编译后使用 UPX 压缩（仅 Windows 和 Linux）：

```bash
# Windows 极限压缩
upx --best --lzma zviewer-cli-windows-amd64.exe

# Linux 正常压缩
upx --lzma zviewer-cli-linux-amd64
upx --lzma zviewer-cli-linux-arm64
```

### 一键编译脚本

项目提供了 `build.ps1` 脚本，自动完成所有平台的交叉编译与压缩：

```powershell
# 确保 Go 已安装，UPX 可选
.\build.ps1
```

脚本执行以下操作：

| 平台 | 压缩方式 | 预期大小 |
|------|---------|---------|
| Windows amd64 | UPX `--best --lzma`（极限压缩） | ~2.4 MB |
| Linux amd64 | UPX `--lzma`（正常压缩） | ~2.3 MB |
| Linux arm64 | UPX `--lzma`（正常压缩） | ~2.0 MB |
| macOS amd64 | 跳过 UPX | ~7.5 MB |
| macOS arm64 | 跳过 UPX | ~7.0 MB |

编译产物输出到 `%TEMP%\zviewer-cli-dist\` 目录。

> 脚本会自动查找 UPX：优先使用参数 `-UPXPath` 指定的路径，其次查找 PATH 环境变量，最后检查常见默认路径。也可通过 `.\build.ps1 -UPXPath "D:\tools\upx.exe"` 手动指定。

### CI/CD 自动构建

项目配置了 GitHub Actions 工作流（`.github/workflows/build.yml`），支持两种触发方式：

- **Tag 推送**：推送 `v*` 格式的 tag 时自动构建并创建 Release
- **手动触发**：在 GitHub Actions 页面手动运行，可选择创建 Draft Release

```bash
# 推送 tag 触发自动构建
git tag v0.1.0
git push origin v0.1.0
```

构建策略与本地脚本一致：

| 平台 | 压缩方式 | Runner |
|------|---------|--------|
| Windows amd64 | UPX `--best --lzma`（极限压缩） | `windows-latest` |
| Linux amd64 | UPX `--best --lzma`（极限压缩） | `ubuntu-latest` |
| Linux arm64 | UPX `--best --lzma`（极限压缩） | `ubuntu-latest`（交叉编译） |
| macOS amd64 | 不压缩 | `macos-13` |
| macOS arm64 | 不压缩 | `macos-14` |

> Windows 和 Linux 在 CI 中统一使用 `--best --lzma` 极限压缩；macOS 由于 UPX 不支持，跳过压缩步骤。

---

## 项目结构

```text
ZViewerCLI/
├── main.go           # 程序入口：参数解析、初始化、启动 HTTP 服务
├── server.go         # HTTP 服务与代理逻辑、WebSocket 连接管理
├── socket.go         # Socket.IO v4 最小化客户端实现
├── resolver.go       # Bilibili 视频解析、VIP 校验、缓存策略
├── bilibili.go       # Bilibili API 请求、二维码登录、Cookie 校验
├── wbi.go            # WBI 签名计算与缓存
├── mp4box.go         # MP4 box 解析与 DASH MPD 生成
├── page.go           # 本地配置页 HTML 与二维码页面
├── config.go         # 本地配置文件持久化
├── state.go          # 运行时状态管理
├── speedlog.go       # 代理流量日志
├── build.ps1         # 一键交叉编译脚本（含 UPX 压缩）
├── .github/workflows/ # GitHub Actions CI/CD 自动构建
├── go.mod            # Go 模块依赖
├── dist/             # 预编译的多平台二进制文件
└── m4s_head.bin      # m4s 初始化段模板
```

### 主要模块说明

| 文件 | 职责 |
|---|---|
| `main.go` | 命令行参数解析、加载本地配置、启动 HTTP 服务、自动连接 |
| `server.go` | 注册 HTTP 路由、处理 `/resolve` 与 `/proxy`、管理 CDN 备用缓存 |
| `socket.go` | 实现 Engine.IO/Socket.IO v4 客户端协议，向后端注册 CLI 代理 |
| `resolver.go` | 编排 Bilibili 解析流程：VIP 校验、视频信息、播放地址、MP4 降级 |
| `bilibili.go` | 封装 Bilibili HTTP 请求、二维码登录、Cookie 验证 |
| `wbi.go` | 获取并缓存 WBI 签名密钥，对请求参数进行签名 |
| `mp4box.go` | 解析 MP4 box 结构，为 DASH 播放生成 MPD 清单 |
| `config.go` | 读写 `~/.zviewer/config.json`，保存 Cookie 与用户信息 |

---

## 与 ZViewer 的关系

ZViewerCLI 是 [ZViewer](../ZViewer) 的可选配套组件，不依赖 CLI 也能正常使用基础解析功能。启用 CLI 后：

- 视频解析由用户本地 Cookie 完成，可获得个人大会员等高画质。
- 视频流通过本地代理，避免浏览器 CORS 与防盗链问题。
- 同一房间内多人共享房主/观众的 CLI 代理，提升播放稳定性。

ZViewer 主项目负责房间状态同步、用户管理、播放控制；ZViewerCLI 负责本地化的 Bilibili 解析与流代理。两者通过 WebSocket 与本地 HTTP API 协作。

---

## 常见问题

### 启动后浏览器未自动打开

默认不自动打开浏览器，请手动访问 `http://127.0.0.1:9333`。如需自动打开，启动时添加 `-no-open=false`。

### Cookie 验证失败

- 确认 Cookie 包含有效的 `SESSDATA`。
- 二维码登录获取的 Cookie 通常最稳定。
- 使用 `-cookie` 参数时注意 Shell 对特殊字符的转义。

### 已连接但前端未使用 CLI 代理

- 确认房间内「CLI 本地高画质代理」开关已开启。
- 确认前端与 CLI 连接的是同一个房间。
- 检查浏览器控制台是否有 CORS 或网络错误。

### 视频流加载卡顿

- CLI 会自动在主 CDN 失败时切换到备用 CDN。
- 检查本地网络到 Bilibili CDN 的连通性。
- 尝试降低清晰度（qn）再试。

### 多平台分发

预编译二进制位于 `dist/` 目录，也可使用 [构建](#构建) 中的交叉编译命令自行生成。
