package client

import (
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func TestSplitDoesNotRedealTheMovedCard(t *testing.T) {
	a, _ := newRecApp(t)
	a.now = time.Now()
	st := baseState(5)
	eight, eight2, king := bj.Card{Rank: 8, Suit: 0}, bj.Card{Rank: 8, Suit: 2}, bj.Card{Rank: 13, Suit: 1}
	st.Seats[0].Hands = []proto.HandView{{Cards: []bj.Card{eight, eight2}}}
	a.onState(st)
	a.onState(st)
	a.now = a.now.Add(10 * time.Second)
	split := baseState(5)
	split.Seats[0].Hands = []proto.HandView{{Cards: []bj.Card{eight, king}}, {Cards: []bj.Card{eight2}}}
	a.onState(split)
	if !a.cardSeen["s0_1_0"].Before(a.now) {
		t.Fatal("the moved eight flies in from the shoe again")
	}
	if !a.cardSeen["s0_0_1"].Equal(a.now) {
		t.Fatal("the new king isn't dealt")
	}
}

func TestBunchedCardsAreStaggered(t *testing.T) {
	a, _ := newRecApp(t)
	a.now = time.Now()
	st := baseState(5)
	a.onState(st)
	st2 := baseState(5)
	st2.Seats[0].Hands = []proto.HandView{{Cards: []bj.Card{{Rank: 2}, {Rank: 3}}}}
	st2.Seats[2].Hands = []proto.HandView{{Cards: []bj.Card{{Rank: 4}}}}
	a.onState(st2)
	t0, t1, t2 := a.cardSeen["s0_0_0"], a.cardSeen["s0_0_1"], a.cardSeen["s2_0_0"]
	if !(t0.Before(t1) && t1.Before(t2)) {
		t.Fatalf("not staggered: %v %v %v", t0, t1, t2)
	}
}
