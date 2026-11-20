// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/dice"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Beggars Table: for anyone (nearly) broke with nobody to buy them back
// in. Throw the dice and make the goal for a small handout. Long odds, but
// it never runs out, so a player can always grind their way back to a table.
//
// The goal (how many dice, and what they must show) is picked from the
// table's odds setting to come as close to it as a sensible number of dice
// allows. Every throw is a 3D physics simulation run here; the faces the
// dice come to rest on decide the roll, and the client replays the recorded
// throw.

// shakeMS is the rattle in the cup before the dice leave it.
const shakeMS = 1400

type beggarCfg struct {
	Odds       int   `json:"odds"`        // aim for 1 in this many rolls winning
	Prize      int64 `json:"prize"`       // what a win pays
	MaxBalance int64 `json:"max_balance"` // you can roll with fewer chips than this; 0 = the refill amount
}

func defaultBeggar() beggarCfg {
	return beggarCfg{Odds: 1000, Prize: 50, MaxBalance: 0}
}

func (c *beggarCfg) validate() {
	d := defaultBeggar()
	if c.Odds < 1 {
		c.Odds = d.Odds
	}
	if c.Prize < 1 {
		c.Prize = d.Prize
	}
	if c.MaxBalance < 0 {
		c.MaxBalance = d.MaxBalance
	}
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
	joined   int // order of arrival, for seating
	user     string
	conn     *conn
	lastTalk time.Time
	hello    bool
	chatAt   []time.Time
}

// Beggar runs a Beggars Table.
type Beggar struct {
	id, name string
	cfg      *Config
	bc       beggarCfg
	goal     dice.Goal
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
	joinSeq  int
	cur      *proto.DiceRoll // the throw in the air, nil when the dice are still
	curAt    time.Time
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
		voice: newVoice(), conns: map[*conn]*bgPlayer{}, goal: dice.Plan(bc.Odds)}
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
		t.joinSeq++
		p := &bgPlayer{user: ev.user, conn: ev.c, joined: t.joinSeq}
		t.conns[ev.c] = p
		t.updateInfo()
		t.send(p, true)
		if t.cur != nil { // join the throw already in the air
			dr := *t.cur
			dr.Elapsed = time.Since(t.curAt).Milliseconds()
			p.conn.send(proto.SDice, dr)
		}
		if t.store.Chips(p.user) < t.limit() {
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
	if t.cur != nil {
		return "The dice are still rolling."
	}
	if chips := t.store.Chips(p.user); chips >= t.limit() {
		return fmt.Sprintf("This table is for players with fewer than %s chips. You have %s.", commas(t.limit()), commas(chips))
	}
	return ""
}

func (t *Beggar) roll(p *bgPlayer) {
	if r := t.why(p); r != "" {
		if t.cur == nil {
			t.say(banter.EvBgTooRich, p, banter.Vars{Amount: t.store.Chips(p.user)}, 0.5, prioNormal)
		}
		p.conn.send(proto.SError, proto.ErrorMsg{Message: r})
		return
	}
	t.nextID++
	t.rolls++
	g := t.goal
	th := dice.Roll(g.Dice, t.rng.Int63())
	faces := th.Faces
	win := g.Made(faces)
	n := t.store.BeggarRoll(p.user)
	if !win {
		t.store.RecordTable(p.user, t.id, 0)
	}
	ms := shakeMS + int(th.Length*1000) + 250
	dr := proto.DiceRoll{ID: t.nextID, By: p.user, Dice: g.Dice, Faces: faces, Win: win, MS: int64(ms), ShakeMS: shakeMS, Track: th.Track}
	for _, h := range th.Hits {
		dr.Hits = append(dr.Hits, int(h.T*1000))
		dr.HitsK = append(dr.HitsK, uint8(h.K*255))
	}
	t.cur, t.curAt = &dr, time.Now()
	for c := range t.conns {
		c.send(proto.SDice, dr)
	}
	switch {
	case n%100 == 0:
		t.say(banter.EvBgMany, p, banter.Vars{Amount: n}, 1, prioHigh)
	case time.Since(p.lastTalk) > 25*time.Second:
		if t.say(banter.EvBgRoll, p, banter.Vars{Amount: t.bc.Prize}, 0.15, prioLow) {
			p.lastTalk = time.Now()
		}
	}
	t.broadcast()
	id := t.nextID
	land := func() { t.landed(p, id, g, faces, win) }
	if t.instant {
		land()
		return
	}
	time.AfterFunc(time.Duration(ms)*time.Millisecond, func() { t.Do(land) })
}

// landed runs when the dice stop. A winning throw pays everyone at the
// table who is under the limit, not just whoever threw.
func (t *Beggar) landed(p *bgPlayer, id int, g dice.Goal, faces []int, win bool) {
	if t.cur == nil || t.cur.ID != id {
		return
	}
	t.cur = nil
	if win {
		what := fmt.Sprintf("%s on %d dice", strings.ToLower(g.Describe()), g.Dice)
		var paid []string
		seen := map[string]bool{}
		for _, o := range t.conns {
			key := strings.ToLower(o.user)
			if seen[key] || t.store.Chips(o.user) >= t.limit() {
				continue
			}
			seen[key] = true
			paid = append(paid, o.user)
			t.store.Handout(o.user, t.bc.Prize)
			t.store.RecordTable(o.user, t.id, t.bc.Prize)
			o.conn.event(proto.Event{Kind: "beggar_win", Seat: -1, Delta: t.bc.Prize, Text: fmt.Sprintf("WINNER! +%s CHIPS", commas(t.bc.Prize))})
			if prof, ok := t.store.Profile(o.user); ok {
				o.conn.send(proto.SProfile, prof)
			}
		}
		switch len(paid) {
		case 0:
			t.addFeed(fmt.Sprintf("%s hit it, but nobody here was broke enough to collect.", p.user))
		case 1:
			t.addFeed(fmt.Sprintf("%s hit it: +%s for %s!", p.user, commas(t.bc.Prize), paid[0]))
		default:
			t.addFeed(fmt.Sprintf("%s hit it: +%s each for %d players!", p.user, commas(t.bc.Prize), len(paid)))
		}
		t.log.Printf("[%s] %s threw %v (%s): paid %d to %s", t.id, p.user, faces, what, t.bc.Prize, strings.Join(paid, ", "))
		t.say(banter.EvBgWin, p, banter.Vars{Amount: t.bc.Prize}, 1, prioHigh)
		for c := range t.conns {
			c.event(proto.Event{Kind: "info", Seat: -1, Text: fmt.Sprintf("%s made the number at the Beggars Table!", p.user)})
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
	g := t.goal
	st := proto.BeggarState{Table: t.id, Name: t.name, Odds: t.bc.Odds, Prize: t.bc.Prize, MaxBalance: t.limit(),
		Dice: g.Dice, GoalKind: g.Kind, GoalNeed: g.Need, GoalFace: g.Face, Chance: g.P,
		Balance: t.store.Chips(p.user), Rolling: t.cur != nil, TableRolls: t.rolls, Feed: t.feed,
		ServerTime: time.Now().UnixMilli(),
		Settings: proto.Settings{ServerName: t.cfg.ServerName, RefillAmount: t.cfg.RefillAmount,
			DealerTrashTalk: t.cfg.DealerTrashTalk, DealerVulgar: t.cfg.DealerTrashTalk && t.cfg.DealerVulgar,
			ChipGifts: t.cfg.ChipGifts && t.cfg.RefillAmount > 0, TableChat: t.cfg.TableChat}}
	if t.cur != nil {
		st.Roller = t.cur.By
	}
	st.Reason = t.why(p)
	st.CanRoll = st.Reason == ""
	if prof, ok := t.store.Profile(p.user); ok {
		st.Rolls, st.Wins = prof.BeggarRolls, prof.BeggarWins
	}
	present := make([]*bgPlayer, 0, len(t.conns))
	seen := map[string]bool{}
	for _, o := range t.conns {
		present = append(present, o)
	}
	sort.Slice(present, func(i, j int) bool { return present[i].joined < present[j].joined })
	for _, o := range present {
		if key := strings.ToLower(o.user); !seen[key] {
			seen[key] = true
			st.Players = append(st.Players, o.user)
			bal := t.store.Chips(o.user)
			sv := proto.BeggarSeat{Username: o.user, Balance: bal, CanRoll: bal < t.limit(),
				Throwing: t.cur != nil && strings.EqualFold(t.cur.By, o.user)}
			if prof, ok := t.store.Profile(o.user); ok {
				sv.AvatarVersion, sv.Hat, sv.Frame, sv.Title = prof.AvatarVersion, prof.Hat, prof.Frame, prof.Title
			}
			st.Seats = append(st.Seats, sv)
		}
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
		Stakes: fmt.Sprintf("Under %s chips only", commas(t.limit()))}
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
		b.goal = dice.Plan(bc.Odds)
		b.updateInfo()
		b.broadcast()
	})
}
func (b beggarTable) inHand(user string) bool {
	in := false
	b.DoWait(func() {
		in = b.cur != nil && strings.EqualFold(b.cur.By, user)
	})
	return in
}
func (b beggarTable) inHandUsers() []string { return nil }
func (b beggarTable) players() []string {
	var out []string
	b.DoWait(func() {
		for _, p := range b.conns {
			st := "begging"
			if b.cur != nil && b.cur.By == p.user {
				st = "rolling"
			}
			out = append(out, fmt.Sprintf("%-16s %s, %d chips", p.user, st, b.store.Chips(p.user)))
		}
		out = append(out, fmt.Sprintf("rolls since start: %d, goal %s on %d dice: 1 in %.1f (set to 1 in %d)",
			b.rolls, b.goal.Describe(), b.goal.Dice, b.goal.Odds(), b.bc.Odds))
	})
	return out
}
func (b beggarTable) summary() string {
	in := b.tinfo()
	return fmt.Sprintf("%s: %d begging", b.name, in.Players)
}

// limit is the chips you have to be under to play here: the max_balance
// setting, or the daily refill amount when that's 0, so a broke player can
// keep winning the prize until they're back up to a refill's worth.
func (t *Beggar) limit() int64 {
	if t.bc.MaxBalance > 0 {
		return t.bc.MaxBalance
	}
	if t.cfg != nil && t.cfg.RefillAmount > 0 {
		return t.cfg.RefillAmount
	}
	return 50
}
