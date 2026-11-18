// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package poker

import "sort"

// Pot is one main or side pot.
type Pot struct {
	Amount   int64
	Eligible []int // players who can win it
	Winners  []int
}

// Settle splits the chips everyone put in into the main pot and side pots
// and pays each to the best eligible hand. contrib is what each player put
// in this hand, folded who folded, scores their hands (ignored for folded
// players), order the players from the left of the button (who gets the odd
// chip when a pot splits unevenly). It returns what each player wins and the
// pots.
func Settle(contrib []int64, folded []bool, scores []Score, order []int) ([]int64, []Pot) {
	n := len(contrib)
	won := make([]int64, n)
	left := append([]int64(nil), contrib...)
	// the distinct contribution levels of players still in
	var levels []int64
	seen := map[int64]bool{}
	for i := 0; i < n; i++ {
		if !folded[i] && contrib[i] > 0 && !seen[contrib[i]] {
			seen[contrib[i]] = true
			levels = append(levels, contrib[i])
		}
	}
	sort.Slice(levels, func(a, b int) bool { return levels[a] < levels[b] })
	var pots []Pot
	prev := int64(0)
	for _, lv := range levels {
		var p Pot
		for i := 0; i < n; i++ {
			take := min(left[i], lv-prev)
			if take > 0 {
				p.Amount += take
				left[i] -= take
			}
			if !folded[i] && contrib[i] >= lv {
				p.Eligible = append(p.Eligible, i)
			}
		}
		prev = lv
		if p.Amount > 0 {
			pots = append(pots, p)
		}
	}
	// chips above the top level (folded players who put in more than anyone
	// still in, which can't really happen) go to the last pot
	var extra int64
	for i := range left {
		extra += left[i]
	}
	if extra > 0 && len(pots) > 0 {
		pots[len(pots)-1].Amount += extra
	}
	for k := range pots {
		p := &pots[k]
		var bestScore Score
		for _, i := range p.Eligible {
			if scores[i] > bestScore {
				bestScore = scores[i]
			}
		}
		for _, i := range order {
			for _, e := range p.Eligible {
				if e == i && scores[i] == bestScore {
					p.Winners = append(p.Winners, i)
				}
			}
		}
		if len(p.Winners) == 0 { // everyone folded but one: they're eligible
			p.Winners = p.Eligible
		}
		share := p.Amount / int64(len(p.Winners))
		odd := p.Amount - share*int64(len(p.Winners))
		for j, w := range p.Winners {
			won[w] += share
			if int64(j) < odd {
				won[w]++
			}
		}
	}
	return won, pots
}
