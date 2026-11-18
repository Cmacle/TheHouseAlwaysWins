// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"math"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The dice land on the server's faces, inside the tray, when the server
// says the roll ends.
func TestDiceSettleOnTime(t *testing.T) {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{}, assets)
	for seed := int64(1); seed <= 60; seed++ {
		ms := int64(3000 + seed*33%2001)
		faces := []int{int(seed%6) + 1, 2, 6, int(seed*7%6) + 1, 4}
		a.onDice(proto.DiceRoll{ID: int(seed), Faces: faces, MS: ms, Seed: seed})
		r := a.roll
		end := r.settleTime()
		if math.Abs(end-float64(ms)/1000) > 0.45 {
			t.Errorf("seed %d: settled at %.2fs, roll is %dms (friction %.2f)", seed, end, ms, r.fric)
		}
		for i, d := range r.dice {
			if d.face != faces[i] {
				t.Errorf("seed %d die %d shows %d, want %d", seed, i, d.face, faces[i])
			}
			if d.x < float64(r.tray.Min.X) || d.x > float64(r.tray.Max.X) || d.y < float64(r.tray.Min.Y) || d.y > float64(r.tray.Max.Y) {
				t.Errorf("seed %d die %d left the tray at %.0f,%.0f", seed, i, d.x, d.y)
			}
		}
	}
}
