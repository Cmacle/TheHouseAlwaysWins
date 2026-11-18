// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build linux

package x11

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Sounds, the picture picker and the clipboard use the desktop's own little
// programs when they're installed (they almost always are).

var (
	playerOnce sync.Once
	player     []string
	playing    atomic.Int32
	soundDir   string
	soundFiles sync.Map // hash -> path
)

func findPlayer() {
	for _, p := range [][]string{{"pw-play"}, {"paplay"}, {"aplay", "-q"}} {
		if _, err := exec.LookPath(p[0]); err == nil {
			player = p
			break
		}
	}
	soundDir = filepath.Join(os.TempDir(), "thehousealwayswins-sounds-"+strings.ReplaceAll(os.Getenv("USER"), "/", ""))
	os.MkdirAll(soundDir, 0o700)
}

func playSound(wav []byte) {
	if len(wav) == 0 {
		return
	}
	playerOnce.Do(findPlayer)
	if player == nil || playing.Load() >= 8 {
		return
	}
	sum := sha1.Sum(wav)
	key := hex.EncodeToString(sum[:8])
	path, ok := soundFiles.Load(key)
	if !ok {
		p := filepath.Join(soundDir, key+".wav")
		if _, err := os.Stat(p); err != nil {
			if os.WriteFile(p, wav, 0o600) != nil {
				return
			}
		}
		soundFiles.Store(key, p)
		path = p
	}
	cmd := exec.Command(player[0], append(player[1:], path.(string))...)
	if cmd.Start() != nil {
		return
	}
	playing.Add(1)
	go func() {
		cmd.Wait()
		playing.Add(-1)
	}()
}

func pickImage() (string, bool) {
	tries := [][]string{
		{"zenity", "--file-selection", "--title=Choose a profile picture", "--file-filter=Pictures | *.png *.jpg *.jpeg *.gif *.PNG *.JPG *.JPEG *.GIF"},
		{"kdialog", "--title", "Choose a profile picture", "--getopenfilename", ".", "*.png *.jpg *.jpeg *.gif|Pictures"},
		{"yad", "--file", "--title=Choose a profile picture"},
	}
	for _, t := range tries {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		out, err := exec.Command(t[0], t[1:]...).Output()
		path := strings.TrimSpace(string(out))
		return path, err == nil && path != ""
	}
	return "", false
}

func clipboardText() string {
	var tries [][]string
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		tries = append(tries, []string{"wl-paste", "-n"})
	}
	tries = append(tries, []string{"xclip", "-selection", "clipboard", "-o"}, []string{"xsel", "-b", "-o"})
	for _, t := range tries {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		out, err := exec.CommandContext(ctx, t[0], t[1:]...).Output()
		cancel()
		if err == nil {
			return string(out)
		}
	}
	return ""
}
