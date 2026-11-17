// Package blackjack implements cards, the shoe and casino blackjack rules.
package blackjack

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand/v2"
)

// Suits, in the row order used by the card sprite sheet.
const (
	Spades = iota
	Hearts
	Diamonds
	Clubs
)

// Card is a playing card. Rank 1 = Ace ... 11 = J, 12 = Q, 13 = K.
// A card with Rank 0 is a face-down (hidden) card.
type Card struct {
	Rank uint8 `json:"r"`
	Suit uint8 `json:"s"`
}

// Hidden reports whether this is a face-down placeholder card.
func (c Card) Hidden() bool { return c.Rank == 0 }

// Value is the blackjack value of a card, counting aces as 1.
func (c Card) Value() int {
	if c.Rank >= 10 {
		return 10
	}
	return int(c.Rank)
}

func (c Card) String() string {
	if c.Rank == 0 {
		return "??"
	}
	return string("A23456789TJQK"[c.Rank-1]) + string("SHDC"[c.Suit])
}

// Total returns the best total for a set of cards and whether it is soft
// (an ace is being counted as 11).
func Total(cards []Card) (total int, soft bool) {
	aces := 0
	for _, c := range cards {
		if c.Hidden() {
			continue
		}
		if c.Rank == 1 {
			aces++
		}
		total += c.Value()
	}
	if aces > 0 && total+10 <= 21 {
		return total + 10, true
	}
	return total, false
}

// IsBlackjack reports whether the cards are a natural 21.
func IsBlackjack(cards []Card) bool {
	if len(cards) != 2 {
		return false
	}
	t, _ := Total(cards)
	return t == 21
}

// Shoe is a multi-deck shoe with a cut card.
type Shoe struct {
	cards []Card
	pos   int
	cut   int
	rng   *rand.Rand
}

// NewShoe creates and shuffles a shoe. penetration is the fraction of the shoe
// dealt before a reshuffle (e.g. 0.75).
func NewShoe(decks int, penetration float64) *Shoe {
	if decks < 1 {
		decks = 1
	}
	if penetration < 0.2 || penetration > 0.95 {
		penetration = 0.75
	}
	var seed [32]byte
	_, _ = crand.Read(seed[:])
	s := &Shoe{rng: rand.New(rand.NewChaCha8(seed))}
	for d := 0; d < decks; d++ {
		for suit := uint8(0); suit < 4; suit++ {
			for rank := uint8(1); rank <= 13; rank++ {
				s.cards = append(s.cards, Card{Rank: rank, Suit: suit})
			}
		}
	}
	s.cut = int(float64(len(s.cards)) * penetration)
	s.Shuffle()
	return s
}

// NewShoeSeeded is a deterministic shoe for tests.
func NewShoeSeeded(decks int, seed uint64) *Shoe {
	s := NewShoe(decks, 0.75)
	var b [32]byte
	binary.LittleEndian.PutUint64(b[:], seed)
	s.rng = rand.New(rand.NewChaCha8(b))
	s.Shuffle()
	return s
}

// Shuffle shuffles every card back into the shoe.
func (s *Shoe) Shuffle() {
	s.rng.Shuffle(len(s.cards), func(i, j int) { s.cards[i], s.cards[j] = s.cards[j], s.cards[i] })
	s.pos = 0
}

// NeedsShuffle reports whether the cut card has been reached.
func (s *Shoe) NeedsShuffle() bool { return s.pos >= s.cut }

// Remaining is the number of undealt cards.
func (s *Shoe) Remaining() int { return len(s.cards) - s.pos }

// Draw deals the next card. If the shoe runs out entirely it reshuffles.
func (s *Shoe) Draw() Card {
	if s.pos >= len(s.cards) {
		s.Shuffle()
	}
	c := s.cards[s.pos]
	s.pos++
	return c
}

// Rules are the table rules.
type Rules struct {
	DealerHitsSoft17 bool
	BlackjackNum     int64 // payout numerator (3 for 3:2)
	BlackjackDen     int64 // payout denominator (2 for 3:2)
	DoubleAfterSplit bool
	DoubleOn         string // "any", "9-11", "10-11"
	MaxHands         int    // maximum hands after splitting
	ResplitAces      bool
	HitSplitAces     bool
	Surrender        bool // late surrender
}

// Hand is one player hand (a seat can hold several after splits).
type Hand struct {
	Cards       []Card
	Bet         int64 // current wager on this hand, including doubles
	Doubled     bool
	FromSplit   bool
	SplitAces   bool
	Surrendered bool
	Done        bool
	Result      string // "", "win", "lose", "push", "blackjack", "bust", "surrender"
	Payout      int64  // total returned to the player (stake + winnings)
}

// Total of the hand.
func (h *Hand) Total() (int, bool) { return Total(h.Cards) }

// Natural reports a two-card 21 that was not formed after a split.
func (h *Hand) Natural() bool { return !h.FromSplit && IsBlackjack(h.Cards) }

// Busted reports whether the hand is over 21.
func (h *Hand) Busted() bool { t, _ := h.Total(); return t > 21 }

// CanHit reports whether the player may take another card.
func (r Rules) CanHit(h *Hand) bool {
	if h.Done {
		return false
	}
	if h.SplitAces && !r.HitSplitAces {
		return false
	}
	t, _ := h.Total()
	return t < 21
}

// CanStand reports whether the player may stand.
func (r Rules) CanStand(h *Hand) bool { return !h.Done }

// CanDouble reports whether the player may double down.
func (r Rules) CanDouble(h *Hand) bool {
	if h.Done || len(h.Cards) != 2 {
		return false
	}
	if h.FromSplit && !r.DoubleAfterSplit {
		return false
	}
	if h.SplitAces && !r.HitSplitAces {
		return false
	}
	t, soft := h.Total()
	switch r.DoubleOn {
	case "9-11":
		return !soft && t >= 9 && t <= 11
	case "10-11":
		return !soft && t >= 10 && t <= 11
	}
	return t < 21
}

// CanSplit reports whether the hand may be split given how many hands the
// player currently holds.
func (r Rules) CanSplit(h *Hand, handCount int) bool {
	if h.Done || len(h.Cards) != 2 {
		return false
	}
	max := r.MaxHands
	if max < 1 {
		max = 4
	}
	if handCount >= max {
		return false
	}
	if h.Cards[0].Value() != h.Cards[1].Value() {
		return false
	}
	if h.Cards[0].Rank == 1 && h.FromSplit && !r.ResplitAces {
		return false
	}
	return true
}

// CanSurrender reports whether late surrender is allowed for this hand.
func (r Rules) CanSurrender(h *Hand, handCount int) bool {
	return r.Surrender && !h.Done && handCount == 1 && len(h.Cards) == 2 && !h.FromSplit
}

// DealerShouldHit reports whether the dealer must draw.
func (r Rules) DealerShouldHit(cards []Card) bool {
	t, soft := Total(cards)
	if t < 17 {
		return true
	}
	return t == 17 && soft && r.DealerHitsSoft17
}

// Settle computes the result and payout of a finished hand against the dealer.
func (r Rules) Settle(h *Hand, dealer []Card) {
	num, den := r.BlackjackNum, r.BlackjackDen
	if num <= 0 || den <= 0 {
		num, den = 3, 2
	}
	dt, _ := Total(dealer)
	dealerBJ := IsBlackjack(dealer)
	pt, _ := h.Total()
	switch {
	case h.Surrendered:
		h.Result, h.Payout = "surrender", h.Bet/2
	case pt > 21:
		h.Result, h.Payout = "bust", 0
	case h.Natural() && dealerBJ:
		h.Result, h.Payout = "push", h.Bet
	case h.Natural():
		h.Result, h.Payout = "blackjack", h.Bet+h.Bet*num/den
	case dealerBJ:
		h.Result, h.Payout = "lose", 0
	case dt > 21 || pt > dt:
		h.Result, h.Payout = "win", h.Bet*2
	case pt == dt:
		h.Result, h.Payout = "push", h.Bet
	default:
		h.Result, h.Payout = "lose", 0
	}
	h.Done = true
}
