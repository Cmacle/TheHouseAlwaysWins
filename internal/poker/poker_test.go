// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package poker

import (
	"math/rand"
	"strings"
	"testing"
)

// parse reads cards like "As Kd 10h 2c".
func parse(s string) []Card {
	var out []Card
	for _, f := range strings.Fields(s) {
		rs, ss := f[:len(f)-1], f[len(f)-1]
		r := map[string]uint8{"A": 1, "J": 11, "Q": 12, "K": 13}[rs]
		if r == 0 {
			for _, c := range rs {
				r = r*10 + uint8(c-'0')
			}
		}
		out = append(out, Card{Rank: r, Suit: uint8(strings.IndexByte("cdhs", ss))})
	}
	return out
}

func best(s string) Score { sc, _ := Best(parse(s)); return sc }

func TestCategories(t *testing.T) {
	cases := []struct {
		cards string
		cat   int
		name  string
	}{
		{"As Ks Qs Js 10s 2d 3c", RoyalFlush, "Royal Flush"},
		{"9h 8h 7h 6h 5h Ac Ad", StraightFlush, "Straight Flush, Nine high"},
		{"7c 7d 7h 7s Kd 2c 3c", Quads, "Four Sevens"},
		{"Kc Kd Kh 7s 7d 2c 3h", FullHouse, "Full House, Kings full of Sevens"},
		{"2h 9h Jh 4h Kh Ac Ad", Flush, "Flush, King high"},
		{"Ah 2d 3c 4s 5h Kd Kc", Straight, "Straight, Five high"},
		{"6s 6d 6c Ad 9h 2c 3h", Trips, "Three Sixes"},
		{"Jc Jd 4h 4s Ad 9h 2c", TwoPair, "Two Pair, Jacks and Fours"},
		{"10c 10d 4h 8s Ad 9h 2c", Pair, "Pair of Tens"},
		{"Ac Qd 4h 8s 7d 9h 2c", HighCard, "Ace high"},
	}
	for _, c := range cases {
		s := best(c.cards)
		if s.Category() != c.cat || s.Name() != c.name {
			t.Errorf("%s: got %d %q, want %d %q", c.cards, s.Category(), s.Name(), c.cat, c.name)
		}
	}
}

func TestOrdering(t *testing.T) {
	order := []string{
		"Ac Qd 4h 8s 7d 9h 2c", // ace high
		"2c 2d 4h 8s 7d 9h Kc", // pair of twos
		"Ac Ad 4h 8s 7d 9h Kc", // pair of aces
		"3c 3d 2h 2s 7d 9h Kc", // two pair
		"4c 4d 4h 8s 7d 9h Kc", // trips
		"Ah 2d 3c 4s 5h Kd Qc", // wheel
		"6h 2d 3c 4s 5h Kd Qc", // six-high straight
		"2h 9h Jh 4h Kh Ac Qd", // flush
		"Kc Kd Kh 7s 7d 2c 3h", // full house
		"7c 7d 7h 7s Kd 2c 3c", // quads
		"9h 8h 7h 6h 5h Ac Ad", // straight flush
	}
	for i := 1; i < len(order); i++ {
		if best(order[i]) <= best(order[i-1]) {
			t.Errorf("%q should beat %q", order[i], order[i-1])
		}
	}
	// kickers
	if best("Ac Ad Kh 8s 7d 3h 2c") <= best("As Ah Qh 8s 7d 3h 2c") {
		t.Error("ace-king should beat ace-queen")
	}
	// board plays: a tie
	if best("2c 3d As Ks Qs Js 10s") != best("4c 5d As Ks Qs Js 10s") {
		t.Error("royal on the board should split")
	}
}

func TestEquity(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	aa := Equity(parse("Ac Ad"), nil, 1, 4000, rng)
	if aa < 0.80 || aa > 0.88 {
		t.Errorf("AA vs one random hand: %.3f, want ~0.85", aa)
	}
	low := Equity(parse("7c 2d"), nil, 1, 4000, rng)
	if low < 0.29 || low > 0.38 {
		t.Errorf("72o vs one random hand: %.3f, want ~0.34", low)
	}
	nuts := Equity(parse("As Ks"), parse("Qs Js 10s"), 3, 500, rng)
	if nuts < 0.999 {
		t.Errorf("flopped royal: %.3f", nuts)
	}
}

func TestSettleSidePots(t *testing.T) {
	// A all-in 50, B all-in 100, C and D 200; D folds after putting in 200?
	// No: D folds having put in 30.
	contrib := []int64{50, 100, 200, 30, 200}
	folded := []bool{false, false, false, true, false}
	// A has the best hand, then B, then E (4), C worst
	scores := []Score{900, 800, 100, 0, 500}
	order := []int{0, 1, 2, 3, 4}
	won, pots := Settle(contrib, folded, scores, order)
	// main pot: 50 from A,B,C,E + 30 from D = 230 -> A
	// side 1: 50 more from B,C,E = 150 -> B
	// side 2: 100 more from C,E = 200 -> E
	want := []int64{230, 150, 0, 0, 200}
	for i := range want {
		if won[i] != want[i] {
			t.Fatalf("won %v, want %v (pots %+v)", won, want, pots)
		}
	}
	var sum, in int64
	for i := range won {
		sum += won[i]
		in += contrib[i]
	}
	if sum != in {
		t.Fatalf("chips not conserved: in %d out %d", in, sum)
	}
}

func TestSettleSplitAndOddChip(t *testing.T) {
	contrib := []int64{25, 25, 25}
	folded := []bool{false, false, true}
	scores := []Score{500, 500, 0}
	won, _ := Settle(contrib, folded, scores, []int{1, 2, 0})
	// 75 split two ways: the odd chip goes to the first after the button (1)
	if won[1] != 38 || won[0] != 37 {
		t.Fatalf("split %v", won)
	}
}

func TestSettleEveryoneFolds(t *testing.T) {
	won, _ := Settle([]int64{10, 20, 5}, []bool{true, false, true}, []Score{0, 0, 0}, []int{0, 1, 2})
	if won[1] != 35 {
		t.Fatalf("last player standing gets %v", won)
	}
}

func TestSettleRandomConserves(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for k := 0; k < 2000; k++ {
		n := 2 + rng.Intn(8)
		contrib := make([]int64, n)
		folded := make([]bool, n)
		scores := make([]Score, n)
		order := rng.Perm(n)
		live := 0
		for i := range contrib {
			contrib[i] = int64(rng.Intn(500))
			folded[i] = rng.Intn(3) == 0
			scores[i] = Score(rng.Intn(5))
			if !folded[i] && contrib[i] > 0 {
				live++
			}
		}
		if live == 0 {
			continue
		}
		won, _ := Settle(contrib, folded, scores, order)
		var a, b int64
		for i := range won {
			a += won[i]
			b += contrib[i]
			if won[i] > 0 && folded[i] {
				t.Fatalf("folded player won: %v %v", contrib, folded)
			}
		}
		if a != b {
			t.Fatalf("not conserved: %v %v %v -> %v", contrib, folded, scores, won)
		}
	}
}

func TestDescribe(t *testing.T) {
	c := func(r, s uint8) Card { return Card{Rank: r, Suit: s} }
	for _, tc := range []struct {
		hole, board []Card
		want        string
	}{
		{[]Card{c(1, 0), c(13, 1)}, nil, "Ace high"},
		{[]Card{c(6, 0), c(6, 1)}, nil, "Pair of Sixes"},
		{[]Card{c(1, 0), c(13, 1)}, []Card{c(13, 2), c(7, 3), c(2, 0)}, "Pair of Kings"},
		{[]Card{c(1, 0), c(13, 1)}, []Card{c(13, 2), c(1, 3), c(2, 0), c(9, 1)}, "Two Pair, Aces and Kings"},
		{[]Card{{}, {}}, nil, ""},
		{[]Card{c(1, 0), c(13, 1)}, []Card{{}, {}, {}}, "Ace high"},
	} {
		if got := Describe(tc.hole, tc.board); got != tc.want {
			t.Errorf("%v + %v = %q, want %q", tc.hole, tc.board, got, tc.want)
		}
	}
}
