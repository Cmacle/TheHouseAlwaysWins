// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The ADMIN window (for admins in the game) and the server window share
// the settings editor and the console below. Where the settings come from
// and where commands go is an AdminBackend.

// AdminBackend is where the settings editor and console send things.
type AdminBackend interface {
	RequestSettings()
	SetSetting(key, value string)
	Exec(line string)
}

// netAdmin talks to the server over the connection (admins in the game).
type netAdmin struct{ a *App }

func (n netAdmin) RequestSettings() { n.a.send(proto.CSettings, nil) }
func (n netAdmin) SetSetting(k, v string) {
	n.a.send(proto.CSetSetting, proto.SetSetting{Key: k, Value: v})
}
func (n netAdmin) Exec(line string) {
	if !strings.HasPrefix(line, "/") {
		line = "/" + line
	}
	n.a.send(proto.CChat, proto.Chat{Text: line})
}

func (a *App) adminBackend() AdminBackend {
	if a.adminB != nil {
		return a.adminB
	}
	return netAdmin{a}
}

// OnSettings gives the editor a fresh list of settings.
func (a *App) onSettings(s proto.SettingsList) {
	a.adminSet = s.Settings
	if a.adminEdit == nil {
		a.adminEdit = map[string]string{}
	}
	switch {
	case s.Error != "":
		a.adminMsg, a.adminErr = s.Error, true
		a.sound("error")
	case s.Changed != "":
		delete(a.adminEdit, s.Changed)
		a.adminMsg, a.adminErr = "Saved "+s.Changed+". Table settings apply from the next hand.", false
	}
}

func (a *App) openAdmin() {
	a.overlay = ovAdmin
	a.adminMsg = ""
	a.adminBackend().RequestSettings()
}

func (a *App) drawAdmin() {
	r := ovRect(560, max(320, ScreenH-40))
	if a.overlayFrame(r, "ADMIN") {
		return
	}
	a.drawAdminBody(image.Rect(r.Min.X+10, r.Min.Y+30, r.Max.X-10, r.Max.Y-8))
}

// drawAdminBody draws the tabs and the chosen page into r.
func (a *App) drawAdminBody(r image.Rectangle) {
	tabs := []string{"SETTINGS", "CONSOLE"}
	if a.pnl != nil {
		tabs[1] = "LOG + COMMANDS"
	}
	tx := r.Min.X
	for i, t := range tabs {
		style := styleRed
		if a.adminTab == i {
			style = styleGold
		}
		w := a.A.Font.Width(t) + 20
		if a.button("adm_tab"+t, image.Rect(tx, r.Min.Y, tx+w, r.Min.Y+16), t, btn{enabled: true, style: style}) {
			a.adminTab = i
			if i == 0 {
				a.adminBackend().RequestSettings()
			}
		}
		tx += w + 4
	}
	body := image.Rect(r.Min.X, r.Min.Y+22, r.Max.X, r.Max.Y)
	if a.adminTab == 0 {
		a.drawAdminSettings(body)
	} else {
		a.drawAdminConsole(body)
	}
}

func (a *App) drawAdminSettings(r image.Rectangle) {
	if a.adminEdit == nil {
		a.adminEdit = map[string]string{}
	}
	// filter box
	a.text(r.Min.X, r.Min.Y+5, "FIND", a.A.C("text_dim"), alignLeft)
	a.textField("adm_filter", image.Rect(r.Min.X+30, r.Min.Y, r.Min.X+200, r.Min.Y+16), &a.adminFilter, false, 40, "type to filter", nil)
	if a.adminMsg != "" {
		c := a.A.C("win")
		if a.adminErr {
			c = a.A.C("lose")
		}
		a.text(r.Max.X, r.Min.Y+5, fitText(a.A.Font, a.adminMsg, r.Dx()-220), c, alignRight)
	}
	if len(a.adminSet) == 0 {
		a.text(r.Min.X+r.Dx()/2, r.Min.Y+60, "Loading settings...", a.A.C("text_dim"), alignCenter)
		return
	}
	f := strings.ToLower(strings.TrimSpace(a.adminFilter))
	var rows []proto.SettingInfo
	for _, s := range a.adminSet {
		if f == "" || strings.Contains(s.Key, f) || strings.Contains(strings.ToLower(s.Label), f) {
			rows = append(rows, s)
		}
	}
	top := r.Min.Y + 22
	rowH := 19
	visible := max(1, (r.Max.Y-top)/rowH)
	maxScroll := max(0, len(rows)-visible)
	list := image.Rect(r.Min.X, top, r.Max.X, r.Max.Y)
	if a.hovered(list) && a.in.Wheel != 0 {
		a.adminScroll -= a.in.Wheel * 2
	}
	if a.focusID == "" || a.focusID == "adm_filter" {
		if a.in.key(KeyPageDown) {
			a.adminScroll += visible
		}
		if a.in.key(KeyPageUp) {
			a.adminScroll -= visible
		}
	}
	a.adminScroll = min(max(a.adminScroll, 0), maxScroll)
	labelW := min(220, r.Dx()/2)
	valX := r.Min.X + labelW
	for i := 0; i < visible && a.adminScroll+i < len(rows); i++ {
		s := rows[a.adminScroll+i]
		y := top + i*rowH
		if i%2 == 1 {
			fillRect(a.fb, image.Rect(r.Min.X, y-2, r.Max.X-8, y+rowH-2), color.RGBA{0, 0, 0, 40})
		}
		label := s.Label
		if s.Group != "" {
			label = strings.ToUpper(s.Group[:1]) + s.Group[1:] + ": " + label
		}
		a.text(r.Min.X+2, y+4, fitText(a.A.Font, label, labelW-6), a.A.C("text"), alignLeft)
		id := "adm_" + s.Key
		if s.Kind == "bool" {
			on := s.Value == "true"
			lbl, style := "OFF", styleRed
			if on {
				lbl, style = "ON", styleGold
			}
			if a.button(id, image.Rect(valX, y, valX+44, y+15), lbl, btn{enabled: true, style: style}) {
				a.adminBackend().SetSetting(s.Key, strconv.FormatBool(!on))
				a.adminMsg = ""
			}
			continue
		}
		v, editing := a.adminEdit[s.Key]
		if !editing {
			v = s.Value
		}
		saveW := 40
		fr := image.Rect(valX, y, r.Max.X-saveW-14, y+15)
		var allowed func(rune) bool
		switch s.Kind {
		case "int":
			allowed = func(c rune) bool { return c >= '0' && c <= '9' || c == '-' }
		case "float":
			allowed = func(c rune) bool { return c >= '0' && c <= '9' || c == '-' || c == '.' }
		}
		enter := a.textField(id, fr, &v, false, 300, "", allowed)
		if v != s.Value {
			a.adminEdit[s.Key] = v
		} else {
			delete(a.adminEdit, s.Key)
		}
		changed := v != s.Value
		if a.button(id+"_save", image.Rect(r.Max.X-saveW-10, y, r.Max.X-10, y+15), "SAVE", btn{enabled: changed, style: styleGold}) || (enter && changed) {
			a.adminBackend().SetSetting(s.Key, v)
			a.adminMsg = ""
		}
	}
	// scrollbar
	if maxScroll > 0 {
		tr := image.Rect(r.Max.X-5, top, r.Max.X-2, r.Max.Y)
		fillRect(a.fb, tr, color.RGBA{0, 0, 0, 80})
		h := max(10, tr.Dy()*visible/len(rows))
		y := tr.Min.Y + (tr.Dy()-h)*a.adminScroll/maxScroll
		fillRect(a.fb, image.Rect(tr.Min.X, y, tr.Max.X, y+h), a.A.C("gold"))
	}
}

func (a *App) drawAdminConsole(r image.Rectangle) {
	in := image.Rect(r.Min.X, r.Max.Y-18, r.Max.X-50, r.Max.Y)
	out := image.Rect(r.Min.X, r.Min.Y, r.Max.X, in.Min.Y-4)
	fillRect(a.fb, out, color.RGBA{0, 0, 0, 150})
	lh := 9
	visible := max(1, (out.Dy()-6)/lh)
	// wrap long lines
	var lines []string
	for _, l := range a.console {
		if a.A.Font.Width(l) <= out.Dx()-10 {
			lines = append(lines, l)
			continue
		}
		lines = append(lines, a.wrap(l, out.Dx()-10)...)
	}
	maxScroll := max(0, len(lines)-visible)
	if a.hovered(out) && a.in.Wheel != 0 {
		a.adminConsoleScroll += a.in.Wheel * 3
	}
	if a.in.key(KeyPageUp) {
		a.adminConsoleScroll += visible
	}
	if a.in.key(KeyPageDown) {
		a.adminConsoleScroll -= visible
	}
	a.adminConsoleScroll = min(max(a.adminConsoleScroll, 0), maxScroll)
	start := max(0, len(lines)-visible-a.adminConsoleScroll)
	for i := 0; i < visible && start+i < len(lines); i++ {
		l := lines[start+i]
		c := a.A.C("text")
		switch {
		case strings.HasPrefix(l, "> "):
			c = a.A.C("gold")
		case strings.Contains(strings.ToLower(l), "fail") || strings.Contains(strings.ToLower(l), "error"):
			c = a.A.C("lose")
		}
		a.A.Font.Draw(a.fb, out.Min.X+5, out.Min.Y+3+i*lh, l, c)
	}
	if len(lines) == 0 {
		a.text(out.Min.X+out.Dx()/2, out.Min.Y+20, "Type a command below. 'help' lists them.", a.A.C("text_dim"), alignCenter)
	}
	if a.focusID == "" && a.adminTab == 1 {
		a.focusID = "adm_cmd"
	}
	enter := a.textField("adm_cmd", in, &a.adminCmd, false, 300, "command (help, players, set card_reveal off, update...)", nil)
	if a.button("adm_run", image.Rect(r.Max.X-46, in.Min.Y, r.Max.X, in.Max.Y), "RUN", btn{enabled: strings.TrimSpace(a.adminCmd) != "", style: styleGold}) || enter {
		if c := strings.TrimSpace(a.adminCmd); c != "" {
			a.adminBackend().Exec(c)
			a.adminHist = append(a.adminHist, c)
			a.adminCmd = ""
			a.adminHistAt = len(a.adminHist)
		}
	}
	// up/down: command history
	if a.focusID == "adm_cmd" && len(a.adminHist) > 0 {
		if a.in.key(KeyUp) && a.adminHistAt > 0 {
			a.adminHistAt--
			a.adminCmd = a.adminHist[a.adminHistAt]
		}
		if a.in.key(KeyDown) {
			if a.adminHistAt < len(a.adminHist)-1 {
				a.adminHistAt++
				a.adminCmd = a.adminHist[a.adminHistAt]
			} else {
				a.adminHistAt, a.adminCmd = len(a.adminHist), ""
			}
		}
	}
}

// ------------------------------------------------------------------ gifts

func (a *App) openGift(user string) {
	a.giftTo = user
	a.giftInfo = proto.GiftInfo{}
	a.giftAmt = ""
	a.overlay = ovGift
	a.send(proto.CGiftInfo, proto.GiftInfo{To: user})
}

func (a *App) drawGift() {
	r := ovRect(300, 170)
	if a.overlayFrame(r, "SEND CHIPS") {
		return
	}
	g := a.giftInfo
	x0, x1 := r.Min.X+16, r.Max.X-16
	y := r.Min.Y + 36
	a.text(x0, y, "To:", a.A.C("text_dim"), alignLeft)
	a.text(x0+24, y, a.giftTo, a.A.C("gold"), alignLeft)
	if g.To != "" {
		a.text(x1, y, commas(g.Balance)+" chips", a.A.C("text"), alignRight)
	}
	y += 14
	if g.To == "" {
		a.text(r.Min.X+r.Dx()/2, y+20, "Checking...", a.A.C("text_dim"), alignCenter)
		return
	}
	for _, l := range a.wrap(g.Reason, x1-x0) {
		c := a.A.C("text_dim")
		if g.Max == 0 {
			c = a.A.C("lose")
		}
		a.text(x0, y, l, c, alignLeft)
		y += 9
	}
	if g.Max == 0 {
		if a.button("gift_ok", image.Rect(r.Min.X+r.Dx()/2-40, r.Max.Y-28, r.Min.X+r.Dx()/2+40, r.Max.Y-10), "OK", btn{enabled: true, key: KeyEnter}) {
			a.overlay = ovNone
		}
		return
	}
	y = max(y+4, r.Min.Y+80)
	fr := image.Rect(x0, y, x0+110, y+18)
	if a.focusID == "" {
		a.focusID = "gift_amt"
	}
	enter := a.textField("gift_amt", fr, &a.giftAmt, false, 12, "amount", func(c rune) bool { return c >= '0' && c <= '9' })
	bx := fr.Max.X + 6
	for _, q := range []struct {
		l string
		n int64
	}{{"1/4", g.Max / 4}, {"1/2", g.Max / 2}, {"MAX", g.Max}} {
		w := a.A.Font.Width(q.l) + 12
		if a.button("gift_"+q.l, image.Rect(bx, y+1, bx+w, y+17), q.l, btn{enabled: q.n > 0}) {
			a.giftAmt = fmt.Sprint(q.n)
		}
		bx += w + 3
	}
	n, _ := strconv.ParseInt(a.giftAmt, 10, 64)
	ok := n > 0 && n <= g.Max
	if n > g.Max {
		a.text(x0, y+22, "That's more than you can send.", a.A.C("lose"), alignLeft)
	}
	if a.button("gift_send", image.Rect(r.Min.X+r.Dx()/2-50, r.Max.Y-28, r.Min.X+r.Dx()/2+50, r.Max.Y-10), "SEND "+commas(n), btn{enabled: ok, style: styleGold}) || (enter && ok) {
		a.send(proto.CGift, proto.Gift{To: a.giftTo, Amount: n})
		a.overlay = ovNone
		a.focusID = ""
	}
}
