package blackjack

import "testing"

func c(r uint8) Card { return Card{Rank: r} }

func TestTotal(t *testing.T) {
	cases := []struct {
		cards []Card
		total int
		soft  bool
	}{
		{[]Card{c(1), c(13)}, 21, true},
		{[]Card{c(1), c(1)}, 12, true},
		{[]Card{c(1), c(1), c(9)}, 21, true},
		{[]Card{c(1), c(5), c(10)}, 16, false},
		{[]Card{c(10), c(12), c(2)}, 22, false},
		{[]Card{c(1), c(6)}, 17, true},
	}
	for _, tc := range cases {
		got, soft := Total(tc.cards)
		if got != tc.total || soft != tc.soft {
			t.Errorf("%v: got %d/%v want %d/%v", tc.cards, got, soft, tc.total, tc.soft)
		}
	}
}

func TestSettle(t *testing.T) {
	r := Rules{BlackjackNum: 3, BlackjackDen: 2}
	h := &Hand{Cards: []Card{c(1), c(12)}, Bet: 100}
	r.Settle(h, []Card{c(10), c(7)})
	if h.Result != "blackjack" || h.Payout != 250 {
		t.Fatalf("bj: %s %d", h.Result, h.Payout)
	}
	h = &Hand{Cards: []Card{c(1), c(12)}, Bet: 100, FromSplit: true}
	r.Settle(h, []Card{c(10), c(7)})
	if h.Result != "win" || h.Payout != 200 {
		t.Fatalf("split 21: %s %d", h.Result, h.Payout)
	}
	h = &Hand{Cards: []Card{c(10), c(9)}, Bet: 100}
	r.Settle(h, []Card{c(1), c(10)})
	if h.Result != "lose" {
		t.Fatalf("dealer bj: %s", h.Result)
	}
	h = &Hand{Cards: []Card{c(10), c(6), c(9)}, Bet: 100}
	r.Settle(h, []Card{c(10), c(6), c(9)})
	if h.Result != "bust" || h.Payout != 0 {
		t.Fatalf("both bust: %s", h.Result)
	}
	h = &Hand{Cards: []Card{c(10), c(8)}, Bet: 100}
	r.Settle(h, []Card{c(10), c(8)})
	if h.Result != "push" || h.Payout != 100 {
		t.Fatalf("push: %s", h.Result)
	}
	h = &Hand{Cards: []Card{c(10), c(6)}, Bet: 100, Surrendered: true}
	r.Settle(h, []Card{c(10), c(8)})
	if h.Payout != 50 {
		t.Fatalf("surrender: %d", h.Payout)
	}
}

func TestDealer(t *testing.T) {
	s17 := Rules{}
	h17 := Rules{DealerHitsSoft17: true}
	soft17 := []Card{c(1), c(6)}
	if s17.DealerShouldHit(soft17) || !h17.DealerShouldHit(soft17) {
		t.Fatal("soft 17 rule")
	}
	if !s17.DealerShouldHit([]Card{c(10), c(6)}) {
		t.Fatal("16 must hit")
	}
}

func TestSplitRules(t *testing.T) {
	r := Rules{MaxHands: 4}
	if !r.CanSplit(&Hand{Cards: []Card{c(12), c(10)}}, 1) {
		t.Fatal("ten values should split")
	}
	if r.CanSplit(&Hand{Cards: []Card{c(1), c(1)}, FromSplit: true}, 2) {
		t.Fatal("resplit aces not allowed")
	}
	if r.CanSplit(&Hand{Cards: []Card{c(8), c(8)}}, 4) {
		t.Fatal("max hands")
	}
}

func TestShoe(t *testing.T) {
	s := NewShoe(6, 0.75)
	if s.Remaining() != 312 {
		t.Fatal(s.Remaining())
	}
	for i := 0; i < 1000; i++ {
		s.Draw()
	}
}
