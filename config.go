package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PersistedConfig 是保存到本地的 CLI 配置。
type PersistedConfig struct {
	Cookie  string `json:"cookie"`
	SavedAt string `json:"savedAt"`
	// ResolveSource 是解析来源偏好：host（房主直链，默认）或 local（本地 Cookie）。
	// 空值表示旧版配置，读取时按 host 处理。
	ResolveSource string `json:"resolveSource,omitempty"`
	UserInfo      *struct {
		Name      string `json:"name,omitempty"`
		Mid       int64  `json:"mid,omitempty"`
		VipStatus int    `json:"vipStatus,omitempty"`
	} `json:"userInfo,omitempty"`
}

const (
	// resolveSourceHost：服务端用房主的 B站 身份解析，观众无需登录。
	resolveSourceHost = "host"
	// resolveSourceLocal：用本机 Cookie 解析，少一次服务端往返、首帧更快。
	resolveSourceLocal = "local"
)

// normalizeResolveSource 把任意输入归一化成 host / local，未知值按 host。
func normalizeResolveSource(v string) string {
	if v == resolveSourceLocal {
		return resolveSourceLocal
	}
	return resolveSourceHost
}

const configDirName = ".zviewer"
const configFileName = "config.json"

// configDir 返回新的配置目录路径（~/.zviewer）。
func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, configDirName)
}

// configPath 返回新的配置文件路径。
func configPath() string {
	return filepath.Join(configDir(), configFileName)
}

// oldConfigPath 是旧版配置路径，用于一次性迁移。
func oldConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zcontrol-cli", configFileName)
}

// migrateOldConfig 将旧版 ~/.zcontrol-cli/config.json 迁移到 ~/.zviewer/config.json。
func migrateOldConfig() error {
	oldPath := oldConfigPath()
	newPath := configPath()
	if _, err := os.Stat(newPath); err == nil {
		return nil // 新配置已存在，不迁移
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(newPath, data, 0o600); err != nil {
		return err
	}
	logf("已迁移旧版配置到 %s", newPath)
	return nil
}

func loadConfig() (*PersistedConfig, error) {
	_ = migrateOldConfig()
	data, err := os.ReadFile(configPath())
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
	// 不再因 Cookie 为空返回 nil：房主直链模式（默认）本来就不需要登录，
	// 解析来源偏好必须能单独持久化。
	return &cfg, nil
}

func saveConfig(cfg *PersistedConfig) error {
	if cfg == nil {
		return nil
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o600)
}

func clearConfig() error {
	if err := os.Remove(configPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// saveUserConfig 将验证后的用户信息持久化到本地。
func saveUserConfig(cookie string, info *UserValidation) error {
	cfg := &PersistedConfig{
		Cookie:  cookie,
		SavedAt: nowISO(),
	}
	// 登录/刷新 Cookie 不应覆盖已有的解析来源选择。
	if prev, err := loadConfig(); err == nil && prev != nil {
		cfg.ResolveSource = prev.ResolveSource
	}
	if info != nil {
		cfg.UserInfo = &struct {
			Name      string `json:"name,omitempty"`
			Mid       int64  `json:"mid,omitempty"`
			VipStatus int    `json:"vipStatus,omitempty"`
		}{
			Name:      info.Name,
			Mid:       info.Mid,
			VipStatus: info.VipStatus,
		}
	}
	return saveConfig(cfg)
}

// saveResolveSource 单独持久化解析来源（允许在没有 Cookie 的情况下保存）。
func saveResolveSource(source string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &PersistedConfig{}
	}
	cfg.ResolveSource = normalizeResolveSource(source)
	return saveConfig(cfg)
}

func newUserInfoMap(name string, mid int64, vipStatus int) map[string]any {
	return map[string]any{
		"name":      name,
		"mid":       mid,
		"vipStatus": vipStatus,
	}
}
