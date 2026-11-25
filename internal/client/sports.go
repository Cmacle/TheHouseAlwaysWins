// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Sportsbook: a board of today's busiest esports markets at Polymarket's
// odds. Click a side's odds to put it on the bet slip, pick a stake, and
// place the bet. MY BETS shows your bets and how they went; TABLE BETS
// shows everyone's. Chips riding on each side show on the board too.

const (
	sbBoard = iota
	sbMine
	sbTable
)

type sbSelection struct {
	market  string
	outcome int
}

// sbDrag tracks a drag on a list, so it can be scrolled by touch.
type sbDrag struct {
	active   bool
	startY   int
	scroll0  int
	dragging bool // moved far enough that it's a scroll, not a tap
}

func (a *App) onSports(sp *proto.SportsState) {
	a.clockOffset = sp.ServerTime - time.Now().UnixMilli()
	a.st = &proto.TableState{TableID: sp.Table, TableName: sp.Name, Phase: "betting", YourSeat: -1, ActiveSeat: -1,
		ServerTime: sp.ServerTime, Settings: sp.Settings, HouseNet: sp.HouseNet, HouseTier: sp.HouseTier}
	a.sp = sp
	if a.sbStake == 0 {
		a.sbStake = min(max(50, sp.MinBet), sp.MaxBet)
	}
	if a.sbSel.market != "" && a.sbMarket(a.sbSel.market) == nil {
		a.sbSel = sbSelection{} // gone from the board
	}
}

func (a *App) sbMarket(id string) *proto.SportsMarket {
	if a.sp == nil {
		return nil
	}
	for i := range a.sp.Markets {
		if a.sp.Markets[i].ID == id {
			return &a.sp.Markets[i]
		}
	}
	return nil
}

// gameTag is a market's game as a short coloured label.
func (a *App) gameTag(game string) (string, color.RGBA) {
	switch game {
	case "cs":
		return "CS2", parseHex("#f0a830")
	case "lol":
		return "LOL", parseHex("#5aa9ff")
	case "dota":
		return "DOTA", parseHex("#e05050")
	case "valorant":
		return "VAL", parseHex("#ff5f7f")
	}
	return strings.ToUpper(game), a.A.C("text")
}

// whenText says when a match starts: "LIVE", "in 2h 15m", "in 40m".
func (a *App) whenText(start int64, live bool) string {
	if live {
		return "LIVE"
	}
	if start == 0 {
		return ""
	}
	d := time.Until(time.Unix(start, 0).Add(-time.Duration(a.clockOffset) * time.Millisecond))
	switch {
	case d <= 0:
		return "starting"
	case d < time.Hour:
		return fmt.Sprintf("in %dm", int(d.Minutes())+1)
	case d < 48*time.Hour:
		return fmt.Sprintf("in %dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return time.Unix(start, 0).Local().Format("Jan 2 15:04")
}

func oddsText(price float64) string {
	if price <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fx", 1/price)
}

// sbContent is the area between the top bar and the bottom panel.
func (a *App) sbContent() image.Rectangle {
	top := max(logButtonRect().Max.Y, chatButtonRect().Max.Y) + 8
	if a.profile.Admin {
		top = max(top, a.adminButtonRect().Max.Y+6)
	}
	return image.Rect(6, top, ScreenW-6, a.L.hudTop-4)
}

func (a *App) drawSports() {
	a.chatPreInput()
	a.computeLayout()
	a.drawTableBG()
	sp, st := a.sp, a.st
	if sp == nil || st == nil {
		a.text(ScreenW/2, ScreenH/2-10, "Joining the Sportsbook...", a.A.C("text"), alignCenter)
		a.drawOverlays()
		return
	}
	overlayOpen := a.overlay != ovNone
	updating := a.blocked
	a.blocked = overlayOpen || updating

	area := a.sbContent()
	// tabs, and the board's game filter
	tabY := area.Min.Y
	x := area.Min.X
	mine, riding := 0, 0
	for _, b := range sp.Mine {
		if b.Status == "open" {
			mine++
		}
	}
	for _, b := range sp.Others {
		if b.Status == "open" {
			riding++
		}
	}
	tabs := []struct {
		id    int
		label string
	}{{sbBoard, "BOARD"}, {sbMine, fmt.Sprintf("MY BETS %d", mine)}, {sbTable, fmt.Sprintf("TABLE BETS %d", riding)}}
	for _, tb := range tabs {
		w := a.A.Font.Width(tb.label) + 12
		r := image.Rect(x, tabY, x+w, tabY+15)
		style := styleRed
		if a.sbTab == tb.id {
			style = styleGold
		}
		if a.button("sbtab"+tb.label, r, tb.label, btn{enabled: true, style: style}) && a.sbTab != tb.id {
			a.sbTab, a.sbScroll = tb.id, 0
		}
		x += w + 3
	}
	if a.sbTab == sbBoard {
		fx := area.Max.X
		for _, f := range []struct{ id, label string }{{"lol", "LOL"}, {"cs", "CS2"}, {"", "ALL"}} {
			w := a.A.Font.Width(f.label) + 10
			r := image.Rect(fx-w, tabY, fx, tabY+15)
			style := styleRed
			if a.sbFilter == f.id {
				style = styleGold
			}
			if a.button("sbf"+f.label, r, f.label, btn{enabled: true, style: style}) {
				a.sbFilter, a.sbScroll = f.id, 0
			}
			fx -= w + 2
		}
	}
	// where the odds come from, and how fresh they are
	srcY := tabY + 19
	src := "Odds from Polymarket"
	if sp.Updated > 0 {
		age := time.Since(time.Unix(sp.Updated, 0).Add(-time.Duration(a.clockOffset) * time.Millisecond))
		src += fmt.Sprintf(", updated %s ago", shortDur(age))
	}
	if !sp.Live {
		src += ". Bets close when a match starts."
	}
	srcCol := a.A.C("text_dim")
	if sp.Error != "" {
		src, srcCol = sp.Error, a.A.C("error")
	}
	a.text(area.Min.X, srcY, fitText(a.A.Font, src, area.Dx()), srcCol, alignLeft)

	list := image.Rect(area.Min.X, srcY+12, area.Max.X, area.Max.Y-12)
	switch a.sbTab {
	case sbBoard:
		a.drawSportsBoard(sp, list)
	case sbMine:
		a.drawSportsBets(sp.Mine, list, false, "You haven't bet yet. Pick a side on the BOARD.")
	case sbTable:
		a.drawSportsBets(sp.Others, list, true, "Nobody has bet at this table yet.")
	}
	// the latest news
	if n := len(sp.Feed); n > 0 {
		a.text(ScreenW/2, area.Max.Y-9, fitText(a.A.Font, sp.Feed[n-1], area.Dx()), a.A.C("gold"), alignCenter)
	}

	a.drawTopBar(st)
	a.drawSportsHUD(sp)
	a.drawChatBar(st)
	a.blocked = updating
	a.drawOverlays()
}

func shortDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(0, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// listScroll handles the wheel and touch drags over a list of rows and
// returns the first row to show. tap says whether a click on a row should
// count (it doesn't when it ended a drag).
func (a *App) listScroll(list image.Rectangle, rowH, rows int) (first int, tap bool) {
	visible := max(1, list.Dy()/rowH)
	maxScroll := max(0, rows-visible)
	if a.hovered(list) && a.in.Wheel != 0 {
		a.sbScroll -= a.in.Wheel
	}
	d := &a.sbDrag
	if a.in.Pressed && a.hovered(list) {
		*d = sbDrag{active: true, startY: a.in.MY, scroll0: a.sbScroll}
	}
	if d.active && a.in.Down {
		dy := a.in.MY - d.startY
		if abs(dy) > 5 {
			d.dragging = true
		}
		if d.dragging {
			a.sbScroll = d.scroll0 - dy/rowH
		}
	}
	tap = a.in.Released && !(d.active && d.dragging)
	if a.in.Released {
		d.active = false
	}
	a.sbScroll = clampi(a.sbScroll, 0, maxScroll)
	if maxScroll > 0 {
		// a scroll bar on the right
		bar := image.Rect(list.Max.X-3, list.Min.Y, list.Max.X, list.Min.Y+visible*rowH)
		fillRect(a.fb, bar, color.RGBA{0, 0, 0, 90})
		h := max(8, bar.Dy()*visible/rows)
		y := bar.Min.Y + (bar.Dy()-h)*a.sbScroll/maxScroll
		fillRect(a.fb, image.Rect(bar.Min.X, y, bar.Max.X, y+h), a.A.C("gold"))
	}
	return a.sbScroll, tap
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (a *App) drawSportsBoard(sp *proto.SportsState, list image.Rectangle) {
	var ms []proto.SportsMarket
	for _, m := range sp.Markets {
		if a.sbFilter == "" || m.Game == a.sbFilter {
			ms = append(ms, m)
		}
	}
	if len(ms) == 0 {
		msg := "No matches on the board right now."
		if sp.Updated == 0 && sp.Error == "" {
			msg = "Fetching the odds from Polymarket..."
		}
		a.text(ScreenW/2, list.Min.Y+20, msg, a.A.C("text"), alignCenter)
		return
	}
	portrait := a.L.portrait
	rowH := 30
	if portrait {
		rowH = 46
	}
	first, tap := a.listScroll(list, rowH, len(ms))
	visible := max(1, list.Dy()/rowH)
	for i := first; i < len(ms) && i < first+visible; i++ {
		m := ms[i]
		y := list.Min.Y + (i-first)*rowH
		row := image.Rect(list.Min.X, y, list.Max.X-5, y+rowH-2)
		fillRect(a.fb, row, color.RGBA{0, 0, 0, 110})
		tag, tc := a.gameTag(m.Game)
		tw := 28
		fillRect(a.fb, image.Rect(row.Min.X+2, row.Min.Y+2, row.Min.X+2+tw, row.Min.Y+12), shade(tc, 0.45))
		a.text(row.Min.X+2+tw/2, row.Min.Y+3, tag, tc, alignCenter)
		// the outcome boxes: right of the text in landscape, under it on a phone
		boxW, boxH := 118, rowH-6
		bx := row.Max.X - 2*boxW - 6
		by := row.Min.Y + 3
		textW := bx - (row.Min.X + tw + 8) - 4
		if portrait {
			boxW, boxH = (row.Dx()-12)/2, 22
			bx, by = row.Min.X+4, row.Min.Y+20
			textW = row.Dx() - tw - 12
		}
		a.text(row.Min.X+tw+8, row.Min.Y+3, fitText(a.A.Font, m.Event, textW), a.A.C("text"), alignLeft)
		when := a.whenText(m.Start, m.Live)
		wc := a.A.C("text_dim")
		if m.Live {
			wc = a.A.C("error")
		}
		sub := m.Label
		if when != "" {
			sub += "  -  " + when
		}
		if portrait {
			a.text(row.Max.X-4, row.Min.Y+3, when, wc, alignRight)
			sub = m.Label
		}
		a.text(row.Min.X+tw+8, row.Min.Y+13, fitText(a.A.Font, sub, textW), wc, alignLeft)
		for k, o := range m.Outcomes {
			if k > 1 {
				break
			}
			r := image.Rect(bx+k*(boxW+4), by, bx+k*(boxW+4)+boxW, by+boxH)
			a.drawOutcomeBox(m, k, o, r, tap)
		}
	}
}

// drawOutcomeBox draws one side's odds, and picks it when tapped.
func (a *App) drawOutcomeBox(m proto.SportsMarket, k int, o proto.SportsOutcome, r image.Rectangle, tap bool) {
	sel := a.sbSel.market == m.ID && a.sbSel.outcome == k
	hov := a.hovered(r) && m.Open
	bg := color.RGBA{24, 64, 40, 255}
	switch {
	case !m.Open:
		bg = color.RGBA{40, 40, 40, 255}
	case sel:
		bg = color.RGBA{120, 92, 20, 255}
	case hov:
		bg = color.RGBA{34, 90, 56, 255}
	}
	fillRect(a.fb, r, bg)
	edge := a.A.C("shadow")
	if sel {
		edge = a.A.C("gold")
	}
	strokeRect(a.fb, r, edge)
	odds := oddsText(o.Price)
	nameW := r.Dx() - a.A.Font.Width(odds) - 10
	a.text(r.Min.X+4, r.Min.Y+3, fitText(a.A.Font, o.Name, nameW), a.A.C("text"), alignLeft)
	oc := a.A.C("gold")
	if !m.Open {
		oc = a.A.C("text_dim")
	}
	a.text(r.Max.X-4, r.Min.Y+3, odds, oc, alignRight)
	info := fmt.Sprintf("%.0f%%", o.Price*100)
	if o.Bets > 0 {
		info = fmt.Sprintf("%.0f%%  -  %d bet%s, %s", o.Price*100, o.Bets, plural(o.Bets), commas(o.Stake))
	}
	ic := a.A.C("text_dim")
	if o.Bets > 0 {
		ic = a.A.C("highlight")
	}
	if r.Dy() >= 20 {
		a.text(r.Min.X+4, r.Min.Y+12, fitText(a.A.Font, info, r.Dx()-8), ic, alignLeft)
	}
	if tap && hov && image.Pt(a.in.MX, a.in.MY).In(r) {
		if sel {
			a.sbSel = sbSelection{}
		} else {
			a.sbSel = sbSelection{market: m.ID, outcome: k}
		}
		a.sound("click")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// drawSportsBets lists bets: yours, or everyone's (with names).
func (a *App) drawSportsBets(bets []proto.SportsBet, list image.Rectangle, names bool, empty string) {
	if len(bets) == 0 {
		a.text(ScreenW/2, list.Min.Y+20, empty, a.A.C("text"), alignCenter)
		return
	}
	portrait := a.L.portrait
	rowH := 24
	if portrait {
		rowH = 34
	}
	first, _ := a.listScroll(list, rowH, len(bets))
	visible := max(1, list.Dy()/rowH)
	for i := first; i < len(bets) && i < first+visible; i++ {
		b := bets[i]
		y := list.Min.Y + (i-first)*rowH
		row := image.Rect(list.Min.X, y, list.Max.X-5, y+rowH-2)
		bg := color.RGBA{0, 0, 0, 110}
		if strings.EqualFold(b.User, a.username) && names {
			bg = color.RGBA{40, 32, 0, 130}
		}
		fillRect(a.fb, row, bg)
		tag, tc := a.gameTag(b.Game)
		a.text(row.Min.X+4, row.Min.Y+3, tag, tc, alignLeft)
		x := row.Min.X + 32
		if names {
			nc := a.A.C("text")
			if strings.EqualFold(b.User, a.username) {
				nc = a.A.C("gold")
			}
			a.text(x, row.Min.Y+3, fitText(a.A.Font, b.User, 84), nc, alignLeft)
			x += 88
		}
		status, sc := a.betStatus(b)
		statusW := 96
		pick := fmt.Sprintf("%s @ %s: %s to win %s", b.Pick, oddsText(b.Price), commas(b.Stake), commas(b.Payout))
		where := b.Event + "  -  " + b.Label
		if portrait {
			a.text(x, row.Min.Y+3, fitText(a.A.Font, pick, row.Max.X-x-statusW), a.A.C("text"), alignLeft)
			a.text(row.Min.X+4, row.Min.Y+14, fitText(a.A.Font, where, row.Dx()-8), a.A.C("text_dim"), alignLeft)
		} else {
			a.text(x, row.Min.Y+3, fitText(a.A.Font, pick, row.Max.X-x-statusW-4), a.A.C("text"), alignLeft)
			a.text(x, row.Min.Y+12, fitText(a.A.Font, where, row.Max.X-x-statusW-4), a.A.C("text_dim"), alignLeft)
		}
		a.text(row.Max.X-4, row.Min.Y+3, status, sc, alignRight)
	}
}

// betStatus says how a bet stands: "OPEN, in 2h 10m", "WON +61", "LOST".
func (a *App) betStatus(b proto.SportsBet) (string, color.RGBA) {
	switch b.Status {
	case "won":
		return "WON +" + commas(b.Payout-b.Stake), a.A.C("win")
	case "lost":
		return "LOST -" + commas(b.Stake), a.A.C("error")
	case "refunded":
		return "REFUNDED", a.A.C("text_dim")
	}
	if m := a.sbMarket(b.Market); m != nil {
		if w := a.whenText(m.Start, m.Live); w != "" {
			if m.Live {
				return "LIVE", a.A.C("error")
			}
			return "OPEN, " + w, a.A.C("highlight")
		}
	}
	return "WAITING", a.A.C("highlight")
}

// drawSportsHUD is the bottom panel: you, your chips and the bet slip.
func (a *App) drawSportsHUD(sp *proto.SportsState) {
	L := a.L
	hudTop := L.hudTop
	a.panel(image.Rect(0, hudTop, ScreenW, ScreenH))
	ar := image.Rect(10, hudTop+12, 42, hudTop+44)
	a.drawFrame(a.profile.Frame, ar.Min.X, ar.Min.Y)
	blit(a.fb, a.avatarImg(a.username, a.profile.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
	a.text(48, hudTop+7, fitText(a.A.Font, a.username, 110), a.A.C("gold"), alignLeft)
	a.text(48, hudTop+18, "CHIPS", a.A.C("text_dim"), alignLeft)
	a.text(86, hudTop+18, commas(sp.Balance), a.A.C("text"), alignLeft)
	var riding int64
	for _, b := range sp.Mine {
		if b.Status == "open" {
			riding += b.Stake
		}
	}
	a.text(48, hudTop+29, "RIDING", a.A.C("text_dim"), alignLeft)
	a.text(86, hudTop+29, commas(riding), a.A.C("text"), alignLeft)

	x0, x1 := L.cx0, ScreenW-8
	top := hudTop + 6
	if L.portrait {
		x0, top = 8, hudTop+50
		fillRect(a.fb, image.Rect(6, top-3, ScreenW-6, top-2), color.RGBA{0, 0, 0, 90})
	}
	m := a.sbMarket(a.sbSel.market)
	if m == nil || a.sbSel.outcome >= len(m.Outcomes) {
		a.text((x0+x1)/2, top+16, "Pick a side's odds on the board to bet on it.", a.A.C("text_dim"), alignCenter)
		return
	}
	o := m.Outcomes[a.sbSel.outcome]
	slip := fmt.Sprintf("%s to win  -  %s, %s", o.Name, m.Event, strings.ToLower(m.Label))
	a.text(x0, top, fitText(a.A.Font, slip, x1-x0), a.A.C("gold"), alignLeft)
	// the stake: chip amounts, then minus / amount / plus
	stake := clamp64(a.sbStake, sp.MinBet, max(sp.MinBet, min(sp.MaxBet, sp.Balance)))
	a.sbStake = stake
	y := top + 11
	x := x0
	for _, v := range []int64{10, 50, 100, 500} {
		if v < sp.MinBet || v > sp.MaxBet {
			continue
		}
		lbl := "+" + commas(v)
		w := a.A.Font.Width(lbl) + 10
		if a.button(fmt.Sprintf("sbc%d", v), image.Rect(x, y, x+w, y+16), lbl, btn{enabled: true}) {
			a.sbStake = min(stake+v, sp.MaxBet)
		}
		x += w + 2
	}
	for _, q := range []struct {
		lbl string
		v   func() int64
	}{{"MIN", func() int64 { return sp.MinBet }}, {"MAX", func() int64 { return min(sp.MaxBet, sp.Balance) }}} {
		w := a.A.Font.Width(q.lbl) + 10
		if a.button("sbq"+q.lbl, image.Rect(x, y, x+w, y+16), q.lbl, btn{enabled: true}) {
			a.sbStake = q.v()
		}
		x += w + 2
	}
	if a.hovered(image.Rect(x0, y, x1, y+16)) && a.in.Wheel != 0 {
		a.sbStake = clamp64(stake+int64(a.in.Wheel)*10, sp.MinBet, sp.MaxBet)
	}
	pays := int64(math.Floor(float64(stake) / math.Max(o.Price, 0.001)))
	label := fmt.Sprintf("BET %s  -  PAYS %s", commas(stake), commas(pays))
	can := m.Open && sp.Balance >= stake && stake >= sp.MinBet
	why := ""
	switch {
	case !m.Open && m.Live:
		why = "That match has started: bets are closed."
	case !m.Open:
		why = "Not taking bets right now."
	case sp.Balance < sp.MinBet:
		why = fmt.Sprintf("You need at least %s chips to bet.", commas(sp.MinBet))
	}
	bw := min(220, x1-x0)
	br := image.Rect(x1-bw, y+19, x1, y+39)
	if L.portrait {
		br = image.Rect(x0, y+19, x1, y+39)
	}
	if a.button("sb_bet", br, label, btn{enabled: can, style: styleGold, key: KeyEnter}) {
		a.send(proto.CSports, proto.SportsAct{Action: "bet", Market: m.ID, Outcome: a.sbSel.outcome, Stake: stake, Price: o.Price})
		a.sbSel = sbSelection{}
	}
	if why != "" {
		a.text(x0, y+24, fitText(a.A.Font, why, br.Min.X-x0-6), a.A.C("error"), alignLeft)
	} else if !L.portrait {
		a.text(x0, y+24, fitText(a.A.Font, fmt.Sprintf("Profit %s if %s wins", commas(pays-stake), o.Name), br.Min.X-x0-6), a.A.C("text_dim"), alignLeft)
	}
}

func clamp64(v, lo, hi int64) int64 { return min(max(v, lo), hi) }
