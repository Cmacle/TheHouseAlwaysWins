// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Sportsbook: chips on real esports matches (Counter-Strike and League
// of Legends by default) at Polymarket's odds. It shows the busiest markets
// for the day. A bet takes the stake at once and locks in the odds: it pays
// stake / price if the pick wins, and is settled when Polymarket decides the
// market, whether or not the player is online. Everyone at the table sees
// everyone's bets. Bets are kept in sports-bets.json (and in backups), so
// they survive a restart.

type sportsCfg struct {
	Markets        int     `json:"markets"`         // how many markets to show (the busiest)
	HoursAhead     int     `json:"hours_ahead"`     // matches starting within this many hours
	MinBet         int64   `json:"min_bet"`         // smallest bet
	MaxBet         int64   `json:"max_bet"`         // biggest bet
	MaxOpen        int     `json:"max_open"`        // open bets one player can have
	LiveBetting    bool    `json:"live_betting"`    // take bets on matches already under way
	MinPrice       float64 `json:"min_price"`       // no bets on outcomes priced below this (0.04 = at most 25x)
	Games          string  `json:"games"`           // which games: cs, lol, dota, valorant
	RefreshSeconds int     `json:"refresh_seconds"` // how often to fetch the odds
	API            string  `json:"api"`             // Polymarket's market data address
	Tag            string  `json:"tag"`             // the Polymarket tag to read
}

func defaultSports() sportsCfg {
	return sportsCfg{Markets: 30, HoursAhead: 36, MinBet: 10, MaxBet: 1000, MaxOpen: 20, MinPrice: 0.04,
		Games: "cs,lol", RefreshSeconds: 60, API: polymarketAPI, Tag: "esports"}
}

func (c *sportsCfg) validate() {
	d := defaultSports()
	c.Markets = min(max(c.Markets, 5), 100)
	if c.HoursAhead < 1 {
		c.HoursAhead = d.HoursAhead
	}
	c.MinBet = max(c.MinBet, 1)
	if c.MaxBet < c.MinBet {
		c.MaxBet = max(d.MaxBet, c.MinBet)
	}
	if c.MaxOpen < 1 {
		c.MaxOpen = d.MaxOpen
	}
	if c.MinPrice <= 0 || c.MinPrice >= 0.5 {
		c.MinPrice = d.MinPrice
	}
	if strings.TrimSpace(c.Games) == "" {
		c.Games = d.Games
	}
	c.RefreshSeconds = max(c.RefreshSeconds, 15)
	if c.API == "" {
		c.API = d.API
	}
	if c.Tag == "" {
		c.Tag = d.Tag
	}
}

func (c sportsCfg) games() map[string]bool {
	out := map[string]bool{}
	for _, g := range strings.FieldsFunc(strings.ToLower(c.Games), func(r rune) bool { return r == ',' || r == ' ' }) {
		switch g {
		case "cs2", "csgo", "counter-strike", "counterstrike":
			g = "cs"
		case "league", "leagueoflegends", "league-of-legends":
			g = "lol"
		case "dota2":
			g = "dota"
		case "val":
			g = "valorant"
		}
		out[g] = true
	}
	return out
}

func sportsConfig(d TableDef) (sportsCfg, error) {
	c := defaultSports()
	if len(d.Settings) > 0 {
		b, _ := json.Marshal(d.Settings)
		if err := json.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("table %s: %v", d.ID, err)
		}
	}
	c.validate()
	return c, nil
}

// sbBet is a bet as kept on disk.
type sbBet struct {
	ID      int       `json:"id"`
	Table   string    `json:"table"`
	User    string    `json:"user"`
	Market  string    `json:"market"`
	Game    string    `json:"game"`
	Event   string    `json:"event"`
	Label   string    `json:"label"`
	Pick    string    `json:"pick"`
	Outcome int       `json:"outcome"`
	Price   float64   `json:"price"`
	Stake   int64     `json:"stake"`
	Payout  int64     `json:"payout"`
	Start   time.Time `json:"start"`
	Status  string    `json:"status"` // open, won, lost, refunded
	Placed  time.Time `json:"placed"`
	Settled time.Time `json:"settled,omitempty"`
}

func (b *sbBet) view() proto.SportsBet {
	v := proto.SportsBet{ID: b.ID, User: b.User, Market: b.Market, Game: b.Game, Event: b.Event, Label: b.Label,
		Pick: b.Pick, Price: b.Price, Stake: b.Stake, Payout: b.Payout, Status: b.Status, Placed: b.Placed.Unix()}
	if !b.Settled.IsZero() {
		v.Settled = b.Settled.Unix()
	}
	return v
}

type sbPlayer struct {
	user   string
	conn   *conn
	chatAt []time.Time
}

// Sports is a Sportsbook table.
type Sports struct {
	id, name string
	cfg      *Config
	sc       sportsCfg
	store    *Store
	notify   Notifier
	tell     func(user, text string) // a message to a player wherever they are
	exec     func(line string, w io.Writer)
	log      *log.Logger
	events   chan event
	conns    map[*conn]*sbPlayer
	info     atomic.Pointer[proto.TableInfo]
	client   *http.Client
	path     string // sports-bets.json

	markets  []sbMarket
	updated  time.Time
	fetchErr string
	fetching bool
	gen      int // bumped when the settings change, so a fetch begun before is ignored
	checking bool
	bets     []*sbBet
	nextID   int
	feed     []string
	now      func() time.Time // the clock, for tests
	refreshC chan struct{}    // tests: fetch now
}

func newSports(id, name string, cfg *Config, sc sportsCfg, store *Store, n Notifier, lg *log.Logger) *Sports {
	sc.validate()
	if n == nil {
		n = nopNotifier{}
	}
	t := &Sports{id: id, name: name, cfg: cfg, sc: sc, store: store, notify: n, log: lg,
		events: make(chan event, 1024), conns: map[*conn]*sbPlayer{},
		client:   &http.Client{Timeout: 20 * time.Second},
		path:     filepath.Join(cfg.DataDir, "sports-bets.json"),
		now:      time.Now,
		refreshC: make(chan struct{}, 1),
		tell:     func(string, string) {},
	}
	t.load()
	t.updateInfo()
	return t
}

// load reads the saved bets (all Sportsbook tables share the file; each
// keeps its own).
func (t *Sports) load() {
	var all []*sbBet
	b, err := os.ReadFile(t.path)
	if err == nil {
		if err := json.Unmarshal(b, &all); err != nil {
			t.log.Printf("[%s] WARNING: can't read %s: %v", t.id, t.path, err)
		}
	}
	for _, x := range all {
		if x.Table == t.id {
			t.bets = append(t.bets, x)
		}
		t.nextID = max(t.nextID, x.ID)
	}
}

// save writes this table's bets, keeping other tables' bets in the file.
func (t *Sports) save() {
	var all []*sbBet
	if b, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	keep := all[:0]
	for _, x := range all {
		if x.Table != t.id {
			keep = append(keep, x)
		}
	}
	keep = append(keep, t.bets...)
	sort.Slice(keep, func(i, j int) bool { return keep[i].ID < keep[j].ID })
	b, _ := json.MarshalIndent(keep, "", "  ")
	if err := writeFileSafe(t.path, b, true); err != nil {
		t.log.Printf("[%s] WARNING: couldn't save bets: %v", t.id, err)
	}
}

func (t *Sports) run(stop <-chan struct{}) {
	t.refresh()
	t.checkResults()
	tick := time.NewTicker(time.Duration(t.sc.RefreshSeconds) * time.Second)
	settle := time.NewTicker(3 * time.Minute)
	defer tick.Stop()
	defer settle.Stop()
	for {
		select {
		case <-stop:
			return
		case ev := <-t.events:
			guard(t.log.Printf, "the Sportsbook", nil, func() { t.handle(ev) })
		case <-tick.C:
			t.refresh()
		case <-settle.C:
			t.checkResults()
		case <-t.refreshC:
			t.refresh()
			t.checkResults()
		}
	}
}

// Do runs fn on the table goroutine.
func (t *Sports) Do(fn func()) { t.events <- event{kind: evFunc, fn: fn} }

// DoWait runs fn on the table goroutine and waits.
func (t *Sports) DoWait(fn func()) {
	done := make(chan struct{})
	t.events <- event{kind: evFunc, fn: func() { fn(); close(done) }}
	<-done
}

// refresh fetches the odds in the background.
func (t *Sports) refresh() {
	if t.fetching {
		return
	}
	t.fetching = true
	sc, gen := t.sc, t.gen
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		ms, err := fetchEsports(ctx, t.client, sc.API, sc.Tag, sc.games())
		t.Do(func() {
			if gen != t.gen {
				return // the settings changed while this was out
			}
			t.fetching = false
			if err != nil {
				if t.fetchErr == "" {
					t.log.Printf("[%s] couldn't fetch the odds: %v", t.id, err)
				}
				t.fetchErr = "Couldn't reach Polymarket for fresh odds. Retrying."
				t.broadcast()
				return
			}
			t.fetchErr = ""
			t.markets = t.pick(ms)
			t.updated = t.now()
			t.updateInfo()
			t.broadcast()
		})
	}()
}

// pick keeps the busiest markets for matches today: ones under way and ones
// starting within hours_ahead.
func (t *Sports) pick(ms []sbMarket) []sbMarket {
	now := t.now()
	var out []sbMarket
	for _, m := range ms {
		if !m.Start.IsZero() && (m.Start.Before(now.Add(-6*time.Hour)) || m.Start.After(now.Add(time.Duration(t.sc.HoursAhead)*time.Hour))) {
			continue
		}
		out = append(out, m)
		if len(out) >= t.sc.Markets {
			break
		}
	}
	return out
}

// checkResults asks Polymarket about every market with open bets.
func (t *Sports) checkResults() {
	if t.checking {
		return
	}
	ids := map[string]bool{}
	for _, b := range t.bets {
		if b.Status == "open" {
			ids[b.Market] = true
		}
	}
	if len(ids) == 0 {
		return
	}
	t.checking = true
	api := t.sc.API
	go func() {
		results := map[string]pmResult{}
		for id := range ids {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			r, err := fetchResult(ctx, t.client, api, id)
			cancel()
			if err == nil {
				results[id] = r
			}
		}
		t.Do(func() {
			t.checking = false
			t.settle(results)
		})
	}()
}

// settle pays out bets on decided markets, and refunds bets on markets
// still undecided three weeks after the match.
func (t *Sports) settle(results map[string]pmResult) {
	now := t.now()
	changed := false
	for _, b := range t.bets {
		if b.Status != "open" {
			continue
		}
		r, ok := results[b.Market]
		stale := !b.Start.IsZero() && now.Sub(b.Start) > 21*24*time.Hour || b.Start.IsZero() && now.Sub(b.Placed) > 21*24*time.Hour
		switch {
		case ok && r.Decided && r.Refund, !r.Decided && stale:
			t.finish(b, "refunded", b.Stake)
		case ok && r.Decided && r.Winner == b.Outcome:
			t.finish(b, "won", b.Payout)
		case ok && r.Decided:
			t.finish(b, "lost", 0)
		default:
			continue
		}
		changed = true
	}
	if changed {
		t.save()
		t.broadcast()
	}
}

// finish settles one bet.
func (t *Sports) finish(b *sbBet, status string, paid int64) {
	b.Status, b.Settled = status, t.now()
	t.store.SettleGame(b.User, GameResult{Game: proto.GameSports, Wagered: b.Stake, Payout: paid})
	net := paid - b.Stake
	t.store.RecordTable(b.User, t.id, net)
	t.store.RecordTableHouse(-net) // SettleGame already counted the house's share against the player
	var msg string
	switch status {
	case "won":
		msg = fmt.Sprintf("%s's bet on %s (%s) won: +%s chips!", b.User, b.Pick, b.Event, commas(net))
		t.tell(b.User, fmt.Sprintf("Your bet on %s won! +%s chips (%s)", b.Pick, commas(net), b.Event))
		if net >= 1000 {
			t.notify.BigWin(b.User, net)
		}
	case "lost":
		msg = fmt.Sprintf("%s's bet on %s (%s) lost %s.", b.User, b.Pick, b.Event, commas(b.Stake))
		t.tell(b.User, fmt.Sprintf("Your bet on %s lost (%s)", b.Pick, b.Event))
	default:
		msg = fmt.Sprintf("%s's bet on %s (%s) was refunded.", b.User, b.Pick, b.Event)
		t.tell(b.User, fmt.Sprintf("Your bet on %s was refunded: the market was called off (%s)", b.Pick, b.Event))
	}
	t.addFeed(msg)
	t.log.Printf("[%s] bet %d settled %s: %s", t.id, b.ID, status, msg)
	for c, p := range t.conns {
		if strings.EqualFold(p.user, b.User) {
			if prof, ok := t.store.Profile(b.User); ok {
				c.send(proto.SProfile, prof)
			}
		}
	}
	t.notify.LeaderboardChanged()
}

func (t *Sports) addFeed(s string) {
	t.feed = append(t.feed, s)
	if len(t.feed) > 6 {
		t.feed = t.feed[len(t.feed)-6:]
	}
}

func (t *Sports) handle(ev event) {
	switch ev.kind {
	case evJoin:
		t.conns[ev.c] = &sbPlayer{user: ev.user, conn: ev.c}
		t.updateInfo()
		t.broadcast()
	case evLeave:
		delete(t.conns, ev.c)
		t.updateInfo()
		t.broadcast()
	case evMsg:
		p := t.conns[ev.c]
		if p == nil {
			return
		}
		switch ev.msg.Type {
		case proto.CSports:
			var a proto.SportsAct
			if json.Unmarshal(ev.msg.Data, &a) != nil {
				return
			}
			switch a.Action {
			case "bet":
				if err := t.bet(p, a); err != nil {
					p.conn.event(proto.Event{Kind: "info", Seat: -1, Text: err.Error()})
				}
				t.broadcast()
			case "refresh":
				t.send(p)
			}
		case proto.CChat:
			var ch proto.Chat
			if json.Unmarshal(ev.msg.Data, &ch) == nil {
				t.onChat(p, ch.Text)
			}
		}
	case evFunc:
		ev.fn()
	}
}

func (t *Sports) onChat(p *sbPlayer, text string) {
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if text == "" {
		return
	}
	if strings.HasPrefix(text, "/") {
		if !t.store.IsAdmin(p.user) {
			p.conn.send(proto.SError, proto.ErrorMsg{Message: "Only admins can use / commands."})
			return
		}
		runAdminCommand(t.log, t.exec, p.conn, p.user, text)
		return
	}
	if !t.cfg.TableChat {
		p.conn.send(proto.SError, proto.ErrorMsg{Message: "Chat is turned off on this server."})
		return
	}
	now := time.Now()
	recent := p.chatAt[:0]
	for _, at := range p.chatAt {
		if now.Sub(at) < 10*time.Second {
			recent = append(recent, at)
		}
	}
	if p.chatAt = recent; len(recent) >= 4 {
		p.conn.send(proto.SError, proto.ErrorMsg{Message: "Slow down a little."})
		return
	}
	p.chatAt = append(p.chatAt, now)
	if len(text) > 80 {
		text = text[:80]
	}
	msg := proto.Chat{From: p.user, Seat: -1, Text: text, Admin: t.store.IsAdmin(p.user), At: now.UnixMilli()}
	for c := range t.conns {
		c.send(proto.SChat, msg)
	}
}

func (t *Sports) market(id string) *sbMarket {
	for i := range t.markets {
		if t.markets[i].ID == id {
			return &t.markets[i]
		}
	}
	return nil
}

// takingBets says whether a market takes bets now, and why not.
func (t *Sports) takingBets(m *sbMarket) (bool, string) {
	switch {
	case !m.Open:
		return false, "Polymarket isn't taking bets on that right now."
	case !t.sc.LiveBetting && !m.Start.IsZero() && !t.now().Before(m.Start):
		return false, "That match has started: bets are closed."
	case t.fetchErr != "" && t.now().Sub(t.updated) > 5*time.Minute:
		return false, "The odds are out of date (Polymarket can't be reached), so bets are paused."
	}
	return true, ""
}

func (t *Sports) bet(p *sbPlayer, a proto.SportsAct) error {
	m := t.market(a.Market)
	if m == nil {
		return errors.New("That market isn't on the board any more.")
	}
	if ok, why := t.takingBets(m); !ok {
		return errors.New(why)
	}
	if a.Outcome < 0 || a.Outcome >= len(m.Outcomes) {
		return errors.New("Pick a side first.")
	}
	price := m.Prices[a.Outcome]
	if price < t.sc.MinPrice || price >= 0.995 {
		return fmt.Errorf("No bets on %s at those odds.", m.Outcomes[a.Outcome])
	}
	if math.Abs(price-a.Price) > 0.02 {
		return fmt.Errorf("The odds on %s just moved to %.2fx. Check them and bet again.", m.Outcomes[a.Outcome], 1/price)
	}
	if a.Stake < t.sc.MinBet || a.Stake > t.sc.MaxBet {
		return fmt.Errorf("Bets here are %s to %s chips.", commas(t.sc.MinBet), commas(t.sc.MaxBet))
	}
	open := 0
	for _, b := range t.bets {
		if b.Status == "open" && strings.EqualFold(b.User, p.user) {
			open++
		}
	}
	if open >= t.sc.MaxOpen {
		return fmt.Errorf("You have %d bets waiting already; that's the most at once.", open)
	}
	if !t.store.Debit(p.user, a.Stake) {
		return errors.New("You don't have enough chips for that.")
	}
	t.nextID++
	b := &sbBet{ID: t.nextID, Table: t.id, User: p.user, Market: m.ID, Game: m.Game, Event: m.Event, Label: m.Label,
		Pick: m.Outcomes[a.Outcome], Outcome: a.Outcome, Price: price, Stake: a.Stake,
		Payout: int64(math.Floor(float64(a.Stake) / price)), Start: m.Start, Status: "open", Placed: t.now()}
	t.bets = append(t.bets, b)
	t.trim()
	t.save()
	t.addFeed(fmt.Sprintf("%s bet %s on %s at %.2fx (%s).", p.user, commas(b.Stake), b.Pick, 1/price, b.Event))
	t.log.Printf("[%s] %s bet %d on %s / %s / %s at %.3f, pays %d", t.id, p.user, b.Stake, b.Event, b.Label, b.Pick, price, b.Payout)
	if prof, ok := t.store.Profile(p.user); ok {
		p.conn.send(proto.SProfile, prof)
	}
	p.conn.event(proto.Event{Kind: "info", Seat: -1, Text: fmt.Sprintf("Bet placed: %s on %s. Pays %s if it wins.", commas(b.Stake), b.Pick, commas(b.Payout))})
	return nil
}

// trim forgets settled bets beyond the latest 300.
func (t *Sports) trim() {
	settled := 0
	for i := len(t.bets) - 1; i >= 0; i-- {
		if t.bets[i].Status != "open" {
			settled++
			if settled > 300 {
				t.bets = append(t.bets[:i], t.bets[i+1:]...)
			}
		}
	}
}

// betViews is bets for the screen: open ones first (soonest match first),
// then the latest settled, at most n.
func betViews(bets []*sbBet, keep func(*sbBet) bool, n int) []proto.SportsBet {
	var open, done []*sbBet
	for _, b := range bets {
		if !keep(b) {
			continue
		}
		if b.Status == "open" {
			open = append(open, b)
		} else {
			done = append(done, b)
		}
	}
	sort.SliceStable(open, func(i, j int) bool { return open[i].Start.Before(open[j].Start) })
	sort.SliceStable(done, func(i, j int) bool { return done[i].Settled.After(done[j].Settled) })
	out := []proto.SportsBet{}
	for _, b := range append(open, done...) {
		if len(out) >= n {
			break
		}
		out = append(out, b.view())
	}
	return out
}

func (t *Sports) view(p *sbPlayer) proto.SportsState {
	st := proto.SportsState{Table: t.id, Name: t.name, Error: t.fetchErr, Balance: t.store.Chips(p.user),
		MinBet: t.sc.MinBet, MaxBet: t.sc.MaxBet, Live: t.sc.LiveBetting, Feed: t.feed, Markets: []proto.SportsMarket{},
		ServerTime: time.Now().UnixMilli(),
		Settings: proto.Settings{ServerName: t.cfg.ServerName, RefillAmount: t.cfg.RefillAmount,
			DealerTrashTalk: t.cfg.DealerTrashTalk, DealerVulgar: t.cfg.DealerTrashTalk && t.cfg.DealerVulgar,
			ChipGifts: t.cfg.ChipGifts && t.cfg.RefillAmount > 0, TableChat: t.cfg.TableChat}}
	if !t.updated.IsZero() {
		st.Updated = t.updated.Unix()
	}
	house := t.store.House()
	st.HouseNet, st.HouseTier = house.Net, house.Tier
	// chips already riding on each outcome
	riding := map[string]map[int][2]int64{}
	for _, b := range t.bets {
		if b.Status != "open" {
			continue
		}
		if riding[b.Market] == nil {
			riding[b.Market] = map[int][2]int64{}
		}
		r := riding[b.Market][b.Outcome]
		riding[b.Market][b.Outcome] = [2]int64{r[0] + 1, r[1] + b.Stake}
	}
	for i := range t.markets {
		m := &t.markets[i]
		ok, _ := t.takingBets(m)
		v := proto.SportsMarket{ID: m.ID, Game: m.Game, Event: m.Event, Label: m.Label, Open: ok, Volume: m.Volume,
			Live: !m.Start.IsZero() && !t.now().Before(m.Start)}
		if !m.Start.IsZero() {
			v.Start = m.Start.Unix()
		}
		for k, o := range m.Outcomes {
			r := riding[m.ID][k]
			v.Outcomes = append(v.Outcomes, proto.SportsOutcome{Name: o, Price: m.Prices[k], Bets: int(r[0]), Stake: r[1]})
		}
		st.Markets = append(st.Markets, v)
	}
	st.Mine = betViews(t.bets, func(b *sbBet) bool { return strings.EqualFold(b.User, p.user) }, 40)
	st.Others = betViews(t.bets, func(*sbBet) bool { return true }, 60)
	seen := map[string]bool{}
	for _, o := range t.conns {
		if !seen[strings.ToLower(o.user)] {
			seen[strings.ToLower(o.user)] = true
			st.Players = append(st.Players, o.user)
		}
	}
	sort.Strings(st.Players)
	return st
}

func (t *Sports) send(p *sbPlayer) { p.conn.send(proto.SSports, t.view(p)) }

func (t *Sports) broadcast() {
	for _, p := range t.conns {
		t.send(p)
	}
}

func (t *Sports) updateInfo() {
	open := 0
	for _, b := range t.bets {
		if b.Status == "open" {
			open++
		}
	}
	in := proto.TableInfo{ID: t.id, Name: t.name, Game: proto.GameSports, Seats: 99,
		Stakes: fmt.Sprintf("Esports at Polymarket odds, %d markets, %d bets riding", len(t.markets), open)}
	for _, p := range t.conns {
		in.Players++
		in.Names = append(in.Names, p.user)
	}
	t.info.Store(&in)
}

func (t *Sports) tinfo() proto.TableInfo {
	if in := t.info.Load(); in != nil {
		return *in
	}
	return proto.TableInfo{ID: t.id, Name: t.name, Game: proto.GameSports}
}

// openStake is the chips a player has riding on open bets.
func (t *Sports) openStake(user string) int64 {
	var n int64
	for _, b := range t.bets {
		if b.Status == "open" && strings.EqualFold(b.User, user) {
			n += b.Stake
		}
	}
	return n
}

// ---------------------------------------------------------------- lobby

type sportsTable struct{ *Sports }

func (s sportsTable) info() proto.TableInfo { return s.tinfo() }
func (s sportsTable) join(c *conn, user string, _ int64) {
	s.events <- event{kind: evJoin, c: c, user: user}
}
func (s sportsTable) leave(c *conn)            { s.events <- event{kind: evLeave, c: c} }
func (s sportsTable) msg(c *conn, m proto.Msg) { s.events <- event{kind: evMsg, c: c, msg: m} }
func (s sportsTable) broadcastAll()            { s.Do(s.broadcast) }
func (s sportsTable) close(reason string) {
	s.DoWait(func() {
		for c := range s.conns {
			c.send(proto.SLeft, proto.ErrorMsg{Message: reason})
			c.event(proto.Event{Kind: "info", Text: reason, Seat: -1})
		}
		s.conns = map[*conn]*sbPlayer{}
	})
}
func (s sportsTable) reconfigure(global *Config, d TableDef) {
	sc, err := sportsConfig(d)
	if err != nil {
		return
	}
	s.Do(func() {
		s.cfg, s.sc, s.name = global, sc, d.Name
		s.gen++
		s.fetching = false
		s.updateInfo()
		s.refresh()
	})
}

// Open bets aren't "in a hand": they can take days, and a player can leave
// the table (or the server) while they ride.
func (s sportsTable) inHand(string) bool    { return false }
func (s sportsTable) inHandUsers() []string { return nil }
func (s sportsTable) players() []string {
	var out []string
	s.DoWait(func() {
		for _, p := range s.conns {
			out = append(out, fmt.Sprintf("%-16s %d chips, %d riding", p.user, s.store.Chips(p.user), s.openStake(p.user)))
		}
		open, riding := 0, int64(0)
		for _, b := range s.bets {
			if b.Status == "open" {
				open++
				riding += b.Stake
			}
		}
		age := "never"
		if !s.updated.IsZero() {
			age = s.now().Sub(s.updated).Round(time.Second).String() + " ago"
		}
		out = append(out, fmt.Sprintf("%d markets (odds fetched %s), %d open bets, %d chips riding", len(s.markets), age, open, riding))
		if s.fetchErr != "" {
			out = append(out, "  "+s.fetchErr)
		}
	})
	return out
}
func (s sportsTable) summary() string {
	in := s.tinfo()
	return fmt.Sprintf("%s: %d watching", s.name, in.Players)
}
