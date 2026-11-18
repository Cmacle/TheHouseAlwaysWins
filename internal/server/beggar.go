// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Beggars Table: for anyone (nearly) broke with nobody to buy them back
// in. Roll five dice; five sixes pays a small handout. Long odds, but it
// never runs out, so a player can always grind their way back to a table.

type beggarCfg struct {
	Odds       int   `json:"odds"`        // 1 in this many rolls wins
	Prize      int64 `json:"prize"`       // what a win pays
	MaxBalance int64 `json:"max_balance"` // you can roll with fewer chips than this
	RollMinMS  int   `json:"roll_min_ms"` // a roll takes between these
	RollMaxMS  int   `json:"roll_max_ms"`
}

func defaultBeggar() beggarCfg {
	return beggarCfg{Odds: 1000, Prize: 50, MaxBalance: 50, RollMinMS: 3000, RollMaxMS: 5000}
}

func (c *beggarCfg) validate() {
	d := defaultBeggar()
	if c.Odds < 1 {
		c.Odds = d.Odds
	}
	if c.Prize < 1 {
		c.Prize = d.Prize
	}
	if c.MaxBalance < 1 {
		c.MaxBalance = d.MaxBalance
	}
	c.RollMinMS = min(max(c.RollMinMS, 500), 20000)
	c.RollMaxMS = min(max(c.RollMaxMS, c.RollMinMS), 20000)
}

// beggarConfig is the defaults with a table's settings.
func beggarConfig(d TableDef) (beggarCfg, error) {
	bc := defaultBeggar()
	if len(d.Settings) > 0 {
		b, _ := json.Marshal(d.Settings)
		if err := json.Unmarshal(b, &bc); err != nil {
			return bc, err
		}
	}
	bc.validate()
	return bc, nil
}

type bgPlayer struct {
	user     string
	conn     *conn
	rolling  bool
	rollID   int
	lastTalk time.Time
	hello    bool
	chatAt   []time.Time
}

// Beggar runs a Beggars Table.
type Beggar struct {
	id, name string
	cfg      *Config
	bc       beggarCfg
	store    *Store
	notify   Notifier
	log      *log.Logger
	exec     func(string, io.Writer)
	events   chan event
	rng      *rand.Rand
	voice    *voice
	conns    map[*conn]*bgPlayer
	feed     []string
	rolls    int64
	nextID   int
	info     atomic.Pointer[proto.TableInfo]
	// tests: rolls land at once
	instant bool
}

func newBeggar(id, name string, cfg *Config, bc beggarCfg, store *Store, n Notifier, lg *log.Logger) *Beggar {
	bc.validate()
	if n == nil {
		n = nopNotifier{}
	}
	t := &Beggar{id: id, name: name, cfg: cfg, bc: bc, store: store, notify: n, log: lg,
		events: make(chan event, 1024), rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		voice: newVoice(), conns: map[*conn]*bgPlayer{}}
	t.voice.budget = 1 << 30 // no hands here to reset his budget
	t.updateInfo()
	return t
}

func (t *Beggar) run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case ev := <-t.events:
			guard(t.log.Printf, "the Beggars Table", nil, func() { t.handle(ev) })
		case <-t.voice.timerC():
			if t.voice.tick() {
				t.broadcast()
			}
		}
	}
}

// Do runs fn on the table goroutine.
func (t *Beggar) Do(fn func()) { t.events <- event{kind: evFunc, fn: fn} }

// DoWait runs fn on the table goroutine and waits.
func (t *Beggar) DoWait(fn func()) {
	done := make(chan struct{})
	t.events <- event{kind: evFunc, fn: func() { fn(); close(done) }}
	<-done
}

func (t *Beggar) handle(ev event) {
	switch ev.kind {
	case evJoin:
		p := &bgPlayer{user: ev.user, conn: ev.c}
		t.conns[ev.c] = p
		t.updateInfo()
		t.send(p, true)
		if t.store.Chips(p.user) < t.bc.MaxBalance {
			t.say(banter.EvBgHello, p, banter.Vars{Amount: t.bc.Prize}, 0.9, prioNormal)
		} else {
			t.say(banter.EvBgTooRich, p, banter.Vars{Amount: t.store.Chips(p.user)}, 0.9, prioNormal)
		}
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
		case proto.CDice:
			t.roll(p)
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

// why says why a player can't roll ("" = they can).
func (t *Beggar) why(p *bgPlayer) string {
	if p.rolling {
		return "Your dice are still rolling."
	}
	if chips := t.store.Chips(p.user); chips >= t.bc.MaxBalance {
		return fmt.Sprintf("This table is for players with fewer than %s chips. You have %s.", commas(t.bc.MaxBalance), commas(chips))
	}
	return ""
}

func (t *Beggar) roll(p *bgPlayer) {
	if r := t.why(p); r != "" {
		if !p.rolling {
			t.say(banter.EvBgTooRich, p, banter.Vars{Amount: t.store.Chips(p.user)}, 0.5, prioNormal)
		}
		p.conn.send(proto.SError, proto.ErrorMsg{Message: r})
		return
	}
	t.nextID++
	t.rolls++
	win := t.rng.Intn(t.bc.Odds) == 0
	n := t.store.BeggarRoll(p.user)
	prize := int64(0)
	if win {
		prize = t.bc.Prize
	}
	t.store.RecordTable(p.user, t.id, prize)
	faces := make([]int, 5)
	for {
		six := 0
		for i := range faces {
			faces[i] = 1 + t.rng.Intn(6)
			if faces[i] == 6 {
				six++
			}
		}
		if six < 5 {
			break
		}
	}
	if win {
		for i := range faces {
			faces[i] = 6
		}
	}
	ms := t.bc.RollMinMS + t.rng.Intn(t.bc.RollMaxMS-t.bc.RollMinMS+1)
	p.rolling, p.rollID = true, t.nextID
	p.conn.send(proto.SDice, proto.DiceRoll{ID: t.nextID, Faces: faces, Win: win, MS: int64(ms), Seed: t.rng.Int63()})
	switch {
	case n%100 == 0:
		t.say(banter.EvBgMany, p, banter.Vars{Amount: n}, 1, prioHigh)
	case time.Since(p.lastTalk) > 25*time.Second:
		if t.say(banter.EvBgRoll, p, banter.Vars{Amount: t.bc.Prize}, 0.15, prioLow) {
			p.lastTalk = time.Now()
		}
	}
	t.send(p, false)
	id := t.nextID
	land := func() { t.landed(p, id, faces, win) }
	if t.instant {
		land()
		return
	}
	time.AfterFunc(time.Duration(ms)*time.Millisecond, func() { t.Do(land) })
}

// landed runs when the dice stop.
func (t *Beggar) landed(p *bgPlayer, id int, faces []int, win bool) {
	if p.rollID != id {
		return
	}
	p.rolling = false
	if win {
		t.store.Handout(p.user, t.bc.Prize)
		t.addFeed(fmt.Sprintf("%s rolled FIVE SIXES and won %s!", p.user, commas(t.bc.Prize)))
		t.log.Printf("[%s] %s rolled five sixes (+%d)", t.id, p.user, t.bc.Prize)
		t.say(banter.EvBgWin, p, banter.Vars{Amount: t.bc.Prize}, 1, prioHigh)
		for c := range t.conns {
			c.event(proto.Event{Kind: "info", Seat: -1, Text: fmt.Sprintf("%s rolled five sixes!", p.user)})
		}
		if t.conns[p.conn] != nil {
			p.conn.event(proto.Event{Kind: "beggar_win", Seat: -1, Delta: t.bc.Prize, Text: fmt.Sprintf("FIVE SIXES! +%s CHIPS", commas(t.bc.Prize))})
			if prof, ok := t.store.Profile(p.user); ok {
				p.conn.send(proto.SProfile, prof)
			}
		}
	}
	t.broadcast()
}

func (t *Beggar) addFeed(s string) {
	t.feed = append(t.feed, s)
	if len(t.feed) > 6 {
		t.feed = t.feed[len(t.feed)-6:]
	}
}

func (t *Beggar) say(ev string, p *bgPlayer, v banter.Vars, chance float64, prio int) bool {
	if !t.cfg.DealerTrashTalk {
		return false
	}
	if p != nil {
		v.Name = p.user
	}
	if t.voice.say(ev, t.cfg.DealerVulgar, v, chance, prio) {
		t.broadcast()
		return true
	}
	return false
}

func (t *Beggar) onChat(p *bgPlayer, text string) {
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
	p.chatAt = recent
	if len(recent) >= 4 {
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

func (t *Beggar) view(p *bgPlayer, withLog bool) proto.BeggarState {
	st := proto.BeggarState{Table: t.id, Name: t.name, Odds: t.bc.Odds, Prize: t.bc.Prize, MaxBalance: t.bc.MaxBalance,
		Balance: t.store.Chips(p.user), Rolling: p.rolling, TableRolls: t.rolls, Feed: t.feed,
		ServerTime: time.Now().UnixMilli(),
		Settings: proto.Settings{ServerName: t.cfg.ServerName, RefillAmount: t.cfg.RefillAmount,
			DealerTrashTalk: t.cfg.DealerTrashTalk, DealerVulgar: t.cfg.DealerTrashTalk && t.cfg.DealerVulgar,
			ChipGifts: t.cfg.ChipGifts && t.cfg.RefillAmount > 0, TableChat: t.cfg.TableChat}}
	st.Reason = t.why(p)
	st.CanRoll = st.Reason == ""
	if prof, ok := t.store.Profile(p.user); ok {
		st.Rolls, st.Wins = prof.BeggarRolls, prof.BeggarWins
	}
	for _, o := range t.conns {
		st.Players = append(st.Players, o.user)
	}
	house := t.store.House()
	st.HouseNet, st.HouseTier = house.Net, house.Tier
	var ps proto.PokerState
	t.voice.fill(&ps, withLog)
	st.DealerSay, st.DealerSayID, st.DealerSayUntil = ps.DealerSay, ps.DealerSayID, ps.DealerSayUntil
	st.DealerSayCont, st.DealerSayPara, st.DealerLog = ps.DealerSayCont, ps.DealerSayPara, ps.DealerLog
	return st
}

func (t *Beggar) send(p *bgPlayer, withLog bool) { p.conn.send(proto.SBeggar, t.view(p, withLog)) }

func (t *Beggar) broadcast() {
	for _, p := range t.conns {
		t.send(p, false)
	}
}

func (t *Beggar) updateInfo() {
	in := proto.TableInfo{ID: t.id, Name: t.name, Game: proto.GameBeggar, Seats: 99,
		Stakes: fmt.Sprintf("Under %s chips only", commas(t.bc.MaxBalance))}
	for _, p := range t.conns {
		in.Players++
		in.Names = append(in.Names, p.user)
	}
	t.info.Store(&in)
}

// ---------------------------------------------------------------- lobby

func (t *Beggar) tinfo() proto.TableInfo {
	if in := t.info.Load(); in != nil {
		return *in
	}
	return proto.TableInfo{ID: t.id, Name: t.name, Game: proto.GameBeggar}
}

type beggarTable struct{ *Beggar }

func (b beggarTable) info() proto.TableInfo { return b.tinfo() }
func (b beggarTable) join(c *conn, user string, _ int64) {
	b.events <- event{kind: evJoin, c: c, user: user}
}
func (b beggarTable) leave(c *conn)            { b.events <- event{kind: evLeave, c: c} }
func (b beggarTable) msg(c *conn, m proto.Msg) { b.events <- event{kind: evMsg, c: c, msg: m} }
func (b beggarTable) broadcastAll()            { b.Do(b.broadcast) }
func (b beggarTable) close(reason string) {
	b.DoWait(func() {
		for c := range b.conns {
			c.send(proto.SLeft, proto.ErrorMsg{Message: reason})
			c.event(proto.Event{Kind: "info", Text: reason, Seat: -1})
		}
		b.conns = map[*conn]*bgPlayer{}
	})
}
func (b beggarTable) reconfigure(global *Config, d TableDef) {
	bc, err := beggarConfig(d)
	if err != nil {
		return
	}
	b.Do(func() {
		b.cfg, b.bc, b.name = global, bc, d.Name
		b.updateInfo()
		b.broadcast()
	})
}
func (b beggarTable) inHand(user string) bool {
	in := false
	b.DoWait(func() {
		for _, p := range b.conns {
			if strings.EqualFold(p.user, user) && p.rolling {
				in = true
			}
		}
	})
	return in
}
func (b beggarTable) inHandUsers() []string { return nil }
func (b beggarTable) players() []string {
	var out []string
	b.DoWait(func() {
		for _, p := range b.conns {
			st := "begging"
			if p.rolling {
				st = "rolling"
			}
			out = append(out, fmt.Sprintf("%-16s %s, %d chips", p.user, st, b.store.Chips(p.user)))
		}
		out = append(out, fmt.Sprintf("rolls since start: %d, odds 1 in %d", b.rolls, b.bc.Odds))
	})
	return out
}
func (b beggarTable) summary() string {
	in := b.tinfo()
	return fmt.Sprintf("%s: %d begging", b.name, in.Players)
}
