// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The lobby: the server runs any number of tables at once, each in its own
// goroutine. After logging in a player sees the list and picks one. Admins
// open, change and close tables while the server runs; the list is saved in
// <data_dir>/tables.json.

// TableDef is one table in tables.json. Settings override the server's own
// (for blackjack) or Hold'em's defaults.
type TableDef struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Game     string         `json:"game"`
	Settings map[string]any `json:"settings,omitempty"`
}

// table is what every kind of table does for the lobby.
type table interface {
	info() proto.TableInfo
	join(c *conn, user string, buyIn int64)
	leave(c *conn)
	msg(c *conn, m proto.Msg)
	run(stop <-chan struct{})
	reconfigure(global *Config, def TableDef)
	close(reason string)
	broadcastAll()
	inHand(user string) bool
	inHandUsers() []string
	players() []string
	summary() string
}

type tableRec struct {
	def  TableDef
	t    table
	stop chan struct{}
}

// Lobby runs the tables.
type Lobby struct {
	s     *Server
	mu    sync.Mutex
	recs  []*tableRec
	at    map[*conn]*tableRec
	conns map[*conn]bool
	path  string
	stop  <-chan struct{}

	lastSent   string
	lastPeriod string
	wg         sync.WaitGroup
	started    bool
}

func newLobby(s *Server) *Lobby {
	return &Lobby{s: s, at: map[*conn]*tableRec{}, conns: map[*conn]bool{},
		path: filepath.Join(s.cfg().DataDir, "tables.json")}
}

// defaultTables is the list on a fresh server: blackjack and Hold'em.
func defaultTables(cfg *Config) []TableDef {
	name := "Blackjack"
	return []TableDef{
		{ID: "blackjack", Name: name, Game: proto.GameBlackjack},
		{ID: "holdem", Name: "Texas Hold'em", Game: proto.GameHoldem},
		{ID: "beggars", Name: "Beggars Table", Game: proto.GameBeggar},
	}
}

func (l *Lobby) load() []TableDef {
	var defs []TableDef
	if b, err := os.ReadFile(l.path); err == nil {
		if err := json.Unmarshal(b, &defs); err != nil {
			l.s.Log.Printf("WARNING: tables.json doesn't parse (%v); using the default tables", err)
			defs = nil
		}
	}
	if len(defs) == 0 {
		defs = defaultTables(l.s.cfg())
	}
	return defs
}

func (l *Lobby) save() {
	defs := make([]TableDef, 0, len(l.recs))
	for _, r := range l.recs {
		defs = append(defs, r.def)
	}
	b, _ := json.MarshalIndent(defs, "", "  ")
	if err := writeFileSafe(l.path, append(b, '\n'), true); err != nil {
		l.s.Log.Printf("couldn't save tables.json: %v", err)
	}
}

// build opens every table (not running yet).
func (l *Lobby) build() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, d := range l.load() {
		if r, err := l.open(d); err == nil {
			l.recs = append(l.recs, r)
		} else {
			l.s.Log.Printf("table %s: %v", d.ID, err)
		}
	}
	if len(l.recs) == 0 {
		for _, d := range defaultTables(l.s.cfg()) {
			if r, err := l.open(d); err == nil {
				l.recs = append(l.recs, r)
			}
		}
	}
	l.save()
}

// start runs the tables and the lobby's own timer.
func (l *Lobby) start(stop <-chan struct{}) {
	l.mu.Lock()
	l.stop = stop
	l.started = true
	for _, r := range l.recs {
		l.launch(r)
	}
	l.mu.Unlock()
	l.wg.Add(1)
	go func() { defer l.wg.Done(); l.loop() }()
}

// wait returns once every table has stopped.
func (l *Lobby) wait() { l.wg.Wait() }

// launch runs a table's goroutine; caller holds l.mu.
func (l *Lobby) launch(r *tableRec) {
	l.wg.Add(1)
	name, t, stop := "table "+r.def.ID, r.t, r.stop
	go func() {
		defer l.wg.Done()
		guard(l.s.Log.Printf, name, nil, func() { t.run(stop) })
	}()
}

// setNotifier points every table at the Discord bot.
func (l *Lobby) setNotifier(n Notifier) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.recs {
		switch t := r.t.(type) {
		case *bjTable:
			t.g.notify = n
		case *Holdem:
			t.notify = n
		case beggarTable:
			t.Do(func() { t.notify = n })
		}
	}
}

// firstGame is the first blackjack table's game (older code and tests).
func (l *Lobby) firstGame() *Game {
	for _, r := range l.recs {
		if b, ok := r.t.(*bjTable); ok {
			return b.g
		}
	}
	return nil
}

// open builds and starts a table; caller holds l.mu.
func (l *Lobby) open(d TableDef) (*tableRec, error) {
	s := l.s
	d.Name = strings.TrimSpace(d.Name)
	if d.ID == "" || d.Name == "" {
		return nil, errors.New("a table needs an id and a name")
	}
	var t table
	switch d.Game {
	case proto.GameBlackjack:
		cfg, err := blackjackConfig(s.cfg(), d)
		if err != nil {
			return nil, err
		}
		g := NewGame(cfg, s.Store, s.notifier(), s.Log)
		g.exec = s.Exec
		g.tableID, g.tableName = d.ID, d.Name
		g.lobbyRefills = true
		t = &bjTable{g: g}
	case proto.GameHoldem:
		hc, err := holdemConfig(d)
		if err != nil {
			return nil, err
		}
		h := newHoldem(d.ID, d.Name, s.cfg(), hc, s.Store, s.notifier(), s.Log)
		h.exec = s.Exec
		t = h
	case proto.GameBeggar:
		bc, err := beggarConfig(d)
		if err != nil {
			return nil, err
		}
		b := newBeggar(d.ID, d.Name, s.cfg(), bc, s.Store, s.notifier(), s.Log)
		b.exec = s.Exec
		t = beggarTable{b}
	default:
		return nil, fmt.Errorf("unknown game %q (blackjack, holdem or beggar)", d.Game)
	}
	return &tableRec{def: d, t: t, stop: make(chan struct{})}, nil
}

func (l *Lobby) loop() {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	refill := time.NewTicker(20 * time.Second)
	defer refill.Stop()
	l.checkRefill()
	for {
		select {
		case <-l.stop:
			l.mu.Lock()
			for _, r := range l.recs {
				close(r.stop)
			}
			l.mu.Unlock()
			return
		case <-tick.C:
			l.sendLobby(false)
		case <-refill.C:
			guard(l.s.Log.Printf, "the daily refill", nil, l.checkRefill)
		}
	}
}

// lobbyMsg builds the table list for one connection.
func (l *Lobby) lobbyMsg(c *conn) proto.Lobby {
	var out proto.Lobby
	for _, r := range l.recs {
		out.Tables = append(out.Tables, r.t.info())
	}
	if r := l.at[c]; r != nil {
		out.Current = r.def.ID
	}
	return out
}

// sendLobby sends the table list to everyone (only when it changed, unless
// force).
func (l *Lobby) sendLobby(force bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	base := l.lobbyMsg(nil)
	b, _ := json.Marshal(base)
	if !force && string(b) == l.lastSent {
		return
	}
	l.lastSent = string(b)
	for c := range l.conns {
		m := base
		if r := l.at[c]; r != nil {
			m.Current = r.def.ID
		}
		c.send(proto.SLobby, m)
	}
}

func (l *Lobby) find(id string) *tableRec {
	for _, r := range l.recs {
		if strings.EqualFold(r.def.ID, id) {
			return r
		}
	}
	return nil
}

// ---------------------------------------------------------------- players

// authed adds a logged-in connection. Older games that don't know about
// the lobby go straight to the first blackjack table, as they used to.
func (l *Lobby) authed(c *conn) {
	l.mu.Lock()
	for other := range l.conns {
		if other != c && strings.EqualFold(other.user, c.user) {
			go other.kick("You logged in from another client.")
		}
	}
	l.conns[c] = true
	l.s.Store.SetOnline(c.user, true)
	m := l.lobbyMsg(c)
	var first *tableRec
	for _, r := range l.recs {
		if r.def.Game == proto.GameBlackjack {
			first = r
			break
		}
	}
	l.mu.Unlock()
	c.send(proto.SLobby, m)
	if !c.lobbyOK && first != nil {
		l.join(c, first.def.ID, 0)
		return
	}
	// chips still at a poker table (a reconnect): go back to it
	if stack, id := l.s.Store.TableStack(c.user); stack > 0 && id != "" {
		l.join(c, id, 0)
	}
}

// join moves a connection to a table.
func (l *Lobby) join(c *conn, id string, buyIn int64) {
	l.mu.Lock()
	r := l.find(id)
	if r == nil {
		l.mu.Unlock()
		c.send(proto.SError, proto.ErrorMsg{Message: "That table isn't open any more."})
		return
	}
	old := l.at[c]
	if old == r {
		l.mu.Unlock()
		if buyIn > 0 {
			r.t.msg(c, mkMsg(proto.CPoker, proto.PokerAct{Action: "sit", Amount: buyIn}))
		}
		return
	}
	l.at[c] = r
	l.mu.Unlock()
	if old != nil {
		old.t.leave(c)
	}
	if stack, at := l.s.Store.TableStack(c.user); stack > 0 && at != r.def.ID {
		// chips left at another table (shouldn't happen): bring them back first
		l.s.Store.CashOut(c.user)
	}
	c.send(proto.SJoined, proto.Joined{ID: r.def.ID, Game: r.def.Game, Name: r.def.Name})
	r.t.join(c, c.user, buyIn)
	l.sendLobby(true)
}

// leaveTable sends a connection back to the lobby.
func (l *Lobby) leaveTable(c *conn) {
	l.mu.Lock()
	r := l.at[c]
	delete(l.at, c)
	m := l.lobbyMsg(c)
	l.mu.Unlock()
	if r != nil {
		r.t.leave(c)
	}
	c.send(proto.SLeft, proto.ErrorMsg{})
	c.send(proto.SLobby, m)
	l.sendLobby(true)
}

// disconnect forgets a connection.
func (l *Lobby) disconnect(c *conn) {
	l.mu.Lock()
	r := l.at[c]
	delete(l.at, c)
	delete(l.conns, c)
	still := false
	for other := range l.conns {
		if strings.EqualFold(other.user, c.user) {
			still = true
		}
	}
	l.mu.Unlock()
	if r != nil {
		r.t.leave(c)
	}
	if !still {
		l.s.Store.SetOnline(c.user, false)
	}
}

// route hands a table message to the player's table.
func (l *Lobby) route(c *conn, m proto.Msg) {
	l.mu.Lock()
	r := l.at[c]
	l.mu.Unlock()
	if r == nil {
		if m.Type == proto.CChat {
			var ch proto.Chat
			_ = json.Unmarshal(m.Data, &ch)
			text := strings.TrimSpace(ch.Text)
			if strings.HasPrefix(text, "/") && l.s.Store.IsAdmin(c.user) {
				runAdminCommand(l.s.Log, l.s.Exec, c, c.user, text)
				return
			}
			c.send(proto.SError, proto.ErrorMsg{Message: "Join a table to chat."})
		}
		return
	}
	r.t.msg(c, m)
}

// connOf finds a user's connection.
func (l *Lobby) connOf(user string) *conn {
	l.mu.Lock()
	defer l.mu.Unlock()
	for c := range l.conns {
		if strings.EqualFold(c.user, user) {
			return c
		}
	}
	return nil
}

func (l *Lobby) tableOf(user string) *tableRec {
	l.mu.Lock()
	defer l.mu.Unlock()
	for c, r := range l.at {
		if strings.EqualFold(c.user, user) {
			return r
		}
	}
	return nil
}

// ---------------------------------------------------------------- server-wide

// Kick disconnects a user.
func (l *Lobby) Kick(user, reason string) bool {
	if c := l.connOf(user); c != nil {
		c.kick(reason)
		return true
	}
	return false
}

// NotifyUser shows a user a message.
func (l *Lobby) NotifyUser(user, text string) {
	if c := l.connOf(user); c != nil {
		c.event(proto.Event{Kind: "info", Text: text, Seat: -1})
	}
}

// RefreshProfile sends a user their profile again.
func (l *Lobby) RefreshProfile(user string) {
	if c := l.connOf(user); c != nil {
		if p, ok := l.s.Store.Profile(c.user); ok {
			c.send(proto.SProfile, p)
		}
	}
}

// Announce tells everyone something.
func (l *Lobby) Announce(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for c := range l.conns {
		c.event(proto.Event{Kind: "info", Text: text, Seat: -1})
	}
}

// Broadcast refreshes every table.
func (l *Lobby) Broadcast() {
	l.mu.Lock()
	recs := append([]*tableRec(nil), l.recs...)
	l.mu.Unlock()
	for _, r := range recs {
		r.t.broadcastAll()
	}
}

// InHand reports whether a user has chips in play at any table.
func (l *Lobby) InHand(user string) bool {
	l.mu.Lock()
	recs := append([]*tableRec(nil), l.recs...)
	l.mu.Unlock()
	for _, r := range recs {
		if r.t.inHand(user) {
			return true
		}
	}
	return false
}

// PlayerList describes every table (console "players").
func (l *Lobby) PlayerList() []string {
	l.mu.Lock()
	recs := append([]*tableRec(nil), l.recs...)
	idle := 0
	for c := range l.conns {
		if l.at[c] == nil {
			idle++
		}
	}
	l.mu.Unlock()
	var out []string
	for _, r := range recs {
		out = append(out, fmt.Sprintf("[%s] %s (%s)", r.def.ID, r.def.Name, r.def.Game))
		for _, p := range r.t.players() {
			out = append(out, "    "+p)
		}
	}
	out = append(out, fmt.Sprintf("in the lobby: %d", idle))
	return out
}

// Summary is everyone at a table (the server window, Discord).
func (l *Lobby) Summary() (seated int, names []string, tables []string) {
	l.mu.Lock()
	recs := append([]*tableRec(nil), l.recs...)
	l.mu.Unlock()
	for _, r := range recs {
		in := r.t.info()
		seated += in.Players
		names = append(names, in.Names...)
		tables = append(tables, r.t.summary())
	}
	return
}

// Reload applies new server settings to every table.
func (l *Lobby) Reload(c *Config) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.recs {
		r.t.reconfigure(c, r.def)
	}
}

// checkRefill runs the daily refill when a new period starts. Players in
// the middle of a blackjack hand get theirs when the hand ends.
func (l *Lobby) checkRefill() {
	cfg := l.s.cfg()
	period := cfg.RefillPeriod(time.Now())
	if period == l.lastPeriod {
		return
	}
	l.lastPeriod = period
	l.mu.Lock()
	recs := append([]*tableRec(nil), l.recs...)
	l.mu.Unlock()
	busy := map[string]bool{}
	for _, r := range recs {
		if bt, ok := r.t.(*bjTable); ok {
			for _, u := range bt.inHandUsers() {
				busy[strings.ToLower(u)] = true
			}
			bt.g.Do(func() { bt.g.lastPeriod = period })
		}
	}
	var refilled []string
	for _, u := range l.s.Store.Usernames() {
		if busy[strings.ToLower(u)] {
			continue
		}
		if d := l.s.Store.ApplyRefill(u, cfg.RefillAmount, period); d > 0 {
			refilled = append(refilled, u)
			if c := l.connOf(u); c != nil {
				c.event(proto.Event{Kind: "refill", Seat: -1, Delta: d, Text: fmt.Sprintf("Daily refill: +%d chips", d)})
				if p, ok := l.s.Store.Profile(u); ok {
					c.send(proto.SProfile, p)
				}
			}
		}
	}
	if len(refilled) > 0 {
		l.s.notifier().Refilled(refilled, cfg.RefillAmount)
		l.s.notifier().LeaderboardChanged()
		l.Broadcast()
	}
}

// ---------------------------------------------------------------- gifts

// reserved is what a user has bet but not got back at a blackjack table (it
// counts toward their chips for gift limits), and what of their balance is
// promised to a bet that hasn't been taken yet.
func (l *Lobby) reserved(user string) (staked, pending int64) {
	r := l.tableOf(user)
	if r == nil {
		return 0, 0
	}
	if bt, ok := r.t.(*bjTable); ok {
		bt.g.DoWait(func() {
			staked = bt.g.staked(user)
			if p := bt.g.byUser[strings.ToLower(user)]; p != nil && p.betPlaced && !p.inHand {
				pending = p.pendingBet
			}
		})
	}
	return
}

func (l *Lobby) giftMax(from, to string) (int64, string) {
	toStaked, _ := l.reserved(to)
	_, fromPending := l.reserved(from)
	return giftLimit(l.s.cfg(), l.s.Store, from, fromPending, to, toStaked)
}

func (l *Lobby) giftInfo(c *conn, to string) {
	max, why := l.giftMax(c.user, to)
	c.send(proto.SGiftInfo, proto.GiftInfo{To: to, Max: max, Reason: why, Balance: l.s.Store.Balance(to)})
}

func (l *Lobby) gift(c *conn, to string, amount int64) {
	max, why := l.giftMax(c.user, to)
	switch {
	case max <= 0:
		c.send(proto.SError, proto.ErrorMsg{Message: why})
		return
	case amount <= 0:
		c.send(proto.SError, proto.ErrorMsg{Message: "Send at least 1 chip."})
		return
	case amount > max:
		c.send(proto.SError, proto.ErrorMsg{Message: why})
		return
	}
	prof, _ := l.s.Store.Profile(to)
	if err := l.s.Store.Gift(c.user, prof.Username, amount, l.s.cfg().RefillPeriod(time.Now())); err != nil {
		c.send(proto.SError, proto.ErrorMsg{Message: err.Error()})
		return
	}
	l.s.Log.Printf("gift: %s sent %d chips to %s", c.user, amount, prof.Username)
	text := fmt.Sprintf("%s sent %s %s chips", c.user, prof.Username, commas(amount))
	l.mu.Lock()
	for oc := range l.conns {
		seat := -1
		if strings.EqualFold(oc.user, prof.Username) {
			seat = -2 // "you": the client shows the amount big
		}
		oc.event(proto.Event{Kind: "gift", Seat: seat, Delta: amount, Text: text})
	}
	l.mu.Unlock()
	l.RefreshProfile(c.user)
	l.RefreshProfile(prof.Username)
	if r := l.tableOf(c.user); r != nil {
		if bt, ok := r.t.(*bjTable); ok {
			bt.g.Do(func() {
				if p := bt.g.byUser[strings.ToLower(c.user)]; p != nil && bt.g.cfg.DealerTrashTalk {
					bt.g.say(banter.EvGift, p, banter.Vars{Amount: amount, Item: prof.Username}, 0.9, prioNormal)
				}
			})
		}
	}
	l.Broadcast()
}

// ---------------------------------------------------------------- admin

// Tables lists the tables (console, admin windows).
func (l *Lobby) Tables() []proto.TableInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lobbyMsg(nil).Tables
}

func slug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "table"
	}
	return s
}

// AddTable opens a new table.
func (l *Lobby) AddTable(game, name string) (string, error) {
	game = strings.ToLower(strings.TrimSpace(game))
	switch game {
	case "bj", "21":
		game = proto.GameBlackjack
	case "poker", "texas", "holdem", "hold'em", "texas-holdem":
		game = proto.GameHoldem
	case "beggar", "beggars", "dice":
		game = proto.GameBeggar
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.recs) >= 20 {
		return "", errors.New("that's plenty of tables (20 at most)")
	}
	id := slug(name)
	for n := 2; l.find(id) != nil; n++ {
		id = slug(name) + "-" + strconv.Itoa(n)
	}
	r, err := l.open(TableDef{ID: id, Name: name, Game: game})
	if err != nil {
		return "", err
	}
	l.recs = append(l.recs, r)
	if l.started {
		l.launch(r)
	}
	l.save()
	go l.sendLobby(true)
	return id, nil
}

// RemoveTable closes a table: everyone at it goes back to the lobby with
// their chips.
func (l *Lobby) RemoveTable(id string) error {
	l.mu.Lock()
	r := l.find(id)
	if r == nil {
		l.mu.Unlock()
		return fmt.Errorf("there's no table %q", id)
	}
	if len(l.recs) == 1 {
		l.mu.Unlock()
		return errors.New("that's the last table; open another one first")
	}
	for i, x := range l.recs {
		if x == r {
			l.recs = append(l.recs[:i], l.recs[i+1:]...)
			break
		}
	}
	var gone []*conn
	for c, x := range l.at {
		if x == r {
			gone = append(gone, c)
			delete(l.at, c)
		}
	}
	l.save()
	l.mu.Unlock()
	r.t.close("The house closed " + r.def.Name + ". Back to the lobby.")
	close(r.stop)
	for _, c := range gone {
		c.send(proto.SLeft, proto.ErrorMsg{Message: "Table closed"})
	}
	l.sendLobby(true)
	return nil
}

// RenameTable changes a table's name.
func (l *Lobby) RenameTable(id, name string) error {
	return l.SetTableSetting(id, "name", name)
}

// tableKeys are the settings a table of each game can have.
var bjTableKeys = []string{"max_players", "max_spectators", "min_bet", "max_bet", "betting_mode", "play_mode",
	"hide_other_hands", "decks", "penetration", "dealer_hits_soft_17", "blackjack_pays", "double_after_split",
	"double_on", "max_split_hands", "resplit_aces", "hit_split_aces", "allow_surrender", "turbo_mode",
	"betting_time_seconds", "turn_time_seconds", "play_time_seconds", "results_time_seconds",
	"deal_delay_ms", "dealer_delay_ms", "card_reveal", "card_reveal_min_seconds", "card_reveal_max_seconds"}

// TableSettings lists one table's settings.
func (l *Lobby) TableSettings(id string) ([]proto.SettingInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.find(id)
	if r == nil {
		return nil, fmt.Errorf("there's no table %q", id)
	}
	out := []proto.SettingInfo{{Key: "name", Label: "Table name", Kind: "string", Value: r.def.Name}}
	switch r.def.Game {
	case proto.GameBlackjack:
		cfg, _ := blackjackConfig(l.s.cfg(), r.def)
		want := map[string]bool{}
		for _, k := range bjTableKeys {
			want[k] = true
		}
		for _, st := range SettingsOf(cfg) {
			if want[st.Key] {
				out = append(out, proto.SettingInfo{Key: st.Key, Label: LabelOf(st.Key), Kind: st.Kind, Value: st.Value})
			}
		}
	case proto.GameHoldem:
		hc, _ := holdemConfig(r.def)
		for _, st := range settingsOfValue(reflect.ValueOf(&hc).Elem(), "", "") {
			out = append(out, proto.SettingInfo{Key: st.Key, Label: LabelOf(st.Key), Kind: st.Kind, Value: st.Value})
		}
	case proto.GameBeggar:
		bc, _ := beggarConfig(r.def)
		for _, st := range settingsOfValue(reflect.ValueOf(&bc).Elem(), "", "") {
			out = append(out, proto.SettingInfo{Key: st.Key, Label: LabelOf(st.Key), Kind: st.Kind, Value: st.Value})
		}
	}
	return out, nil
}

// SetTableSetting changes one setting of one table (from the next hand).
func (l *Lobby) SetTableSetting(id, key, value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.find(id)
	if r == nil {
		return fmt.Errorf("there's no table %q", id)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	def := r.def
	def.Settings = map[string]any{}
	for k, v := range r.def.Settings {
		def.Settings[k] = v
	}
	if key == "name" {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 40 {
			return errors.New("a table name is 1 to 40 characters")
		}
		def.Name = value
	} else {
		var kind string
		switch def.Game {
		case proto.GameBlackjack:
			ok := false
			for _, k := range bjTableKeys {
				if k == key {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("blackjack tables don't have a %q setting (server-wide settings are changed with set)", key)
			}
			for _, st := range SettingsOf(l.s.cfg()) {
				if st.Key == key {
					kind = st.Kind
				}
			}
		case proto.GameHoldem:
			hc := defaultHoldem()
			for _, st := range settingsOfValue(reflect.ValueOf(&hc).Elem(), "", "") {
				if st.Key == key {
					kind = st.Kind
				}
			}
		case proto.GameBeggar:
			bc := defaultBeggar()
			for _, st := range settingsOfValue(reflect.ValueOf(&bc).Elem(), "", "") {
				if st.Key == key {
					kind = st.Kind
				}
			}
		}
		if kind == "" {
			return fmt.Errorf("there's no setting %q for this table", key)
		}
		v, err := parseSetting(key, kind, value)
		if err != nil {
			return err
		}
		def.Settings[key] = v
	}
	// check it builds
	switch def.Game {
	case proto.GameBlackjack:
		if _, err := blackjackConfig(l.s.cfg(), def); err != nil {
			return err
		}
	case proto.GameHoldem:
		if _, err := holdemConfig(def); err != nil {
			return err
		}
	case proto.GameBeggar:
		if _, err := beggarConfig(def); err != nil {
			return err
		}
	}
	r.def = def
	r.t.reconfigure(l.s.cfg(), def)
	l.save()
	go l.sendLobby(true)
	return nil
}

// blackjackConfig is the server config with a table's overrides.
func blackjackConfig(global *Config, d TableDef) (*Config, error) {
	cp := *global
	if len(d.Settings) > 0 {
		b, _ := json.Marshal(d.Settings)
		if err := json.Unmarshal(b, &cp); err != nil {
			return nil, err
		}
	}
	if err := cp.validate(); err != nil {
		return nil, err
	}
	cp.DataDir = global.DataDir
	return &cp, nil
}

// holdemConfig is Hold'em's defaults with a table's settings.
func holdemConfig(d TableDef) (holdemCfg, error) {
	hc := defaultHoldem()
	if len(d.Settings) > 0 {
		b, _ := json.Marshal(d.Settings)
		if err := json.Unmarshal(b, &hc); err != nil {
			return hc, err
		}
	}
	hc.validate()
	return hc, nil
}

// TablesText describes the tables for the console.
func (l *Lobby) TablesText() []string {
	var out []string
	for _, in := range l.Tables() {
		out = append(out, fmt.Sprintf("  %-14s %-24s %-9s %s, %d/%d seated", in.ID, in.Name, in.Game, in.Stakes, in.Players, in.Seats))
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- blackjack

// bjTable is a blackjack Game as a lobby table.
type bjTable struct{ g *Game }

func (b *bjTable) info() proto.TableInfo {
	if in := b.g.info.Load(); in != nil {
		return *in
	}
	return proto.TableInfo{ID: b.g.tableID, Name: b.g.tableName, Game: proto.GameBlackjack}
}
func (b *bjTable) join(c *conn, user string, _ int64) {
	b.g.events <- event{kind: evJoin, c: c, user: user}
}
func (b *bjTable) leave(c *conn)            { b.g.events <- event{kind: evLeave, c: c} }
func (b *bjTable) msg(c *conn, m proto.Msg) { b.g.events <- event{kind: evMsg, c: c, msg: m} }
func (b *bjTable) run(stop <-chan struct{}) { b.g.Run(stop) }
func (b *bjTable) broadcastAll()            { b.g.Broadcast() }
func (b *bjTable) inHand(user string) bool  { return b.g.InHand(user) }
func (b *bjTable) summary() string {
	p, n, _, h := b.g.Summary()
	return fmt.Sprintf("%s: %d seated, %s, hand #%d", b.g.tableName, n, p, h)
}
func (b *bjTable) reconfigure(global *Config, d TableDef) {
	if cfg, err := blackjackConfig(global, d); err == nil {
		b.g.Do(func() { b.g.tableName = d.Name; b.g.updateInfo() })
		b.g.Reload(cfg)
	}
}
func (b *bjTable) inHandUsers() []string {
	var out []string
	b.g.DoWait(func() {
		for _, p := range b.g.seats {
			if p != nil && p.inHand {
				out = append(out, p.user)
			}
		}
	})
	return out
}
func (b *bjTable) players() []string { return b.g.PlayerList() }
func (b *bjTable) close(reason string) {
	b.g.DoWait(func() {
		b.g.abortHand()
		for c := range b.g.conns {
			c.event(proto.Event{Kind: "info", Text: reason, Seat: -1})
		}
		b.g.conns = map[*conn]*player{}
		b.g.byUser = map[string]*player{}
		for i := range b.g.seats {
			b.g.seats[i] = nil
		}
		b.g.spectators = nil
		b.g.stopTimer()
	})
}

// ---------------------------------------------------------------- Hold'em

func (t *Holdem) info() proto.TableInfo {
	if in := t.infoBox.v.Load(); in != nil {
		return *in
	}
	return proto.TableInfo{ID: t.id, Name: t.name, Game: proto.GameHoldem}
}
func (t *Holdem) join(c *conn, user string, buyIn int64) {
	t.events <- event{kind: evJoin, c: c, user: user, buyIn: buyIn}
}
func (t *Holdem) leave(c *conn)            { t.events <- event{kind: evLeave, c: c} }
func (t *Holdem) msg(c *conn, m proto.Msg) { t.events <- event{kind: evMsg, c: c, msg: m} }
func (t *Holdem) broadcastAll()            { t.Do(t.broadcast) }
func (t *Holdem) close(reason string)      { t.closeTable(reason) }
func (t *Holdem) reconfigure(global *Config, d TableDef) {
	if hc, err := holdemConfig(d); err == nil {
		t.setConfig(global, hc, d.Name)
	}
}
func (t *Holdem) inHand(user string) bool {
	in := false
	t.DoWait(func() {
		if p := t.byUser[strings.ToLower(user)]; p != nil {
			in = p.inHand && t.phase != pkWaiting
		}
	})
	return in
}
func (t *Holdem) inHandUsers() []string {
	var out []string
	t.DoWait(func() {
		for _, p := range t.humans() {
			if p.inHand && t.phase != pkWaiting {
				out = append(out, p.user)
			}
		}
	})
	return out
}
func (t *Holdem) players() []string {
	var out []string
	t.DoWait(func() {
		for _, p := range t.humans() {
			st := "waiting"
			switch {
			case p.inHand && p.folded:
				st = "folded"
			case p.inHand:
				st = "in the hand"
			case p.sitOut:
				st = "sitting out"
			}
			out = append(out, fmt.Sprintf("seat %d: %-16s %-12s stack %d", p.seat, p.user, st, p.stack))
		}
		out = append(out, fmt.Sprintf("phase: %s, hand #%d, Dale's stack %d", t.phase, t.handNo, t.seats[0].stack))
	})
	return out
}
func (t *Holdem) summary() string {
	in := t.info()
	return fmt.Sprintf("%s: %d seated, %s", t.name, in.Players, in.Stakes)
}

// mkMsg builds a message as if a client had sent it.
func mkMsg(typ string, v any) proto.Msg {
	b, _ := json.Marshal(v)
	return proto.Msg{Type: typ, Data: b}
}
