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
	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/poker"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// A casino table: everyone bets against the house on one shared deal, which
// Dale deals with the card-reveal suspense. Baccarat and Red Dog run on it;
// each supplies its rules (casinoRules) and the table does the rest: seats,
// the betting window (which waits for the first bet before its clock runs),
// paying out, the house's totals and the table's leaderboard.

const (
	csBetting = "betting"
	csDealing = "dealing"
	csRaise   = "raise" // Red Dog: double your bet or not
	csResult  = "result"
)

// casinoCfg is what both games share.
type casinoCfg struct {
	Seats          int
	MinBet, MaxBet int64
	BetSeconds     int
	RaiseSeconds   int
	BetweenSeconds int
	Commission     int // baccarat: percent taken from Banker wins
}

type baccaratCfg struct {
	Seats          int   `json:"seats"`
	MinBet         int64 `json:"min_bet"`
	MaxBet         int64 `json:"max_bet"`
	BetSeconds     int   `json:"bet_seconds"`     // betting stays open this long after the first bet
	BetweenSeconds int   `json:"between_seconds"` // the result stays up this long
	Commission     int   `json:"banker_commission"`
}

type redDogCfg struct {
	Seats          int   `json:"seats"`
	MinBet         int64 `json:"min_bet"`
	MaxBet         int64 `json:"max_bet"`
	BetSeconds     int   `json:"bet_seconds"`
	RaiseSeconds   int   `json:"raise_seconds"` // time to decide on doubling
	BetweenSeconds int   `json:"between_seconds"`
}

func defaultBaccarat() baccaratCfg {
	return baccaratCfg{Seats: 7, MinBet: 10, MaxBet: 1000, BetSeconds: 15, BetweenSeconds: 5, Commission: 5}
}

func defaultRedDog() redDogCfg {
	return redDogCfg{Seats: 7, MinBet: 10, MaxBet: 1000, BetSeconds: 15, RaiseSeconds: 10, BetweenSeconds: 5}
}

func (c casinoCfg) validated() casinoCfg {
	c.Seats = min(max(c.Seats, 1), 7)
	c.MinBet = max(c.MinBet, 1)
	c.MaxBet = max(c.MaxBet, c.MinBet)
	c.BetSeconds = min(max(c.BetSeconds, 3), 120)
	c.RaiseSeconds = min(max(c.RaiseSeconds, 3), 60)
	c.BetweenSeconds = min(max(c.BetweenSeconds, 1), 60)
	c.Commission = min(max(c.Commission, 0), 50)
	return c
}

// casinoConfig reads a table's settings over its game's defaults.
func casinoConfig(d TableDef) (casinoCfg, error) {
	var b []byte
	if len(d.Settings) > 0 {
		b, _ = json.Marshal(d.Settings)
	}
	switch d.Game {
	case proto.GameBaccarat:
		c := defaultBaccarat()
		if b != nil {
			if err := json.Unmarshal(b, &c); err != nil {
				return casinoCfg{}, err
			}
		}
		return casinoCfg{Seats: c.Seats, MinBet: c.MinBet, MaxBet: c.MaxBet, BetSeconds: c.BetSeconds,
			RaiseSeconds: 10, BetweenSeconds: c.BetweenSeconds, Commission: c.Commission}.validated(), nil
	default:
		c := defaultRedDog()
		if b != nil {
			if err := json.Unmarshal(b, &c); err != nil {
				return casinoCfg{}, err
			}
		}
		return casinoCfg{Seats: c.Seats, MinBet: c.MinBet, MaxBet: c.MaxBet, BetSeconds: c.BetSeconds,
			RaiseSeconds: c.RaiseSeconds, BetweenSeconds: c.BetweenSeconds}.validated(), nil
	}
}

// casinoRules is one game.
type casinoRules interface {
	game() string
	spots() []string
	hello() string // Dale's line when someone sits down
	// deal runs the round from "no more bets"; it ends by calling t.settle.
	deal(t *Casino)
	// payout is what a player's bets return (stake included) once the round
	// is decided, and the stats that go with it.
	payout(t *Casino, p *csPlayer) GameResult
	view(t *Casino, st *proto.CasinoState)
	reset()
}

type csPlayer struct {
	user     string
	conn     *conn
	seat     int // 1..; 0 watching
	bets     map[string]int64
	lastBets map[string]int64 // for REBET
	raised   bool
	staked   int64 // debited for this round
	result   int64
	settled  bool
	leaving  bool
	chatAt   []time.Time
}

func (p *csPlayer) total() int64 {
	var n int64
	for _, v := range p.bets {
		n += v
	}
	return n
}

// Casino runs one baccarat or Red Dog table.
type Casino struct {
	id, name string
	cfg      *Config
	cc       casinoCfg
	rules    casinoRules
	store    *Store
	notify   Notifier
	log      *log.Logger
	exec     func(string, io.Writer)
	events   chan event
	rng      *rand.Rand
	voice    *voice
	conns    map[*conn]*csPlayer
	seats    []*csPlayer // index 1..Seats; 0 unused
	phase    string
	handNo   int
	message  string
	deadline time.Time
	total    time.Duration
	timer    *time.Timer
	timerFn  func()
	reveal   *proto.CardReveal
	deck     *poker.Deck
	pending  *casinoCfg
	info     atomic.Pointer[proto.TableInfo]
	// tests: timers queue instead of running
	instant bool
	queued  func()
}

func newCasino(id, name string, rules casinoRules, cfg *Config, cc casinoCfg, store *Store, n Notifier, lg *log.Logger) *Casino {
	if n == nil {
		n = nopNotifier{}
	}
	t := &Casino{id: id, name: name, cfg: cfg, cc: cc.validated(), rules: rules, store: store, notify: n, log: lg,
		events: make(chan event, 1024), rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		voice: newVoice(), conns: map[*conn]*csPlayer{}, phase: csBetting}
	t.voice.budget = 1 << 30
	t.seats = make([]*csPlayer, t.cc.Seats+1)
	t.message = "Place your bets."
	t.updateInfo()
	return t
}

func (t *Casino) run(stop <-chan struct{}) {
	for {
		var tc <-chan time.Time
		if t.timer != nil {
			tc = t.timer.C
		}
		select {
		case <-stop:
			if t.timer != nil {
				t.timer.Stop()
			}
			return
		case ev := <-t.events:
			guard(t.log.Printf, t.rules.game()+" table", nil, func() { t.handle(ev) })
		case <-tc:
			fn := t.timerFn
			t.timer, t.timerFn = nil, nil
			if fn != nil {
				guard(t.log.Printf, t.rules.game()+" table", nil, fn)
			}
		case <-t.voice.timerC():
			if t.voice.tick() {
				t.broadcast()
			}
		}
	}
}

func (t *Casino) after(d time.Duration, fn func()) {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer, t.timerFn = nil, nil
	if t.instant {
		t.queued = fn
		return
	}
	t.timer = time.NewTimer(d)
	t.timerFn = fn
}

func (t *Casino) setDeadline(d time.Duration) { t.deadline, t.total = time.Now().Add(d), d }
func (t *Casino) clearDeadline()              { t.deadline, t.total = time.Time{}, 0 }

func (t *Casino) Do(fn func()) { t.events <- event{kind: evFunc, fn: fn} }
func (t *Casino) DoWait(fn func()) {
	done := make(chan struct{})
	t.events <- event{kind: evFunc, fn: func() { fn(); close(done) }}
	<-done
}

func (t *Casino) handle(ev event) {
	switch ev.kind {
	case evJoin:
		t.onJoin(ev.c, ev.user)
	case evLeave:
		t.onLeave(ev.c)
	case evMsg:
		p := t.conns[ev.c]
		if p == nil {
			return
		}
		switch ev.msg.Type {
		case proto.CCasino:
			var a proto.CasinoAct
			if json.Unmarshal(ev.msg.Data, &a) == nil {
				t.onAct(p, a)
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

func (t *Casino) onJoin(c *conn, user string) {
	// back from a disconnect mid-round: the seat (and bets) are still theirs
	for _, p := range t.seats {
		if p != nil && strings.EqualFold(p.user, user) {
			p.conn, p.leaving = c, false
			t.conns[c] = p
			t.updateInfo()
			t.send(p, true)
			t.broadcast()
			return
		}
	}
	p := &csPlayer{user: user, conn: c, bets: map[string]int64{}}
	t.conns[c] = p
	t.sit(p)
	t.updateInfo()
	t.send(p, true)
	if p.seat > 0 {
		t.say(t.rules.hello(), p, banter.Vars{}, 0.8, prioNormal)
	}
	t.broadcast()
}

// sit puts a player in the first free seat from the middle out.
func (t *Casino) sit(p *csPlayer) bool {
	for _, i := range seatOrder(len(t.seats) - 1) {
		if t.seats[i+1] == nil {
			t.seats[i+1], p.seat = p, i+1
			return true
		}
	}
	return false
}

func (t *Casino) stand(p *csPlayer) {
	if p.seat > 0 && t.seats[p.seat] == p {
		t.seats[p.seat] = nil
	}
	p.seat = 0
}

func (t *Casino) onLeave(c *conn) {
	p := t.conns[c]
	delete(t.conns, c)
	if p == nil {
		return
	}
	if p.staked > 0 && !p.settled {
		// money's on the table: the round settles without them
		p.conn, p.leaving = nil, true
	} else {
		t.stand(p)
	}
	t.updateInfo()
	t.broadcast()
}

func (t *Casino) errTo(p *csPlayer, msg string) {
	if p.conn != nil {
		p.conn.send(proto.SError, proto.ErrorMsg{Message: msg})
	}
}

func (t *Casino) onAct(p *csPlayer, a proto.CasinoAct) {
	switch a.Action {
	case "sit":
		if p.seat == 0 && !t.sit(p) {
			t.errTo(p, "The table is full.")
		}
	case "stand":
		if t.phase == csBetting {
			p.bets = map[string]int64{}
			t.stand(p)
		} else {
			t.errTo(p, "You can stand up between rounds.")
		}
	case "bet":
		t.bet(p, a.Spot, a.Amount)
		return
	case "clear":
		if t.phase == csBetting {
			p.bets = map[string]int64{}
		}
	case "rebet":
		if t.phase == csBetting && len(p.lastBets) > 0 {
			var need int64
			for _, v := range p.lastBets {
				need += v
			}
			if need > t.store.Balance(p.user) {
				t.errTo(p, "You don't have enough chips to repeat that bet.")
				return
			}
			p.bets = map[string]int64{}
			for k, v := range p.lastBets {
				p.bets[k] = v
			}
			t.startClock()
		}
	case "raise", "stay":
		if r, ok := t.rules.(*RedDog); ok {
			r.decide(t, p, a.Action == "raise")
		}
		return
	default:
		return
	}
	t.updateInfo()
	t.broadcast()
}

func (t *Casino) bet(p *csPlayer, spot string, amount int64) {
	if t.phase != csBetting {
		t.errTo(p, "Bets are closed for this round.")
		return
	}
	if p.seat == 0 {
		t.errTo(p, "Take a seat to bet.")
		return
	}
	ok := false
	for _, s := range t.rules.spots() {
		ok = ok || s == spot
	}
	if !ok || amount <= 0 {
		return
	}
	amount = min(amount, t.cc.MaxBet-p.bets[spot])
	if amount <= 0 {
		t.errTo(p, fmt.Sprintf("The most on one spot is %s.", commas(t.cc.MaxBet)))
		return
	}
	if p.total()+amount > t.store.Balance(p.user) {
		t.errTo(p, "You don't have that many chips.")
		return
	}
	p.bets[spot] += amount
	t.startClock()
	t.broadcast()
}

// startClock runs the betting clock once there's a bet on the table.
func (t *Casino) startClock() {
	if t.phase != csBetting || !t.deadline.IsZero() {
		return
	}
	d := time.Duration(t.cc.BetSeconds) * time.Second
	t.setDeadline(d)
	t.message = "Bets close when the clock runs out."
	t.after(d, t.closeBets)
}

// closeBets takes the bets and deals.
func (t *Casino) closeBets() {
	if t.phase != csBetting {
		return
	}
	t.clearDeadline()
	anyone := false
	for _, p := range t.seats {
		if p == nil {
			continue
		}
		p.result, p.settled, p.raised, p.staked = 0, false, false, 0
		tot := p.total()
		if tot == 0 {
			continue
		}
		if tot < t.cc.MinBet {
			t.errTo(p, fmt.Sprintf("The minimum bet is %s. Your bet came back.", commas(t.cc.MinBet)))
			p.bets = map[string]int64{}
			continue
		}
		if !t.store.Debit(p.user, tot) {
			t.errTo(p, "You don't have enough chips for that bet any more.")
			p.bets = map[string]int64{}
			continue
		}
		p.staked = tot
		p.lastBets = map[string]int64{}
		for k, v := range p.bets {
			p.lastBets[k] = v
		}
		anyone = true
	}
	if !anyone {
		t.message = "Place your bets."
		t.broadcast()
		return
	}
	t.handNo++
	t.voice.talked = 0
	t.deck = poker.NewDeck(t.rng)
	t.rules.reset()
	t.phase = csDealing
	t.message = ""
	t.eventAll(proto.Event{Kind: "deal", Seat: -1})
	t.rules.deal(t)
}

// show deals cards with the reveal suspense (when it's on): the new cards
// sit face down, flip, settle, then next runs.
func (t *Casino) show(spot string, from, n int, next func()) {
	if !t.cfg.CardReveal || t.instant {
		t.eventAll(proto.Event{Kind: "card", Seat: -1})
		t.broadcast()
		t.after(700*time.Millisecond, next)
		return
	}
	lo, hi := t.cfg.CardRevealMinSeconds, t.cfg.CardRevealMaxSeconds
	sec := lo
	if hi > lo {
		sec += t.rng.Float64() * (hi - lo)
	}
	d := time.Duration(sec * float64(time.Second))
	t.reveal = &proto.CardReveal{Spot: spot, From: from, Count: n, Until: time.Now().Add(d).UnixMilli()}
	t.eventAll(proto.Event{Kind: "card", Seat: -1})
	t.broadcast()
	t.after(d, func() {
		t.reveal.Shown = true
		t.eventAll(proto.Event{Kind: "reveal", Seat: -1})
		t.broadcast()
		t.after(revealSettle+time.Duration(n-1)*150*time.Millisecond, func() {
			t.reveal = nil
			next()
		})
	})
}

// hidden says whether card i of a spot is still face down.
func (t *Casino) hidden(spot string, i int) bool {
	r := t.reveal
	return r != nil && !r.Shown && r.Spot == spot && i >= r.From && i < r.From+r.Count
}

// settle pays everyone and records the round.
func (t *Casino) settle() {
	var house int64
	for _, p := range t.seats {
		if p == nil || p.staked == 0 {
			continue
		}
		r := t.rules.payout(t, p)
		r.Game = t.rules.game()
		t.store.SettleGame(p.user, r)
		p.result = r.Payout - r.Wagered
		p.settled = true
		house -= p.result
		t.store.RecordTable(p.user, t.id, p.result)
		if p.conn != nil {
			kind := "push"
			switch {
			case p.result > 0:
				kind = "win"
			case p.result < 0:
				kind = "lose"
			}
			p.conn.event(proto.Event{Kind: kind, Seat: p.seat, Delta: p.result})
			if prof, ok := t.store.Profile(p.user); ok {
				p.conn.send(proto.SProfile, prof)
			}
		}
	}
	if house != 0 || t.handNo > 0 {
		t.store.RecordTableHouse(house)
	}
	// Dale on the big swings
	var best, worst *csPlayer
	for _, p := range t.seats {
		if p == nil || !p.settled {
			continue
		}
		if best == nil || p.result > best.result {
			best = p
		}
		if worst == nil || p.result < worst.result {
			worst = p
		}
	}
	if _, ok := t.rules.(*Baccarat); ok {
		if best != nil && best.result >= 5*t.cc.MinBet*4 {
			t.say(banter.EvBacBigWin, best, banter.Vars{Amount: best.result}, 0.6, prioNormal)
		} else if worst != nil && -worst.result >= 5*t.cc.MinBet*4 {
			t.say(banter.EvBacBigLose, worst, banter.Vars{Amount: -worst.result}, 0.6, prioNormal)
		}
	}
	t.processUnlocks()
	t.phase = csResult
	t.broadcast()
	t.after(time.Duration(t.cc.BetweenSeconds)*time.Second, t.nextRound)
}

func (t *Casino) processUnlocks() {
	for _, u := range t.store.TakeUnlocks() {
		for c, q := range t.conns {
			if strings.EqualFold(q.user, u.User) {
				c.event(proto.Event{Kind: "unlock", Item: u.Item.ID, Seat: q.seat, Text: fmt.Sprintf("Unlocked %s: %s!", u.Item.Kind, u.Item.Name)})
			}
		}
		t.notify.Unlocked(u.User, u.Item)
	}
}

// nextRound clears the table for the next bets.
func (t *Casino) nextRound() {
	for i, p := range t.seats {
		if p == nil {
			continue
		}
		p.bets, p.staked, p.raised = map[string]int64{}, 0, false
		if p.leaving {
			t.seats[i] = nil
		}
	}
	if t.pending != nil {
		t.applyPending()
	}
	t.phase = csBetting
	t.message = "Place your bets."
	t.updateInfo()
	t.broadcast()
}

func (t *Casino) applyPending() {
	cc := *t.pending
	t.pending = nil
	if cc.Seats != t.cc.Seats {
		seats := make([]*csPlayer, cc.Seats+1)
		var spill []*csPlayer
		for i, p := range t.seats {
			if p == nil {
				continue
			}
			if i < len(seats) {
				seats[i] = p
			} else {
				spill = append(spill, p)
			}
		}
		t.seats = seats
		for _, p := range spill {
			p.seat = 0
			if !t.sit(p) {
				t.errTo(p, "The table got smaller and you lost your seat.")
			}
		}
	}
	t.cc = cc
}

func (t *Casino) eventAll(e proto.Event) {
	for c := range t.conns {
		c.event(e)
	}
}

func (t *Casino) say(ev string, p *csPlayer, v banter.Vars, chance float64, prio int) bool {
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

func (t *Casino) onChat(p *csPlayer, text string) {
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
			t.errTo(p, "Only admins can use / commands.")
			return
		}
		runAdminCommand(t.log, t.exec, p.conn, p.user, text)
		return
	}
	if !t.cfg.TableChat {
		t.errTo(p, "Chat is turned off on this server.")
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
		t.errTo(p, "Slow down a little.")
		return
	}
	p.chatAt = append(p.chatAt, now)
	if len(text) > 80 {
		text = text[:80]
	}
	seat := p.seat
	if seat == 0 {
		seat = -1
	}
	msg := proto.Chat{From: p.user, Seat: seat, Text: text, Admin: t.store.IsAdmin(p.user), At: now.UnixMilli()}
	for c := range t.conns {
		c.send(proto.SChat, msg)
	}
}

func (t *Casino) view(viewer *csPlayer, withLog bool) proto.CasinoState {
	st := proto.CasinoState{Table: t.id, Name: t.name, Game: t.rules.game(), Phase: t.phase, HandNumber: t.handNo,
		Message: t.message, ServerTime: time.Now().UnixMilli(), MinBet: t.cc.MinBet, MaxBet: t.cc.MaxBet,
		YourSeat: -1, Spots: t.rules.spots(), Reveal: t.reveal,
		Settings: proto.Settings{ServerName: t.cfg.ServerName, RefillAmount: t.cfg.RefillAmount,
			DealerTrashTalk: t.cfg.DealerTrashTalk, DealerVulgar: t.cfg.DealerTrashTalk && t.cfg.DealerVulgar,
			ChipGifts: t.cfg.ChipGifts && t.cfg.RefillAmount > 0, TableChat: t.cfg.TableChat, CardReveal: t.cfg.CardReveal}}
	if !t.deadline.IsZero() {
		st.Deadline, st.TimerTotal = t.deadline.UnixMilli(), t.total.Milliseconds()
	}
	if viewer.seat > 0 {
		st.YourSeat = viewer.seat
	}
	for i := 1; i < len(t.seats); i++ {
		sv := proto.CasinoSeat{Seat: i}
		if p := t.seats[i]; p != nil {
			sv.Username, sv.Connected = p.user, p.conn != nil
			if prof, ok := t.store.Profile(p.user); ok {
				sv.AvatarVersion, sv.Hat, sv.Frame, sv.Title = prof.AvatarVersion, prof.Hat, prof.Frame, prof.Title
			}
			sv.Balance = t.store.Balance(p.user)
			if p.staked == 0 {
				sv.Balance -= p.total()
			}
			if len(p.bets) > 0 {
				sv.Bets = map[string]int64{}
				for k, v := range p.bets {
					sv.Bets[k] = v
				}
			}
			sv.Raised, sv.Result, sv.Settled = p.raised, p.result, p.settled
		}
		st.Seats = append(st.Seats, sv)
	}
	for _, p := range t.conns {
		if p.seat == 0 {
			st.Spectators++
		}
	}
	st.CanBet = t.phase == csBetting && viewer.seat > 0
	st.CanRebet = st.CanBet && len(viewer.lastBets) > 0 && len(viewer.bets) == 0
	t.rules.view(t, &st)
	if r, ok := t.rules.(*RedDog); ok {
		st.CanRaise = t.phase == csRaise && viewer.staked > 0 && !viewer.raised && !r.decided[viewer]
	}
	house := t.store.House()
	st.HouseNet, st.HouseTier = house.Net, house.Tier
	var ps proto.PokerState
	t.voice.fill(&ps, withLog)
	st.DealerSay, st.DealerSayID, st.DealerSayUntil = ps.DealerSay, ps.DealerSayID, ps.DealerSayUntil
	st.DealerSayCont, st.DealerSayPara, st.DealerLog = ps.DealerSayCont, ps.DealerSayPara, ps.DealerLog
	return st
}

func (t *Casino) send(p *csPlayer, withLog bool) {
	if p.conn != nil {
		p.conn.send(proto.SCasino, t.view(p, withLog))
	}
}

func (t *Casino) broadcast() {
	for _, p := range t.conns {
		t.send(p, false)
	}
	t.updateInfo()
}

func (t *Casino) updateInfo() {
	in := proto.TableInfo{ID: t.id, Name: t.name, Game: t.rules.game(), Seats: len(t.seats) - 1,
		Stakes: fmt.Sprintf("Bets %s-%s", commas(t.cc.MinBet), commas(t.cc.MaxBet))}
	for _, p := range t.seats {
		if p != nil {
			in.Players++
			in.Names = append(in.Names, p.user)
		}
	}
	for _, p := range t.conns {
		if p.seat == 0 {
			in.Watching++
		}
	}
	t.info.Store(&in)
}

// draw takes a card.
func (t *Casino) draw() bj.Card { return t.deck.Draw() }

// ---------------------------------------------------------------- lobby

type casinoTable struct{ *Casino }

func (c casinoTable) info() proto.TableInfo {
	if in := c.Casino.info.Load(); in != nil {
		return *in
	}
	return proto.TableInfo{ID: c.id, Name: c.name, Game: c.rules.game()}
}
func (c casinoTable) join(cn *conn, user string, _ int64) {
	c.events <- event{kind: evJoin, c: cn, user: user}
}
func (c casinoTable) leave(cn *conn)            { c.events <- event{kind: evLeave, c: cn} }
func (c casinoTable) msg(cn *conn, m proto.Msg) { c.events <- event{kind: evMsg, c: cn, msg: m} }
func (c casinoTable) broadcastAll()             { c.Do(c.broadcast) }
func (c casinoTable) close(reason string) {
	c.DoWait(func() {
		// a round in progress is called off: stakes go back
		for _, p := range c.seats {
			if p != nil && p.staked > 0 && !p.settled {
				c.store.Credit(p.user, p.staked)
				p.staked = 0
			}
		}
		for cn := range c.conns {
			cn.send(proto.SLeft, proto.ErrorMsg{Message: reason})
			cn.event(proto.Event{Kind: "info", Text: reason, Seat: -1})
		}
		c.conns = map[*conn]*csPlayer{}
		if c.timer != nil {
			c.timer.Stop()
		}
	})
}
func (c casinoTable) reconfigure(global *Config, d TableDef) {
	cc, err := casinoConfig(d)
	if err != nil {
		return
	}
	c.Do(func() {
		c.cfg, c.name = global, d.Name
		if c.phase == csBetting && c.deadline.IsZero() {
			c.pending = &cc
			c.applyPending()
		} else {
			c.pending = &cc
		}
		c.updateInfo()
		c.broadcast()
	})
}
func (c casinoTable) inHand(user string) bool {
	in := false
	c.DoWait(func() {
		for _, p := range c.seats {
			if p != nil && strings.EqualFold(p.user, user) && p.staked > 0 && !p.settled {
				in = true
			}
		}
	})
	return in
}
func (c casinoTable) inHandUsers() []string {
	var out []string
	c.DoWait(func() {
		for _, p := range c.seats {
			if p != nil && p.staked > 0 && !p.settled {
				out = append(out, p.user)
			}
		}
	})
	return out
}
func (c casinoTable) players() []string {
	var out []string
	c.DoWait(func() {
		for _, p := range c.seats {
			if p != nil {
				out = append(out, fmt.Sprintf("%-16s seat %d, bet %d", p.user, p.seat, p.total()))
			}
		}
	})
	return out
}
func (c casinoTable) summary() string {
	in := c.info()
	return fmt.Sprintf("%s: %d/%d seated, hand %d", c.name, in.Players, in.Seats, c.handNo)
}
