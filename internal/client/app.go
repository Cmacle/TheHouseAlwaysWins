// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package client is the platform-independent game client: it renders every
// frame into a 640x360 RGBA buffer and handles input. A platform layer (see
// internal/win32) owns the window, scaling, sound output and file dialogs.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/imgutil"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Platform is implemented by the OS layer. PlaySound gets the WAV already
// scaled to the volume (desktop) and the volume itself (0..1, for platforms
// that play sounds by name, like Android).
type Platform interface {
	PlaySound(name string, wav []byte, volume float64)
	OpenImageFile() (string, bool)
	ClipboardText() string
	ToggleFullscreen()
	Quit()
}

// AsyncImagePicker is implemented by platforms whose picture picker can't
// block (Android). PickImageAsync starts the picker; the platform later calls
// App.ImagePicked with the chosen file.
type AsyncImagePicker interface {
	PickImageAsync()
}

// TextInputHost is implemented by platforms with an on-screen keyboard; it is
// told when a text field gains or loses focus.
type TextInputHost interface {
	SetTextInput(on bool)
}

// TouchPlatform is implemented by touch-screen platforms (no mouse, no
// physical keyboard), so the UI can adjust hints.
type TouchPlatform interface {
	IsTouch() bool
}

const (
	scrLogin = iota
	scrTable
	scrPanel // the server window
	scrLobby // pick a table
	scrPoker // a Hold'em table
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
	ovSettings
	ovDealerLog
	ovAdmin
	ovGift
	ovBuyIn
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
	fPort        string
	fUser        string
	fPass        string
	fPass2       string
	fServerPass  string
	regAvatar    []byte
	regAvatarImg *image.RGBA
	status       string
	statusErr    bool
	tokenTry     bool
	reconnectAt  time.Time

	// session
	username           string
	profile            proto.Profile
	st                 *proto.TableState
	clockOffset        int64
	avatars            map[string]*avatar
	toasts             []toast
	cardSeen           map[string]time.Time
	cardHidden         map[string]bool
	cardVal            map[string]blackjack.Card
	flipAt             map[string]time.Time
	betDraft           int64
	lastBet            int64
	overlay            int
	leaderboard        []proto.Profile
	lbScroll           int
	lbAt               time.Time
	lastPing           time.Time
	resultAt           time.Time
	resultDelta        int64
	resultKind         string
	lbSort             string
	smallHats          map[string]*image.RGBA
	wardTab            string
	wardSel            string
	fields             map[string]*fieldState
	caretAt            int
	house              proto.HouseStats
	pickTarget         string
	soundCache         map[string][]byte
	kbOn               bool
	sayID              int
	sayAt              time.Time
	sayWords           []string
	sayStarts          []time.Duration
	sayEnd             time.Duration
	sayBlipped         int
	L                  tableLayout
	tableBG            *image.RGBA
	kbAsk              bool // a text field was tapped: show the keyboard (again)
	upd                updater
	focusRect          image.Rectangle
	sayLog             []logEntry // what the dealer said recently, newest last
	logID              int        // id of the last line added to sayLog
	logUnread          bool
	logScroll          int
	rev                *revealFX
	chats              map[int]chatBubble // seat -> latest message
	chatLog            []proto.Chat       // recent chat incl. spectators
	chatOpen           bool               // typing a message
	chatDraft          string
	console            []string // admin command output
	giftTo             string
	giftInfo           proto.GiftInfo
	giftAmt            string
	adminSet           []proto.SettingInfo
	adminEdit          map[string]string
	adminScroll        int
	adminFilter        string
	adminMsg           string
	adminErr           bool
	adminTab           int
	adminCmd           string
	adminHist          []string
	adminHistAt        int
	adminB             AdminBackend // nil = over the connection
	consoleAt          time.Time
	adminConsoleScroll int
	pileImgs           map[int]*image.RGBA // small chips for the seat piles
	records            []proto.Record      // who holds the record cosmetics
	recordsAt          time.Time
	pnl                PanelBackend // the server window's server
	lobby              proto.Lobby
	lobbyKnown         bool   // the server has a lobby (old ones send you straight to a table)
	game               string // the game at the table you're at ("" = lobby)
	tableName          string
	ps                 *proto.PokerState
	pkRaise            int64 // the bet/raise amount being chosen
	pkRaiseFor         int   // hand number the draft belongs to
	buyTable           proto.TableInfo
	buyAmt             string
	buyRebuy           bool // the buy-in dialog adds chips to your stack instead
	lobbyScroll        int
	settingsTable      string // the admin editor is showing this table's settings
	newTable           string // admin: name for a new table
	panelLogNext       int
}

// logEntry is one thing the dealer said (a whole story is one entry).
type logEntry struct {
	text string
	at   time.Time
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
		fUser:      cfg.Username,
	}
	a.fServer, a.fPort = splitServer(cfg.Server)
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
	vol := a.volumeFor(name)
	if vol <= 0.001 {
		return
	}
	if b := a.scaledSound(name, vol); b != nil {
		a.P.PlaySound(name, b, vol)
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
	upd := a.updateOpen()
	a.blocked = upd
	switch a.screen {
	case scrLogin:
		a.drawLogin()
	case scrTable:
		a.drawTable()
	case scrPanel:
		a.drawPanel()
	case scrLobby:
		a.drawLobby()
	case scrPoker:
		a.drawPoker()
	}
	a.blocked = false
	a.handleTab()
	a.drawToasts()
	if upd {
		a.drawUpdate() // on top of everything
	}
	if ti, ok := a.P.(TextInputHost); ok {
		on := a.focusID != "" && a.screen != scrPanel
		// a tap on a text box asks again, in case the keyboard was closed
		if on != a.kbOn || (on && a.kbAsk) {
			a.kbOn = on
			ti.SetTextInput(on)
		}
	}
	a.kbAsk = false
	if a.focusID == "" {
		a.focusRect = image.Rectangle{}
	}
	if in.Released || !in.Down {
		if !in.Down {
			a.activeID = ""
		}
	}
	return a.fb
}

// ---------------------------------------------------------------- networking

// DefaultPort is used when no port is given.
const DefaultPort = "7777"

// cleanHost strips things people paste by accident: a URL scheme, a path or a
// trailing slash ("https://play.example.com/" -> "play.example.com").
func cleanHost(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// splitServer splits "host:port" (or a bare host or domain) into host and port.
func splitServer(s string) (host, port string) {
	s = cleanHost(s)
	if h, p, err := net.SplitHostPort(s); err == nil {
		return h, p
	}
	return strings.Trim(s, "[]"), DefaultPort
}

// joinServer builds the dial address from separate host and port values. A
// port pasted into the host box ("example.com:9001") wins over the port box.
func joinServer(host, port string) string {
	h, p := splitServer(host)
	if _, _, err := net.SplitHostPort(cleanHost(host)); err == nil {
		port = p
	}
	port = strings.TrimSpace(port)
	if port == "" {
		port = DefaultPort
	}
	return net.JoinHostPort(h, port)
}

func normAddr(s string) string {
	h, p := splitServer(s)
	return net.JoinHostPort(h, p)
}

func validPort(p string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(p))
	return err == nil && n >= 1 && n <= 65535
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
	wasPlaying := a.screen == scrTable || a.screen == scrLobby || a.screen == scrPoker
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
		if decode(m, &a.serverInfo) {
			a.checkForUpdate()
		}
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
		a.screen = scrLobby
		a.st, a.ps, a.game, a.lobbyKnown = nil, nil, "", false
		a.tokenTry = false
		a.fPass, a.fPass2, a.fServerPass = "", "", ""
		a.focusID = ""
		a.setStatus("", false)
		a.cardSeen = map[string]time.Time{}
		a.toasts = nil
		a.rev, a.chats, a.chatLog, a.chatOpen = nil, nil, nil, false
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
		if decode(m, &st) && (!a.lobbyKnown || a.game == proto.GameBlackjack) {
			if !a.lobbyKnown {
				a.game = proto.GameBlackjack // an older server: straight to its table
			}
			a.onState(&st)
			if a.screen == scrLobby {
				a.screen = scrTable
			}
		}
	case proto.SLobby:
		var l proto.Lobby
		if decode(m, &l) {
			a.lobby, a.lobbyKnown = l, true
		}
	case proto.SJoined:
		var j proto.Joined
		if decode(m, &j) {
			a.onJoined(j)
		}
	case proto.SLeft:
		a.backToLobby()
	case proto.SPoker:
		var ps proto.PokerState
		if decode(m, &ps) && a.game == proto.GameHoldem {
			a.onPoker(&ps)
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
		if decode(m, &lb) {
			a.house = lb.House
			if lb.Sort == a.lbSort || a.lbSort == "" || a.lbSort == "house" {
				if a.lbSort != "house" {
					a.leaderboard = lb.Entries
				}
			}
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
	case proto.SChat:
		var c proto.Chat
		if decode(m, &c) {
			a.onChat(c)
		}
	case proto.SConsole:
		var c proto.Console
		if decode(m, &c) {
			a.onConsole(c)
		}
	case proto.SRecords:
		var r proto.Records
		if decode(m, &r) {
			a.records = r.Records
		}
	case proto.SGiftInfo:
		var g proto.GiftInfo
		if decode(m, &g) && strings.EqualFold(g.To, a.giftTo) {
			a.giftInfo = g
			if a.giftAmt == "" && g.Max > 0 {
				a.giftAmt = fmt.Sprint(g.Max)
			}
		}
	case proto.SSettings:
		var s proto.SettingsList
		if decode(m, &s) {
			a.onSettings(s)
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

// choosePicture opens the picture picker. target is "register" (new
// account form) or "profile" (change your picture now).
func (a *App) choosePicture(target string) {
	a.activeID = ""
	a.pickTarget = target
	if ap, ok := a.P.(AsyncImagePicker); ok {
		ap.PickImageAsync()
		return
	}
	if path, ok := a.P.OpenImageFile(); ok {
		a.ImagePicked(path)
	}
}

// ImagePicked receives the file chosen in the picture picker.
func (a *App) ImagePicked(path string) {
	data, im, err := loadAvatarFile(path)
	switch a.pickTarget {
	case "register":
		if err != nil {
			a.setStatus(err.Error(), true)
			return
		}
		a.regAvatar, a.regAvatarImg = data, im
		a.setStatus("Looking sharp! Pictures are pixelated to 32x32.", false)
	case "profile":
		if err != nil {
			a.toast(err.Error(), a.A.C("error"), 3, 1)
			return
		}
		a.send(proto.CAvatarSet, proto.AvatarSet{Data: data})
		a.toast("Picture updated!", a.A.C("win"), 2, 1)
	}
}

// loadAvatarFile reads a picture and returns a normalised 32x32 PNG.
func loadAvatarFile(path string) ([]byte, *image.RGBA, error) {
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

func (a *App) isTouch() bool {
	t, ok := a.P.(TouchPlatform)
	return ok && t.IsTouch()
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

// FocusBottom is the bottom edge of the text box being typed in, or -1.
func (a *App) FocusBottom() int {
	if a.focusID == "" || a.focusRect.Empty() {
		return -1
	}
	return a.focusRect.Max.Y
}
