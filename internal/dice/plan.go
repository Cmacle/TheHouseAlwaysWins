// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package dice is the Beggars Table's dice: picking a goal that matches the
// table's odds, a 3D rigid-body simulation of the throw, and the recorded
// track the client replays. The server throws the dice in the simulation and
// reads the faces they come to rest on; that is the result.
package dice

import (
	"fmt"
	"math"
)

// Goal kinds.
const (
	GoalSum   = "sum"   // the dice add up to Need or more
	GoalFaces = "faces" // at least Need of the dice show Face or higher
)

// MaxDice is the most dice a goal uses.
const MaxDice = 10

// Goal is what a throw has to make.
type Goal struct {
	Dice int     `json:"dice"`
	Kind string  `json:"kind"`
	Need int     `json:"need"`
	Face int     `json:"face,omitempty"` // GoalFaces: the lowest face that counts
	P    float64 `json:"p"`              // exact chance of making it with fair dice
}

// Odds is the goal as "1 in X".
func (g Goal) Odds() float64 {
	if g.P <= 0 {
		return math.Inf(1)
	}
	return 1 / g.P
}

// Made says whether faces make the goal.
func (g Goal) Made(faces []int) bool { return g.Score(faces) >= g.Need }

// Counts says whether one die's face counts toward a GoalFaces goal (for
// the sum, every die counts).
func (g Goal) Counts(face int) bool { return g.Kind != GoalFaces || face >= g.Face }

// Score is the number the goal counts: the total, or the dice that count.
func (g Goal) Score(faces []int) int {
	n := 0
	for _, f := range faces {
		if g.Kind == GoalFaces {
			if f >= g.Face {
				n++
			}
		} else {
			n += f
		}
	}
	return n
}

// what a counting die is called: "SIXES", "5S OR 6S", "4 OR HIGHER"
func (g Goal) faceWord() string {
	switch g.Face {
	case 6:
		return "SIXES"
	case 5:
		return "5S OR 6S"
	}
	return fmt.Sprintf("%d OR HIGHER", g.Face)
}

// Describe says the goal in a few words: "29 OR MORE", "4+ SIXES",
// "ALL 4 OR HIGHER".
func (g Goal) Describe() string {
	if g.Kind == GoalFaces {
		switch {
		case g.Dice == 1 && g.Face == 6:
			return "A SIX"
		case g.Dice == 1:
			return fmt.Sprintf("%d OR HIGHER", g.Face)
		case g.Need == g.Dice:
			return "ALL " + g.faceWord()
		case g.Need == 1 && g.Face == 6:
			return "ANY SIX"
		}
		return fmt.Sprintf("%d+ %s", g.Need, g.faceWord())
	}
	switch {
	case g.Need <= g.Dice:
		return "ANYTHING"
	case g.Need == 6*g.Dice:
		return fmt.Sprintf("%d (ALL SIXES)", g.Need)
	}
	return fmt.Sprintf("%d OR MORE", g.Need)
}

// sumAtLeast[t] is the chance n dice total t or more.
func sumAtLeast(n int) []float64 {
	ways := []float64{1}
	for d := 0; d < n; d++ {
		next := make([]float64, len(ways)+6)
		for s, w := range ways {
			for f := 1; f <= 6; f++ {
				next[s+f] += w
			}
		}
		ways = next
	}
	total := math.Pow(6, float64(n))
	out := make([]float64, len(ways)+1)
	for t := len(ways) - 1; t >= 0; t-- {
		out[t] = out[t+1] + ways[t]/total
	}
	return out
}

func binom(n, k int) float64 {
	r := 1.0
	for i := 1; i <= k; i++ {
		r = r * float64(n-k+i) / float64(i)
	}
	return r
}

// facesAtLeast is the chance k or more of n dice show face or higher.
func facesAtLeast(n, k, face int) float64 {
	p := float64(7-face) / 6
	sum := 0.0
	for i := k; i <= n; i++ {
		sum += binom(n, i) * math.Pow(p, float64(i)) * math.Pow(1-p, float64(n-i))
	}
	return sum
}

// Plan picks the goal whose chance is closest to 1 in odds. Each extra die
// has to buy a clearly closer match, so it stays at a sensible handful.
func Plan(odds int) Goal {
	if odds <= 1 {
		return Goal{Dice: 1, Kind: GoalSum, Need: 1, P: 1}
	}
	want := 1 / float64(odds)
	var best Goal
	bestErr := math.Inf(1)
	try := func(c Goal) {
		e := math.Abs(math.Log(c.P/want)) + 0.02*float64(c.Dice)
		if e < bestErr-1e-12 {
			best, bestErr = c, e
		}
	}
	for n := 1; n <= MaxDice; n++ {
		for face := 6; face >= 2; face-- { // sixes first: the most fun to watch
			for k := 1; k <= n; k++ {
				try(Goal{Dice: n, Kind: GoalFaces, Need: k, Face: face, P: facesAtLeast(n, k, face)})
			}
		}
		sa := sumAtLeast(n)
		for t := n + 1; t <= 6*n; t++ {
			try(Goal{Dice: n, Kind: GoalSum, Need: t, P: sa[t]})
		}
	}
	return best
}
