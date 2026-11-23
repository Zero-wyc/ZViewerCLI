package main

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// 带凭证的本地配置接口：读写 B站 Cookie、扫码登录、连接与断开服务器。
// 这些路径一旦被跨站请求打到，等于账号被接管，因此单独加来源校验。
var credentialPaths = map[string]bool{
	"/api/config":     true,
	"/api/qr":         true,
	"/api/qr/poll":    true,
	"/api/connect":    true,
	"/api/disconnect": true,
}

// protectLocalSettings 把带凭证的配置接口限制在本机设置页，其余路由原样放行。
//
// 「只监听 127.0.0.1」并不等于安全：用户浏览器里任意网页都能向本机端口发请求
// （CORS 只限制读响应，不限制发请求），攻击者还可以用自己的域名解析到 127.0.0.1
// （DNS rebinding）绕开同源策略。所以这里同时校验两条：
//
//   - Host 必须是本机回环地址 —— 挡 DNS rebinding
//   - Origin 为空（curl、GUI 这类非浏览器客户端）或本机页面 —— 挡跨站请求
//
// 媒体路由（/proxy、/resolve、/candidates）不在此列：它们不含凭证，需要被
// ZViewer 网页跨源调用，加上校验会直接打断播放。
//
// 包装必须放在 CORS 中间件外层，这样预检请求也一并受校验。
func protectLocalSettings(next http.Handler, port int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !credentialPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if !isLoopbackHost(r.Host, port) || !isLocalOrigin(r.Header.Values("Origin"), port) {
			http.Error(w, "Local CLI settings origin is not permitted", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost 判断 HTTP Host 是否指向本机端口。
func isLoopbackHost(host string, port int) bool {
	hostname, rawPort, err := net.SplitHostPort(host)
	if err != nil {
		// 没有端口：只有默认端口 80 时浏览器才会这么发。
		if port != 80 {
			return false
		}
		hostname, rawPort = host, "80"
	}
	if rawPort != strconv.Itoa(port) {
		return false
	}
	return isLoopbackName(hostname)
}

// isLocalOrigin 判断 Origin 头是否来自本机页面。
//
// 非浏览器客户端通常不带 Origin；个别客户端会发空字符串，一并按本机处理。
func isLocalOrigin(origins []string, port int) bool {
	if len(origins) == 0 {
		return true
	}
	if len(origins) > 1 {
		return false
	}
	origin := strings.TrimSpace(origins[0])
	if origin == "" {
		return true
	}
	// 统一按小写比较：scheme 与 host 本就大小写不敏感。
	origin = strings.ToLower(origin)
	if !strings.HasPrefix(origin, "http://") {
		return false
	}
	host := strings.TrimPrefix(origin, "http://")
	if strings.Contains(host, "/") {
		return false
	}
	return isLoopbackHost(host, port)
}

// isLoopbackName 只认回环字面量，不做 DNS 解析：解析会给攻击者留下 rebinding 窗口。
func isLoopbackName(hostname string) bool {
	switch strings.ToLower(hostname) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}
