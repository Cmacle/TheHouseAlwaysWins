// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"strings"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
)

// Every page of every line (stories included) fits in the speech bubble
// without being cut off.
func TestEveryPageFitsTheBubble(t *testing.T) {
	a, _ := newRecApp(t)
	worst := 0
	for _, ev := range banter.Events() {
		for _, vulgar := range []bool{false, true} {
			for _, line := range banter.Pool(ev, vulgar) {
				line = banter.Fill(line, banter.Vars{Name: "WWWWWWWWWWWWWWWW", Amount: 999999, Total: 26, Net: -9999999, Balance: 999999, Streak: 12, Up: "Queen", Item: banter.TierItems[5]})
				for _, p := range banter.Pages(line) {
					n := len(a.wrap(p.Text, bubbleWrap))
					if n > worst {
						worst = n
					}
					if n > bubbleMaxLines {
						t.Errorf("%s: page wraps to %d lines: %q", ev, n, p.Text)
					}
				}
			}
		}
	}
	t.Logf("tallest page: %d lines", worst)
}

func TestStoriesArePaged(t *testing.T) {
	for _, s := range banter.Pool(banter.EvStory, false) {
		pages := banter.Pages(s)
		paras := 0
		for _, p := range pages {
			if p.Para {
				paras++
			}
			if strings.TrimSpace(p.Text) == "" {
				t.Fatal("empty page")
			}
		}
		if paras < 3 || len(pages) < 8 {
			t.Errorf("story too short: %d pages, %d paragraph breaks: %.60q", len(pages), paras, s)
		}
		if strings.ContainsAny(s, "[]|") {
			t.Errorf("unresolved markup: %.80q", s)
		}
	}
}
