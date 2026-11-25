// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Baccarat (punto banco). Bet on the Player hand, the Banker hand or a tie.
// Cards count their face value, tens and pictures count nothing, and a
// hand's total is the last digit. The third-card rules are fixed. Player
// pays 1 to 1, Banker 1 to 1 less the commission, a tie 8 to 1 (Player and
// Banker bets push on a tie).
type Baccarat struct {
	player, banker []bj.Card
	winner         string
	natural        bool
	history        []string
}

const (
	bacPlayer = "player"
	bacBanker = "banker"
	bacTie    = "tie"
)

func (b *Baccarat) game() string    { return proto.GameBaccarat }
func (b *Baccarat) spots() []string { return []string{bacPlayer, bacTie, bacBanker} }
func (b *Baccarat) hello() string   { return banter.EvBacHello }
func (b *Baccarat) reset()          { b.player, b.banker, b.winner, b.natural = nil, nil, "", false }

// bacValue is a card's baccarat value.
func bacValue(c bj.Card) int {
	if c.Rank >= 10 {
		return 0
	}
	return int(c.Rank)
}

// bacTotal is a hand's total, counting only cards already face up for the
// view when hide says a card is still face down.
func bacTotal(cards []bj.Card) int {
	n := 0
	for _, c := range cards {
		n += bacValue(c)
	}
	return n % 10
}

// bankerDraws is the banker's third-card rule, given the player's third card
// (nil if the player stood).
func bankerDraws(banker int, p3 *bj.Card) bool {
	if p3 == nil {
		return banker <= 5
	}
	v := bacValue(*p3)
	switch banker {
	case 0, 1, 2:
		return true
	case 3:
		return v != 8
	case 4:
		return v >= 2 && v <= 7
	case 5:
		return v >= 4 && v <= 7
	case 6:
		return v == 6 || v == 7
	}
	return false
}

func (b *Baccarat) deal(t *Casino) {
	t.say(banter.EvBacDeal, nil, banter.Vars{}, 0.35, prioLow)
	b.player = []bj.Card{t.draw(), t.draw()}
	b.banker = []bj.Card{t.draw(), t.draw()}
	// the Player's two cards come out together, then the Banker's
	t.show(bacPlayer, 0, 2, func() {
		t.show(bacBanker, 0, 2, func() { b.thirdCards(t) })
	})
}

func (b *Baccarat) thirdCards(t *Casino) {
	pt, bt := bacTotal(b.player), bacTotal(b.banker)
	if pt >= 8 || bt >= 8 {
		b.natural = true
		item, amt := "Player", pt
		if bt > pt {
			item, amt = "Banker", bt
		}
		t.say(banter.EvBacNatural, nil, banter.Vars{Item: item, Total: amt}, 0.7, prioNormal)
		b.finish(t)
		return
	}
	bankerTurn := func(p3 *bj.Card) {
		if bankerDraws(bt, p3) {
			b.banker = append(b.banker, t.draw())
			t.show(bacBanker, 2, 1, func() { b.finish(t) })
			return
		}
		b.finish(t)
	}
	if pt <= 5 {
		b.player = append(b.player, t.draw())
		t.show(bacPlayer, 2, 1, func() {
			p3 := b.player[2]
			bankerTurn(&p3)
		})
		return
	}
	bankerTurn(nil)
}

func (b *Baccarat) finish(t *Casino) {
	pt, bt := bacTotal(b.player), bacTotal(b.banker)
	switch {
	case pt > bt:
		b.winner = bacPlayer
		t.message = "Player wins, " + itoa(pt) + " to " + itoa(bt)
		t.say(banter.EvBacPlayer, nil, banter.Vars{}, 0.4, prioLow)
	case bt > pt:
		b.winner = bacBanker
		t.message = "Banker wins, " + itoa(bt) + " to " + itoa(pt)
		t.say(banter.EvBacBanker, nil, banter.Vars{}, 0.4, prioLow)
	default:
		b.winner = bacTie
		t.message = "Tie, " + itoa(pt) + " all"
		t.say(banter.EvBacTie, nil, banter.Vars{}, 0.9, prioNormal)
	}
	b.history = append(b.history, b.winner)
	if len(b.history) > 40 {
		b.history = b.history[len(b.history)-40:]
	}
	// a streak worth mentioning
	if n := len(b.history); n >= 4 && b.winner != bacTie {
		run := 0
		for i := n - 1; i >= 0 && b.history[i] == b.winner; i-- {
			run++
		}
		if run >= 4 {
			name := "Player"
			if b.winner == bacBanker {
				name = "Banker"
			}
			t.say(banter.EvBacStreak, nil, banter.Vars{Item: name, Streak: run}, 0.5, prioLow)
		}
	}
	t.settle()
}

func (b *Baccarat) payout(t *Casino, p *csPlayer) GameResult {
	r := GameResult{Wagered: p.staked}
	for spot, amt := range p.bets {
		switch {
		case spot == bacTie && b.winner == bacTie:
			r.Payout += amt * 9
			r.Tie = true
		case spot == bacTie:
		case b.winner == bacTie:
			r.Payout += amt // Player and Banker push on a tie
		case spot == b.winner && spot == bacBanker:
			r.Payout += amt + amt*int64(100-t.cc.Commission)/100
		case spot == b.winner:
			r.Payout += amt * 2
		}
	}
	r.Natural = b.natural
	return r
}

func (b *Baccarat) view(t *Casino, st *proto.CasinoState) {
	for i, c := range b.player {
		if t.hidden(bacPlayer, i) {
			c = bj.Card{}
		}
		st.PlayerCards = append(st.PlayerCards, c)
	}
	for i, c := range b.banker {
		if t.hidden(bacBanker, i) {
			c = bj.Card{}
		}
		st.BankerCards = append(st.BankerCards, c)
	}
	st.PlayerTotal, st.BankerTotal = bacTotal(st.PlayerCards), bacTotal(st.BankerCards)
	if t.phase == csResult {
		st.Winner = b.winner
	}
	st.History = append([]string(nil), b.history...)
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
