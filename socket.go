package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SocketClient 是一个最小化的 Socket.IO v4 客户端。
type SocketClient struct {
	conn           *websocket.Conn
	writeMu        sync.Mutex
	serverURL      string
	roomID         string
	proxyURL       string
	state          *State
	onDisconnect   func()
	stopKeepAlive  chan struct{}
	pingInterval   time.Duration
	pingTimeout    time.Duration
	lastActivityAt time.Time
}

// engineOpenPacket 是 Engine.IO open 包中的 JSON 内容。
type engineOpenPacket struct {
	Sid          string        `json:"sid"`
	Upgrades     []string      `json:"upgrades"`
	PingInterval time.Duration `json:"pingInterval"`
	PingTimeout  time.Duration `json:"pingTimeout"`
	MaxPayload   int           `json:"maxPayload"`
}

// writeDeadline 是单次 WebSocket 写操作的最大等待时间。
const writeDeadline = 10 * time.Second

// defaultPingInterval / defaultPingTimeout 在服务器未返回时使用。
const defaultPingInterval = 25 * time.Second
const defaultPingTimeout = 20 * time.Second

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
		conn:           conn,
		serverURL:      serverURL,
		roomID:         roomID,
		proxyURL:       proxyURL,
		state:          state,
		stopKeepAlive:  make(chan struct{}),
		pingInterval:   defaultPingInterval,
		pingTimeout:    defaultPingTimeout,
		lastActivityAt: time.Now(),
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

	var open engineOpenPacket
	if err := json.Unmarshal(msg[1:], &open); err == nil {
		if open.PingInterval > 0 {
			c.pingInterval = open.PingInterval * time.Millisecond
		}
		if open.PingTimeout > 0 {
			c.pingTimeout = open.PingTimeout * time.Millisecond
		}
	} else {
		logf("解析 Engine.IO open 包失败，使用默认心跳参数: %v", err)
	}

	if err := c.writeText(`40{"agent":"zcontrol-cli"}`); err != nil {
		conn.Close()
		return nil, err
	}

	go c.readLoop()
	go c.keepAlive()
	return c, nil
}

func (c *SocketClient) writeText(s string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	err := c.conn.WriteMessage(websocket.TextMessage, []byte(s))
	// 写操作完成后清除 deadline，避免影响后续读取/写入
	_ = c.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return err
	}
	c.lastActivityAt = time.Now()
	return nil
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

// keepAlive 在 Engine.IO/Socket.IO v4 中被动维持长连接。
//
// Engine.IO v4 的心跳由服务器主导：服务器按 pingInterval 发送 '2' ping，
// 客户端必须在 pingTimeout 内回复 '3' pong。客户端主动发送 '2' 不会得到
// 服务器响应，反而可能被服务端判定为协议错误而关闭连接。
//
// 因此本函数不主动发 ping，只做两件事：
// 1. 监控 lastActivityAt，若超过 pingInterval+pingTimeout 未收到任何消息，
//    说明连接已半死，强制关闭以触发重连。
// 2. 定期（取 pingInterval/2）尝试一次无数据写超时探测，提前发现 write 阻塞。
func (c *SocketClient) keepAlive() {
	checkInterval := c.pingInterval / 2
	if checkInterval < 10*time.Second {
		checkInterval = 10 * time.Second
	}
	if checkInterval > 30*time.Second {
		checkInterval = 30 * time.Second
	}
	deadline := c.pingInterval + c.pingTimeout
	if deadline < 45*time.Second {
		deadline = 45 * time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Since(c.lastActivityAt) > deadline {
				logf("连接超过 %v 无活动，主动关闭重连", deadline)
				c.conn.Close()
				return
			}
		case <-c.stopKeepAlive:
			return
		}
	}
}

func (c *SocketClient) readLoop() {
	defer func() {
		c.state.SetConnected(false)
		close(c.stopKeepAlive)
		if c.onDisconnect != nil {
			c.onDisconnect()
		}
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			logf("Socket 断开: %v", err)
			return
		}
		if len(msg) == 0 {
			continue
		}
		c.lastActivityAt = time.Now()
		c.handleMessage(string(msg))
	}
}

func (c *SocketClient) handleMessage(msg string) {
	if len(msg) < 1 {
		return
	}
	engineType := msg[0]
	rest := msg[1:]

	switch engineType {
	case '2': // ping from server: 必须在 pingTimeout 内回复 pong
		if err := c.writeText("3"); err != nil {
			logf("[ping] pong 回复失败: %v", err)
		}
	case '3': // pong from server (reply to our proactive ping, if any)
		// ignore
	case '4': // message
		c.handleSocketPacket(rest)
	default:
		logf("[engine] 未识别包类型: %c", engineType)
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
	// 统一解析为 map，各事件按需读取字段
	var m map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &m)
	}

	switch event {
	// ── CLI 自身 ──────────────────────────────────────────
	case "cli-registered":
		logf("[CLI] 已注册到房间: %s", c.roomID)
		return
	case "cli-error":
		logf("[CLI] 错误: %v", m["message"])
		return
	case "cli-agent-available":
		logf("[CLI] 代理上线: socketId=%v proxyUrl=%v", m["socketId"], m["proxyUrl"])
		return
	case "cli-agent-unavailable":
		logf("[CLI] 代理下线: socketId=%v", m["socketId"])
		return
	case "cli-agents":
		agents, _ := m["agents"].([]any)
		logf("[CLI] 房间内代理列表: %d 个", len(agents))
		return

	// ── 播放状态与控制 ────────────────────────────────────
	case "watch-together-state":
		// 房主广播的完整播放状态（含 sourceUrl / currentTime / isPlaying 等）
		state, _ := m["state"].(map[string]any)
		if state == nil {
			// 兼容直接平铺的 payload
			state = m
		}
		source := ""
		if v, ok := state["sourceUrl"].(string); ok && v != "" {
			source = v
			if len(source) > 50 {
				source = source[:50] + "..."
			}
		}
		logf("[播放] 状态同步 currentTime=%vs isPlaying=%v sourceUrl=%s",
			state["currentTime"], state["isPlaying"], source)
		return
	case "watch-together-control":
		// 房主控制指令（play / pause / seek / rate），亚 500ms 广播
		action, _ := m["action"].(string)
		value := m["value"]
		switch action {
		case "play":
			logf("[播放] ▶ 房主继续播放")
		case "pause":
			logf("[播放] ⏸ 房主暂停")
		case "seek":
			logf("[播放] 🔀 房主跳转到 %vs", value)
		case "rate":
			logf("[播放] ⏩ 房主切换倍速 %vx", value)
		default:
			logf("[播放] 控制指令: action=%s value=%v", action, value)
		}
		return
	case "watch-together-request-state":
		logf("[播放] 观众/房主请求当前播放状态")
		return

	// ── 观众申请 ──────────────────────────────────────────
	case "seek-request":
		logf("[申请] 观众 %v 请求跳转到 %vs", m["viewerUsername"], m["time"])
		return
	case "seek-response":
		logf("[申请] 房主 %s 跳转申请", acceptReject(m["accept"]))
		return
	case "pause-request":
		logf("[申请] 观众 %v 请求暂停", m["viewerUsername"])
		return
	case "pause-response":
		logf("[申请] 房主 %s 暂停申请", acceptReject(m["accept"]))
		return
	case "play-request":
		logf("[申请] 观众 %v 请求继续播放", m["viewerUsername"])
		return
	case "play-response":
		logf("[申请] 房主 %s 继续播放申请", acceptReject(m["accept"]))
		return

	// ── 心跳 ──────────────────────────────────────────────
	case "host-heartbeat":
		logf("[心跳] 房主: currentTime=%vs isPlaying=%v", m["currentTime"], m["isPlaying"])
		return
	case "server-heartbeat":
		logf("[心跳] 服务器: currentTime=%vs isPlaying=%v", m["currentTime"], m["isPlaying"])
		return

	// ── 影片列表 ──────────────────────────────────────────
	case "movie-list":
		movies, _ := m["movies"].([]any)
		logf("[影片] 列表更新: %d 部", len(movies))
		return
	case "current-movie":
		logf("[影片] 切换当前播放: movieId=%v", m["movieId"])
		return
	case "preview-source":
		source, _ := m["source"].(map[string]any)
		title, _ := source["title"].(string)
		logf("[影片] 预览源: title=%s", title)
		return
	case "play-movie":
		// play-movie 是客户端 emit 给后端的事件，后端不广播；
		// 但 current-movie 会被广播，此处保留以防未来扩展
		return

	// ── 观众管理 ──────────────────────────────────────────
	case "viewer-joined":
		name, _ := m["viewerUsername"].(string)
		if name == "" {
			name, _ = m["username"].(string)
		}
		logf("[观众] + 加入: %v", name)
		return
	case "viewer-left":
		logf("[观众] - 离开: %v", m["viewerSocketId"])
		return
	case "join-request":
		logf("[观众] 请求加入房间: %v", m["viewerSocketId"])
		return
	case "join-approved":
		logf("[观众] 加入已批准")
		return
	case "join-rejected":
		logf("[观众] 加入已拒绝")
		return
	case "viewer-muted":
		logf("[观众] 禁言状态变更: %v muted=%v", m["viewerSocketId"], m["muted"])
		return
	case "viewer-kicked":
		logf("[观众] 被踢出")
		return
	case "host-transferred":
		logf("[房间] 房主转移: newHost=%v", m["newHostSocketId"])
		return

	// ── 房间生命周期 ──────────────────────────────────────
	case "host-disconnected":
		logf("[房间] 房主已断开")
		return
	case "room-closed":
		logf("[房间] 房间已关闭: %v", m["roomId"])
		return
	case "room-mode-changed":
		logf("[房间] 模式切换: %v", m["mode"])
		return
	case "room-name-updated":
		logf("[房间] 名称更新: %v", m["name"])
		return
	case "room-settings-updated":
		logf("[房间] 设置更新: %v", m["settings"])
		return
	case "sharer-ready":
		logf("[房间] 房主就绪")
		return
	case "share-method-changed":
		logf("[房间] 共享方式变更: %v", m["method"])
		return
	case "stream-status":
		logf("[房间] 推流状态: %v", m["status"])
		return
	case "p2p-mode-change":
		logf("[房间] P2P 模式变更: %v", m["enabled"])
		return

	// ── 弹幕 / 评论 / 标注 ────────────────────────────────
	case "danmaku":
		logf("[弹幕] %v: %v", m["sender"], m["content"])
		return
	case "new-comment":
		logf("[评论] %v: %v", m["username"], m["content"])
		return
	case "annotation-stroke":
		logf("[标注] 新笔画")
		return
	case "clear-annotations":
		logf("[标注] 清空所有标注")
		return

	// ── 轨道同步 ──────────────────────────────────────────
	case "track-change":
		trackType, _ := m["type"].(string)
		logf("[轨道] 切换: type=%s trackId=%v", trackType, m["trackId"])
		return

	default:
		// 未识别的事件，输出事件名和原始数据（截断）
		raw := string(data)
		if len(raw) > 100 {
			raw = raw[:100] + "..."
		}
		logf("[事件] %s: %s", event, raw)
	}
}

// acceptReject 将布尔值转换为"已同意/已拒绝"文本
func acceptReject(v any) string {
	if b, ok := v.(bool); ok && b {
		return "已同意"
	}
	return "已拒绝"
}
