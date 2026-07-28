package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// PersistedConfig 是保存到本地的 CLI 配置。
type PersistedConfig struct {
	Cookie  string `json:"cookie"`
	SavedAt string `json:"savedAt"`
	UserInfo *struct {
		Name      string `json:"name,omitempty"`
		Mid       int64  `json:"mid,omitempty"`
		VipStatus int    `json:"vipStatus,omitempty"`
	} `json:"userInfo,omitempty"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zcontrol-cli")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func loadConfig() (*PersistedConfig, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg PersistedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Cookie == "" {
		return nil, nil
	}
	return &cfg, nil
}

func saveConfig(cfg *PersistedConfig) error {
	if cfg == nil {
		return nil
	}
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o600)
}

func clearConfig() error {
	path := configPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// UserAgent 所有 B站 请求统一使用的 UA。
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func bilibiliHeaders(cookie string) map[string]string {
	h := map[string]string{
		"User-Agent": UserAgent,
		"Referer":    "https://www.bilibili.com",
	}
	if cookie != "" {
		h["Cookie"] = cookie
	}
	return h
}

func goos() string {
	return runtime.GOOS
}

func logf(format string, args ...any) {
	fmt.Printf("[ZViewer CLI] "+format+"\n", args...)
}
