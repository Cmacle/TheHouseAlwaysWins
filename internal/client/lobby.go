// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The lobby: every open table, with JOIN buttons. Admins can open and close
// tables and change each table's settings from here.

func gameLabel(g string) string {
	switch g {
	case proto.GameHoldem:
		return "TEXAS HOLD'EM"
	case proto.GameBlackjack:
		return "BLACKJACK"
	case proto.GameBeggar:
		return "DICE"
	}
	return strings.ToUpper(g)
}

func (a *App) onJoined(j proto.Joined) {
	a.game, a.tableName = j.Game, j.Name
	a.st, a.ps, a.bs, a.roll, a.pkRev = nil, nil, nil, nil, nil
	a.cardSeen, a.cardHidden, a.cardVal, a.flipAt = map[string]time.Time{}, map[string]bool{}, nil, map[string]time.Time{}
	a.rev, a.chats, a.chatLog, a.chatOpen = nil, nil, nil, false
	a.sayLog, a.sayID, a.logID = nil, 0, 0
	if a.overlay == ovBuyIn {
		a.overlay = ovNone
	}
	switch j.Game {
	case proto.GameHoldem:
		a.screen = scrPoker
	case proto.GameBeggar:
		a.screen = scrBeggar
	default:
		a.screen = scrTable
	}
}

func (a *App) backToLobby() {
	a.game, a.st, a.ps, a.bs, a.roll, a.pkRev = "", nil, nil, nil, nil, nil
	a.chats, a.chatOpen = nil, false
	if a.screen == scrTable || a.screen == scrPoker || a.screen == scrBeggar {
		a.screen = scrLobby
	}
	if a.overlay == ovGift || a.overlay == ovDealerLog {
		a.overlay = ovNone
	}
}

// leaveToLobby asks the server to take you back to the lobby.
func (a *App) leaveToLobby() {
	a.send(proto.CLeaveTable, nil)
}

func (a *App) joinTable(t proto.TableInfo) {
	if t.ID == a.lobby.Current {
		a.screen = map[string]int{proto.GameHoldem: scrPoker, proto.GameBeggar: scrBeggar}[t.Game]
		if a.screen == 0 {
			a.screen = scrTable
		}
		return
	}
	a.send(proto.CJoinTable, proto.JoinTable{ID: t.ID})
}

// openBuyIn opens the buy-in window for a poker table.
func (a *App) openBuyIn(t proto.TableInfo, rebuy bool) {
	a.buyTable, a.buyRebuy = t, rebuy
	bal := a.offTableChips()
	amt := t.MaxBuyIn
	if rebuy && a.ps != nil {
		if me := a.pokerMe(); me != nil {
			amt = t.MaxBuyIn - me.Stack
		}
	}
	amt = min(amt, bal)
	a.buyAmt = ""
	if amt > 0 {
		a.buyAmt = fmt.Sprint(amt)
	}
	a.overlay = ovBuyIn
	a.focusID = "buy_amt"
}

// offTableChips is what you have to buy in with.
func (a *App) offTableChips() int64 {
	if a.ps != nil {
		return a.ps.Balance
	}
	return a.profile.Balance
}

func (a *App) drawBuyIn() {
	t := a.buyTable
	title := "BUY IN"
	if a.buyRebuy {
		title = "ADD CHIPS"
	}
	r := ovRect(300, 180)
	if a.overlayFrame(r, title) {
		a.focusID = ""
		return
	}
	x0, x1 := r.Min.X+16, r.Max.X-16
	y := r.Min.Y + 36
	a.text(x0, y, fitText(a.A.Font, t.Name, 150), a.A.C("gold"), alignLeft)
	a.text(x1, y, t.Stakes, a.A.C("text"), alignRight)
	y += 12
	bal := a.offTableChips()
	room := t.MaxBuyIn
	if a.buyRebuy {
		if me := a.pokerMe(); me != nil {
			room = t.MaxBuyIn - me.Stack
		}
	}
	a.text(x0, y, fmt.Sprintf("Buy-in %s to %s", commas(t.MinBuyIn), commas(t.MaxBuyIn)), a.A.C("text_dim"), alignLeft)
	a.text(x1, y, "You have "+commas(bal), a.A.C("text"), alignRight)
	y += 10
	for _, l := range a.wrap("Your chips go on the table and come back when you leave it.", x1-x0) {
		a.text(x0, y, l, a.A.C("text_dim"), alignLeft)
		y += 9
	}
	y += 6
	lo := t.MinBuyIn
	if a.buyRebuy {
		lo = 1
	}
	hi := min(room, bal)
	fr := image.Rect(x0, y, x0+100, y+18)
	enter := a.textField("buy_amt", fr, &a.buyAmt, false, 12, "chips", func(c rune) bool { return c >= '0' && c <= '9' })
	bx := fr.Max.X + 6
	for _, q := range []struct {
		l string
		n int64
	}{{"MIN", lo}, {"HALF", max(lo, hi/2)}, {"MAX", hi}} {
		w := a.A.Font.Width(q.l) + 12
		if a.button("buy_"+q.l, image.Rect(bx, y+1, bx+w, y+17), q.l, btn{enabled: hi >= lo}) {
			a.buyAmt = fmt.Sprint(q.n)
		}
		bx += w + 3
	}
	n, _ := strconv.ParseInt(a.buyAmt, 10, 64)
	ok := n >= lo && n <= hi
	switch {
	case hi < lo && !a.buyRebuy:
		a.text(x0, y+24, "You don't have enough chips for this table.", a.A.C("lose"), alignLeft)
	case n > bal:
		a.text(x0, y+24, "You don't have that many chips.", a.A.C("lose"), alignLeft)
	case n > room:
		a.text(x0, y+24, "That's over the table's maximum.", a.A.C("lose"), alignLeft)
	case n > 0 && n < lo:
		a.text(x0, y+24, "The minimum is "+commas(lo)+".", a.A.C("lose"), alignLeft)
	}
	cx := r.Min.X + r.Dx()/2
	by := r.Max.Y - 28
	label := "SIT DOWN"
	if a.buyRebuy {
		label = "ADD " + commas(n)
	}
	if a.button("buy_go", image.Rect(cx-104, by, cx-4, by+18), label, btn{enabled: ok, style: styleGold}) || (enter && ok) {
		switch {
		case a.buyRebuy:
			a.send(proto.CPoker, proto.PokerAct{Action: "rebuy", Amount: n})
		case a.game == proto.GameHoldem && a.ps != nil && a.ps.Table == t.ID:
			a.send(proto.CPoker, proto.PokerAct{Action: "sit", Amount: n})
		default:
			a.send(proto.CJoinTable, proto.JoinTable{ID: t.ID, BuyIn: n})
		}
		a.overlay = ovNone
		a.focusID = ""
	}
	alt := "WATCH"
	if a.buyRebuy || (a.ps != nil && a.ps.Table == t.ID) {
		alt = "CANCEL"
	}
	if a.button("buy_watch", image.Rect(cx+4, by, cx+104, by+18), alt, btn{enabled: true}) {
		if alt == "WATCH" {
			a.send(proto.CJoinTable, proto.JoinTable{ID: t.ID})
		}
		a.overlay = ovNone
		a.focusID = ""
	}
}

// ---------------------------------------------------------------- screen

const lobbyRowH = 44

func (a *App) drawLobby() {
	a.computeLayout()
	a.drawTableBG()
	fillRect(a.fb, a.fb.Rect, color.RGBA{0, 0, 0, 170})
	overlayOpen := a.overlay != ovNone
	updating := a.blocked
	a.blocked = overlayOpen || updating

	// top bar
	name := a.serverInfo.Name
	a.text(6, 4, fitText(a.A.Font, name, 200), a.A.C("gold"), alignLeft)
	a.text(6, 14, fmt.Sprintf("%s  -  %s chips", a.username, commas(a.profile.Balance)), a.A.C("text_dim"), alignLeft)
	x := ScreenW - 4
	type tb struct {
		id, label string
		hot       rune
		fn        func()
	}
	btns := []tb{
		{"logout", "LOG OUT", 0, a.logout},
		{"help", "?", '?', func() { a.overlay = ovHelp }},
		{"settings", "SETTINGS", 'o', func() { a.overlay = ovSettings }},
		{"profile", "PROFILE", 0, func() { a.overlay = ovProfile }},
		{"lb", "LEADERBOARD", 'l', a.openLeaderboard},
	}
	if a.profile.Admin {
		btns = append(btns, tb{"lobby_admin", "ADMIN", 0, func() { a.settingsTable = ""; a.openAdmin() }})
	}
	by := 4
	minX := 160
	if Portrait() {
		minX = 4
		by = 26 // the name and chips get the top row
	}
	for _, b := range btns {
		w := a.A.Font.Width(b.label) + 14
		if x-w < minX {
			x, by = ScreenW-4, by+19
		}
		r := image.Rect(x-w, by, x, by+16)
		if a.button(b.id, r, b.label, btn{enabled: true, hotkey: b.hot}) {
			b.fn()
		}
		x -= w + 3
	}

	top := max(30, by+24)
	a.textBig(ScreenW/2, top, "CHOOSE A TABLE", a.A.C("gold"), 2, alignCenter)
	top += 22
	bottom := ScreenH - 8
	if a.profile.Admin {
		bottom = ScreenH - 30
	}
	if !a.lobbyKnown {
		a.text(ScreenW/2, top+20, "Loading tables...", a.A.C("text"), alignCenter)
		a.blocked = updating
		a.drawOverlays()
		return
	}
	tables := a.lobby.Tables
	w := min(ScreenW-16, 520)
	x0 := ScreenW/2 - w/2
	visible := max(1, (bottom-top)/lobbyRowH)
	maxScroll := max(0, len(tables)-visible)
	if a.in.Wheel != 0 && !a.blocked {
		a.lobbyScroll -= a.in.Wheel
	}
	a.lobbyScroll = clampi(a.lobbyScroll, 0, maxScroll)
	if len(tables) == 0 {
		a.text(ScreenW/2, top+20, "No tables are open.", a.A.C("text_dim"), alignCenter)
	}
	for i := a.lobbyScroll; i < len(tables) && i < a.lobbyScroll+visible; i++ {
		t := tables[i]
		y := top + (i-a.lobbyScroll)*lobbyRowH
		r := image.Rect(x0, y, x0+w, y+lobbyRowH-4)
		a.drawLobbyRow(t, r)
	}
	if maxScroll > 0 {
		a.text(ScreenW/2, bottom-2, fmt.Sprintf("scroll for more (%d tables)", len(tables)), a.A.C("text_dim"), alignCenter)
	}
	if a.profile.Admin {
		a.drawLobbyAdmin(image.Rect(x0, ScreenH-26, x0+w, ScreenH-6))
	}
	a.drawConsolePeekAt(top)
	a.blocked = updating
	a.drawOverlays()
}

func (a *App) drawLobbyRow(t proto.TableInfo, r image.Rectangle) {
	cur := t.ID == a.lobby.Current
	bg := color.RGBA{10, 30, 18, 210}
	if a.hovered(r) {
		bg = color.RGBA{16, 44, 26, 230}
	}
	fillRect(a.fb, r, bg)
	edge := a.A.C("shadow")
	if cur {
		edge = a.A.C("gold")
	}
	strokeRect(a.fb, r, edge)
	gc := a.A.C("highlight")
	switch t.Game {
	case proto.GameHoldem:
		gc = a.A.C("win")
	case proto.GameBeggar:
		gc = a.A.C("text_dim")
	}
	a.text(r.Min.X+6, r.Min.Y+4, gameLabel(t.Game), gc, alignLeft)
	a.text(r.Min.X+6+a.A.Font.Width(gameLabel(t.Game))+8, r.Min.Y+4, fitText(a.A.Font, t.Name, 160), a.A.C("gold"), alignLeft)
	info := fmt.Sprintf("%s  -  %d/%d seated", t.Stakes, t.Players, t.Seats)
	if t.Game == proto.GameBeggar {
		info = fmt.Sprintf("%s  -  %d here", t.Stakes, t.Players)
	}
	if t.Watching > 0 {
		info += fmt.Sprintf("  -  %d watching", t.Watching)
	}

	names := "No one here yet"
	if t.Game == proto.GameHoldem {
		names = "Dale is waiting to play"
	}
	if len(t.Names) > 0 {
		names = strings.Join(t.Names, ", ")
		if t.Game == proto.GameHoldem {
			names += ", Dale"
		}
	}
	bw := 64
	adminW := 0
	if a.profile.Admin {
		adminW = 2*22 + 6
	}
	room := r.Dx() - bw*2 - adminW - 24
	a.text(r.Min.X+6, r.Min.Y+15, fitText(a.A.Font, info, room), a.A.C("text"), alignLeft)
	a.text(r.Min.X+6, r.Min.Y+26, fitText(a.A.Font, names, room), a.A.C("text_dim"), alignLeft)

	bx := r.Max.X - 6
	by := r.Min.Y + (r.Dy()-18)/2
	if a.profile.Admin {
		closeR := image.Rect(bx-20, by, bx, by+18)
		if a.button("tclose_"+t.ID, closeR, "X", btn{enabled: true}) {
			a.adminBackend().Exec("table remove " + t.ID)
		}
		if a.hovered(closeR) {
			a.text(closeR.Max.X, closeR.Max.Y+3, "close table", a.A.C("lose"), alignRight)
		}
		bx -= 22
		setR := image.Rect(bx-20, by, bx, by+18)
		if a.button("tset_"+t.ID, setR, "S", btn{enabled: true}) {
			a.settingsTable = t.ID
			a.openAdmin()
		}
		if a.hovered(setR) {
			a.text(setR.Max.X, setR.Max.Y+3, "table settings", a.A.C("text"), alignRight)
		}
		bx -= 26
	}
	full := t.Players >= t.Seats
	switch {
	case cur:
		if a.button("tjoin_"+t.ID, image.Rect(bx-bw*2-4, by, bx, by+18), "BACK TO TABLE", btn{enabled: true, style: styleGold}) {
			a.joinTable(t)
		}
	case t.Game == proto.GameHoldem:
		if a.button("twatch_"+t.ID, image.Rect(bx-bw, by, bx, by+18), "WATCH", btn{enabled: true}) {
			a.send(proto.CJoinTable, proto.JoinTable{ID: t.ID})
		}
		bx -= bw + 4
		if a.button("tjoin_"+t.ID, image.Rect(bx-bw, by, bx, by+18), "SIT", btn{enabled: !full, style: styleGold}) {
			a.openBuyIn(t, false)
		}
	default:
		label := "JOIN"
		if full {
			label = "WATCH"
		}
		if a.button("tjoin_"+t.ID, image.Rect(bx-bw, by, bx, by+18), label, btn{enabled: true, style: styleGold}) {
			a.joinTable(t)
		}
	}
}

func (a *App) drawLobbyAdmin(r image.Rectangle) {
	a.text(r.Min.X, r.Min.Y+5, "NEW TABLE", a.A.C("text_dim"), alignLeft)
	fx := r.Min.X + a.A.Font.Width("NEW TABLE") + 6
	fr := image.Rect(fx, r.Min.Y, fx+140, r.Min.Y+18)
	a.textField("new_table", fr, &a.newTable, false, 30, "name", nil)
	bx := fr.Max.X + 6
	for _, g := range []struct{ id, label string }{{proto.GameBlackjack, "+ BLACKJACK"}, {proto.GameHoldem, "+ HOLD'EM"}} {
		w := a.A.Font.Width(g.label) + 14
		if a.button("tnew_"+g.id, image.Rect(bx, r.Min.Y, bx+w, r.Min.Y+18), g.label, btn{enabled: true}) {
			name := strings.TrimSpace(a.newTable)
			if name == "" {
				name = a.nextTableName(g.id)
			}
			a.adminBackend().Exec("table add " + g.id + " " + name)
			a.newTable = ""
			a.focusID = ""
		}
		bx += w + 4
	}
}

// nextTableName makes up a name like "Hold'em 2".
func (a *App) nextTableName(game string) string {
	base := "Blackjack"
	if game == proto.GameHoldem {
		base = "Hold'em"
	}
	n := 1
	for _, t := range a.lobby.Tables {
		if t.Game == game {
			n++
		}
	}
	return fmt.Sprintf("%s %d", base, n)
}

// drawConsolePeekAt shows admin command output in the lobby.
func (a *App) drawConsolePeekAt(y int) {
	if len(a.console) == 0 || a.overlay == ovAdmin || a.now.Sub(a.consoleAt) > 6*time.Second {
		return
	}
	l := a.console[len(a.console)-1]
	w := a.A.Font.Width(l) + 12
	r := image.Rect(ScreenW/2-w/2, y-14, ScreenW/2+w/2, y-3)
	fillRect(a.fb, r, color.RGBA{0, 0, 0, 220})
	a.text(ScreenW/2, r.Min.Y+2, l, a.A.C("text"), alignCenter)
}
