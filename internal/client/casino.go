// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"strings"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Baccarat and Red Dog: everyone bets on one shared deal. Players sit around
// the rail like blackjack with their bets in front of them; the middle of the
// felt is the game (Player and Banker hands, or Red Dog's three cards); the
// panel picks a chip and places it.

var csChips = []int64{10, 25, 100, 500, 1000}

// csFX is a result popping up over a seat.
type csFX struct {
	seat  int
	label string
	col   color.RGBA
	start time.Time
}

// csRevealFX is cards held big and face down in the middle, then flipped.
type csRevealFX struct {
	spot      string
	from, n   int
	start     time.Time
	flip      time.Time
	flipped   bool
	flippedAt time.Time
	cards     []bj.Card
	slots     []image.Point
	sound     string
	soundAt   time.Time
	soundOn   bool
	label     string
	labelCol  color.RGBA
}

func casinoShell(cs *proto.CasinoState) *proto.TableState {
	st := &proto.TableState{TableID: cs.Table, TableName: cs.Name, Phase: cs.Phase, HandNumber: cs.HandNumber,
		Deadline: cs.Deadline, TimerTotal: cs.TimerTotal, ServerTime: cs.ServerTime, YourSeat: cs.YourSeat, ActiveSeat: -1,
		Message: cs.Message, Settings: cs.Settings, Spectators: cs.Spectators, DealerSay: cs.DealerSay, DealerSayID: cs.DealerSayID,
		DealerSayUntil: cs.DealerSayUntil, DealerSayCont: cs.DealerSayCont, DealerSayPara: cs.DealerSayPara,
		DealerLog: cs.DealerLog, HouseNet: cs.HouseNet, HouseTier: cs.HouseTier}
	for _, s := range cs.Seats {
		st.Seats = append(st.Seats, proto.SeatView{Seat: s.Seat, Username: s.Username, AvatarVersion: s.AvatarVersion,
			Balance: s.Balance, Connected: s.Connected, Hat: s.Hat, Frame: s.Frame, Title: s.Title, ActiveHand: -1})
	}
	return st
}

func (a *App) csMe() *proto.CasinoSeat {
	cs := a.cs
	if cs == nil || cs.YourSeat <= 0 || cs.YourSeat > len(cs.Seats) {
		return nil
	}
	return &cs.Seats[cs.YourSeat-1]
}

func (a *App) onCasino(cs *proto.CasinoState) {
	prev := a.cs
	a.clockOffset = cs.ServerTime - time.Now().UnixMilli()
	newHand := prev == nil || prev.HandNumber != cs.HandNumber
	// cards fly in from the shoe
	seen := map[string]time.Time{}
	hidden := map[string]bool{}
	vals := map[string]bj.Card{}
	fresh := 0
	each := func(spot string, cards []bj.Card) {
		for i, c := range cards {
			k := fmt.Sprintf("c%s%d", spot, i)
			hidden[k], vals[k] = c.Hidden(), c
			if prev == nil {
				seen[k] = a.now.Add(-time.Hour)
				continue
			}
			if t, ok := a.cardSeen[k]; ok && !newHand {
				seen[k] = t
				if a.cardHidden[k] && !c.Hidden() {
					a.flipAt[k] = a.now
				}
				continue
			}
			seen[k] = a.now.Add(time.Duration(fresh) * dealStagger / 2)
			fresh++
		}
	}
	each("player", cs.PlayerCards)
	each("banker", cs.BankerCards)
	each("reddog", cs.Cards)
	a.cardSeen, a.cardHidden, a.cardVal = seen, hidden, vals

	if prev != nil {
		// chips hitting the felt, and results
		for i, s := range cs.Seats {
			if i >= len(prev.Seats) {
				continue
			}
			var was, now int64
			for _, v := range prev.Seats[i].Bets {
				was += v
			}
			for _, v := range s.Bets {
				now += v
			}
			if now > was && s.Username != "" {
				a.sound("chip")
			}
			if s.Settled && !prev.Seats[i].Settled && s.Username != "" {
				label, col := "PUSH", a.A.C("text")
				switch {
				case s.Result > 0:
					label, col = "+"+shortChips(s.Result), a.A.C("win")
				case s.Result < 0:
					label, col = "-"+shortChips(-s.Result), a.A.C("lose")
				}
				a.csFX = append(a.csFX, csFX{seat: s.Seat, label: label, col: col, start: a.now})
			}
		}
	}
	if cs.CanRaise && (prev == nil || !prev.CanRaise) {
		a.sound("turn")
	}
	if a.chipSel == 0 {
		a.chipSel = max(cs.MinBet, 10)
	}
	shell := casinoShell(cs)
	a.onDealerLines(shell)
	a.cs, a.st = cs, shell
	a.onCasinoReveal(cs)
}

// ---------------------------------------------------------------- reveal

func (a *App) csSpotCards(cs *proto.CasinoState, spot string) []bj.Card {
	switch spot {
	case "player":
		return cs.PlayerCards
	case "banker":
		return cs.BankerCards
	}
	return cs.Cards
}

func (a *App) onCasinoReveal(cs *proto.CasinoState) {
	rv := cs.Reveal
	if rv == nil {
		if r := a.csRev; r != nil && !r.flipped {
			a.csRevealFlipped(cs)
		}
		return
	}
	r := a.csRev
	if r == nil || r.spot != rv.Spot || r.from != rv.From {
		flip := time.UnixMilli(rv.Until - a.clockOffset)
		r = &csRevealFX{spot: rv.Spot, from: rv.From, n: rv.Count, start: a.now, flip: flip}
		left := flip.Sub(a.now)
		n := 0
		for i, l := range SuspenseLengths {
			if time.Duration(l)*100*time.Millisecond <= left+50*time.Millisecond {
				n = i
			}
		}
		l := SuspenseLengths[n]
		r.sound = fmt.Sprintf("suspense_%s_%d", SuspenseStyles[rand.Intn(len(SuspenseStyles))], l)
		r.soundAt = flip.Add(-time.Duration(l) * 100 * time.Millisecond)
		a.csRev = r
	}
	cards := a.csSpotCards(cs, rv.Spot)
	r.cards = r.cards[:0]
	for i := rv.From; i < rv.From+rv.Count && i < len(cards); i++ {
		r.cards = append(r.cards, cards[i])
	}
	if rv.Shown && !r.flipped {
		a.csRevealFlipped(cs)
	}
}

// csRevealFlipped says what the flip did.
func (a *App) csRevealFlipped(cs *proto.CasinoState) {
	r := a.csRev
	r.flipped, r.flippedAt = true, a.now
	good, great := false, false
	switch r.spot {
	case "player", "banker":
		total := cs.PlayerTotal
		name := "PLAYER"
		if r.spot == "banker" {
			total, name = cs.BankerTotal, "BANKER"
		}
		cards := a.csSpotCards(cs, r.spot)
		r.label = fmt.Sprintf("%s %d", name, total)
		r.labelCol = parseHex("#6fa8ff")
		if r.spot == "banker" {
			r.labelCol = parseHex("#ff7a7a")
		}
		if len(cards) == 2 && total >= 8 {
			r.label = fmt.Sprintf("%s NATURAL %d!", name, total)
			great = true
		}
		// is it good for you?
		if me := a.csMe(); me != nil && me.Bets[r.spot] > 0 && total >= 7 {
			good = true
		}
	default:
		if len(cs.Cards) >= 3 && r.from == 2 {
			a1, b1, c1 := rdRank(cs.Cards[0]), rdRank(cs.Cards[1]), rdRank(cs.Cards[2])
			switch {
			case a1 == b1 && c1 == a1:
				r.label, r.labelCol, great = "THREE OF A KIND!", a.A.C("gold"), true
			case a1 == b1:
				r.label, r.labelCol = "NO TRIPS: PUSH", a.A.C("text")
			case c1 > min(a1, b1) && c1 < max(a1, b1):
				r.label, r.labelCol, good = "IN BETWEEN!", a.A.C("win"), true
				if cs.Spread == 1 {
					great = true
				}
			default:
				r.label, r.labelCol = "OUTSIDE", a.A.C("lose")
			}
		} else if len(cs.Cards) >= 2 {
			switch {
			case cs.Spread == -1:
				r.label, r.labelCol = "CONSECUTIVE: PUSH", a.A.C("text")
			case cs.Spread == 0:
				r.label, r.labelCol = "A PAIR!", a.A.C("highlight")
			default:
				r.label, r.labelCol = fmt.Sprintf("SPREAD %d: PAYS %s", cs.Spread, strings.ToUpper(cs.Pays)), a.A.C("gold")
			}
		}
	}
	switch {
	case great:
		a.sound("rev_great")
	case good:
		a.sound("rev_good")
	default:
		a.sound("rev_meh")
	}
}

func rdRank(c bj.Card) int {
	if c.Rank == 1 {
		return 14
	}
	return int(c.Rank)
}

func (a *App) csRevealHides(spot string, i, x, y int) bool {
	r := a.csRev
	if r == nil || r.spot != spot || i < r.from || i >= r.from+r.n {
		return false
	}
	for len(r.slots) <= i-r.from {
		r.slots = append(r.slots, image.Point{})
	}
	r.slots[i-r.from] = image.Pt(x, y)
	return true
}

func (a *App) drawCasinoReveal() {
	r := a.csRev
	if r == nil {
		return
	}
	if !r.soundOn && !a.now.Before(r.soundAt) && !r.flipped {
		r.soundOn = true
		a.sound(r.sound)
	}
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	k := max(2, min(3, (ScreenH-120)/ch))
	for r.n*(cw*k+8) > ScreenW-20 && k > 2 {
		k--
	}
	bw, bh := cw*k, ch*k
	gap := 8
	total := r.n*bw + (r.n-1)*gap
	cx, cy := ScreenW/2, ScreenH/2-10
	if a.L.portrait {
		cy = ScreenH * 2 / 5
	}
	popT := a.now.Sub(r.start).Seconds()
	allDone, dimmed := true, false
	labelY := 0
	for i := 0; i < r.n; i++ {
		tx := float64(cx - total/2 + i*(bw+gap) + bw/2)
		ty := float64(cy)
		hx, hy := tx, ty
		if i < len(r.slots) && r.slots[i] != (image.Point{}) {
			hx, hy = float64(r.slots[i].X+cw/2), float64(r.slots[i].Y+ch/2)
		}
		scale, px, py := 1.0, tx, ty
		if popT < revPopSecs {
			e := easeOut(popT / revPopSecs)
			scale = 1/float64(k) + (1-1/float64(k))*e
			px, py = hx+(tx-hx)*e, hy+(ty-hy)*e
		}
		face, flipK, done := false, 1.0, false
		if r.flipped {
			t := a.now.Sub(r.flippedAt).Seconds() - float64(i)*0.15
			switch {
			case t < 0:
			case t < revFlipSecs:
				p := t / revFlipSecs
				if p < 0.5 {
					flipK = 1 - p*2
				} else {
					face, flipK = true, (p-0.5)*2
				}
			case t < revFlipSecs+revHoldSecs+float64(r.n-1-i)*0.15:
				face = true
			default:
				back := t - revFlipSecs - revHoldSecs - float64(r.n-1-i)*0.15
				if back < revBackSecs {
					face = true
					e := easeOut(back / revBackSecs)
					scale = 1 - (1-1/float64(k))*e
					px, py = tx+(hx-tx)*e, ty+(hy-ty)*e
				} else {
					done = true
				}
			}
		} else if left := r.flip.Sub(a.now).Seconds(); left < 0.6 && left > 0 {
			px += math.Sin(popT*60+float64(i)) * (0.6 - left) * 3
		}
		if done {
			continue
		}
		allDone = false
		if scale > 0.5 && !dimmed {
			fillRect(a.fb, a.fb.Rect, color.RGBA{0, 0, 0, uint8(70 * scale)})
			dimmed = true
		}
		img := a.A.CardBack
		if face && i < len(r.cards) && !r.cards[i].Hidden() {
			img = a.A.CardImage(r.cards[i])
		}
		w := max(1, int(float64(bw)*scale*flipK))
		h := int(float64(bh) * scale)
		dr := image.Rect(int(px)-w/2, int(py)-h/2, int(px)-w/2+w, int(py)-h/2+h)
		if scale > 0.6 {
			fillRect(a.fb, dr.Add(image.Pt(k, k)), color.RGBA{0, 0, 0, 110})
		}
		blitScaled(a.fb, img, dr)
		if face && scale >= 0.99 {
			labelY = max(labelY, dr.Max.Y+6)
		}
	}
	if r.label != "" && labelY > 0 {
		lw := a.A.Font.Width(r.label)*2 + 12
		fillRect(a.fb, image.Rect(cx-lw/2, labelY-3, cx+lw/2, labelY+19), color.RGBA{0, 0, 0, 160})
		a.textBig(cx, labelY, r.label, r.labelCol, 2, alignCenter)
	}
	if allDone && r.flipped {
		a.csRev = nil
	}
}

// ---------------------------------------------------------------- screen

func spotLabel(s string) string {
	switch s {
	case "player":
		return "PLAYER"
	case "banker":
		return "BANKER"
	case "tie":
		return "TIE"
	case "ante":
		return "ANTE"
	case "raise":
		return "RAISE"
	}
	return strings.ToUpper(s)
}

func (a *App) spotColor(s string) color.RGBA {
	switch s {
	case "player":
		return parseHex("#6fa8ff")
	case "banker":
		return parseHex("#ff7a7a")
	case "tie":
		return a.A.C("win")
	}
	return a.A.C("gold")
}

func (a *App) drawCasino() {
	a.chatPreInput()
	a.computeLayout()
	a.drawTableBG()
	L := a.L
	cs, st := a.cs, a.st
	if cs == nil || st == nil {
		a.text(ScreenW/2, ScreenH/2-10, "Joining the table...", a.A.C("text"), alignCenter)
		a.drawOverlays()
		return
	}
	overlayOpen := a.overlay != ovNone
	updating := a.blocked
	a.blocked = overlayOpen || updating
	var fly []flying
	blit(a.fb, a.A.Shoe, L.shoe.X-6, L.shoe.Y-2)
	a.drawDealer(st)
	if cs.Game == proto.GameBaccarat {
		a.drawBaccaratFelt(cs, &fly)
	} else {
		a.drawRedDogFelt(cs, &fly)
	}
	a.drawCasinoSeats(cs)
	for _, f := range fly {
		blit(a.fb, f.img, f.x, f.y)
	}
	a.drawSpeech(st)
	a.drawLogButton(st)
	a.drawTopBar(st)
	a.drawCasinoHUD(cs)
	a.drawCasinoReveal()
	a.drawCasinoChat(cs)
	a.drawChatBar(st)
	a.blocked = updating
	a.drawOverlays()
	if a.overlay == ovNone && a.profileHatOnTop() {
		a.drawHat(a.profile.Hat, 10, L.hudTop+12)
	}
}

// drawBaccaratFelt draws the Player and Banker hands, the tie box and the
// history of recent winners.
func (a *App) drawBaccaratFelt(cs *proto.CasinoState, fly *[]flying) {
	L := a.L
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	boxW := 3*(cw+3) + 16
	boxH := ch + 26
	top := L.cardsY + 2
	gap := 46
	if L.portrait {
		top = L.msgY - 10
		gap = 14
	}
	pr := image.Rect(ScreenW/2-gap/2-boxW, top, ScreenW/2-gap/2, top+boxH)
	br := image.Rect(ScreenW/2+gap/2, top, ScreenW/2+gap/2+boxW, top+boxH)
	draw := func(r image.Rectangle, spot string, cards []bj.Card, total int) {
		col := a.spotColor(spot)
		won := cs.Winner == spot || (cs.Winner == "tie" && spot != "tie")
		bg := color.RGBA{0, 0, 0, 70}
		fillRect(a.fb, r, bg)
		edge := col
		if cs.Winner == spot && (a.frame/12)%2 == 0 {
			edge = a.A.C("highlight")
		}
		strokeRect(a.fb, r, edge)
		if cs.Winner == spot {
			strokeRect(a.fb, r.Inset(-1), edge)
		}
		_ = won
		a.text(r.Min.X+4, r.Min.Y+3, spotLabel(spot), col, alignLeft)
		if len(cards) > 0 {
			a.textOutlined(r.Max.X-4, r.Min.Y+3, fmt.Sprint(total), a.A.C("text"), alignRight)
		}
		x0 := r.Min.X + 8
		for i, c := range cards {
			x, y := x0+i*(cw+3), r.Min.Y+14
			if i == 2 {
				// the third card goes in sideways in real baccarat; here it
				// just sits a little lower
				y += 4
			}
			if a.csRevealHides(spot, i, x, y) {
				continue
			}
			a.drawCardAt(c, fmt.Sprintf("c%s%d", spot, i), x, y, fly)
		}
	}
	draw(pr, "player", cs.PlayerCards, cs.PlayerTotal)
	draw(br, "banker", cs.BankerCards, cs.BankerTotal)
	// tie, between them
	tc := a.spotColor("tie")
	if cs.Winner == "tie" && (a.frame/12)%2 == 0 {
		tc = a.A.C("highlight")
	}
	a.text(ScreenW/2, top+boxH/2-9, "TIE", tc, alignCenter)
	a.text(ScreenW/2, top+boxH/2+1, "8:1", a.A.C("felt_text"), alignCenter)
	// history: a dot per recent hand
	hy := top + boxH + 6
	n := min(len(cs.History), (ScreenW-40)/7)
	hx := ScreenW/2 - n*7/2
	for i, w := range cs.History[len(cs.History)-n:] {
		c := a.spotColor(w)
		r := image.Rect(hx+i*7, hy, hx+i*7+5, hy+5)
		fillRect(a.fb, r, c)
		strokeRect(a.fb, r.Inset(-1), color.RGBA{0, 0, 0, 180})
	}
	msgY := hy + 10
	if cs.Message != "" {
		a.text(ScreenW/2, msgY, fitText(a.A.Font, cs.Message, ScreenW-40), a.A.C("highlight"), alignCenter)
	}
	if !L.portrait {
		rules := fmt.Sprintf("PLAYER 1:1  -  BANKER 1:1 LESS COMMISSION  -  TIE 8:1  -  BETS %s-%s", commas(cs.MinBet), commas(cs.MaxBet))
		a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(rules)/2, msgY+11, fitText(a.A.Font, rules, ScreenW-20), a.A.C("felt_text"))
	}
}

// drawRedDogFelt draws the two spread cards with a slot for the third.
func (a *App) drawRedDogFelt(cs *proto.CasinoState, fly *[]flying) {
	L := a.L
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	top := L.cardsY + 4
	if L.portrait {
		top = L.msgY - 6
	}
	spacing := cw + 26
	xs := []int{ScreenW/2 - spacing - cw/2, ScreenW/2 + spacing - cw/2, ScreenW/2 - cw/2}
	for i, x := range xs {
		if i >= len(cs.Cards) {
			strokeRect(a.fb, image.Rect(x, top, x+cw, top+ch), a.A.C("felt_text"))
		}
	}
	for i, c := range cs.Cards {
		if i > 2 {
			break
		}
		if a.csRevealHides("reddog", i, xs[i], top) {
			continue
		}
		a.drawCardAt(c, fmt.Sprintf("creddog%d", i), xs[i], top, fly)
	}
	// the spread between the first two
	y := top + ch + 6
	if len(cs.Cards) >= 2 && !cs.Cards[0].Hidden() {
		var s string
		col := a.A.C("gold")
		switch {
		case cs.Spread == -1:
			s, col = "CONSECUTIVE: PUSH", a.A.C("text")
		case cs.Spread == 0:
			s, col = "PAIR: THREE OF A KIND PAYS 11 TO 1", a.A.C("highlight")
		default:
			s = fmt.Sprintf("SPREAD %d  -  PAYS %s", cs.Spread, strings.ToUpper(cs.Pays))
		}
		a.textOutlined(ScreenW/2, y, s, col, alignCenter)
	}
	y += 11
	if cs.Message != "" {
		a.text(ScreenW/2, y, fitText(a.A.Font, cs.Message, ScreenW-40), a.A.C("highlight"), alignCenter)
	}
	if !L.portrait {
		rules := "SPREAD 1 PAYS 5:1  -  2 PAYS 4:1  -  3 PAYS 2:1  -  4+ PAYS 1:1  -  TRIPS 11:1"
		a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(rules)/2, y+11, rules, a.A.C("felt_text"))
	}
}

func (a *App) drawCasinoSeats(cs *proto.CasinoState) {
	n := len(cs.Seats)
	seatW := a.seatSpacing(n)
	type seatHat struct {
		id   string
		x, y int
	}
	var hats []seatHat
	for i, sv := range cs.Seats {
		x, cy := a.seatPos(i, n)
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
		me := sv.Seat == cs.YourSeat
		a.drawFrame(sv.Frame, ar.Min.X, ar.Min.Y)
		blit(a.fb, a.avatarImg(sv.Username, sv.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
		if !sv.Connected {
			fillRect(a.fb, ar, color.RGBA{0, 0, 0, 140})
		}
		hats = append(hats, seatHat{sv.Hat, ar.Min.X, ar.Min.Y})
		px, left := ar.Max.X+3, false
		if px+pileChip+20 > ScreenW-2 {
			px, left = ar.Min.X-3-pileChip, true
		}
		a.drawChipPile(px, ar.Max.Y, sv.Balance, left)
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
		// bets, stacked above the hat
		by := cy - 16 - a.A.Theme.HatHeight + a.A.Theme.HatOverlap - 4
		for _, spot := range append(append([]string{}, cs.Spots...), "raise") {
			v := sv.Bets[spot]
			if v == 0 {
				continue
			}
			chip := a.smallChip(min(len(a.A.Theme.ChipValues)-1, max(0, a.A.ChipFor(v))))
			cs2 := chip.Rect.Dx()
			txt := spotLabel(spot)[:1] + " " + shortChips(v)
			if len(cs.Spots) == 1 {
				txt = spotLabel(spot) + " " + shortChips(v)
			}
			w := cs2 + 2 + a.A.Font.Width(txt)
			bx := min(max(x-w/2, 2), ScreenW-w-2)
			blit(a.fb, chip, bx, by-cs2)
			a.textOutlined(bx+cs2+2, by-cs2+2, txt, a.spotColor(spot), alignLeft)
			by -= cs2 + 1
		}
	}
	for _, h := range hats {
		a.drawHat(h.id, h.x, h.y)
	}
	// results popping up
	out := a.csFX[:0]
	for _, f := range a.csFX {
		t := a.now.Sub(f.start).Seconds()
		if t > 2.2 || f.seat < 1 || f.seat > n {
			continue
		}
		out = append(out, f)
		x, cy := a.seatPos(f.seat-1, n)
		y := cy - 50 - int(math.Min(t, 1)*10)
		scale := 2
		if t > 1.6 && (a.frame/3)%2 == 0 {
			continue
		}
		w := a.A.Font.Width(f.label)*scale + 8
		xx := min(max(x, w/2+2), ScreenW-w/2-2)
		r := image.Rect(xx-w/2, y, xx+w/2, y+22)
		fillRect(a.fb, r, color.RGBA{0, 0, 0, 170})
		strokeRect(a.fb, r, f.col)
		a.textBig(xx, y+2, f.label, f.col, scale, alignCenter)
	}
	a.csFX = out
}

func (a *App) drawCasinoChat(cs *proto.CasinoState) {
	n := len(cs.Seats)
	for seat, b := range a.chats {
		if !b.visible(a.now) {
			delete(a.chats, seat)
			continue
		}
		if seat < 1 || seat > n || cs.Seats[seat-1].Username == "" {
			continue
		}
		x, cy := a.seatPos(seat-1, n)
		a.chatBubbleAt(b, x, cy-16-a.A.Theme.HatHeight+a.A.Theme.HatOverlap-30)
	}
}

func (a *App) drawCasinoHUD(cs *proto.CasinoState) {
	L := a.L
	hudTop := L.hudTop
	a.panel(image.Rect(0, hudTop, ScreenW, ScreenH))
	me := a.csMe()
	ar := image.Rect(10, hudTop+12, 42, hudTop+44)
	a.drawFrame(a.profile.Frame, ar.Min.X, ar.Min.Y)
	blit(a.fb, a.avatarImg(a.username, a.profile.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
	a.text(48, hudTop+7, fitText(a.A.Font, a.username, 110), a.A.C("gold"), alignLeft)
	bank := a.profile.Balance
	var bet int64
	if me != nil {
		bank = me.Balance
		for _, v := range me.Bets {
			bet += v
		}
	}
	a.text(48, hudTop+18, "CHIPS", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+18, commas(bank), a.A.C("text"), alignLeft)
	a.text(48, hudTop+29, "BET", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+29, commas(bet), a.A.C("text"), alignLeft)
	if me == nil {
		a.text(48, hudTop+44, "watching", a.A.C("text_dim"), alignLeft)
	} else if name, c, ok := titleOf(a.profile.Title); ok {
		a.text(48, hudTop+44, fitText(a.A.Font, name, 116), c, alignLeft)
	}

	x0, x1 := L.cx0, L.cx1
	top := L.center
	if L.portrait {
		fillRect(a.fb, image.Rect(6, top-3, ScreenW-6, top-2), color.RGBA{0, 0, 0, 90})
	}
	switch {
	case me == nil:
		free := false
		for _, s := range cs.Seats {
			free = free || s.Username == ""
		}
		msg := "The table is full. You can watch until a seat opens."
		if free {
			msg = "You're watching. Take a seat to play."
			bw := min(120, x1-x0)
			if a.button("cs_sit", image.Rect((x0+x1)/2-bw/2, top+28, (x0+x1)/2+bw/2, top+46), "SIT DOWN", btn{enabled: true, style: styleGold}) {
				a.send(proto.CCasino, proto.CasinoAct{Action: "sit"})
			}
		}
		a.text((x0+x1)/2, top+12, msg, a.A.C("text"), alignCenter)
	case cs.CanRaise:
		ante := me.Bets["ante"]
		a.text((x0+x1)/2, top+6, fmt.Sprintf("Spread pays %s. Double your bet?", cs.Pays), a.A.C("text"), alignCenter)
		w := (x1 - x0 - 6) / 2
		if a.button("cs_raise", image.Rect(x0, top+20, x0+w, top+42), "RAISE +"+shortChips(ante), btn{enabled: ante <= bank, style: styleGold, hotkey: 'r'}) {
			a.send(proto.CCasino, proto.CasinoAct{Action: "raise"})
		}
		if a.button("cs_stay", image.Rect(x1-w, top+20, x1, top+42), "STAY", btn{enabled: true, hotkey: 's'}) {
			a.send(proto.CCasino, proto.CasinoAct{Action: "stay"})
		}
	case cs.CanBet:
		a.drawCasinoBetting(cs, me, x0, x1, top, bank)
	default:
		msg := cs.Message
		switch {
		case cs.Phase == "raise" && me.Raised:
			msg = "You doubled your bet. Waiting for the others..."
		case cs.Phase == "raise":
			msg = "Waiting for the others to decide..."
		case msg == "":
			msg = "Good luck."
		}
		lines := a.wrap(msg, x1-x0-10)
		y := top + 28 - len(lines)*5
		for _, l := range lines {
			a.text((x0+x1)/2, y, l, a.A.C("text"), alignCenter)
			y += 10
		}
	}

	rx0, rx1 := L.rx0, L.rx1
	phase := map[string]string{"betting": "PLACE BETS", "dealing": "DEALING", "raise": "RAISE?", "result": "RESULT"}[cs.Phase]
	pc := a.A.C("gold")
	if cs.CanRaise && (a.frame/15)%2 == 0 {
		pc = a.A.C("highlight")
	}
	a.text(rx0, hudTop+7, phase, pc, alignLeft)
	if ms, frac := a.remaining(); cs.Deadline != 0 {
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
		if a.button("cs_stand", image.Rect(rx0, hudTop+32, rx0+half, hudTop+50), "STAND", btn{enabled: cs.Phase == "betting" && bet == 0}) {
			a.send(proto.CCasino, proto.CasinoAct{Action: "stand"})
		}
	}
	if a.button("settings", image.Rect(rx1-half, hudTop+32, rx1, hudTop+50), "SETTINGS", btn{enabled: true, hotkey: 'o'}) {
		a.overlay = ovSettings
	}
}

// drawCasinoBetting is the chip picker and the spots to put chips on.
func (a *App) drawCasinoBetting(cs *proto.CasinoState, me *proto.CasinoSeat, x0, x1, top int, bank int64) {
	// row 1: chips
	n := len(csChips)
	cw := min(36, (x1-x0-4*(n-1))/n)
	cx := (x0+x1)/2 - (cw*n+4*(n-1))/2
	for i, v := range csChips {
		r := image.Rect(cx+i*(cw+4), top+2, cx+i*(cw+4)+cw, top+18)
		style := styleRed
		if a.chipSel == v {
			style = styleGold
		}
		if a.button(fmt.Sprintf("cs_chip%d", v), r, chipLabel(v), btn{enabled: v <= max(bank, cs.MinBet) || a.chipSel == v, style: style, hotkey: rune('1' + i)}) {
			a.chipSel = v
		}
	}
	// row 2: the spots (click to add the chip), clear and rebet
	spots := cs.Spots
	extra := 2
	gap := 4
	w := (x1 - x0 - gap*(len(spots)+extra-1)) / (len(spots) + extra)
	x := x0
	for i, s := range spots {
		label := spotLabel(s)
		if v := me.Bets[s]; v > 0 {
			label += " " + shortChips(v)
		}
		hot := []rune{'p', 't', 'b'}
		var hk rune
		if len(spots) == 3 {
			hk = hot[i]
		} else {
			hk = 'a'
		}
		if a.button("cs_spot_"+s, image.Rect(x, top+22, x+w, top+42), label, btn{enabled: a.chipSel <= bank, style: styleGold, hotkey: hk}) {
			a.send(proto.CCasino, proto.CasinoAct{Action: "bet", Spot: s, Amount: a.chipSel})
		}
		x += w + gap
	}
	if a.button("cs_clear", image.Rect(x, top+22, x+w, top+42), "CLEAR", btn{enabled: len(me.Bets) > 0, hotkey: 'c'}) {
		a.send(proto.CCasino, proto.CasinoAct{Action: "clear"})
	}
	x += w + gap
	if a.button("cs_rebet", image.Rect(x, top+22, x1, top+42), "REBET", btn{enabled: cs.CanRebet, hotkey: 'e'}) {
		a.send(proto.CCasino, proto.CasinoAct{Action: "rebet"})
	}
	hint := "Pick a chip, then click where to bet."
	if cs.Deadline == 0 {
		hint = "The first bet starts the clock."
	}
	a.text((x0+x1)/2, top+46, hint, a.A.C("text_dim"), alignCenter)
}
