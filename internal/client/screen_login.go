package client

import (
	"image"
	"regexp"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

var userRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

func userChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func hostChar(r rune) bool { return r != ' ' }

func digitChar(r rune) bool { return r >= '0' && r <= '9' }

// serverAddr validates the server and port boxes and returns host:port.
func (a *App) serverAddr() (string, bool) {
	host, _ := splitServer(a.fServer)
	if host == "" {
		a.setStatus("Enter the server's IP address or domain.", true)
		return "", false
	}
	addr := joinServer(a.fServer, a.fPort)
	if _, p := splitServer(addr); !validPort(p) {
		a.setStatus("The port must be a number from 1 to 65535.", true)
		return "", false
	}
	// tidy the boxes so they show what we'll actually use
	a.fServer, a.fPort = splitServer(addr)
	return addr, true
}

// Version is shown on the login screen.
var Version = "1.0.0"

func (a *App) drawLogin() {
	blit(a.fb, a.A.Table, 0, 0)
	fillRect(a.fb, a.fb.Rect, a.A.C("overlay"))
	lw := a.A.Logo.Rect.Dx()
	blit(a.fb, a.A.Logo, ScreenW/2-lw/2, 10)

	busy := a.connecting || a.tokenTry
	var pr image.Rectangle
	if a.loginMode == modeLogin {
		pr = image.Rect(180, 108, 460, 306)
	} else {
		pr = image.Rect(170, 108, 470, 334)
	}
	a.panel(pr)
	x0, x1 := pr.Min.X+16, pr.Max.X-16
	label := func(y int, s string) { a.text(x0, y, s, a.A.C("gold"), alignLeft) }

	portX := x1 - 56
	label(pr.Min.Y+12, "SERVER (IP OR DOMAIN)")
	a.text(portX, pr.Min.Y+12, "PORT", a.A.C("gold"), alignLeft)
	enter := a.textField("server", image.Rect(x0, pr.Min.Y+22, portX-6, pr.Min.Y+40), &a.fServer, false, 128, "play.example.com", hostChar)
	enter = a.textField("port", image.Rect(portX, pr.Min.Y+22, x1, pr.Min.Y+40), &a.fPort, false, 5, DefaultPort, digitChar) || enter

	if a.loginMode == modeLogin {
		label(pr.Min.Y+46, "USERNAME")
		enter = a.textField("user", image.Rect(x0, pr.Min.Y+56, x1, pr.Min.Y+74), &a.fUser, false, 16, "", userChar) || enter
		label(pr.Min.Y+80, "PASSWORD")
		enter = a.textField("pass", image.Rect(x0, pr.Min.Y+90, x1, pr.Min.Y+108), &a.fPass, true, 128, "", nil) || enter
		a.checkbox("remember", x0, pr.Min.Y+116, a.rememberLabel(), &a.Cfg.RememberMe)

		by := pr.Min.Y + 134
		mid := (x0 + x1) / 2
		if a.button("login", image.Rect(x0, by, mid-4, by+20), "LOG IN", btn{enabled: !busy, style: styleGold}) || (enter && !busy) {
			a.doLogin()
		}
		if a.button("toreg", image.Rect(mid+4, by, x1, by+20), "NEW ACCOUNT", btn{enabled: !busy}) {
			a.loginMode = modeRegister
			a.focusID = "ruser"
			a.setStatus("", false)
		}
		a.drawStatus(x0, x1, pr.Min.Y+164)
	} else {
		fx1 := x1 - 80
		label(pr.Min.Y+46, "USERNAME")
		enter = a.textField("ruser", image.Rect(x0, pr.Min.Y+56, fx1, pr.Min.Y+74), &a.fUser, false, 16, "3-16 letters/numbers", userChar) || enter
		half := (x0 + fx1) / 2
		label(pr.Min.Y+80, "PASSWORD")
		a.text(half+3, pr.Min.Y+80, "CONFIRM", a.A.C("gold"), alignLeft)
		enter = a.textField("rpass", image.Rect(x0, pr.Min.Y+90, half-3, pr.Min.Y+108), &a.fPass, true, 128, "", nil) || enter
		enter = a.textField("rpass2", image.Rect(half+3, pr.Min.Y+90, fx1, pr.Min.Y+108), &a.fPass2, true, 128, "", nil) || enter
		label(pr.Min.Y+114, "SERVER PASSWORD")
		hint, hintC := "if it has one", a.A.C("text_dim")
		if a.serverInfo.NeedsPassword && a.net != nil {
			hint, hintC = "required", a.A.C("gold")
		}
		a.text(x0+a.A.Font.Width("SERVER PASSWORD")+6, pr.Min.Y+114, hint, hintC, alignLeft)
		enter = a.textField("rspass", image.Rect(x0, pr.Min.Y+124, fx1, pr.Min.Y+142), &a.fServerPass, true, 128, "", nil) || enter

		// avatar picker
		ax := fx1 + 10
		a.text(ax+32, pr.Min.Y+46, "PICTURE", a.A.C("gold"), alignCenter)
		box := image.Rect(ax, pr.Min.Y+56, ax+64, pr.Min.Y+120)
		fillRect(a.fb, box.Inset(-1), a.A.C("shadow"))
		img := a.regAvatarImg
		if img == nil {
			img = a.A.DefaultAvatar
		}
		blitScaled(a.fb, img, box)
		if a.button("pick", image.Rect(ax, pr.Min.Y+124, ax+64, pr.Min.Y+142), "BROWSE", btn{enabled: true}) {
			a.choosePicture("register")
		}
		a.checkbox("remember2", x0, pr.Min.Y+150, a.rememberLabel(), &a.Cfg.RememberMe)
		by := pr.Min.Y + 166
		mid := (x0 + x1) / 2
		if a.button("register", image.Rect(x0, by, mid-4, by+20), "CREATE", btn{enabled: !busy, style: styleGold}) || (enter && !busy) {
			a.doRegister()
		}
		if a.button("back", image.Rect(mid+4, by, x1, by+20), "BACK", btn{enabled: true, key: KeyEscape}) {
			a.loginMode = modeLogin
			a.focusID = "user"
			a.setStatus("", false)
		}
		a.drawStatus(x0, x1, pr.Min.Y+196)
	}
	foot := "v" + Version + "  -  F11 fullscreen"
	if a.isTouch() {
		foot = "v" + Version
	}
	a.text(4, ScreenH-10, foot, a.A.C("text_dim"), alignLeft)
	if a.serverInfo.Name != "" && a.net != nil {
		a.text(ScreenW-4, ScreenH-10, a.serverInfo.Name, a.A.C("text_dim"), alignRight)
	}
}

func (a *App) drawStatus(x0, x1, y int) {
	if a.status == "" {
		return
	}
	c := a.A.C("text")
	if a.statusErr {
		c = a.A.C("error")
	}
	for i, l := range a.wrap(a.status, x1-x0) {
		if i > 2 {
			break
		}
		a.text((x0+x1)/2, y+i*10, l, c, alignCenter)
	}
}

func (a *App) doLogin() {
	addr, ok := a.serverAddr()
	if !ok {
		return
	}
	if a.fUser == "" || a.fPass == "" {
		a.setStatus("Enter your username and password.", true)
		return
	}
	a.Cfg.Server = addr
	a.Cfg.Save()
	user, pass := a.fUser, a.fPass
	a.tokenTry = false
	a.connect(addr, func() {
		a.send(proto.CLogin, proto.Login{Username: user, Password: pass})
		a.setStatus("Logging in...", false)
	})
}

func (a *App) doRegister() {
	addr, ok := a.serverAddr()
	if !ok {
		return
	}
	switch {
	case !userRe.MatchString(a.fUser):
		a.setStatus("Usernames are 3-16 letters, numbers or _.", true)
		return
	case len(a.fPass) < 4:
		a.setStatus("Passwords need at least 4 characters.", true)
		return
	case a.fPass != a.fPass2:
		a.setStatus("The passwords don't match.", true)
		return
	}
	a.Cfg.Server = addr
	a.Cfg.Save()
	reg := proto.Register{Username: a.fUser, Password: a.fPass, Avatar: a.regAvatar, ServerPassword: a.fServerPass}
	a.tokenTry = false
	a.connect(addr, func() {
		a.send(proto.CRegister, reg)
		a.setStatus("Creating account...", false)
	})
}

func (a *App) rememberLabel() string {
	if a.isTouch() {
		return "Remember me on this device"
	}
	return "Remember me on this computer"
}
