package client

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the client settings file (client.json, next to the exe).
type Config struct {
	Server      string `json:"server"`
	Username    string `json:"username"`
	Token       string `json:"token,omitempty"`
	RememberMe  bool   `json:"remember_me"`
	Sound       bool   `json:"sound"`
	WindowScale int    `json:"window_scale"` // 0 = automatic
	Fullscreen  bool   `json:"fullscreen"`
	AssetsDir   string `json:"assets_dir"`

	path string
}

// LoadConfig reads client.json from dir, creating defaults if needed.
func LoadConfig(dir string) *Config {
	c := &Config{Server: "127.0.0.1:7777", RememberMe: true, Sound: true, AssetsDir: "assets"}
	c.path = filepath.Join(dir, "client.json")
	if b, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(b, c)
	} else {
		c.Save()
	}
	if c.AssetsDir == "" {
		c.AssetsDir = "assets"
	}
	if !filepath.IsAbs(c.AssetsDir) {
		c.AssetsDir = filepath.Join(dir, c.AssetsDir)
	}
	return c
}

// Save writes the config.
func (c *Config) Save() {
	if c.path == "" {
		return
	}
	cp := *c
	if rel, err := filepath.Rel(filepath.Dir(c.path), c.AssetsDir); err == nil {
		cp.AssetsDir = rel
	}
	b, _ := json.MarshalIndent(cp, "", "  ")
	_ = os.WriteFile(c.path, b, 0o644)
}
