package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"
	"github.com/skip2/go-qrcode"
)

const (
	version   = "0.1.0"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

const setupPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ZViewer CLI 配置</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --md-sys-shape-corner: 16px;
      --md-sys-color-surface: #0b1015;
      --md-sys-color-surface-container: #111920;
      --md-sys-color-surface-container-high: #19232c;
      --md-sys-color-surface-container-highest: #222e3a;
      --md-sys-color-primary: #5fa8f8;
      --md-sys-color-on-primary: #ffffff;
      --md-sys-color-primary-container: #00447d;
      --md-sys-color-on-primary-container: #cfe4ff;
      --md-sys-color-secondary: #8fcafc;
      --md-sys-color-on-secondary: #002e5f;
      --md-sys-color-tertiary: #a890f8;
      --md-sys-color-on-tertiary: #ffffff;
      --md-sys-color-on-surface: #e2e8f0;
      --md-sys-color-on-surface-variant: #94a3b8;
      --md-sys-color-outline: #475569;
      --md-sys-color-outline-variant: #334155;
      --md-sys-color-error: #f87171;
      --md-sys-color-on-error: #ffffff;
      --md-sys-color-error-container: #451515;
      --md-sys-color-success: #34d399;
      --glass-bg: rgba(17, 25, 32, 0.74);
      --glass-border: rgba(148, 163, 184, 0.16);
      --glass-blur: 18px;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      margin: 0;
      font-family: 'Bricolage Grotesque', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
      background:
        radial-gradient(ellipse 80% 60% at 50% -10%, color-mix(in srgb, var(--md-sys-color-primary) 22%, transparent), transparent),
        var(--md-sys-color-surface);
      color: var(--md-sys-color-on-surface);
      min-height: 100vh;
      display: flex;
      justify-content: center;
      padding: 40px 18px;
      line-height: 1.5;
    }
    .container {
      width: 100%;
      max-width: 520px;
      display: flex;
      flex-direction: column;
      gap: 20px;
    }
    .header {
      display: flex;
      align-items: center;
      gap: 16px;
      margin-bottom: 4px;
    }
    .icon-wrap {
      width: 48px;
      height: 48px;
      border-radius: var(--md-sys-shape-corner);
      display: flex;
      align-items: center;
      justify-content: center;
      flex-shrink: 0;
      background: linear-gradient(
        135deg,
        color-mix(in srgb, var(--md-sys-color-tertiary) 28%, transparent),
        color-mix(in srgb, var(--md-sys-color-secondary) 22%, transparent)
      );
      box-shadow: 0 4px 16px color-mix(in srgb, var(--md-sys-color-primary) 22%, transparent);
    }
    .icon-wrap svg { width: 26px; height: 26px; color: var(--md-sys-color-tertiary); }
    .header-text { display: flex; flex-direction: column; gap: 2px; }
    .header-title { font-size: 22px; font-weight: 700; color: var(--md-sys-color-on-surface); letter-spacing: -0.02em; }
    .header-subtitle { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em; color: var(--md-sys-color-on-surface-variant); }
    .card {
      background: var(--glass-bg);
      backdrop-filter: blur(var(--glass-blur));
      -webkit-backdrop-filter: blur(var(--glass-blur));
      border: 1px solid var(--glass-border);
      border-radius: var(--md-sys-shape-corner);
      padding: 24px;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.24);
    }
    .section-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--md-sys-color-on-surface);
      margin-bottom: 18px;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .section-title::before {
      content: '';
      width: 4px;
      height: 16px;
      border-radius: 999px;
      background: var(--md-sys-color-primary);
    }
    .field { margin-bottom: 18px; }
    .field:last-child { margin-bottom: 0; }
    .field label {
      display: block;
      font-size: 10px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      color: var(--md-sys-color-on-surface-variant);
      margin-bottom: 8px;
    }
    .input, .textarea {
      width: 100%;
      background: color-mix(in srgb, var(--md-sys-color-surface-container) 86%, transparent);
      border: 1px solid var(--md-sys-color-outline-variant);
      border-radius: 12px;
      padding: 12px 14px;
      color: var(--md-sys-color-on-surface);
      font-size: 14px;
      outline: none;
      transition: border-color 0.2s, box-shadow 0.2s, background-color 0.2s;
    }
    .input::placeholder, .textarea::placeholder { color: var(--md-sys-color-on-surface-variant); opacity: 0.6; }
    .input:focus, .textarea:focus {
      border-color: var(--md-sys-color-primary);
      background: color-mix(in srgb, var(--md-sys-color-surface-container) 96%, transparent);
      box-shadow: 0 0 0 3px color-mix(in srgb, var(--md-sys-color-primary) 16%, transparent);
    }
    .textarea { min-height: 90px; resize: vertical; font-family: 'SF Mono', 'Cascadia Mono', 'Fira Code', monospace; font-size: 12px; }
    .row { display: flex; gap: 12px; }
    .row .field { flex: 1; }
    .btn {
      width: 100%;
      border: none;
      border-radius: 12px;
      padding: 14px;
      font-size: 14px;
      font-weight: 600;
      cursor: pointer;
      transition: transform 0.15s cubic-bezier(0.34, 1.56, 0.64, 1), box-shadow 0.15s, opacity 0.15s;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
    }
    .btn:hover:not(:disabled) { transform: translateY(-1px); box-shadow: 0 6px 20px rgba(0, 0, 0, 0.22); }
    .btn:active:not(:disabled) { transform: translateY(0); }
    .btn:disabled { opacity: 0.5; cursor: not-allowed; }
    .btn-primary { background: var(--md-sys-color-primary); color: var(--md-sys-color-on-primary); }
    .btn-primary:hover:not(:disabled) { box-shadow: 0 6px 22px color-mix(in srgb, var(--md-sys-color-primary) 30%, transparent); }
    .btn-secondary {
      background: color-mix(in srgb, var(--md-sys-color-surface-container-highest) 70%, transparent);
      color: var(--md-sys-color-on-surface);
      border: 1px solid var(--glass-border);
    }
    .qr-wrap {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 16px;
      margin-top: 20px;
      padding: 20px;
      background: color-mix(in srgb, var(--md-sys-color-surface-container) 60%, transparent);
      border-radius: 12px;
      border: 1px dashed var(--md-sys-color-outline);
    }
    .qr-wrap img {
      width: 200px;
      height: 200px;
      border-radius: 12px;
      background: #fff;
      padding: 8px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
    }
    .status {
      font-size: 13px;
      padding: 12px 14px;
      border-radius: 12px;
      background: color-mix(in srgb, var(--md-sys-color-surface-container) 70%, transparent);
      border: 1px solid var(--glass-border);
      word-break: break-word;
    }
    .status.error { color: var(--md-sys-color-error); border-color: color-mix(in srgb, var(--md-sys-color-error) 40%, transparent); background: color-mix(in srgb, var(--md-sys-color-error-container) 80%, transparent); }
    .status.success { color: var(--md-sys-color-success); border-color: color-mix(in srgb, var(--md-sys-color-success) 40%, transparent); background: color-mix(in srgb, var(--md-sys-color-success) 10%, transparent); }
    .hidden { display: none !important; }
    .hint { font-size: 12px; color: var(--md-sys-color-on-surface-variant); margin-top: 12px; line-height: 1.6; }
    .cookie-state { font-size: 12px; font-weight: 500; margin-top: 8px; }
    .cookie-state.empty { color: var(--md-sys-color-on-surface-variant); }
    .cookie-state.valid { color: var(--md-sys-color-success); }
    .cookie-state.invalid { color: var(--md-sys-color-error); }
    .footer { text-align: center; font-size: 12px; color: var(--md-sys-color-on-surface-variant); margin-top: 8px; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="icon-wrap">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect>
          <line x1="8" y1="21" x2="16" y2="21"></line>
          <line x1="12" y1="17" x2="12" y2="21"></line>
        </svg>
      </div>
      <div class="header-text">
        <div class="header-title">ZViewer CLI</div>
        <div class="header-subtitle">本地高画质代理</div>
      </div>
    </div>

    <div class="card">
      <div class="section-title">连接配置</div>
      <div class="row">
        <div class="field">
          <label>后端地址</label>
          <input type="text" id="server" class="input" placeholder="http://localhost:3333" />
        </div>
        <div class="field">
          <label>房间 ID</label>
          <input type="text" id="room" class="input" placeholder="例如 abc123" />
        </div>
      </div>

      <div class="field">
        <label>B站 Cookie</label>
        <textarea id="cookie" class="textarea" placeholder="可手动粘贴 Cookie，或点击下方按钮扫码登录"></textarea>
        <div class="cookie-state empty" id="cookieState">未设置 Cookie</div>
      </div>

      <button class="btn btn-secondary" id="qrBtn" type="button">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"></rect><rect x="14" y="3" width="7" height="7"></rect><rect x="14" y="14" width="7" height="7"></rect><rect x="3" y="14" width="7" height="7"></rect></svg>
        扫码登录 B站
      </button>

      <div id="qrArea" class="qr-wrap hidden">
        <img id="qrImg" src="" alt="B站登录二维码" />
        <div class="status" id="qrStatus">请使用哔哩哔哩 App 扫码</div>
      </div>

      <div class="hint">扫码登录仅在本地处理 Cookie，不会上传到 ZViewer 服务器。</div>
    </div>

    <div class="card">
      <div class="section-title">房间连接</div>
      <button class="btn btn-primary" id="connectBtn" type="button">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"></path><path d="m12 5 7 7-7 7"></path></svg>
        连接房间
      </button>
      <div class="status hidden" id="connectStatus"></div>
      <div class="hint">连接成功后，可在网页端「B站解析设置」中启用「CLI 高画质代理」。</div>
    </div>

    <div class="footer">ZViewer CLI · 本地高画质代理客户端</div>
  </div>

  <script>
    const params = new URLSearchParams(location.search)
    const serverEl = document.getElementById('server')
    const roomEl = document.getElementById('room')
    const cookieEl = document.getElementById('cookie')
    const cookieStateEl = document.getElementById('cookieState')
    const qrBtn = document.getElementById('qrBtn')
    const qrArea = document.getElementById('qrArea')
    const qrImg = document.getElementById('qrImg')
    const qrStatus = document.getElementById('qrStatus')
    const connectBtn = document.getElementById('connectBtn')
    const connectStatus = document.getElementById('connectStatus')

    if (params.get('server')) serverEl.value = params.get('server')
    if (params.get('room')) roomEl.value = params.get('room')

    function setStatus(el, text, type) {
      el.textContent = text
      el.classList.remove('hidden', 'error', 'success')
      if (type) el.classList.add(type)
    }

    function updateCookieState(valid, userName) {
      const c = cookieEl.value.trim()
      cookieStateEl.classList.remove('empty', 'valid', 'invalid')
      if (!c) {
        cookieStateEl.textContent = '未设置 Cookie'
        cookieStateEl.classList.add('empty')
        return
      }
      if (valid === true) {
        cookieStateEl.textContent = userName ? '已设置 Cookie（有效 · ' + userName + '）' : '已设置 Cookie（有效）'
        cookieStateEl.classList.add('valid')
      } else if (valid === false) {
        cookieStateEl.textContent = '已设置 Cookie（已过期）'
        cookieStateEl.classList.add('invalid')
      } else {
        cookieStateEl.textContent = '已设置 Cookie'
        cookieStateEl.classList.add('valid')
      }
    }
    cookieEl.addEventListener('input', () => updateCookieState(null))

    let qrTimer = null
    let currentQrKey = null

    async function startQrLogin() {
      if (qrTimer) { clearTimeout(qrTimer); qrTimer = null }
      qrArea.classList.remove('hidden')
      qrImg.src = ''
      setStatus(qrStatus, '正在获取二维码…')
      qrBtn.disabled = true

      try {
        const res = await fetch('/api/qr')
        const data = await res.json()
        if (!data.success) throw new Error(data.message || '获取二维码失败')
        currentQrKey = data.qrcodeKey
        qrImg.src = data.qrDataUrl
        setStatus(qrStatus, '请使用哔哩哔哩 App 扫码')
        pollQr(data.qrcodeKey)
      } catch (err) {
        setStatus(qrStatus, err.message, 'error')
        qrBtn.disabled = false
      }
    }

    async function pollQr(key) {
      if (!key || key !== currentQrKey) return
      try {
        const res = await fetch('/api/qr/poll?qrcode_key=' + encodeURIComponent(key))
        const data = await res.json()
        if (!data.success) throw new Error(data.message || '轮询失败')

        if (data.status === 0) setStatus(qrStatus, '请使用哔哩哔哩 App 扫码')
        else if (data.status === 1) setStatus(qrStatus, '已扫码，请在 App 中确认登录')
        else if (data.status === 2) {
          setStatus(qrStatus, '登录成功' + (data.name ? '：' + data.name : ''), 'success')
          if (data.cookie) {
            cookieEl.value = data.cookie
            updateCookieState(data.cookieValid, data.name)
          }
          qrBtn.disabled = false
          return
        } else if (data.status === 3) {
          setStatus(qrStatus, '二维码已过期，请重新获取', 'error')
          qrBtn.disabled = false
          return
        }

        qrTimer = setTimeout(() => pollQr(key), 2000)
      } catch (err) {
        setStatus(qrStatus, err.message, 'error')
        qrBtn.disabled = false
      }
    }

    qrBtn.addEventListener('click', startQrLogin)

    connectBtn.addEventListener('click', async () => {
      const server = serverEl.value.trim()
      const room = roomEl.value.trim()
      const cookie = cookieEl.value.trim()

      if (!server) { setStatus(connectStatus, '请填写后端地址', 'error'); return }
      if (!room) { setStatus(connectStatus, '请填写房间 ID', 'error'); return }
      if (!cookie) { setStatus(connectStatus, '请先设置 B站 Cookie', 'error'); return }

      connectBtn.disabled = true
      setStatus(connectStatus, '正在连接…')

      try {
        const res = await fetch('/api/connect', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ serverUrl: server, roomId: room, cookie })
        })
        const data = await res.json()
        if (!res.ok || !data.success) throw new Error(data.message || '连接失败')
        setStatus(connectStatus, '已连接房间，可在网页启用 CLI 高画质代理', 'success')
      } catch (err) {
        setStatus(connectStatus, err.message, 'error')
      } finally {
        connectBtn.disabled = false
      }
    })

    fetch('/api/config')
      .then(r => r.json())
      .then(data => {
        if (!data.config) return
        if (data.config.serverUrl && !serverEl.value) serverEl.value = data.config.serverUrl
        if (data.config.roomId && !roomEl.value) roomEl.value = data.config.roomId
        if (data.config.cookie) {
          cookieEl.value = data.config.cookie
          updateCookieState(data.cookieValid, data.userInfo && data.userInfo.name)
        }
        if (data.connected) setStatus(connectStatus, '已连接房间', 'success')
      })
      .catch(() => {})
  </script>
</body>
</html>`

func init() {
	log.SetFlags(0)
}

func main() {
	var (
		port      int
		serverURL string
		roomID    string
		cookie    string
		setupMode bool
		noOpen    bool
		showHelp  bool
	)

	flag.IntVar(&port, "port", 9333, "本地 HTTP 服务端口")
	flag.StringVar(&serverURL, "server", "", "ZViewer 后端地址")
	flag.StringVar(&roomID, "room", "", "房间 ID")
	flag.StringVar(&cookie, "cookie", "", "B站 Cookie")
	flag.BoolVar(&setupMode, "setup", true, "启动本地配置页面")
	flag.BoolVar(&noOpen, "no-open", false, "不自动打开浏览器")
	flag.BoolVar(&showHelp, "help", false, "显示帮助")
	flag.Parse()

	if showHelp {
		printHelp()
		os.Exit(0)
	}

	logf("ZViewer CLI v%s", version)

	persisted, err := loadConfig()
	if err != nil {
		logf("加载本地配置失败: %v", err)
	}

	cfg := &LocalConfig{}
	if persisted != nil && persisted.Cookie != "" {
		cfg.Cookie = persisted.Cookie
	}

	if serverURL != "" {
		cfg.ServerURL = strings.TrimSpace(serverURL)
	}
	if roomID != "" {
		cfg.RoomID = strings.TrimSpace(roomID)
	}
	if cookie != "" {
		cfg.Cookie = strings.TrimSpace(cookie)
	}

	agent := newAgent(port, cfg)

	srv, err := agent.startHTTPServer()
	if err != nil {
		logf("启动 HTTP 服务失败: %v", err)
		os.Exit(1)
	}
	defer srv.Close()

	proxyURL := agent.proxyURL()
	logf("本地代理已启动: %s", proxyURL)

	if setupMode {
		u := fmt.Sprintf("http://127.0.0.1:%d", port)
		if cfg.ServerURL != "" {
			u += "?server=" + urlEncode(cfg.ServerURL)
		}
		if cfg.RoomID != "" {
			sep := "?"
			if strings.Contains(u, "?") {
				sep = "&"
			}
			u += sep + "room=" + urlEncode(cfg.RoomID)
		}
		if !noOpen {
			go openBrowser(u)
		}
	}

	if cfg.ServerURL != "" && cfg.RoomID != "" && cfg.Cookie != "" {
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := agent.doConnect(); err != nil {
				logf("自动连接失败: %v", err)
			}
		}()
	}

	select {}
}

func printHelp() {
	fmt.Println(`ZViewer CLI - 本地高画质代理客户端

用法:
  zviewer-cli [选项]

选项:`)
	flag.PrintDefaults()
	fmt.Println(`
示例:
  zviewer-cli                          # 启动本地配置页
  zviewer-cli --server http://localhost:3333 --room abc123 --cookie "..."`)
}

func urlEncode(s string) string {
	return strings.ReplaceAll(s, "&", "%26")
}

func openBrowser(u string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", u}
	case "darwin":
		cmd = "open"
		args = []string{u}
	default:
		cmd = "xdg-open"
		args = []string{u}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		logf("打开浏览器失败: %v", err)
	}
}

func nowISO() string {
	return time.Now().Format(time.RFC3339)
}

func logf(format string, args ...any) {
	fmt.Printf("[ZViewer CLI] "+format+"\n", args...)
}

// --------------------------- config ---------------------------

// PersistedConfig 是保存到本地的 CLI 配置。
type PersistedConfig struct {
	Cookie   string `json:"cookie"`
	SavedAt  string `json:"savedAt"`
	UserInfo *struct {
		Name      string `json:"name,omitempty"`
		Mid       int64  `json:"mid,omitempty"`
		VipStatus int    `json:"vipStatus,omitempty"`
	} `json:"userInfo,omitempty"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zcontrol-cli")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func loadConfig() (*PersistedConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg PersistedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Cookie == "" {
		return nil, nil
	}
	return &cfg, nil
}

func saveConfig(cfg *PersistedConfig) error {
	if cfg == nil {
		return nil
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o600)
}

func clearConfig() error {
	if err := os.Remove(configPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// --------------------------- bilibili ---------------------------

// QRSession 保存一次二维码登录会话。
type QRSession struct {
	QrcodeKey string `json:"qrcodeKey"`
	QrURL     string `json:"qrUrl"`
	QrDataURL string `json:"qrDataUrl"`
	CreatedAt int64  `json:"createdAt"`
}

// QRPollResult 是轮询结果。
type QRPollResult struct {
	Status   int    `json:"status"`
	Message  string `json:"message"`
	Cookie   string `json:"cookie,omitempty"`
	LoggedIn bool   `json:"loggedIn"`
	Name     string `json:"name,omitempty"`
}

// UserValidation Cookie 校验结果。
type UserValidation struct {
	Valid     bool   `json:"valid"`
	Name      string `json:"name,omitempty"`
	Mid       int64  `json:"mid,omitempty"`
	VipStatus int    `json:"vipStatus,omitempty"`
}

func bilibiliHeaders(cookie string) map[string]string {
	h := map[string]string{
		"User-Agent": userAgent,
		"Referer":    "https://www.bilibili.com",
	}
	if cookie != "" {
		h["Cookie"] = cookie
	}
	return h
}

func bilibiliGet(api string, cookie string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range bilibiliHeaders(cookie) {
		req.Header.Set(k, v)
	}
	return http.DefaultClient.Do(req)
}

func bilibiliJSON(api string, cookie string, out any) error {
	res, err := bilibiliGet(api, cookie)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func generateQRCode() (*QRSession, error) {
	api := "https://passport.bilibili.com/x/passport-login/web/qrcode/generate"
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    *struct {
			URL       string `json:"url"`
			QrcodeKey string `json:"qrcode_key"`
		} `json:"data"`
	}
	if err := bilibiliJSON(api, "", &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 || payload.Data == nil || payload.Data.QrcodeKey == "" {
		return nil, fmt.Errorf("生成二维码失败: %s", payload.Message)
	}
	png, err := qrcode.Encode(payload.Data.URL, qrcode.Medium, 256)
	if err != nil {
		return nil, err
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	return &QRSession{
		QrcodeKey: payload.Data.QrcodeKey,
		QrURL:     payload.Data.URL,
		QrDataURL: dataURL,
		CreatedAt: time.Now().UnixMilli(),
	}, nil
}

func pollQRStatus(key string) (*QRPollResult, error) {
	api := "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key=" + url.QueryEscape(key)
	res, err := bilibiliGet(api, "")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	var payload struct {
		Data *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  int    `json:"status"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	data := payload.Data
	if data == nil {
		return nil, fmt.Errorf("轮询返回异常")
	}

	status := data.Status
	if data.Code == 0 && data.URL != "" {
		status = 2
	} else if data.Code == 86101 {
		status = 0
	} else if data.Code == 86090 {
		status = 1
	} else if data.Code == 86038 {
		status = 3
	}

	result := &QRPollResult{Status: status, Message: data.Message}
	if status == 2 {
		cookie, err := fetchCookiesFromSsoURL(data.URL)
		if err != nil || cookie == "" {
			cookie = parseSetCookieHeader(res.Header)
		}
		if cookie != "" {
			result.Cookie = cookie
			result.LoggedIn = true
			return result, nil
		}
		result.Message = "登录确认成功，但未能获取 Cookie"
	}
	return result, nil
}

func fetchCookiesFromSsoURL(ssoURL string) (string, error) {
	cookieMap := make(map[string]string)
	current := ssoURL
	seen := make(map[string]bool)
	const maxRedirects = 10

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for i := 0; i <= maxRedirects; i++ {
		if seen[current] {
			break
		}
		seen[current] = true

		req, err := http.NewRequest(http.MethodGet, current, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Referer", "https://www.bilibili.com")
		if len(cookieMap) > 0 {
			req.Header.Set("Cookie", cookieMapToString(cookieMap))
		}

		res, err := client.Do(req)
		if err != nil {
			return "", err
		}
		for _, c := range res.Cookies() {
			if c.Name != "" {
				cookieMap[c.Name] = c.Value
			}
		}
		loc := res.Header.Get("Location")
		res.Body.Close()
		if loc == "" || res.StatusCode < 300 || res.StatusCode >= 400 {
			break
		}
		u, err := url.Parse(loc)
		if err != nil {
			break
		}
		if u.IsAbs() {
			current = loc
		} else {
			base, _ := url.Parse(current)
			current = base.ResolveReference(u).String()
		}
	}

	required := []string{"SESSDATA", "bili_jct", "DedeUserID"}
	missing := []string{}
	for _, r := range required {
		if _, ok := cookieMap[r]; !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("sso cookie missing: %s", strings.Join(missing, ", "))
	}
	return cookieMapToString(cookieMap), nil
}

func parseSetCookieHeader(h http.Header) string {
	parts := []string{}
	for _, raw := range h.Values("Set-Cookie") {
		for _, c := range strings.Split(raw, ",") {
			c = strings.TrimSpace(c)
			if idx := strings.Index(c, ";"); idx >= 0 {
				c = c[:idx]
			}
			if strings.Contains(c, "=") {
				parts = append(parts, c)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func cookieMapToString(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func validateCookie(cookie string) (*UserValidation, error) {
	api := "https://api.bilibili.com/x/web-interface/nav"
	res, err := bilibiliGet(api, cookie)
	if err != nil {
		return &UserValidation{Valid: false}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &UserValidation{Valid: false}, nil
	}
	var payload struct {
		Data *struct {
			IsLogin   bool   `json:"isLogin"`
			Uname     string `json:"uname"`
			Mid       int64  `json:"mid"`
			VipStatus int    `json:"vipStatus"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return &UserValidation{Valid: false}, err
	}
	if payload.Data == nil || !payload.Data.IsLogin {
		return &UserValidation{Valid: false}, nil
	}
	return &UserValidation{
		Valid:     true,
		Name:      payload.Data.Uname,
		Mid:       payload.Data.Mid,
		VipStatus: payload.Data.VipStatus,
	}, nil
}

// --------------------------- socket ---------------------------

// SocketClient 是一个最小化的 Socket.IO v4 客户端。
type SocketClient struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	serverURL string
	roomID    string
	proxyURL  string
	state     *State
}

// State 代理状态，用于服务端读取。
type State struct {
	mu          sync.RWMutex
	Connected   bool           `json:"connected"`
	Connecting  bool           `json:"connecting"`
	Config      *LocalConfig   `json:"config"`
	UserInfo    map[string]any `json:"userInfo"`
	CookieValid *bool          `json:"cookieValid"`
	LastError   string         `json:"lastError"`
}

func (s *State) SetConnected(v bool) {
	s.mu.Lock()
	s.Connected = v
	s.Connecting = false
	s.mu.Unlock()
}

func (s *State) SetConnecting(v bool) {
	s.mu.Lock()
	s.Connecting = v
	s.mu.Unlock()
}

func (s *State) SetConfig(c *LocalConfig) {
	s.mu.Lock()
	s.Config = c
	s.mu.Unlock()
}

func (s *State) SetCookieValid(v bool) {
	s.mu.Lock()
	b := v
	s.CookieValid = &b
	s.mu.Unlock()
}

func (s *State) SetUserInfo(info map[string]any) {
	s.mu.Lock()
	s.UserInfo = info
	s.mu.Unlock()
}

func (s *State) SetLastError(err string) {
	s.mu.Lock()
	s.LastError = err
	s.mu.Unlock()
}

func (s *State) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := map[string]any{
		"connected":  s.Connected,
		"connecting": s.Connecting,
		"lastError":  s.LastError,
		"config":     s.Config,
		"userInfo":   s.UserInfo,
	}
	if s.CookieValid != nil {
		m["cookieValid"] = *s.CookieValid
	} else {
		m["cookieValid"] = nil
	}
	return m
}

// LocalConfig 保存连接配置。
type LocalConfig struct {
	ServerURL string `json:"serverUrl"`
	RoomID    string `json:"roomId"`
	Cookie    string `json:"cookie"`
}

func toWebSocketURL(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("不支持的服务器协议: %s", u.Scheme)
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	if u.Path == "/" || u.Path == "" {
		u.Path = "/socket.io/"
	}
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func connectSocket(serverURL, roomID, proxyURL string, state *State) (*SocketClient, error) {
	wsURL, err := toWebSocketURL(serverURL)
	if err != nil {
		return nil, err
	}
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return nil, err
	}

	c := &SocketClient{
		conn:      conn,
		serverURL: serverURL,
		roomID:    roomID,
		proxyURL:  proxyURL,
		state:     state,
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if len(msg) == 0 || msg[0] != '0' {
		conn.Close()
		return nil, fmt.Errorf("未收到 Engine.IO open 包")
	}

	if err := c.writeText(`40{"agent":"zcontrol-cli"}`); err != nil {
		conn.Close()
		return nil, err
	}

	go c.readLoop()
	return c, nil
}

func (c *SocketClient) writeText(s string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, []byte(s))
}

func (c *SocketClient) emit(event string, payload any) error {
	arr := []any{event, payload}
	data, err := json.Marshal(arr)
	if err != nil {
		return err
	}
	return c.writeText("42" + string(data))
}

func (c *SocketClient) register() error {
	return c.emit("cli-register", map[string]any{
		"roomId":   c.roomID,
		"proxyUrl": c.proxyURL,
		"agent":    "zcontrol-cli",
		"version":  version,
	})
}

func (c *SocketClient) Close() {
	c.conn.Close()
}

func (c *SocketClient) readLoop() {
	defer c.conn.Close()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			logf("Socket 断开: %v", err)
			c.state.SetConnected(false)
			return
		}
		if len(msg) == 0 {
			continue
		}
		c.handleMessage(string(msg))
	}
}

func (c *SocketClient) handleMessage(msg string) {
	if len(msg) < 2 {
		return
	}
	engineType := msg[0]
	rest := msg[1:]

	switch engineType {
	case '2': // ping
		_ = c.writeText("3")
	case '3': // pong
		// ignore
	case '4': // message
		c.handleSocketPacket(rest)
	}
}

func (c *SocketClient) handleSocketPacket(rest string) {
	if len(rest) == 0 {
		return
	}
	socketType := rest[0]
	payload := rest[1:]

	switch socketType {
	case '0': // connect
		logf("Socket 已连接: %s", c.conn.RemoteAddr())
		c.state.SetConnected(true)
		if err := c.register(); err != nil {
			logf("cli-register 发送失败: %v", err)
		}
	case '2': // event
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(payload), &arr); err != nil {
			return
		}
		if len(arr) < 1 {
			return
		}
		var event string
		if err := json.Unmarshal(arr[0], &event); err != nil {
			return
		}
		var data json.RawMessage
		if len(arr) > 1 {
			data = arr[1]
		}
		c.handleEvent(event, data)
	}
}

func (c *SocketClient) handleEvent(event string, data json.RawMessage) {
	switch event {
	case "cli-registered":
		logf("已注册到房间: %s", c.roomID)
	case "watch-together-state":
		var s map[string]any
		_ = json.Unmarshal(data, &s)
		source := ""
		if v, ok := s["sourceUrl"].(string); ok {
			source = v
			if len(source) > 60 {
				source = source[:60]
			}
		}
		logf("收到房主状态: currentTime=%v isPlaying=%v sourceUrl=%s", s["currentTime"], s["isPlaying"], source)
	case "cli-error":
		var m map[string]string
		_ = json.Unmarshal(data, &m)
		logf("CLI 错误: %s", m["message"])
	}
}

// --------------------------- server ---------------------------

// Agent 持有运行时的配置、状态和 Socket 连接。
type Agent struct {
	port    int
	state   *State
	socket  *SocketClient
	qrCache *QRSession
}

func newAgent(port int, cfg *LocalConfig) *Agent {
	a := &Agent{
		port:  port,
		state: &State{},
	}
	if cfg != nil {
		a.state.SetConfig(cfg)
	}
	return a
}

func (a *Agent) proxyURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", a.port)
}

func (a *Agent) setConfig(cfg *LocalConfig) {
	a.state.SetConfig(cfg)
}

func (a *Agent) doConnect() error {
	cfg := a.state.Config
	if cfg == nil || cfg.ServerURL == "" || cfg.RoomID == "" || cfg.Cookie == "" {
		return fmt.Errorf("配置不完整")
	}
	if a.state.Connected || a.state.Connecting {
		return fmt.Errorf("已在连接或已连接")
	}

	a.state.SetConnecting(true)
	a.state.SetLastError("")

	validation, err := validateCookie(cfg.Cookie)
	if err != nil {
		a.state.SetConnecting(false)
		return fmt.Errorf("校验 Cookie 失败: %w", err)
	}
	a.state.SetCookieValid(validation.Valid)
	if !validation.Valid {
		_ = clearConfig()
		a.state.SetConnecting(false)
		return fmt.Errorf("B站 Cookie 已过期或无效，请重新扫码登录")
	}
	a.state.SetUserInfo(map[string]any{
		"name":      validation.Name,
		"mid":       validation.Mid,
		"vipStatus": validation.VipStatus,
	})
	_ = saveConfig(&PersistedConfig{
		Cookie:  cfg.Cookie,
		SavedAt: nowISO(),
		UserInfo: &struct {
			Name      string `json:"name,omitempty"`
			Mid       int64  `json:"mid,omitempty"`
			VipStatus int    `json:"vipStatus,omitempty"`
		}{Name: validation.Name, Mid: validation.Mid, VipStatus: validation.VipStatus},
	})

	client, err := connectSocket(cfg.ServerURL, cfg.RoomID, a.proxyURL(), a.state)
	if err != nil {
		a.state.SetConnecting(false)
		a.state.SetLastError(err.Error())
		return err
	}
	a.socket = client
	return nil
}

func (a *Agent) disconnect() {
	if a.socket != nil {
		a.socket.Close()
		a.socket = nil
	}
	a.state.SetConnected(false)
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *Agent) setupServer() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(setupPageHTML))
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]any{
			"ok":      true,
			"agent":   "zcontrol-cli",
			"version": version,
		})
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			jsonResponse(w, http.StatusOK, map[string]any{
				"success":     true,
				"config":      a.state.Config,
				"connected":   a.state.Connected,
				"connecting":  a.state.Connecting,
				"userInfo":    a.state.UserInfo,
				"cookieValid": a.state.CookieValid,
			})
		case http.MethodPost:
			var body struct {
				ServerURL string `json:"serverUrl"`
				RoomID    string `json:"roomId"`
				Cookie    string `json:"cookie"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "请求体解析失败"})
				return
			}
			if strings.TrimSpace(body.ServerURL) == "" || strings.TrimSpace(body.RoomID) == "" {
				jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 serverUrl 或 roomId"})
				return
			}
			cfg := &LocalConfig{
				ServerURL: strings.TrimSpace(body.ServerURL),
				RoomID:    strings.TrimSpace(body.RoomID),
				Cookie:    strings.TrimSpace(body.Cookie),
			}
			if cfg.Cookie == "" && a.state.Config != nil {
				cfg.Cookie = a.state.Config.Cookie
			}
			a.setConfig(cfg)

			if cfg.Cookie != "" {
				validation, err := validateCookie(cfg.Cookie)
				if err == nil {
					a.state.SetCookieValid(validation.Valid)
					if validation.Valid {
						a.state.SetUserInfo(map[string]any{
							"name":      validation.Name,
							"mid":       validation.Mid,
							"vipStatus": validation.VipStatus,
						})
						_ = saveConfig(&PersistedConfig{
							Cookie:  cfg.Cookie,
							SavedAt: nowISO(),
							UserInfo: &struct {
								Name      string `json:"name,omitempty"`
								Mid       int64  `json:"mid,omitempty"`
								VipStatus int    `json:"vipStatus,omitempty"`
							}{Name: validation.Name, Mid: validation.Mid, VipStatus: validation.VipStatus},
						})
					}
				}
			}
			jsonResponse(w, http.StatusOK, map[string]any{
				"success":     true,
				"config":      a.state.Config,
				"cookieValid": a.state.CookieValid,
				"userInfo":    a.state.UserInfo,
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/qr", func(w http.ResponseWriter, r *http.Request) {
		session, err := generateQRCode()
		if err != nil {
			logf("生成二维码失败: %v", err)
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
			return
		}
		a.qrCache = session
		jsonResponse(w, http.StatusOK, map[string]any{
			"success":   true,
			"qrcodeKey": session.QrcodeKey,
			"qrUrl":     session.QrURL,
			"qrDataUrl": session.QrDataURL,
		})
	})

	mux.HandleFunc("/api/qr/poll", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("qrcode_key")
		if key == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 qrcode_key"})
			return
		}
		result, err := pollQRStatus(key)
		if err != nil {
			logf("轮询二维码失败: %v", err)
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
			return
		}
		if result.LoggedIn && result.Cookie != "" && a.state.Config != nil {
			a.state.Config.Cookie = result.Cookie
			validation, _ := validateCookie(result.Cookie)
			a.state.SetCookieValid(validation.Valid)
			if validation.Valid {
				a.state.SetUserInfo(map[string]any{
					"name":      validation.Name,
					"mid":       validation.Mid,
					"vipStatus": validation.VipStatus,
				})
				_ = saveConfig(&PersistedConfig{
					Cookie:  result.Cookie,
					SavedAt: nowISO(),
					UserInfo: &struct {
						Name      string `json:"name,omitempty"`
						Mid       int64  `json:"mid,omitempty"`
						VipStatus int    `json:"vipStatus,omitempty"`
					}{Name: validation.Name, Mid: validation.Mid, VipStatus: validation.VipStatus},
				})
			}
		}
		name := ""
		if a.state.UserInfo != nil {
			if n, ok := a.state.UserInfo["name"].(string); ok {
				name = n
			}
		}
		valid := false
		if a.state.CookieValid != nil {
			valid = *a.state.CookieValid
		}
		jsonResponse(w, http.StatusOK, map[string]any{
			"success":     true,
			"status":      result.Status,
			"message":     result.Message,
			"cookie":      result.Cookie,
			"loggedIn":    result.LoggedIn,
			"name":        name,
			"cookieValid": valid,
		})
	})

	mux.HandleFunc("/api/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ServerURL string `json:"serverUrl"`
			RoomID    string `json:"roomId"`
			Cookie    string `json:"cookie"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "请求体解析失败"})
			return
		}
		if strings.TrimSpace(body.ServerURL) == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少后端地址"})
			return
		}
		if strings.TrimSpace(body.RoomID) == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少房间 ID"})
			return
		}
		if strings.TrimSpace(body.Cookie) == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 B站 Cookie"})
			return
		}
		a.setConfig(&LocalConfig{
			ServerURL: strings.TrimSpace(body.ServerURL),
			RoomID:    strings.TrimSpace(body.RoomID),
			Cookie:    strings.TrimSpace(body.Cookie),
		})

		if err := a.doConnect(); err != nil {
			a.state.SetLastError(err.Error())
			logf("连接房间失败: %v", err)
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "Cookie") {
				status = http.StatusUnauthorized
			}
			jsonResponse(w, status, map[string]any{"success": false, "message": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"success": true, "message": "连接成功"})
	})

	mux.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		bvid := q.Get("bvid")
		cid := q.Get("cid")
		qn := q.Get("qn")
		preferMp4 := q.Get("preferMp4") == "true"
		if bvid == "" || cid == "" {
			http.Error(w, `{"error":"缺少 bvid 或 cid"}`, http.StatusBadRequest)
			return
		}
		if a.state.Config == nil || a.state.Config.Cookie == "" {
			http.Error(w, `{"error":"未设置 B站 Cookie"}`, http.StatusBadRequest)
			return
		}
		if a.state.Config.ServerURL == "" {
			http.Error(w, `{"error":"未设置后端地址"}`, http.StatusBadRequest)
			return
		}
		u, err := url.Parse(a.state.Config.ServerURL)
		if err != nil {
			http.Error(w, `{"error":"后端地址无效"}`, http.StatusBadRequest)
			return
		}
		u.Path = "/api/cli/resolve"
		qq := u.Query()
		qq.Set("bvid", bvid)
		qq.Set("cid", cid)
		if qn != "" {
			qq.Set("qn", qn)
		}
		qq.Set("preferMp4", strconv.FormatBool(preferMp4))
		u.RawQuery = qq.Encode()

		req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
		req.Header.Set("Cookie", a.state.Config.Cookie)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			logf("解析失败: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	})

	mux.HandleFunc("/proxy", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("url")
		if target == "" {
			http.Error(w, `{"error":"缺少 url 参数"}`, http.StatusBadRequest)
			return
		}
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Referer", "https://www.bilibili.com")
		req.Header.Set("Origin", "https://www.bilibili.com")
		req.Header.Set("User-Agent", userAgent)
		if rng := r.Header.Get("Range"); rng != "" {
			req.Header.Set("Range", rng)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			logf("代理失败: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()
		for _, key := range []string{"Content-Type", "Content-Length", "Accept-Ranges", "Content-Range"} {
			if v := res.Header.Get(key); v != "" {
				w.Header().Set(key, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	})

	return mux
}

func (a *Agent) startHTTPServer() (*http.Server, error) {
	mux := a.setupServer()
	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
		AllowedHeaders:   []string{"Range", "Content-Type", "Authorization"},
		ExposedHeaders:   []string{"Content-Range", "Accept-Ranges", "Content-Length"},
	}).Handler(mux)

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", a.port),
		Handler: handler,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logf("HTTP 服务异常: %v", err)
		}
	}()
	return srv, nil
}
