// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func sampleSports() *proto.SportsState {
	now := time.Now()
	in := func(h float64) int64 { return now.Add(time.Duration(h * float64(time.Hour))).Unix() }
	sp := &proto.SportsState{Table: "sportsbook", Name: "Sportsbook", Updated: now.Add(-40 * time.Second).Unix(), Balance: 1840,
		MinBet: 10, MaxBet: 1000, Settings: proto.Settings{TableChat: true}, HouseNet: 1520, ServerTime: now.UnixMilli(),
		Players: []string{"Deb", "You_There"}, Feed: []string{"Deb bet 250 on T1 at 1.85x (T1 vs Gen.G (BO5))."}}
	teams := [][3]string{{"lol", "T1", "Gen.G"}, {"cs", "Vitality", "MOUZ"}, {"cs", "Natus Vincere", "FaZe Clan"}, {"lol", "Bilibili Gaming", "Hanwha Life Esports"},
		{"cs", "G2 Esports", "Team Spirit"}, {"lol", "Fnatic", "G2 Esports"}, {"cs", "The MongolZ", "Astralis"}, {"cs", "Aurora", "Liquid"},
		{"lol", "Cloud9", "Team Liquid"}, {"cs", "FURIA", "paiN Gaming"}, {"lol", "KT Rolster", "DRX"}, {"cs", "Falcons", "3DMAX"}}
	for i, tm := range teams {
		p := 0.25 + float64(i%5)*0.12
		bo := "BO3"
		if tm[0] == "lol" {
			bo = "BO5"
		}
		m := proto.SportsMarket{ID: fmt.Sprintf("m%d", i), Game: tm[0], Event: fmt.Sprintf("%s vs %s (%s)", tm[1], tm[2], bo),
			Label: "Match winner", Start: in(float64(i)*1.7 - 0.5), Open: i != 0, Live: i == 0, Volume: float64(90000 - i*5000),
			Outcomes: []proto.SportsOutcome{{Name: tm[1], Price: p}, {Name: tm[2], Price: 1 - p}}}
		if i == 2 {
			m.Label = "Map 1 winner"
		}
		if i == 1 {
			m.Outcomes[0].Bets, m.Outcomes[0].Stake = 2, 350
			m.Outcomes[1].Bets, m.Outcomes[1].Stake = 1, 100
		}
		sp.Markets = append(sp.Markets, m)
	}
	sp.Mine = []proto.SportsBet{
		{ID: 7, User: "You_There", Market: "m1", Game: "cs", Event: "Vitality vs MOUZ (BO3)", Label: "Match winner", Pick: "Vitality", Price: 0.62, Stake: 100, Payout: 161, Status: "open"},
		{ID: 3, User: "You_There", Market: "x", Game: "lol", Event: "Gen.G vs HLE (BO5)", Label: "Match winner", Pick: "Gen.G", Price: 0.55, Stake: 200, Payout: 363, Status: "won"},
		{ID: 2, User: "You_There", Market: "y", Game: "cs", Event: "FaZe vs Liquid (BO3)", Label: "Map 2 winner", Pick: "Liquid", Price: 0.4, Stake: 50, Payout: 125, Status: "lost"},
	}
	sp.Others = append([]proto.SportsBet{
		{ID: 8, User: "Deb", Market: "m1", Game: "cs", Event: "Vitality vs MOUZ (BO3)", Label: "Match winner", Pick: "Vitality", Price: 0.6, Stake: 250, Payout: 416, Status: "open"},
		{ID: 9, User: "Big_Tony", Market: "m1", Game: "cs", Event: "Vitality vs MOUZ (BO3)", Label: "Match winner", Pick: "MOUZ", Price: 0.38, Stake: 100, Payout: 263, Status: "open"},
	}, sp.Mine...)
	return sp
}

func TestPreviewSports(t *testing.T) {
	a, frame, dir := pokerPreviewApp(t)
	a.lobby.Tables = append(a.lobby.Tables, proto.TableInfo{ID: "sportsbook", Name: "Sportsbook", Game: proto.GameSports,
		Stakes: "Esports at Polymarket odds, 30 markets, 5 bets riding", Seats: 99, Players: 2, Names: []string{"Deb", "You_There"}})
	a.screen = scrLobby
	frame()
	save(t, dir, "60_lobby_sports.png", a.fb)
	a.onJoined(proto.Joined{ID: "sportsbook", Game: proto.GameSports, Name: "Sportsbook"})
	a.onSports(sampleSports())
	frame()
	save(t, dir, "61_sports_board.png", a.fb)
	a.sbSel = sbSelection{market: "m1", outcome: 0}
	a.sbStake = 100
	frame()
	save(t, dir, "62_sports_slip.png", a.fb)
	a.sbTab = sbMine
	frame()
	save(t, dir, "63_sports_mine.png", a.fb)
	a.sbTab = sbTable
	frame()
	save(t, dir, "64_sports_table.png", a.fb)
}
