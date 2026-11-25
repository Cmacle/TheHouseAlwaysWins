// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build windows

package win32

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// The client keeps a short diagnostic log, client.log, next to the exe (or
// in the temp folder if that one can't be written). It records how start-up
// went, any drawing failures, crashes, and where the game was stuck if the
// window stops updating, so a player with a blank window can send it in.

var (
	logMu   sync.Mutex
	logFile *os.File
	logPath string

	heartbeat atomic.Int64 // unix nanos of the last finished frame
	stage     atomic.Value // string: what the main loop is doing
)

// OpenLog starts client.log in dir. The previous run's log is kept as
// client-previous.log, so a crash's log survives starting the game again.
// A crash anywhere in the game (any goroutine, which a recover can't catch)
// also writes its details into the log.
func OpenLog(dir string) string {
	logMu.Lock()
	defer logMu.Unlock()
	for _, d := range []string{dir, os.TempDir()} {
		p := filepath.Join(d, "client.log")
		_ = os.Rename(p, filepath.Join(d, "client-previous.log"))
		if f, err := os.Create(p); err == nil {
			logFile, logPath = f, p
			_ = debug.SetCrashOutput(f, debug.CrashOptions{})
			return p
		}
	}
	return ""
}

// LogPath is where the log is being written, or "".
func LogPath() string { logMu.Lock(); defer logMu.Unlock(); return logPath }

// Logf adds a time-stamped line to the log.
func Logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile == nil {
		return
	}
	fmt.Fprintf(logFile, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	logFile.Sync()
}

// LogStacks writes every goroutine's stack to the log.
func LogStacks(why string) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	Logf("%s\n%s", why, buf[:n])
}

func setStage(s string) { stage.Store(s) }

// watchdog logs where the game is stuck when the main loop stops finishing
// frames for several seconds, and notes when it recovers.
func watchdog() {
	stuck := false
	for range time.Tick(time.Second) {
		last := heartbeat.Load()
		if last == 0 {
			continue
		}
		idle := time.Since(time.Unix(0, last))
		switch {
		case idle > 8*time.Second && !stuck:
			stuck = true
			s, _ := stage.Load().(string)
			LogStacks(fmt.Sprintf("WINDOW NOT UPDATING for %s (during: %s)", idle.Round(time.Second), s))
		case idle < 2*time.Second && stuck:
			stuck = false
			Logf("window updating again")
		}
	}
}

// LogSystem records the Windows version and screen setup.
func LogSystem() {
	var vi struct {
		Size, Major, Minor, Build, Platform uint32
		CSD                                 [128]uint16
	}
	vi.Size = uint32(unsafe.Sizeof(vi))
	if p := ntdll.NewProc("RtlGetVersion"); p.Find() == nil {
		p.Call(uintptr(unsafe.Pointer(&vi)))
	}
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := pGetSystemMetrics.Call(smCyScreen)
	dc, _, _ := pGetDC.Call(0)
	dpi, _, _ := pGetDeviceCaps.Call(dc, 88) // LOGPIXELSX
	bpp, _, _ := pGetDeviceCaps.Call(dc, 12) // BITSPIXEL
	pReleaseDC.Call(0, dc)
	Logf("windows %d.%d build %d, screen %dx%d, %d dpi, %d-bit colour, %s/%s",
		vi.Major, vi.Minor, vi.Build, sw, sh, dpi, bpp, runtime.GOOS, runtime.GOARCH)
}
