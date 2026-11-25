// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Polymarket's public market data (the Gamma API) needs no account or key.
// The Sportsbook reads two things from it: the open esports events with
// their markets and prices, and, for markets someone has bet on, whether
// they've been decided. A decided market's winning outcome is priced 1 and
// the others 0; one Polymarket splits 50-50 (a cancelled match) has both at
// 0.5, and bets on it are refunded.

const polymarketAPI = "https://gamma-api.polymarket.com"

// flexString reads a JSON string or number as a string (ids come as both).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// flexFloat reads a JSON number or a number in a string.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	var v float64
	if json.Unmarshal(b, &v) == nil {
		*f = flexFloat(v)
		return nil
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		v, _ = strconv.ParseFloat(s, 64)
		*f = flexFloat(v)
	}
	return nil
}

type pmTag struct {
	Label string `json:"label"`
	Slug  string `json:"slug"`
}

type pmMarket struct {
	ID               flexString `json:"id"`
	Question         string     `json:"question"`
	GroupItemTitle   string     `json:"groupItemTitle"`
	Outcomes         string     `json:"outcomes"`      // a JSON list, in a string
	OutcomePrices    string     `json:"outcomePrices"` // likewise
	Volume24hr       flexFloat  `json:"volume24hr"`
	Active           bool       `json:"active"`
	Closed           bool       `json:"closed"`
	AcceptingOrders  *bool      `json:"acceptingOrders"`
	GameStartTime    string     `json:"gameStartTime"`
	EndDate          string     `json:"endDate"`
	SportsMarketType string     `json:"sportsMarketType"`
	Slug             string     `json:"slug"`
}

type pmEvent struct {
	ID         flexString `json:"id"`
	Title      string     `json:"title"`
	Slug       string     `json:"slug"`
	Volume24hr flexFloat  `json:"volume24hr"`
	Tags       []pmTag    `json:"tags"`
	Markets    []pmMarket `json:"markets"`
}

// sbMarket is a market the Sportsbook offers.
type sbMarket struct {
	ID       string
	Game     string // "cs", "lol", "dota", "valorant"
	Event    string // "Vitality vs MOUZ (BO3)"
	Label    string // "Match winner", "Map 1 winner"
	Start    time.Time
	Outcomes []string
	Prices   []float64
	Volume   float64
	Open     bool // taking orders on Polymarket
	Slug     string
}

func listOf(s string) []string {
	var out []string
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil
	}
	return out
}

func pricesOf(s string) []float64 {
	var out []float64
	for _, p := range listOf(s) {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// parseTimeLoose reads the few ways Polymarket writes times.
func parseTimeLoose(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05-07", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// gameOf says which game an esports event is, from its tags or title.
func gameOf(ev pmEvent) string {
	check := func(s string) string {
		s = strings.ToLower(s)
		switch {
		case strings.Contains(s, "counter-strike"), strings.Contains(s, "counter strike"), strings.Contains(s, "cs2"),
			strings.Contains(s, "csgo"), strings.Contains(s, "cs:go"), strings.HasPrefix(s, "cs:"):
			return "cs"
		case strings.Contains(s, "league-of-legends"), strings.Contains(s, "league of legends"), s == "lol", strings.HasPrefix(s, "lol:"), strings.HasPrefix(s, "lol "):
			return "lol"
		case strings.Contains(s, "dota"):
			return "dota"
		case strings.Contains(s, "valorant"):
			return "valorant"
		}
		return ""
	}
	for _, tg := range ev.Tags {
		if g := check(tg.Slug); g != "" {
			return g
		}
		if g := check(tg.Label); g != "" {
			return g
		}
	}
	return check(ev.Title)
}

// tidyTitle drops the game's name from the front of an event title:
// "Counter-Strike: Vitality vs MOUZ (BO3)" becomes "Vitality vs MOUZ (BO3)".
func tidyTitle(t string) string {
	for _, p := range []string{"Counter-Strike 2:", "Counter-Strike:", "CS2:", "CS:", "League of Legends:", "LoL:", "Dota 2:", "Valorant:"} {
		if len(t) > len(p) && strings.EqualFold(t[:len(p)], p) {
			return strings.TrimSpace(t[len(p):])
		}
	}
	return strings.TrimSpace(t)
}

// marketLabel says what a market within a match is about.
func marketLabel(m pmMarket, ev pmEvent) string {
	switch {
	case m.GroupItemTitle != "":
		return m.GroupItemTitle
	case strings.EqualFold(m.SportsMarketType, "moneyline") || m.Question == ev.Title:
		return "Match winner"
	}
	q := tidyTitle(m.Question)
	if q == tidyTitle(ev.Title) {
		return "Match winner"
	}
	return q
}

func pmGet(ctx context.Context, client *http.Client, u string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TheHouseAlwaysWins/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("polymarket answered %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(into)
}

// fetchEsports reads the open esports markets for the chosen games, the
// busiest first.
func fetchEsports(ctx context.Context, client *http.Client, base, tag string, games map[string]bool) ([]sbMarket, error) {
	var out []sbMarket
	for offset := 0; offset < 600; offset += 200 {
		q := url.Values{}
		q.Set("tag_slug", tag)
		q.Set("closed", "false")
		q.Set("active", "true")
		q.Set("limit", "200")
		q.Set("offset", strconv.Itoa(offset))
		q.Set("order", "volume24hr")
		q.Set("ascending", "false")
		var evs []pmEvent
		if err := pmGet(ctx, client, strings.TrimRight(base, "/")+"/events?"+q.Encode(), &evs); err != nil {
			return nil, err
		}
		for _, ev := range evs {
			g := gameOf(ev)
			if !games[g] {
				continue
			}
			for _, m := range ev.Markets {
				outs, prices := listOf(m.Outcomes), pricesOf(m.OutcomePrices)
				if len(outs) != 2 || len(prices) != 2 || m.Closed { // two sides: team against team
					continue
				}
				start, ok := parseTimeLoose(m.GameStartTime)
				if !ok {
					// esports markets end six hours after the scheduled start
					if end, ok2 := parseTimeLoose(m.EndDate); ok2 {
						start = end.Add(-6 * time.Hour)
					}
				}
				vol := float64(m.Volume24hr)
				if vol == 0 {
					vol = float64(ev.Volume24hr) / float64(max(1, len(ev.Markets)))
				}
				open := m.Active && (m.AcceptingOrders == nil || *m.AcceptingOrders)
				out = append(out, sbMarket{ID: string(m.ID), Game: g, Event: tidyTitle(ev.Title), Label: marketLabel(m, ev),
					Start: start, Outcomes: outs, Prices: prices, Volume: vol, Open: open, Slug: ev.Slug})
			}
		}
		if len(evs) < 200 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Volume > out[j].Volume })
	return out, nil
}

// pmResult is what a market has come to.
type pmResult struct {
	Decided bool
	Winner  int  // the winning outcome, when decided
	Refund  bool // decided with no winner (a 50-50 split)
}

// fetchResult checks whether a market has been decided.
func fetchResult(ctx context.Context, client *http.Client, base, id string) (pmResult, error) {
	var m pmMarket
	if err := pmGet(ctx, client, strings.TrimRight(base, "/")+"/markets/"+url.PathEscape(id), &m); err != nil {
		return pmResult{}, err
	}
	return resultOf(m), nil
}

func resultOf(m pmMarket) pmResult {
	prices := pricesOf(m.OutcomePrices)
	if !m.Closed || len(prices) < 2 {
		return pmResult{}
	}
	for i, p := range prices {
		if p >= 0.99 {
			return pmResult{Decided: true, Winner: i}
		}
	}
	split := true
	for _, p := range prices {
		if p < 0.4 || p > 0.6 {
			split = false
		}
	}
	if split {
		return pmResult{Decided: true, Refund: true}
	}
	return pmResult{} // closed, but the answer isn't in yet
}
