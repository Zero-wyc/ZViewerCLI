package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
	Connected   bool                   `json:"connected"`
	Connecting  bool                   `json:"connecting"`
	Config      *LocalConfig           `json:"config"`
	UserInfo    map[string]any         `json:"userInfo"`
	CookieValid *bool                  `json:"cookieValid"`
	LastError   string                 `json:"lastError"`
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
	headers := http.Header{}
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	conn, _, err := dialer.Dial(wsURL, headers)
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

	// 读取 Engine.IO open 包
	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if len(msg) == 0 || msg[0] != '0' {
		conn.Close()
		return nil, fmt.Errorf("未收到 Engine.IO open 包")
	}

	// 连接默认 namespace 并携带 auth
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
		"roomId":  c.roomID,
		"proxyUrl": c.proxyURL,
		"agent":   "zcontrol-cli",
		"version": "0.1.0",
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
	// Engine.IO 包类型为第一个字符，后面是 Socket.IO 包
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

func init() {
	// 抑制 websocket 库的 log（可选）
	log.SetFlags(0)
}
