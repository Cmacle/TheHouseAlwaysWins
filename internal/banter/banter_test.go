// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package banter

import (
	"strings"
	"testing"
)

func TestNoRepeatsWithinDeck(t *testing.T) {
	p := NewPicker()
	pool := Pool(EvBust, false)
	seen := map[string]bool{}
	for i := 0; i < len(pool); i++ {
		l := p.Line(EvBust, false, Vars{Name: "bob", Total: 24})
		if seen[l] {
			t.Fatalf("repeat after %d lines: %q", i, l)
		}
		seen[l] = true
	}
}

func TestPlaceholders(t *testing.T) {
	for _, ev := range Events() {
		for _, l := range append(Pool(ev, true), clean[ev]...) {
			f := Fill(l, Vars{Name: "n", Amount: 1, Total: 2, Net: 3, Balance: 4, Streak: 5, Up: "Ace"})
			if strings.ContainsAny(f, "{}") {
				t.Errorf("unfilled placeholder in %s: %q", ev, l)
			}
		}
		if len(clean[ev]) == 0 {
			t.Errorf("event %s has no clean lines", ev)
		}
	}
	c, v := Count()
	t.Logf("%d clean lines, %d vulgar lines", c, v)
}

func TestTier(t *testing.T) {
	steps := []struct {
		net  int64
		want int
	}{
		{0, 0}, {9_999, 0}, {10_000, 1}, {9_000, 1}, {7_999, 0}, {9_999, 0},
		{60_000, 2}, {2_000_000, 5}, {1_400_000, 5}, {1_340_000, 4},
		{0, 0}, {-5_000, -1}, {-4_000, -1}, {-2_999, 0}, {-60_000, -3}, {-48_000, -3}, {-44_000, -2},
		{200_000, 3},
	}
	tier := 0
	for _, s := range steps {
		tier = Tier(s.net, tier)
		if tier != s.want {
			t.Fatalf("net %d: tier %d want %d", s.net, tier, s.want)
		}
	}
}

// Lines about counts (rolls, wins in a row, a natural 8 or 9) never use
// {amount}, which is written as money ("$241").
func TestCountsAreNotMoney(t *testing.T) {
	for _, ev := range []string{EvBgMany, EvBacStreak, EvBacNatural} {
		for _, pool := range []map[string][]string{clean, vulgar} {
			for _, l := range pool[ev] {
				if strings.Contains(l, "{amount}") {
					t.Errorf("%s: %q writes a count as money", ev, l)
				}
			}
		}
	}
}
