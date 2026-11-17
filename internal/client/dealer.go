package client

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func signedMoney(n int64) string {
	switch {
	case n > 0:
		return "+$" + commas(n)
	case n < 0:
		return "-$" + commas(-n)
	}
	return "$0"
}

// dealerSprite picks the dealer's cell: column = wealth tier, row = vulgar.
func (a *App) dealerSprite(st *proto.TableState) *image.RGBA {
	w, h := a.A.Theme.DealerCellW, a.A.Theme.DealerCellH
	col, row := 3, 0
	if st != nil {
		col = st.HouseTier - banter.MinTier
		if st.Settings.DealerVulgar {
			row = 1
		}
	}
	if col < 0 {
		col = 0
	}
	if col > banter.MaxTier-banter.MinTier {
		col = banter.MaxTier - banter.MinTier
	}
	r := image.Rect(col*w, row*h, col*w+w, row*h+h)
	if w <= 0 || h <= 0 || !r.In(a.A.Dealers.Rect) {
		return a.A.Dealers
	}
	return a.A.Dealers.SubImage(r).(*image.RGBA)
}

func (a *App) dealerVulgar(st *proto.TableState) bool {
	return st != nil && st.Settings.DealerVulgar
}

func (a *App) talking(st *proto.TableState) bool {
	return st.DealerSay != "" && a.now.UnixMilli()+a.clockOffset < st.DealerSayUntil
}

// drawDealer draws the dealer (bobbing while talking) and cigar smoke.
func (a *App) drawDealer(st *proto.TableState) {
	spr := a.dealerSprite(st)
	w, h := spr.Rect.Dx(), spr.Rect.Dy()
	x := ScreenW/2 - w/2
	y := 52 - h
	if a.talking(st) && (a.frame/6)%2 == 0 {
		y--
	}
	blit(a.fb, spr, x, y)
	if !a.dealerVulgar(st) || st.HouseTier <= -2 {
		return // no cigar for the poorest
	}
	// smoke: a few puffs rising and drifting from the cigar tip
	ox, oy := x+a.A.Theme.CigarX, y+a.A.Theme.CigarY
	t := float64(a.now.UnixMilli()%100000) / 1000
	smoke := a.A.C("smoke")
	for i := 0; i < 7; i++ {
		ph := math.Mod(t*0.55+float64(i)/7, 1) // 0..1 life
		py := oy - int(ph*22)
		px := ox + int(math.Sin(t*1.7+float64(i)*1.9)*2.5+ph*5)
		size := 1
		if ph > 0.45 {
			size = 2
		}
		c := smoke
		fade := 1 - ph
		c = color.RGBA{uint8(float64(c.R) * fade), uint8(float64(c.G) * fade), uint8(float64(c.B) * fade), uint8(float64(c.A) * fade)}
		fillRect(a.fb, image.Rect(px, py, px+size, py+size), c)
	}
}

// drawSpeech draws the dealer's speech bubble.
func (a *App) drawSpeech(st *proto.TableState) {
	if !a.talking(st) {
		return
	}
	mouthX := ScreenW/2 + 16
	lines := a.wrap(st.DealerSay, bubbleWrap)
	if len(lines) > bubbleMaxLines {
		lines = lines[:bubbleMaxLines]
		lines[len(lines)-1] = strings.TrimRight(lines[len(lines)-1], ".,!? ") + "..."
	}
	w := 0
	for _, l := range lines {
		if lw := a.A.Font.Width(l); lw > w {
			w = lw
		}
	}
	lh := 9
	bx := mouthX + 14
	by := 22
	// pop in
	if age := a.now.Sub(a.sayAt); age < 120*time.Millisecond {
		by += 2 - int(age/(60*time.Millisecond))
	}
	r := image.Rect(bx, by, bx+w+12, by+len(lines)*lh+7)
	bub, edge := a.A.C("bubble"), a.A.C("bubble_txt")
	fillRect(a.fb, image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X+1, r.Max.Y+1), color.RGBA{0, 0, 0, 120})
	fillRect(a.fb, r, bub)
	strokeRect(a.fb, r, edge)
	for _, p := range []image.Point{r.Min, {r.Max.X - 1, r.Min.Y}, {r.Min.X, r.Max.Y - 1}, {r.Max.X - 1, r.Max.Y - 1}} {
		fillRect(a.fb, image.Rect(p.X, p.Y, p.X+1, p.Y+1), color.RGBA{})
	}
	// tail toward the dealer's mouth
	ty := r.Min.Y + 8
	for i := 0; i < 5; i++ {
		fillRect(a.fb, image.Rect(r.Min.X-i-1, ty+i/2, r.Min.X, ty+i/2+1), bub)
		fillRect(a.fb, image.Rect(r.Min.X-i-2, ty+i/2, r.Min.X-i-1, ty+i/2+1), edge)
	}
	fillRect(a.fb, image.Rect(r.Min.X-6, ty+2, r.Min.X, ty+3), edge)
	fillRect(a.fb, image.Rect(r.Min.X, ty, r.Min.X+1, ty+3), bub)
	for i, l := range lines {
		a.A.Font.Draw(a.fb, r.Min.X+6, r.Min.Y+4+i*lh, l, a.A.C("bubble_txt"))
	}
}

const (
	bubbleWrap     = 220
	bubbleMaxLines = 4
)

// drawHouse is the leaderboard's house tab.
func (a *App) drawHouse(r image.Rectangle) {
	h := a.house
	spr := a.dealerSprite(a.st)
	pw, ph := spr.Rect.Dx()*2, spr.Rect.Dy()*2
	px, py := r.Min.X+28, r.Min.Y+50
	fillRect(a.fb, image.Rect(px-4, py-4, px+pw+4, py+ph+4), color.RGBA{0, 0, 0, 120})
	blitScaled(a.fb, spr, image.Rect(px, py, px+pw, py+ph))
	a.text(px+pw/2, py+ph+10, strings.ToUpper(banter.TierNames[h.Tier]), a.A.C("gold"), alignCenter)

	x := r.Min.X + 170
	y := r.Min.Y + 64
	nc := a.A.C("text")
	if h.Net > 0 {
		nc = a.A.C("win")
	} else if h.Net < 0 {
		nc = a.A.C("lose")
	}
	a.text(x, y-14, banter.DealerName, a.A.C("gold"), alignLeft)
	a.text(x, y, "LIFETIME", a.A.C("text_dim"), alignLeft)
	a.textBig(x, y+10, signedMoney(h.Net), nc, 2, alignLeft)
	y += 34
	row := func(label, val string, c color.RGBA) {
		a.text(x, y, label, a.A.C("text_dim"), alignLeft)
		a.text(r.Max.X-30, y, val, c, alignRight)
		y += 13
	}
	txt := a.A.C("text")
	pct := func(n, d int) string {
		if d == 0 {
			return "-"
		}
		return commas(int64(n*100/d)) + "%"
	}
	row("Rounds dealt", commas(int64(h.HandsDealt)), txt)
	row("Player hands beaten up", commas(int64(h.PlayerHands)), txt)
	row("Rounds won / lost", commas(int64(h.RoundsWon))+" / "+commas(int64(h.RoundsLost))+"  ("+pct(h.RoundsWon, h.HandsDealt)+")", txt)
	row("Taken from players", "$"+commas(h.Won), a.A.C("win"))
	row("Paid out to players", "$"+commas(h.Lost), a.A.C("lose"))
	row("Best round for the house", "$"+commas(h.BiggestWin), a.A.C("gold"))
	row("Worst round for the house", "$"+commas(h.BiggestLoss), a.A.C("gold"))
	row("Dealer blackjacks", commas(int64(h.DealerBlackjacks)), txt)
	row("Dealer busts", commas(int64(h.DealerBusts))+"  ("+pct(h.DealerBusts, h.HandsDealt)+")", txt)
	row("Player busts", commas(int64(h.PlayerBusts)), txt)
	up, ok, down, dok := banter.Thresholds(h.Tier)
	if ok {
		row("Next outfit upgrade at", signedMoney(up), a.A.C("highlight"))
	}
	if dok {
		row("Outfit downgrade below", signedMoney(down), a.A.C("highlight"))
	}
	if h.HandsDealt == 0 {
		a.text(x, y, "No hands dealt yet.", a.A.C("text_dim"), alignLeft)
	}
	by := y + 8
	if lab := py + ph + 22; lab > by {
		by = lab
	}
	for _, l := range a.wrap(banter.Bio, r.Dx()-48) {
		if by > r.Max.Y-16 {
			break
		}
		a.text(r.Min.X+24, by, l, a.A.C("text_dim"), alignLeft)
		by += 10
	}
}
