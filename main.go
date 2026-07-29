package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	version   = "0.1.0"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

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
