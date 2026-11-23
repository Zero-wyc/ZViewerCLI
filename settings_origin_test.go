package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newProtectedHandler 构造一个只记录是否被放行的受保护 handler。
func newProtectedHandler(port int) (http.Handler, *bool) {
	passed := new(bool)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*passed = true
		w.WriteHeader(http.StatusOK)
	})
	return protectLocalSettings(next, port), passed
}

// TestProtectLocalSettingsAllowsLocalPage 验证本机设置页的正常访问不被误伤。
func TestProtectLocalSettingsAllowsLocalPage(t *testing.T) {
	const port = 9333
	cases := []struct {
		name   string
		host   string
		origin string
	}{
		{"127.0.0.1 + 同源 Origin", "127.0.0.1:9333", "http://127.0.0.1:9333"},
		{"localhost + 同源 Origin", "localhost:9333", "http://localhost:9333"},
		{"大写 Localhost", "LocalHost:9333", "http://LOCALHOST:9333"},
		{"IPv6 回环", "[::1]:9333", "http://[::1]:9333"},
		{"无 Origin（curl / GUI）", "127.0.0.1:9333", ""},
		{"空 Origin 头", "127.0.0.1:9333", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, passed := newProtectedHandler(port)
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9333/api/config", nil)
			request.Host = tc.host
			if tc.origin == "empty" {
				request.Header.Set("Origin", "")
			} else if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || !*passed {
				t.Fatalf("状态码 = %d，期望 200（body=%s）", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestProtectLocalSettingsBlocksCrossSite 是这条防护的核心用例：
// 浏览器里任意网页都能向本机端口发请求，凭证接口必须拒绝跨站来源。
func TestProtectLocalSettingsBlocksCrossSite(t *testing.T) {
	const port = 9333
	cases := []struct {
		name   string
		host   string
		origin []string
	}{
		{"恶意页面 Origin", "127.0.0.1:9333", []string{"https://evil.example"}},
		{"同机不同端口", "127.0.0.1:9333", []string{"http://127.0.0.1:3333"}},
		{"https 的伪造本机 Origin", "127.0.0.1:9333", []string{"https://127.0.0.1:9333"}},
		{"多个 Origin 头", "127.0.0.1:9333", []string{"http://127.0.0.1:9333", "https://evil.example"}},
		{"Origin 带路径", "127.0.0.1:9333", []string{"http://127.0.0.1:9333/evil"}},
		// DNS rebinding：攻击者域名解析到 127.0.0.1，Origin 与 Host 都是自己的域名。
		{"rebinding 的 Host 与 Origin", "evil.example:9333", []string{"http://evil.example:9333"}},
		{"只有 Host 被改写", "evil.example:9333", nil},
		{"端口不匹配", "127.0.0.1:9334", []string{"http://127.0.0.1:9334"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, passed := newProtectedHandler(port)
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9333/api/config", nil)
			request.Host = tc.host
			for _, origin := range tc.origin {
				request.Header.Add("Origin", origin)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("状态码 = %d，期望 403", recorder.Code)
			}
			if *passed {
				t.Fatal("请求被放行，防护没有生效")
			}
		})
	}
}

// TestProtectLocalSettingsCoversAllCredentialPaths 确保新增凭证接口时不会漏掉。
func TestProtectLocalSettingsCoversAllCredentialPaths(t *testing.T) {
	for path := range credentialPaths {
		handler, passed := newProtectedHandler(9333)
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9333"+path, nil)
		request.Header.Set("Origin", "https://evil.example")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || *passed {
			t.Fatalf("%s 未被保护（状态码 %d）", path, recorder.Code)
		}
	}
}

// TestProtectLocalSettingsLeavesMediaRoutesOpen 验证媒体路由仍可被 ZViewer 网页跨源调用。
// 这里一旦加严，播放会直接断——比凭证泄露更容易被误判成「偶发故障」。
func TestProtectLocalSettingsLeavesMediaRoutesOpen(t *testing.T) {
	for _, path := range []string{"/proxy", "/resolve", "/candidates", "/health", "/"} {
		handler, passed := newProtectedHandler(9333)
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9333"+path, nil)
		request.Header.Set("Origin", "http://127.0.0.1:3333")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !*passed {
			t.Fatalf("%s 被误拦（状态码 %d）", path, recorder.Code)
		}
	}
}

// TestProtectLocalSettingsAcceptsDefaultPort 验证端口 80 时浏览器省略端口的写法。
func TestProtectLocalSettingsAcceptsDefaultPort(t *testing.T) {
	handler, passed := newProtectedHandler(80)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/config", nil)
	request.Host = "127.0.0.1"
	request.Header.Set("Origin", "http://localhost")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !*passed {
		t.Fatalf("状态码 = %d，期望 200", recorder.Code)
	}

	// 非 80 端口时不能接受无端口写法，否则 Host 校验形同虚设。
	handler, _ = newProtectedHandler(9333)
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/config", nil)
	request.Host = "127.0.0.1"
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("状态码 = %d，期望 403", recorder.Code)
	}
}

// TestSetupPageCarriesDesktopTheme 验证设置页内联了桌面主题，且默认不启用。
//
// 主题仍由 desktop=1 控制：独立使用 CLI 时保持它自己的深色页面。
func TestSetupPageCarriesDesktopTheme(t *testing.T) {
	if !strings.Contains(setupPageHTML, "html.desktop-embedded") {
		t.Fatal("设置页缺少桌面主题样式")
	}
	if !strings.Contains(setupPageHTML, "params.get('desktop') === '1'") {
		t.Fatal("设置页缺少 desktop=1 开关")
	}
	// 不能无条件给 <html> 加类，否则独立 CLI 会被换成桌面配色。
	if strings.Contains(setupPageHTML, `<html lang="zh-CN" class="desktop-embedded">`) {
		t.Fatal("主题不应默认启用")
	}
}
