// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image/color"
	"os"
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/dice"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func pokerPreviewApp(t *testing.T) (*App, func(), string) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{Username: "You_There", Sound: true}, assets)
	if sz := os.Getenv("PREVIEW_SIZE"); sz != "" {
		var w, h int
		fmt.Sscanf(sz, "%dx%d", &w, &h)
		a.SetSize(w, h)
		t.Cleanup(func() { a.SetSize(640, 360) })
	}
	a.username = "You_There"
	a.profile = proto.Profile{Username: "You_There", Balance: 2400, Admin: true}
	cols := []color.RGBA{{200, 60, 60, 255}, {60, 120, 220, 255}, {60, 180, 90, 255}, {200, 160, 40, 255}, {150, 70, 200, 255}}
	for i, n := range []string{"Cmacle", "LuckyLou", "Big_Tony", "You_There", "Deb"} {
		fakeAvatar(a, n, cols[i])
	}
	a.lobbyKnown = true
	a.lobby = proto.Lobby{Tables: []proto.TableInfo{
		{ID: "main", Name: "Main Table", Game: proto.GameBlackjack, Stakes: "Bets 10-1,000", Seats: 7, Players: 3, Names: []string{"Cmacle", "LuckyLou", "Deb"}},
		{ID: "holdem", Name: "Hold'em", Game: proto.GameHoldem, Stakes: "Blinds 5/10", Seats: 6, Players: 2, Watching: 1, Names: []string{"Big_Tony", "You_There"}, MinBuyIn: 200, MaxBuyIn: 1000, BigBlind: 10},
		{ID: "high-rollers", Name: "High Rollers", Game: proto.GameHoldem, Stakes: "Blinds 50/100", Seats: 6, MinBuyIn: 2000, MaxBuyIn: 10000, BigBlind: 100},
	}}
	return a, func() { a.Frame(Input{MX: -1, MY: -1}) }, dir
}

func pokerPreviewState() *proto.PokerState {
	ps := &proto.PokerState{Table: "holdem", Name: "Hold'em", Phase: "turn", HandNumber: 12, YourSeat: 3, Button: 1, Active: 3,
		SmallBlind: 5, BigBlind: 10, MinBuyIn: 200, MaxBuyIn: 1000, Balance: 1400, Pot: 180,
		Board:    []bj.Card{card(1, 0), card(10, 1), card(7, 1), card(2, 3)},
		Settings: proto.Settings{TableChat: true, DealerTrashTalk: true, ServerName: "The House"},
		HouseNet: 1520, HouseTier: 1, ServerTime: time.Now().UnixMilli(),
		Deadline: time.Now().Add(18 * time.Second).UnixMilli(), TimerTotal: 25000,
		DealerSay: "Raising, You_There? With those hands? They're shaking.", DealerSayID: 3, DealerSayUntil: time.Now().Add(5 * time.Second).UnixMilli(),
		Actions: []string{"fold", "call", "raise"}, ToCall: 40, MinRaise: 80, MaxRaise: 610, Message: "Turn: 2 of spades"}
	ps.Seats = make([]proto.PokerSeat, 7)
	for i := range ps.Seats {
		ps.Seats[i].Seat = i
	}
	ps.Seats[0] = proto.PokerSeat{Seat: 0, Username: "Dale", Dale: true, Stack: 860, Bet: 40, InHand: true, Connected: true, Cards: []bj.Card{{}, {}}, Action: "bet 40"}
	ps.Seats[1] = proto.PokerSeat{Seat: 1, Username: "Big_Tony", Stack: 420, InHand: true, Folded: true, Connected: true, Action: "fold"}
	ps.Seats[5] = proto.PokerSeat{Seat: 5, Username: "LuckyLou", Stack: 300, Bet: 120, InHand: true, Connected: true, Cards: []bj.Card{{}, {}}, Action: "raise 120"}
	ps.Seats[6] = proto.PokerSeat{Seat: 6, Username: "Cmacle", Stack: 2200, Bet: 120, InHand: true, Connected: true, Cards: []bj.Card{{}, {}}, Action: "call 120"}
	ps.Seats[3] = proto.PokerSeat{Seat: 3, Username: "You_There", Stack: 570, InHand: true, Connected: true, Cards: []bj.Card{card(1, 2), card(13, 2)}, Hat: "fedora", Title: "t_bluffer"}
	ps.Seats[4] = proto.PokerSeat{Seat: 4, Username: "Deb", Stack: 990, Bet: 40, InHand: true, Connected: true, Cards: []bj.Card{{}, {}}, Action: "call 40"}
	return ps
}

func TestPreviewPoker(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	a.screen = scrLobby
	frame()
	save(t, dir, "40_lobby.png", a.fb)

	a.openBuyIn(a.lobby.Tables[1], false)
	frame()
	save(t, dir, "41_buyin.png", a.fb)
	a.overlay = ovNone
	a.focusID = ""

	a.onJoined(proto.Joined{ID: "holdem", Game: proto.GameHoldem, Name: "Hold'em"})
	ps := pokerPreviewState()
	a.onPoker(ps)
	a.onChat(proto.Chat{From: "Deb", Seat: 4, Text: "nice hat"})
	for i := 0; i < 3; i++ {
		frame()
	}
	save(t, dir, "42_poker_turn.png", a.fb)

	// showdown
	sd := pokerPreviewState()
	sd.Phase, sd.Actions, sd.Active, sd.Deadline = "showdown", nil, -1, 0
	sd.Board = append(sd.Board, card(1, 3))
	sd.Pot, sd.Message = 300, "You_There wins 300 with three of a kind"
	sd.Seats[0].Cards = []bj.Card{card(10, 0), card(9, 0)}
	sd.Seats[0].Action, sd.Seats[0].Bet = "", 0
	sd.Seats[3].HandName, sd.Seats[3].Won = "Three of a kind", 300
	sd.Seats[4].Cards = []bj.Card{card(7, 0), card(7, 2)}
	sd.Seats[4].HandName, sd.Seats[4].Bet, sd.Seats[4].Action = "Three of a kind", 0, ""
	sd.Seats[0].HandName = "Two pair"
	a.onPoker(sd)
	a.flipAt = map[string]time.Time{}
	for k := range a.cardSeen {
		a.cardSeen[k] = time.Now().Add(-time.Hour)
	}
	frame()
	save(t, dir, "43_poker_showdown.png", a.fb)

	// watching
	w := pokerPreviewState()
	w.YourSeat, w.Actions = -1, nil
	w.Seats[3] = proto.PokerSeat{Seat: 3}
	a.onPoker(w)
	frame()
	save(t, dir, "44_poker_watch.png", a.fb)
}

func TestPreviewBeggar(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	a.lobby.Tables = append(a.lobby.Tables, proto.TableInfo{ID: "beggars", Name: "Beggars Table", Game: proto.GameBeggar, Stakes: "Under 50 chips only", Seats: 99, Players: 1, Names: []string{"Deb"}})
	a.screen = scrLobby
	frame()
	save(t, dir, "50_lobby_beggar.png", a.fb)
	a.onJoined(proto.Joined{ID: "beggars", Game: proto.GameBeggar, Name: "Beggars Table"})
	bs := &proto.BeggarState{Table: "beggars", Name: "Beggars Table", Odds: 1000, Prize: 50, MaxBalance: 50, Balance: 12, CanRoll: true,
		Rolls: 241, TableRolls: 1302, Players: []string{"You_There", "Deb"}, Feed: []string{"Deb rolled 8+ 5s or 6s on 9 dice: 50 chips each for 2 players!"},
		Settings: proto.Settings{TableChat: true, DealerTrashTalk: true}, HouseNet: 1520, HouseTier: 1, ServerTime: time.Now().UnixMilli(),
		DealerSay: "Again, You_There? Persistence. Or a problem.", DealerSayID: 4, DealerSayUntil: time.Now().Add(5 * time.Second).UnixMilli()}
	a.onBeggar(bs)
	frame()
	save(t, dir, "51_beggar_idle.png", a.fb)
	g := dice.Plan(1000)
	bs.Dice, bs.GoalKind, bs.GoalNeed, bs.GoalFace, bs.Chance = g.Dice, g.Kind, g.Need, g.Face, g.P
	roll := throwMsg(1, g.Dice, 42, 0)
	for i, at := range []float64{0.4, 1.25, 1.7, 9} {
		a.onDice(roll)
		a.roll.start = time.Now().Add(-time.Duration(at * float64(time.Second)))
		frame()
		save(t, dir, fmt.Sprintf("52_beggar_roll_%d.png", i), a.fb)
	}
	win := throwMsg(2, g.Dice, 7, 0)
	win.Win = true
	a.onDice(win)
	a.roll.start = time.Now().Add(-12 * time.Second)
	frame()
	save(t, dir, "53_beggar_win.png", a.fb)
	bs.CanRoll, bs.Balance, bs.Reason = false, 80, "This table is for players with fewer than 50 chips. You have 80."
	a.roll = nil
	a.onBeggar(bs)
	frame()
	save(t, dir, "54_beggar_rich.png", a.fb)
}

func TestPreviewPokerDealInReveal(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	a.onJoined(proto.Joined{ID: "holdem", Game: proto.GameHoldem, Name: "Hold'em"})
	r := pokerPreviewState()
	r.Phase, r.Actions, r.Active, r.Board, r.Pot, r.Message = "ready", nil, -1, nil, 0, "New hand. Press DEAL ME IN to play it."
	r.CanDealIn = true
	for i := range r.Seats {
		r.Seats[i].Cards, r.Seats[i].Bet, r.Seats[i].Action, r.Seats[i].InHand, r.Seats[i].Folded = nil, 0, "", false, false
	}
	r.Seats[4].DealtIn, r.Seats[6].DealtIn = true, true
	r.Deadline = time.Now().Add(7 * time.Second).UnixMilli()
	r.TimerTotal = 10000
	a.onPoker(r)
	frame()
	save(t, dir, "45_poker_dealin.png", a.fb)

	// the flop, face down and big
	f := pokerPreviewState()
	f.Phase, f.Actions, f.Active, f.Deadline = "flop", nil, -1, 0
	f.HandNumber = 13
	f.Seats[3].Cards = []bj.Card{card(1, 2), card(10, 3)}
	f.Board = []bj.Card{{}, {}, {}}
	f.Reveal = &proto.PokerReveal{From: 0, Count: 3, Until: time.Now().Add(1500 * time.Millisecond).UnixMilli()}
	a.onPoker(f)
	a.pkRev.start = time.Now().Add(-time.Second)
	frame()
	save(t, dir, "46_poker_reveal_wait.png", a.fb)
	g := *f
	g.Board = []bj.Card{card(1, 0), card(10, 1), card(7, 1)}
	g.Reveal = &proto.PokerReveal{From: 0, Count: 3, Until: f.Reveal.Until, Shown: true}
	a.onPoker(&g)
	a.pkRev.flippedAt = time.Now().Add(-900 * time.Millisecond)
	frame()
	save(t, dir, "47_poker_reveal_flipped.png", a.fb)
}

func TestPreviewPokerActions(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	a.onJoined(proto.Joined{ID: "holdem", Game: proto.GameHoldem, Name: "Hold'em"})
	before := pokerPreviewState()
	before.Actions = nil
	before.Active = 1
	before.Seats[0].Bet, before.Seats[0].Action = 0, ""
	before.Seats[1] = proto.PokerSeat{Seat: 1, Username: "Big_Tony", Stack: 420, InHand: true, Connected: true, Cards: []bj.Card{{}, {}}}
	before.Seats[4].Bet, before.Seats[4].Action = 0, ""
	before.Seats[5].Bet, before.Seats[5].Action = 0, ""
	a.onPoker(before)
	for k := range a.cardSeen {
		a.cardSeen[k] = time.Now().Add(-time.Hour)
	}
	after := pokerPreviewState()
	after.Actions = nil
	after.Seats[5].Bet, after.Seats[5].Action = 0, "check"
	a.onPoker(after)
	for k := range a.cardSeen {
		a.cardSeen[k] = time.Now().Add(-time.Hour)
	}
	// stagger them: fold just thrown, Deb's call chips mid-air, Dale's bet label up, check rings
	for i := range a.pkFX {
		switch a.pkFX[i].seat {
		case 1:
			a.pkFX[i].start = time.Now().Add(-200 * time.Millisecond)
		case 4:
			a.pkFX[i].start = time.Now().Add(-150 * time.Millisecond)
		case 5:
			a.pkFX[i].start = time.Now().Add(-120 * time.Millisecond)
		default:
			a.pkFX[i].start = time.Now().Add(-600 * time.Millisecond)
		}
	}
	frame()
	save(t, dir, "48_poker_actions.png", a.fb)
}

func TestPreviewPokerShowdownReveal(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	a.onJoined(proto.Joined{ID: "holdem", Game: proto.GameHoldem, Name: "Hold'em"})
	base := pokerPreviewState()
	base.Actions, base.Active, base.Deadline = nil, -1, 0
	base.Board = append(base.Board, card(9, 0))
	a.onPoker(base)
	for k := range a.cardSeen {
		a.cardSeen[k] = time.Now().Add(-time.Hour)
	}
	s := pokerPreviewState()
	s.Phase, s.Actions, s.Active, s.Deadline = "shows", nil, -1, 0
	s.Board = base.Board
	s.Message = "Deb shows..."
	s.Reveal = &proto.PokerReveal{Hand: true, Seat: 4, Count: 2, Until: time.Now().Add(1200 * time.Millisecond).UnixMilli()}
	a.onPoker(s)
	a.pkRev.start = time.Now().Add(-time.Second)
	frame()
	save(t, dir, "49_poker_show_wait.png", a.fb)
	s2 := *s
	s2.Seats = append([]proto.PokerSeat{}, s.Seats...)
	s2.Seats[4].Cards = []bj.Card{card(7, 0), card(7, 2)}
	s2.Seats[4].HandName = "Three of a kind, Sevens"
	s2.Message = "Deb: Three of a kind, Sevens"
	s2.Reveal = &proto.PokerReveal{Hand: true, Seat: 4, Count: 2, Until: s.Reveal.Until, Shown: true}
	a.onPoker(&s2)
	a.pkRev.flippedAt = time.Now().Add(-700 * time.Millisecond)
	frame()
	save(t, dir, "49_poker_show_flipped.png", a.fb)
}
