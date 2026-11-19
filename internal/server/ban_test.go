// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"io"
	"log"
	"math/rand"
	"strings"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Banning deletes the account and takes the player's part out of the house.
func TestBanRemovesPlayerAndHouseShare(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	s, err := New(cfg, cfg.DataDir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	st := s.Store
	for _, u := range []string{"cheater", "honest"} {
		if _, _, err := st.Register(u, "pw123456", nil, 1000, ""); err != nil {
			t.Fatal(err)
		}
	}
	// the cheater wins 900 from the house and loses 100; honest wins 500
	st.Settle("cheater", []HandResult{{Wagered: 900, Payout: 1800}, {Wagered: 100, Payout: 0}})
	st.Settle("honest", []HandResult{{Wagered: 500, Payout: 1000}})
	st.RecordTableHouse(-900 + 100 - 500)
	before := st.House()
	if top := st.Leaderboard(1, SortBalance); top[0].Username != "cheater" {
		t.Fatalf("setup: richest is %s", top[0].Username)
	}
	var out strings.Builder
	s.Exec("ban cheater using a bot", &out)
	if !strings.Contains(out.String(), "is banned") {
		t.Fatalf("ban said: %s", out.String())
	}
	if _, ok := st.Profile("cheater"); ok {
		t.Fatal("the account is still there")
	}
	after := st.House()
	// the house had lost 800 net to the cheater: that comes back
	if after.Net != before.Net+800 || after.Lost != max(before.Lost-800, 0) || after.Won != before.Won {
		t.Fatalf("house before %+v after %+v", before, after)
	}
	for _, p := range st.Leaderboard(10, SortNet) {
		if p.Username == "cheater" {
			t.Fatal("still on the leaderboard")
		}
	}
	if p, _ := st.Profile("honest"); !p.RankRich {
		t.Fatal("the next player should take the record")
	}
	if _, _, err := st.Login("cheater", "pw123456", ""); err == nil {
		t.Fatal("a banned player can still log in")
	}
	// the name is free again, like a deleted account
	if _, _, err := st.Register("cheater", "pw123456", nil, 1000, ""); err != nil {
		t.Fatalf("can't reuse the name: %v", err)
	}
}

// Accounts from before tracking get an estimate from their results, leaving
// out the poker tables.
func TestEstimateHouseShares(t *testing.T) {
	st, _ := newTestStore(t)
	st.Register("old", "pw123456", nil, 1000, "")
	st.mu.Lock()
	a := st.accounts["old"]
	a.HouseTracked = false
	a.HouseWon, a.HouseLost = 0, 0
	a.Won, a.Lost = 1500, 2000
	a.Tables = map[string]*TableStat{"holdem": {Won: 500, Lost: 200}}
	st.mu.Unlock()
	st.EstimateHouseShares([]string{"holdem"})
	st.EstimateHouseShares([]string{"holdem"}) // only once
	st.mu.Lock()
	defer st.mu.Unlock()
	if a.HouseWon != 1800 || a.HouseLost != 1000 {
		t.Fatalf("estimate won %d lost %d, want 1800 and 1000", a.HouseWon, a.HouseLost)
	}
}

// In poker, the players' shares add up to what Dale won or lost.
func TestPokerHouseShares(t *testing.T) {
	hc := defaultHoldem()
	hc.DaleThinkMS = 0
	tb, st, conns := pokerTable(t, 3, hc)
	before := st.House().Net
	play(t, tb, conns, 150, rand.New(rand.NewSource(3)), nil)
	var shares int64
	for _, u := range []string{"player0", "player1", "player2"} {
		st.mu.Lock()
		a := st.accounts[u]
		shares += a.HouseWon - a.HouseLost
		st.mu.Unlock()
	}
	if got := st.House().Net - before; got != shares {
		t.Fatalf("house made %d at poker, players' shares add to %d", got, shares)
	}
}

// From the game, admins can't ban other admins.
func TestInGameAdminCantBanAdmin(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	s, err := New(cfg, cfg.DataDir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"boss", "mod"} {
		s.Store.Register(u, "pw123456", nil, 1000, "")
		s.Store.SetAdmin(u, true)
	}
	c := &conn{out: make(chan []byte, 16), closed: make(chan struct{}), srv: s, user: "mod"}
	runAdminCommand(s.Log, s.Exec, c, "mod", "/ban boss")
	if _, ok := s.Store.Profile("boss"); !ok {
		t.Fatal("an admin was banned from the game")
	}
	msg := string(<-c.out)
	if !strings.Contains(msg, "server window") {
		t.Fatalf("got %s", msg)
	}
	_ = proto.SConsole
}
