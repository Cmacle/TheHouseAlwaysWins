// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ProtectSecret and UnprotectSecret encrypt a saved password for the
// current OS user, where the platform can (set by the Windows build).
var ProtectSecret, UnprotectSecret func([]byte) ([]byte, error)

// scramble keeps a saved password from being readable at a glance where
// there's no OS encryption. It is not encryption.
func scramble(b []byte) []byte {
	key := []byte("TheHouseAlwaysWins")
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[i] ^ key[i%len(key)] ^ byte(i*31)
	}
	return out
}

// SetPassword saves (or, with "", forgets) the remembered password.
func (c *Config) SetPassword(pw string) {
	if pw == "" {
		c.SavedPassword = ""
		return
	}
	if ProtectSecret != nil {
		if enc, err := ProtectSecret([]byte(pw)); err == nil {
			c.SavedPassword = "os:" + base64.StdEncoding.EncodeToString(enc)
			return
		}
	}
	c.SavedPassword = "x:" + base64.StdEncoding.EncodeToString(scramble([]byte(pw)))
}

// Password is the remembered password, or "".
func (c *Config) Password() string {
	kind, data, ok := strings.Cut(c.SavedPassword, ":")
	if !ok {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return ""
	}
	switch kind {
	case "os":
		if UnprotectSecret == nil {
			return ""
		}
		if pw, err := UnprotectSecret(raw); err == nil {
			return string(pw)
		}
	case "x":
		return string(scramble(raw))
	}
	return ""
}

// Config is the client settings file (client.json, next to the exe).
type Config struct {
	Server     string `json:"server"`
	Username   string `json:"username"`
	Token      string `json:"token,omitempty"`
	RememberMe bool   `json:"remember_me"`
	// SavedPassword is the password when "remember me" is on, so the login
	// form is filled in after a restart. On Windows it's encrypted for this
	// Windows user (DPAPI); elsewhere it's only scrambled, and client.json
	// is kept readable by you alone.
	SavedPassword string `json:"saved_password,omitempty"`
	Sound         bool   `json:"sound"` // false = everything muted
	// Volumes, 0-100. Effects and voice are on top of the master volume.
	Volume        int    `json:"volume"`
	EffectsVolume int    `json:"effects_volume"`
	VoiceVolume   int    `json:"voice_volume"`
	RevealVolume  int    `json:"reveal_volume"` // card reveal suspense and stings
	WindowScale   int    `json:"window_scale"`  // 0 = automatic
	Fullscreen    bool   `json:"fullscreen"`
	AssetsDir     string `json:"assets_dir"`

	path string
}

// LoadConfig reads client.json from dir, creating defaults if needed.
func LoadConfig(dir string) *Config {
	c := &Config{Server: "127.0.0.1:7777", RememberMe: true, Sound: true, AssetsDir: "assets",
		Volume: 80, EffectsVolume: 100, VoiceVolume: 70, RevealVolume: 85}
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
	_ = os.WriteFile(c.path, b, 0o600) // it may hold a saved login
	_ = os.Chmod(c.path, 0o600)
}
