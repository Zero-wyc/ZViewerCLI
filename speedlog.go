package main

import (
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"
)

// speedLogInterval 速度日志定期输出间隔。
const speedLogInterval = 5 * time.Second

// speedLoggerReader 在读取上游响应体的同时统计字节数并输出速度日志。
type speedLoggerReader struct {
	r     io.Reader
	label string
	start time.Time
	total int64
	mu    sync.Mutex
	done  chan struct{}
	once  sync.Once
}

// newSpeedLoggerReader 创建一个带速度统计的 reader。
// label 会显示在日志中，通常使用上游 URL 的 host。
func newSpeedLoggerReader(r io.Reader, label string) *speedLoggerReader {
	s := &speedLoggerReader{
		r:     r,
		label: label,
		start: time.Now(),
		done:  make(chan struct{}),
	}
	go s.logLoop()
	return s
}

func (s *speedLoggerReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.mu.Lock()
		s.total += int64(n)
		s.mu.Unlock()
	}
	if err != nil {
		s.finish()
	}
	return n, err
}

// finish 结束速度统计并输出最终日志。
func (s *speedLoggerReader) finish() {
	s.once.Do(func() { close(s.done) })
}

func (s *speedLoggerReader) logLoop() {
	ticker := time.NewTicker(speedLogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.logCurrent()
		case <-s.done:
			s.logFinal()
			return
		}
	}
}

func (s *speedLoggerReader) logCurrent() {
	s.mu.Lock()
	total := s.total
	elapsed := time.Since(s.start)
	s.mu.Unlock()
	if elapsed < time.Second || total == 0 {
		return
	}
	speed := float64(total) / elapsed.Seconds()
	logf("[proxy] %s 速度 %s/s, 已传输 %s", s.label, formatSpeed(speed), formatBytes(total))
}

func (s *speedLoggerReader) logFinal() {
	s.mu.Lock()
	total := s.total
	elapsed := time.Since(s.start)
	s.mu.Unlock()
	if total == 0 {
		return
	}
	speed := float64(total) / elapsed.Seconds()
	logf("[proxy] %s 完成，平均速度 %s/s, 总大小 %s, 耗时 %.2fs",
		s.label, formatSpeed(speed), formatBytes(total), elapsed.Seconds())
}

// hostFromURL 从 URL 中提取 host，失败时返回原始字符串。
func hostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

func formatSpeed(bps float64) string {
	return formatBytes(int64(bps))
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	for n >= div*unit && exp < len(units)-1 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), units[exp])
}
