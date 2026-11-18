// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package poker has the rules of Texas Hold'em that don't depend on the
// table: the deck, hand ranking and equity estimates.
package poker

import (
	"fmt"
	"math/rand"
	"sort"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
)

// Card is a playing card (shared with blackjack so the client draws it the
// same way). Rank 1 = Ace ... 13 = King.
type Card = bj.Card

// high is a card's rank for poker: 2..14, Ace high.
func high(c Card) int {
	if c.Rank == 1 {
		return 14
	}
	return int(c.Rank)
}

// Deck is a shuffled 52-card deck.
type Deck struct {
	cards []Card
	pos   int
}

// NewDeck shuffles a fresh deck.
func NewDeck(rng *rand.Rand) *Deck {
	d := &Deck{cards: make([]Card, 0, 52)}
	for s := uint8(0); s < 4; s++ {
		for r := uint8(1); r <= 13; r++ {
			d.cards = append(d.cards, Card{Rank: r, Suit: s})
		}
	}
	rng.Shuffle(len(d.cards), func(i, j int) { d.cards[i], d.cards[j] = d.cards[j], d.cards[i] })
	return d
}

// Draw deals the next card.
func (d *Deck) Draw() Card {
	c := d.cards[d.pos]
	d.pos++
	return c
}

// Hand categories, weakest first.
const (
	HighCard = iota
	Pair
	TwoPair
	Trips
	Straight
	Flush
	FullHouse
	Quads
	StraightFlush
	RoyalFlush // a straight flush to the ace; ranked as a straight flush
)

// CategoryNames names the categories.
var CategoryNames = []string{"High Card", "Pair", "Two Pair", "Three of a Kind", "Straight", "Flush",
	"Full House", "Four of a Kind", "Straight Flush", "Royal Flush"}

// Score ranks a hand: higher is better. Equal scores split the pot.
type Score uint32

// Category is the kind of hand a score is.
func (s Score) Category() int {
	c := int(s >> 20)
	if c == StraightFlush && (s>>16)&0xF == 14 {
		return RoyalFlush
	}
	return c
}

// Name describes the hand ("Full House, Kings full of Sevens").
func (s Score) Name() string {
	c := s.Category()
	k := func(i int) string { return rankName(int(s>>(16-4*i)) & 0xF) }
	switch c {
	case HighCard:
		return k(0) + " high"
	case Pair:
		return "Pair of " + plural(k(0))
	case TwoPair:
		return "Two Pair, " + plural(k(0)) + " and " + plural(k(1))
	case Trips:
		return "Three " + plural(k(0))
	case Straight:
		return "Straight, " + k(0) + " high"
	case Flush:
		return "Flush, " + k(0) + " high"
	case FullHouse:
		return "Full House, " + plural(k(0)) + " full of " + plural(k(1))
	case Quads:
		return "Four " + plural(k(0))
	case StraightFlush:
		return "Straight Flush, " + k(0) + " high"
	case RoyalFlush:
		return "Royal Flush"
	}
	return "?"
}

func rankName(r int) string {
	switch r {
	case 14:
		return "Ace"
	case 13:
		return "King"
	case 12:
		return "Queen"
	case 11:
		return "Jack"
	case 10:
		return "Ten"
	}
	if r >= 2 && r <= 9 {
		return []string{"", "", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"}[r]
	}
	return fmt.Sprint(r)
}

func plural(n string) string {
	if n == "Six" {
		return "Sixes"
	}
	return n + "s"
}

func score(cat int, ranks ...int) Score {
	s := Score(cat) << 20
	for i, r := range ranks {
		if i >= 5 {
			break
		}
		s |= Score(r) << (16 - 4*i)
	}
	return s
}

// Eval5 scores exactly five cards.
func Eval5(c [5]Card) Score {
	var r [5]int
	flush := true
	for i := range c {
		r[i] = high(c[i])
		if c[i].Suit != c[0].Suit {
			flush = false
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(r[:])))
	straightHigh := 0
	if r[0]-r[4] == 4 && distinct(r) {
		straightHigh = r[0]
	} else if r == [5]int{14, 5, 4, 3, 2} {
		straightHigh = 5 // the wheel
	}
	switch {
	case straightHigh > 0 && flush:
		return score(StraightFlush, straightHigh)
	case flush:
		return score(Flush, r[0], r[1], r[2], r[3], r[4])
	case straightHigh > 0:
		return score(Straight, straightHigh)
	}
	// group by rank: (count, rank), biggest groups first
	type g struct{ n, r int }
	var gs []g
	for i := 0; i < 5; {
		j := i
		for j < 5 && r[j] == r[i] {
			j++
		}
		gs = append(gs, g{j - i, r[i]})
		i = j
	}
	sort.Slice(gs, func(a, b int) bool {
		if gs[a].n != gs[b].n {
			return gs[a].n > gs[b].n
		}
		return gs[a].r > gs[b].r
	})
	ranks := make([]int, len(gs))
	for i, x := range gs {
		ranks[i] = x.r
	}
	switch {
	case gs[0].n == 4:
		return score(Quads, ranks...)
	case gs[0].n == 3 && gs[1].n == 2:
		return score(FullHouse, ranks...)
	case gs[0].n == 3:
		return score(Trips, ranks...)
	case gs[0].n == 2 && gs[1].n == 2:
		return score(TwoPair, ranks...)
	case gs[0].n == 2:
		return score(Pair, ranks...)
	}
	return score(HighCard, ranks...)
}

func distinct(r [5]int) bool {
	for i := 1; i < 5; i++ {
		if r[i] == r[i-1] {
			return false
		}
	}
	return true
}

// Best scores the best five of the given cards (5 to 7), and returns those
// five cards.
func Best(cards []Card) (Score, [5]Card) {
	var best Score
	var bestHand [5]Card
	n := len(cards)
	if n < 5 {
		return 0, bestHand
	}
	var h [5]Card
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			for c := b + 1; c < n; c++ {
				for d := c + 1; d < n; d++ {
					for e := d + 1; e < n; e++ {
						h = [5]Card{cards[a], cards[b], cards[c], cards[d], cards[e]}
						if s := Eval5(h); s > best {
							best, bestHand = s, h
						}
					}
				}
			}
		}
	}
	return best, bestHand
}

// Equity estimates the chance hole+board beats `opponents` random hands,
// with ties counting as a share. sims random run-outs are tried.
func Equity(hole []Card, board []Card, opponents, sims int, rng *rand.Rand) float64 {
	if opponents < 1 {
		return 1
	}
	used := map[Card]bool{}
	for _, c := range hole {
		used[c] = true
	}
	for _, c := range board {
		used[c] = true
	}
	var rest []Card
	for s := uint8(0); s < 4; s++ {
		for r := uint8(1); r <= 13; r++ {
			if c := (Card{Rank: r, Suit: s}); !used[c] {
				rest = append(rest, c)
			}
		}
	}
	need := 5 - len(board) + 2*opponents
	if need > len(rest) {
		return 0
	}
	total := 0.0
	mine := make([]Card, 0, 7)
	theirs := make([]Card, 0, 7)
	full := make([]Card, 0, 5)
	for i := 0; i < sims; i++ {
		// partial shuffle: just the cards we need
		for k := 0; k < need; k++ {
			j := k + rng.Intn(len(rest)-k)
			rest[k], rest[j] = rest[j], rest[k]
		}
		full = append(full[:0], board...)
		full = append(full, rest[:5-len(board)]...)
		mine = append(append(mine[:0], hole...), full...)
		me, _ := Best(mine)
		ties, lost := 0, false
		for o := 0; o < opponents; o++ {
			off := 5 - len(board) + 2*o
			theirs = append(append(theirs[:0], rest[off], rest[off+1]), full...)
			them, _ := Best(theirs)
			if them > me {
				lost = true
				break
			}
			if them == me {
				ties++
			}
		}
		if !lost {
			total += 1 / float64(ties+1)
		}
	}
	return total / float64(sims)
}
