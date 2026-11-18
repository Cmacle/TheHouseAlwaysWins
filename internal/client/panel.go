// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"image"
	"strings"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The server window: the server's settings, its log with a command box, and
// an update button. It's drawn with the game's own UI so it looks the part.

// PanelStatus is what the server window shows in its header.
type PanelStatus struct {
	Name     string
	Version  string
	Address  string
	Players  []string
	Latest   string // newest release on GitHub ("" = unknown)
	Newer    bool   // Latest is newer than Version
	Update   string // what the updater is doing
	Updating bool
	Stopped  bool // the server has shut down: close the window
}

// PanelBackend is the running server, as the window sees it.
type PanelBackend interface {
	Settings() []proto.SettingInfo
	SetSetting(key, value string) error
	Exec(line string) // output goes to the log
	Log(from int) (lines []string, next int)
	Status() PanelStatus
	Update() // check GitHub and install a newer server (in the background)
	Quit()
}

// panelAdmin adapts a PanelBackend to the settings editor and console.
type panelAdmin struct {
	a *App
	b PanelBackend
}

func (p panelAdmin) RequestSettings() { p.a.onSettings(proto.SettingsList{Settings: p.b.Settings()}) }
func (p panelAdmin) SetSetting(k, v string) {
	res := proto.SettingsList{Changed: k}
	if err := p.b.SetSetting(k, v); err != nil {
		res = proto.SettingsList{Error: err.Error()}
	}
	res.Settings = p.b.Settings()
	p.a.onSettings(res)
}
func (p panelAdmin) Exec(line string) { go p.b.Exec(strings.TrimPrefix(line, "/")) }

// NewPanel makes the server window's app.
func NewPanel(p Platform, assets *Assets, b PanelBackend) *App {
	a := New(p, &Config{Sound: true, Volume: 60, EffectsVolume: 100}, assets)
	a.screen = scrPanel
	a.pnl = b
	a.adminB = panelAdmin{a, b}
	a.adminTab = 1 // the log first
	a.focusID = ""
	a.reconnectAt = a.reconnectAt.AddDate(-1, 0, 0)
	a.adminB.RequestSettings()
	return a
}

func (a *App) drawPanel() {
	b := a.pnl
	st := b.Status()
	if st.Stopped {
		a.P.Quit()
	}
	if lines, next := b.Log(a.panelLogNext); len(lines) > 0 || next != a.panelLogNext {
		a.console = append(a.console, lines...)
		if len(a.console) > 2000 {
			a.console = a.console[len(a.console)-2000:]
		}
		a.panelLogNext = next
	}
	a.computeLayout()
	a.drawTableBG()
	r := image.Rect(4, 4, ScreenW-4, ScreenH-4)
	a.panel(r)
	// header
	a.textBig(r.Min.X+10, r.Min.Y+8, "THE HOUSE ALWAYS WINS", a.A.C("gold"), 2, alignLeft)
	sub := "SERVER v" + st.Version + "  -  " + Copyright
	if st.Name != "" {
		sub = st.Name + "  -  " + sub
	}
	a.text(r.Min.X+10, r.Min.Y+26, fitText(a.A.Font, sub, r.Dx()/2), a.A.C("text"), alignLeft)
	who := "Nobody's playing"
	if n := len(st.Players); n > 0 {
		who = commas(int64(n)) + " online: " + strings.Join(st.Players, ", ")
	}
	line := st.Address
	if line != "" {
		line += "  -  "
	}
	a.text(r.Min.X+10, r.Min.Y+36, fitText(a.A.Font, line+who, r.Dx()-200), a.A.C("text_dim"), alignLeft)

	// update
	ux := r.Max.X - 10
	label, style := "CHECK FOR UPDATE", styleRed
	if st.Newer {
		label, style = "UPDATE TO v"+st.Latest, styleGold
	}
	w := a.A.Font.Width(label) + 18
	if a.button("pnl_update", image.Rect(ux-w, r.Min.Y+8, ux, r.Min.Y+26), label, btn{enabled: !st.Updating, style: style}) {
		go b.Update()
		a.adminTab = 1
	}
	info := st.Update
	if info == "" {
		switch {
		case st.Newer:
			info = "v" + st.Latest + " is out on GitHub."
		case st.Latest != "":
			info = "Up to date."
		}
	}
	a.text(ux, r.Min.Y+30, fitText(a.A.Font, info, 220), a.A.C("text_dim"), alignRight)

	a.drawAdminBody(image.Rect(r.Min.X+8, r.Min.Y+48, r.Max.X-8, r.Max.Y-6))
}
