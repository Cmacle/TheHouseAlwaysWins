// Package client is the platform-independent game client: it renders every
// frame into a 640x360 RGBA buffer and handles input. A platform layer (see
// internal/win32) owns the window, scaling, sound output and file dialogs.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/imgutil"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Platform is implemented by the OS layer.
type Platform interface {
	PlaySound(wav []byte)
	OpenImageFile() (string, bool)
	ClipboardText() string
	ToggleFullscreen()
	Quit()
}

const (
	scrLogin = iota
	scrTable
)

const (
	modeLogin = iota
	modeRegister
)

const (
	ovNone = iota
	ovLeaderboard
	ovProfile
	ovHelp
	ovWardrobe
)

type dialRes struct {
	n   *NetClient
	err error
}

type avatar struct {
	version   string
	requested string
	img       *image.RGBA
	scaled    map[int]*image.RGBA
}

type toast struct {
	text  string
	col   color.RGBA
	until time.Time
	scale int
}

// App is the game client.
type App struct {
	P   Platform
	Cfg *Config
	A   *Assets

	fb    *image.RGBA
	in    Input
	frame int
	now   time.Time

	// immediate-mode UI state
	activeID   string
	focusID    string
	focusOrder []string
	blocked    bool

	screen     int
	net        *NetClient
	connecting bool
	dialCh     chan dialRes
	afterDial  func()
	serverInfo proto.ServerInfo
	kicked     bool

	// login screen
	loginMode    int
	fServer      string
	fUser        string
	fPass        string
	fPass2       string
	regAvatar    []byte
	regAvatarImg *image.RGBA
	status       string
	statusErr    bool
	tokenTry     bool
	reconnectAt  time.Time

	// session
	username    string
	profile     proto.Profile
	st          *proto.TableState
	clockOffset int64
	avatars     map[string]*avatar
	toasts      []toast
	cardSeen    map[string]time.Time
	cardHidden  map[string]bool
	flipAt      map[string]time.Time
	betDraft    int64
	lastBet     int64
	overlay     int
	leaderboard []proto.Profile
	lbScroll    int
	lbAt        time.Time
	lastPing    time.Time
	resultAt    time.Time
	resultDelta int64
	resultKind  string
	lbSort      string
	smallHats   map[string]*image.RGBA
	wardTab     string
	wardSel     string
}

// New creates the client app.
func New(p Platform, cfg *Config, assets *Assets) *App {
	a := &App{
		P: p, Cfg: cfg, A: assets,
		fb:         image.NewRGBA(image.Rect(0, 0, ScreenW, ScreenH)),
		dialCh:     make(chan dialRes, 1),
		avatars:    map[string]*avatar{},
		cardSeen:   map[string]time.Time{},
		cardHidden: map[string]bool{},
		flipAt:     map[string]time.Time{},
		fServer:    cfg.Server,
		fUser:      cfg.Username,
	}
	if cfg.RememberMe && cfg.Token != "" && cfg.Username != "" && cfg.Server != "" {
		a.reconnectAt = time.Now()
	}
	if a.fUser == "" {
		a.focusID = "user"
	} else {
		a.focusID = "pass"
	}
	return a
}

// Close shuts the connection.
func (a *App) Close() {
	if a.net != nil {
		a.net.Close()
	}
}

func (a *App) sound(name string) {
	if !a.Cfg.Sound {
		return
	}
	if b := a.A.Sounds[name]; b != nil {
		a.P.PlaySound(b)
	}
}

func (a *App) toast(text string, c color.RGBA, secs float64, scale int) {
	if text == "" {
		return
	}
	a.toasts = append(a.toasts, toast{text: text, col: c, until: a.now.Add(time.Duration(secs * float64(time.Second))), scale: scale})
	if len(a.toasts) > 4 {
		a.toasts = a.toasts[len(a.toasts)-4:]
	}
}

func (a *App) setStatus(s string, isErr bool) {
	a.status, a.statusErr = s, isErr
}

// Frame advances one frame with the given input and returns the rendered image.
func (a *App) Frame(in Input) *image.RGBA {
	a.in = in
	a.now = time.Now()
	a.frame++
	a.pollNet()
	if in.key(KeyF11) {
		a.P.ToggleFullscreen()
	}
	a.focusOrder = a.focusOrder[:0]
	a.blocked = false
	switch a.screen {
	case scrLogin:
		a.drawLogin()
	case scrTable:
		a.drawTable()
	}
	a.blocked = false
	a.handleTab()
	a.drawToasts()
	if in.Released || !in.Down {
		if !in.Down {
			a.activeID = ""
		}
	}
	return a.fb
}

// ---------------------------------------------------------------- networking

func normAddr(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ":") {
		s += ":7777"
	}
	return s
}

func (a *App) connect(addr string, then func()) {
	addr = normAddr(addr)
	if a.net != nil {
		select {
		case <-a.net.Dead():
			a.net = nil
		default:
			if normAddr(a.net.Addr) == addr {
				then()
				return
			}
			a.net.Close()
			a.net = nil
		}
	}
	if a.connecting {
		return
	}
	a.connecting = true
	a.kicked = false
	a.afterDial = then
	a.setStatus("Connecting to "+addr+"...", false)
	go func() {
		n, err := Dial(addr)
		a.dialCh <- dialRes{n, err}
	}()
}

func (a *App) send(t string, v any) {
	if a.net != nil {
		a.net.Send(t, v)
	}
}

func (a *App) pollNet() {
	select {
	case r := <-a.dialCh:
		a.connecting = false
		if r.err != nil {
			a.setStatus("Could not connect: "+shortErr(r.err), true)
			a.afterDial = nil
			a.tokenTry = false
		} else {
			a.net = r.n
			a.setStatus("Connected.", false)
			if a.afterDial != nil {
				f := a.afterDial
				a.afterDial = nil
				f()
			}
		}
	default:
	}
	if a.net == nil {
		if a.screen == scrLogin && !a.reconnectAt.IsZero() && a.now.After(a.reconnectAt) && !a.connecting {
			a.reconnectAt = time.Time{}
			a.tokenLogin()
		}
		return
	}
	for _, m := range a.net.Poll() {
		if a.net == nil {
			break
		}
		a.handleMsg(m)
	}
	if a.net == nil {
		return
	}
	select {
	case <-a.net.Dead():
		a.onDisconnect()
		return
	default:
	}
	if a.now.Sub(a.lastPing) > 20*time.Second {
		a.lastPing = a.now
		a.send(proto.CPing, nil)
	}
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

func (a *App) onDisconnect() {
	a.net = nil
	wasPlaying := a.screen == scrTable
	a.screen = scrLogin
	a.overlay = ovNone
	if !a.kicked {
		if wasPlaying {
			a.setStatus("Disconnected from the server.", true)
		}
		if wasPlaying && a.Cfg.RememberMe && a.Cfg.Token != "" {
			a.reconnectAt = a.now.Add(3 * time.Second)
			a.setStatus("Disconnected - reconnecting...", true)
		}
	}
}

func (a *App) tokenLogin() {
	a.tokenTry = true
	user, tok := a.Cfg.Username, a.Cfg.Token
	a.connect(a.Cfg.Server, func() {
		a.send(proto.CLogin, proto.Login{Username: user, Token: tok})
		a.setStatus("Logging in...", false)
	})
}

func decode(m proto.Msg, v any) bool { return json.Unmarshal(m.Data, v) == nil }

func (a *App) handleMsg(m proto.Msg) {
	switch m.Type {
	case proto.SServerInfo:
		decode(m, &a.serverInfo)
	case proto.SAuthOK:
		var ok proto.AuthOK
		if !decode(m, &ok) {
			return
		}
		a.username = ok.Username
		a.profile = ok.Profile
		a.Cfg.Username = ok.Username
		a.Cfg.Server = normAddr(a.net.Addr)
		if a.Cfg.RememberMe {
			a.Cfg.Token = ok.Token
		} else {
			a.Cfg.Token = ""
		}
		a.Cfg.Save()
		a.screen = scrTable
		a.st = nil
		a.tokenTry = false
		a.fPass, a.fPass2 = "", ""
		a.focusID = ""
		a.setStatus("", false)
		a.cardSeen = map[string]time.Time{}
		a.toasts = nil
		if a.serverInfo.MOTD != "" {
			a.toast(a.serverInfo.MOTD, a.A.C("gold"), 5, 1)
		}
	case proto.SAuthFail:
		var f proto.AuthFail
		decode(m, &f)
		if a.tokenTry {
			a.Cfg.Token = ""
			a.Cfg.Save()
			a.tokenTry = false
		}
		a.setStatus(f.Reason, true)
		a.sound("error")
	case proto.SState:
		var st proto.TableState
		if decode(m, &st) {
			a.onState(&st)
		}
	case proto.SEvent:
		var e proto.Event
		if decode(m, &e) {
			a.onEvent(e)
		}
	case proto.SError:
		var e proto.ErrorMsg
		decode(m, &e)
		if a.screen == scrLogin {
			a.setStatus(e.Message, true)
		} else {
			a.toast(e.Message, a.A.C("error"), 3, 1)
		}
		a.sound("error")
	case proto.SLeaderboard:
		var lb proto.Leaderboard
		if decode(m, &lb) && (lb.Sort == a.lbSort || a.lbSort == "") {
			a.leaderboard = lb.Entries
		}
	case proto.SAvatar:
		var av proto.Avatar
		if decode(m, &av) {
			a.storeAvatar(av)
		}
	case proto.SProfile:
		var p proto.Profile
		if decode(m, &p) && strings.EqualFold(p.Username, a.username) {
			a.profile = p
		}
	case proto.SKicked:
		var e proto.ErrorMsg
		decode(m, &e)
		a.kicked = true
		a.screen = scrLogin
		a.setStatus(e.Message, true)
		a.net.Close()
		a.net = nil
	}
}

// ---------------------------------------------------------------- avatars

func (a *App) storeAvatar(av proto.Avatar) {
	key := strings.ToLower(av.Username)
	e := a.avatars[key]
	if e == nil {
		e = &avatar{}
		a.avatars[key] = e
	}
	e.version = av.Version
	e.requested = av.Version
	e.scaled = map[int]*image.RGBA{}
	e.img = nil
	if len(av.Data) > 0 {
		if img, err := png.Decode(bytes.NewReader(av.Data)); err == nil {
			e.img = toRGBA(img)
		}
	}
}

func (a *App) avatarImg(user, version string, size int) *image.RGBA {
	key := strings.ToLower(user)
	e := a.avatars[key]
	if version != "" && (e == nil || e.version != version) {
		if e == nil {
			e = &avatar{scaled: map[int]*image.RGBA{}}
			a.avatars[key] = e
		}
		if e.requested != version && a.net != nil {
			e.requested = version
			a.send(proto.CAvatarGet, proto.AvatarGet{Username: user})
		}
	}
	src := a.A.DefaultAvatar
	cacheKey := -size
	if e != nil && e.img != nil && version != "" {
		src = e.img
		cacheKey = size
	}
	if e == nil {
		e = &avatar{scaled: map[int]*image.RGBA{}}
		a.avatars[key] = e
	}
	if img, ok := e.scaled[cacheKey]; ok {
		return img
	}
	img := src
	if src.Rect.Dx() != size || src.Rect.Dy() != size {
		img = scaleImage(src, size, size)
	}
	e.scaled[cacheKey] = img
	return img
}

// pickAvatar opens a file dialog and returns a normalised 32x32 PNG.
func (a *App) pickAvatar() ([]byte, *image.RGBA, error) {
	path, ok := a.P.OpenImageFile()
	a.activeID = ""
	if !ok {
		return nil, nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, nil, fmt.Errorf("can't read that picture (use PNG, JPEG or GIF)")
	}
	small := imgutil.Downsample(img, 32)
	var buf bytes.Buffer
	if err := png.Encode(&buf, small); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), toRGBA(small), nil
}

// ---------------------------------------------------------------- toasts

func (a *App) drawToasts() {
	live := a.toasts[:0]
	for _, t := range a.toasts {
		if a.now.Before(t.until) {
			live = append(live, t)
		}
	}
	a.toasts = live
	y := 150
	for i := len(a.toasts) - 1; i >= 0; i-- {
		t := a.toasts[i]
		w := a.A.Font.Width(t.text)*t.scale + 12
		h := a.A.Font.Height()*t.scale + 6
		r := image.Rect(ScreenW/2-w/2, y-h, ScreenW/2+w/2, y)
		fillRect(a.fb, r, color.RGBA{0, 0, 0, 170})
		strokeRect(a.fb, r, color.RGBA{0, 0, 0, 255})
		a.textBig(ScreenW/2, r.Min.Y+3, t.text, t.col, t.scale, alignCenter)
		y -= h + 3
	}
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func signed(n int64) string {
	if n > 0 {
		return "+" + commas(n)
	}
	return commas(n)
}
