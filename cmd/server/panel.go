package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/release"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/server"
)

// panelBackend connects the server window to the running server.
type panelBackend struct {
	s       *server.Server
	log     *server.LogBuffer
	stopped atomic.Bool

	mu     sync.Mutex
	status client.PanelStatus
}

func newPanelBackend(s *server.Server, lb *server.LogBuffer) *panelBackend {
	b := &panelBackend{s: s, log: lb}
	b.refresh()
	go func() {
		for !b.stopped.Load() {
			time.Sleep(time.Second)
			b.refresh()
		}
	}()
	go func() { // ask GitHub once at start (cached after that)
		b.s.Latest(false)
		b.refresh()
	}()
	return b
}

// refresh gathers the header info off the window's thread (some of it
// waits on the game loop or GitHub).
func (b *panelBackend) refresh() {
	if b.stopped.Load() {
		return
	}
	cfg := b.s.Config()
	_, _, names, _ := b.s.Game.Summary()
	latest, upd, busy := b.s.UpdateStatus()
	host := cfg.ListenAddress
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "all addresses"
	}
	st := client.PanelStatus{
		Name: cfg.ServerName, Version: server.Version,
		Address: "port " + strconv.Itoa(cfg.Port) + " on " + host,
		Players: names, Latest: latest, Newer: release.Newer(latest, server.Version),
		Update: upd, Updating: busy,
	}
	if ip := localIP(); ip != "" {
		st.Address += " (this PC: " + net.JoinHostPort(ip, strconv.Itoa(cfg.Port)) + ")"
	}
	b.mu.Lock()
	b.status = st
	b.mu.Unlock()
}

func (b *panelBackend) Status() client.PanelStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.status
	st.Stopped = b.stopped.Load()
	if _, upd, busy := b.s.UpdateStatus(); busy || upd != "" {
		st.Update, st.Updating = upd, busy
	}
	return st
}

func (b *panelBackend) Settings() []proto.SettingInfo { return b.s.SettingInfos(false) }

func (b *panelBackend) SetSetting(k, v string) error {
	if err := b.s.SetSetting(k, v); err != nil {
		return err
	}
	fmt.Fprintf(b.log, "setting %s changed to %s\n", k, v)
	return nil
}

func (b *panelBackend) Exec(line string) {
	var out bytes.Buffer
	b.s.Exec(line, &out)
	fmt.Fprintf(b.log, "> %s\n%s", line, out.String())
	if out.Len() > 0 && out.Bytes()[out.Len()-1] != '\n' {
		fmt.Fprintln(b.log)
	}
	b.refresh()
}

func (b *panelBackend) Log(from int) ([]string, int) { return b.log.Since(from) }

func (b *panelBackend) Update() {
	fmt.Fprintln(b.log, "> update")
	b.s.Update(b.log, false)
}

func (b *panelBackend) Quit() { b.s.Quit() }

// localIP is this machine's LAN address, for telling friends where to connect.
func localIP() string {
	c, err := net.Dial("udp", "192.0.2.1:9") // nothing is sent
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() {
		return a.IP.String()
	}
	return ""
}

// runWithWindow runs the server with its window. It returns when the window
// is closed (the server is stopped first) or the server stops.
func runWithWindow(cfgPath string, console bool) error {
	lb := server.NewLogBuffer(3000)
	started := make(chan *server.Server, 1)
	done := make(chan error, 1)
	go func() {
		done <- server.Main(cfgPath, server.Options{Console: console, LogTo: lb, Started: func(s *server.Server) { started <- s }})
	}()
	var s *server.Server
	select {
	case s = <-started:
	case err := <-done:
		return err
	}
	b := newPanelBackend(s, lb)
	finished := make(chan error, 1)
	go func() {
		err := <-done
		b.stopped.Store(true)
		finished <- err
	}()
	assets, err := client.LoadAssets("")
	if err != nil {
		return err
	}
	title := "House Server"
	if n := s.Config().ServerName; n != "" {
		title = n + " - House Server"
	}
	if werr := runWindow(title, assets.Icon, func(p client.Platform) *client.App { return client.NewPanel(p, assets, b) }); werr != nil {
		fmt.Fprintln(os.Stderr, "server window:", werr)
		if !b.stopped.Load() {
			// no window after all: keep running headless
			return <-finished
		}
	}
	if !b.stopped.Load() {
		s.Quit()
	}
	select {
	case err := <-finished:
		return err
	case <-time.After(30 * time.Second):
		return fmt.Errorf("the server didn't stop in time")
	}
}
