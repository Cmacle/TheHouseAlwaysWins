// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// talkBudget is how many small remarks he makes in one hand. Big moments
// (busts, blackjacks, broke players) don't count against it.
func (g *Game) talkBudget() int {
	if g.cfg.TurboMode {
		return 5
	}
	return 3
}

// reactToBet comments on a bet as it's placed: how it compares with the
// player's balance and with what they did last hand.
func (g *Game) reactToBet(p *player, amt int64) {
	if !g.cfg.DealerTrashTalk {
		return
	}
	bal := g.store.Balance(p.user)
	min := g.cfg.MinBet
	v := banter.Vars{Amount: amt}
	switch {
	case amt == bal && bal >= min*5:
		g.say(banter.EvAllIn, p, v, 0.9, prioNormal)
	case amt == g.cfg.MaxBet:
		g.say(banter.EvMaxBet, p, v, 0.5, prioNormal)
	case p.prevDelta <= -200 && p.prevBet > 0 && amt > p.prevBet:
		g.say(banter.EvChase, p, banter.Vars{Amount: -p.prevDelta}, 0.55, prioNormal)
	case p.prevBet > 0 && amt >= 2*p.prevBet && amt >= 3*min:
		g.say(banter.EvBetUp, p, v, 0.45, prioNormal)
	case p.prevBet > 0 && amt*2 <= p.prevBet && p.prevDelta < 0:
		g.say(banter.EvBetDown, p, v, 0.45, prioLow)
	case amt >= bal/4 && amt >= min*10:
		g.say(banter.EvBetBig, p, v, 0.35, prioLow)
	case amt == min:
		g.say(banter.EvMinBet, p, v, 0.3, prioLow)
	default:
		placed, waiting := 0, 0
		for _, o := range g.seats {
			if o == nil || !o.eligible || o == p || o.conn == nil {
				continue
			}
			if o.betPlaced {
				placed++
			} else {
				waiting++
			}
		}
		switch {
		case placed == 0 && waiting > 0:
			g.say(banter.EvFirstBet, p, v, 0.15, prioLow)
		case waiting == 0 && placed > 0 && g.timeLeft() < g.cfg.Timing().Betting/3:
			g.say(banter.EvLastBet, p, v, 0.45, prioLow)
		}
	}
}

func (g *Game) timeLeft() time.Duration {
	if g.deadline.IsZero() {
		return time.Hour
	}
	return time.Until(g.deadline)
}

// nudge runs every second: nags slow players and dares people to bet big.
func (g *Game) nudge() {
	if !g.cfg.DealerTrashTalk || g.seatedCount() == 0 {
		return
	}
	switch g.phase {
	case proto.PhaseBetting:
		// a few seconds in: dare somebody who bet small last time to go big
		if !g.dared && time.Since(g.bettingAt) > 3*time.Second {
			g.dared = true
			if p, amt := g.dareTarget(); p != nil {
				g.say(banter.EvDare, p, banter.Vars{Amount: amt}, 0.35, prioLow)
			}
		}
		if !g.betNudged && g.timeLeft() <= 6*time.Second {
			g.betNudged = true
			for _, p := range g.seats {
				if p == nil || !p.eligible || p.betPlaced || p.conn == nil {
					continue
				}
				if g.cfg.BettingMode == "turns" && p.seat != g.betTurn {
					continue
				}
				g.say(banter.EvBetSlow, p, banter.Vars{}, 0.6, prioNormal)
				break
			}
		}
	case proto.PhasePlaying:
		if g.cfg.PlayMode != "turns" || g.activeSeat < 0 || g.activeSeat >= len(g.seats) {
			return
		}
		p := g.seats[g.activeSeat]
		if p == nil || p.conn == nil || g.timeLeft() > 6*time.Second {
			return
		}
		key := p.user + ":" + string(rune('0'+curHand(p)))
		if g.turnNudged != key {
			g.turnNudged = key
			g.say(banter.EvTurnSlow, p, banter.Vars{}, 0.5, prioNormal)
		}
	}
}

// dareTarget picks a player still betting who bet small last hand, and a
// bigger amount to dare them with.
func (g *Game) dareTarget() (*player, int64) {
	var best *player
	var bestAmt int64
	for _, p := range g.seats {
		if p == nil || !p.eligible || p.conn == nil || p.prevBet <= 0 {
			continue
		}
		bal := g.store.Balance(p.user)
		if p.prevBet*10 > bal {
			continue // already betting big for them
		}
		amt := roundBet(p.prevBet*5, g.cfg.MinBet)
		amt = min(amt, g.cfg.MaxBet, bal)
		if amt <= p.prevBet {
			continue
		}
		if best == nil || g.banter.Chance() < 0.5 {
			best, bestAmt = p, amt
		}
	}
	return best, bestAmt
}

// roundBet rounds to a nice number of chips.
func roundBet(v, minBet int64) int64 {
	for _, step := range []int64{1000, 500, 100, 50, 25, 10, 5} {
		if v >= step*4 {
			return max(v/step*step, minBet)
		}
	}
	return max(v, minBet)
}

// reactToUpCard comments on the dealer's up card once everyone has cards.
func (g *Game) reactToUpCard() {
	if !g.cfg.DealerTrashTalk || len(g.dealer) == 0 {
		return
	}
	up := g.dealer[0]
	switch {
	case up.Rank == 1:
		g.say(banter.EvDealerAce, nil, banter.Vars{Up: upCardName(up)}, 0.35, prioLow)
	case up.Value() >= 4 && up.Value() <= 6:
		g.say(banter.EvDealerWeak, nil, banter.Vars{Up: upCardName(up)}, 0.3, prioLow)
	}
}

// reactToLeave: somebody walked away from the table.
func (g *Game) reactToLeave(p *player) {
	if !g.cfg.DealerTrashTalk || p.seat < 0 {
		return
	}
	others := 0
	for _, o := range g.seats {
		if o != nil && o != p && o.conn != nil {
			others++
		}
	}
	if others > 0 {
		v := banter.Vars{}
		if p.prevDelta < 0 {
			v.Amount = -p.prevDelta
		}
		g.say(banter.EvLeave, p, v, 0.55, prioNormal)
	}
}

// reactToRefill: the daily refill just landed.
func (g *Game) reactToRefill(p *player, amount int64) {
	if g.cfg.DealerTrashTalk && p != nil && p.seat >= 0 {
		g.say(banter.EvRefill, p, banter.Vars{Amount: amount}, 0.8, prioNormal)
	}
}
