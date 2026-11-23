package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestIsMcdnMediaUrl 验证 mcdn 域名识别：只认 P2P 回源节点，不误伤形近域名。
func TestIsMcdnMediaUrl(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"根域", "https://mcdn.bilivideo.cn/video.m4s", true},
		{"子域带端口", "https://xy1x2x3.mcdn.bilivideo.cn:8082/video.m4s", true},
		{"大写域名", "https://XY.MCDN.BILIVIDEO.CN/video.m4s", true},
		{"upos 不算", "https://upos-sz-mirrorcos.bilivideo.com/video.m4s", false},
		{"形近域名不算", "https://notmcdn.bilivideo.cn/video.m4s", false},
		{"空串", "", false},
	}
	for _, c := range cases {
		if got := isMcdnMediaUrl(c.raw); got != c.want {
			t.Errorf("%s: isMcdnMediaUrl(%q) = %v，期望 %v", c.name, c.raw, got, c.want)
		}
	}
}

// TestOrderMediaCandidates 验证「非 mcdn 优先」是稳定分区：只挪 mcdn，其余保持原序。
func TestOrderMediaCandidates(t *testing.T) {
	const (
		mcdn1  = "https://a.mcdn.bilivideo.cn:8082/v1.m4s"
		upos1  = "https://upos-sz-mirrorcos.bilivideo.com/v1.m4s"
		upos2  = "https://upos-sz-mirrorali.bilivideo.com/v1.m4s"
		mcdn2  = "https://b.mcdn.bilivideo.cn:8082/v1.m4s"
		direct = "https://cn-hbxy-cu-01.bilivideo.com/v1.m4s"
	)
	got := orderMediaCandidates(mcdn1, []string{upos1, mcdn2, upos2})
	want := []string{upos1, upos2, mcdn1, mcdn2}
	if len(got) != len(want) {
		t.Fatalf("候选数 = %d，期望 %d（%v）", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 个候选 = %q，期望 %q（完整结果 %v）", i, got[i], want[i], got)
		}
	}

	// 全非 mcdn：顺序必须原样保留。
	got = orderMediaCandidates(direct, []string{upos1, upos2})
	if got[0] != direct || got[1] != upos1 || got[2] != upos2 {
		t.Fatalf("全非 mcdn 时顺序被改变: %v", got)
	}

	if got := orderMediaCandidates("", nil); len(got) != 0 {
		t.Fatalf("空输入应返回空切片，实际 %v", got)
	}
}

// TestPickPreferredMediaUrl 验证跳过探测时不会把 mcdn 死链排在最前。
func TestPickPreferredMediaUrl(t *testing.T) {
	if got := pickPreferredMediaUrl(
		"https://xy1.mcdn.bilivideo.cn:8082/video.m4s",
		[]string{"https://upos-sz-mirrorcos.bilivideo.com/video.m4s"},
	); got != "https://upos-sz-mirrorcos.bilivideo.com/video.m4s" {
		t.Fatalf("应优先选非 mcdn 候选，实际 %q", got)
	}

	// 全部是 mcdn 时退回第一个（总比没有地址强）。
	if got := pickPreferredMediaUrl(
		"https://a.mcdn.bilivideo.cn:8082/video.m4s",
		[]string{"https://b.mcdn.bilivideo.cn:8082/video.m4s"},
	); got != "https://a.mcdn.bilivideo.cn:8082/video.m4s" {
		t.Fatalf("全 mcdn 时应退回 baseUrl，实际 %q", got)
	}

	if got := pickPreferredMediaUrl("", nil); got != "" {
		t.Fatalf("空输入应返回空串，实际 %q", got)
	}
}

// TestFindReachableMediaUrlSkipsDeadCandidate 验证探测会跳过返回 5xx 的候选，
// 而不是把第一个候选当成可用（这就是 mcdn 502 时必须换节点的那条路径）。
func TestFindReachableMediaUrlSkipsDeadCandidate(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer dead.Close()
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer alive.Close()

	got, err := findReachableMediaUrl(dead.URL+"/v.m4s", []string{alive.URL + "/v.m4s"})
	if err != nil {
		t.Fatalf("应找到可用候选，实际报错: %v", err)
	}
	if got != alive.URL+"/v.m4s" {
		t.Fatalf("应选中 200 的候选，实际 %q", got)
	}
}

// TestFindReachableMediaUrlAllDead 验证全不可达时返回错误（由上层决定回退策略）。
func TestFindReachableMediaUrlAllDead(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer dead.Close()

	if _, err := findReachableMediaUrl(dead.URL+"/v.m4s", nil); err == nil {
		t.Fatal("全不可达时应返回错误")
	}
}

// TestDoProxyRequestRetriesOn5xx 验证上游 5xx 会被当成可重试失败返回，
// 而不是把状态码直接写给客户端（否则换节点的那层循环永远不会触发）。
func TestDoProxyRequestRetriesOn5xx(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()

	agent := &Agent{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/proxy?url=x", nil)

	_, err := agent.doProxyRequest(recorder, request, upstream.URL+"/v.m4s", false)
	var statusErr *proxyStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("5xx 应返回 proxyStatusError，实际 %v", err)
	}
	if statusErr.code != http.StatusBadGateway {
		t.Fatalf("状态码 = %d，期望 502", statusErr.code)
	}
	// 关键：不能提前写响应头，否则外层无法重试。
	if recorder.Code != http.StatusOK {
		t.Fatalf("重试前不应写响应头，实际 code=%d", recorder.Code)
	}
}

// TestDoProxyRequestPassesThrough4xx 验证 4xx 仍直接透传（防盗链/签名问题换节点无用）。
func TestDoProxyRequestPassesThrough4xx(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer upstream.Close()

	agent := &Agent{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/proxy?url=x", nil)

	if _, err := agent.doProxyRequest(recorder, request, upstream.URL+"/v.m4s", false); err != nil {
		t.Fatalf("4xx 不应返回错误，实际 %v", err)
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("状态码 = %d，期望 403", recorder.Code)
	}
}

// postCandidates 向 handleCandidates 发一次请求，返回响应记录器。
func postCandidates(t *testing.T, agent *Agent, method string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body string
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("构造请求体失败: %v", err)
		}
		body = string(encoded)
	}
	request := httptest.NewRequest(method, "http://127.0.0.1/candidates", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	agent.handleCandidates(recorder, request)
	return recorder
}

// TestHandleCandidatesRegistersBackups 验证前端下发的候选表能被 /proxy 取到。
func TestHandleCandidatesRegistersBackups(t *testing.T) {
	agent := newAgent(0, nil)
	const (
		primary = "https://upos-sz-mirrorcos.bilivideo.com/v1.m4s"
		backup  = "https://upos-sz-mirrorali.bilivideo.com/v1.m4s"
	)
	recorder := postCandidates(t, agent, http.MethodPost, map[string]any{
		"items": []map[string]any{{
			"primaryUrl": primary,
			"backupUrls": []string{primary, backup, "ftp://example.com/v.m4s", ""},
		}},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200（body=%s）", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Success    bool `json:"success"`
		Registered int  `json:"registered"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	if !resp.Success || resp.Registered != 1 {
		t.Fatalf("登记结果 = %+v，期望 success=true registered=1", resp)
	}

	// 非法 scheme 与空串应被过滤，只剩合法候选；primary 自身会被 getBackupUrls 排除。
	got := agent.getBackupUrls(primary)
	if len(got) != 1 || got[0] != backup {
		t.Fatalf("候选 = %v，期望 [%s]", got, backup)
	}
}

// TestHandleCandidatesRejectsBadRequest 验证方法与非 URL 入参的拒绝路径。
func TestHandleCandidatesRejectsBadRequest(t *testing.T) {
	agent := newAgent(0, nil)

	if recorder := postCandidates(t, agent, http.MethodGet, nil); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 状态码 = %d，期望 405", recorder.Code)
	}
	if recorder := postCandidates(t, agent, http.MethodPost, "not-an-object"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求体状态码 = %d，期望 400", recorder.Code)
	}

	// 全是非法地址：不应写入缓存。
	recorder := postCandidates(t, agent, http.MethodPost, map[string]any{
		"items": []map[string]any{{"primaryUrl": "file:///etc/passwd", "backupUrls": []string{"javascript:alert(1)"}}},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("非法地址不应报错，实际 %d", recorder.Code)
	}
	if got := agent.getBackupUrls("file:///etc/passwd"); len(got) != 0 {
		t.Fatalf("非法地址被写进缓存: %v", got)
	}
}

// TestProxySwitchesToRegisteredCandidate 是这条链路的端到端验证：
// 主节点 502 → /proxy 换到候选节点 → 客户端拿到 200。
// 房主直链模式就是靠这个避免「一个分段 502 整段黑屏」。
func TestProxySwitchesToRegisteredCandidate(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer dead.Close()
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("ALIVE"))
	}))
	defer alive.Close()

	agent := newAgent(0, nil)
	primary := dead.URL + "/v.m4s"
	postCandidates(t, agent, http.MethodPost, map[string]any{
		"items": []map[string]any{{
			"primaryUrl": primary,
			"backupUrls": []string{primary, alive.URL + "/v.m4s"},
		}},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/proxy?url="+url.QueryEscape(primary), nil)
	agent.handleProxy(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望换节点后 200（body=%s）", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "ALIVE" {
		t.Fatalf("响应体 = %q，期望来自候选节点", recorder.Body.String())
	}
}
