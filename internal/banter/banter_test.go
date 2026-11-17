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
