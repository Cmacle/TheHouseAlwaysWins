// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/release"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/selfupdate"
)

// ChatMaxLen is the longest chat message.
const ChatMaxLen = 80

// ------------------------------------------------------------------ chat

// onChat handles a chat message: shown in a bubble by the player's seat, or
// for admins starting with "/", run as a server command.
func (g *Game) onChat(p *player, text string) {
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if text == "" || p.conn == nil {
		return
	}
	if strings.HasPrefix(text, "/") {
		if !g.store.IsAdmin(p.user) {
			g.errTo(p, "Only admins can use / commands.")
			return
		}
		runAdminCommand(g.log, g.exec, p.conn, p.user, text)
		return
	}
	if !g.cfg.TableChat {
		g.errTo(p, "Chat is turned off on this server.")
		return
	}
	// a little flood control: 4 messages per 10 seconds
	now := time.Now()
	keep := p.chatAt[:0]
	for _, t := range p.chatAt {
		if now.Sub(t) < 10*time.Second {
			keep = append(keep, t)
		}
	}
	p.chatAt = keep
	if len(p.chatAt) >= 4 {
		g.errTo(p, "Slow down a little.")
		return
	}
	p.chatAt = append(p.chatAt, now)
	if r := []rune(text); len(r) > ChatMaxLen {
		text = string(r[:ChatMaxLen])
	}
	msg := proto.Chat{From: p.user, Seat: p.seat, Text: text, Admin: g.store.IsAdmin(p.user), At: now.UnixMilli()}
	g.log.Printf("chat %s: %s", p.user, text)
	for c := range g.conns {
		c.send(proto.SChat, msg)
	}
}

// ---------------------------------------------------------------- update

type updateState struct {
	mu      sync.Mutex
	latest  string
	checked time.Time
	busy    bool
	status  string
	restart bool
}

// serverAsset is the release file for this server's platform.
func serverAsset() string {
	switch runtime.GOOS {
	case "windows":
		return "HouseServer.exe"
	case "linux":
		if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
			return "house-server-linux-" + runtime.GOARCH
		}
	}
	return ""
}

// Latest is the newest released version, asked of GitHub at most every 30
// minutes unless force is set. "" if it couldn't be found out.
func (s *Server) Latest(force bool) string {
	s.upd.mu.Lock()
	if !force && time.Since(s.upd.checked) < 30*time.Minute {
		v := s.upd.latest
		s.upd.mu.Unlock()
		return v
	}
	s.upd.checked = time.Now()
	s.upd.mu.Unlock()
	v, err := release.Latest("HouseServer/" + Version)
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	if err != nil {
		s.Log.Printf("update check: %v", err)
		return s.upd.latest
	}
	s.upd.latest = v
	return v
}

// UpdateStatus says what the updater is doing (for the server window).
func (s *Server) UpdateStatus() (latest, status string, busy bool) {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	return s.upd.latest, s.upd.status, s.upd.busy
}

func (s *Server) setUpdStatus(st string) {
	s.upd.mu.Lock()
	s.upd.status = st
	s.upd.mu.Unlock()
}

// Update downloads the newest release from GitHub, checks it, swaps it in
// and restarts the server. With force it reinstalls even when this is
// already the newest version. Progress goes to w.
func (s *Server) Update(w io.Writer, force bool) {
	s.upd.mu.Lock()
	if s.upd.busy {
		s.upd.mu.Unlock()
		fmt.Fprintln(w, "  an update is already running")
		return
	}
	s.upd.busy = true
	s.upd.mu.Unlock()
	defer func() {
		s.upd.mu.Lock()
		s.upd.busy = false
		s.upd.mu.Unlock()
	}()
	asset := serverAsset()
	if asset == "" {
		fmt.Fprintf(w, "  there are no server builds for %s/%s on GitHub\n", runtime.GOOS, runtime.GOARCH)
		return
	}
	s.setUpdStatus("Checking GitHub...")
	v := s.Latest(true)
	if v == "" {
		s.setUpdStatus("Couldn't reach GitHub.")
		fmt.Fprintln(w, "  couldn't find out the latest version (see the log)")
		return
	}
	if !force && !release.Newer(v, Version) {
		s.setUpdStatus("Up to date (v" + Version + ").")
		fmt.Fprintf(w, "  already up to date: this is v%s, the latest is v%s (\"update force\" reinstalls)\n", Version, v)
		return
	}
	fmt.Fprintf(w, "  downloading v%s (%s)...\n", v, asset)
	s.Log.Printf("updating to v%s", v)
	s.setUpdStatus("Downloading v" + v + "...")
	last := time.Now()
	path, err := release.Download(v, asset, selfupdate.Dir(), "HouseServer/"+Version, nil, func(got, size int64) {
		if time.Since(last) > 500*time.Millisecond {
			last = time.Now()
			if size > 0 {
				s.setUpdStatus(fmt.Sprintf("Downloading v%s... %d%%", v, got*100/size))
			}
		}
	})
	if err != nil {
		s.setUpdStatus("Update failed: " + err.Error())
		s.Log.Printf("update failed: %v", err)
		fmt.Fprintln(w, "  update failed:", err)
		return
	}
	s.backup("before-update")
	if err := selfupdate.Swap(path); err != nil {
		os.Remove(path)
		s.setUpdStatus("Update failed: " + err.Error())
		s.Log.Printf("update failed: %v", err)
		fmt.Fprintln(w, "  couldn't replace the server program:", err)
		return
	}
	s.setUpdStatus("Restarting into v" + v + "...")
	fmt.Fprintf(w, "  v%s is installed and checked. Restarting now; reconnect in a few seconds.\n", v)
	s.Log.Printf("v%s installed; restarting", v)
	s.Lobby.Announce(fmt.Sprintf("The house is updating to v%s. Back in a few seconds.", v))
	s.upd.mu.Lock()
	s.upd.restart = true
	s.upd.mu.Unlock()
	go func() {
		time.Sleep(1500 * time.Millisecond) // let the messages go out
		s.quit()
	}()
}

// restartPending reports whether the server should come back up as the new
// version after shutting down.
func (s *Server) restartPending() bool {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	return s.upd.restart
}

// supervised reports whether something restarts the server when it exits
// (the systemd unit, run-server.bat): then it just exits after an update.
func supervised() bool {
	return os.Getenv("HOUSE_SUPERVISED") != "" || os.Getenv("INVOCATION_ID") != ""
}

// finishRestart runs after shutdown when an update is waiting.
func (s *Server) finishRestart() {
	if supervised() {
		s.Log.Printf("exiting; the service will start the new version")
		return
	}
	if err := selfupdate.Restart(); err != nil {
		s.Log.Printf("couldn't start the new version (%v); putting the old one back", err)
		exe, _ := os.Executable()
		selfupdate.Unswap(filepath.Join(filepath.Dir(exe), serverAsset()+".download"))
		selfupdate.Restart()
	}
}

func (s *Server) reload() error {
	if s.reloadFn == nil {
		return fmt.Errorf("reload isn't available here")
	}
	return s.reloadFn()
}

func (s *Server) quit() {
	if s.quitFn != nil {
		s.quitFn()
	}
}

// Quit shuts the server down (saving first).
func (s *Server) Quit() { s.quit() }

// Config is the configuration in use.
func (s *Server) Config() *Config { return s.cfg() }

// ConfigPath is server.json's location.
func (s *Server) ConfigPath() string { return s.configPath }
