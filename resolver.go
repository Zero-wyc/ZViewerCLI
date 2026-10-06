package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ===================== 常量与类型（与 backend resolver.ts 对齐） =====================

const (
	bilibiliRequestTimeoutMs = 10000
	bilibiliMaxRetries       = 3
	cdnHealthCheckTimeoutMs  = 2000
	mp4MaxQn                 = 80 // MP4 直链最高支持 1080P
	defaultQn                = 80
	vipDefaultQn             = 120
	qn8K                     = 127
)

// VIP 专属清晰度列表
var vipOnlyQns = []int{112, 116, 120, 125, 126, 127}

// 未登录用户允许的最高清晰度
const anonymousMaxQn = 32

// qn 到标签/分辨率的兜底映射表
var qnQualityMap = map[int]struct {
	Label      string
	Resolution string
}{
	127: {Label: "8K 超高清", Resolution: "7680x4320"},
	126: {Label: "杜比视界", Resolution: "3840x2160"},
	125: {Label: "HDR 真彩", Resolution: "3840x2160"},
	120: {Label: "4K 超清", Resolution: "3840x2160"},
	116: {Label: "1080P60", Resolution: "1920x1080"},
	112: {Label: "1080P+", Resolution: "1920x1080"},
	80:  {Label: "1080P", Resolution: "1920x1080"},
	74:  {Label: "720P60", Resolution: "1280x720"},
	64:  {Label: "720P", Resolution: "1280x720"},
	32:  {Label: "480P", Resolution: "854x480"},
	16:  {Label: "360P", Resolution: "640x360"},
}

// BilibiliVideoPage 分集信息。
type BilibiliVideoPage struct {
	Cid      int64  `json:"cid"`
	Page     int    `json:"page"`
	Part     string `json:"part"`
	Duration int    `json:"duration"`
}

// BilibiliVideoInfo 视频信息。
type BilibiliVideoInfo struct {
	Bvid     string              `json:"bvid"`
	Aid      int64               `json:"aid"`
	Cid      int64               `json:"cid"`
	Title    string              `json:"title"`
	Pic      string              `json:"pic,omitempty"`
	Duration int                 `json:"duration"`
	Pages    []BilibiliVideoPage `json:"pages"`
}

// DashMediaTrack DASH 轨道。
type DashMediaTrack struct {
	BaseUrl   string   `json:"baseUrl"`
	BackupUrl []string `json:"backupUrl,omitempty"`
	Bandwidth int      `json:"bandwidth"`
	Codecs    string   `json:"codecs"`
	ID        int      `json:"id"`
}

// DurlSegment MP4 直链分片。
type DurlSegment struct {
	Url    string `json:"url"`
	Size   int64  `json:"size"`
	Length int64  `json:"length"`
}

// BilibiliPlayUrlResult 播放地址结果。
type BilibiliPlayUrlResult struct {
	Format       string             `json:"format"`
	Video        []DashMediaTrack   `json:"video,omitempty"`
	Audio        []DashMediaTrack   `json:"audio,omitempty"`
	Durl         []DurlSegment      `json:"durl,omitempty"`
	BestVideo    *DashMediaTrack    `json:"bestVideo,omitempty"`
	BestAudio    *DashMediaTrack    `json:"bestAudio,omitempty"`
	CurrentQn    int                `json:"currentQn,omitempty"`
	AcceptQuality []QualityItem      `json:"acceptQuality,omitempty"`
}

// QualityItem 清晰度项。
type QualityItem struct {
	ID         int    `json:"id"`
	Label      string `json:"label"`
	Resolution string `json:"resolution,omitempty"`
}

// ResolvePageInfo 返回给前端的分集信息。
type ResolvePageInfo struct {
	Page     int    `json:"page"`
	Cid      int64  `json:"cid"`
	Part     string `json:"part"`
	Duration int    `json:"duration"`
	// PGC：番剧分集 ep_id（切集时按 ep 链接重解析）
	EpId int64 `json:"epId,omitempty"`
	// PGC：B站 badge（'会员'/'限免'），空串不序列化
	Badge string `json:"badge,omitempty"`
}

// ResolveResult 解析结果。
type ResolveResult struct {
	Title         string            `json:"title"`
	Duration      int               `json:"duration"`
	Cid           int64             `json:"cid"`
	VideoUrl      string            `json:"videoUrl"`
	AudioUrl      string            `json:"audioUrl,omitempty"`
	VideoCodec    string            `json:"videoCodec,omitempty"`
	AudioCodec    string            `json:"audioCodec,omitempty"`
	Format        string            `json:"format"`
	LoggedIn      bool              `json:"loggedIn"`
	VipStatus     int               `json:"vipStatus"`
	CurrentQn     int               `json:"currentQn,omitempty"`
	AcceptQuality []QualityItem     `json:"acceptQuality,omitempty"`
	Pages         []ResolvePageInfo `json:"pages,omitempty"`
	CurrentPage   int               `json:"currentPage,omitempty"`
	// VideoBackupUrls 存储视频轨道的所有候选 CDN URL（主 URL + backup），供 /proxy 失败重试。
	VideoBackupUrls []string `json:"videoBackupUrls,omitempty"`
	// AudioBackupUrls 存储音频轨道的所有候选 CDN URL（主 URL + backup），供 /proxy 失败重试。
	AudioBackupUrls []string `json:"audioBackupUrls,omitempty"`
	// PGC：当前播放集 ep_id
	EpId int64 `json:"epId,omitempty"`
	// PGC：整季 season_id
	SeasonId int64 `json:"seasonId,omitempty"`
	// PGC：整季标题（番剧/影视名，已拼进 title）
	SeasonTitle string `json:"seasonTitle,omitempty"`
	// PGC：当前集为试看/预览流（会员专享集 + 非大会员账号）
	Preview bool `json:"preview,omitempty"`
}

// ResolveOptions 解析选项。
type ResolveOptions struct {
	Url          string
	Cookie       string
	Qn           int
	Codec        string
	PreferMp4    bool
	Page         int
	Cid          int64
	SkipCdnCheck bool
	// ForceDash 为 true 时强制使用 DASH 格式，禁用 MP4 降级（CLI 代理模式下使用）。
	ForceDash bool
}

// ResolveError 解析错误。
type ResolveError struct {
	Message string
	Code    string
}

func (e *ResolveError) Error() string { return e.Message }

// NoPermissionError 无权限错误。
type NoPermissionError struct {
	Message string
}

func (e *NoPermissionError) Error() string {
	if e.Message == "" {
		return "无权限播放，可能需要登录或大会员"
	}
	return e.Message
}

// ===================== 缓存 =====================

var (
	videoInfoCache   = make(map[string]bilibiliVideoInfoCacheEntry)
	videoInfoCacheMu sync.RWMutex
	vipStatusCache   = make(map[string]vipStatusCacheEntry)
	vipStatusCacheMu sync.RWMutex
)

type bilibiliVideoInfoCacheEntry struct {
	info      *BilibiliVideoInfo
	cachedAt  time.Time
}

type vipStatusCacheEntry struct {
	isVip    bool
	cachedAt time.Time
}

const videoInfoCacheTTL = 2 * time.Minute
const vipStatusCacheTTL = 5 * time.Minute

func normalizeVipCacheKey(cookie string) string {
	re := regexp.MustCompile(`(?:^|;\s*)DedeUserID=(\d+)`)
	m := re.FindStringSubmatch(cookie)
	if len(m) > 1 {
		return "mid:" + m[1]
	}
	return cookie
}

func getCachedVideoInfo(bvid string) *BilibiliVideoInfo {
	videoInfoCacheMu.RLock()
	entry, ok := videoInfoCache[bvid]
	videoInfoCacheMu.RUnlock()
	if !ok || time.Since(entry.cachedAt) > videoInfoCacheTTL {
		return nil
	}
	return entry.info
}

func setCachedVideoInfo(bvid string, info *BilibiliVideoInfo) {
	videoInfoCacheMu.Lock()
	videoInfoCache[bvid] = bilibiliVideoInfoCacheEntry{info: info, cachedAt: time.Now()}
	videoInfoCacheMu.Unlock()
}

func getCachedVipStatus(cookie string) *bool {
	key := normalizeVipCacheKey(cookie)
	vipStatusCacheMu.RLock()
	entry, ok := vipStatusCache[key]
	vipStatusCacheMu.RUnlock()
	if !ok || time.Since(entry.cachedAt) > vipStatusCacheTTL {
		return nil
	}
	v := entry.isVip
	return &v
}

func setCachedVipStatus(cookie string, isVip bool) {
	key := normalizeVipCacheKey(cookie)
	vipStatusCacheMu.Lock()
	vipStatusCache[key] = vipStatusCacheEntry{isVip: isVip, cachedAt: time.Now()}
	vipStatusCacheMu.Unlock()
}

// ===================== bilibiliFetch（与 backend client.ts 对齐） =====================

var anonymousCookieJar string
var anonymousCookieJarMu sync.Mutex

func parseSetCookieHeaderSimple(headers http.Header) string {
	values := headers.Values("Set-Cookie")
	if len(values) == 0 {
		if single := headers.Get("Set-Cookie"); single != "" {
			values = strings.Split(single, ",")
		}
	}
	parts := make([]string, 0, len(values))
	for _, raw := range values {
		for _, c := range strings.Split(raw, ",") {
			c = strings.TrimSpace(c)
			if idx := strings.Index(c, ";"); idx >= 0 {
				c = c[:idx]
			}
			if strings.Contains(c, "=") {
				parts = append(parts, c)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func randomBackoffMs() int {
	return 1000 + rand.Intn(1000)
}

// bilibiliFetch 封装对 B站 API 的请求：自动补充头、412 重试、超时、匿名 Cookie 收集。
// bilibiliHTTPClient 专用于 B站 API 请求的 HTTP 客户端，设置超时避免 context 管理问题。
var bilibiliHTTPClient = &http.Client{
	Timeout: bilibiliRequestTimeoutMs * time.Millisecond,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     60 * time.Second,
	},
}

func bilibiliFetch(api string, cookie string, out any) error {
	var lastErr error
	for attempt := 0; attempt < bilibiliMaxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodGet, api, nil)
		if err != nil {
			return err
		}

		effectiveCookie := cookie
		if effectiveCookie == "" {
			anonymousCookieJarMu.Lock()
			effectiveCookie = anonymousCookieJar
			anonymousCookieJarMu.Unlock()
		}

		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Referer", "https://www.bilibili.com")
		req.Header.Set("Origin", "https://www.bilibili.com")
		if effectiveCookie != "" {
			req.Header.Set("Cookie", effectiveCookie)
		}

		res, err := bilibiliHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < bilibiliMaxRetries-1 {
				time.Sleep(time.Duration(randomBackoffMs()) * time.Millisecond)
			}
			continue
		}

		// 未登录时收集匿名 Cookie
		if cookie == "" {
			setCookie := parseSetCookieHeaderSimple(res.Header)
			if setCookie != "" {
				anonymousCookieJarMu.Lock()
				if anonymousCookieJar == "" {
					anonymousCookieJar = setCookie
				} else {
					anonymousCookieJar = anonymousCookieJar + "; " + setCookie
				}
				anonymousCookieJarMu.Unlock()
			}
		}

		if res.StatusCode == 412 {
			res.Body.Close()
			lastErr = fmt.Errorf("B站 API 返回 412 风控拦截: %s", api)
			if attempt < bilibiliMaxRetries-1 {
				time.Sleep(time.Duration(randomBackoffMs()) * time.Millisecond)
			}
			continue
		}

		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			lastErr = fmt.Errorf("B站 API 请求失败 [%d]: %s", res.StatusCode, api)
			if attempt < bilibiliMaxRetries-1 {
				time.Sleep(time.Duration(randomBackoffMs()) * time.Millisecond)
			}
			continue
		}

		var payload struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
			Result  json.RawMessage `json:"result"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			res.Body.Close()
			lastErr = err
			if attempt < bilibiliMaxRetries-1 {
				time.Sleep(time.Duration(randomBackoffMs()) * time.Millisecond)
			}
			continue
		}
		res.Body.Close()

		if payload.Code != 0 {
			lastErr = fmt.Errorf("B站 API 业务错误 [%d] %s: %s", payload.Code, payload.Message, api)
			// 业务错误不重试
			return lastErr
		}

		if out != nil {
			// UGC 接口载荷在 data，PGC（番剧）接口载荷在顶层 result。
			// 两者互斥：data 存在时优先（UGC 语义不变），否则回退 result（PGC）。
			body := payload.Data
			if len(body) == 0 || string(body) == "null" {
				body = payload.Result
			}
			if err := json.Unmarshal(body, out); err != nil {
				return err
			}
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("B站 API 请求失败: %s", api)
}

// ===================== VIP / 权限 =====================

// getVipStatus 查询用户大会员状态（带缓存）。
func getVipStatus(cookie string) (bool, error) {
	if strings.TrimSpace(cookie) == "" {
		return false, nil
	}
	if cached := getCachedVipStatus(cookie); cached != nil {
		return *cached, nil
	}

	type navData struct {
		IsLogin   bool `json:"isLogin"`
		VipStatus int  `json:"vipStatus"`
		VipType   int  `json:"vipType"`
	}
	var data navData
	if err := bilibiliFetch("https://api.bilibili.com/x/web-interface/nav", cookie, &data); err != nil {
		return false, err
	}
	isVip := data.IsLogin && (data.VipStatus == 1 || data.VipType > 0)
	setCachedVipStatus(cookie, isVip)
	return isVip, nil
}

// filterQualitiesByVip 根据会员状态和登录态过滤可用清晰度列表。
func filterQualitiesByVip(list []QualityItem, isVip bool, hasCookie bool) []QualityItem {
	if isVip {
		return list
	}
	if !hasCookie {
		filtered := make([]QualityItem, 0, len(list))
		for _, q := range list {
			if q.ID <= anonymousMaxQn {
				filtered = append(filtered, q)
			}
		}
		if len(filtered) == 0 {
			return []QualityItem{
				{ID: 32, Label: "480P", Resolution: "854x480"},
				{ID: 16, Label: "360P", Resolution: "640x360"},
			}
		}
		return filtered
	}
	filtered := make([]QualityItem, 0, len(list))
	for _, q := range list {
		if !intSliceContains(vipOnlyQns, q.ID) {
			filtered = append(filtered, q)
		}
	}
	if len(filtered) == 0 {
		return []QualityItem{{ID: 80, Label: "1080P", Resolution: "1920x1080"}}
	}
	return filtered
}

func intSliceContains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// computeFnval 根据 VIP 状态和请求的 qn 计算 fnval 位标志。
func computeFnval(isVip bool, qn int) int {
	dash := 16
	fourK := 64
	eightK := 2048
	if !isVip {
		return dash
	}
	fnval := dash | fourK
	if qn == qn8K {
		fnval |= eightK
	}
	return fnval
}

// getDefaultQn 根据 VIP 状态和登录态返回默认清晰度。
func getDefaultQn(isVip bool, hasCookie bool) int {
	if !hasCookie {
		return anonymousMaxQn
	}
	if isVip {
		return vipDefaultQn
	}
	return defaultQn
}

// ===================== video.ts =====================

// getVideoInfoWbi 使用 WBI 签名调用 view 接口。
func getVideoInfoWbi(bvid, cookie string) (*BilibiliVideoInfo, error) {
	imgKey, subKey, err := getWbiKeys(cookie)
	if err != nil {
		return nil, err
	}
	signed := signParamsWithWbi(map[string]string{"bvid": bvid}, imgKey, subKey)
	qs := url.Values{}
	for k, v := range signed {
		qs.Set(k, v)
	}
	api := "https://api.bilibili.com/x/web-interface/wbi/view?" + qs.Encode()

	var info BilibiliVideoInfo
	if err := bilibiliFetch(api, cookie, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// getVideoInfoLegacy 使用未签名接口作为降级方案。
func getVideoInfoLegacy(bvid, cookie string) (*BilibiliVideoInfo, error) {
	api := "https://api.bilibili.com/x/web-interface/view?bvid=" + url.QueryEscape(bvid)
	var info BilibiliVideoInfo
	if err := bilibiliFetch(api, cookie, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// getVideoInfo 获取 B站视频信息，优先 WBI，失败降级。
func getVideoInfo(bvid, cookie string) (*BilibiliVideoInfo, error) {
	info, err := getVideoInfoWbi(bvid, cookie)
	if err != nil {
		logf("[bilibili] WBI view 失败，降级到未签名接口: %v", err)
		clearWbiKeyCache()
		return getVideoInfoLegacy(bvid, cookie)
	}
	return info, nil
}

// ===================== playurl.ts =====================

// getPlayUrlOptions 选项。
type getPlayUrlOptions struct {
	Qn       int
	Codec    string
	IsVip    bool
	Fnval    int
	Platform string
}

// buildAcceptQuality 构建可用清晰度列表。
func buildAcceptQuality(acceptQuality []int, acceptDescription map[int]string, currentQn int) []QualityItem {
	qns := acceptQuality
	if len(qns) == 0 {
		for qn := range acceptDescription {
			qns = append(qns, qn)
		}
	}
	if len(qns) == 0 {
		qns = []int{currentQn}
	}
	items := make([]QualityItem, 0, len(qns))
	for _, qn := range qns {
		fallback := qnQualityMap[qn]
		label := acceptDescription[qn]
		if label == "" {
			label = fallback.Label
		}
		if label == "" {
			label = strconv.Itoa(qn)
		}
		res := fallback.Resolution
		items = append(items, QualityItem{ID: qn, Label: label, Resolution: res})
	}
	return items
}

func detectCodec(codecs string) string {
	c := strings.TrimSpace(codecs)
	if matched, _ := regexp.MatchString(`^avc\d`, c); matched {
		return "avc"
	}
	if matched, _ := regexp.MatchString(`^hvc\d|^hev\d`, c); matched {
		return "hevc"
	}
	if matched, _ := regexp.MatchString(`^av01`, c); matched {
		return "av1"
	}
	return "unknown"
}

func sortByBandwidthDesc(tracks []DashMediaTrack) []DashMediaTrack {
	result := make([]DashMediaTrack, len(tracks))
	copy(result, tracks)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Bandwidth < result[j].Bandwidth {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func sortDashTracks(tracks []DashMediaTrack, codec string) []DashMediaTrack {
	sorted := sortByBandwidthDesc(tracks)
	if len(sorted) == 0 {
		return sorted
	}
	preferred := codec
	if preferred == "" || preferred == "auto" {
		preferred = "avc"
	}
	matched := make([]DashMediaTrack, 0, len(sorted))
	for _, t := range sorted {
		if detectCodec(t.Codecs) == preferred {
			matched = append(matched, t)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return sorted
}

// rewriteMcdnPort 部分网络环境无法连接 B站 mcdn P2P CDN 的 8082 端口，去掉该端口。
func rewriteMcdnPort(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if strings.HasSuffix(u.Hostname(), ".mcdn.bilivideo.cn") && u.Port() == "8082" {
		u.Host = u.Hostname()
		return u.String()
	}
	return raw
}

func normalizeDashMedia(raw map[string]any) DashMediaTrack {
	baseUrl := ""
	if v, ok := raw["baseUrl"].(string); ok && v != "" {
		baseUrl = v
	} else if v, ok := raw["base_url"].(string); ok && v != "" {
		baseUrl = v
	}
	var backupUrl []string
	if arr, ok := raw["backupUrl"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				backupUrl = append(backupUrl, rewriteMcdnPort(s))
			}
		}
	} else if arr, ok := raw["backup_url"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				backupUrl = append(backupUrl, rewriteMcdnPort(s))
			}
		}
	}
	bandwidth := 0
	if v, ok := raw["bandwidth"].(float64); ok {
		bandwidth = int(v)
	}
	codecs := ""
	if v, ok := raw["codecs"].(string); ok {
		codecs = v
	}
	id := 0
	if v, ok := raw["id"].(float64); ok {
		id = int(v)
	}
	return DashMediaTrack{
		BaseUrl:   rewriteMcdnPort(baseUrl),
		BackupUrl: backupUrl,
		Bandwidth: bandwidth,
		Codecs:    codecs,
		ID:        id,
	}
}

func normalizePlayUrlData(data map[string]any, requestedQn int, codec string) (*BilibiliPlayUrlResult, error) {
	if data == nil {
		return nil, &NoPermissionError{}
	}

	qn := requestedQn
	if v, ok := data["quality"].(float64); ok {
		qn = int(v)
	} else if qn == 0 {
		qn = defaultQn
	}

	acceptQualityRaw := []int{}
	if arr, ok := data["accept_quality"].([]any); ok {
		for _, item := range arr {
			if v, ok := item.(float64); ok {
				acceptQualityRaw = append(acceptQualityRaw, int(v))
			}
		}
	}

	acceptDescription := make(map[int]string)
	if arr, ok := data["accept_description"].([]any); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				id := 0
				if v, ok := m["qn"].(float64); ok {
					id = int(v)
				}
				desc := ""
				if v, ok := m["desc"].(string); ok {
					desc = v
				}
				if id != 0 {
					acceptDescription[id] = desc
				}
			}
		}
	}

	acceptQuality := buildAcceptQuality(acceptQualityRaw, acceptDescription, qn)

	if dashRaw, ok := data["dash"].(map[string]any); ok {
		if videoArr, ok := dashRaw["video"].([]any); ok && len(videoArr) > 0 {
			allTracks := make([]DashMediaTrack, 0, len(videoArr))
			for _, item := range videoArr {
				if m, ok := item.(map[string]any); ok {
					allTracks = append(allTracks, normalizeDashMedia(m))
				}
			}
			matchedQnTracks := make([]DashMediaTrack, 0, len(allTracks))
			for _, t := range allTracks {
				if t.ID == qn {
					matchedQnTracks = append(matchedQnTracks, t)
				}
			}
			tracksToSort := allTracks
			if len(matchedQnTracks) > 0 {
				tracksToSort = matchedQnTracks
			}
			video := sortDashTracks(tracksToSort, codec)

			var audio []DashMediaTrack
			if audioArr, ok := dashRaw["audio"].([]any); ok {
				for _, item := range audioArr {
					if m, ok := item.(map[string]any); ok {
						audio = append(audio, normalizeDashMedia(m))
					}
				}
			}
			audio = sortByBandwidthDesc(audio)

			result := &BilibiliPlayUrlResult{
				Format:        "dash",
				Video:         video,
				Audio:         audio,
				CurrentQn:     qn,
				AcceptQuality: acceptQuality,
			}
			if len(video) > 0 {
				result.BestVideo = &video[0]
			}
			if len(audio) > 0 {
				result.BestAudio = &audio[0]
			}
			return result, nil
		}
	}

	if durlArr, ok := data["durl"].([]any); ok && len(durlArr) > 0 {
		segments := make([]DurlSegment, 0, len(durlArr))
		for _, item := range durlArr {
			if m, ok := item.(map[string]any); ok {
				seg := DurlSegment{}
				if v, ok := m["url"].(string); ok {
					seg.Url = v
				}
				if v, ok := m["size"].(float64); ok {
					seg.Size = int64(v)
				}
				if v, ok := m["length"].(float64); ok {
					seg.Length = int64(v)
				}
				segments = append(segments, seg)
			}
		}
		return &BilibiliPlayUrlResult{
			Format:        "mp4",
			Durl:          segments,
			CurrentQn:     qn,
			AcceptQuality: acceptQuality,
		}, nil
	}

	return nil, &NoPermissionError{}
}

func isPermissionError(err error) bool {
	if _, ok := err.(*NoPermissionError); ok {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "-101") || strings.Contains(msg, "账号未登录") {
		return false
	}
	keywords := []string{"大会员", "付费", "无权限", "购买", "权限"}
	for _, k := range keywords {
		if strings.Contains(msg, k) {
			return true
		}
	}
	if strings.Contains(msg, "-10403") {
		return true
	}
	return false
}

// getPlayUrlWbi 使用 WBI 签名调用 playurl。
func getPlayUrlWbi(bvid string, cid int64, cookie string, options *getPlayUrlOptions) (*BilibiliPlayUrlResult, error) {
	isVip := false
	requestedQn := defaultQn
	fnval := 0
	platform := ""
	if options != nil {
		isVip = options.IsVip
		if options.Qn != 0 {
			requestedQn = options.Qn
		}
		if options.Fnval != 0 {
			fnval = options.Fnval
		} else {
			fnval = computeFnval(isVip, requestedQn)
		}
		platform = options.Platform
	}

	imgKey, subKey, err := getWbiKeys(cookie)
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"bvid":  bvid,
		"cid":   strconv.FormatInt(cid, 10),
		"qn":    strconv.Itoa(requestedQn),
		"fnver": "0",
		"fnval": strconv.Itoa(fnval),
		"fourk": "1",
	}
	if platform == "html5" {
		params["platform"] = "html5"
		params["high_quality"] = "1"
		params["try_look"] = "1"
	}
	signed := signParamsWithWbi(params, imgKey, subKey)
	qs := url.Values{}
	for k, v := range signed {
		qs.Set(k, v)
	}
	api := "https://api.bilibili.com/x/player/wbi/playurl?" + qs.Encode()

	var raw map[string]any
	if err := bilibiliFetch(api, cookie, &raw); err != nil {
		return nil, err
	}
	return normalizePlayUrlData(raw, requestedQn, options.Codec)
}

// getPlayUrlLegacy 使用未签名接口作为降级方案。
func getPlayUrlLegacy(bvid string, cid int64, cookie string, options *getPlayUrlOptions) (*BilibiliPlayUrlResult, error) {
	isVip := false
	requestedQn := defaultQn
	fnval := 0
	platform := ""
	if options != nil {
		isVip = options.IsVip
		if options.Qn != 0 {
			requestedQn = options.Qn
		}
		if options.Fnval != 0 {
			fnval = options.Fnval
		} else {
			fnval = computeFnval(isVip, requestedQn)
		}
		platform = options.Platform
	}

	qs := url.Values{
		"bvid":  {bvid},
		"cid":   {strconv.FormatInt(cid, 10)},
		"qn":    {strconv.Itoa(requestedQn)},
		"fnver": {"0"},
		"fnval": {strconv.Itoa(fnval)},
		"fourk": {"1"},
	}
	if platform == "html5" {
		qs.Set("platform", "html5")
		qs.Set("high_quality", "1")
		qs.Set("try_look", "1")
	}
	api := "https://api.bilibili.com/x/player/playurl?" + qs.Encode()

	var raw map[string]any
	if err := bilibiliFetch(api, cookie, &raw); err != nil {
		return nil, err
	}
	return normalizePlayUrlData(raw, requestedQn, options.Codec)
}

// getPlayUrl 获取播放地址，优先 WBI，失败降级。
func getPlayUrl(bvid string, cid int64, cookie string, options *getPlayUrlOptions) (*BilibiliPlayUrlResult, error) {
	result, err := getPlayUrlWbi(bvid, cid, cookie, options)
	if err != nil {
		if isPermissionError(err) {
			return nil, &NoPermissionError{}
		}
		logf("[bilibili] WBI playurl 失败，降级到未签名接口: %v", err)
		clearWbiKeyCache()
		result, err = getPlayUrlLegacy(bvid, cid, cookie, options)
		if err != nil {
			if isPermissionError(err) {
				return nil, &NoPermissionError{}
			}
			return nil, err
		}
	}
	return result, nil
}

// ===================== cdn.ts =====================

// checkUrlReachable 使用 HEAD + Range 探测单个 URL 是否可达。
// cdnCheckClient 专用于 CDN 健康检查的 HTTP 客户端，短超时 + 连接池。
var cdnCheckClient = &http.Client{
	Timeout: cdnHealthCheckTimeoutMs * time.Millisecond,
	Transport: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
	},
}

func checkUrlReachable(target string) bool {
	req, err := http.NewRequest(http.MethodHead, target, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Referer", "https://www.bilibili.com")
	req.Header.Set("Origin", "https://www.bilibili.com")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", "bytes=0-0")
	res, err := cdnCheckClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 300 || res.StatusCode == 405
}

// findReachableMediaUrl 并行检测所有候选 URL，返回第一个可达的 URL。
func findReachableMediaUrl(baseUrl string, backupUrl []string) (string, error) {
	candidates := []string{baseUrl}
	for _, u := range backupUrl {
		if u != "" {
			candidates = append(candidates, u)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("没有可用的媒体 URL")
	}

	type result struct {
		url string
		ok  bool
	}
	ch := make(chan result, len(candidates))
	var wg sync.WaitGroup
	for _, u := range candidates {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ok := checkUrlReachable(target)
			ch <- result{url: target, ok: ok}
		}(u)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	done := make(chan string, 1)
	go func() {
		for r := range ch {
			if r.ok {
				done <- r.url
				return
			}
		}
		close(done)
	}()

	select {
	case url, ok := <-done:
		if !ok {
			return "", fmt.Errorf("所有 CDN 均不可达")
		}
		return url, nil
	case <-time.After(cdnHealthCheckTimeoutMs*time.Millisecond + 500*time.Millisecond):
		return "", fmt.Errorf("CDN 健康检查超时")
	}
}

// ===================== resolver.ts =====================

// extractBvid 从任意输入提取 BV 号或 av 号。
func extractBvid(input string) (string, error) {
	reBV := regexp.MustCompile(`BV[0-9A-Za-z]{10}`)
	if m := reBV.FindString(input); m != "" {
		return m, nil
	}
	reAV := regexp.MustCompile(`(?i)av(\d+)`)
	if m := reAV.FindStringSubmatch(input); len(m) > 1 {
		return m[1], nil
	}
	return "", fmt.Errorf("无法解析 B站 BV 号")
}

func fetchVideoInfo(bvid, cookie string) (*BilibiliVideoInfo, error) {
	if cached := getCachedVideoInfo(bvid); cached != nil {
		return cached, nil
	}
	info, err := getVideoInfo(bvid, cookie)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "[-404]") {
			return nil, &ResolveError{Message: fmt.Sprintf("视频不存在或已被删除（%s），请检查 BV 号是否正确", bvid), Code: "VIDEO_NOT_FOUND"}
		}
		if strings.Contains(msg, "[-101]") {
			return nil, &ResolveError{Message: "B站账号未登录或登录已过期，请重新扫码登录", Code: "NOT_LOGGED_IN"}
		}
		if strings.Contains(msg, "[-403]") || strings.Contains(msg, "[-514]") {
			return nil, &ResolveError{Message: "无权限访问该视频，可能为会员专享或地区限制", Code: "NO_PERMISSION"}
		}
		return nil, &ResolveError{Message: "获取视频信息失败: " + msg, Code: "INFO_FAILED"}
	}
	if info == nil {
		return nil, &ResolveError{Message: "获取视频信息失败", Code: "INFO_FAILED"}
	}
	setCachedVideoInfo(bvid, info)
	return info, nil
}

// getCurrentPageDuration 使用 page-specific duration。
func getCurrentPageDuration(info *BilibiliVideoInfo, cid int64) int {
	if len(info.Pages) > 0 {
		targetCid := cid
		if targetCid == 0 {
			targetCid = info.Cid
		}
		for _, p := range info.Pages {
			if p.Cid == targetCid {
				return p.Duration
			}
		}
		return info.Pages[0].Duration
	}
	return info.Duration
}

// fallbackToMp4 DASH 不可达时降级为 MP4 直链。
func fallbackToMp4(bvid string, cid int64, cookie string, qn int, isVip bool, skipCdnCheck bool) (string, int, error) {
	result, err := getPlayUrl(bvid, cid, cookie, &getPlayUrlOptions{
		Qn:       qn,
		Fnval:    1,
		IsVip:    isVip,
		Platform: "html5",
	})
	if err != nil {
		return "", 0, err
	}
	if result != nil && result.Format == "mp4" && len(result.Durl) > 0 && result.Durl[0].Url != "" {
		if skipCdnCheck {
			return result.Durl[0].Url, result.CurrentQn, nil
		}
		url, err := findReachableMediaUrl(result.Durl[0].Url, nil)
		if err != nil {
			return "", 0, err
		}
		return url, result.CurrentQn, nil
	}
	return "", 0, fmt.Errorf("MP4 直链不可用")
}

func narrowAcceptQualityForMp4(list []QualityItem) []QualityItem {
	filtered := make([]QualityItem, 0, len(list))
	for _, q := range list {
		if q.ID <= mp4MaxQn {
			filtered = append(filtered, q)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return []QualityItem{
		{ID: 32, Label: qnQualityMap[32].Label, Resolution: qnQualityMap[32].Resolution},
		{ID: 16, Label: qnQualityMap[16].Label, Resolution: qnQualityMap[16].Resolution},
	}
}

// ResolveBilibiliVideo 编排完整解析流程（与 backend resolveBilibiliVideo 对齐）。
func ResolveBilibiliVideo(opts ResolveOptions) (*ResolveResult, error) {
	bvid, bvidErr := extractBvid(opts.Url)
	if bvidErr != nil {
		// BV/av 优先；未命中时尝试番剧 ep/ss（PGC 独立编排）
		if epId, seasonId, ok := extractBangumiId(opts.Url); ok {
			return resolvePgcVideo(opts, epId, seasonId)
		}
		return nil, &ResolveError{Message: "无法解析 B站 视频/番剧链接", Code: "INVALID_INPUT"}
	}

	logf("[bilibili] 开始解析视频: %s, qn=%d, preferMp4=%v, forceDash=%v", bvid, opts.Qn, opts.PreferMp4, opts.ForceDash)

	cookie := strings.TrimSpace(opts.Cookie)
	hasCookie := cookie != ""

	// 并行：VIP 校验与视频信息获取
	var isVip bool
	var info *BilibiliVideoInfo
	var vipErr, infoErr error
	done := make(chan struct{}, 2)
	go func() {
		isVip, vipErr = getVipStatus(cookie)
		done <- struct{}{}
	}()
	go func() {
		info, infoErr = fetchVideoInfo(bvid, cookie)
		done <- struct{}{}
	}()
	for i := 0; i < 2; i++ {
		<-done
	}
	if infoErr != nil {
		return nil, infoErr
	}
	if vipErr != nil {
		// VIP 校验失败不阻断，按非会员处理
		isVip = false
	}

	// 确定当前播放分集：优先使用传入的 cid，其次使用 page 参数，最后使用默认 cid
	effectiveCid := info.Cid
	currentPage := 1
	if opts.Cid != 0 {
		effectiveCid = opts.Cid
		if len(info.Pages) > 0 {
			for _, p := range info.Pages {
				if p.Cid == opts.Cid {
					currentPage = p.Page
					break
				}
			}
		}
	} else if opts.Page > 0 && len(info.Pages) > 0 {
		idx := opts.Page - 1
		if idx >= len(info.Pages) {
			idx = len(info.Pages) - 1
		}
		if info.Pages[idx].Cid != 0 {
			effectiveCid = info.Pages[idx].Cid
			currentPage = info.Pages[idx].Page
		}
	} else if len(info.Pages) > 0 {
		for _, p := range info.Pages {
			if p.Cid == info.Cid {
				currentPage = p.Page
				break
			}
		}
	}

	var pagesInfo []ResolvePageInfo
	if len(info.Pages) > 0 {
		pagesInfo = make([]ResolvePageInfo, 0, len(info.Pages))
		for _, p := range info.Pages {
			pagesInfo = append(pagesInfo, ResolvePageInfo{
				Page:     p.Page,
				Cid:      p.Cid,
				Part:     p.Part,
				Duration: p.Duration,
			})
		}
	}

	defaultQn := getDefaultQn(isVip, hasCookie)
	requestedQn := opts.Qn
	if requestedQn == 0 {
		requestedQn = defaultQn
	}

	// 播放地址
	playUrl, err := getPlayUrl(info.Bvid, effectiveCid, cookie, &getPlayUrlOptions{
		Qn:    requestedQn,
		Codec: opts.Codec,
		IsVip: isVip,
	})
	if err != nil {
		if _, ok := err.(*NoPermissionError); ok {
			// 权限错误：逐级降级重试
			fallbackQn := requestedQn
			if requestedQn > 32 {
				fallbackQn = 32
			} else {
				fallbackQn = 16
			}
			if fallbackQn != requestedQn {
				playUrl, err = getPlayUrl(info.Bvid, effectiveCid, cookie, &getPlayUrlOptions{
					Qn:    fallbackQn,
					Codec: opts.Codec,
					IsVip: isVip,
				})
			}
		}
		if err != nil {
			return nil, &ResolveError{Message: "无法获取播放地址，可能需要登录或大会员", Code: "NO_PERMISSION"}
		}
	}
	if playUrl == nil {
		return nil, &ResolveError{Message: "无法获取播放地址，可能需要登录或大会员", Code: "NO_PERMISSION"}
	}

	// 清晰度匹配
	acceptQuality := filterQualitiesByVip(playUrl.AcceptQuality, isVip, hasCookie)
	effectiveQn := playUrl.CurrentQn
	if effectiveQn != 0 && !qualityListContains(acceptQuality, effectiveQn) {
		effectiveQn = acceptQuality[0].ID
	}

	if effectiveQn != 0 && effectiveQn != playUrl.CurrentQn {
		refetched, err := getPlayUrl(info.Bvid, effectiveCid, cookie, &getPlayUrlOptions{
			Qn:    effectiveQn,
			Codec: opts.Codec,
			IsVip: isVip,
		})
		if err != nil {
			if _, ok := err.(*NoPermissionError); !ok {
				return nil, err
			}
		} else if refetched != nil {
			playUrl = refetched
			acceptQuality = filterQualitiesByVip(playUrl.AcceptQuality, isVip, hasCookie)
		}
	}

	// preferMp4 优先路径
	// ForceDash 为 true 时（CLI 代理模式已连接），完全禁用 MP4 降级，强制走 DASH 代理。
	if opts.PreferMp4 && !opts.ForceDash {
		mp4Url, mp4Qn, err := fallbackToMp4(info.Bvid, effectiveCid, cookie, effectiveQn, isVip, opts.SkipCdnCheck)
		if err == nil && mp4Url != "" {
			logf("[bilibili] 解析完成: %s, format=mp4, qn=%d", bvid, mp4Qn)
			mp4AcceptQuality := narrowAcceptQualityForMp4(acceptQuality)
			return &ResolveResult{
				Title:         info.Title,
				Duration:      getCurrentPageDuration(info, effectiveCid),
				Cid:           effectiveCid,
				VideoUrl:      mp4Url,
				Format:        "mp4",
				LoggedIn:      hasCookie,
				VipStatus:     boolToInt(isVip),
				CurrentQn:     mp4Qn,
				AcceptQuality: mp4AcceptQuality,
				Pages:         pagesInfo,
				CurrentPage:   currentPage,
			}, nil
		}
	}

	// DASH 路径
	if playUrl.Format == "dash" && playUrl.BestVideo != nil {
		// 收集视频/音频各自的主 URL + backup URL，供 /proxy 备用重试。
		videoBackupUrls, audioBackupUrls := collectBackupUrls(playUrl.BestVideo, playUrl.BestAudio)

		// 对候选 CDN 做本地可达性探测。
		// CLI 与用户浏览器在同一台机器，本地探测结果具有参考价值；
		// SkipCdnCheck 为 true 时探测失败不阻断、不降级 MP4，仅回退到 BaseUrl。
		var videoUrl, audioUrl string
		var videoErr, audioErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			videoUrl, videoErr = findReachableMediaUrl(playUrl.BestVideo.BaseUrl, playUrl.BestVideo.BackupUrl)
		}()
		if playUrl.BestAudio != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				audioUrl, audioErr = findReachableMediaUrl(playUrl.BestAudio.BaseUrl, playUrl.BestAudio.BackupUrl)
			}()
		}
		wg.Wait()

		if videoErr != nil {
			if opts.SkipCdnCheck {
				logf("[bilibili] 视频 CDN 本地探测均不可达，因 SkipCdnCheck 回退到 BaseUrl: %v", videoErr)
				videoUrl = playUrl.BestVideo.BaseUrl
			} else {
				return nil, &ResolveError{Message: "无法找到可达的视频 CDN", Code: "CDN_UNREACHABLE"}
			}
		}
		if playUrl.BestAudio != nil && audioErr != nil {
			if opts.SkipCdnCheck {
				logf("[bilibili] 音频 CDN 本地探测均不可达，因 SkipCdnCheck 回退到 BaseUrl: %v", audioErr)
				audioUrl = playUrl.BestAudio.BaseUrl
			} else {
				// 非 SkipCdnCheck 模式下音频 CDN 不可达不阻断，但记录日志
				logf("[bilibili] 音频 CDN 均不可达，尝试无音频播放: %v", audioErr)
			}
		}

		videoCodec := ""
		if playUrl.BestVideo != nil {
			videoCodec = playUrl.BestVideo.Codecs
		}
		audioCodec := ""
		if playUrl.BestAudio != nil {
			audioCodec = playUrl.BestAudio.Codecs
		}

		logf("[bilibili] 解析完成: %s, format=dash, qn=%d, codec=%s, videoCandidates=%d, audioCandidates=%d",
			bvid, playUrl.CurrentQn, videoCodec, len(videoBackupUrls), len(audioBackupUrls))
		logf("[bilibili] 选中视频 CDN: %s", videoUrl)
		if audioUrl != "" {
			logf("[bilibili] 选中音频 CDN: %s", audioUrl)
		}
		return &ResolveResult{
			Title:           info.Title,
			Duration:        getCurrentPageDuration(info, effectiveCid),
			Cid:             effectiveCid,
			VideoUrl:        videoUrl,
			AudioUrl:        audioUrl,
			VideoCodec:      videoCodec,
			AudioCodec:      audioCodec,
			Format:          "dash",
			LoggedIn:        hasCookie,
			VipStatus:       boolToInt(isVip),
			CurrentQn:       playUrl.CurrentQn,
			AcceptQuality:   acceptQuality,
			Pages:           pagesInfo,
			CurrentPage:     currentPage,
			VideoBackupUrls: videoBackupUrls,
			AudioBackupUrls: audioBackupUrls,
		}, nil
	}

	logf("[bilibili] 解析失败: %s, 无法获取可用播放地址", bvid)
	return nil, &ResolveError{Message: "无法获取可用播放地址", Code: "NO_PLAYABLE_URL"}
}

func qualityListContains(list []QualityItem, qn int) bool {
	for _, q := range list {
		if q.ID == qn {
			return true
		}
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// collectBackupUrls 返回视频和音频轨道各自的主 URL + backup URL 候选列表（已去重）。
func collectBackupUrls(video, audio *DashMediaTrack) (videoUrls, audioUrls []string) {
	add := func(seen map[string]struct{}, u string, urls *[]string) {
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		*urls = append(*urls, u)
	}

	seenVideo := make(map[string]struct{})
	if video != nil {
		add(seenVideo, video.BaseUrl, &videoUrls)
		for _, u := range video.BackupUrl {
			add(seenVideo, u, &videoUrls)
		}
	}

	seenAudio := make(map[string]struct{})
	if audio != nil {
		add(seenAudio, audio.BaseUrl, &audioUrls)
		for _, u := range audio.BackupUrl {
			add(seenAudio, u, &audioUrls)
		}
	}
	return videoUrls, audioUrls
}

// ===================== pgc.ts（番剧/影视 PGC，与 backend pgc.ts 对齐） =====================

// PGC MP4 直链最高清晰度（与后端 MP4_MAX_QN 一致：番剧 MP4 实测匿名 360P/登录 720P 级别）
const pgcMp4MaxQn = 64

// PgcEpisodeInfo 番剧分集信息。
type PgcEpisodeInfo struct {
	EpId        int64
	Cid         int64
	Bvid        string
	Aid         int64
	Title       string
	LongTitle   string
	Badge       string
	Status      int
	DurationSec int
}

// PgcSeasonInfo 番剧整季信息。
type PgcSeasonInfo struct {
	SeasonId int64
	Title    string
	Episodes []PgcEpisodeInfo
}

// PgcError PGC 接口错误（与 backend PgcError 对齐，resolvePgcVideo 转 ResolveError）。
type PgcError struct {
	Message string
	Code    string
}

func (e *PgcError) Error() string { return e.Message }

// pgcEpisodeRaw season 接口的分集原始结构（snake_case）。
type pgcEpisodeRaw struct {
	Id        int64  `json:"id"`
	EpId      int64  `json:"ep_id"`
	Cid       int64  `json:"cid"`
	Bvid      string `json:"bvid"`
	Aid       int64  `json:"aid"`
	Badge     string `json:"badge"`
	Status    int    `json:"status"`
	Duration  int64  `json:"duration"`
	Title     string `json:"title"`
	LongTitle string `json:"long_title"`
}

// ep→season 缓存（TTL 10 分钟，与后端一致）
type seasonByEpCacheEntry struct {
	season   *PgcSeasonInfo
	cachedAt time.Time
}

var (
	seasonByEpCache   = make(map[int64]seasonByEpCacheEntry)
	seasonByEpCacheMu sync.RWMutex
)

const seasonByEpCacheTTL = 10 * time.Minute

func mapPgcEpisodes(raws []pgcEpisodeRaw) []PgcEpisodeInfo {
	episodes := make([]PgcEpisodeInfo, 0, len(raws))
	for idx, item := range raws {
		epId := item.EpId
		if epId == 0 {
			epId = item.Id
		}
		if epId == 0 || item.Cid == 0 {
			continue
		}
		title := item.Title
		if title == "" {
			title = strconv.Itoa(idx + 1)
		}
		status := item.Status
		if status == 0 {
			status = 2
		}
		episodes = append(episodes, PgcEpisodeInfo{
			EpId:        epId,
			Cid:         item.Cid,
			Bvid:        item.Bvid,
			Aid:         item.Aid,
			Title:       title,
			LongTitle:   item.LongTitle,
			Badge:       item.Badge,
			Status:      status,
			DurationSec: int(item.Duration / 1000),
		})
	}
	return episodes
}

func mapPgcBizError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "[-404]") {
		return &PgcError{Message: "该番剧或集数不存在/已下架", Code: "EP_NOT_FOUND"}
	}
	if strings.Contains(msg, "[-403]") || strings.Contains(msg, "[-514]") {
		return &PgcError{Message: "该内容为付费影视或大会员专享，请使用大会员账号解析", Code: "NO_PERMISSION"}
	}
	if strings.Contains(msg, "[-10403]") || strings.Contains(msg, "地区") {
		return &PgcError{Message: "该内容存在地区观看限制", Code: "REGION_LIMITED"}
	}
	if strings.Contains(msg, "[-101]") || strings.Contains(msg, "账号未登录") {
		return &PgcError{Message: "获取 480P 以上清晰度需要登录 B站 账号", Code: "NOT_LOGGED_IN"}
	}
	return err
}

// fetchPgcSeason 调 pgc/view/web/season（payload 在顶层 result，bilibiliFetch 已感知）。
func fetchPgcSeason(query, cookie string) (*PgcSeasonInfo, error) {
	api := "https://api.bilibili.com/pgc/view/web/season?" + query
	var raw struct {
		SeasonId int64           `json:"season_id"`
		Title    string          `json:"title"`
		Episodes []pgcEpisodeRaw `json:"episodes"`
		MainSection *struct {
			Episodes []pgcEpisodeRaw `json:"episodes"`
		} `json:"main_section"`
		Section []struct {
			Episodes []pgcEpisodeRaw `json:"episodes"`
		} `json:"section"`
	}
	if err := bilibiliFetch(api, cookie, &raw); err != nil {
		return nil, mapPgcBizError(err)
	}

	raws := raw.Episodes
	if len(raws) == 0 && raw.MainSection != nil {
		raws = raw.MainSection.Episodes
	}
	if len(raws) == 0 {
		for _, s := range raw.Section {
			raws = append(raws, s.Episodes...)
		}
	}
	episodes := mapPgcEpisodes(raws)
	if len(episodes) == 0 {
		return nil, &PgcError{Message: "该番剧暂无可用分集", Code: "EP_NOT_FOUND"}
	}
	return &PgcSeasonInfo{SeasonId: raw.SeasonId, Title: raw.Title, Episodes: episodes}, nil
}

func fetchPgcSeasonByEp(epId int64, cookie string) (*PgcSeasonInfo, error) {
	seasonByEpCacheMu.RLock()
	if entry, ok := seasonByEpCache[epId]; ok && time.Since(entry.cachedAt) <= seasonByEpCacheTTL {
		seasonByEpCacheMu.RUnlock()
		logf("[bilibili-pgc] season served from cache: ep_id=%d", epId)
		return entry.season, nil
	}
	seasonByEpCacheMu.RUnlock()

	season, err := fetchPgcSeason(fmt.Sprintf("ep_id=%d", epId), cookie)
	if err != nil {
		return nil, err
	}
	seasonByEpCacheMu.Lock()
	seasonByEpCache[epId] = seasonByEpCacheEntry{season: season, cachedAt: time.Now()}
	seasonByEpCacheMu.Unlock()
	return season, nil
}

func fetchPgcSeasonBySeasonId(seasonId int64, cookie string) (*PgcSeasonInfo, error) {
	return fetchPgcSeason(fmt.Sprintf("season_id=%d", seasonId), cookie)
}

// getPgcPlayUrl 调 pgc/player/web/playurl v1（SESSDATA 鉴权、无 WBI；
// 不带 platform=html5/try_look，实测对 PGC 无差异）。
func getPgcPlayUrl(epId, cid int64, cookie string, qn, fnval int, codec string) (*BilibiliPlayUrlResult, error) {
	qs := url.Values{
		"ep_id": {strconv.FormatInt(epId, 10)},
		"cid":   {strconv.FormatInt(cid, 10)},
		"qn":    {strconv.Itoa(qn)},
		"fnver": {"0"},
		"fnval": {strconv.Itoa(fnval)},
		"fourk": {"1"},
	}
	api := "https://api.bilibili.com/pgc/player/web/playurl?" + qs.Encode()
	var raw map[string]any
	if err := bilibiliFetch(api, cookie, &raw); err != nil {
		return nil, mapPgcBizError(err)
	}
	return normalizePlayUrlData(raw, qn, codec)
}

// extractBangumiId 从任意输入提取番剧 ep/ss 标识（与后端 resolver.extractBangumiId 语义一致：
// 显式 /bangumi/play/ 路径优先；裸 ep/ss 仅在无 BV/av 上下文且前缀非字母数字时认定）。
func extractBangumiId(input string) (epId, seasonId int64, ok bool) {
	rePath := regexp.MustCompile(`(?i)bangumi/play/(ep|ss)(\d+)`)
	if m := rePath.FindStringSubmatch(input); len(m) > 2 {
		n, _ := strconv.ParseInt(m[2], 10, 64)
		if strings.EqualFold(m[1], "ep") {
			return n, 0, true
		}
		return 0, n, true
	}
	if _, err := extractBvid(input); err == nil {
		return 0, 0, false
	}
	if regexp.MustCompile(`(?i)av\d+`).MatchString(input) {
		return 0, 0, false
	}
	reEp := regexp.MustCompile(`(?i)(?:^|[^a-z0-9])ep(\d+)`)
	if m := reEp.FindStringSubmatch(input); len(m) > 1 {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		return n, 0, true
	}
	reSs := regexp.MustCompile(`(?i)(?:^|[^a-z0-9])ss(\d+)`)
	if m := reSs.FindStringSubmatch(input); len(m) > 1 {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		return 0, n, true
	}
	return 0, 0, false
}

func toResolveError(err error) error {
	if pgcErr, ok := err.(*PgcError); ok {
		return &ResolveError{Message: pgcErr.Message, Code: pgcErr.Code}
	}
	return err
}

// resolvePgcVideo 番剧/影视解析编排（与 backend resolver.ts resolvePgcVideo 对齐）。
// 试看判定：badge 非空 + 非大会员 → 强制 MP4（B站试看只发 durl，且 timelength 虚报完整时长）。
func resolvePgcVideo(opts ResolveOptions, epId, seasonId int64) (*ResolveResult, error) {
	cookie := strings.TrimSpace(opts.Cookie)
	hasCookie := cookie != ""

	isVip, vipErr := getVipStatus(cookie)
	if vipErr != nil {
		isVip = false
	}

	var season *PgcSeasonInfo
	var err error
	if epId != 0 {
		season, err = fetchPgcSeasonByEp(epId, cookie)
	} else {
		season, err = fetchPgcSeasonBySeasonId(seasonId, cookie)
	}
	if err != nil {
		return nil, toResolveError(err)
	}

	// 定位目标集：cid 匹配 → 链接 ep_id → page 序号 → 默认第一集
	target := &season.Episodes[0]
	if opts.Cid != 0 {
		for i := range season.Episodes {
			if season.Episodes[i].Cid == opts.Cid {
				target = &season.Episodes[i]
				break
			}
		}
	} else if epId != 0 {
		for i := range season.Episodes {
			if season.Episodes[i].EpId == epId {
				target = &season.Episodes[i]
				break
			}
		}
	} else if opts.Page > 0 {
		idx := opts.Page - 1
		if idx >= len(season.Episodes) {
			idx = len(season.Episodes) - 1
		}
		target = &season.Episodes[idx]
	}

	// 注：不做 status 硬拦截——实测 2=免费可看、13=会员可看；
	// 不可播场景由 playurl 业务码（-404 等）判定
	preview := target.Badge != "" && !isVip

	currentPage := 1
	pagesInfo := make([]ResolvePageInfo, 0, len(season.Episodes))
	for i, ep := range season.Episodes {
		part := ep.LongTitle
		if part == "" {
			part = ep.Title
		}
		badge := ""
		if ep.Badge != "" {
			badge = ep.Badge
		}
		pagesInfo = append(pagesInfo, ResolvePageInfo{
			Page:     i + 1,
			Cid:      ep.Cid,
			Part:     part,
			Duration: ep.DurationSec,
			EpId:     ep.EpId,
			Badge:    badge,
		})
		if ep.EpId == target.EpId {
			currentPage = i + 1
		}
	}

	title := target.LongTitle
	if title == "" {
		title = target.Title
	}
	if season.Title != "" {
		title = season.Title + " " + title
	}

	requestedQn := opts.Qn
	if requestedQn == 0 {
		requestedQn = getDefaultQn(isVip, hasCookie)
	}
	logf("[bilibili-pgc] resolve ep=%d cid=%d badge=%s preview=%v qn=%d cookie=%v",
		target.EpId, target.Cid, target.Badge, preview, requestedQn, hasCookie)

	buildCommon := func(playUrl *BilibiliPlayUrlResult) *ResolveResult {
		return &ResolveResult{
			Title:        title,
			Duration:     target.DurationSec,
			Cid:          target.Cid,
			Format:       playUrl.Format,
			LoggedIn:     hasCookie,
			VipStatus:    boolToInt(isVip),
			CurrentQn:    playUrl.CurrentQn,
			AcceptQuality: playUrl.AcceptQuality,
			Pages:        pagesInfo,
			CurrentPage:  currentPage,
			EpId:         target.EpId,
			SeasonId:     season.SeasonId,
			SeasonTitle:  season.Title,
			Preview:      preview,
		}
	}

	// 试看集强制 MP4（试看只发 durl）；preferMp4 同路径
	if preview || opts.PreferMp4 {
		mp4PlayUrl, err := getPgcPlayUrl(target.EpId, target.Cid, cookie, minInt(requestedQn, pgcMp4MaxQn), 1, opts.Codec)
		if err != nil {
			return nil, toResolveError(err)
		}
		if mp4PlayUrl == nil || mp4PlayUrl.Format != "mp4" || len(mp4PlayUrl.Durl) == 0 || mp4PlayUrl.Durl[0].Url == "" {
			return nil, &ResolveError{Message: "该集不支持 MP4 直链播放，请切换到 DASH 高清模式", Code: "MP4_NOT_AVAILABLE"}
		}
		mp4Url := mp4PlayUrl.Durl[0].Url
		if !opts.SkipCdnCheck {
			if reachable, err := findReachableMediaUrl(mp4Url, nil); err == nil {
				mp4Url = reachable
			}
		}
		result := buildCommon(mp4PlayUrl)
		result.VideoUrl = mp4Url
		result.Format = "mp4"
		result.CurrentQn = minInt(mp4PlayUrl.CurrentQn, pgcMp4MaxQn)
		result.AcceptQuality = narrowAcceptQualityForPgcMp4(mp4PlayUrl.AcceptQuality)
		// 试看流时长以 durl[0].length 为准（timelength 是虚报的完整时长）
		if preview && len(mp4PlayUrl.Durl) > 0 && mp4PlayUrl.Durl[0].Length > 0 {
			previewLen := int(mp4PlayUrl.Durl[0].Length / 1000)
			if previewLen > 0 && previewLen < target.DurationSec {
				result.Duration = previewLen
			}
		}
		result.VideoBackupUrls = []string{mp4Url}
		return result, nil
	}

	// DASH 主路径
	playUrl, err := getPgcPlayUrl(target.EpId, target.Cid, cookie, requestedQn, computeFnval(isVip, requestedQn), opts.Codec)
	if err != nil {
		if _, ok := err.(*NoPermissionError); ok {
			fallbackQn := 16
			if requestedQn > 32 {
				fallbackQn = 32
			}
			if fallbackQn != requestedQn {
				logf("[bilibili-pgc] 当前清晰度无权限，降级到 qn=%d", fallbackQn)
				playUrl, err = getPgcPlayUrl(target.EpId, target.Cid, cookie, fallbackQn, computeFnval(isVip, fallbackQn), opts.Codec)
			}
		}
		if err != nil {
			return nil, toResolveError(err)
		}
	}
	if playUrl == nil {
		return nil, &ResolveError{Message: "无法获取播放地址，可能需要登录或大会员", Code: "NO_PERMISSION"}
	}

	// 清晰度匹配：请求 qn 不在可用列表时回退并重取
	acceptQuality := filterQualitiesByVip(playUrl.AcceptQuality, isVip, hasCookie)
	effectiveQn := playUrl.CurrentQn
	if effectiveQn != 0 && !qualityListContains(acceptQuality, effectiveQn) && len(acceptQuality) > 0 {
		effectiveQn = acceptQuality[0].ID
	}
	if effectiveQn != 0 && effectiveQn != playUrl.CurrentQn {
		refetched, err := getPgcPlayUrl(target.EpId, target.Cid, cookie, effectiveQn, computeFnval(isVip, effectiveQn), opts.Codec)
		if err != nil {
			if _, ok := err.(*NoPermissionError); !ok {
				return nil, toResolveError(err)
			}
		} else if refetched != nil {
			playUrl = refetched
			acceptQuality = filterQualitiesByVip(playUrl.AcceptQuality, isVip, hasCookie)
		}
	}

	if playUrl.Format == "dash" && playUrl.BestVideo != nil {
		videoBackupUrls, audioBackupUrls := collectBackupUrls(playUrl.BestVideo, playUrl.BestAudio)

		var videoUrl, audioUrl string
		if opts.SkipCdnCheck {
			videoUrl = playUrl.BestVideo.BaseUrl
			if playUrl.BestAudio != nil {
				audioUrl = playUrl.BestAudio.BaseUrl
			}
		} else {
			var videoErr, audioErr error
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				videoUrl, videoErr = findReachableMediaUrl(playUrl.BestVideo.BaseUrl, playUrl.BestVideo.BackupUrl)
			}()
			if playUrl.BestAudio != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					audioUrl, audioErr = findReachableMediaUrl(playUrl.BestAudio.BaseUrl, playUrl.BestAudio.BackupUrl)
				}()
			}
			wg.Wait()
			if videoErr != nil {
				// CLI 场景默认 SkipCdnCheck；此处保底回退 BaseUrl
				logf("[bilibili-pgc] 视频 CDN 本地探测不可达，回退 BaseUrl: %v", videoErr)
				videoUrl = playUrl.BestVideo.BaseUrl
			}
			if playUrl.BestAudio != nil && audioErr != nil {
				logf("[bilibili-pgc] 音频 CDN 均不可达，尝试无音频播放: %v", audioErr)
			}
		}

		videoCodec := playUrl.BestVideo.Codecs
		audioCodec := ""
		if playUrl.BestAudio != nil {
			audioCodec = playUrl.BestAudio.Codecs
		}
		logf("[bilibili-pgc] 解析完成: format=dash, qn=%d, codec=%s", playUrl.CurrentQn, videoCodec)

		result := buildCommon(playUrl)
		result.VideoUrl = videoUrl
		result.AudioUrl = audioUrl
		result.VideoCodec = videoCodec
		result.AudioCodec = audioCodec
		result.AcceptQuality = acceptQuality
		result.VideoBackupUrls = videoBackupUrls
		result.AudioBackupUrls = audioBackupUrls
		return result, nil
	}

	// 请求 DASH 却返回 MP4 的边缘场景（PGC 偶发，如试看）
	if playUrl.Format == "mp4" && len(playUrl.Durl) > 0 && playUrl.Durl[0].Url != "" {
		mp4Url := playUrl.Durl[0].Url
		result := buildCommon(playUrl)
		result.VideoUrl = mp4Url
		result.Format = "mp4"
		result.AcceptQuality = narrowAcceptQualityForPgcMp4(playUrl.AcceptQuality)
		result.VideoBackupUrls = []string{mp4Url}
		if preview && playUrl.Durl[0].Length > 0 {
			previewLen := int(playUrl.Durl[0].Length / 1000)
			if previewLen > 0 && previewLen < target.DurationSec {
				result.Duration = previewLen
			}
		}
		return result, nil
	}

	return nil, &ResolveError{Message: "未找到可用播放地址", Code: "NO_PLAYABLE_URL"}
}

func narrowAcceptQualityForPgcMp4(list []QualityItem) []QualityItem {
	filtered := make([]QualityItem, 0, len(list))
	for _, q := range list {
		if q.ID <= pgcMp4MaxQn {
			filtered = append(filtered, q)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return []QualityItem{
		{ID: 32, Label: qnQualityMap[32].Label, Resolution: qnQualityMap[32].Resolution},
		{ID: 16, Label: qnQualityMap[16].Label, Resolution: qnQualityMap[16].Resolution},
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
