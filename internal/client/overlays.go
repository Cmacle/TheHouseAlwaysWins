package client

import (
	"fmt"
	"image"
	"strings"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func (a *App) overlayFrame(r image.Rectangle, title string) bool {
	fillRect(a.fb, a.fb.Rect, a.A.C("overlay"))
	a.panel(r)
	a.textBig(r.Min.X+r.Dx()/2, r.Min.Y+10, title, a.A.C("gold"), 2, alignCenter)
	closeR := image.Rect(r.Max.X-24, r.Min.Y+8, r.Max.X-8, r.Min.Y+24)
	if a.button("ov_close", closeR, "X", btn{enabled: true, key: KeyEscape}) {
		a.overlay = ovNone
		return true
	}
	// clicking outside closes too
	if a.in.Pressed && !image.Pt(a.in.MX, a.in.MY).In(r) {
		a.overlay = ovNone
		return true
	}
	return false
}

func (a *App) drawLeaderboard() {
	r := ovRect(480, max(324, ScreenH-80))
	if a.overlayFrame(r, "LEADERBOARD") {
		return
	}
	// tabs
	tabs := []struct{ sort, label string }{{"balance", "RICHEST NOW"}, {"net", "LIFETIME GAIN"}, {"house", "THE HOUSE"}}
	tx := r.Min.X + r.Dx()/2 - 158
	for _, t := range tabs {
		style := styleRed
		if a.lbSort == t.sort {
			style = styleGold
		}
		if a.button("lbt_"+t.sort, image.Rect(tx, r.Min.Y+32, tx+100, r.Min.Y+48), t.label, btn{enabled: true, style: style}) && a.lbSort != t.sort {
			a.lbSort = t.sort
			if t.sort != "house" {
				a.leaderboard = nil
			}
			a.lbScroll = 0
			a.requestLeaderboard()
		}
		tx += 108
	}
	if a.in.key(KeyLeft) || a.in.key(KeyRight) {
		order := []string{"balance", "net", "house"}
		i := 0
		for k, v := range order {
			if v == a.lbSort {
				i = k
			}
		}
		if a.in.key(KeyRight) {
			i = (i + 1) % len(order)
		} else {
			i = (i + len(order) - 1) % len(order)
		}
		a.lbSort = order[i]
		if a.lbSort != "house" {
			a.leaderboard = nil
		}
		a.lbScroll = 0
		a.requestLeaderboard()
	}
	if a.now.Sub(a.lbAt).Seconds() > 10 {
		a.requestLeaderboard()
	}
	if a.lbSort == "house" {
		a.drawHouse(r)
		return
	}
	rich := a.lbSort == "balance"
	x0 := r.Min.X + 14
	right := r.Max.X - 18
	narrow := r.Dx() < 440
	cols := []struct {
		x     int
		label string
		al    align
	}{
		{x0 + 16, "#", alignRight}, {x0 + 48, "PLAYER", alignLeft}, {right - 128, "CHIPS NOW", alignRight},
		{right - 48, "LIFETIME", alignRight}, {right, "HANDS", alignRight},
	}
	if narrow { // no room for hands played
		cols[2].x, cols[3].x, cols[4].label = right-72, right, ""
	}
	nameW := cols[2].x - 70 - cols[1].x
	hy := r.Min.Y + 56
	for i, c := range cols {
		col := a.A.C("text_dim")
		if (i == 2 && rich) || (i == 3 && !rich) {
			col = a.A.C("gold")
		}
		a.text(c.x, hy, c.label, col, c.al)
	}
	rowH := 22
	top := hy + 11
	visible := (r.Max.Y - 22 - top) / rowH
	maxScroll := len(a.leaderboard) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.in.Wheel != 0 {
		a.lbScroll -= a.in.Wheel * 2
	}
	if a.in.key(KeyDown) {
		a.lbScroll++
	}
	if a.in.key(KeyUp) {
		a.lbScroll--
	}
	if a.in.key(KeyPageDown) {
		a.lbScroll += visible
	}
	if a.in.key(KeyPageUp) {
		a.lbScroll -= visible
	}
	if a.lbScroll > maxScroll {
		a.lbScroll = maxScroll
	}
	if a.lbScroll < 0 {
		a.lbScroll = 0
	}
	if len(a.leaderboard) == 0 {
		a.text(r.Min.X+r.Dx()/2, top+30, "Loading...", a.A.C("text_dim"), alignCenter)
	}
	for i := 0; i < visible && a.lbScroll+i < len(a.leaderboard); i++ {
		idx := a.lbScroll + i
		p := a.leaderboard[idx]
		y := top + i*rowH
		row := image.Rect(x0, y, r.Max.X-14, y+rowH)
		if strings.EqualFold(p.Username, a.username) {
			fillRect(a.fb, row, a.A.C("row_me"))
		} else if idx%2 == 1 {
			fillRect(a.fb, row, a.A.C("row_alt"))
		}
		ty := y + 7
		rankCol := a.A.C("text")
		switch idx {
		case 0:
			rankCol = a.A.C("gold")
		case 1:
			rankCol = parseHex("#d8dde6")
		case 2:
			rankCol = parseHex("#d08a4a")
		}
		a.text(cols[0].x, ty, fmt.Sprint(idx+1), rankCol, alignRight)
		ax, ay := x0+24, y+4
		blit(a.fb, a.avatarImg(p.Username, p.AvatarVersion, 16), ax, ay)
		if h := a.smallHat(p.Hat); h != nil {
			blit(a.fb, h, ax+8-h.Rect.Dx()/2, ay-h.Rect.Dy()+3)
		}
		name := fitText(a.A.Font, p.Username, nameW)
		a.text(cols[1].x, y+3, name, a.A.C("text"), alignLeft)
		if p.Online {
			dx := cols[1].x + a.A.Font.Width(name) + 5
			fillRect(a.fb, image.Rect(dx, y+5, dx+3, y+8), a.A.C("online"))
		}
		if t, c, ok := titleOf(p.Title); ok {
			a.text(cols[1].x, y+12, fitText(a.A.Font, t, nameW+20), c, alignLeft)
		}
		nc := a.A.C("text")
		if p.Net > 0 {
			nc = a.A.C("win")
		} else if p.Net < 0 {
			nc = a.A.C("lose")
		}
		bc := a.A.C("text")
		if rich {
			bc = a.A.C("gold")
		}
		a.text(cols[2].x, ty, commas(p.Balance), bc, alignRight)
		a.text(cols[3].x, ty, signed(p.Net), nc, alignRight)
		if !narrow {
			a.text(cols[4].x, ty, fmt.Sprint(p.HandsPlayed), a.A.C("text"), alignRight)
		}
	}
	foot := "The richest player wears the crown."
	if !rich {
		foot = "Lifetime = won minus lost, minus daily refills."
	}
	if maxScroll > 0 {
		foot += "  Scroll for more."
	}
	a.text(r.Min.X+r.Dx()/2, r.Max.Y-16, fitText(a.A.Font, foot, r.Dx()-20), a.A.C("text_dim"), alignCenter)
}

func (a *App) drawProfile() {
	r := ovRect(360, 306)
	if a.overlayFrame(r, "PROFILE") {
		return
	}
	p := a.profile
	ar := image.Rect(r.Min.X+36, r.Min.Y+56, r.Min.X+68, r.Min.Y+88)
	a.drawAvatarFull(a.username, p.AvatarVersion, p.Hat, p.Frame, ar.Min.X, ar.Min.Y, false)
	if a.button("wardrobe", image.Rect(r.Min.X+14, ar.Max.Y+10, r.Min.X+90, ar.Max.Y+28), "WARDROBE", btn{enabled: true, style: styleGold, hotkey: 'w'}) {
		a.overlay = ovWardrobe
		a.wardSel = ""
	}
	if a.button("chpic", image.Rect(r.Min.X+14, ar.Max.Y+32, r.Min.X+90, ar.Max.Y+50), "NEW PICTURE", btn{enabled: true}) {
		a.choosePicture("profile")
	}
	x := r.Min.X + 110
	y := r.Min.Y + 42
	a.textBig(x, y, a.username, a.A.C("text"), 2, alignLeft)
	y += 18
	if t, c, ok := titleOf(p.Title); ok {
		a.text(x, y, t, c, alignLeft)
	}
	y += 12
	row := func(label, val string, c string) {
		a.text(x, y, label, a.A.C("text_dim"), alignLeft)
		a.text(r.Max.X-24, y, val, a.A.C(c), alignRight)
		y += 12
	}
	netC := "text"
	if p.Net > 0 {
		netC = "win"
	} else if p.Net < 0 {
		netC = "lose"
	}
	row("Chips", commas(p.Balance), "text")
	row("Lifetime gain/loss", signed(p.Net), netC)
	row("Total won", commas(p.Won), "win")
	row("Total lost", commas(p.Lost), "lose")
	row("Refills received", commas(p.RefillTotal), "text")
	row("Hands played", commas(int64(p.HandsPlayed)), "text")
	row("Won / lost / pushed", fmt.Sprintf("%d / %d / %d", p.HandsWon, p.HandsLost, p.HandsPushed), "text")
	row("Blackjacks", fmt.Sprint(p.Blackjacks), "gold")
	row("Biggest win", commas(p.BiggestWin), "gold")
	row("Total wagered", commas(p.Wagered), "text")
	row("Most chips held", commas(p.PeakBalance), "text")
	row("Best win / lose streak", fmt.Sprintf("%d / %d", p.BestWinStreak, p.WorstLoseStreak), "text")
	if a.st != nil && a.st.Settings.RefillAmount > 0 {
		a.text(r.Min.X+r.Dx()/2, r.Max.Y-20, fmt.Sprintf("Below %s chips? You're topped up once a day.", commas(a.st.Settings.RefillAmount)), a.A.C("text_dim"), alignCenter)
	}
}

func (a *App) drawHelp() {
	r := ovRect(420, 304)
	if Portrait() {
		r = ovRect(420, ScreenH-60)
	}
	if a.overlayFrame(r, "HOW TO PLAY") {
		return
	}
	s := proto.Settings{}
	if a.st != nil {
		s = a.st.Settings
	}
	dealer := "stands on all 17s"
	if s.DealerHitsSoft17 {
		dealer = "hits soft 17"
	}
	betMode := "Everyone bets at the same time."
	if s.BettingMode == "turns" {
		betMode = "Players bet one at a time, in seat order."
	}
	playMode := "Players take turns, left to right."
	if s.PlayMode == "simultaneous" {
		playMode = "Everyone plays their hand at the same time."
	}
	hidden := ""
	if s.HideOtherHands {
		hidden = " Other players' cards stay hidden until the hand ends."
	}
	lines := []string{
		"Beat the dealer by getting closer to 21 without going over.",
		fmt.Sprintf("Blackjack pays %s. The dealer %s. %d deck shoe.", s.BlackjackPays, dealer, s.Decks),
		fmt.Sprintf("Bets: %s to %s. %s", commas(s.MinBet), commas(s.MaxBet), betMode),
		playMode + hidden,
		"",
		"HIT (H) take a card    STAND (S) keep your hand",
		"DOUBLE (D) double the bet, take exactly one card",
		"SPLIT (P) split a pair into two hands",
	}
	if s.Surrender {
		lines = append(lines, "SURRENDER (U) give up half your bet")
	}
	lines = append(lines,
		"",
		"Betting: click chips, ENTER deals, R rebets, C clears.",
		fmt.Sprintf("Daily refill: below %s chips, you're topped back up -", commas(s.RefillAmount)),
		"but the refill counts against your lifetime total.",
		"Unlock hats, frames and titles by playing (PROFILE > WARDROBE).",
		"Whoever has the most chips wears the crown!",
		"",
		"L leaderboard   T dealer log   O settings   M mute   F11 fullscreen",
	)
	y := r.Min.Y + 36
	for _, l := range lines {
		for _, wl := range a.wrap(l, r.Dx()-36) {
			a.text(r.Min.X+18, y, wl, a.A.C("text"), alignLeft)
			y += 11
		}
		if l == "" {
			y -= 5
		}
	}
}
