// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"time"
	"unicode"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/poker"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// No-limit Texas Hold'em. Seat 0 is Dale's: he plays from the dealer's spot
// as a real opponent, and what he wins or loses goes into the house's
// totals. Players buy in from their balance; their stack is saved after
// every hand and goes back to their balance when they leave (or, after a
// crash, when the server starts).

// holdemCfg is a Hold'em table's settings.
type holdemCfg struct {
	Seats          int   `json:"seats"` // for players (Dale has his own)
	SmallBlind     int64 `json:"small_blind"`
	BigBlind       int64 `json:"big_blind"`
	MinBuyIn       int64 `json:"min_buy_in"`
	MaxBuyIn       int64 `json:"max_buy_in"`
	ActionSeconds  int   `json:"action_seconds"`   // time to act
	BetweenSeconds int   `json:"between_seconds"`  // pause between hands
	DealerSoloOnly bool  `json:"dealer_solo_only"` // Dale only plays someone who's alone
	DaleThinkMS    int   `json:"dale_think_ms"`    // how long he takes to act
}

func defaultHoldem() holdemCfg {
	return holdemCfg{Seats: 6, SmallBlind: 5, BigBlind: 10, MinBuyIn: 200, MaxBuyIn: 1000,
		ActionSeconds: 25, BetweenSeconds: 5, DaleThinkMS: 1600}
}

func (h *holdemCfg) validate() {
	h.Seats = min(max(h.Seats, 1), 9)
	h.BigBlind = max(h.BigBlind, 2)
	if h.SmallBlind <= 0 || h.SmallBlind > h.BigBlind {
		h.SmallBlind = max(1, h.BigBlind/2)
	}
	h.MinBuyIn = max(h.MinBuyIn, h.BigBlind*10)
	h.MaxBuyIn = max(h.MaxBuyIn, h.MinBuyIn)
	h.ActionSeconds = min(max(h.ActionSeconds, 5), 300)
	h.BetweenSeconds = min(max(h.BetweenSeconds, 2), 60)
	h.DaleThinkMS = min(max(h.DaleThinkMS, 0), 10000)
}

// Hold'em phases.
const (
	pkWaiting  = "waiting"
	pkPreflop  = "preflop"
	pkFlop     = "flop"
	pkTurn     = "turn"
	pkRiver    = "river"
	pkShowdown = "showdown"
)

type hPlayer struct {
	user     string
	conn     *conn
	seat     int // -1 = watching
	dale     bool
	stack    int64
	bet      int64 // this betting round
	put      int64 // this hand
	hole     []poker.Card
	inHand   bool // dealt into this hand
	folded   bool
	allIn    bool
	acted    bool // has acted since the last full raise
	canRais  bool // may raise when it's their turn
	sitOut   bool
	away     int // missed turns in a row
	leaving  bool
	action   string // last action, shown by the seat
	show     bool   // cards face up for everyone
	result   string // hand name at the showdown
	best     []poker.Card
	won      int64
	needLog  bool
	chatAt   []time.Time
	dbgBluff bool // Dale: his last bet was a bluff
}

// Holdem is one Hold'em table.
type Holdem struct {
	id, name string
	cfg      *Config
	hc       holdemCfg
	store    *Store
	notify   Notifier
	log      *log.Logger
	exec     func(string, io.Writer)
	events   chan event
	rng      *rand.Rand
	voice    *voice

	seats  []*hPlayer // [0] is Dale
	conns  map[*conn]*hPlayer
	byUser map[string]*hPlayer

	phase    string
	handNo   int
	button   int
	active   int // seat to act, -1 = nobody
	deck     *poker.Deck
	board    []poker.Card
	curBet   int64 // biggest bet this round
	minRaise int64 // smallest raise increment
	lastAgg  int   // last player to bet or raise this round
	message  string
	pots     []int64

	timer      *time.Timer
	timerFn    func()
	deadline   time.Time
	timerTotal time.Duration
	pending    *holdemCfg // settings change waiting for the hand to end
	infoBox    infoBox
	instant    bool   // tests: timers don't wait, the test runs them
	queued     func() // tests: the timer that would have fired
}

func newHoldem(id, name string, cfg *Config, hc holdemCfg, store *Store, n Notifier, lg *log.Logger) *Holdem {
	hc.validate()
	if n == nil {
		n = nopNotifier{}
	}
	t := &Holdem{id: id, name: name, cfg: cfg, hc: hc, store: store, notify: n, log: lg,
		events: make(chan event, 1024), rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		voice: newVoice(), conns: map[*conn]*hPlayer{}, byUser: map[string]*hPlayer{},
		phase: pkWaiting, active: -1, button: 0}
	t.seats = make([]*hPlayer, hc.Seats+1)
	t.seats[0] = &hPlayer{user: "Dale", seat: 0, dale: true}
	t.updateInfo()
	return t
}

// ---------------------------------------------------------------- loop

func (t *Holdem) run(stop <-chan struct{}) {
	for {
		var tc <-chan time.Time
		if t.timer != nil {
			tc = t.timer.C
		}
		select {
		case <-stop:
			t.stopTimer()
			return
		case ev := <-t.events:
			t.safely("handling a player action", func() { t.handle(ev) })
		case <-tc:
			fn := t.timerFn
			t.timer, t.timerFn = nil, nil
			if fn != nil {
				t.safely("the table timer", fn)
			}
		case <-t.voice.timerC():
			if t.voice.tick() {
				t.broadcast()
			}
		}
	}
}

// safely runs f; a crash mid-hand calls the hand off and gives every chip
// back.
func (t *Holdem) safely(what string, f func()) {
	guard(t.log.Printf, what, func() { t.abortHand() }, f)
}

func (t *Holdem) abortHand() {
	if t.phase == pkWaiting {
		return
	}
	t.log.Printf("[%s] calling off hand #%d after a crash", t.id, t.handNo)
	for _, p := range t.seats {
		if p != nil && p.inHand {
			p.stack += p.put
			p.put, p.bet = 0, 0
			if !p.dale {
				t.store.SetTableStack(p.user, p.stack)
			}
		}
	}
	t.endHandCleanup()
	t.message = "That hand was called off. Everyone got their chips back."
	t.phase = pkWaiting
	t.after(time.Duration(t.hc.BetweenSeconds)*time.Second, t.maybeStart)
	t.broadcast()
}

func (t *Holdem) after(d time.Duration, fn func()) {
	t.stopTimer()
	if t.instant {
		t.queued = fn
		return
	}
	t.timer = time.NewTimer(d)
	t.timerFn = fn
}

func (t *Holdem) stopTimer() {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer, t.timerFn = nil, nil
	t.queued = nil
}

func (t *Holdem) setDeadline(d time.Duration) {
	t.deadline = time.Now().Add(d)
	t.timerTotal = d
}

func (t *Holdem) clearDeadline() { t.deadline, t.timerTotal = time.Time{}, 0 }

// Do runs fn on the table goroutine.
func (t *Holdem) Do(fn func()) { t.events <- event{kind: evFunc, fn: fn} }

// DoWait runs fn on the table goroutine and waits.
func (t *Holdem) DoWait(fn func()) {
	done := make(chan struct{})
	t.events <- event{kind: evFunc, fn: func() { fn(); close(done) }}
	<-done
}

func (t *Holdem) handle(ev event) {
	switch ev.kind {
	case evJoin:
		t.onJoin(ev.c, ev.user, ev.buyIn)
	case evLeave:
		t.onLeave(ev.c)
	case evMsg:
		if p := t.conns[ev.c]; p != nil {
			t.onMessage(p, ev.msg)
		}
	case evFunc:
		ev.fn()
	}
}

// ---------------------------------------------------------------- seating

func (t *Holdem) humans() []*hPlayer {
	var out []*hPlayer
	for _, p := range t.seats[1:] {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}

// playing reports whether someone will be dealt in the next hand.
func (t *Holdem) playing(p *hPlayer) bool {
	return p != nil && !p.dale && p.stack > 0 && !p.sitOut && !p.leaving
}

// daleIn reports whether Dale plays the next hand.
func (t *Holdem) daleIn() bool {
	n := 0
	for _, p := range t.humans() {
		if t.playing(p) {
			n++
		}
	}
	if n == 0 {
		return false
	}
	return !t.hc.DealerSoloOnly || n == 1
}

func (t *Holdem) onJoin(c *conn, user string, buyIn int64) {
	key := strings.ToLower(user)
	p := t.byUser[key]
	if p != nil {
		if p.conn != nil && p.conn != c {
			delete(t.conns, p.conn)
		}
		p.conn = c
		p.leaving = false
	} else {
		p = &hPlayer{user: user, conn: c, seat: -1}
		t.byUser[key] = p
	}
	p.needLog = true
	t.conns[c] = p
	if p.seat < 0 && buyIn > 0 {
		t.sitDown(p, buyIn)
	}
	t.log.Printf("[%s] %s joined (seat %d)", t.id, user, p.seat)
	if p.seat > 0 {
		t.say(banter.EvPkHello, p, banter.Vars{}, 0.9, prioNormal)
	}
	t.maybeStart()
	t.broadcast()
	t.updateInfo()
}

// sitDown seats a watcher with a buy-in (or with chips already at the
// table, after a reconnect).
func (t *Holdem) sitDown(p *hPlayer, buyIn int64) bool {
	free := -1
	for i := 1; i < len(t.seats); i++ {
		if t.seats[i] == nil {
			free = i
			break
		}
	}
	if free < 0 {
		t.errTo(p, "The table is full. You can watch until a seat opens.")
		return false
	}
	have, at := t.store.TableStack(p.user)
	if have > 0 && at == t.id {
		p.stack = have // chips still here from before (a reconnect)
	} else {
		buyIn = min(max(buyIn, t.hc.MinBuyIn), t.hc.MaxBuyIn)
		if err := t.store.BuyIn(p.user, t.id, buyIn); err != nil {
			if err == ErrNotEnough {
				bal := t.store.Balance(p.user)
				if bal >= t.hc.MinBuyIn {
					buyIn = min(bal, t.hc.MaxBuyIn)
					err = t.store.BuyIn(p.user, t.id, buyIn)
				}
			}
			if err != nil {
				t.errTo(p, fmt.Sprintf("You need at least %s chips to sit here (%s).", commas(t.hc.MinBuyIn), err.Error()))
				return false
			}
		}
		p.stack = buyIn
	}
	p.seat = free
	t.seats[free] = p
	p.sitOut, p.away = false, 0
	t.sendProfile(p)
	return true
}

// standUp takes a player out of their seat and cashes them out. If they're
// in the hand they fold first.
func (t *Holdem) standUp(p *hPlayer) {
	if p.seat <= 0 {
		return
	}
	if p.inHand && !p.folded && t.phase != pkShowdown && t.phase != pkWaiting {
		p.leaving = true
		t.fold(p, "left")
		return // finishHand takes them out
	}
	if p.inHand && t.phase != pkWaiting {
		p.leaving = true // folded already: out at the end of the hand
		return
	}
	t.seats[p.seat] = nil
	p.seat = -1
	p.stack = 0
	got := t.store.CashOut(p.user)
	if got > 0 && p.conn != nil {
		p.conn.event(proto.Event{Kind: "info", Text: fmt.Sprintf("Cashed out %s chips.", commas(got)), Seat: -1})
	}
	t.sendProfile(p)
	t.updateInfo()
}

func (t *Holdem) onLeave(c *conn) {
	p := t.conns[c]
	delete(t.conns, c)
	if p == nil || p.conn != c {
		return
	}
	p.conn = nil
	t.log.Printf("[%s] %s left", t.id, p.user)
	if p.seat > 0 {
		t.say(banter.EvPkLeave, p, banter.Vars{Amount: p.stack}, 0.5, prioNormal)
		t.standUp(p)
	}
	if p.seat <= 0 {
		delete(t.byUser, strings.ToLower(p.user))
	}
	if t.active == p.seat && p.seat > 0 {
		// it was their turn: standUp folded them
	}
	t.broadcast()
	t.updateInfo()
}

// ---------------------------------------------------------------- messages

func (t *Holdem) onMessage(p *hPlayer, m proto.Msg) {
	switch m.Type {
	case proto.CPoker:
		var a proto.PokerAct
		if decode(m, &a) == nil {
			t.onAct(p, strings.ToLower(a.Action), a.Amount)
		}
	case proto.CChat:
		var c proto.Chat
		if decode(m, &c) == nil {
			t.onChat(p, c.Text)
		}
	}
}

func (t *Holdem) onAct(p *hPlayer, act string, amount int64) {
	switch act {
	case "sit", "buyin":
		if p.seat <= 0 {
			t.sitDown(p, amount)
			t.maybeStart()
			t.broadcast()
			t.updateInfo()
		}
		return
	case "stand", "leave":
		t.standUp(p)
		t.broadcast()
		return
	case "sitout":
		p.sitOut = true
		t.broadcast()
		return
	case "sitin":
		p.sitOut, p.away = false, 0
		t.maybeStart()
		t.broadcast()
		return
	case "rebuy":
		t.rebuy(p, amount)
		return
	case "show":
		if t.phase == pkShowdown && p.inHand {
			p.show = true
			t.broadcast()
		}
		return
	}
	if p.seat != t.active || p.seat <= 0 || t.phase == pkWaiting || t.phase == pkShowdown {
		return
	}
	p.away = 0
	t.apply(p, act, amount)
}

// rebuy adds chips between hands, up to the maximum buy-in.
func (t *Holdem) rebuy(p *hPlayer, amount int64) {
	if p.seat <= 0 {
		return
	}
	if p.inHand && t.phase != pkWaiting && t.phase != pkShowdown {
		t.errTo(p, "You can add chips between hands.")
		return
	}
	room := t.hc.MaxBuyIn - p.stack
	if room <= 0 {
		t.errTo(p, "You're already at the maximum buy-in.")
		return
	}
	amount = min(max(amount, 1), room)
	if err := t.store.BuyIn(p.user, t.id, amount); err != nil {
		t.errTo(p, err.Error())
		return
	}
	p.stack += amount
	t.say(banter.EvPkRebuy, p, banter.Vars{Amount: amount}, 0.8, prioNormal)
	t.sendProfile(p)
	t.maybeStart()
	t.broadcast()
}

func (t *Holdem) onChat(p *hPlayer, text string) {
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if text == "" || p.conn == nil {
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
	keep := p.chatAt[:0]
	for _, at := range p.chatAt {
		if now.Sub(at) < 10*time.Second {
			keep = append(keep, at)
		}
	}
	p.chatAt = keep
	if len(p.chatAt) >= 4 {
		t.errTo(p, "Slow down a little.")
		return
	}
	p.chatAt = append(p.chatAt, now)
	if r := []rune(text); len(r) > ChatMaxLen {
		text = string(r[:ChatMaxLen])
	}
	msg := proto.Chat{From: p.user, Seat: p.seat, Text: text, Admin: t.store.IsAdmin(p.user), At: now.UnixMilli()}
	for c := range t.conns {
		c.send(proto.SChat, msg)
	}
}

func (t *Holdem) errTo(p *hPlayer, msg string) {
	if p.conn != nil {
		p.conn.send(proto.SError, proto.ErrorMsg{Message: msg})
	}
}

func (t *Holdem) sendProfile(p *hPlayer) {
	if p.conn == nil || p.dale {
		return
	}
	if prof, ok := t.store.Profile(p.user); ok {
		p.conn.send(proto.SProfile, prof)
	}
}

func (t *Holdem) eventAll(e proto.Event) {
	for c := range t.conns {
		c.event(e)
	}
}

// ---------------------------------------------------------------- the hand

// inOrder lists seats clockwise starting after seat `from`.
func (t *Holdem) inOrder(from int, pred func(*hPlayer) bool) []*hPlayer {
	var out []*hPlayer
	n := len(t.seats)
	for k := 1; k <= n; k++ {
		p := t.seats[(from+k)%n]
		if p != nil && pred(p) {
			out = append(out, p)
		}
	}
	return out
}

func (t *Holdem) maybeStart() {
	if t.phase != pkWaiting || t.timer != nil || t.queued != nil {
		return
	}
	if t.pending != nil {
		t.applyPending()
	}
	n := 0
	for _, p := range t.humans() {
		if t.playing(p) {
			n++
		}
	}
	if t.daleIn() {
		n++
	}
	if n < 2 {
		t.message = "Waiting for players..."
		if len(t.humans()) > 0 && !t.daleIn() && t.hc.DealerSoloOnly {
			t.message = "Waiting for another player..."
		}
		return
	}
	t.message = ""
	t.after(1200*time.Millisecond, t.startHand)
}

func (t *Holdem) startHand() {
	if t.phase != pkWaiting {
		return
	}
	dale := t.seats[0]
	daleIn := t.daleIn()
	if daleIn && dale.stack < t.hc.MaxBuyIn/2 {
		dale.stack = t.hc.MaxBuyIn // the house always has more chips
	}
	in := func(p *hPlayer) bool {
		if p.dale {
			return daleIn
		}
		return t.playing(p)
	}
	players := t.inOrder(t.button, in)
	if len(players) < 2 {
		t.maybeStart()
		return
	}
	t.handNo++
	t.voice.talked = 0
	for _, p := range t.seats {
		if p == nil {
			continue
		}
		p.bet, p.put, p.hole, p.folded, p.allIn, p.acted, p.show = 0, 0, nil, false, false, false, false
		p.action, p.result, p.best, p.won, p.inHand = "", "", nil, 0, false
	}
	for _, p := range players {
		p.inHand = true
	}
	// move the button to the next player in the hand
	t.button = players[0].seat
	t.deck = poker.NewDeck(t.rng)
	t.board = nil
	t.pots = nil
	t.message = ""
	order := t.inOrder(t.button-1, func(p *hPlayer) bool { return p.inHand }) // button first
	var sb, bb *hPlayer
	if len(order) == 2 {
		sb, bb = order[0], order[1] // heads up: the button posts the small blind
	} else {
		sb, bb = order[1], order[2]
	}
	t.post(sb, t.hc.SmallBlind, "small blind")
	t.post(bb, t.hc.BigBlind, "big blind")
	t.curBet = t.hc.BigBlind
	t.minRaise = t.hc.BigBlind
	t.lastAgg = bb.seat
	for k := 0; k < 2; k++ {
		for _, p := range order {
			p.hole = append(p.hole, t.deck.Draw())
		}
	}
	t.phase = pkPreflop
	t.eventAll(proto.Event{Kind: "deal", Seat: -1})
	for _, p := range order {
		p.acted = false
		p.canRais = true
	}
	t.log.Printf("[%s] hand #%d: %d players, button seat %d", t.id, t.handNo, len(order), t.button)
	if daleIn && len(order) == 2 && t.handNo%7 == 1 {
		t.say(banter.EvPkSolo, t.humanIn(), banter.Vars{}, 0.8, prioNormal)
	}
	// first to act: after the big blind
	t.nextTurn(bb.seat)
}

func (t *Holdem) humanIn() *hPlayer {
	for _, p := range t.humans() {
		if p.inHand {
			return p
		}
	}
	return nil
}

func (t *Holdem) post(p *hPlayer, amt int64, what string) {
	amt = min(amt, p.stack)
	p.stack -= amt
	p.bet += amt
	p.put += amt
	p.action = what
	if p.stack == 0 {
		p.allIn = true
	}
}

// live is everyone still in the hand.
func (t *Holdem) live() []*hPlayer {
	return t.inOrder(t.button, func(p *hPlayer) bool { return p.inHand && !p.folded })
}

// canAct is everyone who still has decisions to make.
func (t *Holdem) canAct() []*hPlayer {
	return t.inOrder(t.button, func(p *hPlayer) bool { return p.inHand && !p.folded && !p.allIn })
}

func (t *Holdem) pot() int64 {
	var n int64
	for _, p := range t.seats {
		if p != nil {
			n += p.put
		}
	}
	return n
}

// nextTurn passes the action to the next player after seat `after` who
// still needs to act, or ends the betting round.
func (t *Holdem) nextTurn(after int) {
	if len(t.live()) == 1 {
		t.winByFold()
		return
	}
	for _, p := range t.inOrder(after, func(p *hPlayer) bool { return p.inHand && !p.folded && !p.allIn }) {
		if !p.acted || p.bet < t.curBet {
			// the last player who can act doesn't need to when nobody can
			// call them
			if len(t.canAct()) == 1 && p.bet >= t.curBet {
				break
			}
			t.giveTurn(p)
			return
		}
	}
	t.endRound()
}

func (t *Holdem) giveTurn(p *hPlayer) {
	t.active = p.seat
	if p.dale {
		t.clearDeadline()
		think := time.Duration(t.hc.DaleThinkMS) * time.Millisecond
		think += time.Duration(t.rng.Intn(max(1, t.hc.DaleThinkMS))) * time.Millisecond / 2
		t.after(think, func() { t.daleAct(p) })
		t.broadcast()
		return
	}
	d := time.Duration(t.hc.ActionSeconds) * time.Second
	if p.conn == nil || p.sitOut {
		d = 1500 * time.Millisecond // gone: don't make everyone wait
	}
	t.setDeadline(d)
	t.after(d, func() { t.timeout(p) })
	if p.conn != nil {
		p.conn.event(proto.Event{Kind: "turn", Seat: p.seat, Text: "Your turn"})
	}
	t.broadcast()
	if d > 10*time.Second && !t.instant {
		// nag them if they're slow
		seat, hand := p.seat, t.handNo
		go func() {
			time.Sleep(d * 3 / 5)
			t.Do(func() {
				if t.active == seat && t.handNo == hand && t.seats[seat] == p {
					t.say(banter.EvPkSlow, p, banter.Vars{}, 0.6, prioNormal)
				}
			})
		}()
	}
}

func (t *Holdem) timeout(p *hPlayer) {
	if t.active != p.seat {
		return
	}
	p.away++
	if p.away >= 2 && p.conn != nil {
		p.sitOut = true
		t.errTo(p, "You've been sat out for missing your turn. Press SIT IN to play again.")
	}
	if p.bet >= t.curBet {
		t.apply(p, "check", 0)
	} else {
		t.apply(p, "fold", 0)
	}
}

// apply carries out an action for the player whose turn it is.
func (t *Holdem) apply(p *hPlayer, act string, amount int64) {
	toCall := t.curBet - p.bet
	switch act {
	case "fold":
		t.fold(p, "fold")
		return
	case "check":
		if toCall > 0 {
			t.errTo(p, fmt.Sprintf("You can't check: it's %s to call.", commas(toCall)))
			return
		}
		p.action = "check"
	case "call":
		if toCall <= 0 {
			p.action = "check"
			break
		}
		amt := min(toCall, p.stack)
		t.move(p, amt)
		p.action = "call"
		if p.allIn {
			p.action = "all in"
		}
	case "bet", "raise", "allin":
		target := amount
		if act == "allin" {
			target = p.bet + p.stack
		}
		maxTotal := p.bet + p.stack
		target = min(target, maxTotal)
		minTotal := t.curBet + t.minRaise
		if target <= t.curBet {
			// not actually a raise: treat as a call
			t.apply(p, "call", 0)
			return
		}
		if !p.canRais {
			t.errTo(p, "You can only call or fold: nobody has re-raised since you acted.")
			return
		}
		if target < minTotal && target < maxTotal {
			t.errTo(p, fmt.Sprintf("The smallest raise is to %s.", commas(minTotal)))
			return
		}
		raise := target - t.curBet
		t.move(p, target-p.bet)
		full := raise >= t.minRaise
		if full {
			t.minRaise = raise
		}
		wasBet := t.curBet == 0
		t.curBet = target
		t.lastAgg = p.seat
		for _, q := range t.canAct() {
			if q != p {
				q.acted = false
				if full {
					q.canRais = true
				}
			}
		}
		switch {
		case p.allIn:
			p.action = "all in"
		case wasBet:
			p.action = "bet " + commas(target)
		default:
			p.action = "raise to " + commas(target)
		}
		t.reactToBet(p, target, p.allIn)
	default:
		return
	}
	p.acted = true
	p.canRais = false
	if act == "check" || act == "call" {
		t.reactToCall(p, act, toCall)
	}
	t.stopTimer()
	t.nextTurn(p.seat)
}

func (t *Holdem) move(p *hPlayer, amt int64) {
	amt = min(amt, p.stack)
	p.stack -= amt
	p.bet += amt
	p.put += amt
	if p.stack == 0 {
		p.allIn = true
	}
}

func (t *Holdem) fold(p *hPlayer, why string) {
	p.folded = true
	p.action = "fold"
	if why == "left" {
		p.action = "left"
	}
	t.reactToFold(p)
	if t.active == p.seat {
		t.stopTimer()
		t.nextTurn(p.seat)
		return
	}
	if len(t.live()) == 1 {
		t.winByFold()
		return
	}
	t.broadcast()
}

// endRound closes a betting round: deal the next street, or the showdown.
func (t *Holdem) endRound() {
	t.active = -1
	t.clearDeadline()
	for _, p := range t.seats {
		if p != nil {
			p.bet = 0
			if p.inHand && !p.folded && !p.allIn {
				p.acted, p.canRais = false, true
				if p.action != "fold" {
					p.action = ""
				}
			}
		}
	}
	t.curBet, t.minRaise = 0, t.hc.BigBlind
	// nobody (or one player) left with decisions: run the board out
	runOut := len(t.canAct()) <= 1
	if runOut {
		for _, p := range t.live() {
			p.show = true
		}
	}
	var next string
	switch t.phase {
	case pkPreflop:
		next = pkFlop
		t.board = append(t.board, t.deck.Draw(), t.deck.Draw(), t.deck.Draw())
		t.reactToFlop()
	case pkFlop:
		next = pkTurn
		t.board = append(t.board, t.deck.Draw())
	case pkTurn:
		next = pkRiver
		t.board = append(t.board, t.deck.Draw())
	default:
		t.showdown()
		return
	}
	t.phase = next
	t.eventAll(proto.Event{Kind: "card", Seat: -1})
	t.broadcast()
	pause := 900 * time.Millisecond
	if runOut {
		pause = 1800 * time.Millisecond // let the drama play out
	}
	t.after(pause, func() {
		if runOut {
			t.endRound()
			return
		}
		t.nextTurn(t.button)
	})
}

// returnUncalled gives back the part of the biggest bet nobody matched.
func (t *Holdem) returnUncalled() {
	var top, second *hPlayer
	for _, p := range t.seats {
		if p == nil || p.put == 0 {
			continue
		}
		switch {
		case top == nil || p.put > top.put:
			second, top = top, p
		case second == nil || p.put > second.put:
			second = p
		}
	}
	if top == nil {
		return
	}
	var sec int64
	if second != nil {
		sec = second.put
	}
	if extra := top.put - sec; extra > 0 {
		top.put -= extra
		top.stack += extra
		if top.stack > 0 {
			top.allIn = false
		}
	}
}

func (t *Holdem) winByFold() {
	t.stopTimer()
	t.active = -1
	t.clearDeadline()
	t.returnUncalled()
	w := t.live()[0]
	pot := t.pot()
	w.won = pot
	w.stack += pot
	t.message = fmt.Sprintf("%s wins %s", t.display(w), commas(pot))
	t.phase = pkShowdown
	// a bluff: won without a showdown holding nothing
	bluff := false
	if len(w.hole) == 2 {
		if sc, _ := poker.Best(append(append([]poker.Card{}, w.hole...), t.board...)); len(t.board) >= 3 && sc.Category() == poker.HighCard {
			bluff = true
		} else if len(t.board) == 0 && !strongPreflop(w.hole) && t.pot() > t.hc.BigBlind*6 {
			bluff = true
		}
	}
	t.finishHand(map[*hPlayer]bool{w: bluff}, nil, false)
}

func strongPreflop(h []poker.Card) bool {
	a, b := pkRank(h[0]), pkRank(h[1])
	return a == b || a+b >= 21
}

func pkRank(c poker.Card) int {
	if c.Rank == 1 {
		return 14
	}
	return int(c.Rank)
}

func (t *Holdem) showdown() {
	t.active = -1
	t.clearDeadline()
	t.returnUncalled()
	t.phase = pkShowdown
	n := len(t.seats)
	contrib := make([]int64, n)
	folded := make([]bool, n)
	scores := make([]poker.Score, n)
	for i, p := range t.seats {
		if p == nil || !p.inHand {
			folded[i] = true
			continue
		}
		contrib[i] = p.put
		folded[i] = p.folded
		if !p.folded {
			sc, best := poker.Best(append(append([]poker.Card{}, p.hole...), t.board...))
			scores[i] = sc
			p.result = sc.Name()
			p.best = best[:]
			p.show = true
		}
	}
	var order []int
	for _, p := range t.inOrder(t.button, func(p *hPlayer) bool { return p.inHand }) {
		order = append(order, p.seat)
	}
	won, pots := poker.Settle(contrib, folded, scores, order)
	t.pots = nil
	for _, pt := range pots {
		t.pots = append(t.pots, pt.Amount)
	}
	var names []string
	daleThere := t.seats[0].inHand && !t.seats[0].folded
	beatDale := map[*hPlayer]bool{}
	for i, w := range won {
		if w <= 0 {
			continue
		}
		p := t.seats[i]
		p.won = w
		p.stack += w
		names = append(names, t.display(p))
		if daleThere && !p.dale && scores[i] > scores[0] {
			beatDale[p] = true
		}
	}
	best := poker.Score(0)
	for i := range scores {
		if !folded[i] && scores[i] > best {
			best = scores[i]
		}
	}
	t.message = strings.Join(names, " and ") + " wins with " + best.Name()
	if len(names) > 1 {
		t.message = strings.Join(names, " and ") + " split it with " + best.Name()
	}
	t.finishHand(nil, beatDale, true)
}

// finishHand records the hand and starts the pause before the next one.
func (t *Holdem) finishHand(bluffs map[*hPlayer]bool, beatDale map[*hPlayer]bool, showdown bool) {
	full := len(t.board) == 5
	var daleDelta int64
	type res struct {
		p     *hPlayer
		delta int64
	}
	var results []res
	for _, p := range t.seats {
		if p == nil || !p.inHand {
			continue
		}
		delta := p.won - p.put
		results = append(results, res{p, delta})
		if p.dale {
			daleDelta = delta
			continue
		}
		cat := -1
		if !p.folded && full {
			sc, _ := poker.Best(append(append([]poker.Card{}, p.hole...), t.board...))
			cat = sc.Category()
		}
		h := PokerHandRecord(delta, p.stack, p.put, p.won, cat, bluffs[p], beatDale[p])
		t.store.RecordPokerHand(p.user, h)
		if p.conn != nil {
			kind := "lose"
			switch {
			case delta > 0:
				kind = "win"
			case delta == 0:
				kind = "push"
			}
			if p.won > 0 || !p.folded {
				p.conn.event(proto.Event{Kind: kind, Seat: p.seat, Delta: delta})
			}
		}
		if t.cfg.Discord.BigWinThreshold > 0 && delta >= t.cfg.Discord.BigWinThreshold {
			t.notify.BigWin(p.user, delta)
		}
	}
	if t.seats[0].inHand {
		house, prevTier := t.store.RecordPokerHouse(daleDelta)
		_ = house
		_ = prevTier
	}
	t.reactToResults(showdown, bluffs, daleDelta)
	t.notify.LeaderboardChanged()
	t.processUnlocks()
	for _, r := range results {
		t.sendProfile(r.p)
	}
	t.setDeadline(time.Duration(t.hc.BetweenSeconds) * time.Second)
	t.after(time.Duration(t.hc.BetweenSeconds)*time.Second, func() {
		t.endHandCleanup()
		t.phase = pkWaiting
		t.clearDeadline()
		t.maybeStart()
		t.broadcast()
	})
	t.broadcast()
	t.updateInfo()
}

func (t *Holdem) endHandCleanup() {
	for _, p := range t.seats {
		if p == nil {
			continue
		}
		p.inHand = false
		p.bet, p.put = 0, 0
		if !p.dale && p.stack == 0 && p.conn != nil {
			p.conn.event(proto.Event{Kind: "info", Seat: p.seat, Text: "You're out of chips. Add more with REBUY, or LEAVE."})
			t.say(banter.EvPkBust, p, banter.Vars{}, 0.9, prioHigh)
		}
	}
	for _, p := range t.humans() {
		if p.leaving || (p.conn == nil && p.seat > 0) {
			p.leaving = false
			p.inHand = false
			t.standUp(p)
			if p.conn == nil {
				delete(t.byUser, strings.ToLower(p.user))
			}
		}
	}
	t.active = -1
}

func (t *Holdem) display(p *hPlayer) string {
	if p.dale {
		return "Dale"
	}
	return p.user
}

// processUnlocks tells players about new cosmetics.
func (t *Holdem) processUnlocks() {
	for _, u := range t.store.TakeUnlocks() {
		p := t.byUser[strings.ToLower(u.User)]
		if p != nil && p.conn != nil {
			p.conn.event(proto.Event{Kind: "unlock", Item: u.Item.ID, Seat: p.seat,
				Text: fmt.Sprintf("Unlocked %s: %s!", u.Item.Kind, u.Item.Name)})
		}
		t.notify.Unlocked(u.User, u.Item)
		t.log.Printf("%s unlocked %s", u.User, u.Item.Name)
	}
}

// ---------------------------------------------------------------- view

func (t *Holdem) settingsView() proto.Settings {
	c := t.cfg
	return proto.Settings{MaxSeats: t.hc.Seats, MinBet: t.hc.BigBlind, ServerName: c.ServerName,
		RefillAmount: c.RefillAmount, DealerTrashTalk: c.DealerTrashTalk, DealerVulgar: c.DealerTrashTalk && c.DealerVulgar,
		ChipGifts: c.ChipGifts && c.RefillAmount > 0, TableChat: c.TableChat}
}

func (t *Holdem) view(viewer *hPlayer) proto.PokerState {
	st := proto.PokerState{Table: t.id, Name: t.name, Phase: t.phase, HandNumber: t.handNo,
		YourSeat: viewer.seat, Button: t.button, Active: t.active, Board: append([]poker.Card{}, t.board...),
		Pot: t.pot(), Pots: t.pots, Message: t.message, SmallBlind: t.hc.SmallBlind, BigBlind: t.hc.BigBlind,
		MinBuyIn: t.hc.MinBuyIn, MaxBuyIn: t.hc.MaxBuyIn, ServerTime: time.Now().UnixMilli(),
		Settings: t.settingsView()}
	if st.Board == nil {
		st.Board = []poker.Card{}
	}
	if !viewer.dale {
		st.Balance = t.store.Balance(viewer.user)
	}
	house := t.store.House()
	st.HouseNet, st.HouseTier = house.Net, house.Tier
	if !t.deadline.IsZero() {
		st.Deadline = t.deadline.UnixMilli()
		st.TimerTotal = t.timerTotal.Milliseconds()
	}
	for _, p := range t.conns {
		if p.seat <= 0 {
			st.Spectators++
		}
	}
	for i, p := range t.seats {
		sv := proto.PokerSeat{Seat: i}
		if p != nil {
			sv.Username = t.display(p)
			sv.Dale = p.dale
			sv.Stack, sv.Bet = p.stack, p.bet
			sv.InHand, sv.Folded, sv.AllIn, sv.SittingOut = p.inHand, p.folded, p.allIn, p.sitOut
			sv.Connected = p.dale || p.conn != nil
			sv.Action, sv.Won = p.action, p.won
			if !p.dale {
				if prof, ok := t.store.Profile(p.user); ok {
					sv.AvatarVersion, sv.Hat, sv.Frame, sv.Title = prof.AvatarVersion, prof.Hat, prof.Frame, prof.Title
				}
			}
			if p.inHand && len(p.hole) > 0 && !p.folded {
				if p == viewer || p.show {
					sv.Cards = append([]poker.Card{}, p.hole...)
				} else {
					sv.Cards = make([]poker.Card, len(p.hole))
				}
			}
			if p.show && t.phase == pkShowdown {
				sv.HandName = p.result
				sv.Best = p.best
			}
		}
		st.Seats = append(st.Seats, sv)
	}
	if viewer.seat > 0 && viewer.seat == t.active && t.phase != pkShowdown && t.phase != pkWaiting {
		toCall := t.curBet - viewer.bet
		st.ToCall = min(toCall, viewer.stack)
		st.Actions = append(st.Actions, "fold")
		if toCall <= 0 {
			st.Actions = append(st.Actions, "check")
		} else {
			st.Actions = append(st.Actions, "call")
		}
		maxTotal := viewer.bet + viewer.stack
		if viewer.canRais && maxTotal > t.curBet {
			st.MinRaise = min(t.curBet+t.minRaise, maxTotal)
			st.MaxRaise = maxTotal
			if t.curBet == 0 {
				st.Actions = append(st.Actions, "bet")
			} else {
				st.Actions = append(st.Actions, "raise")
			}
		}
	}
	if viewer.needLog {
		t.voice.fill(&st, true)
		viewer.needLog = false
	} else {
		t.voice.fill(&st, false)
	}
	return st
}

func (t *Holdem) broadcast() {
	for c, p := range t.conns {
		c.send(proto.SPoker, t.view(p))
	}
}

// ---------------------------------------------------------------- lobby

// infoBox holds the lobby summary, safe to read from other goroutines.
type infoBox struct {
	v atomicInfo
}

func (t *Holdem) updateInfo() {
	in := proto.TableInfo{ID: t.id, Name: t.name, Game: proto.GameHoldem, Seats: t.hc.Seats,
		Stakes:   fmt.Sprintf("Blinds %s/%s", commas(t.hc.SmallBlind), commas(t.hc.BigBlind)),
		MinBuyIn: t.hc.MinBuyIn, MaxBuyIn: t.hc.MaxBuyIn, BigBlind: t.hc.BigBlind}
	for _, p := range t.humans() {
		in.Players++
		in.Names = append(in.Names, p.user)
	}
	for _, p := range t.conns {
		if p.seat <= 0 {
			in.Watching++
		}
	}
	t.infoBox.v.Store(&in)
}

// setConfig applies new settings (at the end of the hand if one is running).
func (t *Holdem) setConfig(cfg *Config, hc holdemCfg, name string) {
	t.Do(func() {
		hc.validate()
		t.cfg = cfg
		t.name = name
		t.pending = &hc
		if t.phase == pkWaiting {
			t.applyPending()
			t.maybeStart()
		}
		t.updateInfo()
		t.broadcast()
	})
}

func (t *Holdem) applyPending() {
	hc := *t.pending
	t.pending = nil
	if hc.Seats+1 != len(t.seats) {
		seats := make([]*hPlayer, hc.Seats+1)
		seats[0] = t.seats[0]
		next := 1
		for _, p := range t.seats[1:] {
			if p == nil {
				continue
			}
			if next < len(seats) {
				seats[next] = p
				p.seat = next
				next++
			} else {
				// no room any more: cash out and watch
				p.seat = -1
				t.store.CashOut(p.user)
				p.stack = 0
				t.errTo(p, "The table got smaller and your seat went with it. Your chips are back in your balance.")
			}
		}
		t.seats = seats
	}
	t.hc = hc
}

// closeTable sends everyone back to the lobby, cashing out, and stops.
func (t *Holdem) closeTable(reason string) {
	t.DoWait(func() {
		for _, p := range t.seats {
			if p != nil && p.inHand {
				p.stack += p.put // the hand's off: chips back
				p.put = 0
			}
		}
		for c, p := range t.conns {
			if p.seat > 0 {
				t.seats[p.seat] = nil
				p.seat = -1
			}
			t.store.CashOut(p.user)
			c.send(proto.SLeft, proto.ErrorMsg{Message: reason})
			c.event(proto.Event{Kind: "info", Text: reason, Seat: -1})
		}
		// anyone seated but disconnected
		for _, p := range t.humans() {
			t.store.CashOut(p.user)
		}
		t.conns = map[*conn]*hPlayer{}
		t.byUser = map[string]*hPlayer{}
		t.stopTimer()
	})
}
