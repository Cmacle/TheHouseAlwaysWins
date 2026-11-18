// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Red Dog (Acey-Deucey). Everyone antes, two cards come up, and the bet is
// that the third lands strictly between them (aces high). The gap sets the
// pay: one card 5 to 1, two 4 to 1, three 2 to 1, four or more even money.
// Before the third card, anyone can double their bet. Consecutive cards are
// a push; a pair draws a third card that pays 11 to 1 if it makes three of a
// kind (and pushes otherwise).
type RedDog struct {
	cards   []bj.Card
	spread  int // -1 consecutive, 0 pair, n cards between
	trips   bool
	hit     bool
	decided map[*csPlayer]bool
}

const rdAnte = "ante"

func (r *RedDog) game() string    { return proto.GameRedDog }
func (r *RedDog) spots() []string { return []string{rdAnte} }
func (r *RedDog) hello() string   { return banter.EvRdHello }
func (r *RedDog) reset() {
	r.cards, r.spread, r.trips, r.hit, r.decided = nil, 0, false, false, map[*csPlayer]bool{}
}

func rdRank(c bj.Card) int {
	if c.Rank == 1 {
		return 14
	}
	return int(c.Rank)
}

// rdPays is the multiple a spread pays.
func rdPays(spread int) int64 {
	switch spread {
	case 1:
		return 5
	case 2:
		return 4
	case 3:
		return 2
	}
	return 1
}

func rdPaysText(spread int) string { return fmt.Sprintf("%d to 1", rdPays(spread)) }

func (r *RedDog) deal(t *Casino) {
	r.cards = []bj.Card{t.draw(), t.draw()}
	t.show("reddog", 0, 2, func() {
		a, b := rdRank(r.cards[0]), rdRank(r.cards[1])
		gap := a - b
		if gap < 0 {
			gap = -gap
		}
		switch {
		case gap == 0:
			r.spread = 0
			t.message = "A pair! Three of a kind pays 11 to 1."
			t.say(banter.EvRdPair, nil, banter.Vars{}, 0.8, prioNormal)
			t.broadcast()
			r.third(t)
		case gap == 1:
			r.spread = -1
			t.message = "Consecutive cards: a push."
			t.say(banter.EvRdConsec, nil, banter.Vars{}, 0.6, prioLow)
			t.settle()
		default:
			r.spread = gap - 1
			t.message = fmt.Sprintf("Spread of %d pays %s. Raise to double your bet.", r.spread, rdPaysText(r.spread))
			t.say(banter.EvRdSpread, nil, banter.Vars{Amount: int64(r.spread), Item: rdPaysText(r.spread)}, 0.5, prioLow)
			t.phase = csRaise
			d := time.Duration(t.cc.RaiseSeconds) * time.Second
			t.setDeadline(d)
			t.broadcast()
			t.after(d, func() { r.closeRaise(t) })
		}
	})
}

// decide is a player's raise-or-stay.
func (r *RedDog) decide(t *Casino, p *csPlayer, raise bool) {
	if t.phase != csRaise || p.staked == 0 || r.decided[p] {
		return
	}
	if raise {
		ante := p.bets[rdAnte]
		if !t.store.Debit(p.user, ante) {
			t.errTo(p, "You don't have the chips to double that bet.")
			return
		}
		p.staked += ante
		p.bets["raise"] = ante
		p.raised = true
		t.say(banter.EvRdRaise, p, banter.Vars{}, 0.5, prioLow)
	}
	r.decided[p] = true
	for _, q := range t.seats {
		if q != nil && q.staked > 0 && !r.decided[q] {
			t.broadcast()
			return
		}
	}
	r.closeRaise(t)
}

func (r *RedDog) closeRaise(t *Casino) {
	if t.phase != csRaise {
		return
	}
	t.clearDeadline()
	t.phase = csDealing
	t.message = ""
	r.third(t)
}

func (r *RedDog) third(t *Casino) {
	r.cards = append(r.cards, t.draw())
	t.show("reddog", 2, 1, func() {
		a, b, c := rdRank(r.cards[0]), rdRank(r.cards[1]), rdRank(r.cards[2])
		if r.spread == 0 {
			r.trips = c == a
			if r.trips {
				t.message = "Three of a kind! 11 to 1!"
				t.say(banter.EvRdTrips, nil, banter.Vars{}, 1, prioHigh)
			} else {
				t.message = "No three of a kind: a push."
			}
			t.settle()
			return
		}
		lo, hi := min(a, b), max(a, b)
		r.hit = c > lo && c < hi
		if r.hit {
			t.message = fmt.Sprintf("In between! Pays %s.", rdPaysText(r.spread))
			if r.spread == 1 {
				t.say(banter.EvRdGutshot, nil, banter.Vars{}, 0.9, prioHigh)
			} else {
				var best *csPlayer
				for _, p := range t.seats {
					if p != nil && p.staked > 0 && (best == nil || p.staked > best.staked) {
						best = p
					}
				}
				if best != nil {
					t.say(banter.EvRdWin, best, banter.Vars{Amount: best.staked * rdPays(r.spread)}, 0.5, prioLow)
				}
			}
		} else {
			t.message = "Outside the spread. The house wins."
			t.say(banter.EvRdMiss, nil, banter.Vars{}, 0.4, prioLow)
		}
		t.settle()
	})
}

func (r *RedDog) payout(t *Casino, p *csPlayer) GameResult {
	res := GameResult{Wagered: p.staked}
	switch {
	case r.spread == -1:
		res.Payout = p.staked
	case r.spread == 0 && r.trips:
		res.Payout = p.staked * 12
		res.Trips = true
	case r.spread == 0:
		res.Payout = p.staked
	case r.hit:
		res.Payout = p.staked * (1 + rdPays(r.spread))
		res.Gutshot = r.spread == 1
	}
	return res
}

func (r *RedDog) view(t *Casino, st *proto.CasinoState) {
	for i, c := range r.cards {
		if t.hidden("reddog", i) {
			c = bj.Card{}
		}
		st.Cards = append(st.Cards, c)
	}
	if len(r.cards) >= 2 && !t.hidden("reddog", 0) {
		st.Spread = r.spread
		if r.spread > 0 {
			st.Pays = rdPaysText(r.spread)
		}
	}
}
