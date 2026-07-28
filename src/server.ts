import express from "express";
import cors from "cors";
import fetch from "node-fetch";
import type { Server } from "http";
import { createServer } from "http";
import { generateQrCode, pollQrStatus, validateCookie } from "./setup";
import { savePersistedConfig, clearPersistedConfig } from "./config";

export interface LocalProxyOptions {
  port: number;
  /** 初始后端地址（setup 模式下可被页面覆盖） */
  serverUrl?: string;
  /** 初始房间 ID（setup 模式下可被页面覆盖） */
  roomId?: string;
  /** 初始 B站 Cookie（setup 模式下可被页面覆盖） */
  cookie?: string;
}

export interface LocalProxyConfig {
  serverUrl: string;
  roomId: string;
  cookie: string;
}

export interface LocalProxyState {
  config: LocalProxyConfig | null;
  connected: boolean;
  connecting: boolean;
  lastError: string | null;
  userInfo: { name?: string; mid?: number; vipStatus?: number } | null;
  /** Cookie 是否校验通过；null 表示尚未校验 */
  cookieValid: boolean | null;
}

export interface LocalProxyResult {
  server: Server;
  url: string;
  state: LocalProxyState;
  connect: () => Promise<void>;
}

export type OnConnectCallback = (config: LocalProxyConfig) => Promise<void>;

const DEFAULT_USER_AGENT =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

const QR_TTL_MS = 180000; // B站 二维码默认 3 分钟有效

function renderSetupPage(): string {
  return `<!DOCTYPE html>
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
      /* Material You 暗色主题 token（种子色 #0066cc） */
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
</html>`;
}

export async function createLocalProxy(
  options: LocalProxyOptions,
  onConnect?: OnConnectCallback,
): Promise<LocalProxyResult> {
  const { port } = options;
  const app = express();
  const server = createServer(app);

  const state: LocalProxyState = {
    config:
      options.serverUrl && options.roomId
        ? {
            serverUrl: options.serverUrl,
            roomId: options.roomId,
            cookie: options.cookie || "",
          }
        : null,
    connected: false,
    connecting: false,
    lastError: null,
    userInfo: null,
    cookieValid: null,
  };

  // 如果启动时已提供 cookie，立即校验一次并记录用户信息
  if (state.config?.cookie) {
    validateCookie(state.config.cookie).then((validation) => {
      state.cookieValid = validation.valid;
      if (validation.valid) {
        state.userInfo = {
          name: validation.name,
          mid: validation.mid,
          vipStatus: validation.vipStatus,
        };
      }
    });
  }

  // 允许浏览器跨域访问本地代理
  app.use(
    cors({
      origin: true,
      credentials: true,
      allowedHeaders: ["Range", "Content-Type", "Authorization"],
      exposedHeaders: ["Content-Range", "Accept-Ranges", "Content-Length"],
    }),
  );
  app.use(express.json());

  // 配置首页：setup 向导
  app.get("/", (_req, res) => {
    res.setHeader("Content-Type", "text/html; charset=utf-8");
    res.send(renderSetupPage());
  });

  // 健康检查：浏览器用此接口判断 CLI 是否在线
  app.get("/health", (_req, res) => {
    res.json({ ok: true, agent: "zcontrol-cli", version: "0.1.0" });
  });

  // 读取当前配置与连接状态
  app.get("/api/config", (_req, res) => {
    res.json({
      success: true,
      config: state.config,
      connected: state.connected,
      connecting: state.connecting,
      userInfo: state.userInfo,
      cookieValid: state.cookieValid,
    });
  });

  // 保存配置（不连接）
  app.post("/api/config", async (req, res) => {
    const { serverUrl, roomId, cookie } = req.body || {};
    if (typeof serverUrl !== "string" || typeof roomId !== "string") {
      res
        .status(400)
        .json({ success: false, message: "缺少 serverUrl 或 roomId" });
      return;
    }
    const trimmedCookie =
      typeof cookie === "string" ? cookie.trim() : (state.config?.cookie ?? "");

    state.config = {
      serverUrl: serverUrl.trim(),
      roomId: roomId.trim(),
      cookie: trimmedCookie,
    };

    // 如果提供了新的 cookie，校验并持久化保存
    if (trimmedCookie) {
      const validation = await validateCookie(trimmedCookie);
      state.cookieValid = validation.valid;
      if (validation.valid) {
        state.userInfo = {
          name: validation.name,
          mid: validation.mid,
          vipStatus: validation.vipStatus,
        };
        await savePersistedConfig({
          cookie: trimmedCookie,
          userInfo: state.userInfo,
          savedAt: new Date().toISOString(),
        });
      }
    }

    res.json({
      success: true,
      config: state.config,
      cookieValid: state.cookieValid,
      userInfo: state.userInfo,
    });
  });

  // 生成 B站 登录二维码
  let currentQrSession: Awaited<ReturnType<typeof generateQrCode>> | null =
    null;
  app.get("/api/qr", async (_req, res) => {
    try {
      currentQrSession = await generateQrCode();
      res.json({
        success: true,
        qrcodeKey: currentQrSession.qrcodeKey,
        qrUrl: currentQrSession.qrUrl,
        qrDataUrl: currentQrSession.qrDataUrl,
      });
    } catch (err) {
      console.error("[ZViewer CLI] 生成二维码失败:", err);
      res.status(500).json({
        success: false,
        message: err instanceof Error ? err.message : "生成二维码失败",
      });
    }
  });

  // 轮询二维码状态
  app.get("/api/qr/poll", async (req, res) => {
    const key = req.query.qrcode_key as string | undefined;
    if (!key) {
      res.status(400).json({ success: false, message: "缺少 qrcode_key" });
      return;
    }
    try {
      const result = await pollQrStatus(key);
      if (result.loggedIn && result.cookie && state.config) {
        state.config.cookie = result.cookie;
        const validation = await validateCookie(result.cookie);
        state.cookieValid = validation.valid;
        if (validation.valid) {
          state.userInfo = {
            name: validation.name,
            mid: validation.mid,
            vipStatus: validation.vipStatus,
          };
          await savePersistedConfig({
            cookie: result.cookie,
            userInfo: state.userInfo,
            savedAt: new Date().toISOString(),
          });
        }
      }
      res.json({
        success: true,
        status: result.status,
        message: result.message,
        cookie: result.cookie,
        loggedIn: result.loggedIn,
        name: state.userInfo?.name,
        cookieValid: state.cookieValid,
      });
    } catch (err) {
      console.error("[ZViewer CLI] 轮询二维码失败:", err);
      res.status(500).json({
        success: false,
        message: err instanceof Error ? err.message : "轮询二维码失败",
      });
    }
  });

  // 连接房间：校验配置并触发 agent 连接
  app.post("/api/connect", async (req, res) => {
    if (state.connecting || state.connected) {
      res.status(400).json({ success: false, message: "已在连接或已连接" });
      return;
    }

    const { serverUrl, roomId, cookie } = req.body || {};
    if (typeof serverUrl !== "string" || !serverUrl.trim()) {
      res.status(400).json({ success: false, message: "缺少后端地址" });
      return;
    }
    if (typeof roomId !== "string" || !roomId.trim()) {
      res.status(400).json({ success: false, message: "缺少房间 ID" });
      return;
    }
    if (typeof cookie !== "string" || !cookie.trim()) {
      res.status(400).json({ success: false, message: "缺少 B站 Cookie" });
      return;
    }

    const trimmedCookie = cookie.trim();
    state.config = {
      serverUrl: serverUrl.trim(),
      roomId: roomId.trim(),
      cookie: trimmedCookie,
    };

    // 连接前强制校验 Cookie，过期则拒绝连接并清除本地持久化配置
    const validation = await validateCookie(trimmedCookie);
    state.cookieValid = validation.valid;
    if (validation.valid) {
      state.userInfo = {
        name: validation.name,
        mid: validation.mid,
        vipStatus: validation.vipStatus,
      };
      await savePersistedConfig({
        cookie: trimmedCookie,
        userInfo: state.userInfo,
        savedAt: new Date().toISOString(),
      });
    } else {
      await clearPersistedConfig();
      res.status(401).json({
        success: false,
        message: "B站 Cookie 已过期或无效，请重新扫码登录",
      });
      return;
    }

    if (!onConnect) {
      res.status(500).json({ success: false, message: "CLI 未配置连接回调" });
      return;
    }

    state.connecting = true;
    state.lastError = null;

    try {
      await onConnect(state.config);
      state.connected = true;
      res.json({ success: true, message: "连接成功" });
    } catch (err) {
      state.connected = false;
      state.lastError = err instanceof Error ? err.message : String(err);
      console.error("[ZViewer CLI] 连接房间失败:", err);
      res.status(500).json({
        success: false,
        message: state.lastError,
      });
    } finally {
      state.connecting = false;
    }
  });

  // 解析视频：把请求转发给后端 /api/cli/resolve，携带用户 Cookie
  app.get("/resolve", async (req, res) => {
    const bvid = req.query.bvid as string | undefined;
    const cid = req.query.cid as string | undefined;
    const qn = req.query.qn as string | undefined;
    const preferMp4 = req.query.preferMp4 === "true";
    const cookie = state.config?.cookie;

    if (!bvid || !cid) {
      res.status(400).json({ error: "缺少 bvid 或 cid" });
      return;
    }
    if (!cookie) {
      res.status(400).json({ error: "未设置 B站 Cookie" });
      return;
    }
    if (!state.config?.serverUrl) {
      res.status(400).json({ error: "未设置后端地址" });
      return;
    }

    try {
      const resolveUrl = new URL("/api/cli/resolve", state.config.serverUrl);
      resolveUrl.searchParams.set("bvid", bvid);
      resolveUrl.searchParams.set("cid", cid);
      if (qn) resolveUrl.searchParams.set("qn", qn);
      resolveUrl.searchParams.set("preferMp4", String(preferMp4));

      const response = await fetch(resolveUrl.toString(), {
        headers: {
          Cookie: cookie,
          "Content-Type": "application/json",
        },
      });

      if (!response.ok) {
        const text = await response.text();
        res.status(response.status).json({ error: text });
        return;
      }

      const data = (await response.json()) as unknown;
      res.json(data);
    } catch (err) {
      console.error("[ZViewer CLI] 解析失败:", err);
      res
        .status(500)
        .json({ error: err instanceof Error ? err.message : "解析失败" });
    }
  });

  // 代理视频流：带 Referer 转发 B站 CDN，绕过防盗链
  app.get("/proxy", async (req, res) => {
    const targetUrl = req.query.url as string | undefined;
    if (!targetUrl) {
      res.status(400).json({ error: "缺少 url 参数" });
      return;
    }

    try {
      const rangeHeader = req.headers.range;
      const requestHeaders: Record<string, string> = {
        Referer: "https://www.bilibili.com",
        Origin: "https://www.bilibili.com",
        "User-Agent": DEFAULT_USER_AGENT,
      };
      if (rangeHeader) {
        requestHeaders.Range = rangeHeader;
      }

      const response = await fetch(targetUrl, {
        headers: requestHeaders,
      });

      if (!response.ok) {
        res.status(response.status).send(await response.text());
        return;
      }

      response.headers.forEach((value: string, key: string) => {
        if (
          [
            "content-type",
            "content-length",
            "accept-ranges",
            "content-range",
          ].includes(key)
        ) {
          res.setHeader(key, value);
        }
      });

      if (response.body) {
        response.body.pipe(res);
      } else {
        const buffer = await response.buffer();
        res.send(buffer);
      }
    } catch (err) {
      console.error("[ZViewer CLI] 代理失败:", err);
      res
        .status(500)
        .json({ error: err instanceof Error ? err.message : "代理失败" });
    }
  });

  return new Promise((resolve, reject) => {
    server.listen(port, "127.0.0.1", () => {
      resolve({
        server,
        url: `http://127.0.0.1:${port}`,
        state,
        connect: async () => {
          if (state.config && onConnect && !state.connected) {
            state.connecting = true;
            try {
              await onConnect(state.config);
              state.connected = true;
            } finally {
              state.connecting = false;
            }
          }
        },
      });
    });
    server.on("error", reject);
  });
}
