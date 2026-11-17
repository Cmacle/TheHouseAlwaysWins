package server

import (
	"strings"
	"testing"
)

func TestUnlocksAndCrown(t *testing.T) {
	st, _ := OpenStore(t.TempDir())
	st.CrownMin = 1000
	st.Register("alice", "pass", nil, 1000, "p")
	st.Register("bob", "pass", nil, 1000, "p")
	if r, _ := st.Tops(); r != "" {
		t.Fatalf("crown with everyone tied: %q", r)
	}
	// alice bets 1000 and wins: party hat (wagered 1000), sombrero + big fish (peak 2000)
	st.Debit("alice", 1000)
	st.Settle("alice", []HandResult{{Wagered: 1000, Payout: 2000, Result: "win"}})
	got := map[string]bool{}
	for _, u := range st.TakeUnlocks() {
		if u.User == "alice" {
			got[u.Item.ID] = true
		}
	}
	for _, id := range []string{"party_hat", "sombrero", "t_big_fish"} {
		if !got[id] {
			t.Errorf("missing unlock %s (got %v)", id, got)
		}
	}
	if r, n := st.Tops(); r != "alice" || n != "alice" {
		t.Fatalf("tops %q %q", r, n)
	}
	p, _ := st.Profile("alice")
	if p.Hat != "crown" {
		t.Fatalf("hat %q", p.Hat)
	}
	if err := st.Equip("bob", "hat", "party_hat"); err == nil {
		t.Fatal("bob equipped a locked hat")
	}
	if err := st.Equip("alice", "title", "t_owns_house"); err != nil {
		t.Fatal(err)
	}
	// bob overtakes alice
	st.Debit("bob", 1000)
	st.Settle("bob", []HandResult{{Wagered: 1000, Payout: 2500, Result: "blackjack", Blackjack: true}})
	if r, _ := st.Tops(); r != "bob" {
		t.Fatalf("crown should move to bob, got %q", r)
	}
	p, _ = st.Profile("alice")
	if p.Hat == "crown" || p.Title != "t_rookie" {
		t.Fatalf("alice kept unique items: %q %q", p.Hat, p.Title)
	}
	// split accounting: one split => 2 split hands => 1 split
	st.Settle("bob", []HandResult{{Wagered: 10, Payout: 0, FromSplit: true, Result: "bust"}, {Wagered: 10, Payout: 20, FromSplit: true, Doubled: true, Result: "win"}})
	b, _ := st.Profile("bob")
	if b.Splits != 1 || b.Busts != 1 || b.DoublesWon != 1 {
		t.Fatalf("splits %d busts %d doubles %d", b.Splits, b.Busts, b.DoublesWon)
	}
}

func TestSeatOrder(t *testing.T) {
	cases := map[int][]int{
		1:  {0},
		2:  {0, 1},
		5:  {2, 1, 3, 0, 4},
		10: {4, 5, 3, 6, 2, 7, 1, 8, 0, 9},
	}
	for n, want := range cases {
		got := seatOrder(n)
		if len(got) != n {
			t.Fatalf("n=%d got %v", n, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("n=%d got %v want %v", n, got, want)
			}
		}
	}
}

func TestHouseStats(t *testing.T) {
	st, _ := OpenStore(t.TempDir())
	st.RecordRound(500, 3, true, false, 1)
	st.RecordRound(-200, 2, false, true, 0)
	h := st.House()
	if h.Net != 300 || h.Won != 500 || h.Lost != 200 || h.HandsDealt != 2 || h.DealerBusts != 1 || h.DealerBlackjacks != 1 || h.BiggestLoss != 200 {
		t.Fatalf("%+v", h)
	}
	st.dirty = true
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	st2, _ := OpenStore(st.dir)
	if st2.House().Net != 300 {
		t.Fatal("house stats not persisted")
	}
}

func TestDealerTierSpeech(t *testing.T) {
	s := testServer(t, func(c *Config) { c.DealerVulgar = true })
	p := &player{user: "victim", seat: 0}
	res := []roundResult{{p: p, delta: -500}}
	for _, step := range [][2]int{{0, 1}, {1, 2}, {2, 1}, {1, 0}, {0, -1}, {-1, -3}, {-3, -2}, {-2, 0}} {
		var line string
		s.Game.DoWait(func() {
			s.Game.tierChanged(step[0], step[1], res)
			line = s.Game.speech.text
		})
		if line == "" {
			t.Fatalf("no line for tier %d -> %d", step[0], step[1])
		}
		if strings.Contains(line, "{") {
			t.Fatalf("unfilled: %q", line)
		}
		t.Logf("%d -> %d: %s", step[0], step[1], line)
	}
}
