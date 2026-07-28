package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/cors"
)

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
		Cookie:   cfg.Cookie,
		SavedAt:  nowISO(),
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
			"version": "0.1.0",
		})
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			jsonResponse(w, http.StatusOK, map[string]any{
				"success":      true,
				"config":       a.state.Config,
				"connected":    a.state.Connected,
				"connecting":   a.state.Connecting,
				"userInfo":     a.state.UserInfo,
				"cookieValid":  a.state.CookieValid,
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
			"success":    true,
			"qrcodeKey":  session.QrcodeKey,
			"qrUrl":      session.QrURL,
			"qrDataUrl":  session.QrDataURL,
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
		req.Header.Set("User-Agent", UserAgent)
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
