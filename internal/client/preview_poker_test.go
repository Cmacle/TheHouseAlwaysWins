// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image/color"
	"os"
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
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
