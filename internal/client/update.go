package client

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Auto-update. When the server you connect to is a newer version than this
// client, you're offered the update. It always comes from the game's
// official GitHub releases (never from the server itself), and it's checked
// against the release's SHA256SUMS.txt before it's installed.

// ReleaseBase is where release files are downloaded from; %s is the version.
var ReleaseBase = "https://github.com/Cmacle/TheHouseAlwaysWins/releases/download/v%s/"

// Updater is a platform that can install a new version of the client.
type Updater interface {
	// UpdateAsset is the release file for this platform ("" = can't update).
	UpdateAsset() string
	// UpdateDir is a folder the new version can be downloaded into.
	UpdateDir() string
	// InstallUpdate installs the downloaded file. Desktop platforms replace
	// the program and start the new one (the app then quits); Android opens
	// the system installer.
	InstallUpdate(path string) error
}

type updState int

const (
	updNone updState = iota
	updOffer
	updDownloading
	updInstalling
	updDone // handed to the installer / restarting
	updError
)

type updater struct {
	mu        sync.Mutex
	state     updState
	version   string // version on offer
	got, size int64
	err       string
	dismissed string // version the player said "later" to
	show      bool
	cancel    chan struct{}
	ready     string // downloaded and verified file
}

// newerVersion reports whether version a is newer than b ("1.10.0" > "1.9.2").
// Anything that isn't a version number (like "dev") is never newer or older.
func newerVersion(a, b string) bool {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// checkForUpdate runs when the server says hello.
func (a *App) checkForUpdate() {
	si := a.serverInfo
	up, ok := a.P.(Updater)
	if !ok || up.UpdateAsset() == "" || !si.Updates || !newerVersion(si.ServerVersion, Version) {
		return
	}
	u := &a.upd
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.state != updNone && u.state != updOffer && u.state != updError {
		return // already on it
	}
	if u.dismissed == si.ServerVersion {
		return
	}
	u.state, u.version, u.show, u.err = updOffer, si.ServerVersion, true, ""
}

// startUpdate downloads and checks the new version in the background.
func (a *App) startUpdate() {
	up, ok := a.P.(Updater)
	if !ok {
		return
	}
	u := &a.upd
	u.mu.Lock()
	ver := u.version
	u.state, u.got, u.size, u.err = updDownloading, 0, 0, ""
	cancel := make(chan struct{})
	u.cancel = cancel
	u.mu.Unlock()
	asset, dir := up.UpdateAsset(), up.UpdateDir()
	go func() {
		path, err := downloadRelease(ver, asset, dir, cancel, func(got, size int64) {
			u.mu.Lock()
			u.got, u.size = got, size
			u.mu.Unlock()
		})
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.cancel != cancel {
			return // cancelled
		}
		if err != nil {
			u.state, u.err = updError, err.Error()
			return
		}
		u.state, u.ready = updInstalling, path
	}()
}

// downloadRelease fetches one release file and checks it against the
// release's SHA256SUMS.txt. It returns the path of the verified file.
func downloadRelease(version, asset, dir string, cancel <-chan struct{}, progress func(got, size int64)) (string, error) {
	base := fmt.Sprintf(ReleaseBase, version)
	if t := os.Getenv("HOUSE_TEST_RELEASES"); t != "" { // for testing updates locally
		base = strings.TrimRight(t, "/") + "/"
	}
	hc := &http.Client{Timeout: 15 * time.Minute}
	get := func(url string) (*http.Response, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "TheHouseAlwaysWins/"+Version)
		resp, err := hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("couldn't reach GitHub: %v", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("the v%s download isn't available yet (HTTP %d)", version, resp.StatusCode)
		}
		return resp, nil
	}
	// the checksum list first
	resp, err := get(base + "SHA256SUMS.txt")
	if err != nil {
		return "", err
	}
	want := ""
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
		}
	}
	resp.Body.Close()
	if len(want) != 64 {
		return "", fmt.Errorf("v%s has no checksum for %s", version, asset)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, asset+".download")
	resp, err = get(base + asset)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var got int64
	buf := make([]byte, 64*1024)
	const limit = 400 << 20
	for {
		select {
		case <-cancel:
			f.Close()
			os.Remove(path)
			return "", errors.New("cancelled")
		default:
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				os.Remove(path)
				return "", err
			}
			h.Write(buf[:n])
			got += int64(n)
			progress(got, resp.ContentLength)
			if got > limit {
				f.Close()
				os.Remove(path)
				return "", errors.New("download is too big")
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(path)
			return "", fmt.Errorf("download failed: %v", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		os.Remove(path)
		return "", errors.New("the download didn't match its checksum, so it was thrown away")
	}
	return path, nil
}

// drawUpdate draws the update window when there's one to show, over
// whatever screen is up. It returns true while the window is open.
func (a *App) drawUpdate() {
	if !a.updateOpen() {
		return
	}
	u := &a.upd
	u.mu.Lock()
	state, show, ver, got, size, msg, ready := u.state, u.show, u.version, u.got, u.size, u.err, u.ready
	u.mu.Unlock()
	if !show || state == updNone {
		return
	}
	if state == updInstalling && ready != "" {
		// hand it to the platform once, outside the lock
		u.mu.Lock()
		u.state, u.ready = updDone, ""
		u.mu.Unlock()
		up := a.P.(Updater)
		if err := up.InstallUpdate(ready); err != nil {
			u.mu.Lock()
			u.state, u.err = updError, "couldn't install: "+err.Error()
			u.mu.Unlock()
		} else if _, android := a.P.(AsyncImagePicker); !android {
			a.Close()
			a.P.Quit() // the new version is starting
		}
		return
	}
	r := ovRect(380, 172)
	fillRect(a.fb, a.fb.Rect, a.A.C("overlay"))
	a.panel(r)
	cx := r.Min.X + r.Dx()/2
	a.textBig(cx, r.Min.Y+12, "UPDATE AVAILABLE", a.A.C("gold"), 2, alignCenter)
	y := r.Min.Y + 44
	line := func(s string, c string) {
		for _, l := range a.wrap(s, r.Dx()-32) {
			a.text(cx, y, l, a.A.C(c), alignCenter)
			y += 11
		}
	}
	bw := 110
	left := image.Rect(cx-bw-6, r.Max.Y-30, cx-6, r.Max.Y-10)
	right := image.Rect(cx+6, r.Max.Y-30, cx+bw+6, r.Max.Y-10)
	mid := image.Rect(cx-bw/2, r.Max.Y-30, cx+bw/2, r.Max.Y-10)
	switch state {
	case updOffer:
		line(fmt.Sprintf("This server is running version %s. You have %s.", ver, Version), "text")
		line("Update now to get the latest dealer, fixes and features.", "text_dim")
		if a.button("upd_go", left, "UPDATE NOW", btn{enabled: true, style: styleGold, key: KeyEnter}) {
			a.startUpdate()
		}
		if a.button("upd_later", right, "LATER", btn{enabled: true, key: KeyEscape}) {
			u.mu.Lock()
			u.show, u.state, u.dismissed = false, updNone, ver
			u.mu.Unlock()
		}
	case updDownloading:
		line(fmt.Sprintf("Downloading version %s...", ver), "text")
		bar := image.Rect(r.Min.X+30, y+6, r.Max.X-30, y+16)
		fillRect(a.fb, bar, a.A.C("bubble_txt"))
		in := bar.Inset(1)
		if size > 0 {
			fillRect(a.fb, image.Rect(in.Min.X, in.Min.Y, in.Min.X+int(float64(in.Dx())*float64(got)/float64(size)), in.Max.Y), a.A.C("gold"))
		} else { // unknown size: a moving block
			off := int(a.frame*3) % max(1, in.Dx()-20)
			fillRect(a.fb, image.Rect(in.Min.X+off, in.Min.Y, in.Min.X+off+20, in.Max.Y), a.A.C("gold"))
		}
		y += 24
		mb := func(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }
		if size > 0 {
			line(mb(got)+" of "+mb(size), "text_dim")
		} else {
			line(mb(got), "text_dim")
		}
		if a.button("upd_cancel", mid, "CANCEL", btn{enabled: true, key: KeyEscape}) {
			u.mu.Lock()
			if u.cancel != nil {
				close(u.cancel)
				u.cancel = nil
			}
			u.state = updOffer
			u.mu.Unlock()
		}
	case updDone:
		if _, android := a.P.(AsyncImagePicker); android {
			line("Follow the installer to finish updating.", "text")
			line("If nothing opened, allow this app to install updates when Android asks, then tap TRY AGAIN.", "text_dim")
			if a.button("upd_retry", left, "TRY AGAIN", btn{enabled: true, style: styleGold}) {
				a.startUpdate()
			}
			if a.button("upd_close", right, "CLOSE", btn{enabled: true, key: KeyEscape}) {
				u.mu.Lock()
				u.show, u.state, u.dismissed = false, updNone, ver
				u.mu.Unlock()
			}
		} else {
			line("Restarting with the new version...", "text")
		}
	case updError:
		line("The update didn't work:", "lose")
		line(msg, "text")
		if a.button("upd_retry", left, "TRY AGAIN", btn{enabled: true, style: styleGold, key: KeyEnter}) {
			a.startUpdate()
		}
		if a.button("upd_close", right, "LATER", btn{enabled: true, key: KeyEscape}) {
			u.mu.Lock()
			u.show, u.state, u.dismissed = false, updNone, ver
			u.mu.Unlock()
		}
	}
}

// updateOpen reports whether the update window is up (it blocks the rest).
func (a *App) updateOpen() bool {
	a.upd.mu.Lock()
	defer a.upd.mu.Unlock()
	return a.upd.show && a.upd.state != updNone
}
