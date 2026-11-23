package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/cors"
)

//go:embed Nacho.webp
var nachoBackground []byte

// proxyUpstreamTimeoutMs 上游代理默认超时。
const proxyUpstreamTimeoutMs = 60_000

// 直播 CDN 的防盗链口径：Referer/Origin 必须是 live.bilibili.com。
// 点播用 www.bilibili.com，两者不能混用（拿点播 Referer 拉直播流会被 403）。
const (
	vodReferer  = "https://www.bilibili.com"
	liveReferer = "https://live.bilibili.com"
)

// /candidates 的入参约束：候选表只是失败换节点用的辅助数据，
// 上限压得比较紧，避免一个畸形请求把内存撑大。
const (
	maxCandidatesPerTrack    = 16
	maxBackupUrlCacheEntries = 256
	candidateMaxBodyBytes    = 64 << 10
	candidateMaxURLLength    = 2048
)

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
		nextReq.Header.Set("Referer", vodReferer)
		nextReq.Header.Set("Origin", vodReferer)
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

// liveProxyClient 专用于直播流：**不能设 Client.Timeout**。
//
// 直播是一根不断吐出 FLV 数据的长连接，请求本身永远「没结束」；带上点播用的
// 60 秒总超时，观众看满一分钟就会被 CLI 自己掐断，表现为「每隔一分钟卡一次」。
// 取消由客户端断开（写失败）或进程退出时自然结束。
var liveProxyClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
		// 直播不能有「响应头超时」：某些边缘节点要等首个 GOP 才回头；
		// 这里只靠 TCP 自身的超时兜底。
		ResponseHeaderTimeout: 20 * time.Second,
	},
	CheckRedirect: func(nextReq *http.Request, via []*http.Request) error {
		nextReq.Header.Set("User-Agent", userAgent)
		nextReq.Header.Set("Accept", "*/*")
		nextReq.Header.Set("Referer", liveReferer)
		nextReq.Header.Set("Origin", liveReferer)
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
	// 近似淘汰：缓存只用于失败换节点，丢一条最多多一次 502 直连，
	// 不值得为它维护完整 LRU 链表。map 遍历顺序随机，删掉任意一条即可
	// 阻止缓存无界增长（长时间播放不同视频时会持续写入）。
	if len(a.backupUrlCache) >= maxBackupUrlCacheEntries {
		for key := range a.backupUrlCache {
			delete(a.backupUrlCache, key)
			break
		}
	}
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
	if cfg != nil {
		cfg.ResolveSource = normalizeResolveSource(cfg.ResolveSource)
	}
	a.state.SetConfig(cfg)
}

func (a *Agent) doConnect() error {
	cfg := a.state.Config
	if cfg == nil || cfg.ServerURL == "" {
		return fmt.Errorf("配置不完整")
	}
	source := normalizeResolveSource(cfg.ResolveSource)
	if source == resolveSourceLocal && cfg.Cookie == "" {
		return fmt.Errorf("本地 Cookie 解析需要先登录 B站")
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
	} else if cfg.Cookie != "" {
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
	} else {
		// 房主直链模式：不需要 B站 账号，也不再展示 Cookie 状态。
		a.state.ClearCookieValid()
	}

	client, err := connectSocket(cfg.ServerURL, cfg.User, a.proxyURL(), source, a.state)
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
	if cfg == nil || cfg.ServerURL == "" {
		return
	}

	const baseDelay = 2 * time.Second
	const maxDelay = 60 * time.Second
	delay := baseDelay

	for {
		if a.state.Connected || a.state.Connecting {
			return
		}

		logf("正在尝试重新连接服务器 %s...", cfg.ServerURL)
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
		// /health 被前端跨域轮询，显式返回 CORS 头。
		// 外层 rs/cors 中间件通常已处理，此处再设置一次可防止旧版构建或
		// 某些浏览器安全扩展导致响应头丢失。
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{
			"ok":      true,
			"agent":   "zcontrol-cli",
			"version": version,
		})
	})

	mux.HandleFunc("/Nacho.webp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// 优先使用内嵌的背景图数据（编译时通过 //go:embed 打包），
		// 这样 exe 单独运行时无需外部文件即可显示背景。
		data := nachoBackground
		// 若 exe 同目录存在同名文件，则使用外部文件覆盖（便于用户自定义背景）。
		if ex, err := os.Executable(); err == nil {
			extPath := filepath.Join(filepath.Dir(ex), "Nacho.webp")
			if extData, err := os.ReadFile(extPath); err == nil {
				data = extData
			}
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/qr", a.handleQR)
	mux.HandleFunc("/api/qr/poll", a.handleQRPoll)
	mux.HandleFunc("/api/connect", a.handleConnect)
	mux.HandleFunc("/api/disconnect", a.handleDisconnect)
	mux.HandleFunc("/api/bili-info", a.handleBiliInfo)
	mux.HandleFunc("/api/dash-mpd", a.handleDashMpd)
	mux.HandleFunc("/resolve", a.handleResolve)
	mux.HandleFunc("/proxy", a.handleProxy)
	mux.HandleFunc("/candidates", a.handleCandidates)

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
			"insecure":    allowInsecureTLS,
		})
	case http.MethodPost:
		var body struct {
			ServerURL     string  `json:"serverUrl"`
			User          string  `json:"user"`
			Cookie        string  `json:"cookie"`
			ResolveSource *string `json:"resolveSource"`
			Insecure      *bool   `json:"insecure"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "请求体解析失败"})
			return
		}
		if body.Insecure != nil {
			SetAllowInsecureTLS(*body.Insecure)
		}
		if strings.TrimSpace(body.ServerURL) == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 serverUrl"})
			return
		}
		source := resolveSourceHost
		if a.state.Config != nil && a.state.Config.ResolveSource != "" {
			source = normalizeResolveSource(a.state.Config.ResolveSource)
		}
		if body.ResolveSource != nil {
			source = normalizeResolveSource(*body.ResolveSource)
		}
		cfg := &LocalConfig{
			ServerURL:     strings.TrimSpace(body.ServerURL),
			User:          strings.TrimSpace(body.User),
			Cookie:        strings.TrimSpace(body.Cookie),
			ResolveSource: source,
		}
		if cfg.Cookie == "" && a.state.Config != nil {
			cfg.Cookie = a.state.Config.Cookie
		}
		// 本地解析必须有 Cookie；房主直链模式允许留空。
		if cfg.ResolveSource == resolveSourceLocal && cfg.Cookie == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "本地 Cookie 解析需要先登录 B站"})
			return
		}
		a.setConfig(cfg)
		_ = saveResolveSource(cfg.ResolveSource)
		// 连接信息落盘：重启后自动回填并自动连接（房主直链模式重启不再失联）。
		_ = saveConnectionConfig(cfg)

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
		ServerURL     string  `json:"serverUrl"`
		User          string  `json:"user"`
		Cookie        string  `json:"cookie"`
		ResolveSource *string `json:"resolveSource"`
		Insecure      *bool   `json:"insecure"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "请求体解析失败"})
		return
	}
	if body.Insecure != nil {
		SetAllowInsecureTLS(*body.Insecure)
	}
	if strings.TrimSpace(body.ServerURL) == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少后端地址"})
		return
	}
	source := resolveSourceHost
	if a.state.Config != nil && a.state.Config.ResolveSource != "" {
		source = normalizeResolveSource(a.state.Config.ResolveSource)
	}
	if body.ResolveSource != nil {
		source = normalizeResolveSource(*body.ResolveSource)
	}
	cookie := strings.TrimSpace(body.Cookie)
	if source == resolveSourceLocal && cookie == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "本地 Cookie 解析需要先登录 B站"})
		return
	}
	a.setConfig(&LocalConfig{
		ServerURL:     strings.TrimSpace(body.ServerURL),
		User:          strings.TrimSpace(body.User),
		Cookie:        cookie,
		ResolveSource: source,
	})
	_ = saveResolveSource(source)
	// 连接信息落盘：重启后自动回填并自动连接（房主直链模式重启不再失联）。
	_ = saveConnectionConfig(a.state.Config)

	if err := a.doConnect(); err != nil {
		a.state.SetLastError(err.Error())
		logf("连接服务器失败: %v", err)
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "Cookie") {
			status = http.StatusUnauthorized
		}
		jsonResponse(w, status, map[string]any{"success": false, "message": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true, "message": "连接成功"})
}

// handleDisconnect 取消房间连接，关闭 socket 并停止自动重连。
func (a *Agent) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.disconnect()
	logf("已断开服务器连接")
	jsonResponse(w, http.StatusOK, map[string]any{
		"success":   true,
		"message":   "已断开连接",
		"connected": a.state.Connected,
	})
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
	epIdRaw := q.Get("epId")
	cidRaw := q.Get("cid")
	qnRaw := q.Get("qn")
	preferMp4 := q.Get("preferMp4") == "true"
	forceDash := q.Get("forceDash") == "true"
	// bvid（UGC）或 epId（PGC 番剧）二选一；UGC 路径仍需 cid
	if bvid == "" && epIdRaw == "" {
		http.Error(w, `{"error":"缺少 bvid 或 epId"}`, http.StatusBadRequest)
		return
	}
	if bvid != "" && cidRaw == "" {
		http.Error(w, `{"error":"缺少 bvid 或 cid"}`, http.StatusBadRequest)
		return
	}
	cookie := a.resolveCookie(r)
	if cookie == "" {
		// 房主直链模式下 CLI 不负责解析，给一条能指导下一步的提示，
		// 避免用户以为是 Cookie 没填对。
		if a.state.Config != nil && normalizeResolveSource(a.state.Config.ResolveSource) == resolveSourceHost {
			http.Error(w, `{"error":"当前为房主直链模式：请在 ZViewer 房间内播放；如需本机解析，请在配置页切换为「本地 Cookie」并登录 B站"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"缺少 B站 Cookie"}`, http.StatusBadRequest)
		return
	}

	var cid int64
	if cidRaw != "" {
		var err error
		cid, err = strconv.ParseInt(cidRaw, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"cid 格式错误"}`, http.StatusBadRequest)
			return
		}
	}
	qn := 0
	if qnRaw != "" {
		qn, _ = strconv.Atoi(qnRaw)
	}

	// 番剧（epId）走 PGC 编排：Url 构造为 ep 链接，cid 可选（定位目标集）
	targetUrl := ""
	if epIdRaw != "" {
		epId, err := strconv.ParseInt(epIdRaw, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"epId 格式错误"}`, http.StatusBadRequest)
			return
		}
		targetUrl = fmt.Sprintf("https://www.bilibili.com/bangumi/play/ep%d", epId)
	} else {
		targetUrl = fmt.Sprintf("https://www.bilibili.com/video/%s", bvid)
	}

	logf("[resolve] 收到解析请求 bvid=%s epId=%s cid=%d qn=%d preferMp4=%v forceDash=%v", bvid, epIdRaw, cid, qn, preferMp4, forceDash)

	// 默认做本地 CDN 探测：CLI 与浏览器在同一台机器，只有本地探测才代表
	// 用户网络；服务器侧探测的是机房出口，结论经常相反。
	// 探测失败不阻断播放（resolver 会按非 mcdn 优先回退），但也绝不能回到
	// 「直接跳过」——跳过会把 mcdn P2P 死链原样交给播放器，表现为点开 502。
	// skipCdnCheck=1 仅保留给排障。
	skipCdnCheck := q.Get("skipCdnCheck") == "1" || q.Get("skipCdnCheck") == "true"

	result, err := ResolveBilibiliVideo(ResolveOptions{
		Url:          targetUrl,
		Cookie:       cookie,
		Qn:           qn,
		PreferMp4:    preferMp4,
		ForceDash:    forceDash,
		Cid:          cid,
		SkipCdnCheck: skipCdnCheck,
	})
	if err != nil {
		logf("解析失败: %v", err)
		code := http.StatusInternalServerError
		if re, ok := err.(*ResolveError); ok {
			switch re.Code {
			case "NOT_LOGGED_IN", "NO_PERMISSION":
				code = http.StatusUnauthorized
			case "VIDEO_NOT_FOUND", "INVALID_INPUT", "EP_NOT_FOUND":
				code = http.StatusBadRequest
			}
		}
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), code)
		return
	}

	// 解析结果日志：输出标题、格式、清晰度、时长、可用清晰度列表等关键信息，
	// 便于在 CLI 终端直观确认解析是否符合预期（如 MP4 限 720P、DASH 高画质等）。
	qnLabel := fmt.Sprintf("qn=%d", result.CurrentQn)
	for _, q := range result.AcceptQuality {
		if q.ID == result.CurrentQn {
			qnLabel = fmt.Sprintf("qn=%d(%s)", q.ID, q.Label)
			break
		}
	}
	durationStr := "-"
	if result.Duration > 0 {
		m := result.Duration / 60
		s := result.Duration % 60
		durationStr = fmt.Sprintf("%02d:%02d", m, s)
	}
	acceptLabels := make([]string, 0, len(result.AcceptQuality))
	for _, q := range result.AcceptQuality {
		acceptLabels = append(acceptLabels, fmt.Sprintf("%d(%s)", q.ID, q.Label))
	}
	logf("[resolve] 解析成功: title=%q duration=%s format=%s %s",
		result.Title, durationStr, result.Format, qnLabel)
	logf("[resolve] 可用清晰度: %s", strings.Join(acceptLabels, ", "))
	logf("[resolve] 视频流: %s", result.VideoUrl)
	if result.AudioUrl != "" {
		logf("[resolve] 音频流: %s", result.AudioUrl)
	}

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

// candidateItem 是一条主 URL 与它的回退候选（同一轨道上的不同 CDN 节点）。
type candidateItem struct {
	PrimaryUrl string   `json:"primaryUrl"`
	BackupUrls []string `json:"backupUrls"`
}

// isProxyableMediaUrl 校验候选地址：只接受 http/https 且长度合理。
func isProxyableMediaUrl(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > candidateMaxURLLength {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// handleCandidates 登记「主 URL → 候选 URL 列表」，供 /proxy 失败换节点。
//
// 房主直链模式（服务端解析 + 本地代理拉流）下，客户端只知道一个媒体地址，
// 候选表不会被 /resolve 写入——某个 CDN 节点回 5xx 时 /proxy 没有节点可换，
// 一个分段 502 就会让整段视频黑屏。前端解析成功后把服务端下发的候选 POST
// 到这里，换节点的能力才真正闭环。
func (a *Agent) handleCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		jsonResponse(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "只支持 POST"})
		return
	}
	var payload struct {
		Items []candidateItem `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, candidateMaxBodyBytes)).Decode(&payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "message": "请求体解析失败"})
		return
	}

	registered := 0
	for _, item := range payload.Items {
		primary := strings.TrimSpace(item.PrimaryUrl)
		if !isProxyableMediaUrl(primary) {
			continue
		}
		cleaned := make([]string, 0, len(item.BackupUrls))
		for _, backup := range item.BackupUrls {
			if !isProxyableMediaUrl(backup) {
				continue
			}
			cleaned = append(cleaned, strings.TrimSpace(backup))
			if len(cleaned) >= maxCandidatesPerTrack {
				break
			}
		}
		if len(cleaned) == 0 {
			continue
		}
		a.setBackupUrls(primary, cleaned)
		registered++
	}
	logf("[candidates] 登记候选表 %d/%d 条", registered, len(payload.Items))
	jsonResponse(w, http.StatusOK, map[string]any{"success": true, "registered": registered})
}

func (a *Agent) handleProxy(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, `{"error":"缺少 url 参数"}`, http.StatusBadRequest)
		return
	}

	// ref=live：直播流（B站直播 CDN 与点播是两套防盗链口径）。
	// 直播是长连接、永远不「响应完」，因此换用不带总超时的客户端，
	// 否则观众每看满 60 秒就会被这个代理掐断一次。
	live := r.URL.Query().Get("ref") == "live"

	// 候选 URL：主 URL + backup URL。主 URL 失败时按顺序尝试其他 CDN。
	// 直播地址带签名、没有候选概念，缓存里查不到就是自己一条。
	candidates := append([]string{target}, a.getBackupUrls(target)...)
	if live {
		candidates = []string{target}
	}
	logf("[proxy] 收到代理请求，共 %d 个候选 URL（live=%v）", len(candidates), live)

	var lastErr error
	// lastStatus 记录最后一个候选的 5xx：全部候选都 5xx 时透传它，
	// 比笼统的 502 更能说明上游真实状态。
	lastStatus := 0
	for i, candidate := range candidates {
		res, err := a.doProxyRequest(w, r, candidate, live)
		if err != nil {
			lastErr = err
			var statusErr *proxyStatusError
			if errors.As(err, &statusErr) {
				lastStatus = statusErr.code
			}
			// 客户端已断开时不继续尝试，直接返回
			if r.Context().Err() != nil {
				return
			}
			// 连接类错误（拒绝连接、超时、DNS 失败）与上游 5xx 都换下一个
			// CDN 重试。这条是 mcdn 502 的正解：mcdn 回源失败时 upos 往往正常，
			// 一个分段 502 不该让整段视频黑屏。
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
	if lastStatus != 0 {
		logf("[proxy] 所有候选均失败，透传上游状态码 %d", lastStatus)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "所有 CDN 候选均不可用"), lastStatus)
		return
	}
	if lastErr != nil {
		logf("[proxy] 所有候选均失败 (%d 个): %v", len(candidates), lastErr)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, lastErr.Error()), http.StatusBadGateway)
		return
	}
	http.Error(w, `{"error":"代理请求失败"}`, http.StatusBadGateway)
}

// proxyStatusError 表示上游返回了 5xx：属于「换下一个候选可能就好了」的失败，
// 因此不写响应头、交给外层循环重试。
type proxyStatusError struct {
	code   int
	target string
}

func (e *proxyStatusError) Error() string {
	return fmt.Sprintf("上游返回 %d（%s）", e.code, hostFromURL(e.target))
}

// doProxyRequest 执行单次上游代理请求。若成功已将响应写入 w 并返回非 nil res；
// 若发生可重试的网络/连接错误返回 err；上游返回非 2xx 时直接透传状态码并返回 nil err。
func (a *Agent) doProxyRequest(w http.ResponseWriter, r *http.Request, target string, live bool) (*http.Response, error) {
	// 使用 context.Background() 而非 r.Context()，避免 dash.js seek 取消
	// segment 请求时代理也被连锁取消。客户端断连时 io.Copy 会自然终止。
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		logf("代理请求构造失败: %v", err)
		return nil, err
	}

	// 上游请求头：与 backend http-proxy.ts 对齐，注入 B站 Referer/Origin 防盗链。
	// 直播与点播的 Referer 不同域（live. / www.），用错会被 CDN 判 403。
	referer := vodReferer
	client := proxyHTTPClient
	if live {
		referer = liveReferer
		client = liveProxyClient
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", referer)
	req.Header.Set("Origin", referer)
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// 上游 5xx 属于「换个候选可能就好」：mcdn 回源失败常见 502，而同一时刻
	// upos 正常。此时还没写响应头，交给外层循环换下一个候选重试。
	if res.StatusCode >= 500 {
		return nil, &proxyStatusError{code: res.StatusCode, target: target}
	}

	// 其余非 2xx 直接透传，让客户端（dash.js / video 标签）自行处理。
	// 4xx 多半是防盗链或签名过期，换节点也救不回来。
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		w.WriteHeader(res.StatusCode)
		return res, nil
	}

	// Content-Type 纠正：B站 CDN 偶发返回 application/json（实际是视频数据），
	// 使用兜底类型避免 MSE 引擎因类型不匹配拒绝处理。直播流同理兜底 video/x-flv。
	upstreamContentType := res.Header.Get("Content-Type")
	defaultContentType := "video/mp4"
	if live {
		defaultContentType = "video/x-flv"
	}
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
	copyStreaming(w, logReader)
	logReader.finish()
	return res, nil
}

// copyStreaming 把上游响应体写给客户端，每读必写、写后立即 flush。
//
// 与点播分段不同，直播是一根实时吐数据的流：Go 的 ResponseWriter 自带
// 缓冲，小段数据会攒在缓冲里等下一次写满才上线——观众侧表现为起播慢、
// 码率低谷期画面一顿一顿。后端 streamBody（internal/proxy/upstream.go）
// 同样逐次 flush，这里必须对齐。缓冲取 64KB：大于 FLV 直播常见的单次
// 到达量，同时避免过小缓冲带来的高频系统调用。
func copyStreaming(w http.ResponseWriter, body io.Reader) {
	buf := make([]byte, 64<<10)
	rc := http.NewResponseController(w)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				// 客户端断开（关闭页面 / flv.js 重连）：终止转发即可。
				return
			}
			_ = rc.Flush()
		}
		if readErr != nil {
			return
		}
	}
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
