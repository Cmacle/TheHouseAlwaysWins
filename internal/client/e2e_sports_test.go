// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/server"
)

// A player opens the Sportsbook, sees the board, picks a side and bets with
// the bet slip; another player at the table sees the bet.
func TestSportsEndToEnd(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	pm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{{
			"id": "1", "title": "Counter-Strike: Vitality vs MOUZ (BO3)",
			"tags": []map[string]string{{"label": "Counter-Strike 2", "slug": "counter-strike-2"}},
			"markets": []map[string]any{{"id": "m1", "question": "Counter-Strike: Vitality vs MOUZ (BO3)", "outcomes": `["Vitality","MOUZ"]`,
				"outcomePrices": `["0.5","0.5"]`, "volume24hr": 1000, "active": true, "closed": false, "acceptingOrders": true, "gameStartTime": start}},
		}})
	}))
	defer pm.Close()
	s := startServerS(t, 39530, nil)
	var out strings.Builder
	s.Exec("table set sportsbook api "+pm.URL, &out)
	a := newTestApp(t, s.Addr().String(), "punter")
	b := newTestApp(t, s.Addr().String(), "railbird")
	apps := []*App{a, b}
	pump(t, apps, nil, func() bool { return a.lobbyKnown && b.lobbyKnown && len(a.lobby.Tables) == 6 }, "lobby")
	a.joinTable(tableOf(a, proto.GameSports))
	b.joinTable(tableOf(b, proto.GameSports))
	pump(t, apps, nil, func() bool {
		return a.screen == scrSports && a.sp != nil && len(a.sp.Markets) == 1 && b.sp != nil && len(b.sp.Markets) == 1
	}, "the board")
	before := a.sp.Balance
	a.sbSel, a.sbStake = sbSelection{market: "m1", outcome: 0}, 100
	pump(t, apps, map[*App]Input{a: {Keys: []Key{KeyEnter}}}, func() bool { return true }, "")
	pump(t, apps, nil, func() bool {
		return a.sp != nil && len(a.sp.Mine) == 1 && b.sp != nil && len(b.sp.Others) == 1
	}, "bet placed and seen")
	bet := b.sp.Others[0]
	if bet.User != "punter" || bet.Pick != "Vitality" || bet.Stake != 100 || bet.Payout != 200 {
		t.Fatalf("the railbird sees %+v", bet)
	}
	if a.sp.Balance != before-100 || b.sp.Markets[0].Outcomes[0].Stake != 100 {
		t.Fatalf("balance %d (was %d), riding %+v", a.sp.Balance, before, b.sp.Markets[0].Outcomes[0])
	}
	_ = server.Version
}
