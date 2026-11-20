// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package dice is the Beggars Table's dice: picking goals that match the
// table's odds, a 3D rigid-body simulation of the throw, and the recorded
// track the client replays. The server throws the dice in the simulation and
// reads the faces they come to rest on; that is the result.
package dice

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// Goal kinds.
const (
	GoalSum      = "sum"      // the dice add up to Need or more
	GoalLow      = "low"      // the dice add up to Need or less
	GoalExact    = "exact"    // the dice add up to exactly Need
	GoalFaces    = "faces"    // at least Need of the dice show Face or higher
	GoalKind     = "kind"     // at least Need of the dice show the same number
	GoalUnique   = "unique"   // every die shows a different number
	GoalStraight = "straight" // the dice show a run of numbers in a row
	GoalOdd      = "odd"      // every die shows an odd number
	GoalEven     = "even"     // every die shows an even number
)

// Kinds is every kind of goal.
var Kinds = []string{GoalSum, GoalLow, GoalExact, GoalFaces, GoalKind, GoalUnique, GoalStraight, GoalOdd, GoalEven}

// MaxDice is the most dice a goal uses.
const MaxDice = 10

// maxDiceFor keeps the bigger dice to a handful, so they fit the tray.
func maxDiceFor(sides int) int {
	switch sides {
	case 8:
		return 8
	case 12:
		return 6
	case 20:
		return 5
	}
	return MaxDice
}

// Goal is what a throw has to make.
type Goal struct {
	Dice  int     `json:"dice"`
	Sides int     `json:"sides,omitempty"` // 6 when not set
	Kind  string  `json:"kind"`
	Need  int     `json:"need"`
	Face  int     `json:"face,omitempty"` // GoalFaces: the lowest face that counts
	P     float64 `json:"p"`              // exact chance of making it with fair dice
}

// S is how many sides the dice have.
func (g Goal) S() int {
	if g.Sides == 0 {
		return 6
	}
	return g.Sides
}

// Odds is the goal as "1 in X".
func (g Goal) Odds() float64 {
	if g.P <= 0 {
		return math.Inf(1)
	}
	return 1 / g.P
}

func total(faces []int) int {
	n := 0
	for _, f := range faces {
		n += f
	}
	return n
}

// counts is how many dice show each number.
func counts(faces []int) map[int]int {
	c := map[int]int{}
	for _, f := range faces {
		c[f]++
	}
	return c
}

// Made says whether faces make the goal.
func (g Goal) Made(faces []int) bool {
	switch g.Kind {
	case GoalSum:
		return total(faces) >= g.Need
	case GoalLow:
		return total(faces) <= g.Need
	case GoalExact:
		return total(faces) == g.Need
	case GoalFaces, GoalKind:
		return g.Score(faces) >= g.Need
	case GoalUnique:
		return len(counts(faces)) == len(faces)
	case GoalStraight:
		if len(faces) == 0 || len(counts(faces)) != len(faces) {
			return false
		}
		lo, hi := faces[0], faces[0]
		for _, f := range faces {
			lo, hi = min(lo, f), max(hi, f)
		}
		return hi-lo == len(faces)-1
	case GoalOdd, GoalEven:
		return g.Score(faces) == len(faces)
	}
	return false
}

// Score is the number that matters for the goal: the total, the dice that
// count, the biggest set of matching dice, how many different numbers, or
// the dice of the right parity.
func (g Goal) Score(faces []int) int {
	switch g.Kind {
	case GoalFaces:
		n := 0
		for _, f := range faces {
			if f >= g.Face {
				n++
			}
		}
		return n
	case GoalKind:
		best := 0
		for _, c := range counts(faces) {
			best = max(best, c)
		}
		return best
	case GoalUnique, GoalStraight:
		return len(counts(faces))
	case GoalOdd, GoalEven:
		n := 0
		for _, f := range faces {
			if (f%2 == 1) == (g.Kind == GoalOdd) {
				n++
			}
		}
		return n
	}
	return total(faces)
}

// Counts says which dice count toward the goal, to light them up.
func (g Goal) Counts(faces []int) []bool {
	out := make([]bool, len(faces))
	switch g.Kind {
	case GoalFaces:
		for i, f := range faces {
			out[i] = f >= g.Face
		}
	case GoalKind:
		bestN, bestF := 0, 0
		for f, c := range counts(faces) {
			if c > bestN || c == bestN && f > bestF {
				bestN, bestF = c, f
			}
		}
		for i, f := range faces {
			out[i] = bestN >= 2 && f == bestF
		}
	case GoalOdd, GoalEven:
		for i, f := range faces {
			out[i] = (f%2 == 1) == (g.Kind == GoalOdd)
		}
	}
	return out
}

// faceWord is what a die that counts is called: "SIXES", "20S", "5S OR 6S",
// "9 OR HIGHER".
func (g Goal) faceWord() string {
	switch {
	case g.Face == g.S() && g.S() == 6:
		return "SIXES"
	case g.Face == g.S():
		return fmt.Sprintf("%dS", g.Face)
	case g.Face == g.S()-1:
		return fmt.Sprintf("%dS OR %dS", g.Face, g.S())
	}
	return fmt.Sprintf("%d OR HIGHER", g.Face)
}

// Die names the dice: "D6", "D20".
func (g Goal) Die() string { return fmt.Sprintf("D%d", g.S()) }

// Describe says the goal in a few words: "TOTAL 29 OR MORE", "4 OF A
// KIND", "A STRAIGHT (4 IN A ROW)".
func (g Goal) Describe() string {
	switch g.Kind {
	case GoalSum:
		if g.Need <= g.Dice {
			return "ANYTHING"
		}
		if g.Need == g.S()*g.Dice {
			return fmt.Sprintf("TOTAL %d (ALL %dS)", g.Need, g.S())
		}
		return fmt.Sprintf("TOTAL %d OR MORE", g.Need)
	case GoalLow:
		if g.Need == g.Dice {
			return fmt.Sprintf("TOTAL %d (ALL 1S)", g.Need)
		}
		return fmt.Sprintf("TOTAL %d OR LESS", g.Need)
	case GoalExact:
		return fmt.Sprintf("TOTAL EXACTLY %d", g.Need)
	case GoalFaces:
		switch {
		case g.Dice == 1 && g.Face == g.S():
			return fmt.Sprintf("A %d", g.Face)
		case g.Dice == 1:
			return fmt.Sprintf("%d OR HIGHER", g.Face)
		case g.Need == g.Dice:
			return "ALL " + g.faceWord()
		case g.Need == 1 && g.Face == g.S():
			return fmt.Sprintf("ANY %d", g.Face)
		}
		return fmt.Sprintf("%d+ %s", g.Need, g.faceWord())
	case GoalKind:
		if g.Need == g.Dice {
			return "ALL MATCHING"
		}
		return fmt.Sprintf("%d OF A KIND", g.Need)
	case GoalUnique:
		return "ALL DIFFERENT"
	case GoalStraight:
		return fmt.Sprintf("A STRAIGHT (%d IN A ROW)", g.Dice)
	case GoalOdd:
		return "ALL ODD"
	case GoalEven:
		return "ALL EVEN"
	}
	return "?"
}

// ---------------------------------------------------------------- chances

// sumDist[t] is the chance n dice of s sides total exactly t.
func sumDist(n, s int) []float64 {
	d := []float64{1}
	for k := 0; k < n; k++ {
		next := make([]float64, len(d)+s)
		for t, p := range d {
			if p == 0 {
				continue
			}
			for f := 1; f <= s; f++ {
				next[t+f] += p / float64(s)
			}
		}
		d = next
	}
	return d
}

func binom(n, k int) float64 {
	r := 1.0
	for i := 1; i <= k; i++ {
		r = r * float64(n-k+i) / float64(i)
	}
	return r
}

// facesAtLeast is the chance k or more of n dice show face or higher.
func facesAtLeast(n, k, face, s int) float64 {
	p := float64(s-face+1) / float64(s)
	sum := 0.0
	for i := k; i <= n; i++ {
		sum += binom(n, i) * math.Pow(p, float64(i)) * math.Pow(1-p, float64(n-i))
	}
	return sum
}

// kindAtLeast is the chance some number shows on k or more of n dice: one
// minus the chance every number shows fewer than k times. That's counted
// as n! times the x^n term of (sum over j<k of x^j/j!)^s, over s^n.
func kindAtLeast(n, k, s int) float64 {
	term := make([]float64, k)
	f := 1.0
	for j := 0; j < k; j++ {
		if j > 0 {
			f *= float64(j)
		}
		term[j] = 1 / f
	}
	poly := []float64{1}
	for i := 0; i < s; i++ {
		next := make([]float64, min(len(poly)+k-1, n+1))
		for a, pa := range poly {
			for b, tb := range term {
				if a+b <= n {
					next[a+b] += pa * tb
				}
			}
		}
		poly = next
	}
	below := 0.0
	if n < len(poly) {
		below = poly[n]
		for i := 2; i <= n; i++ {
			below *= float64(i)
		}
		below /= math.Pow(float64(s), float64(n))
	}
	return 1 - below
}

// chance works out a goal's exact chance with fair dice.
func chance(g Goal) float64 {
	n, s := g.Dice, g.S()
	switch g.Kind {
	case GoalSum, GoalLow, GoalExact:
		p := 0.0
		for t, q := range sumDist(n, s) {
			if g.Kind == GoalSum && t >= g.Need || g.Kind == GoalLow && t <= g.Need || g.Kind == GoalExact && t == g.Need {
				p += q
			}
		}
		return p
	case GoalFaces:
		return facesAtLeast(n, g.Need, g.Face, s)
	case GoalKind:
		return kindAtLeast(n, g.Need, s)
	case GoalUnique, GoalStraight:
		if n > s {
			return 0
		}
		// different numbers on every die: s!/(s-n)! of the s^n outcomes
		p := 1.0
		for i := 0; i < n; i++ {
			p *= float64(s-i) / float64(s)
		}
		if g.Kind == GoalStraight {
			// of those, the runs: s-n+1 of them, each in n! orders
			perm, fact := 1.0, 1.0
			for i := 0; i < n; i++ {
				perm *= float64(s - i)
				fact *= float64(i + 1)
			}
			p *= float64(s-n+1) * fact / perm
		}
		return p
	case GoalOdd, GoalEven:
		return math.Pow(float64(s/2)/float64(s), float64(n))
	}
	return 0
}

// candidates is every goal with a sensible number of dice.
func candidates() []Goal {
	var out []Goal
	add := func(g Goal) {
		if g.P = chance(g); g.P > 0 && g.P < 1 {
			out = append(out, g)
		}
	}
	for _, s := range Sides {
		for n := 1; n <= maxDiceFor(s); n++ {
			for t := n + 1; t <= s*n; t++ {
				add(Goal{Dice: n, Sides: s, Kind: GoalSum, Need: t})
			}
			for face := s; face >= 2; face-- {
				for k := 1; k <= n; k++ {
					add(Goal{Dice: n, Sides: s, Kind: GoalFaces, Need: k, Face: face})
				}
			}
			if n < 2 {
				continue
			}
			for t := n; t < s*n; t++ {
				add(Goal{Dice: n, Sides: s, Kind: GoalLow, Need: t})
			}
			for t := n; t <= s*n; t++ {
				add(Goal{Dice: n, Sides: s, Kind: GoalExact, Need: t})
			}
			for k := 2; k <= n; k++ {
				add(Goal{Dice: n, Sides: s, Kind: GoalKind, Need: k})
			}
			add(Goal{Dice: n, Sides: s, Kind: GoalOdd})
			add(Goal{Dice: n, Sides: s, Kind: GoalEven})
			if n >= 3 && n <= s {
				add(Goal{Dice: n, Sides: s, Kind: GoalUnique})
				add(Goal{Dice: n, Sides: s, Kind: GoalStraight})
			}
		}
	}
	return out
}

var allGoals = candidates()

// Plan picks the goal whose chance is closest to 1 in odds with six-sided
// dice, each extra die having to buy a clearly closer match.
func Plan(odds int) Goal {
	if odds <= 1 {
		return Goal{Dice: 1, Sides: 6, Kind: GoalSum, Need: 1, P: 1}
	}
	want := 1 / float64(odds)
	var best Goal
	bestErr := math.Inf(1)
	for _, g := range allGoals {
		if g.S() != 6 || g.Kind != GoalSum && g.Kind != GoalFaces {
			continue
		}
		if e := math.Abs(math.Log(g.P/want)) + 0.02*float64(g.Dice); e < bestErr-1e-12 {
			best, bestErr = g, e
		}
	}
	return best
}

// Pick chooses a fresh goal near 1 in odds (between about three quarters
// and one and a third of it), unlike prev: a different kind of goal, and
// usually different dice. The kind is picked first, evenly, so every kind
// turns up, not just the ones with the most variations; then the dice,
// evenly; then a goal, leaning toward fewer dice.
func Pick(odds int, prev *Goal, rng *rand.Rand) Goal {
	if odds <= 1 {
		return Plan(1)
	}
	want := 1 / float64(odds)
	for _, band := range []float64{1.33, 1.8, 3} {
		byKind := map[string][]Goal{}
		for _, g := range allGoals {
			if r := g.P / want; r < 1/band || r > band {
				continue
			}
			if g.Dice > 1 && g.Kind == GoalFaces && g.Need == 1 && g.Face < g.S() {
				continue // "1+ showing 3 or higher" is a dull goal
			}
			if prev != nil && g.Kind == prev.Kind {
				continue
			}
			byKind[g.Kind] = append(byKind[g.Kind], g)
		}
		if len(byKind) == 0 {
			continue
		}
		kinds := make([]string, 0, len(byKind))
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		list := byKind[kinds[rng.Intn(len(kinds))]]
		if prev != nil {
			var other []Goal
			for _, g := range list {
				if g.S() != prev.S() {
					other = append(other, g)
				}
			}
			if len(other) > 0 && rng.Intn(4) != 0 {
				list = other
			}
		}
		bySides := map[int][]Goal{}
		for _, g := range list {
			bySides[g.S()] = append(bySides[g.S()], g)
		}
		sides := make([]int, 0, len(bySides))
		for s := range bySides {
			sides = append(sides, s)
		}
		sort.Ints(sides)
		list = bySides[sides[rng.Intn(len(sides))]]
		weight := 0.0
		for _, g := range list {
			weight += 1 / float64(g.Dice)
		}
		x := rng.Float64() * weight
		for _, g := range list {
			if x -= 1 / float64(g.Dice); x <= 0 {
				return g
			}
		}
		return list[len(list)-1]
	}
	return Plan(odds)
}
