package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PersistedConfig 是保存到本地的 CLI 配置。
type PersistedConfig struct {
	Cookie   string `json:"cookie"`
	SavedAt  string `json:"savedAt"`
	UserInfo *struct {
		Name      string `json:"name,omitempty"`
		Mid       int64  `json:"mid,omitempty"`
		VipStatus int    `json:"vipStatus,omitempty"`
	} `json:"userInfo,omitempty"`
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
	if cfg.Cookie == "" {
		return nil, nil
	}
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

func newUserInfoMap(name string, mid int64, vipStatus int) map[string]any {
	return map[string]any{
		"name":      name,
		"mid":       mid,
		"vipStatus": vipStatus,
	}
}
