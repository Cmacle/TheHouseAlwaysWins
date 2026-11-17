package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

const (
	hudTop    = 298
	animSecs  = 0.30
	flipSecs  = 0.22
	shoeX     = 588
	shoeY     = 24
	dealerTop = 58
)

// ---------------------------------------------------------------- state updates

func cardKeys(st *proto.TableState, fn func(key string, c bj.Card)) {
	for i, c := range st.Dealer.Cards {
		fn(fmt.Sprintf("d%d", i), c)
	}
	for _, s := range st.Seats {
		for h, hv := range s.Hands {
			for i, c := range hv.Cards {
				fn(fmt.Sprintf("s%d_%d_%d", s.Seat, h, i), c)
			}
		}
	}
}

func (a *App) onState(st *proto.TableState) {
	prev := a.st
	a.clockOffset = st.ServerTime - a.now.UnixMilli()
	seen := map[string]time.Time{}
	hidden := map[string]bool{}
	past := a.now.Add(-time.Hour)
	cardKeys(st, func(k string, c bj.Card) {
		hidden[k] = c.Hidden()
		if prev == nil {
			seen[k] = past
			return
		}
		if t, ok := a.cardSeen[k]; ok {
			seen[k] = t
			if a.cardHidden[k] && !c.Hidden() {
				a.flipAt[k] = a.now
			}
		} else {
			seen[k] = a.now
		}
	})
	a.cardSeen, a.cardHidden = seen, hidden
	if st.DealerSayID != 0 && st.DealerSayID != a.sayID && st.DealerSay != "" {
		a.sayID = st.DealerSayID
		a.sayAt = a.now
		a.sound("talk")
	}
	if st.Phase == proto.PhaseBetting && (prev == nil || prev.Phase != proto.PhaseBetting) {
		a.betDraft = a.lastBet
		if me := a.mySeat(st); me != nil && a.betDraft > me.Balance {
			a.betDraft = 0
		}
	}
	a.st = st
}

func (a *App) mySeat(st *proto.TableState) *proto.SeatView {
	if st == nil || st.YourSeat < 0 || st.YourSeat >= len(st.Seats) {
		return nil
	}
	return &st.Seats[st.YourSeat]
}

func (a *App) onEvent(e proto.Event) {
	mine := a.st != nil && e.Seat >= 0 && e.Seat == a.st.YourSeat
	switch e.Kind {
	case "card":
		a.sound("card")
	case "chip":
		a.sound("chip")
	case "shuffle":
		a.sound("shuffle")
		a.toast(e.Text, a.A.C("text"), 2.5, 1)
	case "turn":
		a.sound("turn")
		a.toast(e.Text, a.A.C("highlight"), 1.6, 2)
	case "info":
		a.toast(e.Text, a.A.C("text"), 4, 1)
	case "unlock":
		a.sound("blackjack")
		c := a.A.C("gold")
		if it, ok := cosmetics.Get(e.Item); ok {
			c = tierColor(it.Tier)
		}
		a.toast(e.Text, c, 5, 2)
		a.toast("Open PROFILE > WARDROBE to wear it", a.A.C("text"), 5, 1)
	case "crown":
		a.sound("refill")
		a.toast(e.Text, a.A.C("gold"), 4, 1)
	case "refill":
		a.sound("refill")
		a.toast(e.Text, a.A.C("win"), 5, 1)
	case "bust":
		a.sound("bust")
	case "blackjack_dealt":
		if mine {
			a.sound("blackjack")
			a.toast("BLACKJACK!", a.A.C("gold"), 2.5, 2)
		}
	case "win", "lose", "push", "blackjack":
		a.sound(e.Kind)
		a.resultAt, a.resultDelta, a.resultKind = a.now, e.Delta, e.Kind
		switch e.Kind {
		case "win":
			a.toast("YOU WIN "+signed(e.Delta), a.A.C("win"), 3, 2)
		case "blackjack":
			a.toast("BLACKJACK! "+signed(e.Delta), a.A.C("gold"), 3.5, 2)
		case "lose":
			a.toast("YOU LOSE "+signed(e.Delta), a.A.C("lose"), 3, 2)
		default:
			a.toast("PUSH", a.A.C("push"), 2.5, 2)
		}
	}
}

// ---------------------------------------------------------------- layout

func (a *App) seatPos(i, n int) (x, avatarCY int) {
	if n <= 1 {
		x = ScreenW / 2
	} else {
		x = 46 + i*548/(n-1)
	}
	dx := (float64(x) - 320) / 420
	rail := 40 + 222*math.Sqrt(math.Max(0, 1-dx*dx))
	return x, int(rail) - 10
}

type cardPos struct {
	hand, idx int
	x, y      int
}

func (a *App) handLayout(s proto.SeatView, cx, cardTop, seatMaxW int) []cardPos {
	cw := a.A.Theme.CardWidth
	step := a.A.Theme.CardFanStep
	type hl struct{ step, w int }
	var hs []hl
	total := 0
	for _, h := range s.Hands {
		n := len(h.Cards)
		st := step
		if n > 1 && cw+st*(n-1) > seatMaxW {
			st = (seatMaxW - cw) / (n - 1)
			if st < 3 {
				st = 3
			}
		}
		w := cw
		if n > 1 {
			w += st * (n - 1)
		}
		hs = append(hs, hl{st, w})
		total += w
	}
	if len(hs) == 0 {
		return nil
	}
	gap := 4
	total += gap * (len(hs) - 1)
	var xs []int
	if total <= seatMaxW+4 {
		x := cx - total/2
		for _, h := range hs {
			xs = append(xs, x)
			x += h.w + gap
		}
	} else {
		last := hs[len(hs)-1].w
		off := (seatMaxW + 4 - last) / (len(hs) - 1)
		x := cx - (seatMaxW+4)/2
		for i := range hs {
			xs = append(xs, x+i*off)
		}
	}
	var out []cardPos
	for hi, h := range s.Hands {
		y := cardTop
		if hi == s.ActiveHand && len(s.Hands) > 1 {
			y -= 4
		}
		for ci := range h.Cards {
			out = append(out, cardPos{hand: hi, idx: ci, x: xs[hi] + ci*hs[hi].step, y: y})
		}
	}
	return out
}

func (a *App) remaining() (ms int64, frac float64) {
	if a.st == nil || a.st.Deadline == 0 || a.st.TimerTotal <= 0 {
		return 0, 0
	}
	ms = a.st.Deadline - (a.now.UnixMilli() + a.clockOffset)
	if ms < 0 {
		ms = 0
	}
	frac = float64(ms) / float64(a.st.TimerTotal)
	if frac > 1 {
		frac = 1
	}
	return ms, frac
}

// ---------------------------------------------------------------- drawing

type flying struct {
	img  *image.RGBA
	x, y int
}

func easeOut(t float64) float64 { return 1 - math.Pow(1-t, 3) }

func (a *App) drawCardAt(c bj.Card, key string, x, y int, fly *[]flying) {
	img := a.A.CardImage(c)
	if start, ok := a.cardSeen[key]; ok {
		t := a.now.Sub(start).Seconds() / animSecs
		if t < 1 {
			e := easeOut(math.Max(0, t))
			fx := float64(shoeX) + (float64(x)-shoeX)*e
			fy := float64(shoeY) + (float64(y)-shoeY)*e
			back := a.A.CardBack
			if t > 0.6 {
				back = img
			}
			*fly = append(*fly, flying{back, int(fx), int(fy)})
			return
		}
	}
	if ft, ok := a.flipAt[key]; ok {
		t := a.now.Sub(ft).Seconds() / flipSecs
		if t < 1 {
			src := a.A.CardBack
			k := 1 - t*2
			if t >= 0.5 {
				src = img
				k = (t - 0.5) * 2
			}
			w := int(float64(img.Rect.Dx()) * k)
			if w < 1 {
				w = 1
			}
			cx := x + img.Rect.Dx()/2
			blitScaled(a.fb, src, image.Rect(cx-w/2, y, cx-w/2+w, y+img.Rect.Dy()))
			return
		}
		delete(a.flipAt, key)
	}
	// soft shadow
	fillRect(a.fb, image.Rect(x+1, y+img.Rect.Dy(), x+img.Rect.Dx()-1, y+img.Rect.Dy()+1), color.RGBA{0, 0, 0, 90})
	blit(a.fb, img, x, y)
}

func totalLabel(total int, soft, natural bool, n int) string {
	switch {
	case natural:
		return "BJ"
	case total > 21:
		return "BUST"
	case soft && total < 21:
		return fmt.Sprintf("%d/%d", total-10, total)
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprint(total)
}

func (a *App) resultLabel(h proto.HandView) (string, color.RGBA) {
	switch h.Result {
	case "win":
		return "WIN", a.A.C("win")
	case "blackjack":
		return "BJ!", a.A.C("gold")
	case "push":
		return "PUSH", a.A.C("push")
	case "bust":
		return "BUST", a.A.C("lose")
	case "surrender":
		return "SURR", a.A.C("text_dim")
	case "lose":
		return "LOSE", a.A.C("lose")
	}
	return "", a.A.C("text")
}

func (a *App) drawTable() {
	blit(a.fb, a.A.Table, 0, 0)
	st := a.st
	if st == nil {
		a.text(ScreenW/2, 170, "Joining the table...", a.A.C("text"), alignCenter)
		return
	}
	overlayOpen := a.overlay != ovNone
	a.blocked = overlayOpen
	var fly []flying

	// dealer
	blit(a.fb, a.A.Shoe, shoeX-6, shoeY-2)
	a.drawDealer(st)
	if n := len(st.Dealer.Cards); n > 0 {
		cw := a.A.Theme.CardWidth
		step := 14
		w := cw + step*(n-1)
		x0 := ScreenW/2 - w/2
		for i, c := range st.Dealer.Cards {
			a.drawCardAt(c, fmt.Sprintf("d%d", i), x0+i*step, dealerTop, &fly)
		}
		lbl := totalLabel(st.Dealer.Total, st.Dealer.Soft, false, n)
		if bj.IsBlackjack(st.Dealer.Cards) {
			lbl = "BJ"
		}
		if lbl != "" {
			col := a.A.C("text")
			if st.Dealer.Total > 21 {
				col = a.A.C("lose")
			}
			a.text(x0-6, dealerTop+12, lbl, col, alignRight)
		}
	}

	// felt text
	s := st.Settings
	if st.Message != "" {
		a.text(ScreenW/2, 96, st.Message, a.A.C("highlight"), alignCenter)
	}
	rule := "DEALER STANDS ON ALL 17s"
	if s.DealerHitsSoft17 {
		rule = "DEALER HITS SOFT 17"
	}
	felt := a.A.C("felt_text")
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(rule)/2, 110, rule, felt)
	mm := fmt.Sprintf("MIN %s  -  MAX %s", commas(s.MinBet), commas(s.MaxBet))
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(mm)/2, 120, mm, felt)
	pays := "BLACKJACK PAYS " + strings.ReplaceAll(s.BlackjackPays, ":", " TO ")
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(pays)/2, 133, pays, a.A.C("gold"))

	// seats
	n := len(st.Seats)
	seatW := 60
	if n > 1 {
		if sp := 548/(n-1) - 2; sp < seatW {
			seatW = sp
		}
	}
	if seatW < a.A.Theme.CardWidth+4 {
		seatW = a.A.Theme.CardWidth + 4
	}
	blink := (a.frame/20)%2 == 0
	type seatHat struct {
		id   string
		x, y int
	}
	var hats []seatHat
	for i, sv := range st.Seats {
		x, cy := a.seatPos(i, n)
		ar := image.Rect(x-16, cy-16, x+16, cy+16)
		if sv.Username == "" {
			// open seat
			for k := 0; k < 64; k++ {
				ang := float64(k) / 64 * 2 * math.Pi
				px := x + int(math.Round(14*math.Cos(ang)))
				py := cy + int(math.Round(14*math.Sin(ang)))
				fillRect(a.fb, image.Rect(px, py, px+1, py+1), a.A.C("seat_open"))
			}
			a.A.Font.Draw(a.fb, x-a.A.Font.Width("OPEN")/2, cy-3, "OPEN", a.A.C("seat_open"))
			continue
		}
		me := i == st.YourSeat
		active := st.ActiveSeat == i || (st.Phase == proto.PhasePlaying && sv.ActiveHand >= 0)
		// avatar
		frame := a.A.C("shadow")
		if active && blink {
			frame = a.A.C("highlight")
		}
		a.drawFrame(sv.Frame, ar.Min.X, ar.Min.Y)
		if frame != a.A.C("shadow") {
			strokeRect(a.fb, ar.Inset(-3), frame)
		}
		blit(a.fb, a.avatarImg(sv.Username, sv.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
		if !sv.Connected || sv.Status == "sitting_out" {
			fillRect(a.fb, ar, color.RGBA{0, 0, 0, 140})
		}
		hats = append(hats, seatHat{sv.Hat, ar.Min.X, ar.Min.Y})
		// name + status
		nameCol := a.A.C("text")
		if me {
			nameCol = a.A.C("gold")
		}
		a.text(x, ar.Max.Y+4, fitText(a.A.Font, sv.Username, seatW), nameCol, alignCenter)
		if name, c, ok := titleOf(sv.Title); ok {
			a.text(x, ar.Max.Y+13, fitText(a.A.Font, name, seatW+16), c, alignCenter)
		}
		if line, lc := a.seatLine(st, sv); line != "" {
			a.text(x, ar.Max.Y+22, line, lc, alignCenter)
		}
		// hover tooltip with balance
		if a.hovered(ar) {
			tip := fmt.Sprintf("%s: %s chips", sv.Username, commas(sv.Balance))
			if name, _, ok := titleOf(sv.Title); ok {
				tip = fmt.Sprintf("%s, %s: %s chips", sv.Username, name, commas(sv.Balance))
			}
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
		// pending bet chip on the felt
		cardTop := ar.Min.Y - 4 - a.A.Theme.HatHeight + a.A.Theme.HatOverlap - a.A.Theme.CardHeight + 6
		if len(sv.Hands) == 0 && sv.PendingBet > 0 {
			chip := a.A.ChipImage(a.A.ChipFor(sv.PendingBet))
			cs := chip.Rect.Dx()
			blit(a.fb, chip, x-cs/2, cardTop+12)
			blit(a.fb, chip, x-cs/2, cardTop+10)
		}
		// hands
		pos := a.handLayout(sv, x, cardTop, seatW)
		for _, p := range pos {
			if p.hand == sv.ActiveHand {
				continue // draw active hand last
			}
			a.drawCardAt(sv.Hands[p.hand].Cards[p.idx], fmt.Sprintf("s%d_%d_%d", i, p.hand, p.idx), p.x, p.y, &fly)
		}
		for _, p := range pos {
			if p.hand == sv.ActiveHand {
				a.drawCardAt(sv.Hands[p.hand].Cards[p.idx], fmt.Sprintf("s%d_%d_%d", i, p.hand, p.idx), p.x, p.y, &fly)
			}
		}
		// hand labels
		for hi, h := range sv.Hands {
			var hx, hy, last int
			found := false
			for _, p := range pos {
				if p.hand == hi {
					if !found {
						hx, hy = p.x, p.y
						found = true
					}
					last = p.x
				}
			}
			if !found {
				continue
			}
			mid := (hx + last + a.A.Theme.CardWidth) / 2
			lbl, col := a.resultLabel(h)
			if lbl == "" {
				if len(h.Cards) > 0 && h.Cards[0].Hidden() {
					lbl = "?"
				} else {
					lbl = totalLabel(h.Total, h.Soft, h.Natural, len(h.Cards))
				}
				col = a.A.C("text")
				if h.Total > 21 {
					col = a.A.C("lose")
				}
				if h.Doubled {
					lbl += " x2"
				}
			}
			if hi == sv.ActiveHand && st.Phase == proto.PhasePlaying {
				aw := a.A.Arrow.Rect.Dx()
				bob := int(math.Round(math.Sin(float64(a.frame)/6) * 1.5))
				blit(a.fb, a.A.Arrow, mid-aw/2, hy-18+bob)
			}
			if lbl != "" {
				a.text(mid, hy-9, lbl, col, alignCenter)
			}
		}
	}
	for _, h := range hats {
		a.drawHat(h.id, h.x, h.y)
	}
	for _, f := range fly {
		blit(a.fb, f.img, f.x, f.y)
	}

	a.drawSpeech(st)
	a.drawTopBar(st)
	a.drawHUD(st)
	a.blocked = false

	switch a.overlay {
	case ovLeaderboard:
		a.drawLeaderboard()
	case ovProfile:
		a.drawProfile()
	case ovHelp:
		a.drawHelp()
	case ovWardrobe:
		a.drawWardrobe()
	}
	if a.overlay == ovNone && a.profileHatOnTop() {
		a.drawHat(a.profile.Hat, 10, hudTop+12)
	}
}

func (a *App) seatLine(st *proto.TableState, sv proto.SeatView) (string, color.RGBA) {
	if st.Phase == proto.PhaseResults && len(sv.Hands) > 0 {
		var d int64
		for _, h := range sv.Hands {
			d += h.Payout - h.Bet
		}
		switch {
		case d > 0:
			return signed(d), a.A.C("win")
		case d < 0:
			return signed(d), a.A.C("lose")
		}
		return "PUSH", a.A.C("push")
	}
	if len(sv.Hands) > 0 {
		var b int64
		for _, h := range sv.Hands {
			b += h.Bet
		}
		return "BET " + commas(b), a.A.C("gold")
	}
	switch sv.Status {
	case "bet_placed":
		return "BET " + commas(sv.PendingBet), a.A.C("gold")
	case "betting":
		return "betting...", a.A.C("text_dim")
	case "sitting_out":
		return "sitting out", a.A.C("text_dim")
	case "away":
		return "away", a.A.C("text_dim")
	case "broke":
		return "out of chips", a.A.C("lose")
	case "waiting":
		if st.Phase == proto.PhaseBetting || st.Phase == proto.PhaseWaiting {
			return "", a.A.C("text_dim")
		}
		return "next hand", a.A.C("text_dim")
	}
	return "", a.A.C("text_dim")
}

func (a *App) drawTopBar(st *proto.TableState) {
	seated := 0
	for _, s := range st.Seats {
		if s.Username != "" {
			seated++
		}
	}
	name := st.Settings.ServerName
	if name == "" {
		name = a.serverInfo.Name
	}
	a.text(6, 4, fitText(a.A.Font, name, 200), a.A.C("gold"), alignLeft)
	info := fmt.Sprintf("Seats %d/%d", seated, len(st.Seats))
	if st.HandNumber > 0 {
		info += fmt.Sprintf("  -  Hand #%d", st.HandNumber)
	}
	if st.Spectators > 0 {
		info += fmt.Sprintf("  -  %d watching", st.Spectators)
	}
	info += "  -  House " + signedMoney(st.HouseNet)
	a.text(6, 14, info, a.A.C("text_dim"), alignLeft)

	y := 4
	x := ScreenW - 4
	type tb struct {
		id, label string
		hot       rune
		fn        func()
	}
	btns := []tb{
		{"logout", "LOG OUT", 0, a.logout},
		{"help", "?", '?', func() { a.overlay = ovHelp }},
		{"profile", "PROFILE", 0, func() { a.overlay = ovProfile }},
		{"lb", "LEADERBOARD", 'l', a.openLeaderboard},
	}
	for _, b := range btns {
		w := a.A.Font.Width(b.label) + 14
		r := image.Rect(x-w, y, x, y+16)
		if a.button(b.id, r, b.label, btn{enabled: true, hotkey: b.hot}) {
			b.fn()
		}
		x -= w + 3
	}
	if a.in.key(KeyF1) && !a.blocked {
		a.overlay = ovHelp
	}
}

func (a *App) logout() {
	a.send(proto.CLogout, nil)
	a.Cfg.Token = ""
	a.Cfg.Save()
	a.kicked = true
	if a.net != nil {
		a.net.Close()
		a.net = nil
	}
	a.screen = scrLogin
	a.loginMode = modeLogin
	a.st = nil
	a.focusID = "pass"
	a.setStatus("Logged out.", false)
}

func (a *App) profileHatOnTop() bool { return a.profile.Hat != "" }

func (a *App) openLeaderboard() {
	a.overlay = ovLeaderboard
	a.lbScroll = 0
	if a.lbSort == "" {
		a.lbSort = "balance"
	}
	a.requestLeaderboard()
}

func (a *App) requestLeaderboard() {
	a.lbAt = a.now
	a.send(proto.CLeaderboard, proto.LeaderboardReq{Sort: a.lbSort})
}

func chipLabel(v int64) string {
	switch {
	case v >= 1000000 && v%1000000 == 0:
		return fmt.Sprintf("%dM", v/1000000)
	case v >= 1000 && v%1000 == 0:
		return fmt.Sprintf("%dK", v/1000)
	}
	return fmt.Sprint(v)
}

func (a *App) drawHUD(st *proto.TableState) {
	hud := image.Rect(0, hudTop, ScreenW, ScreenH)
	a.panel(hud)
	me := a.mySeat(st)

	// left: you
	ar := image.Rect(10, hudTop+12, 42, hudTop+44)
	a.drawFrame(a.profile.Frame, ar.Min.X, ar.Min.Y)
	blit(a.fb, a.avatarImg(a.username, a.profile.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
	a.text(48, hudTop+7, fitText(a.A.Font, a.username, 110), a.A.C("gold"), alignLeft)
	bal := a.profile.Balance
	if me != nil {
		bal = me.Balance
	}
	a.text(48, hudTop+18, "CHIPS", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+18, commas(bal), a.A.C("text"), alignLeft)
	netCol := a.A.C("text")
	if a.profile.Net > 0 {
		netCol = a.A.C("win")
	} else if a.profile.Net < 0 {
		netCol = a.A.C("lose")
	}
	a.text(48, hudTop+29, "NET", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+29, signed(a.profile.Net), netCol, alignLeft)
	if me == nil {
		a.text(48, hudTop+44, "watching from the rail", a.A.C("text_dim"), alignLeft)
	} else if me.Status == "sitting_out" {
		a.text(48, hudTop+44, "sitting out", a.A.C("text_dim"), alignLeft)
	} else if name, c, ok := titleOf(a.profile.Title); ok {
		a.text(48, hudTop+44, fitText(a.A.Font, name, 116), c, alignLeft)
	}

	// center
	cx0, cx1 := 168, 476
	switch {
	case st.CanBet && me != nil:
		a.drawBetting(st, me, cx0, cx1)
	case len(st.Actions) > 0 && me != nil:
		a.drawActions(st, me, cx0, cx1)
	default:
		msg := st.Message
		if me != nil && st.Phase == proto.PhaseBetting && me.PendingBet > 0 {
			msg = "Bet placed: " + commas(me.PendingBet) + ". Waiting for the others..."
		} else if me != nil && st.Phase == proto.PhasePlaying && len(me.Hands) > 0 && me.ActiveHand < 0 {
			if st.ActiveSeat >= 0 && st.ActiveSeat < len(st.Seats) {
				msg = "Waiting for " + st.Seats[st.ActiveSeat].Username + "..."
			} else {
				msg = "Waiting for the other players..."
			}
		} else if me != nil && len(me.Hands) == 0 && (st.Phase == proto.PhaseDealing || st.Phase == proto.PhasePlaying || st.Phase == proto.PhaseDealer) {
			msg = "You'll be dealt in next hand."
		}
		if msg == "" {
			msg = "Waiting for players..."
		}
		lines := a.wrap(msg, cx1-cx0-10)
		y := hudTop + 30 - len(lines)*5
		for _, l := range lines {
			a.text((cx0+cx1)/2, y, l, a.A.C("text"), alignCenter)
			y += 10
		}
	}

	// right: phase, timer, toggles
	rx0, rx1 := 484, ScreenW-10
	phase := strings.ToUpper(st.Phase)
	yourTurn := len(st.Actions) > 0 || st.CanBet
	pc := a.A.C("gold")
	if yourTurn {
		phase = "YOUR MOVE"
		if (a.frame/15)%2 == 0 {
			pc = a.A.C("highlight")
		}
	}
	a.text(rx0, hudTop+7, phase, pc, alignLeft)
	if ms, frac := a.remaining(); st.Deadline != 0 {
		a.text(rx1, hudTop+7, fmt.Sprintf("%ds", (ms+999)/1000), a.A.C("text"), alignRight)
		bar := image.Rect(rx0, hudTop+18, rx1, hudTop+24)
		fillRect(a.fb, bar, color.RGBA{0, 0, 0, 255})
		in := bar.Inset(1)
		w := int(float64(in.Dx()) * frac)
		c := a.A.C("gold")
		if frac < 0.25 {
			c = a.A.C("lose")
		}
		fillRect(a.fb, image.Rect(in.Min.X, in.Min.Y, in.Min.X+w, in.Max.Y), c)
	}
	half := (rx1 - rx0 - 4) / 2
	if me != nil {
		label, on := "SIT OUT", true
		if me.Status == "sitting_out" {
			label, on = "SIT IN", false
		}
		if a.button("sit", image.Rect(rx0, hudTop+32, rx0+half, hudTop+50), label, btn{enabled: true}) {
			a.send(proto.CSitOut, proto.SitOut{On: on})
		}
	}
	snd := "SOUND ON"
	if !a.Cfg.Sound {
		snd = "SOUND OFF"
	}
	if a.button("snd", image.Rect(rx1-half, hudTop+32, rx1, hudTop+50), snd, btn{enabled: true, hotkey: 'm'}) {
		a.Cfg.Sound = !a.Cfg.Sound
		a.Cfg.Save()
	}
}

func (a *App) drawBetting(st *proto.TableState, me *proto.SeatView, x0, x1 int) {
	s := st.Settings
	limit := s.MaxBet
	if me.Balance < limit {
		limit = me.Balance
	}
	cs := a.A.Theme.ChipSize
	x := x0 + 4
	y := hudTop + 7
	for i, v := range a.A.Theme.ChipValues {
		r := image.Rect(x, y, x+cs, y+cs)
		ok := a.betDraft+v <= limit
		hov := a.hovered(r)
		dy := 0
		if hov && ok {
			dy = -2
		}
		blit(a.fb, a.A.ChipImage(i), x, y+dy)
		if !ok {
			fillRect(a.fb, r, color.RGBA{0, 0, 0, 140})
		}
		a.text(x+cs/2, y+cs+3, chipLabel(v), a.A.C("text"), alignCenter)
		if hov && a.in.Pressed && ok {
			a.betDraft += v
			a.sound("chip")
		}
		if hov && a.in.RightPressed && a.betDraft > 0 {
			a.betDraft -= v
			if a.betDraft < 0 {
				a.betDraft = 0
			}
			a.sound("chip")
		}
		x += cs + 4
	}
	if a.in.Wheel != 0 && !a.blocked {
		step := s.MinBet
		a.betDraft += int64(a.in.Wheel) * step
		if a.betDraft < 0 {
			a.betDraft = 0
		}
		if a.betDraft > limit {
			a.betDraft = limit
		}
	}
	a.text(x0+4, hudTop+42, "right-click removes", a.A.C("text_dim"), alignLeft)

	bx := x0 + 158
	label := "YOUR BET"
	if me.PendingBet > 0 {
		label = "BET PLACED: " + commas(me.PendingBet)
	}
	a.text(bx, hudTop+6, label, a.A.C("text_dim"), alignLeft)
	a.textBig(bx, hudTop+16, commas(a.betDraft), a.A.C("gold"), 2, alignLeft)
	by := hudTop + 34
	if a.button("clear", image.Rect(bx, by, bx+44, by+18), "CLEAR", btn{enabled: a.betDraft > 0, hotkey: 'c'}) {
		a.betDraft = 0
	}
	if a.button("rebet", image.Rect(bx+48, by, bx+96, by+18), "REBET", btn{enabled: a.lastBet > 0 && a.lastBet <= limit, hotkey: 'r'}) {
		a.betDraft = a.lastBet
	}
	valid := a.betDraft >= s.MinBet && a.betDraft <= limit
	bl := "DEAL"
	if me.PendingBet > 0 {
		bl = "CHANGE"
	}
	if a.button("bet", image.Rect(bx+100, by, x1, by+18), bl, btn{enabled: valid && a.betDraft != me.PendingBet, style: styleGold, key: KeyEnter}) {
		a.lastBet = a.betDraft
		a.send(proto.CBet, proto.Bet{Amount: a.betDraft})
	}
	if a.betDraft > 0 && a.betDraft < s.MinBet {
		a.text(bx+100, hudTop+6, "min "+commas(s.MinBet), a.A.C("error"), alignLeft)
	}
}

func (a *App) drawActions(st *proto.TableState, me *proto.SeatView, x0, x1 int) {
	can := map[string]bool{}
	for _, act := range st.Actions {
		can[act] = true
	}
	if me.ActiveHand >= 0 && me.ActiveHand < len(me.Hands) {
		h := me.Hands[me.ActiveHand]
		lbl := totalLabel(h.Total, h.Soft, h.Natural, len(h.Cards))
		title := "YOUR HAND: " + lbl
		if len(me.Hands) > 1 {
			title = fmt.Sprintf("HAND %d OF %d: %s", me.ActiveHand+1, len(me.Hands), lbl)
		}
		a.text((x0+x1)/2, hudTop+7, title, a.A.C("gold"), alignCenter)
	}
	type ab struct {
		act, label string
		hot        rune
	}
	acts := []ab{{"hit", "HIT", 'h'}, {"stand", "STAND", 's'}, {"double", "DOUBLE", 'd'}, {"split", "SPLIT", 'p'}}
	if st.Settings.Surrender {
		acts = append(acts, ab{"surrender", "SURRENDER", 'u'})
	}
	gap := 4
	w := (x1 - x0 - gap*(len(acts)-1)) / len(acts)
	x := x0
	for _, b := range acts {
		r := image.Rect(x, hudTop+19, x+w, hudTop+41)
		style := styleRed
		if b.act == "stand" {
			style = styleGold
		}
		if a.button("act_"+b.act, r, b.label, btn{enabled: can[b.act], style: style, hotkey: b.hot}) {
			a.send(proto.CAction, proto.Action{Action: b.act})
		}
		x += w + gap
	}
	keys := "keys: H hit  S stand  D double  P split"
	if st.Settings.Surrender {
		keys += "  U surrender"
	}
	a.text((x0+x1)/2, hudTop+45, keys, a.A.C("text_dim"), alignCenter)
}
