// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"math/rand"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Card reveal: a card drawn after the deal stays face down while everyone
// watches it, big, in the middle of the table, then flips after a random
// wait. The game pauses meanwhile.

type revealing struct {
	seat, hand, card int // seat -1 is the dealer
	until            time.Time
	shown            bool // flipped; the game waits while it's on show
}

// revealSettle is how long the game waits after a flip: the card stays big
// and face up, then slides back into the hand (the client's animation).
// Without it the next reveal would start on top of this one.
var revealSettle = 1400 * time.Millisecond

func (g *Game) revealDelay() time.Duration {
	lo, hi := g.cfg.CardRevealMinSeconds, g.cfg.CardRevealMaxSeconds
	s := lo
	if hi > lo {
		s += rand.Float64() * (hi - lo)
	}
	return time.Duration(s * float64(time.Second))
}

// startReveal holds the game while a card is revealed. When it flips,
// outcome says how it went (for the sound effects) and then carries on. A
// running turn or play clock is paused and picks up where it was.
func (g *Game) startReveal(seat, hand, card int, outcome func() string, then func()) {
	d := g.revealDelay()
	savedFn := g.timerFn
	var savedLeft time.Duration
	if !g.deadline.IsZero() {
		savedLeft = time.Until(g.deadline)
	}
	g.stopTimer()
	g.clearDeadline()
	g.reveal = &revealing{seat: seat, hand: hand, card: card, until: time.Now().Add(d)}
	g.broadcast()
	g.after(d, func() {
		g.reveal.shown = true // face up now, still no actions
		g.eventAll(proto.Event{Kind: "reveal", Seat: seat, Text: outcome()})
		g.broadcast()
		g.after(revealSettle, func() {
			g.reveal = nil
			then()
			if g.timer == nil && savedFn != nil && g.phase == proto.PhasePlaying {
				left := max(savedLeft, 5*time.Second)
				g.setDeadline(left)
				g.after(left, savedFn)
				g.broadcast()
			}
		})
	})
}

// playerOutcome rates a player's hand right after the card turned over.
func playerOutcome(h *bj.Hand) string {
	t, _ := h.Total()
	switch {
	case t > 21:
		return "bust"
	case t == 21:
		return "21"
	case t >= 19:
		return "great"
	case t >= 17:
		return "good"
	case t >= 12:
		return "meh"
	default:
		return "safe"
	}
}

// dealerOutcome rates the dealer's card from the players' point of view.
func (g *Game) dealerOutcome() string {
	t, _ := bj.Total(g.dealer)
	switch {
	case t > 21:
		return "dealer_bust" // everybody cheers
	case t >= 20:
		return "dealer_strong" // groans
	case t >= 17:
		return "dealer_ok"
	case g.dealerNeedsToDraw():
		return "dealer_more" // he has to draw again
	default:
		return "dealer_ok"
	}
}
