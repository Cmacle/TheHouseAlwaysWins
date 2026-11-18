// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// quiet resets the speech state so the next line isn't blocked by timing.
func quiet(g *Game) {
	g.speech = speech{}
	g.sayQueue = nil
}

func TestTalkBudgetLimitsSmallTalk(t *testing.T) {
	g := storyGame(t)
	said := 0
	for i := 0; i < 10; i++ {
		quiet(g)
		if g.say(banter.EvIdle, nil, banter.Vars{}, 1e9, prioLow) {
			said++
		}
	}
	if said != g.talkBudget() {
		t.Fatalf("said %d small lines in a hand, budget is %d", said, g.talkBudget())
	}
	quiet(g)
	if !g.say(banter.EvBust, g.seats[0], banter.Vars{Total: 24}, 1e9, prioHigh) {
		t.Fatal("a bust should still get a reaction over budget")
	}
}

func TestReactToBetChase(t *testing.T) {
	g := storyGame(t)
	p := g.seats[0]
	if _, _, err := g.store.Register("alice", "pw123456", nil, 1000, ""); err != nil {
		t.Fatal(err)
	}
	p.prevBet, p.prevDelta = 100, -300
	for i := 0; i < 100 && g.speech.text == ""; i++ {
		quiet(g)
		g.handTalk = 0
		g.reactToBet(p, 200)
	}
	if g.speech.text == "" {
		t.Fatal("no reaction to chasing losses")
	}
	for _, l := range banter.Pool(banter.EvChase, false) {
		if strings.HasPrefix(banter.Fill(l, banter.Vars{Name: "alice", Amount: 300}), g.speech.text) {
			return
		}
	}
	t.Fatalf("expected a chase line, got %q", g.speech.text)
}

func TestDareTarget(t *testing.T) {
	g := storyGame(t)
	g.store.Register("alice", "pw123456", nil, 1000, "")
	p := g.seats[0]
	p.eligible, p.prevBet = true, 10
	target, amt := g.dareTarget()
	if target != p || amt <= 10 || amt > g.cfg.MaxBet {
		t.Fatalf("dare target %v amount %d", target, amt)
	}
	p.prevBet = 900 // already betting most of their stack
	if target, _ := g.dareTarget(); target != nil {
		t.Fatal("dared somebody already betting big")
	}
}

func TestNudgeSlowBettor(t *testing.T) {
	g := storyGame(t)
	g.store.Register("alice", "pw123456", nil, 1000, "")
	p := g.seats[0]
	p.eligible = true
	g.phase = proto.PhaseBetting
	g.bettingAt = time.Now().Add(-10 * time.Second)
	g.dared = true
	g.deadline = time.Now().Add(4 * time.Second)
	for i := 0; i < 100 && g.speech.text == ""; i++ {
		quiet(g)
		g.handTalk, g.betNudged = 0, false
		g.nudge()
	}
	if !g.betNudged || g.speech.text == "" {
		t.Fatal("didn't nag the slow bettor")
	}
}
