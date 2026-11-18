// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/poker"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

type atomicInfo = atomic.Pointer[proto.TableInfo]

// PokerHandRecord builds what the store records for one player's hand.
func PokerHandRecord(delta, stack, put, won int64, cat int, bluff, beatDale bool) PokerHand {
	return PokerHand{Delta: delta, Stack: stack, Put: put, Pot: won, Category: cat, Bluff: bluff, BeatDale: beatDale}
}

// ---------------------------------------------------------------- Dale plays

// daleAct is Dale's decision. He estimates his chance of winning against
// the hands still in, compares it with the price of calling, and adds a bit
// of personality: he raises his good hands, bluffs now and then, and hates
// folding to small bets.
func (t *Holdem) daleAct(d *hPlayer) {
	if t.active != d.seat || d.folded || t.phase == pkShowdown || t.phase == pkWaiting {
		return
	}
	opp := len(t.live()) - 1
	sims := 350
	if len(t.board) == 0 {
		sims = 250
	}
	eq := poker.Equity(d.hole, t.board, opp, sims, t.rng)
	toCall := t.curBet - d.bet
	pot := t.pot()
	r := t.rng.Float64()
	maxTotal := d.bet + d.stack
	raiseTo := func(frac float64) int64 {
		size := int64(float64(pot+toCall) * frac)
		target := t.curBet + max(t.minRaise, size)
		// round to the big blind so it looks human
		bb := t.hc.BigBlind
		target = (target + bb/2) / bb * bb
		return min(max(target, t.curBet+t.minRaise), maxTotal)
	}
	// a little bit braver heads up
	edge := 0.0
	if opp == 1 {
		edge = 0.05
	}
	bluffing := false
	act, amt := "", int64(0)
	switch {
	case toCall <= 0:
		switch {
		case eq > 0.72-edge && r < 0.85:
			act, amt = "bet", raiseTo(0.55+0.4*t.rng.Float64())
		case eq > 0.55-edge && r < 0.45:
			act, amt = "bet", raiseTo(0.4+0.25*t.rng.Float64())
		case r < 0.10+edge && len(t.board) >= 3: // a stab at it
			act, amt = "bet", raiseTo(0.5)
			bluffing = true
		default:
			act = "check"
		}
	default:
		odds := float64(toCall) / float64(pot+toCall)
		cheap := toCall <= t.hc.BigBlind*2 && toCall*20 < d.stack
		switch {
		case eq > 0.80-edge && d.canRais && r < 0.75:
			act, amt = "raise", raiseTo(0.7+0.5*t.rng.Float64())
		case eq > 0.65-edge && d.canRais && r < 0.25:
			act, amt = "raise", raiseTo(0.6)
		case eq >= odds+0.04 || (cheap && eq >= odds-0.08):
			act = "call"
		case d.canRais && r < 0.04+edge/2 && len(t.board) >= 3 && toCall*4 < d.stack:
			act, amt = "raise", raiseTo(0.9)
			bluffing = true
		default:
			act = "fold"
		}
	}
	if (act == "bet" || act == "raise") && amt >= maxTotal {
		act = "allin"
	}
	if amt >= maxTotal*3/4 && act != "fold" && act != "check" && act != "call" && eq < 0.5 && !bluffing {
		act = "call" // don't shove a mediocre hand
	}
	d.dbgBluff = bluffing
	switch act {
	case "fold":
		t.voice.say(banter.EvPkDaleFold, t.vulgar(), t.vars(nil), 0.55, prioNormal)
	case "allin":
		t.voice.say(banter.EvPkDaleAllIn, t.vulgar(), t.vars(nil), 1, prioHigh)
	case "bet", "raise":
		t.voice.say(banter.EvPkDaleRaise, t.vulgar(), t.vars(nil), 0.4, prioNormal)
	case "call":
		if toCall >= t.hc.BigBlind*10 {
			t.voice.say(banter.EvPkDaleCall, t.vulgar(), t.vars(nil), 0.45, prioNormal)
		}
	case "check":
		t.voice.say(banter.EvPkDaleCheck, t.vulgar(), t.vars(nil), 0.12, prioLow)
	}
	t.apply(d, act, amt)
}

// ---------------------------------------------------------------- talking

func (t *Holdem) vulgar() bool { return t.cfg.DealerVulgar }

func (t *Holdem) vars(p *hPlayer) banter.Vars {
	v := banter.Vars{}
	if p != nil && !p.dale {
		v.Name = p.user
		if prof, ok := t.store.Profile(p.user); ok {
			v.Net, v.Balance = prof.Net, prof.Balance
		}
	}
	return v
}

// say has Dale react to a player.
func (t *Holdem) say(ev string, p *hPlayer, v banter.Vars, chance float64, prio int) bool {
	if !t.cfg.DealerTrashTalk {
		return false
	}
	base := t.vars(p)
	if v.Amount != 0 {
		base.Amount = v.Amount
	}
	if v.Item != "" {
		base.Item = v.Item
	}
	if v.Total != 0 {
		base.Total = v.Total
	}
	if t.voice.say(ev, t.vulgar(), base, chance, prio) {
		t.broadcast()
		return true
	}
	return false
}

func (t *Holdem) reactToBet(p *hPlayer, target int64, allIn bool) {
	if p.dale {
		return
	}
	switch {
	case allIn:
		t.say(banter.EvPkAllIn, p, banter.Vars{Amount: target}, 0.9, prioHigh)
	case target >= t.hc.BigBlind*8:
		t.say(banter.EvPkRaise, p, banter.Vars{Amount: target}, 0.5, prioNormal)
	default:
		t.say(banter.EvPkRaise, p, banter.Vars{Amount: target}, 0.2, prioLow)
	}
}

func (t *Holdem) reactToCall(p *hPlayer, act string, toCall int64) {
	if p.dale {
		return
	}
	switch {
	case act == "check":
		t.say(banter.EvPkCheck, p, banter.Vars{}, 0.08, prioLow)
	case toCall >= t.hc.BigBlind*10:
		t.say(banter.EvPkCall, p, banter.Vars{Amount: toCall}, 0.45, prioNormal)
	}
}

func (t *Holdem) reactToFold(p *hPlayer) {
	if p.dale {
		return
	}
	chance := 0.15
	if p.put >= t.hc.BigBlind*6 {
		chance = 0.5 // folded after putting money in
	}
	t.say(banter.EvPkFold, p, banter.Vars{Amount: p.put}, chance, prioLow)
}

func (t *Holdem) reactToFlop() {
	if len(t.board) < 3 {
		return
	}
	// a monotone or paired flop gets a comment now and then
	b := t.board
	mono := b[0].Suit == b[1].Suit && b[1].Suit == b[2].Suit
	pair := b[0].Rank == b[1].Rank || b[1].Rank == b[2].Rank || b[0].Rank == b[2].Rank
	if mono || pair {
		t.say(banter.EvPkFlop, t.humanIn(), banter.Vars{}, 0.4, prioLow)
	}
}

// reactToResults comments on how the hand went.
func (t *Holdem) reactToResults(showdown bool, bluffs map[*hPlayer]bool, daleDelta int64) {
	dale := t.seats[0]
	var winner *hPlayer
	var bigPot int64
	for _, p := range t.seats {
		if p != nil && p.won > bigPot {
			winner, bigPot = p, p.won
		}
	}
	if winner == nil {
		return
	}
	// a royal flush beats any other comment
	for _, p := range t.seats {
		if p != nil && p.inHand && !p.folded && showdown && len(t.board) == 5 {
			if sc, _ := poker.Best(append(append([]poker.Card{}, p.hole...), t.board...)); sc.Category() == poker.RoyalFlush {
				t.say(banter.EvPkRoyal, p, banter.Vars{}, 1, prioHigh)
				return
			}
		}
	}
	switch {
	case winner.dale && !showdown && dale.dbgBluff:
		// he stole it: sometimes he shows the bluff
		if t.rng.Float64() < 0.5 {
			dale.show = true
			t.say(banter.EvPkDaleBluff, t.humanIn(), banter.Vars{Amount: bigPot}, 1, prioHigh)
			return
		}
		t.say(banter.EvPkDaleWin, t.humanIn(), banter.Vars{Amount: bigPot}, 0.7, prioNormal)
	case winner.dale:
		loser := t.humanIn()
		ev := banter.EvPkDaleWin
		if showdown {
			// a bad beat: the human had a strong hand and still lost
			for _, p := range t.humans() {
				if p.inHand && !p.folded {
					if sc, _ := poker.Best(append(append([]poker.Card{}, p.hole...), t.board...)); sc.Category() >= poker.Trips {
						ev, loser = banter.EvPkBadBeat, p
					}
				}
			}
		}
		t.say(ev, loser, banter.Vars{Amount: bigPot, Item: winner.result}, 0.85, prioHigh)
	case showdown && dale.inHand && !dale.folded:
		t.say(banter.EvPkDaleLose, winner, banter.Vars{Amount: bigPot, Item: winner.result}, 0.9, prioHigh)
	case showdown:
		if sc, _ := poker.Best(append(append([]poker.Card{}, winner.hole...), t.board...)); sc.Category() >= poker.FullHouse {
			t.say(banter.EvPkMonster, winner, banter.Vars{Item: sc.Name(), Amount: bigPot}, 0.9, prioHigh)
			return
		}
		t.say(banter.EvPkWin, winner, banter.Vars{Amount: bigPot, Item: winner.result}, 0.6, prioNormal)
	case bluffs[winner]:
		t.say(banter.EvPkWin, winner, banter.Vars{Amount: bigPot}, 0.5, prioNormal)
	case bigPot >= t.hc.BigBlind*40:
		t.say(banter.EvPkBigPot, winner, banter.Vars{Amount: bigPot}, 0.8, prioHigh)
	default:
		t.say(banter.EvPkWin, winner, banter.Vars{Amount: bigPot}, 0.3, prioLow)
	}
	_ = daleDelta
}

// ---------------------------------------------------------------- admin commands

// runAdminCommand runs a /command for an admin and sends them the output.
func runAdminCommand(lg interface{ Printf(string, ...any) }, exec func(string, io.Writer), c *conn, user, line string) {
	if exec == nil {
		return
	}
	lg.Printf("admin %s: %s", user, line)
	go func() {
		var b lineBuffer
		guard(lg.Printf, "admin command", nil, func() { exec(line, &b) })
		out := b.lines()
		if len(out) == 0 {
			out = []string{"  done"}
		}
		c.send(proto.SConsole, proto.Console{Lines: append([]string{"> " + line}, out...)})
	}()
}

// lineBuffer collects command output.
type lineBuffer struct{ buf []byte }

func (l *lineBuffer) Write(p []byte) (int, error) { l.buf = append(l.buf, p...); return len(p), nil }

func (l *lineBuffer) lines() []string {
	var out []string
	cur := ""
	for _, b := range l.buf {
		if b == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(b)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

var _ = time.Second
