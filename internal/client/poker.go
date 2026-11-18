// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/poker"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Texas Hold'em table. Dale deals from his usual spot and plays seat 0;
// the players sit around the rail like at blackjack. The shared pieces (Dale,
// his speech bubble and log, the top bar, chat, the windows) run off a
// blackjack-shaped copy of the poker state in a.st.

// pokerShell turns a poker state into the table state the shared code reads.
func pokerShell(ps *proto.PokerState) *proto.TableState {
	st := &proto.TableState{TableID: ps.Table, TableName: ps.Name, Phase: ps.Phase, HandNumber: ps.HandNumber,
		Deadline: ps.Deadline, TimerTotal: ps.TimerTotal, ServerTime: ps.ServerTime,
		ActiveSeat: ps.Active, YourSeat: ps.YourSeat, Message: ps.Message, Settings: ps.Settings,
		Spectators: ps.Spectators, DealerSay: ps.DealerSay, DealerSayID: ps.DealerSayID,
		DealerSayUntil: ps.DealerSayUntil, DealerSayCont: ps.DealerSayCont, DealerSayPara: ps.DealerSayPara,
		DealerLog: ps.DealerLog, HouseNet: ps.HouseNet, HouseTier: ps.HouseTier, Actions: ps.Actions}
	if ps.YourSeat <= 0 {
		st.YourSeat = -1
	}
	for _, s := range ps.Seats {
		sv := proto.SeatView{Seat: s.Seat, Username: s.Username, AvatarVersion: s.AvatarVersion,
			Balance: s.Stack, Connected: s.Connected, Hat: s.Hat, Frame: s.Frame, Title: s.Title, ActiveHand: -1}
		if s.SittingOut {
			sv.Status = "sitting_out"
		}
		st.Seats = append(st.Seats, sv)
	}
	return st
}

func (a *App) pokerMe() *proto.PokerSeat {
	ps := a.ps
	if ps == nil || ps.YourSeat <= 0 || ps.YourSeat >= len(ps.Seats) {
		return nil
	}
	return &ps.Seats[ps.YourSeat]
}

func pokerCardKeys(ps *proto.PokerState, fn func(key string, c bj.Card)) {
	for i, c := range ps.Board {
		fn(fmt.Sprintf("pb%d", i), c)
	}
	for _, s := range ps.Seats {
		for i, c := range s.Cards {
			fn(fmt.Sprintf("ps%d_%d", s.Seat, i), c)
		}
	}
}

func (a *App) onPoker(ps *proto.PokerState) {
	prev := a.ps
	a.clockOffset = ps.ServerTime - time.Now().UnixMilli()
	newHand := prev == nil || prev.HandNumber != ps.HandNumber
	seen := map[string]time.Time{}
	hidden := map[string]bool{}
	vals := map[string]bj.Card{}
	fresh := 0
	pokerCardKeys(ps, func(k string, c bj.Card) {
		hidden[k], vals[k] = c.Hidden(), c
		if prev == nil {
			seen[k] = a.now.Add(-time.Hour)
			return
		}
		if t, ok := a.cardSeen[k]; ok && !newHand {
			seen[k] = t
			if a.cardHidden[k] && !c.Hidden() {
				a.flipAt[k] = a.now
			}
			return
		}
		seen[k] = a.now.Add(time.Duration(fresh) * dealStagger / 2)
		fresh++
	})
	a.cardSeen, a.cardHidden, a.cardVal = seen, hidden, vals

	if prev != nil && !newHand {
		for i, s := range ps.Seats {
			if i < len(prev.Seats) && s.Bet > prev.Seats[i].Bet {
				a.sound("chip")
				break
			}
		}
	}
	// your result
	if me := pokerSeat(ps, ps.YourSeat); me != nil && ps.Phase == "showdown" && (prev == nil || prev.Phase != "showdown") {
		switch {
		case me.Won > 0:
			a.sound("win")
			msg := "YOU WIN " + commas(me.Won)
			if me.HandName != "" {
				msg += " - " + strings.ToUpper(me.HandName)
			}
			a.toast(msg, a.A.C("win"), 3, 2)
		case me.InHand && me.HandName != "":
			a.sound("lose")
		}
	}
	// the bet slider starts at the minimum each time it's your turn
	if len(ps.Actions) > 0 && (prev == nil || len(prev.Actions) == 0 || a.pkRaiseFor != ps.HandNumber || a.pkRaise < ps.MinRaise || a.pkRaise > ps.MaxRaise) {
		a.pkRaise = ps.MinRaise
		a.pkRaiseFor = ps.HandNumber
	}
	if (len(ps.Actions) > 0 && (prev == nil || len(prev.Actions) == 0)) || (ps.CanDealIn && (prev == nil || !prev.CanDealIn)) {
		a.sound("turn")
	}
	if newHand {
		a.pkRev = nil
	}
	a.pokerFX(prev, ps)
	shell := pokerShell(ps)
	a.onDealerLines(shell)
	a.ps, a.st = ps, shell
	a.onPokerReveal(ps)
}

func pokerSeat(ps *proto.PokerState, i int) *proto.PokerSeat {
	if ps == nil || i <= 0 || i >= len(ps.Seats) {
		return nil
	}
	return &ps.Seats[i]
}

// pokerSeatPos is where player seat i (1..n-1) sits.
func (a *App) pokerSeatPos(i, n int) (x, cy int) {
	return a.seatPos(i-1, n-1)
}

func (a *App) pokerCardTop(cy int) int {
	return cy - 16 - 4 - a.A.Theme.HatHeight + a.A.Theme.HatOverlap - a.A.Theme.CardHeight + 6
}

func (a *App) drawPoker() {
	a.chatPreInput()
	a.computeLayout()
	a.drawTableBG()
	L := a.L
	ps, st := a.ps, a.st
	if ps == nil || st == nil {
		a.text(ScreenW/2, ScreenH/2-10, "Joining the table...", a.A.C("text"), alignCenter)
		a.drawOverlays()
		return
	}
	overlayOpen := a.overlay != ovNone
	updating := a.blocked
	a.blocked = overlayOpen || updating
	var fly []flying
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	a.pkGeo = map[int]pkGeo{}
	blink := (a.frame/20)%2 == 0

	// Dale: the dealer, and seat 0
	blit(a.fb, a.A.Shoe, L.shoe.X-6, L.shoe.Y-2)
	a.drawDealer(st)
	if len(ps.Seats) > 0 {
		d := ps.Seats[0]
		x0 := ScreenW/2 - (cw+14)/2
		for i, c := range d.Cards {
			a.drawCardAt(c, fmt.Sprintf("ps0_%d", i), x0+i*14, L.cardsY, &fly)
		}
		lx := ScreenW/2 - 40
		if ps.Active == 0 && blink {
			a.text(lx, L.cardsY+2, "DALE", a.A.C("highlight"), alignRight)
		} else {
			a.text(lx, L.cardsY+2, "DALE", a.A.C("gold"), alignRight)
		}
		if line, c := a.pokerSeatLine(ps, d); line != "" {
			a.text(lx, L.cardsY+12, fitText(a.A.Font, line, 90), c, alignRight)
		}
		if d.HandName != "" {
			a.text(lx, L.cardsY+22, strings.ToUpper(d.HandName), a.A.C("highlight"), alignRight)
		}
		if ps.Button == 0 {
			a.drawButtonPuck(lx-a.A.Font.Width("DALE")-12, L.cardsY+1)
		}
		// his chips and his bet, right of his cards, like everyone else's
		dpx := x0 + cw + 14 + 8
		dBottom := L.cardsY + ch
		a.drawChipPile(dpx, dBottom, d.Stack, false)
		if d.Bet > 0 {
			a.drawSeatBet(dpx+pileChip+14, dBottom, d.Bet, false)
		}
		a.pkGeo[0] = pkGeo{x: ScreenW / 2, cy: L.cardsY + ch/2, pile: image.Pt(dpx+6, dBottom-8),
			bet: image.Pt(dpx+pileChip+20, dBottom-6), cards: image.Pt(x0, L.cardsY), labelBottom: L.cardsY + 44, labelX: lx - 30}
	}

	// the pot and the board
	potY := L.cardsY + ch + 4
	boardY := potY + 11
	if L.portrait {
		potY = L.msgY - 4
		boardY = potY + 11
	}
	if ps.Pot > 0 {
		label := "POT " + commas(ps.Pot)
		if len(ps.Pots) > 1 {
			parts := make([]string, len(ps.Pots))
			for i, p := range ps.Pots {
				parts[i] = shortChips(p)
			}
			label += "  (" + strings.Join(parts, " + ") + ")"
		}
		a.textOutlined(ScreenW/2, potY, label, a.A.C("gold"), alignCenter)
	}
	step := cw + 3
	bx := ScreenW/2 - (5*step-3)/2
	for i := 0; i < 5; i++ {
		x := bx + i*step
		if i < len(ps.Board) {
			continue
		}
		strokeRect(a.fb, image.Rect(x, boardY, x+cw, boardY+ch), a.A.C("felt_text"))
	}
	for i, c := range ps.Board {
		if a.pkRevealHides(i, bx+i*step, boardY) {
			continue
		}
		a.drawCardAt(c, fmt.Sprintf("pb%d", i), bx+i*step, boardY, &fly)
	}
	msgY := boardY + ch + 5
	if ps.Message != "" {
		a.text(ScreenW/2, msgY, fitText(a.A.Font, ps.Message, ScreenW-40), a.A.C("highlight"), alignCenter)
	}
	if !L.noRules && !L.portrait {
		felt := a.A.C("felt_text")
		blinds := fmt.Sprintf("NO-LIMIT HOLD'EM  -  BLINDS %s/%s", commas(ps.SmallBlind), commas(ps.BigBlind))
		a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(blinds)/2, msgY+11, blinds, felt)
	}

	// players
	n := len(ps.Seats)
	seatW := max(a.seatSpacing(n-1), cw+4)
	type seatHat struct {
		id   string
		x, y int
	}
	var hats []seatHat
	for i := 1; i < n; i++ {
		sv := ps.Seats[i]
		x, cy := a.pokerSeatPos(i, n)
		ar := image.Rect(x-16, cy-16, x+16, cy+16)
		if sv.Username == "" {
			for k := 0; k < 64; k++ {
				ang := float64(k) / 64 * 2 * math.Pi
				px := x + int(math.Round(14*math.Cos(ang)))
				py := cy + int(math.Round(14*math.Sin(ang)))
				fillRect(a.fb, image.Rect(px, py, px+1, py+1), a.A.C("seat_open"))
			}
			a.A.Font.Draw(a.fb, x-a.A.Font.Width("OPEN")/2, cy-3, "OPEN", a.A.C("seat_open"))
			continue
		}
		me := i == ps.YourSeat
		a.drawFrame(sv.Frame, ar.Min.X, ar.Min.Y)
		if ps.Active == i && blink {
			strokeRect(a.fb, ar.Inset(-3), a.A.C("highlight"))
		}
		blit(a.fb, a.avatarImg(sv.Username, sv.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
		if !sv.Connected || sv.SittingOut || sv.Folded {
			fillRect(a.fb, ar, color.RGBA{0, 0, 0, 140})
		}
		hats = append(hats, seatHat{sv.Hat, ar.Min.X, ar.Min.Y})
		px, left := ar.Max.X+3, false
		if px+pileChip+20 > ScreenW-2 {
			px, left = ar.Min.X-3-pileChip, true
		}
		a.drawChipPile(px, ar.Max.Y, sv.Stack, left)
		if ps.Button == i {
			bxp := ar.Min.X - 12
			if left || bxp < 2 {
				bxp = ar.Max.X + 3
			}
			a.drawButtonPuck(bxp, ar.Min.Y)
		}
		nameCol := a.A.C("text")
		if me {
			nameCol = a.A.C("gold")
		}
		label := func(y int, s string, c color.RGBA) {
			hw := a.A.Font.Width(s)/2 + 2
			a.text(min(max(x, hw), ScreenW-hw), y, s, c, alignCenter)
		}
		label(ar.Max.Y+4, fitText(a.A.Font, sv.Username, seatW), nameCol)
		if name, c, ok := titleOf(sv.Title); ok {
			label(ar.Max.Y+13, fitText(a.A.Font, name, seatW+16), c)
		}
		if line, lc := a.pokerSeatLine(ps, sv); line != "" {
			label(ar.Max.Y+22, fitText(a.A.Font, line, seatW+20), lc)
		}
		// hole cards over the hat
		cardTop := a.pokerCardTop(cy)
		cx0 := x - (cw+a.A.Theme.CardFanStep+4)/2
		betX := px + pileChip/2
		a.pkGeo[i] = pkGeo{x: x, cy: cy, pile: image.Pt(px+pileChip/2, ar.Max.Y-8), bet: image.Pt(betX, ar.Min.Y-10),
			cards: image.Pt(cx0, cardTop), labelBottom: cardTop - 2}
		for ci, c := range sv.Cards {
			a.drawCardAt(c, fmt.Sprintf("ps%d_%d", i, ci), cx0+ci*(a.A.Theme.CardFanStep+4), cardTop, &fly)
		}
		if sv.HandName != "" && len(sv.Cards) > 0 {
			c := a.A.C("text")
			if sv.Won > 0 {
				c = a.A.C("highlight")
			}
			a.textOutlined(x, cardTop-10, strings.ToUpper(sv.HandName), c, alignCenter)
		} else if me && !sv.Folded {
			if h := poker.Describe(sv.Cards, ps.Board); h != "" {
				hw := a.A.Font.Width(strings.ToUpper(h))/2 + 2
				a.textOutlined(min(max(x, hw), ScreenW-hw), cardTop-10, strings.ToUpper(h), a.A.C("gold"), alignCenter)
			}
		}
		// this round's bet, just above their chips
		if sv.Bet > 0 {
			a.drawSeatBet(px, ar.Min.Y-4, sv.Bet, left)
		}
		if a.hovered(ar) {
			tip := fmt.Sprintf("%s: %s on the table", sv.Username, commas(sv.Stack))
			w := a.A.Font.Width(tip) + 8
			tr := image.Rect(x-w/2, ar.Min.Y-48, x+w/2, ar.Min.Y-36)
			if tr.Min.X < 2 {
				tr = tr.Add(image.Pt(2-tr.Min.X, 0))
			}
			if tr.Max.X > ScreenW-2 {
				tr = tr.Sub(image.Pt(tr.Max.X-ScreenW+2, 0))
			}
			defer func(tr image.Rectangle, tip string) {
				fillRect(a.fb, tr, color.RGBA{0, 0, 0, 220})
				a.text(tr.Min.X+4, tr.Min.Y+2, tip, a.A.C("text"), alignLeft)
			}(tr, tip)
		}
	}
	for _, h := range hats {
		a.drawHat(h.id, h.x, h.y)
	}
	for _, f := range fly {
		blit(a.fb, f.img, f.x, f.y)
	}
	a.drawPokerFX(image.Pt(ScreenW/2, boardY+ch/2))

	a.drawSpeech(st)
	a.drawLogButton(st)
	a.drawTopBar(st)
	a.drawPokerHUD(ps)
	a.drawPokerReveal()
	a.drawPokerChat(ps)
	a.drawChatBar(st)
	a.blocked = updating
	a.drawOverlays()
	if a.overlay == ovNone && a.profileHatOnTop() {
		a.drawHat(a.profile.Hat, 10, L.hudTop+12)
	}
}

// drawButtonPuck draws the dealer button.
func (a *App) drawButtonPuck(x, y int) {
	r := image.Rect(x, y, x+10, y+10)
	fillRect(a.fb, r.Inset(1), color.RGBA{240, 236, 220, 255})
	fillRect(a.fb, image.Rect(r.Min.X+2, r.Min.Y, r.Max.X-2, r.Max.Y), color.RGBA{240, 236, 220, 255})
	fillRect(a.fb, image.Rect(r.Min.X, r.Min.Y+2, r.Max.X, r.Max.Y-2), color.RGBA{240, 236, 220, 255})
	a.A.Font.Draw(a.fb, x+5-a.A.Font.Width("D")/2, y+2, "D", color.RGBA{20, 20, 20, 255})
}

func (a *App) pokerSeatLine(ps *proto.PokerState, s proto.PokerSeat) (string, color.RGBA) {
	if ps.Phase == "ready" && !s.Dale {
		switch {
		case s.DealtIn:
			return "IN", a.A.C("win")
		case s.Stack > 0 && !s.SittingOut:
			return "deciding...", a.A.C("text_dim")
		}
	}
	switch {
	case ps.Phase == "showdown" && s.Won > 0:
		return "WON " + commas(s.Won), a.A.C("win")
	case s.SittingOut:
		return "sitting out", a.A.C("text_dim")
	case s.Folded:
		return "folded", a.A.C("text_dim")
	case s.AllIn:
		return "ALL IN", a.A.C("lose")
	case s.Action != "":
		return strings.ToUpper(s.Action), a.A.C("highlight")
	case s.Stack == 0 && !s.InHand:
		return "out of chips", a.A.C("lose")
	case !s.InHand && ps.Phase != "waiting" && ps.Phase != "showdown" && ps.Phase != "ready":
		return "next hand", a.A.C("text_dim")
	}
	return "", a.A.C("text_dim")
}

func (a *App) drawPokerChat(ps *proto.PokerState) {
	n := len(ps.Seats)
	for seat, b := range a.chats {
		if !b.visible(a.now) {
			delete(a.chats, seat)
			continue
		}
		if seat <= 0 || seat >= n || ps.Seats[seat].Username == "" {
			continue
		}
		x, cy := a.pokerSeatPos(seat, n)
		bottom := a.pokerCardTop(cy) - 4
		if ps.Seats[seat].Bet > 0 {
			bottom -= a.A.Theme.ChipSize
		}
		a.chatBubbleAt(b, x, bottom)
	}
}

// ---------------------------------------------------------------- the panel

func (a *App) drawPokerHUD(ps *proto.PokerState) {
	L := a.L
	hudTop := L.hudTop
	a.panel(image.Rect(0, hudTop, ScreenW, ScreenH))
	me := a.pokerMe()

	// left: you
	ar := image.Rect(10, hudTop+12, 42, hudTop+44)
	a.drawFrame(a.profile.Frame, ar.Min.X, ar.Min.Y)
	blit(a.fb, a.avatarImg(a.username, a.profile.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
	a.text(48, hudTop+7, fitText(a.A.Font, a.username, 110), a.A.C("gold"), alignLeft)
	a.text(48, hudTop+18, "TABLE", a.A.C("text_dim"), alignLeft)
	stack := int64(0)
	if me != nil {
		stack = me.Stack
	}
	a.text(80, hudTop+18, commas(stack), a.A.C("text"), alignLeft)
	a.text(48, hudTop+29, "BANK", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+29, commas(ps.Balance), a.A.C("text"), alignLeft)
	hand := ""
	if me != nil && me.InHand && !me.Folded {
		hand = poker.Describe(me.Cards, ps.Board)
	}
	switch {
	case hand != "":
		a.text(48, hudTop+44, fitText(a.A.Font, strings.ToUpper(hand), 116), a.A.C("highlight"), alignLeft)
	case me == nil:
		a.text(48, hudTop+44, "watching from the rail", a.A.C("text_dim"), alignLeft)
	case me.SittingOut:
		a.text(48, hudTop+44, "sitting out", a.A.C("text_dim"), alignLeft)
	default:
		if name, c, ok := titleOf(a.profile.Title); ok {
			a.text(48, hudTop+44, fitText(a.A.Font, name, 116), c, alignLeft)
		}
	}

	cx0, cx1 := L.cx0, L.cx1
	if L.portrait {
		fillRect(a.fb, image.Rect(6, L.center-3, ScreenW-6, L.center-2), color.RGBA{0, 0, 0, 90})
	}
	top := L.center
	switch {
	case me != nil && len(ps.Actions) > 0:
		a.drawPokerActions(ps, me, cx0, cx1, top)
	default:
		a.drawPokerIdle(ps, me, cx0, cx1, top)
	}

	// right: phase, timer, buttons
	rx0, rx1 := L.rx0, L.rx1
	phase := strings.ToUpper(ps.Phase)
	pc := a.A.C("gold")
	if len(ps.Actions) > 0 {
		phase = "YOUR MOVE"
		if (a.frame/15)%2 == 0 {
			pc = a.A.C("highlight")
		}
	}
	a.text(rx0, hudTop+7, phase, pc, alignLeft)
	if ms, frac := a.remaining(); ps.Deadline != 0 {
		a.text(rx1, hudTop+7, fmt.Sprintf("%ds", (ms+999)/1000), a.A.C("text"), alignRight)
		bar := image.Rect(rx0, hudTop+18, rx1, hudTop+24)
		fillRect(a.fb, bar, color.RGBA{0, 0, 0, 255})
		in := bar.Inset(1)
		c := a.A.C("gold")
		if frac < 0.25 {
			c = a.A.C("lose")
		}
		fillRect(a.fb, image.Rect(in.Min.X, in.Min.Y, in.Min.X+int(float64(in.Dx())*frac), in.Max.Y), c)
	}
	half := (rx1 - rx0 - 4) / 2
	if me != nil {
		label, act := "SIT OUT", "sitout"
		if me.SittingOut {
			label, act = "SIT IN", "sitin"
		}
		if a.button("pk_sit", image.Rect(rx0, hudTop+32, rx0+half, hudTop+50), label, btn{enabled: true}) {
			a.send(proto.CPoker, proto.PokerAct{Action: act})
		}
	}
	if a.button("settings", image.Rect(rx1-half, hudTop+32, rx1, hudTop+50), "SETTINGS", btn{enabled: true, hotkey: 'o'}) {
		a.overlay = ovSettings
	}
}

func (a *App) tableInfo() proto.TableInfo {
	ps := a.ps
	t := proto.TableInfo{ID: ps.Table, Name: ps.Name, Game: proto.GameHoldem, MinBuyIn: ps.MinBuyIn, MaxBuyIn: ps.MaxBuyIn,
		BigBlind: ps.BigBlind, Stakes: fmt.Sprintf("Blinds %s/%s", commas(ps.SmallBlind), commas(ps.BigBlind))}
	for _, lt := range a.lobby.Tables {
		if lt.ID == ps.Table {
			t.Seats, t.Players = lt.Seats, lt.Players
		}
	}
	return t
}

func (a *App) drawPokerIdle(ps *proto.PokerState, me *proto.PokerSeat, x0, x1, top int) {
	msg := ps.Message
	type ib struct {
		id, label string
		fn        func()
		hot       rune
		key       Key
	}
	var btns []ib
	add := func(id, label string, fn func()) { btns = append(btns, ib{id: id, label: label, fn: fn}) }
	between := ps.Phase == "waiting" || ps.Phase == "showdown"
	switch {
	case me == nil:
		free := false
		for _, s := range ps.Seats[1:] {
			if s.Username == "" {
				free = true
			}
		}
		msg = "You're watching. Take a seat to play."
		if !free {
			msg = "The table is full. You can watch until a seat opens."
		} else {
			add("pk_sitdown", "SIT DOWN", func() { a.openBuyIn(a.tableInfo(), false) })
		}
	default:
		switch {
		case me.Stack == 0 && !me.InHand:
			msg = "You're out of chips. Add some to keep playing."
		case me.Folded:
			msg = "You folded. Wait for the next hand."
		case me.InHand && ps.Active > 0 && ps.Active < len(ps.Seats):
			msg = "Waiting for " + ps.Seats[ps.Active].Username + "..."
		case me.InHand && ps.Active == 0:
			msg = "Dale is thinking..."
		case !me.InHand && !between:
			msg = "You'll be dealt in next hand."
		}
		if ps.CanDealIn {
			msg = "New hand starting. Deal yourself in?"
			btns = append(btns, ib{"pk_dealin", "DEAL ME IN", func() { a.send(proto.CPoker, proto.PokerAct{Action: "in"}) }, 'd', KeyEnter})
		} else if ps.Phase == "ready" && me.DealtIn {
			msg = "You're in. Waiting for the others..."
		}
		if ps.Phase == "showdown" && me.InHand && !me.Folded && me.HandName == "" {
			add("pk_show", "SHOW CARDS", func() { a.send(proto.CPoker, proto.PokerAct{Action: "show"}) })
		}
		if (between || !me.InHand) && me.Stack < ps.MaxBuyIn {
			add("pk_rebuy", "ADD CHIPS", func() { a.openBuyIn(a.tableInfo(), true) })
		}
		add("pk_stand", "STAND UP", func() { a.send(proto.CPoker, proto.PokerAct{Action: "stand"}) })
	}
	if msg == "" {
		msg = "Waiting for players..."
	}
	lines := a.wrap(msg, x1-x0-10)
	y := top + 10
	if len(btns) == 0 {
		y = top + 30 - len(lines)*5
	}
	for _, l := range lines {
		a.text((x0+x1)/2, y, l, a.A.C("text"), alignCenter)
		y += 10
	}
	if len(btns) > 0 {
		gap := 4
		w := min(96, (x1-x0-gap*(len(btns)-1))/len(btns))
		total := w*len(btns) + gap*(len(btns)-1)
		bx := (x0+x1)/2 - total/2
		by := top + 34
		for i, b := range btns {
			style := styleRed
			if i == 0 {
				style = styleGold
			}
			if a.button(b.id, image.Rect(bx, by, bx+w, by+18), b.label, btn{enabled: true, style: style, hotkey: b.hot, key: b.key}) {
				b.fn()
			}
			bx += w + gap
		}
	}
}

func (a *App) drawPokerActions(ps *proto.PokerState, me *proto.PokerSeat, x0, x1, top int) {
	can := map[string]bool{}
	for _, act := range ps.Actions {
		can[act] = true
	}
	raiseAct := "raise"
	if can["bet"] {
		raiseAct = "bet"
	}
	canRaise := can["raise"] || can["bet"]
	lo, hi := ps.MinRaise, ps.MaxRaise
	curBet := me.Bet + ps.ToCall
	clamp := func(v int64) int64 { return min(max(v, lo), hi) }
	// round to the big blind, except at the ends
	snap := func(v int64) int64 {
		if bb := ps.BigBlind; bb > 0 && v != hi {
			v = (v + bb/2) / bb * bb
		}
		return clamp(v)
	}

	// row 1: the amount
	y := top + 4
	if canRaise {
		a.pkRaise = clamp(a.pkRaise)
		if a.in.Wheel != 0 && !a.blocked {
			a.pkRaise = snap(a.pkRaise + int64(a.in.Wheel)*max(ps.BigBlind, 1))
		}
		label := "RAISE TO"
		if raiseAct == "bet" {
			label = "BET"
		}
		a.text(x0+2, y+1, label, a.A.C("text_dim"), alignLeft)
		a.text(x0+2, y+10, commas(a.pkRaise), a.A.C("gold"), alignLeft)
		bx := x0 + 62
		presets := []struct {
			id, label string
			v         int64
		}{
			{"min", "MIN", lo},
			{"half", "1/2 POT", snap(curBet + (ps.Pot+ps.ToCall)/2)},
			{"pot", "POT", snap(curBet + ps.Pot + ps.ToCall)},
			{"max", "ALL IN", hi},
		}
		pw := (x1 - bx - 2*18 - 4*3) / len(presets)
		for _, p := range presets {
			if a.button("pk_"+p.id, image.Rect(bx, y, bx+pw, y+16), p.label, btn{enabled: true}) {
				a.pkRaise = p.v
			}
			bx += pw + 3
		}
		if a.button("pk_minus", image.Rect(bx, y, bx+16, y+16), "-", btn{enabled: a.pkRaise > lo, hotkey: '-'}) {
			a.pkRaise = snap(a.pkRaise - max(ps.BigBlind, 1))
		}
		bx += 19
		if a.button("pk_plus", image.Rect(bx, y, bx+16, y+16), "+", btn{enabled: a.pkRaise < hi, hotkey: '+'}) {
			a.pkRaise = snap(a.pkRaise + max(ps.BigBlind, 1))
		}
	} else {
		a.text((x0+x1)/2, y+5, "Pot "+commas(ps.Pot), a.A.C("gold"), alignCenter)
	}

	// row 2: the actions
	y = top + 22
	gap := 4
	w := (x1 - x0 - 2*gap) / 3
	x := x0
	if a.button("pk_fold", image.Rect(x, y, x+w, y+20), "FOLD", btn{enabled: can["fold"], hotkey: 'f'}) {
		a.send(proto.CPoker, proto.PokerAct{Action: "fold"})
	}
	x += w + gap
	mid, midAct := "CHECK", "check"
	if can["call"] {
		mid, midAct = "CALL "+commas(ps.ToCall), "call"
		if ps.ToCall >= me.Stack {
			mid = "CALL ALL IN"
		}
	}
	if a.button("pk_call", image.Rect(x, y, x+w, y+20), mid, btn{enabled: can[midAct], style: styleGold, hotkey: 'c'}) {
		a.send(proto.CPoker, proto.PokerAct{Action: midAct})
	}
	x += w + gap
	rl := "RAISE " + shortChips(a.pkRaise)
	if raiseAct == "bet" {
		rl = "BET " + shortChips(a.pkRaise)
	}
	act := raiseAct
	if canRaise && a.pkRaise >= hi {
		rl, act = "ALL IN "+shortChips(hi), "allin"
	}
	if !canRaise {
		rl = "RAISE"
	}
	if a.button("pk_raise", image.Rect(x, y, x1, y+20), rl, btn{enabled: canRaise, hotkey: 'r'}) {
		a.send(proto.CPoker, proto.PokerAct{Action: act, Amount: a.pkRaise})
	}
	if !a.isTouch() {
		a.text((x0+x1)/2, top+46, "keys: F fold  C check/call  R raise  wheel or +/- amount", a.A.C("text_dim"), alignCenter)
	}
}

// drawSeatBet shows a player's bet this round as a small chip and the
// amount, sitting just above their chip pile.
func (a *App) drawSeatBet(pileX, bottom int, bet int64, leftOfPortrait bool) {
	chip := a.smallChip(min(len(a.A.Theme.ChipValues)-1, max(0, a.A.ChipFor(bet))))
	cs := chip.Rect.Dx()
	label := "BET " + shortChips(bet)
	if !leftOfPortrait {
		blit(a.fb, chip, pileX, bottom-cs)
		a.textOutlined(pileX+cs+2, bottom-cs+2, label, a.A.C("highlight"), alignLeft)
		return
	}
	blit(a.fb, chip, pileX+pileChip-cs, bottom-cs)
	a.textOutlined(pileX+pileChip-cs-2, bottom-cs+2, label, a.A.C("highlight"), alignRight)
}
