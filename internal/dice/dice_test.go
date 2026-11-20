// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package dice

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// Plan gets close to the asked odds without piling on dice.
func TestPlan(t *testing.T) {
	for _, odds := range []int{1, 2, 3, 6, 10, 37, 100, 500, 1000, 5000} {
		g := Plan(odds)
		if g.Dice < 1 || g.Dice > MaxDice {
			t.Fatalf("odds %d: %d dice", odds, g.Dice)
		}
		if r := g.Odds() / float64(odds); odds > 1 && (r < 0.8 || r > 1.25) {
			t.Errorf("odds %d: got 1 in %.1f (%s on %d dice)", odds, g.Odds(), g.Describe(), g.Dice)
		}
	}
	if g := Plan(1); !g.Made([]int{1}) {
		t.Error("1 in 1 should always win")
	}
}

// Every kind of goal's chance matches counting every outcome by hand.
func TestChancesExact(t *testing.T) {
	var goals []Goal
	for _, s := range []int{6, 8} {
		for n := 2; n <= 4; n++ {
			goals = append(goals,
				Goal{Dice: n, Sides: s, Kind: GoalSum, Need: n*s*2/3 + 1},
				Goal{Dice: n, Sides: s, Kind: GoalLow, Need: n + 2},
				Goal{Dice: n, Sides: s, Kind: GoalExact, Need: n * (s + 1) / 2},
				Goal{Dice: n, Sides: s, Kind: GoalFaces, Need: 2, Face: s - 1},
				Goal{Dice: n, Sides: s, Kind: GoalKind, Need: 2},
				Goal{Dice: n, Sides: s, Kind: GoalKind, Need: n},
				Goal{Dice: n, Sides: s, Kind: GoalUnique},
				Goal{Dice: n, Sides: s, Kind: GoalStraight},
				Goal{Dice: n, Sides: s, Kind: GoalOdd},
				Goal{Dice: n, Sides: s, Kind: GoalEven})
		}
	}
	goals = append(goals, Goal{Dice: 3, Sides: 20, Kind: GoalKind, Need: 2}, Goal{Dice: 2, Sides: 12, Kind: GoalStraight})
	for _, g := range goals {
		s := g.S()
		faces := make([]int, g.Dice)
		total := int(math.Pow(float64(s), float64(g.Dice)))
		made := 0
		for c := 0; c < total; c++ {
			x := c
			for i := range faces {
				faces[i] = x%s + 1
				x /= s
			}
			if g.Made(faces) {
				made++
			}
		}
		want := float64(made) / float64(total)
		if got := chance(g); math.Abs(got-want) > 1e-9 {
			t.Errorf("%d%s %s: worked out %.6f, counted %.6f", g.Dice, g.Die(), g.Describe(), got, want)
		}
	}
}

// Pick stays near the odds, never repeats the last kind of goal, and over
// a run of wins turns up every kind of goal and every kind of die.
func TestPickVaries(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	kinds, sides := map[string]int{}, map[int]int{}
	for _, odds := range []int{1000, 200, 50} {
		var prev *Goal
		for i := 0; i < 300; i++ {
			g := Pick(odds, prev, rng)
			if r := g.Odds() / float64(odds); r < 0.5 || r > 2 {
				t.Errorf("odds %d: picked 1 in %.0f (%s)", odds, g.Odds(), g.Describe())
			}
			if prev != nil && g.Kind == prev.Kind {
				t.Fatalf("same kind twice: %s then %s", prev.Describe(), g.Describe())
			}
			if g.Dice > maxDiceFor(g.S()) {
				t.Fatalf("%d %s", g.Dice, g.Die())
			}
			kinds[g.Kind]++
			sides[g.S()]++
			prev = &g
		}
	}
	t.Logf("kinds %v, dice %v", kinds, sides)
	for _, k := range Kinds {
		if kinds[k] == 0 {
			t.Errorf("never picked a %s goal", k)
		}
	}
	for _, s := range Sides {
		if sides[s] == 0 {
			t.Errorf("never picked d%d", s)
		}
	}
	for i := 0; i < 12; i++ {
		g := Pick(1000, nil, rng)
		t.Logf("  %d%s: %s  (1 in %.0f)", g.Dice, g.Die(), g.Describe(), g.Odds())
	}
}

// Thrown dice of every kind stay in the tray, come to rest flat in a few
// seconds, and the track ends where the simulation did.
func TestThrowSettles(t *testing.T) {
	for _, s := range Sides {
		var longest, sum float64
		const throws = 60
		for seed := int64(1); seed <= throws; seed++ {
			n := int(seed%int64(maxDiceFor(s))) + 1
			th := Roll(s, n, seed)
			sum += th.Length
			longest = math.Max(longest, th.Length)
			if th.Length >= maxTime {
				t.Errorf("d%d seed %d (%d dice): never settled", s, seed, n)
			}
			tr, err := ReadTrack(n, th.Track)
			if err != nil {
				t.Fatal(err)
			}
			sh := ShapeFor(s)
			for i, p := range tr.At(tr.Length() + 1) {
				m := sh.Inner - 0.1
				if p.Pos.X < m || p.Pos.X > TrayW-m || p.Pos.Y < m || p.Pos.Y > TrayD-m || p.Pos.Z < m || p.Pos.Z > 2.5*sh.Radius {
					t.Errorf("d%d seed %d die %d rests at %+v", s, seed, i, p.Pos)
				}
				b := Body{S: sh, Rot: p.Rot}
				f, sq := b.Up()
				if f != th.Faces[i] || sq < 0.9 {
					t.Errorf("d%d seed %d die %d: track shows %d (%.2f), result %d", s, seed, i, f, sq, th.Faces[i])
				}
			}
		}
		t.Logf("d%d: average %.2fs, longest %.2fs", s, sum/throws, longest)
	}
}

// The physics is fair: every face of every die comes up about as often.
func TestFacesFair(t *testing.T) {
	// chi-square limits for p < 0.001
	limit := map[int]float64{6: 20.5, 8: 24.3, 12: 31.3, 20: 43.8}
	for _, s := range Sides {
		count := make([]int, s+1)
		n := 0
		for seed := int64(1000); n < 120*s; seed++ {
			for _, f := range Roll(s, min(5, maxDiceFor(s)), seed).Faces {
				count[f]++
				n++
			}
		}
		chi := 0.0
		e := float64(n) / float64(s)
		for f := 1; f <= s; f++ {
			chi += (float64(count[f]) - e) * (float64(count[f]) - e) / e
		}
		t.Logf("d%d: faces %v, chi-square %.1f", s, count[1:], chi)
		if chi > limit[s] {
			t.Errorf("d%d isn't fair: %v (chi-square %.1f)", s, count[1:], chi)
		}
	}
}

// Dice never visibly sink into each other: across many throws, at every
// step, no two dice overlap by more than a sliver.
func TestDiceDontOverlap(t *testing.T) {
	for _, s := range Sides {
		worst, bad := 0.0, 0
		for seed := int64(1); seed <= 40; seed++ {
			w := NewWorld(s, maxDiceFor(s)-int(seed%3), seed)
			for !w.Settled() {
				w.Step()
				for i := range w.Dice {
					for j := i + 1; j < len(w.Dice); j++ {
						if d, _, _, ok := overlap(&w.Dice[i], &w.Dice[j]); ok {
							worst = math.Max(worst, d)
							if d > 0.06 {
								bad++
							}
						}
					}
				}
			}
		}
		t.Logf("d%d: deepest overlap %.3f die widths, %d steps past 0.06", s, worst, bad)
		if worst > 0.12 || bad > 20 {
			t.Errorf("d%d dice overlap: deepest %.3f, %d steps past 0.06", s, worst, bad)
		}
	}
}

// The overlap test agrees with simple cases.
func TestOverlapCases(t *testing.T) {
	a := Body{Pos: V3{0, 0, 0}, Rot: Quat{W: 1}}
	b := Body{Pos: V3{0.9, 0, 0}, Rot: Quat{W: 1}}
	if d, n, _, ok := overlap(&a, &b); !ok || math.Abs(d-0.1) > 1e-6 || n.X > -0.99 {
		t.Errorf("side by side: %v %.3f %+v", ok, d, n)
	}
	b.Pos.X = 1.01
	if _, _, _, ok := overlap(&a, &b); ok {
		t.Error("apart but overlapping")
	}
	c, s := math.Cos(math.Pi/8), math.Sin(math.Pi/8) // 45 degree turns
	a.Rot, b.Rot = Quat{W: c, Z: s}, Quat{W: c, Y: s}
	b.Pos = V3{1.35, 0, 0} // edges crossing: 0.707 + 0.707 > 1.35
	if d, _, _, ok := overlap(&a, &b); !ok || d < 0.03 {
		t.Errorf("crossed edges should overlap: %v %.3f", ok, d)
	}
	// two d20s just touching, then apart
	ic := ShapeFor(20)
	a, b = Body{S: ic, Rot: Quat{W: 1}}, Body{S: ic, Rot: Quat{W: 1}, Pos: V3{2*ic.Inner - 0.05, 0, 0}}
	if _, _, _, ok := overlap(&a, &b); !ok {
		t.Error("d20s pressed together don't overlap")
	}
	b.Pos.X = 2*ic.Radius + 0.01
	if _, _, _, ok := overlap(&a, &b); ok {
		t.Error("d20s apart overlap")
	}
}

func ExampleGoal_Describe() {
	for _, g := range []Goal{
		{Dice: 5, Sides: 6, Kind: GoalSum, Need: 28},
		{Dice: 4, Sides: 20, Kind: GoalLow, Need: 12},
		{Dice: 3, Sides: 12, Kind: GoalExact, Need: 30},
		{Dice: 9, Sides: 6, Kind: GoalFaces, Need: 8, Face: 5},
		{Dice: 6, Sides: 8, Kind: GoalKind, Need: 4},
		{Dice: 5, Sides: 8, Kind: GoalStraight},
		{Dice: 6, Sides: 6, Kind: GoalUnique},
		{Dice: 8, Sides: 12, Kind: GoalOdd},
	} {
		fmt.Printf("%d%s: %s\n", g.Dice, g.Die(), g.Describe())
	}
	// Output:
	// 5D6: TOTAL 28 OR MORE
	// 4D20: TOTAL 12 OR LESS
	// 3D12: TOTAL EXACTLY 30
	// 9D6: 8+ 5S OR 6S
	// 6D8: 4 OF A KIND
	// 5D8: A STRAIGHT (5 IN A ROW)
	// 6D6: ALL DIFFERENT
	// 8D12: ALL ODD
}
