package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// mp4Box 表示一个 MP4 box 的位置和类型。
type mp4Box struct {
	typ   string
	start int
	end   int
}

// findBoxes 在 buffer 中查找所有指定类型的 MP4 box。
func findBoxes(buf []byte, types ...string) []mp4Box {
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	var boxes []mp4Box
	pos := 0
	for pos+8 <= len(buf) {
		size := int(binary.BigEndian.Uint32(buf[pos : pos+4]))
		typ := string(buf[pos+4 : pos+8])
		if size < 8 {
			break
		}
		end := pos + size
		if end > len(buf) {
			end = len(buf)
		}
		if typeSet[typ] {
			boxes = append(boxes, mp4Box{typ: typ, start: pos, end: end})
		}
		// 递归搜索 container box 内部（moov, trak, mdia, minf, stbl 等）
		if typ == "moov" || typ == "trak" || typ == "mdia" || typ == "minf" || typ == "stbl" {
			inner := findBoxes(buf[pos+8:end], types...)
			for i := range inner {
				inner[i].start += pos + 8
				inner[i].end += pos + 8
			}
			boxes = append(boxes, inner...)
		}
		pos += size
	}
	return boxes
}

// fetchInitSegment 通过代理获取 m4s 文件的前 N 字节，用于解析 moov/sidx。
func fetchInitSegment(proxyURL, targetURL string, maxBytes int) ([]byte, error) {
	u := fmt.Sprintf("%s/proxy?url=%s", proxyURL, encodeURIComponent(targetURL))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxBytes-1))
	res, err := proxyHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

// encodeURIComponent 模拟 JavaScript 的 encodeURIComponent。
func encodeURIComponent(s string) string {
	var buf bytes.Buffer
	for _, b := range []byte(s) {
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '!' || b == '~' || b == '*' || b == '\'' || b == '(' || b == ')' {
			buf.WriteByte(b)
		} else {
			buf.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return buf.String()
}

// sidxEntry 表示 sidx box 中的一个 segment 引用。
type sidxEntry struct {
	referencedSize   int
	subsegmentDuration uint32
}

// parseSidx 解析 sidx box 中的 segment 引用列表。
func parseSidx(buf []byte, boxStart, boxEnd int) ([]sidxEntry, error) {
	if boxEnd-boxStart < 20 {
		return nil, fmt.Errorf("sidx box too small")
	}
	data := buf[boxStart:boxEnd]
	// version(1) + flags(3) + reference_ID(4) + timescale(4)
	off := 8 // skip version+flags+reference_ID
	timescale := binary.BigEndian.Uint32(data[off : off+4])
	_ = timescale
	off += 4
	// if version == 0: earliest_presentation_time(4) + first_offset(4)
	// else: earliest_presentation_time(8) + first_offset(8)
	version := data[0]
	if version == 0 {
		off += 8
	} else {
		off += 16
	}
	// reserved(2) + reference_count(2)
	if off+4 > len(data) {
		return nil, fmt.Errorf("sidx header truncated")
	}
	refCount := binary.BigEndian.Uint16(data[off+2 : off+4])
	off += 4

	var entries []sidxEntry
	for i := 0; i < int(refCount); i++ {
		if off+12 > len(data) {
			break
		}
		// reference_type(1bit) + referenced_size(31bits)
		refSize := int(binary.BigEndian.Uint32(data[off:off+4]) & 0x7FFFFFFF)
		subDuration := binary.BigEndian.Uint32(data[off+4 : off+8])
		entries = append(entries, sidxEntry{
			referencedSize:     refSize,
			subsegmentDuration: subDuration,
		})
		off += 12 // 4(size) + 4(duration) + 4(sap)
	}
	return entries, nil
}

// generateMpd 生成 DASH MPD manifest，使用正确的 init range 和 sidx range。
func generateMpd(proxyBase, videoUrl, audioUrl, videoCodec, audioCodec string, duration int) string {
	dur := duration
	if dur <= 0 {
		dur = 600
	}

	// 并行探测 video 和 audio 的 moov/sidx 范围，避免串行等待
	type probeResult struct {
		initRange, indexRange string
	}
	vCh := make(chan probeResult, 1)
	aCh := make(chan probeResult, 1)
	go func() {
		ir, xr := probeM4sRanges(proxyBase, videoUrl)
		vCh <- probeResult{ir, xr}
	}()
	go func() {
		ir, xr := probeM4sRanges(proxyBase, audioUrl)
		aCh <- probeResult{ir, xr}
	}()
	vRes := <-vCh
	aRes := <-aCh
	videoInitRange, videoIndexRange := vRes.initRange, vRes.indexRange
	audioInitRange, audioIndexRange := aRes.initRange, aRes.indexRange

	videoProxyUrl := fmt.Sprintf("%s/proxy?url=%s", proxyBase, encodeURIComponent(videoUrl))
	audioProxyUrl := ""
	if audioUrl != "" {
		audioProxyUrl = fmt.Sprintf("%s/proxy?url=%s", proxyBase, encodeURIComponent(audioUrl))
	}

	var sb bytes.Buffer
	fmt.Fprintf(&sb, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprintf(&sb, `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT%dS" minBufferTime="PT2S">`+"\n", dur)
	sb.WriteString("  <Period>\n")

	// Video AdaptationSet
	sb.WriteString(`    <AdaptationSet mimeType="video/mp4" segmentAlignment="true">` + "\n")
	fmt.Fprintf(&sb, `      <Representation id="v0" codecs="%s" bandwidth="2000000">`+"\n", videoCodec)
	fmt.Fprintf(&sb, `        <BaseURL>%s</BaseURL>`+"\n", escapeXML(videoProxyUrl))
	if videoIndexRange != "" {
		fmt.Fprintf(&sb, `        <SegmentBase indexRange="%s">`+"\n", videoIndexRange)
	} else {
		fmt.Fprintf(&sb, `        <SegmentBase indexRange="0-1000">`+"\n")
	}
	if videoInitRange != "" {
		fmt.Fprintf(&sb, `          <Initialization range="%s" />`+"\n", videoInitRange)
	} else {
		fmt.Fprintf(&sb, `          <Initialization range="0-500" />`+"\n")
	}
	sb.WriteString("        </SegmentBase>\n")
	sb.WriteString("      </Representation>\n")
	sb.WriteString("    </AdaptationSet>\n")

	// Audio AdaptationSet
	if audioProxyUrl != "" {
		sb.WriteString(`    <AdaptationSet mimeType="audio/mp4" segmentAlignment="true">` + "\n")
		fmt.Fprintf(&sb, `      <Representation id="a0" codecs="%s" bandwidth="128000">`+"\n", audioCodec)
		fmt.Fprintf(&sb, `        <BaseURL>%s</BaseURL>`+"\n", escapeXML(audioProxyUrl))
		if audioIndexRange != "" {
			fmt.Fprintf(&sb, `        <SegmentBase indexRange="%s">`+"\n", audioIndexRange)
		} else {
			fmt.Fprintf(&sb, `        <SegmentBase indexRange="0-1000">`+"\n")
		}
		if audioInitRange != "" {
			fmt.Fprintf(&sb, `          <Initialization range="%s" />`+"\n", audioInitRange)
		} else {
			fmt.Fprintf(&sb, `          <Initialization range="0-500" />`+"\n")
		}
		sb.WriteString("        </SegmentBase>\n")
		sb.WriteString("      </Representation>\n")
		sb.WriteString("    </AdaptationSet>\n")
	}

	sb.WriteString("  </Period>\n")
	sb.WriteString("</MPD>")
	return sb.String()
}

// probeM4sRanges 通过代理预下载 m4s 文件头部，解析 moov 和 sidx box 的字节范围。
// 采用渐进式策略：先下载 32KB，找不到 sidx 再扩大到 256KB，最后 1MB。
func probeM4sRanges(proxyBase, targetURL string) (initRange, indexRange string) {
	if targetURL == "" {
		return
	}
	// 渐进式探测大小：32KB → 256KB → 1MB
	probeSizes := []int{32 * 1024, 256 * 1024, 1024 * 1024}
	for _, size := range probeSizes {
		buf, err := fetchInitSegment(proxyBase, targetURL, size)
		if err != nil {
			logf("预下载 m4s 失败 (%d bytes): %v", size, err)
			return
		}
		boxes := findBoxes(buf, "moov", "sidx")
		for _, b := range boxes {
			if b.typ == "moov" {
				initRange = fmt.Sprintf("0-%d", b.end-1)
			}
			if b.typ == "sidx" {
				indexRange = fmt.Sprintf("%d-%d", b.start, b.end-1)
			}
		}
		// 同时找到 moov 和 sidx 即可返回
		if initRange != "" && indexRange != "" {
			return
		}
		// 如果已找到 moov 但没找到 sidx，且 buf 不足 size（文件太小），不再扩大
		if len(buf) < size {
			break
		}
	}
	if initRange == "" {
		logf("未找到 moov box，使用默认 init range")
	}
	if indexRange == "" {
		logf("未找到 sidx box，使用默认 index range")
	}
	return
}

func escapeXML(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// handleDashMpd 生成本地 DASH MPD manifest。
// 接受前端传入的原始 videoUrl/audioUrl（来自 /resolve 的 sourceVideoUrl/sourceAudioUrl），
// 直接生成 MPD，避免重复调用 ResolveBilibiliVideo 导致二次解析失败。
func (a *Agent) handleDashMpd(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	videoUrl := q.Get("videoUrl")
	audioUrl := q.Get("audioUrl")
	videoCodec := q.Get("videoCodec")
	audioCodec := q.Get("audioCodec")
	durationRaw := q.Get("duration")

	if videoUrl == "" {
		http.Error(w, `{"error":"缺少 videoUrl 参数"}`, http.StatusBadRequest)
		return
	}

	if videoCodec == "" {
		videoCodec = "avc1.64001E"
	}
	if audioCodec == "" {
		audioCodec = "mp4a.40.2"
	}
	duration := 0
	if durationRaw != "" {
		duration, _ = strconv.Atoi(durationRaw)
	}

	mpd := generateMpd(a.proxyURL(), videoUrl, audioUrl, videoCodec, audioCodec, duration)

	w.Header().Set("Content-Type", "application/dash+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(mpd))
}
