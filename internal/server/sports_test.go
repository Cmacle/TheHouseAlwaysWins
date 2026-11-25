// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// fakePolymarket serves esports events and market results like Polymarket's
// Gamma API.
type fakePolymarket struct {
	mu      sync.Mutex
	prices  map[string][2]string // market id -> outcome prices
	closed  map[string]bool
	start   time.Time
	srv     *httptest.Server
	queries []string
}

func newFakePolymarket(t *testing.T) *fakePolymarket {
	f := &fakePolymarket{prices: map[string][2]string{"m1": {"0.62", "0.38"}, "m2": {"0.5", "0.5"}, "m3": {"0.2", "0.8"}, "m9": {"0.5", "0.5"}},
		closed: map[string]bool{}, start: time.Now().Add(3 * time.Hour)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePolymarket) market(id, q, group string, outs [2]string, vol float64) map[string]any {
	p := f.prices[id]
	op, _ := json.Marshal([]string{p[0], p[1]})
	oo, _ := json.Marshal([]string{outs[0], outs[1]})
	return map[string]any{"id": id, "question": q, "groupItemTitle": group, "outcomes": string(oo), "outcomePrices": string(op),
		"volume24hr": vol, "active": true, "closed": f.closed[id], "acceptingOrders": !f.closed[id],
		"gameStartTime": f.start.UTC().Format("2006-01-02 15:04:05+00"), "sportsMarketType": "moneyline"}
}

func (f *fakePolymarket) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, r.URL.String())
	switch {
	case r.URL.Path == "/events":
		evs := []map[string]any{
			{"id": 1, "title": "Counter-Strike: Vitality vs MOUZ (BO3)", "tags": []map[string]string{{"label": "Esports", "slug": "esports"}, {"label": "Counter-Strike 2", "slug": "counter-strike-2"}},
				"markets": []any{f.market("m1", "Counter-Strike: Vitality vs MOUZ (BO3)", "", [2]string{"Vitality", "MOUZ"}, 50000),
					f.market("m2", "Map 1 Winner", "Map 1 winner", [2]string{"Vitality", "MOUZ"}, 9000)}},
			{"id": 2, "title": "LoL: T1 vs Gen.G (BO5)", "tags": []map[string]string{{"label": "Esports", "slug": "esports"}},
				"markets": []any{f.market("m3", "LoL: T1 vs Gen.G (BO5)", "", [2]string{"T1", "Gen.G"}, 80000)}},
			{"id": 3, "title": "Dota 2: Spirit vs Liquid", "tags": []map[string]string{{"label": "Dota 2", "slug": "dota-2"}},
				"markets": []any{f.market("m9", "Dota 2: Spirit vs Liquid", "", [2]string{"Spirit", "Liquid"}, 99999)}},
		}
		json.NewEncoder(w).Encode(evs)
	case strings.HasPrefix(r.URL.Path, "/markets/"):
		id := strings.TrimPrefix(r.URL.Path, "/markets/")
		p, ok := f.prices[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(f.market(id, "", "", [2]string{"A", "B"}, 0))
		_ = p
	default:
		http.NotFound(w, r)
	}
}

func (f *fakePolymarket) set(id string, a, b string, closed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prices[id] = [2]string{a, b}
	f.closed[id] = closed
}

// state is the latest Sportsbook state a connection was sent.
func sportsState(t *testing.T, c *conn) (proto.SportsState, []string) {
	t.Helper()
	var st proto.SportsState
	var notes []string
	for len(c.out) > 0 {
		var m proto.Msg
		if json.Unmarshal(<-c.out, &m) != nil {
			continue
		}
		switch m.Type {
		case proto.SSports:
			json.Unmarshal(m.Data, &st)
		case proto.SEvent:
			var e proto.Event
			json.Unmarshal(m.Data, &e)
			notes = append(notes, e.Text)
		}
	}
	return st, notes
}

func sportsServer(t *testing.T, f *fakePolymarket) (*Server, sportsTable) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	s, err := New(cfg, dir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	rec := s.Lobby.find("sportsbook")
	if rec == nil {
		t.Fatal("no Sportsbook table by default")
	}
	sp := rec.t.(sportsTable)
	sp.Sports.sc.API = f.srv.URL
	s.Lobby.start(stop)
	waitFor(t, sp, func() bool { return len(sp.markets) > 0 })
	return s, sp
}

func waitFor(t *testing.T, sp sportsTable, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		done := false
		sp.DoWait(func() { done = ok() })
		if done {
			return
		}
		select {
		case sp.refreshC <- struct{}{}:
		default:
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("timed out")
}

// The board shows the busiest CS and League markets (not Dota, which isn't
// one of the table's games), labelled and with odds.
func TestSportsBoard(t *testing.T) {
	f := newFakePolymarket(t)
	_, sp := sportsServer(t, f)
	var ms []sbMarket
	sp.DoWait(func() { ms = sp.markets })
	if len(ms) != 3 {
		t.Fatalf("markets %+v", ms)
	}
	if ms[0].ID != "m3" || ms[0].Game != "lol" || ms[0].Event != "T1 vs Gen.G (BO5)" || ms[0].Label != "Match winner" {
		t.Errorf("busiest first: %+v", ms[0])
	}
	if ms[1].Game != "cs" || ms[2].Label != "Map 1 winner" || ms[1].Prices[0] != 0.62 {
		t.Errorf("cs markets: %+v %+v", ms[1], ms[2])
	}
	if ms[0].Start.IsZero() || ms[0].Start.Sub(f.start).Abs() > time.Second {
		t.Errorf("start %v, want %v", ms[0].Start, f.start)
	}
	f.mu.Lock()
	q := f.queries[0]
	f.mu.Unlock()
	if !strings.Contains(q, "tag_slug=esports") || !strings.Contains(q, "closed=false") {
		t.Errorf("query %s", q)
	}
}

// Betting takes the stake and locks the odds; moved odds, a started match,
// too big a bet or too few chips are refused; everyone sees the bets; a
// win pays stake / price, a loss pays nothing, a 50-50 split refunds; and
// bets survive a restart.
func TestSportsBetting(t *testing.T) {
	f := newFakePolymarket(t)
	s, sp := sportsServer(t, f)
	conns := map[string]*conn{}
	for _, name := range []string{"punter", "watcher"} {
		if _, _, err := s.Store.Register(name, "pass1234", nil, 1000, ""); err != nil {
			t.Fatal(err)
		}
		c := testConn()
		c.user = name
		s.Lobby.join(c, "sportsbook", 0)
		conns[name] = c
	}
	c := conns["punter"]
	bet := func(market string, outcome int, stake int64, price float64) []string {
		s.Lobby.route(c, mkMsg(proto.CSports, proto.SportsAct{Action: "bet", Market: market, Outcome: outcome, Stake: stake, Price: price}))
		sp.DoWait(func() {})
		_, notes := sportsState(t, c)
		return notes
	}
	if n := bet("m1", 0, 100, 0.55); len(n) == 0 || !strings.Contains(n[len(n)-1], "moved") {
		t.Fatalf("moved odds: %v", n)
	}
	if n := bet("m1", 0, 5000, 0.62); !strings.Contains(strings.Join(n, " "), "Bets here are") {
		t.Fatalf("too big: %v", n)
	}
	bet("m1", 0, 100, 0.62) // Vitality at 1.61x: pays 161
	bet("m3", 0, 200, 0.2)  // T1 at 5x: pays 1000
	bet("m2", 1, 300, 0.5)  // MOUZ map 1: refunded later
	if p, _ := s.Store.Profile("punter"); p.Balance != 400 {
		t.Fatalf("after 600 in bets: %d", p.Balance)
	}
	if n := bet("m1", 1, 500, 0.38); !strings.Contains(strings.Join(n, " "), "enough chips") {
		t.Fatalf("broke: %v", n)
	}
	st, _ := sportsState(t, conns["watcher"])
	if len(st.Others) != 3 || st.Others[0].User != "punter" {
		t.Fatalf("the watcher should see the punter's bets: %+v", st.Others)
	}
	for _, m := range st.Markets {
		if m.ID == "m1" && (m.Outcomes[0].Bets != 1 || m.Outcomes[0].Stake != 100) {
			t.Errorf("chips riding on Vitality: %+v", m.Outcomes[0])
		}
	}

	// a started match takes no more bets
	sp.DoWait(func() { sp.now = func() time.Time { return f.start.Add(time.Minute) } })
	if n := bet("m1", 0, 50, 0.62); !strings.Contains(strings.Join(n, " "), "started") {
		t.Fatalf("started: %v", n)
	}

	// results come in
	f.set("m1", "1", "0", true)     // Vitality won
	f.set("m3", "0", "1", true)     // T1 lost
	f.set("m2", "0.5", "0.5", true) // map called off: refund
	waitFor(t, sp, func() bool {
		for _, b := range sp.bets {
			if b.Status == "open" {
				return false
			}
		}
		return true
	})
	p, _ := s.Store.Profile("punter")
	if want := int64(400 + 161 + 300); p.Balance != want {
		t.Fatalf("after results: %d chips, want %d", p.Balance, want)
	}
	var statuses []string
	sp.DoWait(func() {
		for _, b := range sp.bets {
			statuses = append(statuses, fmt.Sprintf("%s:%s", b.Market, b.Status))
		}
	})
	if strings.Join(statuses, " ") != "m1:won m3:lost m2:refunded" {
		t.Errorf("statuses %v", statuses)
	}
	if house := s.Store.House(); house.Net != 200-61 {
		t.Errorf("house net %d, want %d", house.Net, 200-61)
	}

	// bets are saved
	sp2 := newSports("sportsbook", "Sportsbook", s.cfg(), defaultSports(), s.Store, nil, log.New(io.Discard, "", 0))
	if len(sp2.bets) != 3 || sp2.nextID != 3 {
		t.Fatalf("reloaded %d bets, next id %d", len(sp2.bets), sp2.nextID)
	}
}

// A closed market without a clear answer yet isn't settled.
func TestSportsResultOf(t *testing.T) {
	for _, c := range []struct {
		closed bool
		prices string
		want   pmResult
	}{
		{false, `["1","0"]`, pmResult{}},
		{true, `["0.999","0.001"]`, pmResult{Decided: true, Winner: 0}},
		{true, `["0","1"]`, pmResult{Decided: true, Winner: 1}},
		{true, `["0.5","0.5"]`, pmResult{Decided: true, Refund: true}},
		{true, `["0.7","0.3"]`, pmResult{}},
	} {
		if got := resultOf(pmMarket{Closed: c.closed, OutcomePrices: c.prices}); got != c.want {
			t.Errorf("%v %s: %+v, want %+v", c.closed, c.prices, got, c.want)
		}
	}
	for title, want := range map[string]string{"Counter-Strike: A vs B": "cs", "LoL: A vs B": "lol", "League of Legends: A vs B": "lol",
		"Dota 2: A vs B": "dota", "Valorant: A vs B": "valorant", "Chess: A vs B": ""} {
		if g := gameOf(pmEvent{Title: title}); g != want {
			t.Errorf("%q: %q, want %q", title, g, want)
		}
	}
}
