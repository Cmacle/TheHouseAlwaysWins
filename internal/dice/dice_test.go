// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package dice

import (
	"math"
	"testing"
)

// Plan gets close to the asked odds without piling on dice.
func TestPlan(t *testing.T) {
	for _, odds := range []int{1, 2, 3, 6, 10, 37, 100, 500, 1000, 5000} {
		g := Plan(odds)
		if g.Dice < 1 || g.Dice > MaxDice {
			t.Fatalf("odds %d: %d dice", odds, g.Dice)
		}
		if r := g.Odds() / float64(odds); r < 0.8 || r > 1.25 {
			t.Errorf("odds %d: got 1 in %.1f (%s on %d dice)", odds, g.Odds(), g.Describe(), g.Dice)
		}
	}
	if g := Plan(1); !g.Made([]int{1}) {
		t.Error("1 in 1 should always win")
	}
}

// The exact chances match counting every outcome.
func TestPlanChancesExact(t *testing.T) {
	for _, g := range []Goal{{Dice: 3, Kind: GoalSum, Need: 14}, {Dice: 4, Kind: GoalFaces, Need: 2, Face: 5}, {Dice: 3, Kind: GoalFaces, Need: 3, Face: 6}} {
		want := 0
		faces := make([]int, g.Dice)
		total := int(math.Pow(6, float64(g.Dice)))
		for c := 0; c < total; c++ {
			x := c
			for i := range faces {
				faces[i] = x%6 + 1
				x /= 6
			}
			if g.Made(faces) {
				want++
			}
		}
		var p float64
		if g.Kind == GoalSum {
			p = sumAtLeast(g.Dice)[g.Need]
		} else {
			p = facesAtLeast(g.Dice, g.Need, g.Face)
		}
		if math.Abs(p-float64(want)/float64(total)) > 1e-12 {
			t.Errorf("%+v: %.6f, counted %.6f", g, p, float64(want)/float64(total))
		}
	}
}

// Thrown dice stay in the tray, come to rest flat in a few seconds, and the
// track ends where the simulation did.
func TestThrowSettles(t *testing.T) {
	var longest, total float64
	for seed := int64(1); seed <= 150; seed++ {
		n := int(seed%MaxDice) + 1
		th := Roll(n, seed)
		total += th.Length
		longest = math.Max(longest, th.Length)
		if th.Length >= maxTime {
			t.Errorf("seed %d (%d dice): never settled", seed, n)
		}
		tr, err := ReadTrack(n, th.Track)
		if err != nil {
			t.Fatal(err)
		}
		end := tr.At(tr.Length() + 1)
		for i, p := range end {
			if p.Pos.X < 0.4 || p.Pos.X > TrayW-0.4 || p.Pos.Y < 0.4 || p.Pos.Y > TrayD-0.4 || p.Pos.Z < 0.4 || p.Pos.Z > 1.7 {
				t.Errorf("seed %d die %d rests at %+v", seed, i, p.Pos)
			}
			b := Body{Rot: p.Rot}
			f, sq := b.Up()
			if f != th.Faces[i] || sq < 0.9 {
				t.Errorf("seed %d die %d: track shows %d (%.2f), result %d", seed, i, f, sq, th.Faces[i])
			}
		}
	}
	t.Logf("average %.2fs, longest %.2fs", total/150, longest)
}

// The physics is fair: every face comes up about as often.
func TestFacesFair(t *testing.T) {
	var count [7]int
	n := 0
	for seed := int64(1000); n < 4800; seed++ {
		for _, f := range Roll(6, seed).Faces {
			count[f]++
			n++
		}
	}
	chi := 0.0
	for f := 1; f <= 6; f++ {
		e := float64(n) / 6
		chi += (float64(count[f]) - e) * (float64(count[f]) - e) / e
	}
	t.Logf("faces %v, chi-square %.1f", count[1:], chi)
	if chi > 20.5 { // p < 0.001 with 5 degrees of freedom
		t.Errorf("faces aren't fair: %v (chi-square %.1f)", count[1:], chi)
	}
}
