package server

import (
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
)

// Dealer trash talk. Lines have a priority: a line only interrupts the one
// on screen if it's more important, or the current one has been up a while.
const (
	prioLow = iota
	prioNormal
	prioHigh
)

type speech struct {
	text  string
	id    int
	prio  int
	at    time.Time
	until time.Time
}

func cardName(c bj.Card) string {
	switch c.Rank {
	case 1:
		return "Ace"
	case 11:
		return "Jack"
	case 12:
		return "Queen"
	case 13:
		return "King"
	}
	return string("0123456789T"[c.Rank : c.Rank+1])
}

func upCardName(c bj.Card) string {
	if c.Rank == 10 {
		return "10"
	}
	return cardName(c)
}

// say makes the dealer speak, if trash talk is on and the dice allow it.
// It returns true if a line was said.
func (g *Game) say(ev string, p *player, v banter.Vars, chance float64, prio int) bool {
	if !g.cfg.DealerTrashTalk || g.banter == nil {
		return false
	}
	now := time.Now()
	cur := g.speech
	if now.Before(cur.until) {
		age := now.Sub(cur.at)
		if prio < cur.prio || (prio == cur.prio && age < 2500*time.Millisecond) || age < 1200*time.Millisecond {
			return false
		}
	}
	if g.banter.Chance() >= chance {
		return false
	}
	if p != nil {
		v.Name = p.user
		if prof, ok := g.store.Profile(p.user); ok {
			v.Net = prof.Net
			v.Balance = prof.Balance
		}
	}
	if v.Up == "" && len(g.dealer) > 0 {
		v.Up = upCardName(g.dealer[0])
	}
	line := g.banter.Line(ev, g.cfg.DealerVulgar, v)
	if line == "" {
		return false
	}
	d := 3500*time.Millisecond + time.Duration(len(line))*45*time.Millisecond
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	g.speech = speech{text: line, id: cur.id + 1, prio: prio, at: now, until: now.Add(d)}
	g.broadcast()
	return true
}

func (g *Game) greet(p *player) {
	prof, ok := g.store.Profile(p.user)
	if !ok {
		return
	}
	ev := banter.EvJoin
	switch {
	case prof.HandsPlayed == 0:
		ev = banter.EvJoinNew
	case prof.Net <= -1000:
		ev = banter.EvJoinDown
	case prof.Net >= 1000:
		ev = banter.EvJoinUp
	}
	g.say(ev, p, banter.Vars{}, 1, prioNormal)
}

// chatter runs at the start of a betting round: roast somebody's lifetime
// score, brag about the house, or ramble.
func (g *Game) chatter() {
	if !g.cfg.DealerTrashTalk {
		return
	}
	r := g.banter.Chance()
	var seated []*player
	for _, p := range g.seats {
		if p != nil && p.conn != nil {
			seated = append(seated, p)
		}
	}
	switch {
	case r < 0.30 && len(seated) > 0:
		p := seated[g.banter.Intn(len(seated))]
		prof, _ := g.store.Profile(p.user)
		rich, _ := g.store.Tops()
		switch {
		case strings.EqualFold(g.store.Worst(), p.user):
			g.say(banter.EvRoastWorst, p, banter.Vars{}, 1, prioLow)
		case strings.EqualFold(rich, p.user):
			g.say(banter.EvRoastCrown, p, banter.Vars{}, 1, prioLow)
		case prof.Net <= -500:
			g.say(banter.EvRoastDown, p, banter.Vars{}, 1, prioLow)
		case prof.Net >= 500:
			g.say(banter.EvRoastUp, p, banter.Vars{}, 1, prioLow)
		default:
			g.say(banter.EvIdle, nil, banter.Vars{}, 1, prioLow)
		}
	case r < 0.38:
		h := g.store.House()
		if h.Net >= 0 {
			g.say(banter.EvHouseUp, nil, banter.Vars{Amount: h.Net}, 1, prioLow)
		} else {
			g.say(banter.EvHouseDown, nil, banter.Vars{Amount: -h.Net}, 1, prioLow)
		}
	case r < 0.55:
		g.say(banter.EvIdle, nil, banter.Vars{}, 1, prioLow)
	}
}

// reactToAction comments on a player's decision. before is the hand total
// (and softness) before the action.
func (g *Game) reactToAction(p *player, act string, h *bj.Hand, before int, soft bool, splitTens bool) {
	if !g.cfg.DealerTrashTalk {
		return
	}
	t, _ := h.Total()
	upVal := 0
	if len(g.dealer) > 0 {
		upVal = g.dealer[0].Value()
		if g.dealer[0].Rank == 1 {
			upVal = 11
		}
	}
	switch act {
	case "stand":
		switch {
		case before <= 11:
			g.say(banter.EvStandTiny, p, banter.Vars{Total: before}, 0.95, prioHigh)
		case before <= 16 && !soft && upVal >= 7:
			g.say(banter.EvCoward, p, banter.Vars{Total: before}, 0.6, prioNormal)
		case before <= 16 && soft:
			g.say(banter.EvCoward, p, banter.Vars{Total: before}, 0.5, prioNormal)
		}
	case "hit", "double":
		greedy := act == "hit" && before >= 17 && !soft
		switch {
		case t > 21 && greedy:
			g.say(banter.EvBustGreedy, p, banter.Vars{Total: t}, 1, prioHigh)
		case t > 21:
			g.say(banter.EvBust, p, banter.Vars{Total: t}, 0.8, prioHigh)
		case greedy:
			g.say(banter.EvHit17, p, banter.Vars{Total: before}, 0.7, prioNormal)
		case act == "double":
			g.say(banter.EvDouble, p, banter.Vars{}, 0.35, prioLow)
		}
	case "split":
		if splitTens {
			g.say(banter.EvSplitTens, p, banter.Vars{}, 0.95, prioHigh)
		}
	case "surrender":
		g.say(banter.EvSurrender, p, banter.Vars{}, 0.9, prioNormal)
	}
}

type roundResult struct {
	p          *player
	delta      int64
	before     int64 // balance before the round (stake included)
	doubleLost bool
}

// reactToResults picks the juiciest thing to say about a settled round.
func (g *Game) reactToResults(res []roundResult, dealerBust, dealerBJ bool) {
	if !g.cfg.DealerTrashTalk || len(res) == 0 {
		return
	}
	// someone went broke
	for _, r := range res {
		if g.store.Balance(r.p.user) < g.cfg.MinBet {
			if g.say(banter.EvBroke, r.p, banter.Vars{}, 0.85, prioHigh) {
				return
			}
		}
	}
	var loser, winner *roundResult
	allLost := true
	for i := range res {
		r := &res[i]
		if loser == nil || r.delta < loser.delta {
			loser = r
		}
		if winner == nil || r.delta > winner.delta {
			winner = r
		}
		if r.delta >= 0 {
			allLost = false
		}
	}
	if loser != nil && loser.delta < 0 {
		lost := -loser.delta
		if lost >= 200 && (lost >= 500 || loser.before <= 0 || float64(lost) >= 0.3*float64(loser.before)) {
			if g.say(banter.EvBigLoss, loser.p, banter.Vars{Amount: lost}, 0.9, prioHigh) {
				return
			}
		}
	}
	if dealerBust && g.say(banter.EvDealerBust, nil, banter.Vars{}, 0.7, prioNormal) {
		return
	}
	for _, r := range res {
		if r.doubleLost && g.say(banter.EvDoubleLose, r.p, banter.Vars{}, 0.6, prioNormal) {
			return
		}
	}
	if winner != nil && winner.delta >= 500 && g.say(banter.EvBigWin, winner.p, banter.Vars{Amount: winner.delta}, 0.6, prioNormal) {
		return
	}
	if allLost && len(res) >= 2 && !dealerBJ && g.say(banter.EvSweep, nil, banter.Vars{}, 0.8, prioNormal) {
		return
	}
	for _, r := range res {
		if s := g.store.LoseStreak(r.p.user); s >= 4 {
			if g.say(banter.EvLoseStreak, r.p, banter.Vars{Streak: s}, 0.5, prioNormal) {
				return
			}
		}
	}
}
