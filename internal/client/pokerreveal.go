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
	"github.com/Cmacle/TheHouseAlwaysWins/internal/poker"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The card reveal at the poker table: the flop, turn or river comes out
// face down, big in the middle of the table, with the suspense sound, and
// turns over when the server says. Then the cards drop onto the board.

type pkRevealFX struct {
	from, n   int
	start     time.Time
	flip      time.Time
	flipped   bool
	flippedAt time.Time
	cards     []bj.Card
	slots     []image.Point // where each card sits on the board
	sound     string
	soundAt   time.Time
	soundOn   bool
	label     string
	labelCol  color.RGBA
	before    int // your hand's strength before these cards
	hand      bool
	seat      int // hand reveals: whose
}

var categoryNames = map[int]string{
	poker.Pair: "A PAIR", poker.TwoPair: "TWO PAIR", poker.Trips: "THREE OF A KIND", poker.Straight: "A STRAIGHT",
	poker.Flush: "A FLUSH", poker.FullHouse: "A FULL HOUSE", poker.Quads: "FOUR OF A KIND",
	poker.StraightFlush: "A STRAIGHT FLUSH", poker.RoyalFlush: "A ROYAL FLUSH",
}

// onPokerReveal follows the reveal in a poker state.
func (a *App) onPokerReveal(ps *proto.PokerState) {
	rv := ps.Reveal
	if rv == nil {
		if r := a.pkRev; r != nil && !r.flipped {
			a.pkRevealFlipped(ps) // missed the flip: show it now
		}
		return
	}
	r := a.pkRev
	if r == nil || r.from != rv.From || r.hand != rv.Hand || r.seat != rv.Seat {
		flip := time.UnixMilli(rv.Until - a.clockOffset)
		r = &pkRevealFX{from: rv.From, n: rv.Count, start: a.now, flip: flip, before: -1, hand: rv.Hand, seat: rv.Seat}
		if me := a.pokerMe(); me != nil && me.InHand && !me.Folded && !rv.Hand {
			r.before = poker.Strength(me.Cards, ps.Board[:min(rv.From, len(ps.Board))])
		}
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
		a.pkRev = r
	}
	r.cards = r.cards[:0]
	if rv.Hand {
		if rv.Shown && rv.Seat >= 0 && rv.Seat < len(ps.Seats) {
			r.cards = append(r.cards, ps.Seats[rv.Seat].Cards...)
		}
	} else {
		for i := rv.From; i < rv.From+rv.Count && i < len(ps.Board); i++ {
			r.cards = append(r.cards, ps.Board[i])
		}
	}
	if rv.Shown && !r.flipped {
		a.pkRevealFlipped(ps)
	}
}

// pkRevealFlipped is the moment the cards turn over: how they went for you.
func (a *App) pkRevealFlipped(ps *proto.PokerState) {
	r := a.pkRev
	r.flipped, r.flippedAt = true, a.now
	if r.hand {
		// a hand at the showdown: what it makes, and how good that is
		if r.seat < 0 || r.seat >= len(ps.Seats) {
			return
		}
		sv := ps.Seats[r.seat]
		cat := poker.Strength(sv.Cards, ps.Board)
		name := sv.HandName
		if name == "" {
			name = poker.Describe(sv.Cards, ps.Board)
		}
		who := sv.Username
		if r.seat == ps.YourSeat {
			who = "YOU"
		}
		r.label, r.labelCol = strings.ToUpper(who+": "+name), a.A.C("text")
		switch {
		case cat >= poker.Trips:
			r.labelCol = a.A.C("gold")
			a.sound("rev_great")
		case cat >= poker.Pair:
			r.labelCol = a.A.C("highlight")
			a.sound("rev_good")
		default:
			a.sound("rev_meh")
		}
		return
	}
	me := a.pokerMe()
	if me == nil || !me.InHand || me.Folded || r.before < 0 {
		a.sound("rev_meh")
		return
	}
	after := poker.Strength(me.Cards, ps.Board)
	switch {
	case after > r.before:
		r.label, r.labelCol = "YOU HAVE "+categoryNames[after]+"!", a.A.C("win")
		if after >= poker.Trips {
			r.labelCol = a.A.C("gold")
			a.sound("rev_great")
		} else {
			a.sound("rev_good")
		}
	default:
		a.sound("rev_meh")
	}
}

// pkRevealHides says whether board card i is out being revealed, and
// remembers where it goes.
func (a *App) pkRevealHides(i, x, y int) bool {
	r := a.pkRev
	if r == nil || r.hand || i < r.from || i >= r.from+r.n {
		return false
	}
	for len(r.slots) <= i-r.from {
		r.slots = append(r.slots, image.Point{})
	}
	r.slots[i-r.from] = image.Pt(x, y)
	return true
}

// drawPokerReveal draws the big cards over the table.
func (a *App) drawPokerReveal() {
	r := a.pkRev
	if r == nil {
		return
	}
	if !r.soundOn && !a.now.Before(r.soundAt) && !r.flipped {
		r.soundOn = true
		a.sound(r.sound)
	}
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	k := max(2, min(3, (ScreenH-120)/ch))
	if r.n == 1 {
		k = max(2, min(4, (ScreenH-120)/ch))
	}
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
	allDone := true
	dimmed := false
	var labelY int
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
			// the cards turn over one after another
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
		a.pkRev = nil
	}
}

// pkRevealHidesHand says whether a seat's hole card ci is out being shown at
// the showdown, and remembers where it goes back to.
func (a *App) pkRevealHidesHand(seat, ci, x, y int) bool {
	r := a.pkRev
	if r == nil || !r.hand || r.seat != seat {
		return false
	}
	for len(r.slots) <= ci {
		r.slots = append(r.slots, image.Point{})
	}
	r.slots[ci] = image.Pt(x, y)
	return true
}
