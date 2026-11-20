// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// casinoTestTable makes an instant-mode baccarat or Red Dog table with n
// seated players holding 5,000 chips each.
func casinoTestTable(t *testing.T, game string, n int) (*Casino, *Store, []*csPlayer) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	cfg.CardReveal = true // instant mode skips the waits anyway
	st, err := OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	var rules casinoRules = &Baccarat{}
	if game == proto.GameRedDog {
		rules = &RedDog{}
	}
	rules.reset()
	cc, _ := casinoConfig(TableDef{Game: game})
	tb := newCasino(game, "Test", rules, cfg, cc, st, nil, log.New(io.Discard, "", 0))
	tb.instant = true
	var ps []*csPlayer
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("player%d", i)
		if _, _, err := st.Register(u, "pw123456", nil, 5000, ""); err != nil {
			t.Fatal(err)
		}
		c := &conn{out: make(chan []byte, 1<<16), closed: make(chan struct{}), user: u}
		tb.onJoin(c, u)
		ps = append(ps, tb.conns[c])
	}
	return tb, st, ps
}

// runQueued runs timers until the table is back to taking bets.
func runQueued(t *testing.T, tb *Casino, ps []*csPlayer, rng *rand.Rand) {
	for steps := 0; ; steps++ {
		if steps > 200 {
			t.Fatalf("stuck in %s", tb.phase)
		}
		for _, p := range ps {
			for len(p.conn.out) > 0 {
				<-p.conn.out
			}
		}
		if tb.phase == csRaise {
			for _, p := range ps {
				tb.onAct(p, proto.CasinoAct{Action: []string{"raise", "stay"}[rng.Intn(2)]})
			}
		}
		fn := tb.queued
		if fn == nil {
			if tb.phase == csBetting {
				return
			}
			t.Fatalf("no timer in %s", tb.phase)
		}
		tb.queued = nil
		fn()
	}
}

// Chips are never made or lost: what players win the house loses.
func TestCasinoConservesChips(t *testing.T) {
	for _, game := range []string{proto.GameBaccarat, proto.GameRedDog} {
		tb, st, ps := casinoTestTable(t, game, 4)
		rng := rand.New(rand.NewSource(7))
		total := func() int64 {
			var n int64
			for _, p := range ps {
				n += st.Balance(p.user)
			}
			return n + st.House().Net
		}
		start := total()
		for round := 0; round < 400; round++ {
			for _, p := range ps {
				if rng.Intn(5) == 0 {
					continue
				}
				spots := tb.rules.spots()
				tb.onAct(p, proto.CasinoAct{Action: "bet", Spot: spots[rng.Intn(len(spots))], Amount: int64(10 * (1 + rng.Intn(20)))})
			}
			if tb.queued == nil {
				continue
			}
			runQueued(t, tb, ps, rng)
			if got := total(); got != start {
				t.Fatalf("%s round %d: chips %d, started with %d", game, round, got, start)
			}
		}
		if tb.handNo < 150 {
			t.Fatalf("%s: only %d rounds", game, tb.handNo)
		}
		// everyone who played is on the table's leaderboard, and it adds up
		var net int64
		for _, e := range st.TableBoard(game, 0) {
			net += e.Net
		}
		if net != -st.House().Net {
			t.Fatalf("%s: table board nets %d, house %d", game, net, st.House().Net)
		}
	}
}

func TestBaccaratRules(t *testing.T) {
	c := func(r uint8) bj.Card { return bj.Card{Rank: r, Suit: 0} }
	if bacTotal([]bj.Card{c(13), c(9)}) != 9 || bacTotal([]bj.Card{c(7), c(8)}) != 5 || bacTotal([]bj.Card{c(1), c(10), c(4)}) != 5 {
		t.Fatal("totals")
	}
	p := func(r uint8) *bj.Card { x := c(r); return &x }
	for _, tc := range []struct {
		banker int
		p3     *bj.Card
		want   bool
	}{
		{5, nil, true}, {6, nil, false}, {2, p(8), true}, {3, p(8), false}, {3, p(9), true},
		{4, p(1), false}, {4, p(2), true}, {4, p(7), true}, {4, p(8), false},
		{5, p(3), false}, {5, p(4), true}, {6, p(6), true}, {6, p(5), false}, {7, p(6), false},
	} {
		if got := bankerDraws(tc.banker, tc.p3); got != tc.want {
			t.Errorf("banker %d, player third %v: draws %v, want %v", tc.banker, tc.p3, got, tc.want)
		}
	}
	// payouts
	b := &Baccarat{}
	tb := &Casino{cc: casinoCfg{Commission: 5}}
	pl := &csPlayer{bets: map[string]int64{bacPlayer: 100, bacBanker: 100, bacTie: 10}, staked: 210}
	for _, tc := range []struct {
		winner string
		want   int64
	}{{bacPlayer, 200}, {bacBanker, 195}, {bacTie, 290}} {
		b.winner = tc.winner
		if got := b.payout(tb, pl).Payout; got != tc.want {
			t.Errorf("%s wins: paid %d, want %d", tc.winner, got, tc.want)
		}
	}
}

func TestRedDogPayouts(t *testing.T) {
	tb := &Casino{}
	p := &csPlayer{staked: 200}
	for _, tc := range []struct {
		r    RedDog
		want int64
	}{
		{RedDog{spread: -1}, 200},
		{RedDog{spread: 0}, 200},
		{RedDog{spread: 0, trips: true}, 2400},
		{RedDog{spread: 1, hit: true}, 1200},
		{RedDog{spread: 2, hit: true}, 1000},
		{RedDog{spread: 3, hit: true}, 600},
		{RedDog{spread: 7, hit: true}, 400},
		{RedDog{spread: 7}, 0},
	} {
		if got := tc.r.payout(tb, p).Payout; got != tc.want {
			t.Errorf("%+v: paid %d, want %d", tc.r, got, tc.want)
		}
	}
}

// Raising doubles the stake before the third card.
func TestRedDogRaise(t *testing.T) {
	tb, st, ps := casinoTestTable(t, proto.GameRedDog, 1)
	p := ps[0]
	for tries := 0; tries < 50; tries++ {
		tb.onAct(p, proto.CasinoAct{Action: "bet", Spot: rdAnte, Amount: 100})
		for steps := 0; tb.phase != csRaise && tb.queued != nil; steps++ {
			if steps > 100 {
				t.Fatal("stuck")
			}
			fn := tb.queued
			tb.queued = nil
			fn()
			if tb.phase == csBetting && tb.handNo > 0 && tb.queued == nil {
				break
			}
		}
		if tb.phase == csRaise {
			before := st.Balance(p.user)
			tb.onAct(p, proto.CasinoAct{Action: "raise"})
			if p.staked != 200 || st.Balance(p.user) != before-100 {
				t.Fatalf("staked %d after raising", p.staked)
			}
			runQueued(t, tb, ps, rand.New(rand.NewSource(1)))
			return
		}
	}
	t.Fatal("never got a spread to raise on")
}

// A server that already has tables gets tables for new games once.
func TestOfferNewGames(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	if err := os.WriteFile(filepath.Join(dir, "tables.json"), []byte(`[
  {"id": "blackjack", "name": "Blackjack", "game": "blackjack"},
  {"id": "beggars", "name": "Beggars Table", "game": "beggar"}
]`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(cfg, dir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	s.Lobby.start(stop)
	games := map[string]bool{}
	for _, ti := range s.Lobby.Tables() {
		games[ti.Game] = true
	}
	if !games[proto.GameBaccarat] || !games[proto.GameRedDog] || games[proto.GameHoldem] {
		t.Fatalf("tables after update: %v", games)
	}
	if last := s.Lobby.Tables()[len(s.Lobby.Tables())-1]; last.Game != proto.GameBeggar {
		t.Fatalf("Beggars Table should stay last, got %s", last.Game)
	}
	// closing one keeps it closed
	if err := s.Lobby.RemoveTable("reddog"); err != nil {
		t.Fatal(err)
	}
	cfg2 := DefaultConfig() // the first server's lobby is still reading cfg
	cfg2.DataDir = dir
	s2, err := New(cfg2, dir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, ti := range s2.Lobby.Tables() {
		if ti.Game == proto.GameRedDog {
			t.Fatal("a closed Red Dog table came back")
		}
	}
}

// A Beggars Table still on the old limit of 50 moves to the refill amount
// once; one someone set to another number keeps it.
func TestBeggarLimitMigrates(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	os.WriteFile(filepath.Join(dir, "tables.json"), []byte(`[
  {"id": "beggars", "name": "Beggars Table", "game": "beggar", "settings": {"max_balance": 50}},
  {"id": "pit", "name": "The Pit", "game": "beggar", "settings": {"max_balance": 200}}
]`), 0o644)
	os.WriteFile(filepath.Join(dir, "tables-offered.json"), []byte(`["baccarat","beggar","blackjack","holdem","reddog"]`), 0o644)
	s, err := New(cfg, dir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	limits := map[string]int64{}
	for _, id := range []string{"beggars", "pit"} {
		b := s.Lobby.find(id).t.(beggarTable)
		limits[id] = b.limit()
	}
	if limits["beggars"] != cfg.RefillAmount || limits["pit"] != 200 {
		t.Fatalf("limits %v", limits)
	}
	// the change is saved, so it only happens once
	b, _ := os.ReadFile(filepath.Join(dir, "tables.json"))
	if !strings.Contains(string(b), `"max_balance": 0`) {
		t.Fatalf("tables.json not updated:\n%s", b)
	}
}
