package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/cors"
)

// proxyUpstreamTimeoutMs 上游代理默认超时。
const proxyUpstreamTimeoutMs = 60_000

// proxyPassThroughHeaders 需要透传给客户端的上游响应头（与 backend http-proxy.ts 对齐）。
var proxyPassThroughHeaders = []string{
	"Content-Length",
	"Accept-Ranges",
	"Content-Range",
	"ETag",
	"Last-Modified",
}

// proxyHTTPClient 全局代理 HTTP 客户端，复用 TCP 连接池提升 seek 性能。
// 使用 context.Background() + Timeout 而非 r.Context()，避免 dash.js seek 取消
// 之前的 segment 请求时代理也被连锁取消（导致 ERR_CONNECTION_REFUSED）。
var proxyHTTPClient = &http.Client{
	Timeout: proxyUpstreamTimeoutMs * time.Millisecond,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
	CheckRedirect: func(nextReq *http.Request, via []*http.Request) error {
		nextReq.Header.Set("User-Agent", userAgent)
		nextReq.Header.Set("Accept", "*/*")
		nextReq.Header.Set("Referer", "https://www.bilibili.com")
		nextReq.Header.Set("Origin", "https://www.bilibili.com")
		if len(via) > 0 {
			if rng := via[len(via)-1].Header.Get("Range"); rng != "" {
				nextReq.Header.Set("Range", rng)
			}
		}
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

// Agent 持有运行时的配置、状态和 Socket 连接。
type Agent struct {
	port    int
	state   *State
	socket  *SocketClient
	qrCache *QRSession
	// backupUrlCache 缓存每个主 CDN URL 对应的候选 URL 列表（含 backup）。
	// 当 /proxy 代理主 URL 失败时，可按顺序尝试候选列表中的其他 URL。
	backupUrlCache   map[string][]string
	backupUrlCacheMu sync.RWMutex
}

func newAgent(port int, cfg *LocalConfig) *Agent {
	a := &Agent{
		port:           port,
		state:          &State{},
		backupUrlCache: make(map[string][]string),
	}
	if cfg != nil {
		a.state.SetConfig(cfg)
	}
	return a
}

// setBackupUrls 将一组候选 URL 与主 URL 关联缓存。
func (a *Agent) setBackupUrls(primaryUrl string, candidates []string) {
	if primaryUrl == "" || len(candidates) == 0 {
		return
	}
	a.backupUrlCacheMu.Lock()
	defer a.backupUrlCacheMu.Unlock()
	a.backupUrlCache[primaryUrl] = candidates
}

// getBackupUrls 获取主 URL 的候选 URL 列表（不含主 URL 自身）。
func (a *Agent) getBackupUrls(primaryUrl string) []string {
	a.backupUrlCacheMu.RLock()
	defer a.backupUrlCacheMu.RUnlock()
	candidates, ok := a.backupUrlCache[primaryUrl]
	if !ok {
		return nil
	}
	var result []string
	for _, u := range candidates {
		if u != primaryUrl {
			result = append(result, u)
		}
	}
	return result
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
	if a.state.Connecting {
		return fmt.Errorf("正在连接中")
	}
	if a.state.Connected && a.socket != nil {
		return fmt.Errorf("已连接")
	}

	a.state.SetConnecting(true)
	a.state.SetLastError("")

	if os.Getenv("ZVIEWER_CLI_SKIP_COOKIE") == "1" {
		a.state.SetCookieValid(true)
		a.state.SetUserInfo(newUserInfoMap("test", 0, 0))
	} else {
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
		a.state.SetUserInfo(newUserInfoMap(validation.Name, validation.Mid, validation.VipStatus))
		_ = saveUserConfig(cfg.Cookie, validation)
	}

	client, err := connectSocket(cfg.ServerURL, cfg.RoomID, a.proxyURL(), a.state)
	if err != nil {
		a.state.SetConnecting(false)
		a.state.SetLastError(err.Error())
		return err
	}
	a.socket = client

	// 断线后自动重连：仅非主动断开时触发
	client.onDisconnect = func() {
		if a.socket == client {
			a.socket = nil
		}
		go a.reconnectLoop()
	}

	return nil
}

// reconnectLoop 在连接断开后使用指数退避 + 抖动策略尝试重新连接。
func (a *Agent) reconnectLoop() {
	cfg := a.state.Config
	if cfg == nil || cfg.ServerURL == "" || cfg.RoomID == "" {
		return
	}

	const baseDelay = 2 * time.Second
	const maxDelay = 60 * time.Second
	delay := baseDelay

	for {
		if a.state.Connected || a.state.Connecting {
			return
		}

		logf("正在尝试重新连接房间 %s...", cfg.RoomID)
		if err := a.doConnect(); err != nil {
			logf("重新连接失败: %v", err)
			// 指数退避：delay = min(maxDelay, delay*2)，并加入 ±25% 抖动避免重连风暴
			if delay < maxDelay/2 {
				delay *= 2
			} else {
				delay = maxDelay
			}
			jitter := time.Duration(float64(delay) * (0.75 + 0.5*rand.Float64()))
			logf("%v 后再次重试...", jitter)
			time.Sleep(jitter)
			continue
		}
		logf("重新连接成功")
		return
	}
}

func (a *Agent) disconnect() {
	if a.socket != nil {
		old := a.socket
		a.socket = nil
		old.onDisconnect = nil
		old.Close()
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

	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/qr", a.handleQR)
	mux.HandleFunc("/api/qr/poll", a.handleQRPoll)
	mux.HandleFunc("/api/connect", a.handleConnect)
	mux.HandleFunc("/api/bili-info", a.handleBiliInfo)
	mux.HandleFunc("/api/dash-mpd", a.handleDashMpd)
	mux.HandleFunc("/resolve", a.handleResolve)
	mux.HandleFunc("/proxy", a.handleProxy)

	return mux
}

func (a *Agent) handleConfig(w http.ResponseWriter, r *http.Request) {
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
					a.state.SetUserInfo(newUserInfoMap(validation.Name, validation.Mid, validation.VipStatus))
					_ = saveUserConfig(cfg.Cookie, validation)
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
}

func (a *Agent) handleQR(w http.ResponseWriter, r *http.Request) {
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
}

func (a *Agent) handleQRPoll(w http.ResponseWriter, r *http.Request) {
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
			a.state.SetUserInfo(newUserInfoMap(validation.Name, validation.Mid, validation.VipStatus))
			_ = saveUserConfig(result.Cookie, validation)
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
}

func (a *Agent) handleConnect(w http.ResponseWriter, r *http.Request) {
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
}

// handleBiliInfo 通过 B站 API 获取视频信息（cid、标题、时长），避免浏览器直接请求 B站 API 的 CORS 问题。
// 现在复用本地 resolver 的 WBI 签名接口，与 backend 行为一致。
func (a *Agent) handleBiliInfo(w http.ResponseWriter, r *http.Request) {
	bvid := r.URL.Query().Get("bvid")
	if bvid == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 bvid"})
		return
	}
	cookie := a.resolveCookie(r)
	if cookie == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 B站 Cookie"})
		return
	}

	info, err := getVideoInfo(bvid, cookie)
	if err != nil {
		logf("获取视频信息失败: %v", err)
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	if info == nil {
		jsonResponse(w, http.StatusOK, map[string]any{"success": false, "message": "视频信息为空"})
		return
	}
	pages := make([]map[string]any, 0, len(info.Pages))
	for _, p := range info.Pages {
		pages = append(pages, map[string]any{
			"cid":      p.Cid,
			"page":     p.Page,
			"part":     p.Part,
			"duration": p.Duration,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"success":  true,
		"cid":      info.Cid,
		"title":    info.Title,
		"duration": info.Duration,
		"pages":    pages,
	})
}

// resolveCookie 从请求参数或当前配置中读取 B站 Cookie。
func (a *Agent) resolveCookie(r *http.Request) string {
	if c := r.URL.Query().Get("cookie"); c != "" {
		return strings.TrimSpace(c)
	}
	if a.state.Config != nil {
		return a.state.Config.Cookie
	}
	return ""
}

func (a *Agent) handleResolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bvid := q.Get("bvid")
	cidRaw := q.Get("cid")
	qnRaw := q.Get("qn")
	preferMp4 := q.Get("preferMp4") == "true"
	forceDash := q.Get("forceDash") == "true"
	if bvid == "" || cidRaw == "" {
		http.Error(w, `{"error":"缺少 bvid 或 cid"}`, http.StatusBadRequest)
		return
	}
	cookie := a.resolveCookie(r)
	if cookie == "" {
		http.Error(w, `{"error":"缺少 B站 Cookie"}`, http.StatusBadRequest)
		return
	}

	cid, err := strconv.ParseInt(cidRaw, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"cid 格式错误"}`, http.StatusBadRequest)
		return
	}
	qn := 0
	if qnRaw != "" {
		qn, _ = strconv.Atoi(qnRaw)
	}

	logf("[resolve] 收到解析请求 bvid=%s cid=%d qn=%d preferMp4=%v forceDash=%v", bvid, cid, qn, preferMp4, forceDash)

	result, err := ResolveBilibiliVideo(ResolveOptions{
		Url:          fmt.Sprintf("https://www.bilibili.com/video/%s", bvid),
		Cookie:       cookie,
		Qn:           qn,
		PreferMp4:    preferMp4,
		ForceDash:    forceDash,
		Cid:          cid,
		// CLI 使用本地代理播放，实际视频流由用户本机浏览器→CLI 拉取，
		// 无需校验 B站 CDN 可达性，避免本地网络差异导致错误降级为 MP4。
		SkipCdnCheck: true,
	})
	if err != nil {
		logf("解析失败: %v", err)
		code := http.StatusInternalServerError
		if re, ok := err.(*ResolveError); ok {
			switch re.Code {
			case "NOT_LOGGED_IN", "NO_PERMISSION":
				code = http.StatusUnauthorized
			case "VIDEO_NOT_FOUND", "INVALID_INPUT":
				code = http.StatusBadRequest
			}
		}
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), code)
		return
	}

	logf("[resolve] 解析成功 bvid=%s format=%s qn=%d videoUrl=%s", bvid, result.Format, result.CurrentQn, result.VideoUrl)

	// 缓存主 URL 与候选 URL 映射（视频/音频分开，避免 /proxy 交叉尝试）。
	a.setBackupUrls(result.VideoUrl, result.VideoBackupUrls)
	if result.AudioUrl != "" {
		a.setBackupUrls(result.AudioUrl, result.AudioBackupUrls)
	}

	// 同时返回原始 URL 和代理 URL：
	// - videoUrl/audioUrl: 代理 URL（供 MP4 直连播放用）
	// - sourceVideoUrl/sourceAudioUrl: 原始 B站 CDN URL（供 /api/dash-mpd 生成 MPD 用，避免重复解析）
	proxyBase := a.proxyURL()
	type resolveResponse struct {
		ResolveResult
		SourceVideoUrl string `json:"sourceVideoUrl,omitempty"`
		SourceAudioUrl string `json:"sourceAudioUrl,omitempty"`
	}
	resp := resolveResponse{ResolveResult: *result}
	if result.VideoUrl != "" {
		resp.SourceVideoUrl = result.VideoUrl
		result.VideoUrl = fmt.Sprintf("%s/proxy?url=%s", proxyBase, url.QueryEscape(result.VideoUrl))
	}
	if result.AudioUrl != "" {
		resp.SourceAudioUrl = result.AudioUrl
		result.AudioUrl = fmt.Sprintf("%s/proxy?url=%s", proxyBase, url.QueryEscape(result.AudioUrl))
	}
	resp.ResolveResult = *result

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *Agent) handleProxy(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, `{"error":"缺少 url 参数"}`, http.StatusBadRequest)
		return
	}

	// 候选 URL：主 URL + backup URL。主 URL 失败时按顺序尝试其他 CDN。
	candidates := append([]string{target}, a.getBackupUrls(target)...)
	logf("[proxy] 收到代理请求，共 %d 个候选 URL", len(candidates))

	var lastErr error
	for i, candidate := range candidates {
		res, err := a.doProxyRequest(w, r, candidate)
		if err != nil {
			lastErr = err
			// 客户端已断开时不继续尝试，直接返回
			if r.Context().Err() != nil {
				return
			}
			// 连接类错误（拒绝连接、超时、DNS 失败）时尝试下一个 CDN；
			// HTTP 4xx/5xx 状态码不在这里返回 err，因此会走成功分支透传状态码。
			logf("[proxy] 候选 %d/%d 失败: %v", i+1, len(candidates), err)
			if i < len(candidates)-1 {
				continue
			}
			break
		}
		if i > 0 {
			logf("[proxy] 候选 %d/%d 成功", i+1, len(candidates))
		}
		// 只有第一个成功响应的候选才会走到这里；后续不需要再尝试。
		_ = res
		return
	}

	// 所有候选均失败
	if r.Context().Err() != nil {
		return
	}
	if lastErr != nil {
		logf("[proxy] 所有候选均失败 (%d 个): %v", len(candidates), lastErr)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, lastErr.Error()), http.StatusBadGateway)
		return
	}
	http.Error(w, `{"error":"代理请求失败"}`, http.StatusBadGateway)
}

// doProxyRequest 执行单次上游代理请求。若成功已将响应写入 w 并返回非 nil res；
// 若发生可重试的网络/连接错误返回 err；上游返回非 2xx 时直接透传状态码并返回 nil err。
func (a *Agent) doProxyRequest(w http.ResponseWriter, r *http.Request, target string) (*http.Response, error) {
	// 使用 context.Background() 而非 r.Context()，避免 dash.js seek 取消
	// segment 请求时代理也被连锁取消。客户端断连时 io.Copy 会自然终止。
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		logf("代理请求构造失败: %v", err)
		return nil, err
	}

	// 上游请求头：与 backend http-proxy.ts 对齐，注入 B站 Referer/Origin 防盗链。
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://www.bilibili.com")
	req.Header.Set("Origin", "https://www.bilibili.com")
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}

	res, err := proxyHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// 上游非 2xx 时透传状态码，让客户端（dash.js / video 标签）自行处理。
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		w.WriteHeader(res.StatusCode)
		return res, nil
	}

	// Content-Type 纠正：B站 CDN 偶发返回 application/json（实际是视频数据），
	// 使用兜底 video/mp4 避免 MSE 引擎因类型不匹配拒绝处理。
	upstreamContentType := res.Header.Get("Content-Type")
	defaultContentType := "video/mp4"
	if upstreamContentType != "" &&
		strings.Contains(strings.ToLower(upstreamContentType), "application/json") &&
		!strings.Contains(strings.ToLower(defaultContentType), "json") {
		w.Header().Set("Content-Type", defaultContentType)
	} else if upstreamContentType != "" {
		w.Header().Set("Content-Type", upstreamContentType)
	} else {
		w.Header().Set("Content-Type", defaultContentType)
	}

	for _, key := range proxyPassThroughHeaders {
		if v := res.Header.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}

	w.WriteHeader(res.StatusCode)
	logReader := newSpeedLoggerReader(res.Body, hostFromURL(target))
	_, _ = io.Copy(w, logReader)
	logReader.finish()
	return res, nil
}

func (a *Agent) startHTTPServer() (*http.Server, error) {
	mux := a.setupServer()
	// CORS 配置：使用 AllowedOriginFunc 回显具体 Origin 而非通配符 *。
	// CLI 代理不需要浏览器发送 Cookie（本地配置已持有 Cookie），因此禁用 credentials，
	// 避免浏览器因 Access-Control-Allow-Origin: * 与 Allow-Credentials: true 同时存在而拦截。
	// CLI 仅监听 127.0.0.1，外部无法访问，回显任意 Origin 无安全风险。
	handler := cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		// 浏览器安全策略：当 AllowedOrigins 为 * 时，Access-Control-Allow-Origin 不能为 * 且同时允许 credentials。
		// CLI 代理不需要浏览器发送 Cookie（本地配置已持有 Cookie），禁用 credentials 避免 CORS 预检失败。
		AllowCredentials: false,
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
