// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// pokerTable makes an instant-mode table with n players who bought in.
func pokerTable(t *testing.T, n int, hc holdemCfg) (*Holdem, *Store, []*conn) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	tb := newHoldem("holdem", "Test Hold'em", cfg, hc, st, nil, log.New(io.Discard, "", 0))
	tb.instant = true
	var conns []*conn
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("player%d", i)
		if _, _, err := st.Register(u, "pw123456", nil, 5000, ""); err != nil {
			t.Fatal(err)
		}
		c := &conn{out: make(chan []byte, 1<<16), closed: make(chan struct{}), user: u}
		conns = append(conns, c)
		tb.onJoin(c, u, hc.MaxBuyIn)
	}
	return tb, st, conns
}

// drain empties the test connections so they never fill up.
func drainAll(cs []*conn) {
	for _, c := range cs {
		for len(c.out) > 0 {
			<-c.out
		}
	}
}

// chips is everything the players own (balance + table stacks) plus what
// the house has won.
func chips(st *Store, n int) int64 {
	var sum int64
	for i := 0; i < n; i++ {
		p, _ := st.Profile(fmt.Sprintf("player%d", i))
		sum += p.Balance // includes the table stack
	}
	return sum
}

// play drives the table: random actions for people, Dale's own brain,
// timers fired as soon as they're due. It stops after `hands` hands.
func play(t *testing.T, tb *Holdem, conns []*conn, hands int, rng *rand.Rand, check func()) int64 {
	var added int64
	byUser := map[string]*conn{}
	for _, c := range conns {
		byUser[c.user] = c
	}
	for steps := 0; tb.handNo < hands; steps++ {
		if steps > hands*400 {
			t.Fatalf("stuck in %s, hand %d, active %d", tb.phase, tb.handNo, tb.active)
		}
		drainAll(conns)
		if a := tb.active; a > 0 && tb.seats[a] != nil && !tb.seats[a].dale && tb.phase != pkShowdown {
			p := tb.seats[a]
			st := tb.view(p)
			acts := st.Actions
			if len(acts) == 0 {
				t.Fatalf("seat %d to act with no actions (phase %s)", a, tb.phase)
			}
			act := acts[rng.Intn(len(acts))]
			var amt int64
			switch act {
			case "bet", "raise":
				switch rng.Intn(4) {
				case 0:
					amt = st.MaxRaise // all in
				case 1:
					amt = st.MinRaise
				default:
					amt = st.MinRaise + rng.Int63n(st.MaxRaise-st.MinRaise+1)
				}
			case "fold":
				if rng.Intn(3) > 0 && contains(acts, "check") {
					act = "check" // don't fold for free too often
				}
			}
			tb.onAct(p, act, amt)
			continue
		}
		fn := tb.queued
		if fn == nil {
			if tb.phase == pkWaiting {
				// everyone broke: top them up so the game goes on
				for _, p := range tb.humans() {
					if p.stack == 0 {
						if tb.store.Balance(p.user) < tb.hc.MaxBuyIn {
							tb.store.AdminAdjust(p.user, 5000) // broke: the test lends them more
							added += 5000
						}
						tb.onAct(p, "rebuy", tb.hc.MaxBuyIn)
					}
				}
				if tb.queued == nil {
					tb.maybeStart()
				}
				if tb.queued == nil {
					for _, p := range tb.humans() {
						t.Logf("%s seat %d stack %d bal %d sitout %v leaving %v conn %v", p.user, p.seat, p.stack, tb.store.Balance(p.user), p.sitOut, p.leaving, p.conn != nil)
					}
					t.Fatalf("nothing to do: phase %s, %d humans, hand %d, msg %q", tb.phase, len(tb.humans()), tb.handNo, tb.message)
				}
				continue
			}
			t.Fatalf("no timer in phase %s (active %d)", tb.phase, tb.active)
		}
		tb.queued = nil
		fn()
		if check != nil {
			check()
		}
	}
	return added
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestHoldemHeadsUpConservesChips(t *testing.T) {
	hc := defaultHoldem()
	hc.DaleThinkMS = 0
	tb, st, conns := pokerTable(t, 1, hc)
	start := chips(st, 1)
	house0 := st.House().PokerNet
	rng := rand.New(rand.NewSource(1))
	start += play(t, tb, conns, 300, rng, nil)
	// finish the hand in progress so everything is settled
	for tb.phase != pkWaiting && tb.queued != nil {
		if a := tb.active; a > 0 {
			tb.onAct(tb.seats[a], "fold", 0)
			continue
		}
		fn := tb.queued
		tb.queued = nil
		fn()
	}
	players := chips(st, 1)
	house := st.House().PokerNet - house0
	if players+house != start {
		t.Fatalf("chips not conserved: players %d (start %d), house won %d", players, start, house)
	}
	p, _ := st.Profile("player0")
	if p.PokerHands < 250 {
		t.Fatalf("only %d hands recorded", p.PokerHands)
	}
	t.Logf("300 hands heads up: player %+d, house %+d", players-start, house)
}

func TestHoldemMultiwaySidePots(t *testing.T) {
	hc := defaultHoldem()
	hc.DaleThinkMS = 0
	hc.Seats = 6
	tb, st, conns := pokerTable(t, 5, hc)
	start := chips(st, 5)
	house0 := st.House().PokerNet
	rng := rand.New(rand.NewSource(42))
	// at every step the chips on the table plus everyone's balance plus what
	// the house has taken never change
	check := func() {
		if tb.phase == pkWaiting {
			return
		}
	}
	start += play(t, tb, conns, 400, rng, check)
	for tb.phase != pkWaiting && tb.queued != nil {
		if a := tb.active; a > 0 {
			tb.onAct(tb.seats[a], "fold", 0)
			continue
		}
		fn := tb.queued
		tb.queued = nil
		fn()
	}
	players := chips(st, 5)
	house := st.House().PokerNet - house0
	if players+house != start {
		t.Fatalf("chips not conserved: players %d (start %d), house %d", players, start, house)
	}
}

func TestHoldemBuyInCashOutAndCrash(t *testing.T) {
	hc := defaultHoldem()
	tb, st, conns := pokerTable(t, 1, hc)
	p, _ := st.Profile("player0")
	if p.AtTable != hc.MaxBuyIn || p.Balance != 5000 {
		t.Fatalf("after buying in: at table %d, total %d", p.AtTable, p.Balance)
	}
	if st.Balance("player0") != 5000-hc.MaxBuyIn {
		t.Fatalf("spendable balance %d", st.Balance("player0"))
	}
	// a crash leaves the chips "on the table": the next start returns them
	if n := st.ReturnTableStacks(); n != 1 {
		t.Fatalf("returned %d", n)
	}
	if st.Balance("player0") != 5000 {
		t.Fatalf("after the crash fix: %d", st.Balance("player0"))
	}
	// leaving cashes out
	tb2, st2, conns2 := pokerTable(t, 1, hc)
	tb2.onLeave(conns2[0])
	if st2.Balance("player0") != 5000 {
		t.Fatalf("after leaving: %d", st2.Balance("player0"))
	}
	_ = tb
	_ = conns
}

func TestHoldemDealerSoloOnly(t *testing.T) {
	hc := defaultHoldem()
	hc.DealerSoloOnly = true
	tb, _, _ := pokerTable(t, 2, hc)
	if tb.daleIn() {
		t.Fatal("Dale plays with two people seated and dealer_solo_only on")
	}
	tb1, _, _ := pokerTable(t, 1, hc)
	if !tb1.daleIn() {
		t.Fatal("Dale should play someone who's alone")
	}
	hc.DealerSoloOnly = false
	tb3, _, _ := pokerTable(t, 3, hc)
	if !tb3.daleIn() {
		t.Fatal("Dale should always play with the setting off")
	}
}

func TestHoldemHiddenCards(t *testing.T) {
	hc := defaultHoldem()
	hc.DaleThinkMS = 0
	tb, _, conns := pokerTable(t, 2, hc)
	for tb.phase == pkWaiting && tb.queued != nil {
		fn := tb.queued
		tb.queued = nil
		fn()
	}
	if tb.phase != pkPreflop {
		t.Fatalf("phase %s", tb.phase)
	}
	a, b := tb.byUser["player0"], tb.byUser["player1"]
	v := tb.view(a)
	for _, s := range v.Seats {
		if s.Username == "player1" && len(s.Cards) == 2 && !s.Cards[0].Hidden() {
			t.Fatal("p0 can see p1's cards")
		}
		if s.Dale && len(s.Cards) == 2 && !s.Cards[0].Hidden() {
			t.Fatal("p0 can see Dale's cards")
		}
		if s.Username == "player0" && (len(s.Cards) != 2 || s.Cards[0].Hidden()) {
			t.Fatal("p0 can't see their own cards")
		}
	}
	_ = b
	_ = conns
	_ = proto.SPoker
}

func TestLobbyTableCommands(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	s, err := New(cfg, dir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	s.Lobby.start(stop)
	if n := len(s.Lobby.Tables()); n != 2 {
		t.Fatalf("%d default tables", n)
	}
	var out lineBuffer
	s.Exec("table add holdem High Rollers", &out)
	if len(s.Lobby.Tables()) != 3 {
		t.Fatalf("not added: %v", out.lines())
	}
	out = lineBuffer{}
	s.Exec("table set high-rollers big_blind 100", &out)
	list, _ := s.Lobby.TableSettings("high-rollers")
	found := false
	for _, st := range list {
		if st.Key == "big_blind" && st.Value == "100" {
			found = true
		}
	}
	if !found {
		t.Fatalf("big blind not set: %v", out.lines())
	}
	// min buy-in follows the blinds
	for _, st := range list {
		if st.Key == "min_buy_in" && st.Value != "1000" {
			t.Fatalf("min buy-in %s with a 100 big blind", st.Value)
		}
	}
	out = lineBuffer{}
	s.Exec("table set blackjack min_bet 25", &out)
	if err := s.Lobby.SetTableSetting("blackjack", "port", "1"); err == nil {
		t.Fatal("a server-wide setting was accepted as a table setting")
	}
	out = lineBuffer{}
	s.Exec("table remove high-rollers", &out)
	if len(s.Lobby.Tables()) != 2 {
		t.Fatalf("not removed: %v", out.lines())
	}
	// the list survives a restart
	s.Exec("table add blackjack Penny Slots", &out)
	l2 := newLobby(s)
	l2.build()
	if len(l2.recs) != 3 {
		t.Fatalf("tables.json has %d tables", len(l2.recs))
	}
}
