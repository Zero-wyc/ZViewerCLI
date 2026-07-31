package main

const setupPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ZViewer CLI</title>
  <style>
:root {
      --bg: #0b0e12;
      --surface: #11161d;
      --surface-raised: #171d25;
      --surface-hover: #1d242d;
      --border: #202831;
      --border-strong: #2a343f;
      --text: #e8eaed;
      --text-muted: #8b949e;
      --text-subtle: #5f6b7a;
      --accent: #2dd4bf;
      --accent-hover: #5eead4;
      --accent-text: #042f2e;
      --warn: #f59e0b;
      --error: #ef4444;
      --error-dim: #fca5a5;
      --success: #22c55e;
      --success-dim: #86efac;
      --radius: 10px;
      --radius-sm: 8px;
      --shadow: 0 1px 2px rgba(0,0,0,0.25), 0 4px 12px rgba(0,0,0,0.15);
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      background-color: #05070a;
      background-image:
        linear-gradient(rgba(5,7,10,0.5), rgba(5,7,10,0.5)),
        url('/Nacho.webp');
      background-position: center, center;
      background-size: cover, cover;
      background-attachment: fixed, fixed;
      color: var(--text);
      min-height: 100vh;
      display: flex;
      justify-content: center;
      padding: 48px 18px;
      line-height: 1.5;
      -webkit-font-smoothing: antialiased;
    }
    .container { width: 100%; max-width: 540px; display: flex; flex-direction: column; gap: 18px; }
    .header { display: flex; align-items: center; gap: 14px; padding-bottom: 8px; }
    .brand-icon {
      width: 44px; height: 44px; border-radius: var(--radius-sm);
      background: var(--surface); border: 1px solid var(--border-strong);
      display: flex; align-items: center; justify-content: center;
      color: var(--accent); flex-shrink: 0;
      box-shadow: var(--shadow);
    }
    .brand-icon svg { width: 22px; height: 22px; }
    .header-text { display: flex; flex-direction: column; gap: 2px; }
    .header-title { font-size: 20px; font-weight: 700; color: var(--text); letter-spacing: -0.01em; }
    .header-subtitle { font-size: 13px; color: var(--text-muted); }
    .card {
      background: rgba(17, 22, 29, 0.72);
      backdrop-filter: blur(12px);
      -webkit-backdrop-filter: blur(12px);
      border: 1px solid rgba(255,255,255,0.07);
      border-radius: var(--radius); padding: 22px; box-shadow: var(--shadow);
    }
    .section-title {
      font-size: 14px; font-weight: 600; color: var(--text);
      margin-bottom: 16px; display: flex; align-items: center; gap: 8px;
    }
    .section-title svg { width: 16px; height: 16px; color: var(--text-subtle); flex-shrink: 0; }
    .field { margin-bottom: 16px; }
    .field:last-child { margin-bottom: 0; }
    .field label {
      display: block; font-size: 13px; font-weight: 500;
      color: var(--text-muted); margin-bottom: 6px;
    }
    .input, .textarea, select.input {
      width: 100%; background: rgba(5, 7, 10, 0.55); border: 1px solid rgba(255,255,255,0.08);
      border-radius: var(--radius-sm); padding: 10px 12px; color: var(--text);
      font-size: 14px; outline: none; transition: border-color .15s, box-shadow .15s;
    }
    .input::placeholder, .textarea::placeholder { color: var(--text-subtle); }
    .input:focus, .textarea:focus { border-color: var(--accent); box-shadow: 0 0 0 2px rgba(45,212,191,0.12); }
    .textarea { min-height: 84px; resize: vertical; font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace; font-size: 12px; line-height: 1.5; }
    select.input { appearance: none; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%235f6b7a' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E"); background-repeat: no-repeat; background-position: right 10px center; padding-right: 32px; }
    .row { display: flex; gap: 12px; }
    .row .field { flex: 1; }
    .btn {
      width: 100%; border: none; border-radius: var(--radius-sm); padding: 11px 14px;
      font-size: 14px; font-weight: 600; cursor: pointer; transition: all .15s;
      display: flex; align-items: center; justify-content: center; gap: 8px;
    }
    .btn:hover:not(:disabled) { transform: translateY(-1px); }
    .btn:active:not(:disabled) { transform: translateY(0); }
    .btn:disabled { opacity: .5; cursor: not-allowed; }
    .btn-primary { background: var(--accent); color: var(--accent-text); }
    .btn-primary:hover:not(:disabled) { background: var(--accent-hover); }
    .btn-secondary { background: var(--surface-raised); color: var(--text); border: 1px solid var(--border-strong); }
    .btn-secondary:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
    .btn-danger { background: transparent; color: var(--error-dim); border: 1px solid var(--border-strong); }
    .btn-danger:hover:not(:disabled) { border-color: var(--error); color: var(--error); background: rgba(239,68,68,0.06); }
    .btn-row { display: flex; gap: 10px; }
    .btn-row .btn { width: auto; flex: 1; }
    .qr-wrap {
      display: flex; flex-direction: column; align-items: center; gap: 14px;
      margin-top: 16px; padding: 18px; background: rgba(5, 7, 10, 0.5);
      border-radius: var(--radius-sm); border: 1px dashed rgba(255,255,255,0.12);
    }
    .qr-wrap img { width: 180px; height: 180px; border-radius: var(--radius-sm); background: #fff; padding: 6px; }
    .status {
      font-size: 13px; padding: 10px 12px; border-radius: var(--radius-sm);
      background: rgba(23, 29, 37, 0.72); border: 1px solid rgba(255,255,255,0.07);
      border-left: 3px solid var(--border-strong); word-break: break-word;
    }
    .status.error { border-left-color: var(--error); color: var(--error-dim); }
    .status.success { border-left-color: var(--success); color: var(--success-dim); }
    .hidden { display: none !important; }
    .hint { font-size: 12px; color: var(--text-muted); margin-top: 12px; line-height: 1.6; }
    .cookie-state { font-size: 12px; font-weight: 500; margin-top: 6px; }
    .cookie-state.empty { color: var(--text-subtle); }
    .cookie-state.valid { color: var(--success); }
    .cookie-state.invalid { color: var(--error); }
    .footer { text-align: center; font-size: 12px; color: var(--text-subtle); margin-top: 8px; }
    .test-player-wrap {
      margin-top: 16px; border-radius: var(--radius-sm); overflow: hidden;
      background: #000; aspect-ratio: 16 / 9; display: none; border: 1px solid var(--border);
    }
    .test-player-wrap video { width: 100%; height: 100%; display: block; }
    .test-info {
      margin-top: 12px; font-size: 12px; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      padding: 12px; border-radius: var(--radius-sm); background: rgba(5, 7, 10, 0.55);
      border: 1px solid rgba(255,255,255,0.08); max-height: 220px; overflow-y: auto;
      display: none; line-height: 1.6; word-break: break-all; white-space: pre-wrap;
    }
    .test-info-row { display: flex; gap: 8px; padding: 2px 0; }
    .test-info-label { color: var(--text-muted); min-width: 84px; flex-shrink: 0; }
    .test-info-value { color: var(--text); }
    .test-info-value.success { color: var(--success); }
    .test-info-value.error { color: var(--error); }
    .badge {
      display: inline-block; font-size: 10px; font-weight: 600;
      padding: 2px 7px; border-radius: 999px; text-transform: uppercase; letter-spacing: 0.04em;
    }
    .badge-dash { background: rgba(245,158,11,0.12); color: var(--warn); }
    .badge-mp4 { background: rgba(45,212,191,0.12); color: var(--accent); }
    @media (max-width: 480px) {
      body { padding: 24px 14px; }
      .card { padding: 18px; }
      .row { flex-direction: column; gap: 0; }
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="brand-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect>
          <line x1="8" y1="21" x2="16" y2="21"></line>
          <line x1="12" y1="17" x2="12" y2="21"></line>
        </svg>
      </div>
      <div class="header-text">
        <div class="header-title">ZViewer CLI</div>
        <div class="header-subtitle">本地高画质代理客户端</div>
      </div>
    </div>

    <div class="card">
      <div class="section-title">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg>
        连接配置
      </div>
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
      <div class="section-title">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
        房间连接
      </div>
      <div class="btn-row">
        <button class="btn btn-primary" id="connectBtn" type="button">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"></path><path d="m12 5 7 7-7 7"></path></svg>
          连接房间
        </button>
        <button class="btn btn-danger hidden" id="disconnectBtn" type="button">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>
          断开
        </button>
      </div>
      <div class="status hidden" id="connectStatus"></div>
      <div class="hint">连接成功后，可在网页端「B站解析设置」中启用「CLI 高画质代理」。</div>
    </div>

    <div class="card">
      <div class="section-title">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        视频流测试
      </div>
      <div class="field">
        <label>B站视频链接 / BV号</label>
        <input type="text" id="testInput" class="input" placeholder="BV1GJ411x7h7 或 https://www.bilibili.com/video/BV1GJ411x7h7" />
      </div>
      <div class="row">
        <div class="field">
          <label>清晰度</label>
          <select id="testQn" class="input">
            <option value="">自动</option>
            <option value="127">8K</option>
            <option value="126">杜比视界</option>
            <option value="125">HDR</option>
            <option value="120">4K</option>
            <option value="116">1080P60</option>
            <option value="112">1080P+</option>
            <option value="80">1080P</option>
            <option value="74">720P60</option>
            <option value="64">720P</option>
            <option value="32">480P</option>
            <option value="16">360P</option>
          </select>
        </div>
        <div class="field">
          <label>格式</label>
          <select id="testFormat" class="input">
            <option value="dash">DASH (m4s)</option>
            <option value="mp4">MP4 直链</option>
          </select>
        </div>
      </div>
      <button class="btn btn-secondary" id="testBtn" type="button">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>
        解析并播放
      </button>
      <div class="status hidden" id="testStatus"></div>
      <div class="test-player-wrap" id="testPlayerWrap">
        <video id="testVideo" controls playsinline></video>
      </div>
      <div class="test-info" id="testInfo"></div>
      <div class="hint">测试功能直接通过 CLI 本地代理解析并播放视频，用于验证 DASH 流是否正常。需要先设置有效的 B站 Cookie。</div>
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
    const disconnectBtn = document.getElementById('disconnectBtn')
    let roomState = { connected: false, connecting: false }

    function updateDisconnectVisibility() {
      if (roomState.connected || roomState.connecting) {
        disconnectBtn.classList.remove('hidden')
      } else {
        disconnectBtn.classList.add('hidden')
      }
    }

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
      roomState.connecting = true
      updateDisconnectVisibility()
      setStatus(connectStatus, '正在连接…')

      try {
        const res = await fetch('/api/connect', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ serverUrl: server, roomId: room, cookie })
        })
        const data = await res.json()
        if (!res.ok || !data.success) throw new Error(data.message || '连接失败')
        roomState.connected = true
        setStatus(connectStatus, '已连接房间，可在网页启用 CLI 高画质代理', 'success')
      } catch (err) {
        setStatus(connectStatus, err.message, 'error')
      } finally {
        connectBtn.disabled = false
        roomState.connecting = false
        updateDisconnectVisibility()
      }
    })

    disconnectBtn.addEventListener('click', async () => {
      disconnectBtn.disabled = true
      setStatus(connectStatus, '正在断开连接…')
      try {
        const res = await fetch('/api/disconnect', { method: 'POST' })
        const data = await res.json()
        if (!res.ok || !data.success) throw new Error(data.message || '断开失败')
        roomState.connected = false
        roomState.connecting = false
        setStatus(connectStatus, '已断开房间连接', 'success')
      } catch (err) {
        setStatus(connectStatus, err.message, 'error')
      } finally {
        disconnectBtn.disabled = false
        updateDisconnectVisibility()
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
        roomState.connected = !!data.connected
        roomState.connecting = !!data.connecting
        if (roomState.connecting) {
          connectBtn.disabled = true
          setStatus(connectStatus, '正在连接…')
        } else if (roomState.connected) {
          setStatus(connectStatus, '已连接房间', 'success')
        }
        updateDisconnectVisibility()
      })
      .catch(() => {})

    const testInput = document.getElementById('testInput')
    const testQn = document.getElementById('testQn')
    const testFormat = document.getElementById('testFormat')
    const testBtn = document.getElementById('testBtn')
    const testStatus = document.getElementById('testStatus')
    const testPlayerWrap = document.getElementById('testPlayerWrap')
    const testVideo = document.getElementById('testVideo')
    const testInfo = document.getElementById('testInfo')
    let dashPlayer = null
    let testLogs = []

    function logTest(msg) {
      testLogs.push('[' + new Date().toLocaleTimeString() + '] ' + msg)
      if (testLogs.length > 50) testLogs.shift()
    }

    function renderTestInfo(rows) {
      if (rows && rows.length > 0) {
        testInfo.style.display = 'block'
        testInfo.innerHTML = rows.map(r =>
          '<div class="test-info-row">' +
          '<span class="test-info-label">' + r.label + '</span>' +
          '<span class="test-info-value' + (r.cls || '') + '">' + r.value + '</span>' +
          '</div>'
        ).join('') + '<div class="test-info-row"><span class="test-info-label">日志</span></div>' +
        testLogs.map(l => '<div class="test-info-row"><span class="test-info-value" style="font-size:11px;opacity:0.7">' + l + '</span></div>').join('')
      } else {
        testInfo.style.display = 'block'
        testInfo.innerHTML = testLogs.map(l => '<div class="test-info-row"><span class="test-info-value" style="font-size:11px;opacity:0.7">' + l + '</span></div>').join('')
      }
    }

    function extractBvid(input) {
      const trimmed = input.trim()
      const m = trimmed.match(/BV[a-zA-Z0-9]{10}/)
      if (m) return m[0]
      return trimmed
    }

    async function fetchCid(bvid) {
      const res = await fetch('/api/bili-info?bvid=' + encodeURIComponent(bvid))
      const data = await res.json()
      if (!res.ok || !data.success || !data.cid) {
        throw new Error('获取视频信息失败: ' + (data.message || '未知错误'))
      }
      return { cid: data.cid, title: data.title, duration: data.duration }
    }

    testBtn.addEventListener('click', async () => {
      const raw = testInput.value.trim()
      if (!raw) { setStatus(testStatus, '请输入 B站视频链接或 BV号', 'error'); return }
      const cookie = cookieEl.value.trim()
      if (!cookie) { setStatus(testStatus, '请先设置 B站 Cookie', 'error'); return }

      testBtn.disabled = true
      setStatus(testStatus, '正在解析…')
      testPlayerWrap.style.display = 'none'
      testInfo.style.display = 'none'
      testLogs = []
      logTest('开始解析')

      if (dashPlayer) {
        try { dashPlayer.reset() } catch (e) {}
        try { dashPlayer.release() } catch (e) {}
        dashPlayer = null
      }
      testVideo.src = ''

      const format = testFormat.value
      const preferMp4 = format === 'mp4'
      const bvid = extractBvid(raw)
      logTest('BV号: ' + bvid + ', 格式: ' + format)

      try {
        setStatus(testStatus, '正在获取视频信息…')
        const info = await fetchCid(bvid)
        logTest('cid=' + info.cid + ', title=' + info.title)
        setStatus(testStatus, '正在解析视频流…')

        const p = new URLSearchParams({ bvid, cid: String(info.cid), preferMp4: String(preferMp4) })
        const qnVal = testQn.value
        if (qnVal) p.set('qn', qnVal)
        const res = await fetch('/resolve?' + p.toString())
        const data = await res.json()
        if (!res.ok || data.error || !data.videoUrl) {
          throw new Error(data.error || data.message || '解析失败')
        }

        logTest('解析成功: format=' + (data.format || format))
        logTest('videoUrl=' + data.videoUrl.substring(0, 80) + '...')
        if (data.audioUrl) logTest('audioUrl=' + data.audioUrl.substring(0, 80) + '...')

        const isDash = data.format === 'dash' || (data.videoUrl && data.audioUrl)
        const badge = isDash ? '<span class="badge badge-dash">DASH</span>' : '<span class="badge badge-mp4">MP4</span>'
        setStatus(testStatus, '解析成功，正在播放' + badge, 'success')

        const rows = [
          { label: '标题', value: data.title || info.title || '-' },
          { label: '格式', value: (data.format || format).toUpperCase() + ' ' + badge },
          { label: '清晰度', value: data.currentQn ? String(data.currentQn) : (qnVal || '自动') },
          { label: 'Video URL', value: data.videoUrl.substring(0, 100) + '...' },
          { label: 'Audio URL', value: data.audioUrl ? data.audioUrl.substring(0, 100) + '...' : '无（MP4 内含音频）' },
          { label: 'Video Codec', value: data.videoCodec || '-' },
          { label: 'Audio Codec', value: data.audioCodec || '-' },
          { label: 'Duration', value: data.duration ? data.duration + 's' : (info.duration ? info.duration + 's' : '-') },
        ]
        renderTestInfo(rows)

        testPlayerWrap.style.display = 'block'

        if (isDash) {
          logTest('使用 dash.js 播放 DASH 流')
          if (!window.dashjs) {
            logTest('加载 dash.js...')
            await loadScript('https://cdn.jsdelivr.net/npm/dashjs@4.7.4/dist/dash.all.min.js')
            logTest('dash.js 加载完成')
          }

          dashPlayer = dashjs.MediaPlayer().create()
          dashPlayer.updateSettings({
            streaming: {
              buffer: { fastSwitchEnabled: true, stableBufferTime: 2, bufferTimeAtTopQuality: 10, bufferTimeAtTopQualityLongForm: 20 },
              abr: { autoSwitchBitrate: { video: false, audio: false } }
            }
          })
          dashPlayer.setXHRWithCredentialsForType('MediaSegment', false)
          dashPlayer.setXHRWithCredentialsForType('MPD', false)
          dashPlayer.setXHRWithCredentialsForType('InitializationSegment', false)
          dashPlayer.setXHRWithCredentialsForType('BitstreamSwitchingSegment', false)

          logTest('请求 CLI 生成 MPD manifest...')
          var mpdParams = new URLSearchParams({
            videoUrl: data.sourceVideoUrl || '',
            audioUrl: data.sourceAudioUrl || '',
            videoCodec: data.videoCodec || '',
            audioCodec: data.audioCodec || '',
            duration: String(data.duration || info.duration || 0)
          })
          var mpdRes = await fetch('/api/dash-mpd?' + mpdParams.toString())
          if (!mpdRes.ok) {
            var mpdErr = await mpdRes.text()
            throw new Error('生成 MPD 失败: ' + mpdErr)
          }
          var mpdText = await mpdRes.text()
          logTest('MPD 生成完成，长度=' + mpdText.length)
          var mpdBlob = new Blob([mpdText], { type: 'application/dash+xml' })
          var mpdUrl = URL.createObjectURL(mpdBlob)
          dashPlayer.initialize(testVideo, mpdUrl, true)
          logTest('dash.js 初始化完成，等待播放')

          var seekTested = false
          dashPlayer.on(dashjs.MediaPlayer.events['ERROR'], (e) => {
            logTest('dash.js ERROR: ' + (e.error ? (e.error.code + ' ' + e.error.message) : JSON.stringify(e)))
            renderTestInfo()
          })
          dashPlayer.on(dashjs.MediaPlayer.events['STREAM_INITIALIZED'], () => { logTest('流初始化完成'); renderTestInfo() })
          dashPlayer.on(dashjs.MediaPlayer.events['PLAYBACK_PLAYING'], () => {
            logTest('开始播放, currentTime=' + testVideo.currentTime.toFixed(2) + ', duration=' + testVideo.duration.toFixed(2))
            if (!seekTested) { seekTested = true; setTimeout(function() { testSeek(100) }, 3000) }
            renderTestInfo()
          })
          dashPlayer.on(dashjs.MediaPlayer.events['PLAYBACK_PAUSED'], () => { logTest('暂停, currentTime=' + testVideo.currentTime.toFixed(2)); renderTestInfo() })
          dashPlayer.on(dashjs.MediaPlayer.events['PLAYBACK_ERROR'], (e) => { logTest('播放错误: ' + JSON.stringify(e)); renderTestInfo() })
          dashPlayer.on(dashjs.MediaPlayer.events['PLAYBACK_SEEKING'], () => { logTest('seeking 开始, currentTime=' + testVideo.currentTime.toFixed(2)); renderTestInfo() })
          dashPlayer.on(dashjs.MediaPlayer.events['PLAYBACK_SEEKED'], () => { logTest('seeking 完成, currentTime=' + testVideo.currentTime.toFixed(2)); renderTestInfo() })

          window.testSeek = function(targetTime) {
            logTest('--- seek 测试: 目标=' + targetTime + 's ---')
            logTest('seek 前 currentTime=' + testVideo.currentTime.toFixed(2) + ', seeking=' + testVideo.seeking)
            var seekStart = Date.now()
            testVideo.currentTime = targetTime
            var seekTimeout = setTimeout(function() {
              if (testVideo.seeking) {
                logTest('!!! seek 超时 (10s), currentTime=' + testVideo.currentTime.toFixed(2) + ', seeking=' + testVideo.seeking + ', readyState=' + testVideo.readyState)
                renderTestInfo()
                setTimeout(function() { testSeek2(50) }, 2000)
              }
            }, 10000)
            var seekedHandler = function() {
              clearTimeout(seekTimeout)
              var elapsed = Date.now() - seekStart
              logTest('seek 完成, 耗时=' + elapsed + 'ms, currentTime=' + testVideo.currentTime.toFixed(2) + ', readyState=' + testVideo.readyState)
              renderTestInfo()
              setTimeout(function() { testSeek2(50) }, 2000)
            }
            testVideo.addEventListener('seeked', seekedHandler, { once: true })
          }
          window.testSeek2 = function(targetTime) {
            logTest('--- seek 测试2: 目标=' + targetTime + 's ---')
            logTest('seek 前 currentTime=' + testVideo.currentTime.toFixed(2))
            var seekStart = Date.now()
            testVideo.currentTime = targetTime
            var seekTimeout = setTimeout(function() {
              if (testVideo.seeking) {
                logTest('!!! seek2 超时 (10s), currentTime=' + testVideo.currentTime.toFixed(2) + ', seeking=' + testVideo.seeking)
                renderTestInfo()
              }
            }, 10000)
            var seekedHandler = function() {
              clearTimeout(seekTimeout)
              var elapsed = Date.now() - seekStart
              logTest('seek2 完成, 耗时=' + elapsed + 'ms, currentTime=' + testVideo.currentTime.toFixed(2))
              renderTestInfo()
            }
            testVideo.addEventListener('seeked', seekedHandler, { once: true })
          }
        } else {
          logTest('使用 video src 播放 MP4 直链')
          testVideo.src = data.videoUrl
          testVideo.play().catch(err => logTest('播放失败: ' + err.message))
        }

        renderTestInfo(rows)
      } catch (err) {
        logTest('错误: ' + err.message)
        setStatus(testStatus, err.message, 'error')
        renderTestInfo()
      } finally {
        testBtn.disabled = false
      }
    })

    function loadScript(src) {
      return new Promise((resolve, reject) => {
        const s = document.createElement('script')
        s.src = src
        s.onload = resolve
        s.onerror = () => reject(new Error('加载脚本失败: ' + src))
        document.head.appendChild(s)
      })
    }
  </script>
</body>
</html>`