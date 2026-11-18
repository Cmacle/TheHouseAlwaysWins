// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Poker action animations, so it's clear what everyone just did: a label
// pops up over the seat, chips slide from the stack to the bet, folded cards
// fly into the muck, a check knocks on the table, and the pot slides to
// whoever wins it.

type pkFXKind int

const (
	fxCheck pkFXKind = iota
	fxChips          // call, bet, raise, blinds
	fxAllIn
	fxFold
	fxWin
)

type pkFX struct {
	seat  int
	kind  pkFXKind
	label string
	col   color.RGBA
	start time.Time
	ncard int // fold: how many cards to throw in
}

// pkGeo is where a seat's things are on screen this frame.
type pkGeo struct {
	x, cy       int         // portrait centre (Dale: his cards)
	pile        image.Point // top of the chip pile
	bet         image.Point // where the bet chip sits
	cards       image.Point // top-left of the hole cards
	labelBottom int         // action labels pop up above this
	labelX      int         // ...centred here (0 = x)
}

const (
	fxLabelSecs = 1.5
	fxChipSecs  = 0.35
	fxFoldSecs  = 0.45
	fxWinSecs   = 0.7
)

// pokerFX compares two states and starts animations for what changed.
func (a *App) pokerFX(prev, ps *proto.PokerState) {
	if prev == nil {
		return
	}
	newHand := prev.HandNumber != ps.HandNumber
	for i, s := range ps.Seats {
		if s.Username == "" {
			continue
		}
		var was proto.PokerSeat
		if i < len(prev.Seats) && !newHand {
			was = prev.Seats[i]
		}
		if was.Username != "" && !strings.EqualFold(was.Username, s.Username) {
			continue
		}
		changed := s.Action != "" && (s.Action != was.Action || s.Bet > was.Bet)
		if changed {
			a.startActionFX(i, s, s.Bet-was.Bet, len(was.Cards))
		}
	}
	// the pot goes to the winners
	if ps.Phase == "showdown" && prev.Phase != "showdown" {
		for i, s := range ps.Seats {
			if s.Won > 0 {
				a.pkFX = append(a.pkFX, pkFX{seat: i, kind: fxWin, label: "WINS " + shortChips(s.Won), col: a.A.C("win"), start: a.now})
			}
		}
	}
}

func (a *App) startActionFX(seat int, s proto.PokerSeat, added int64, hadCards int) {
	act := strings.ToLower(s.Action)
	fx := pkFX{seat: seat, start: a.now, col: a.A.C("highlight")}
	switch {
	case act == "fold":
		fx.kind, fx.label, fx.col = fxFold, "FOLD", color.RGBA{190, 190, 190, 255}
		fx.ncard = max(hadCards, 2)
		a.sound("card")
	case act == "check":
		fx.kind, fx.label, fx.col = fxCheck, "CHECK", a.A.C("text")
		a.sound("click")
	case act == "all in":
		fx.kind, fx.label, fx.col = fxAllIn, "ALL IN!", a.A.C("lose")
	case act == "left":
		return
	default:
		fx.kind = fxChips
		fx.label = strings.ToUpper(s.Action)
		if act == "call" && added > 0 {
			fx.label = "CALL " + shortChips(added)
		}
		if strings.HasPrefix(act, "bet") || strings.HasPrefix(act, "raise") {
			fx.col = a.A.C("gold")
		}
	}
	// one animation per seat at a time: the newest wins
	out := a.pkFX[:0]
	for _, f := range a.pkFX {
		if f.seat != seat || f.kind == fxWin {
			out = append(out, f)
		}
	}
	a.pkFX = append(out, fx)
}

// drawPokerFX draws the running animations.
func (a *App) drawPokerFX(muck image.Point) {
	if len(a.pkFX) == 0 {
		return
	}
	out := a.pkFX[:0]
	for _, f := range a.pkFX {
		g, ok := a.pkGeo[f.seat]
		t := a.now.Sub(f.start).Seconds()
		if !ok || t > fxLabelSecs {
			continue
		}
		out = append(out, f)
		switch f.kind {
		case fxFold:
			a.fxFold(f, g, muck, t)
		case fxCheck:
			a.fxKnock(g, t)
		case fxChips:
			a.fxChipsTo(g.pile, g.bet, t, 3)
		case fxAllIn:
			a.fxChipsTo(g.pile, g.bet, t, 6)
			if t < 0.6 && (a.frame/4)%2 == 0 {
				strokeRect(a.fb, image.Rect(g.x-19, g.cy-19, g.x+19, g.cy+19), a.A.C("lose"))
			}
		case fxWin:
			a.fxChipsTo(muck, g.pile, t, 6)
		}
		a.fxLabel(f, g, t)
	}
	a.pkFX = out
}

// fxLabel pops the action up over the seat, then floats it away.
func (a *App) fxLabel(f pkFX, g pkGeo, t float64) {
	if f.kind == fxWin && t < fxWinSecs*0.6 {
		return // after the chips land
	}
	rise := int(math.Min(t, 1) * 8)
	y := g.labelBottom - 12 - rise
	scale := 1
	if t < 0.5 || f.kind == fxAllIn || f.kind == fxWin {
		scale = 2
	}
	if t > 1.1 && (a.frame/3)%2 == 0 {
		return // flickers out
	}
	w := a.A.Font.Width(f.label)*scale + 8
	h := 9*scale + 4
	x := g.x
	if g.labelX != 0 {
		x = g.labelX
	}
	x = min(max(x, w/2+2), ScreenW-w/2-2)
	r := image.Rect(x-w/2, y-h+8, x+w/2, y+8)
	fillRect(a.fb, r, color.RGBA{0, 0, 0, 170})
	strokeRect(a.fb, r, f.col)
	if scale == 2 {
		a.textBig(x, r.Min.Y+2, f.label, f.col, 2, alignCenter)
	} else {
		a.text(x, r.Min.Y+2, f.label, f.col, alignCenter)
	}
}

// fxChipsTo slides a few chips from one point to another along an arc.
func (a *App) fxChipsTo(from, to image.Point, t float64, n int) {
	chip := a.smallChip(2)
	cs := chip.Rect.Dx()
	for i := 0; i < n; i++ {
		ti := (t - float64(i)*0.05) / fxChipSecs
		if ti < 0 || ti > 1 {
			continue
		}
		e := easeOut(ti)
		x := float64(from.X) + float64(to.X-from.X)*e
		y := float64(from.Y) + float64(to.Y-from.Y)*e - math.Sin(e*math.Pi)*10
		blit(a.fb, chip, int(x)-cs/2, int(y)-cs/2)
	}
}

// fxFold throws the cards face down toward the muck, spinning smaller.
func (a *App) fxFold(f pkFX, g pkGeo, muck image.Point, t float64) {
	if t > fxFoldSecs {
		return
	}
	e := easeOut(t / fxFoldSecs)
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	for i := 0; i < f.ncard; i++ {
		sx := float64(g.cards.X + i*(a.A.Theme.CardFanStep+4) + cw/2)
		sy := float64(g.cards.Y + ch/2)
		x := sx + (float64(muck.X)-sx)*e
		y := sy + (float64(muck.Y)-sy)*e
		k := 1 - 0.6*e
		w, h := max(1, int(float64(cw)*k)), max(1, int(float64(ch)*k))
		blitScaled(a.fb, a.A.CardBack, image.Rect(int(x)-w/2, int(y)-h/2, int(x)-w/2+w, int(y)-h/2+h))
	}
}

// fxKnock draws two rings spreading out from the seat, like knuckles on the
// felt.
func (a *App) fxKnock(g pkGeo, t float64) {
	for k := 0; k < 2; k++ {
		tk := t - float64(k)*0.18
		if tk < 0 || tk > 0.4 {
			continue
		}
		rad := 18 + tk*40
		c := a.A.C("text")
		f := 1 - tk/0.4 // premultiplied alpha
		c = color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), uint8(255 * f)}
		for s := 0; s < 72; s++ {
			ang := float64(s) / 72 * 2 * math.Pi
			px := g.x + int(math.Round(rad*math.Cos(ang)))
			py := g.cy + int(math.Round(rad*math.Sin(ang)))
			fillRect(a.fb, image.Rect(px, py, px+1, py+1), c)
		}
	}
}
