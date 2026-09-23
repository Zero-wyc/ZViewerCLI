package main

import "sync"

// LocalConfig 保存连接配置。
// 2026-09 去房间化：CLI 只配置服务器地址即可全局注册（对所有房间可用），
// User 为注册归属的用户名（网页端配置页经 ?user= 传入，供前端按用户过滤）。
type LocalConfig struct {
	ServerURL string `json:"serverUrl"`
	User      string `json:"user,omitempty"`
	Cookie    string `json:"cookie"`
}

// State 代理状态，用于服务端读取。
type State struct {
	mu          sync.RWMutex
	Connected   bool           `json:"connected"`
	Connecting  bool           `json:"connecting"`
	Config      *LocalConfig   `json:"config"`
	UserInfo    map[string]any `json:"userInfo"`
	CookieValid *bool          `json:"cookieValid"`
	LastError   string         `json:"lastError"`
}

func (s *State) SetConnected(v bool) {
	s.mu.Lock()
	s.Connected = v
	s.Connecting = false
	s.mu.Unlock()
}

func (s *State) SetConnecting(v bool) {
	s.mu.Lock()
	s.Connecting = v
	s.mu.Unlock()
}

func (s *State) SetConfig(c *LocalConfig) {
	s.mu.Lock()
	s.Config = c
	s.mu.Unlock()
}

func (s *State) SetCookieValid(v bool) {
	s.mu.Lock()
	b := v
	s.CookieValid = &b
	s.mu.Unlock()
}

func (s *State) SetUserInfo(info map[string]any) {
	s.mu.Lock()
	s.UserInfo = info
	s.mu.Unlock()
}

func (s *State) SetLastError(err string) {
	s.mu.Lock()
	s.LastError = err
	s.mu.Unlock()
}
