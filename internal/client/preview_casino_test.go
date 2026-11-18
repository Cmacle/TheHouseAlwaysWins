// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func casinoPreviewState(game string) *proto.CasinoState {
	cs := &proto.CasinoState{Table: game, Name: map[string]string{proto.GameBaccarat: "Baccarat", proto.GameRedDog: "Red Dog"}[game], Game: game,
		Phase: "betting", HandNumber: 9, MinBet: 10, MaxBet: 1000, YourSeat: 4, ServerTime: time.Now().UnixMilli(),
		Deadline: time.Now().Add(11 * time.Second).UnixMilli(), TimerTotal: 15000, CanBet: true,
		Settings: proto.Settings{TableChat: true, DealerTrashTalk: true, ServerName: "The House"}, HouseNet: 1520, HouseTier: 1,
		Message: "Bets close when the clock runs out."}
	cs.Spots = []string{"player", "tie", "banker"}
	if game == proto.GameRedDog {
		cs.Spots = []string{"ante"}
	}
	names := []string{"Big_Tony", "", "Deb", "You_There", "LuckyLou", "", "Cmacle"}
	hats := []string{"beret", "", "fascinator", "fedora", "dog_ears", "", "hot_dog"}
	frames := []string{"frame_felt", "", "frame_velvet", "frame_gold", "frame_silver", "", "frame_vault"}
	for i, n := range names {
		sv := proto.CasinoSeat{Seat: i + 1}
		if n != "" {
			sv.Username, sv.Connected, sv.Balance, sv.Hat, sv.Frame = n, true, int64(800+i*311), hats[i], frames[i]
		}
		cs.Seats = append(cs.Seats, sv)
	}
	if game == proto.GameBaccarat {
		cs.Seats[0].Bets = map[string]int64{"banker": 200}
		cs.Seats[2].Bets = map[string]int64{"player": 100, "tie": 25}
		cs.Seats[3].Bets = map[string]int64{"player": 250}
		cs.History = []string{"banker", "banker", "player", "tie", "banker", "player", "player", "player", "banker", "banker", "banker", "banker"}
	} else {
		cs.Seats[0].Bets = map[string]int64{"ante": 100}
		cs.Seats[3].Bets = map[string]int64{"ante": 50}
		cs.Seats[6].Bets = map[string]int64{"ante": 500, "raise": 500}
	}
	return cs
}

func TestPreviewCasino(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	for _, n := range []string{"LuckyLou", "Big_Tony", "Cmacle", "Deb", "You_There"} {
		_ = n
	}
	a.lobby.Tables = append(a.lobby.Tables[:2],
		proto.TableInfo{ID: "baccarat", Name: "Baccarat", Game: proto.GameBaccarat, Stakes: "Bets 10-1,000", Seats: 7, Players: 3, Names: []string{"Deb", "Big_Tony", "You_There"}},
		proto.TableInfo{ID: "reddog", Name: "Red Dog", Game: proto.GameRedDog, Stakes: "Bets 10-1,000", Seats: 7, Players: 2, Names: []string{"Cmacle", "LuckyLou"}},
		proto.TableInfo{ID: "beggars", Name: "Beggars Table", Game: proto.GameBeggar, Stakes: "Under 50 chips only", Seats: 99})
	a.screen = scrLobby
	frame()
	save(t, dir, "60_lobby_all.png", a.fb)

	// baccarat: betting
	a.onJoined(proto.Joined{ID: "baccarat", Game: proto.GameBaccarat, Name: "Baccarat"})
	b := casinoPreviewState(proto.GameBaccarat)
	a.onCasino(b)
	frame()
	save(t, dir, "61_baccarat_betting.png", a.fb)
	// the Player's cards being squeezed
	d := casinoPreviewState(proto.GameBaccarat)
	d.Phase, d.CanBet, d.Deadline, d.HandNumber = "dealing", false, 0, 10
	d.PlayerCards = []bj.Card{{}, {}}
	d.BankerCards = []bj.Card{{}, {}}
	d.Reveal = &proto.CardReveal{Spot: "player", From: 0, Count: 2, Until: time.Now().Add(time.Second).UnixMilli()}
	a.onCasino(d)
	a.csRev.start = time.Now().Add(-time.Second)
	frame()
	save(t, dir, "62_baccarat_reveal.png", a.fb)
	// result: Banker wins 8 to 6
	r := casinoPreviewState(proto.GameBaccarat)
	r.Phase, r.CanBet, r.Deadline, r.HandNumber = "result", false, 0, 10
	r.PlayerCards = []bj.Card{card(4, 0), card(12, 1), card(2, 2)}
	r.BankerCards = []bj.Card{card(5, 3), card(3, 0)}
	r.PlayerTotal, r.BankerTotal, r.Winner, r.Message = 6, 8, "banker", "Banker wins, 8 to 6"
	r.History = append(r.History, "banker")
	for i := range r.Seats {
		r.Seats[i].Settled = r.Seats[i].Username != "" && len(r.Seats[i].Bets) > 0
	}
	r.Seats[0].Result, r.Seats[2].Result, r.Seats[3].Result = 190, -125, -250
	a.csRev = nil
	a.onCasino(r)
	for k := range a.cardSeen {
		a.cardSeen[k] = time.Now().Add(-time.Hour)
	}
	a.flipAt = map[string]time.Time{}
	frame()
	save(t, dir, "63_baccarat_result.png", a.fb)

	// Red Dog: a spread, deciding whether to raise
	a.onJoined(proto.Joined{ID: "reddog", Game: proto.GameRedDog, Name: "Red Dog"})
	rd := casinoPreviewState(proto.GameRedDog)
	rd.Phase, rd.CanBet, rd.CanRaise = "raise", false, true
	rd.Cards = []bj.Card{card(4, 0), card(11, 1)}
	rd.Spread, rd.Pays = 6, "1 to 1"
	rd.Message = "Spread of 6 pays 1 to 1. Raise to double your bet."
	rd.Deadline = time.Now().Add(7 * time.Second).UnixMilli()
	rd.TimerTotal = 10000
	a.onCasino(rd)
	for k := range a.cardSeen {
		a.cardSeen[k] = time.Now().Add(-time.Hour)
	}
	frame()
	save(t, dir, "64_reddog_raise.png", a.fb)
}

// A strip of the animated hats over a few frames, to see them move.
func TestPreviewHatMotion(t *testing.T) {
	a, _, dir := pokerPreviewApp(t)
	ids := []string{"crown", "tiara", "flame", "chip_tower", "nightcap", "halo", "shark_fin", "moneybag", "unicorn", "fascinator"}
	for f := 0; f < 4; f++ {
		a.frame = f * 12
		fillRect(a.fb, a.fb.Rect, a.A.C("felt"))
		for i, id := range ids {
			x, y := 10+i*60, 60
			a.drawFrame([]string{"frame_rainbow", "frame_diamond", "frame_fire", "frame_vault", "frame_marathon", "frame_bone", "frame_velvet", "frame_gold", "frame_ice", "frame_ruby"}[i], x, y)
			blit(a.fb, a.avatarImg("Deb", "", 32), x, y)
			a.drawHat(id, x, y)
		}
		save(t, dir, "70_hats_frame"+string(rune('0'+f))+".png", a.fb)
	}
}
