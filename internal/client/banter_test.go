package client

import (
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
)

// Every dealer line, with a worst-case name and amounts, must fit the bubble.
func TestBanterFitsBubble(t *testing.T) {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{}, assets)
	v := banter.Vars{Name: "WWWWWWWWWWWWWWWW", Amount: 999999, Total: 26, Net: -9999999, Balance: 999999, Streak: 12, Up: "Queen"}
	worst := 0
	for _, ev := range banter.Events() {
		for _, l := range banter.Pool(ev, true) {
			n := len(a.wrap(banter.Fill(l, v), bubbleWrap))
			if n > worst {
				worst = n
			}
			if n > bubbleMaxLines {
				t.Errorf("%s line too long (%d lines): %q", ev, n, l)
			}
		}
	}
	t.Logf("longest line wraps to %d lines", worst)
}
