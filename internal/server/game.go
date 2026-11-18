// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Notifier receives game events worth announcing (Discord).
type Notifier interface {
	BigWin(user string, amount int64)
	Refilled(users []string, amount int64)
	LeaderboardChanged()
	Unlocked(user string, item *cosmetics.Item)
	NewTop(user string)
}

type nopNotifier struct{}

func (nopNotifier) BigWin(string, int64)             {}
func (nopNotifier) Refilled([]string, int64)         {}
func (nopNotifier) LeaderboardChanged()              {}
func (nopNotifier) Unlocked(string, *cosmetics.Item) {}
func (nopNotifier) NewTop(string)                    {}

type player struct {
	user       string
	conn       *conn
	seat       int // -1 = spectator
	sitOut     bool
	eligible   bool // may bet in the current betting round
	betPlaced  bool
	pendingBet int64
	inHand     bool
	hands      []*bj.Hand
	leaving    bool
	missed     int
	needLog    bool  // send the dealer's recent lines with the next state
	prevBet    int64 // last round's bet and how it went, for the dealer
	prevDelta  int64
	chatAt     []time.Time // recent chat messages, for flood control
}

type evKind int

const (
	evJoin evKind = iota
	evLeave
	evMsg
	evFunc
)

type event struct {
	kind evKind
	c    *conn
	user string
	msg  proto.Msg
	fn   func()
}

// Game is the table state machine. All state is owned by the Run goroutine;
// other goroutines talk to it through the events channel.
type Game struct {
	cfg     *Config
	pending *Config // config to apply at the next hand (reload)
	store   *Store
	notify  Notifier
	log     *log.Logger

	shoe  *bj.Shoe
	rules bj.Rules

	events chan event

	seats      []*player
	spectators []*player
	byUser     map[string]*player
	conns      map[*conn]*player

	phase      string
	handNo     int
	dealer     []bj.Card
	holeHidden bool
	activeSeat int
	betTurn    int
	deadline   time.Time
	timerTotal time.Duration
	timer      *time.Timer
	timerFn    func()
	message    string
	lastPeriod string
	lastRich   string
	banter     *banter.Picker
	speech     speech
	sayQueue   []banter.Page // pages of the current line still to show
	sayPrio    int
	sayTimer   *time.Timer
	story      *storyRun
	lastStory  time.Time
	forceSay   bool
	sayLog     []proto.DealerLine
	crashes    int  // crashes caught (see safely)
	aborting   bool // calling off a hand after a crash
	reveal     *revealing
	exec       func(line string, w io.Writer) // run a server command (admins)
	holdBust   bool                           // a card reveal is coming: don't announce a bust yet
	handTalk   int                            // lines this hand (he keeps it to a few)
	betNudged  bool                           // nagged a slow bettor this round
	dared      bool                           // dared someone to bet big this round
	bettingAt  time.Time                      // when betting opened
	turnNudged string                         // whose turn he last nagged (seat:hand)
	idleLogged bool
}

// NewGame creates the table.
func NewGame(cfg *Config, store *Store, n Notifier, lg *log.Logger) *Game {
	if n == nil {
		n = nopNotifier{}
	}
	store.CrownMin = cfg.RefillAmount
	rich, _ := store.Tops()
	return &Game{
		lastRich:  rich,
		lastStory: time.Now(),
		banter:    banter.NewPicker(),
		cfg:       cfg, store: store, notify: n, log: lg,
		shoe:       bj.NewShoe(cfg.Decks, cfg.Penetration),
		rules:      cfg.Rules(),
		events:     make(chan event, 1024),
		seats:      make([]*player, cfg.MaxPlayers),
		byUser:     map[string]*player{},
		conns:      map[*conn]*player{},
		phase:      proto.PhaseWaiting,
		activeSeat: -1,
		betTurn:    -1,
	}
}

// Do runs fn on the game goroutine.
func (g *Game) Do(fn func()) { g.events <- event{kind: evFunc, fn: fn} }

// DoWait runs fn on the game goroutine and waits for it.
func (g *Game) DoWait(fn func()) {
	done := make(chan struct{})
	g.events <- event{kind: evFunc, fn: func() { fn(); close(done) }}
	<-done
}

// Reload schedules a new config to take effect at the next hand.
func (g *Game) Reload(c *Config) { g.Do(func() { g.pending = c; g.maybeApplyPending() }) }

// Run processes events until stop is closed.
func (g *Game) Run(stop <-chan struct{}) {
	refill := time.NewTicker(20 * time.Second)
	defer refill.Stop()
	g.checkRefill()
	nudge := time.NewTicker(time.Second)
	defer nudge.Stop()
	for {
		var tc, sc <-chan time.Time
		if g.timer != nil {
			tc = g.timer.C
		}
		if g.sayTimer != nil {
			sc = g.sayTimer.C
		}
		select {
		case <-stop:
			g.stopTimer()
			return
		case ev := <-g.events:
			g.safely("handling a player action", func() { g.handle(ev) })
		case <-tc:
			fn := g.timerFn
			g.timer, g.timerFn = nil, nil
			if fn != nil {
				g.safely("the table timer", fn)
			}
		case <-sc:
			g.safely("dealer speech", g.onSayTimer)
		case <-nudge.C:
			g.safely("dealer speech", g.nudge)
		case <-refill.C:
			g.safely("the daily refill", g.checkRefill)
		}
	}
}

func (g *Game) after(d time.Duration, fn func()) {
	g.stopTimer()
	g.timer = time.NewTimer(d)
	g.timerFn = fn
}

func (g *Game) stopTimer() {
	if g.timer != nil {
		g.timer.Stop()
	}
	g.timer, g.timerFn = nil, nil
}

func (g *Game) setDeadline(d time.Duration) {
	g.deadline = time.Now().Add(d)
	g.timerTotal = d
}

func (g *Game) clearDeadline() { g.deadline = time.Time{}; g.timerTotal = 0 }

func secs(n int) time.Duration { return time.Duration(n) * time.Second }
func ms(n int) time.Duration   { return time.Duration(n) * time.Millisecond }

func (g *Game) handle(ev event) {
	switch ev.kind {
	case evJoin:
		g.join(ev.c, ev.user)
	case evLeave:
		g.leave(ev.c)
	case evMsg:
		p := g.conns[ev.c]
		if p == nil {
			return
		}
		g.onMessage(p, ev.msg)
	case evFunc:
		ev.fn()
	}
}

// ---------------------------------------------------------------- players

func (g *Game) seatedCount() int {
	n := 0
	for _, p := range g.seats {
		if p != nil {
			n++
		}
	}
	return n
}

func (g *Game) join(c *conn, user string) {
	key := strings.ToLower(user)
	p := g.byUser[key]
	if p != nil {
		if p.conn != nil && p.conn != c {
			old := p.conn
			delete(g.conns, old)
			old.kick("You logged in from another client.")
		}
		p.conn = c
		p.needLog = true
		p.leaving = false
	} else {
		p = &player{user: user, conn: c, seat: -1, needLog: true}
		g.byUser[key] = p
		if !g.seatPlayer(p) {
			if len(g.spectators) >= g.cfg.MaxSpectators {
				delete(g.byUser, key)
				c.kick("The table and the rail are full. Try again later.")
				return
			}
			g.spectators = append(g.spectators, p)
		}
	}
	g.conns[c] = p
	g.store.SetOnline(user, true)
	g.idleLogged = false
	g.log.Printf("%s joined (seat %d)", user, p.seat)
	if p.seat < 0 {
		c.event(proto.Event{Kind: "info", Text: "The table is full - you'll get a seat when one opens.", Seat: -1})
	} else if g.phase != proto.PhaseWaiting && g.phase != proto.PhaseBetting {
		c.event(proto.Event{Kind: "info", Text: "You'll be dealt in on the next hand.", Seat: -1})
	}
	if g.phase == proto.PhaseWaiting {
		g.startBetting()
	} else {
		if g.phase == proto.PhaseBetting && g.cfg.BettingMode == "simultaneous" && p.seat >= 0 && g.canPlay(p) {
			p.eligible = true
		}
		g.broadcast()
	}
	g.greet(p)
}

// seatOrder lists seat indexes from the middle of the table outward,
// alternating sides: for 10 seats 4,5,3,6,2,7,1,8,0,9.
func seatOrder(n int) []int {
	order := make([]int, 0, n)
	lo, hi := (n-1)/2, (n-1)/2+1
	if n%2 == 1 {
		order = append(order, lo)
		lo, hi = lo-1, lo+1
	}
	for lo >= 0 || hi < n {
		if lo >= 0 {
			order = append(order, lo)
			lo--
		}
		if hi < n {
			order = append(order, hi)
			hi++
		}
	}
	return order
}

func (g *Game) seatPlayer(p *player) bool {
	for _, i := range seatOrder(len(g.seats)) {
		if s := g.seats[i]; s == nil {
			g.seats[i] = p
			p.seat = i
			return true
		}
	}
	return false
}

func (g *Game) leave(c *conn) {
	p := g.conns[c]
	delete(g.conns, c)
	if p == nil || p.conn != c {
		return
	}
	p.conn = nil
	g.store.SetOnline(p.user, false)
	g.log.Printf("%s disconnected", p.user)
	g.reactToLeave(p)
	if p.inHand {
		p.leaving = true
		// Disconnected players stand automatically.
		if g.phase == proto.PhasePlaying {
			if g.cfg.PlayMode == "turns" && p.seat == g.activeSeat {
				g.standAll(p)
				g.nextTurn()
				return
			}
			if g.cfg.PlayMode == "simultaneous" {
				g.standAll(p)
				if g.allDone() {
					g.dealerPlay()
					return
				}
			}
		}
		g.broadcast()
		return
	}
	g.removePlayer(p)
	if g.phase == proto.PhaseBetting {
		if g.cfg.BettingMode == "turns" && p.seat == g.betTurn {
			g.advanceBetTurn()
			return
		}
		g.checkAllBetsIn()
	}
	g.broadcast()
}

func (g *Game) removePlayer(p *player) {
	if p.seat >= 0 && p.seat < len(g.seats) && g.seats[p.seat] == p {
		g.seats[p.seat] = nil
	}
	for i, s := range g.spectators {
		if s == p {
			g.spectators = append(g.spectators[:i], g.spectators[i+1:]...)
			break
		}
	}
	delete(g.byUser, strings.ToLower(p.user))
	if p.conn != nil {
		delete(g.conns, p.conn)
	}
	g.fillSeats()
	if g.seatedCount() == 0 && len(g.spectators) == 0 && !g.handInProgress() {
		g.goIdle()
	}
}

func (g *Game) fillSeats() {
	for len(g.spectators) > 0 {
		sp := g.spectators[0]
		if !g.seatPlayer(sp) {
			return
		}
		g.spectators = g.spectators[1:]
		if sp.conn != nil {
			sp.conn.event(proto.Event{Kind: "info", Text: "A seat opened up - you're in!", Seat: sp.seat})
		}
	}
}

func (g *Game) goIdle() {
	g.stopTimer()
	g.clearDeadline()
	g.phase = proto.PhaseWaiting
	g.dealer = nil
	g.activeSeat, g.betTurn = -1, -1
	g.message = ""
	if !g.idleLogged {
		g.log.Printf("table idle")
		g.idleLogged = true
	}
}

func (g *Game) handInProgress() bool {
	switch g.phase {
	case proto.PhaseDealing, proto.PhasePlaying, proto.PhaseDealer, proto.PhaseResults:
		return true
	}
	return false
}

func (g *Game) canPlay(p *player) bool {
	return p.seat >= 0 && p.conn != nil && !p.sitOut && g.store.Balance(p.user) >= g.cfg.MinBet
}

// ---------------------------------------------------------------- messages

func (g *Game) onMessage(p *player, m proto.Msg) {
	switch m.Type {
	case proto.CBet:
		var b proto.Bet
		if decode(m, &b) == nil {
			g.onBet(p, b.Amount)
		}
	case proto.CAction:
		var a proto.Action
		if decode(m, &a) == nil {
			g.onAction(p, strings.ToLower(a.Action))
		}
	case proto.CGiftInfo:
		var gi proto.GiftInfo
		if decode(m, &gi) == nil && p.conn != nil {
			max, why := g.giftMax(p, gi.To)
			p.conn.send(proto.SGiftInfo, proto.GiftInfo{To: gi.To, Max: max, Reason: why, Balance: g.store.Balance(gi.To)})
		}
	case proto.CGift:
		var gf proto.Gift
		if decode(m, &gf) == nil {
			g.onGift(p, gf.To, gf.Amount)
		}
	case proto.CChat:
		var c proto.Chat
		if decode(m, &c) == nil {
			g.onChat(p, c.Text)
		}
	case proto.CSitOut:
		var s proto.SitOut
		if decode(m, &s) == nil {
			p.sitOut = s.On
			p.missed = 0
			if s.On && g.phase == proto.PhaseBetting && p.eligible && !p.betPlaced {
				p.eligible = false
				if g.cfg.BettingMode == "turns" && p.seat == g.betTurn {
					g.advanceBetTurn()
					return
				}
				g.checkAllBetsIn()
			}
			if !s.On {
				if g.phase == proto.PhaseWaiting {
					g.startBetting()
					return
				}
				if g.phase == proto.PhaseBetting && g.cfg.BettingMode == "simultaneous" && g.canPlay(p) {
					p.eligible = true
				}
			}
			g.broadcast()
		}
	}
}

// processUnlocks tells players about newly earned cosmetics and announces a
// new holder of the crown.
func (g *Game) processUnlocks() {
	for _, u := range g.store.TakeUnlocks() {
		p := g.byUser[strings.ToLower(u.User)]
		kind := map[string]string{cosmetics.Hat: "hat", cosmetics.Frame: "frame", cosmetics.Title: "title"}[u.Item.Kind]
		if p != nil && p.conn != nil {
			p.conn.event(proto.Event{Kind: "unlock", Item: u.Item.ID, Seat: p.seat,
				Text: fmt.Sprintf("Unlocked %s: %s!", kind, u.Item.Name)})
			g.sendProfile(p)
		}
		if u.Item.Tier >= cosmetics.Rare {
			for c, q := range g.conns {
				if q != p {
					c.event(proto.Event{Kind: "info", Item: u.Item.ID, Seat: -1,
						Text: fmt.Sprintf("%s unlocked the %s %s %s!", u.User, cosmetics.TierNames[u.Item.Tier], u.Item.Name, kind)})
				}
			}
		}
		g.notify.Unlocked(u.User, u.Item)
		g.log.Printf("%s unlocked %s", u.User, u.Item.Name)
	}
	rich, _ := g.store.Tops()
	if rich != g.lastRich {
		g.lastRich = rich
		if rich != "" {
			g.eventAll(proto.Event{Kind: "crown", Seat: -1, Text: rich + " has the most chips and takes the crown!"})
			g.notify.NewTop(rich)
			if p := g.byUser[strings.ToLower(rich)]; p != nil {
				g.say(banter.EvCrown, p, banter.Vars{}, 1, prioHigh)
			}
			g.log.Printf("%s takes the crown", rich)
		}
	}
}

func (g *Game) sendProfile(p *player) {
	if p.conn == nil {
		return
	}
	if prof, ok := g.store.Profile(p.user); ok {
		p.conn.send(proto.SProfile, prof)
	}
}

func (g *Game) errTo(p *player, msg string) {
	if p.conn != nil {
		p.conn.send(proto.SError, proto.ErrorMsg{Message: msg})
	}
}

func (g *Game) eventAll(e proto.Event) {
	for c := range g.conns {
		c.event(e)
	}
}

// ---------------------------------------------------------------- betting

func (g *Game) maybeApplyPending() {
	if g.pending == nil || g.handInProgress() {
		return
	}
	old := g.cfg
	g.cfg = g.pending
	g.pending = nil
	g.rules = g.cfg.Rules()
	g.store.CrownMin = g.cfg.RefillAmount
	if g.cfg.Decks != old.Decks || g.cfg.Penetration != old.Penetration {
		g.shoe = bj.NewShoe(g.cfg.Decks, g.cfg.Penetration)
	}
	if g.cfg.MaxPlayers != len(g.seats) {
		ns := make([]*player, g.cfg.MaxPlayers)
		var extra []*player
		for _, p := range g.seats {
			if p == nil {
				continue
			}
			if p.seat < len(ns) && ns[p.seat] == nil {
				ns[p.seat] = p
			} else {
				extra = append(extra, p)
			}
		}
		g.seats = ns
		for _, p := range extra {
			p.seat = -1
			if !g.seatPlayer(p) {
				g.spectators = append(g.spectators, p)
			}
		}
		g.fillSeats()
	}
	g.log.Printf("configuration reloaded")
	g.eventAll(proto.Event{Kind: "info", Text: "Table settings were updated.", Seat: -1})
	if g.phase == proto.PhaseBetting {
		g.startBetting()
	} else {
		g.broadcast()
	}
}

func (g *Game) startBetting() {
	g.stopTimer()
	g.maybeApplyPending()
	if g.pending != nil {
		return
	}
	g.dealer = nil
	g.holeHidden = false
	g.activeSeat = -1
	for _, p := range g.seats {
		if p == nil {
			continue
		}
		p.hands, p.inHand = nil, false
		p.betPlaced, p.pendingBet = false, 0
		p.eligible = g.canPlay(p)
	}
	any := false
	for _, p := range g.seats {
		if p != nil && p.eligible {
			any = true
		}
	}
	if !any {
		g.goIdle()
		if g.seatedCount() > 0 {
			g.message = "Waiting for players..."
		}
		g.broadcast()
		return
	}
	g.idleLogged = false
	g.handTalk, g.betNudged, g.dared = 0, false, false
	g.bettingAt = time.Now()
	if g.shoe.NeedsShuffle() {
		g.shoe.Shuffle()
		g.eventAll(proto.Event{Kind: "shuffle", Text: "Shuffling the shoe...", Seat: -1})
		defer g.say(banter.EvShuffle, nil, banter.Vars{}, 0.5, prioLow)
	}
	g.phase = proto.PhaseBetting
	if g.cfg.BettingMode == "turns" {
		g.betTurn = -1
		g.advanceBetTurn()
		g.chatter()
		return
	}
	g.betTurn = -1
	g.message = "Place your bets!"
	g.setDeadline(g.cfg.Timing().Betting)
	g.after(g.cfg.Timing().Betting, g.endBetting)
	g.broadcast()
	g.chatter()
}

func (g *Game) onBet(p *player, amt int64) {
	if g.phase != proto.PhaseBetting || !p.eligible || p.seat < 0 {
		g.errTo(p, "You can't bet right now.")
		return
	}
	turns := g.cfg.BettingMode == "turns"
	if turns && (p.seat != g.betTurn || p.betPlaced) {
		g.errTo(p, "It's not your turn to bet.")
		return
	}
	if amt < g.cfg.MinBet || amt > g.cfg.MaxBet {
		g.errTo(p, fmt.Sprintf("Bets must be between %d and %d.", g.cfg.MinBet, g.cfg.MaxBet))
		return
	}
	if amt > g.store.Balance(p.user) {
		g.errTo(p, "You don't have enough chips for that bet.")
		return
	}
	changed := p.betPlaced
	p.pendingBet, p.betPlaced = amt, true
	g.eventAll(proto.Event{Kind: "chip", Seat: p.seat})
	if !changed {
		g.reactToBet(p, amt)
	}
	if turns {
		g.advanceBetTurn()
		return
	}
	g.checkAllBetsIn()
	g.broadcast()
}

func (g *Game) checkAllBetsIn() {
	if g.phase != proto.PhaseBetting || g.cfg.BettingMode != "simultaneous" {
		return
	}
	waiting, placed := 0, 0
	for _, p := range g.seats {
		if p != nil && p.eligible {
			if p.betPlaced {
				placed++
			} else {
				waiting++
			}
		}
	}
	if waiting == 0 {
		if placed == 0 {
			g.startBetting()
			return
		}
		g.message = "All bets are in!"
		g.setDeadline(800 * time.Millisecond)
		g.after(800*time.Millisecond, g.endBetting)
	}
}

func (g *Game) advanceBetTurn() {
	start := g.betTurn + 1
	for i := start; i < len(g.seats); i++ {
		p := g.seats[i]
		if p != nil && p.eligible && !p.betPlaced {
			if !g.canPlay(p) {
				p.eligible = false
				continue
			}
			g.betTurn = i
			g.message = p.user + " is betting..."
			g.setDeadline(g.cfg.Timing().Betting)
			g.after(g.cfg.Timing().Betting, func() {
				if q := g.seats[i]; q != nil && !q.betPlaced {
					q.eligible = false
				}
				g.advanceBetTurn()
			})
			if p.conn != nil {
				p.conn.event(proto.Event{Kind: "turn", Text: "Your bet!", Seat: i})
			}
			g.broadcast()
			return
		}
	}
	g.betTurn = -1
	g.setDeadline(500 * time.Millisecond)
	g.after(500*time.Millisecond, g.endBetting)
	g.broadcast()
}

func (g *Game) endBetting() {
	g.stopTimer()
	g.clearDeadline()
	g.betTurn = -1
	inHand := 0
	for _, p := range g.seats {
		if p == nil {
			continue
		}
		if p.eligible && p.betPlaced && p.conn != nil {
			if g.store.Debit(p.user, p.pendingBet) {
				p.inHand = true
				p.hands = []*bj.Hand{{Bet: p.pendingBet}}
				p.missed = 0
				inHand++
			}
		} else if p.eligible && !p.betPlaced {
			if p.conn != nil {
				defer g.say(banter.EvNoBet, p, banter.Vars{}, 0.35, prioLow)
			}
			p.missed++
			if p.missed >= 3 && !p.sitOut {
				p.sitOut = true
				if p.conn != nil {
					p.conn.event(proto.Event{Kind: "info", Text: "You missed 3 hands, so you're sitting out. Click Sit In to play.", Seat: p.seat})
				}
			}
		}
		p.eligible, p.betPlaced = false, false
	}
	if inHand == 0 {
		g.startBetting()
		return
	}
	g.handNo++
	g.phase = proto.PhaseDealing
	g.message = "Dealing..."
	g.dealer = nil
	g.holeHidden = true
	var queue []*player // nil entry = dealer
	for round := 0; round < 2; round++ {
		for _, p := range g.seats {
			if p != nil && p.inHand {
				queue = append(queue, p)
			}
		}
		queue = append(queue, nil)
	}
	g.broadcast()
	g.dealStep(queue)
}

func (g *Game) dealStep(queue []*player) {
	p := queue[0]
	card := g.shoe.Draw()
	seat := -1
	if p == nil {
		g.dealer = append(g.dealer, card)
	} else {
		p.hands[0].Cards = append(p.hands[0].Cards, card)
		seat = p.seat
	}
	g.eventAll(proto.Event{Kind: "card", Seat: seat})
	g.broadcast()
	rest := queue[1:]
	if len(rest) > 0 {
		g.after(g.cfg.Timing().Deal, func() { g.dealStep(rest) })
	} else {
		g.after(g.cfg.Timing().Deal, g.afterDeal)
	}
}

func (g *Game) afterDeal() {
	up := g.dealer[0]
	if up.Rank == 1 || up.Value() == 10 {
		if bj.IsBlackjack(g.dealer) {
			g.holeHidden = false
			g.message = "Dealer has Blackjack!"
			defer g.say(banter.EvDealerBJ, nil, banter.Vars{}, 1, prioHigh)
			g.eventAll(proto.Event{Kind: "card", Seat: -1})
			for _, p := range g.seats {
				if p != nil && p.inHand {
					for _, h := range p.hands {
						h.Done = true
					}
				}
			}
			g.phase = proto.PhaseDealer
			g.broadcast()
			g.after(g.cfg.Timing().Dealer*2, g.settle)
			return
		}
	}
	for _, p := range g.seats {
		if p != nil && p.inHand && p.hands[0].Natural() {
			p.hands[0].Done = true
			g.eventAll(proto.Event{Kind: "blackjack_dealt", Seat: p.seat, Text: p.user + " has Blackjack!"})
			defer g.say(banter.EvPlayerBJ, p, banter.Vars{}, 0.7, prioNormal)
		}
	}
	g.reactToUpCard()
	g.startPlay()
}

// ---------------------------------------------------------------- playing

func curHand(p *player) int {
	for i, h := range p.hands {
		if !h.Done {
			return i
		}
	}
	return -1
}

func (g *Game) standAll(p *player) {
	for _, h := range p.hands {
		if len(h.Cards) == 1 { // a split hand that never got its second card
			h.Cards = append(h.Cards, g.shoe.Draw())
			g.eventAll(proto.Event{Kind: "card", Seat: p.seat})
		}
		h.Done = true
	}
}

// fillSplit gives the hand a player has moved on to its second card, if it's
// one half of a split.
func (g *Game) fillSplit(p *player) {
	for {
		idx := curHand(p)
		if idx < 0 || len(p.hands[idx].Cards) != 1 {
			return
		}
		g.drawTo(p, p.hands[idx])
	}
}

// endTurn moves on to the next player after a short breather.
func (g *Game) endTurn() {
	gap := g.cfg.Timing().TurnGap
	if gap <= 0 {
		g.nextTurn()
		return
	}
	g.clearDeadline()
	g.broadcast()
	g.after(gap, g.nextTurn)
}

func (g *Game) allDone() bool {
	for _, p := range g.seats {
		if p != nil && p.inHand && curHand(p) >= 0 {
			return false
		}
	}
	return true
}

func (g *Game) startPlay() {
	g.phase = proto.PhasePlaying
	if g.cfg.PlayMode == "turns" {
		g.activeSeat = -1
		g.nextTurn()
		return
	}
	g.activeSeat = -1
	for _, p := range g.seats {
		if p != nil && p.inHand && p.conn == nil {
			g.standAll(p)
		}
	}
	if g.allDone() {
		g.dealerPlay()
		return
	}
	g.message = "Everyone play your hands!"
	for _, p := range g.seats {
		if p != nil && p.inHand && curHand(p) >= 0 && p.conn != nil {
			p.conn.event(proto.Event{Kind: "turn", Text: "Your turn!", Seat: p.seat})
		}
	}
	g.setDeadline(g.cfg.Timing().Play)
	g.after(g.cfg.Timing().Play, func() {
		for _, p := range g.seats {
			if p != nil && p.inHand {
				g.standAll(p)
			}
		}
		g.dealerPlay()
	})
	g.broadcast()
}

func (g *Game) nextTurn() {
	for i, p := range g.seats {
		if p == nil || !p.inHand || curHand(p) < 0 {
			continue
		}
		if p.conn == nil {
			g.standAll(p)
			continue
		}
		g.activeSeat = i
		g.message = p.user + "'s turn"
		g.armTurnTimer()
		p.conn.event(proto.Event{Kind: "turn", Text: "Your turn!", Seat: i})
		g.broadcast()
		g.heckleTurn(p)
		return
	}
	g.dealerPlay()
}

func (g *Game) armTurnTimer() {
	seat := g.activeSeat
	g.setDeadline(g.cfg.Timing().Turn)
	g.after(g.cfg.Timing().Turn, func() {
		if p := g.seats[seat]; p != nil {
			g.standAll(p)
			if p.conn != nil {
				p.conn.event(proto.Event{Kind: "info", Text: "Time's up - you stand.", Seat: seat})
			}
		}
		g.endTurn()
	})
}

func (g *Game) onAction(p *player, act string) {
	if g.reveal != nil {
		return // wait for the card to turn over
	}
	if g.phase != proto.PhasePlaying || !p.inHand {
		g.errTo(p, "You can't do that right now.")
		return
	}
	turns := g.cfg.PlayMode == "turns"
	if turns && p.seat != g.activeSeat {
		g.errTo(p, "It's not your turn.")
		return
	}
	idx := curHand(p)
	if idx < 0 {
		return
	}
	h := p.hands[idx]
	r := g.rules
	before, beforeSoft := h.Total()
	splitTens := act == "split" && len(h.Cards) == 2 && h.Cards[0].Value() == 10 && h.Cards[1].Value() == 10
	reveal := false
	g.holdBust = g.cfg.CardReveal // the bust news waits for the reveal
	defer func() { g.holdBust = false }()
	switch act {
	case "hit":
		if !r.CanHit(h) {
			g.errTo(p, "You can't hit this hand.")
			return
		}
		g.drawTo(p, h)
		reveal = true
	case "stand":
		h.Done = true
	case "double":
		if !r.CanDouble(h) {
			g.errTo(p, "You can't double this hand.")
			return
		}
		if !g.store.Debit(p.user, h.Bet) {
			g.errTo(p, "Not enough chips to double.")
			return
		}
		h.Bet *= 2
		h.Doubled = true
		g.eventAll(proto.Event{Kind: "chip", Seat: p.seat})
		g.drawTo(p, h)
		h.Done = true
		reveal = true
	case "split":
		if !r.CanSplit(h, len(p.hands)) {
			g.errTo(p, "You can't split this hand.")
			return
		}
		if !g.store.Debit(p.user, h.Bet) {
			g.errTo(p, "Not enough chips to split.")
			return
		}
		second := h.Cards[1]
		h.Cards = h.Cards[:1]
		aces := second.Rank == 1
		nh := &bj.Hand{Cards: []bj.Card{second}, Bet: h.Bet, FromSplit: true, SplitAces: aces}
		h.FromSplit, h.SplitAces = true, aces
		p.hands = append(p.hands[:idx+1], append([]*bj.Hand{nh}, p.hands[idx+1:]...)...)
		g.eventAll(proto.Event{Kind: "chip", Seat: p.seat})
		// like a real table: the first hand gets its card now, the second
		// gets one when you move on to it (split aces get both right away)
		g.drawTo(p, h)
		if aces && !r.HitSplitAces {
			g.drawTo(p, nh)
			h.Done, nh.Done = true, true
		}
	case "surrender":
		if !r.CanSurrender(h, len(p.hands)) {
			g.errTo(p, "You can't surrender this hand.")
			return
		}
		h.Surrendered, h.Done = true, true
	default:
		g.errTo(p, "Unknown action.")
		return
	}
	g.holdBust = false
	finish := func() {
		g.fillSplit(p)
		switch {
		case turns && curHand(p) < 0:
			g.endTurn()
		case turns:
			g.armTurnTimer()
			g.broadcast()
		case g.allDone():
			g.dealerPlay()
		default:
			g.broadcast()
		}
		if g.phase == proto.PhasePlaying || g.phase == proto.PhaseDealer {
			g.reactToAction(p, act, h, before, beforeSoft, splitTens)
		}
	}
	if reveal && g.cfg.CardReveal {
		g.startReveal(p.seat, idx, len(h.Cards)-1, func() string {
			if h.Busted() && p.conn != nil {
				p.conn.event(proto.Event{Kind: "bust", Seat: p.seat, Text: "Bust!"})
			}
			return playerOutcome(h)
		}, finish)
		return
	}
	finish()
}

func (g *Game) drawTo(p *player, h *bj.Hand) {
	h.Cards = append(h.Cards, g.shoe.Draw())
	g.eventAll(proto.Event{Kind: "card", Seat: p.seat})
	if t, _ := h.Total(); t >= 21 {
		h.Done = true
		if t > 21 && p.conn != nil && !g.holdBust {
			p.conn.event(proto.Event{Kind: "bust", Seat: p.seat, Text: "Bust!"})
		}
	}
}

// ---------------------------------------------------------------- dealer

func (g *Game) dealerPlay() {
	g.stopTimer()
	g.clearDeadline()
	g.phase = proto.PhaseDealer
	g.activeSeat = -1
	g.message = "Dealer's turn"
	g.broadcast()
	g.after(g.cfg.Timing().Dealer, func() {
		g.holeHidden = false
		g.eventAll(proto.Event{Kind: "card", Seat: -1})
		if g.cfg.CardReveal {
			g.startReveal(-1, 0, 1, g.dealerOutcome, g.dealerContinue)
			return
		}
		g.broadcast()
		g.dealerContinue()
	})
}

func (g *Game) dealerNeedsToDraw() bool {
	live := false
	for _, p := range g.seats {
		if p == nil || !p.inHand {
			continue
		}
		for _, h := range p.hands {
			if !h.Busted() && !h.Surrendered && !h.Natural() {
				live = true
			}
		}
	}
	return live && g.rules.DealerShouldHit(g.dealer)
}

func (g *Game) dealerContinue() {
	if g.dealerNeedsToDraw() {
		g.after(g.cfg.Timing().Dealer, func() {
			g.dealer = append(g.dealer, g.shoe.Draw())
			g.eventAll(proto.Event{Kind: "card", Seat: -1})
			if g.cfg.CardReveal {
				g.startReveal(-1, 0, len(g.dealer)-1, g.dealerOutcome, g.dealerContinue)
				return
			}
			g.broadcast()
			g.dealerContinue()
		})
		return
	}
	g.after(g.cfg.Timing().Dealer, g.settle)
}

func (g *Game) settle() {
	g.stopTimer()
	g.holeHidden = false
	g.phase = proto.PhaseResults
	dt, _ := bj.Total(g.dealer)
	switch {
	case bj.IsBlackjack(g.dealer):
		g.message = "Dealer has Blackjack."
	case dt > 21:
		g.message = fmt.Sprintf("Dealer busts with %d!", dt)
	default:
		g.message = fmt.Sprintf("Dealer stands on %d.", dt)
	}
	var results []roundResult
	var houseDelta int64
	playerHands, playerBusts := 0, 0
	for _, p := range g.seats {
		if p == nil || !p.inHand {
			continue
		}
		var res []HandResult
		var staked int64
		doubleLost, doubleWon, pushed := false, false, false
		var delta int64
		bjCount := 0
		for _, h := range p.hands {
			g.rules.Settle(h, g.dealer)
			res = append(res, HandResult{Wagered: h.Bet, Payout: h.Payout, Result: h.Result, Blackjack: h.Result == "blackjack", Doubled: h.Doubled, FromSplit: h.FromSplit})
			delta += h.Payout - h.Bet
			staked += h.Bet
			if h.Result == "blackjack" {
				bjCount++
			}
			if h.Result == "bust" {
				playerBusts++
			}
			if h.Doubled && h.Payout < h.Bet {
				doubleLost = true
			}
			if h.Doubled && h.Payout > h.Bet {
				doubleWon = true
			}
			if h.Result == "push" {
				pushed = true
			}
			playerHands++
		}
		houseDelta -= delta
		results = append(results, roundResult{p: p, delta: delta, before: g.store.Balance(p.user) + staked, doubleLost: doubleLost, doubleWon: doubleWon, pushed: pushed})
		p.prevBet, p.prevDelta = p.pendingBet, delta
		g.store.Settle(p.user, res)
		g.sendProfile(p)
		kind := "push"
		switch {
		case bjCount > 0 && delta > 0:
			kind = "blackjack"
		case delta > 0:
			kind = "win"
		case delta < 0:
			kind = "lose"
		}
		if p.conn != nil {
			p.conn.event(proto.Event{Kind: kind, Seat: p.seat, Delta: delta})
		}
		if g.cfg.Discord.BigWinThreshold > 0 && delta >= g.cfg.Discord.BigWinThreshold {
			g.notify.BigWin(p.user, delta)
		}
	}
	if len(results) > 0 {
		house, prevTier := g.store.RecordRound(houseDelta, playerHands, bj.IsBlackjack(g.dealer), dt > 21, playerBusts)
		if house.Tier != prevTier {
			defer g.tierChanged(prevTier, house.Tier, results)
		}
	}
	g.processUnlocks()
	g.notify.LeaderboardChanged()
	defer g.reactToResults(results, houseDelta, dt > 21, bj.IsBlackjack(g.dealer))
	g.setDeadline(g.cfg.Timing().Results)
	g.after(g.cfg.Timing().Results, g.endRound)
	g.broadcast()
}

func (g *Game) endRound() {
	period := g.cfg.RefillPeriod(time.Now())
	var refilled []string
	for _, p := range g.seats {
		if p == nil || !p.inHand {
			continue
		}
		p.inHand = false
		if g.lastPeriod == period {
			if d := g.store.ApplyRefill(p.user, g.cfg.RefillAmount, period); d > 0 {
				refilled = append(refilled, p.user)
				if p.conn != nil {
					p.conn.event(proto.Event{Kind: "refill", Seat: p.seat, Delta: d, Text: fmt.Sprintf("Daily refill: +%d chips", d)})
					defer g.reactToRefill(p, d)
					g.sendProfile(p)
				}
			}
		}
	}
	if len(refilled) > 0 {
		g.notify.Refilled(refilled, g.cfg.RefillAmount)
	}
	g.processUnlocks()
	for _, p := range append([]*player(nil), g.seats...) {
		if p != nil && p.leaving {
			p.inHand = false
			g.removePlayer(p)
		}
	}
	g.fillSeats()
	g.phase = proto.PhaseWaiting // lets startBetting apply pending config
	g.startBetting()
}

// checkRefill runs the daily refill when a new refill period begins. Players
// in the middle of a hand are refilled when the hand ends.
func (g *Game) checkRefill() {
	period := g.cfg.RefillPeriod(time.Now())
	if period == g.lastPeriod {
		return
	}
	g.lastPeriod = period
	var refilled []string
	for _, u := range g.store.Usernames() {
		if p := g.byUser[strings.ToLower(u)]; p != nil && p.inHand {
			continue
		}
		if d := g.store.ApplyRefill(u, g.cfg.RefillAmount, period); d > 0 {
			refilled = append(refilled, u)
			if p := g.byUser[strings.ToLower(u)]; p != nil && p.conn != nil {
				p.conn.event(proto.Event{Kind: "refill", Seat: p.seat, Delta: d, Text: fmt.Sprintf("Daily refill: +%d chips", d)})
				defer g.reactToRefill(p, d)
				g.sendProfile(p)
			}
		}
	}
	if len(refilled) > 0 {
		g.log.Printf("daily refill (%s): %d account(s) topped up to %d", period, len(refilled), g.cfg.RefillAmount)
		g.notify.Refilled(refilled, g.cfg.RefillAmount)
		g.notify.LeaderboardChanged()
	}
	g.processUnlocks()
	if g.phase == proto.PhaseWaiting && g.seatedCount() > 0 {
		g.startBetting()
	} else {
		g.broadcast()
	}
}

// ---------------------------------------------------------------- views

func (g *Game) settings() proto.Settings {
	c := g.cfg
	return proto.Settings{
		MaxSeats: len(g.seats), MinBet: c.MinBet, MaxBet: c.MaxBet,
		BettingMode: c.BettingMode, PlayMode: c.PlayMode, HideOtherHands: c.HideOtherHands,
		Decks: c.Decks, DealerHitsSoft17: c.DealerHitsSoft17, BlackjackPays: c.BlackjackPays,
		Surrender: c.AllowSurrender, RefillAmount: c.RefillAmount, ServerName: c.ServerName,
		DealerTrashTalk: c.DealerTrashTalk, DealerVulgar: c.DealerTrashTalk && c.DealerVulgar,
		ChipGifts: c.ChipGifts && c.RefillAmount > 0, TableChat: c.TableChat, CardReveal: c.CardReveal,
	}
}

func (g *Game) seatStatus(p *player) string {
	switch {
	case p.conn == nil:
		return "away"
	case p.inHand:
		return "playing"
	case p.sitOut:
		return "sitting_out"
	case g.phase == proto.PhaseBetting && p.eligible && p.betPlaced:
		return "bet_placed"
	case g.phase == proto.PhaseBetting && p.eligible:
		return "betting"
	case g.store.Balance(p.user) < g.cfg.MinBet:
		return "broke"
	}
	return "waiting"
}

func (g *Game) view(viewer *player) proto.TableState {
	st := proto.TableState{
		Phase: g.phase, HandNumber: g.handNo, ServerTime: time.Now().UnixMilli(),
		ActiveSeat: -1, YourSeat: viewer.seat, Message: g.message, Settings: g.settings(),
		Spectators: len(g.spectators), ShoeLeft: g.shoe.Remaining(),
	}
	if g.cfg.DealerTrashTalk && time.Now().Before(g.speech.until) {
		st.DealerSay, st.DealerSayID, st.DealerSayUntil = g.speech.text, g.speech.id, g.speech.until.UnixMilli()
		st.DealerSayCont, st.DealerSayPara = g.speech.cont, g.speech.cont && g.speech.page.Para
	}
	if viewer.needLog {
		viewer.needLog = false
		st.DealerLog = g.sayLog
	}
	house := g.store.House()
	st.HouseNet, st.HouseTier = house.Net, house.Tier
	if !g.deadline.IsZero() {
		st.Deadline = g.deadline.UnixMilli()
		st.TimerTotal = g.timerTotal.Milliseconds()
	}
	// dealer
	for i, c := range g.dealer {
		if (i == 1 && g.holeHidden) || (g.reveal != nil && !g.reveal.shown && g.reveal.seat == -1 && g.reveal.card == i) {
			st.Dealer.Cards = append(st.Dealer.Cards, bj.Card{})
			continue
		}
		st.Dealer.Cards = append(st.Dealer.Cards, c)
	}
	st.Dealer.Total, st.Dealer.Soft = bj.Total(st.Dealer.Cards)
	if st.Dealer.Cards == nil {
		st.Dealer.Cards = []bj.Card{}
	}

	switch {
	case g.phase == proto.PhasePlaying && g.cfg.PlayMode == "turns":
		st.ActiveSeat = g.activeSeat
	case g.phase == proto.PhaseBetting && g.cfg.BettingMode == "turns":
		st.ActiveSeat = g.betTurn
	}
	hide := g.cfg.HideOtherHands && g.phase != proto.PhaseResults
	st.Seats = make([]proto.SeatView, len(g.seats))
	for i, p := range g.seats {
		sv := proto.SeatView{Seat: i, ActiveHand: -1, Hands: []proto.HandView{}}
		if p != nil {
			prof, _ := g.store.Profile(p.user)
			sv.Username = p.user
			sv.AvatarVersion = prof.AvatarVersion
			sv.Balance = prof.Balance
			sv.Status = g.seatStatus(p)
			sv.Connected = p.conn != nil
			sv.Hat, sv.Frame, sv.Title = prof.Hat, prof.Frame, prof.Title
			if p.betPlaced {
				sv.PendingBet = p.pendingBet
			}
			if g.phase == proto.PhasePlaying && p.inHand {
				if g.cfg.PlayMode == "simultaneous" || i == g.activeSeat {
					sv.ActiveHand = curHand(p)
				}
			}
			for hi, h := range p.hands {
				hv := proto.HandView{Bet: h.Bet, Done: h.Done, Doubled: h.Doubled, Result: h.Result, Payout: h.Payout}
				if hide && p != viewer {
					hv.Cards = make([]bj.Card, len(h.Cards))
				} else {
					hv.Cards = append([]bj.Card(nil), h.Cards...)
					hv.Total, hv.Soft = h.Total()
					hv.Natural = h.Natural()
				}
				if rv := g.reveal; rv != nil && !rv.shown && rv.seat == i && rv.hand == hi && rv.card < len(hv.Cards) {
					// still face down: nobody knows yet, not even its owner
					hv.Cards[rv.card] = bj.Card{}
					hv.Total, hv.Soft = bj.Total(hv.Cards)
					hv.Done = false
				}
				if hv.Cards == nil {
					hv.Cards = []bj.Card{}
				}
				sv.Hands = append(sv.Hands, hv)
			}
		}
		st.Seats[i] = sv
	}
	if rv := g.reveal; rv != nil && !rv.shown {
		st.Reveal = &proto.RevealView{Seat: rv.seat, Hand: rv.hand, Card: rv.card, Until: rv.until.UnixMilli()}
	}
	// what can the viewer do?
	if viewer.seat >= 0 {
		if g.reveal != nil {
			// nothing to press while a card is being revealed
		} else if g.phase == proto.PhaseBetting && viewer.eligible {
			if g.cfg.BettingMode == "simultaneous" || (viewer.seat == g.betTurn && !viewer.betPlaced) {
				st.CanBet = true
			}
		}
		if g.reveal == nil && g.phase == proto.PhasePlaying && viewer.inHand && (g.cfg.PlayMode == "simultaneous" || viewer.seat == g.activeSeat) {
			if idx := curHand(viewer); idx >= 0 {
				h := viewer.hands[idx]
				bal := g.store.Balance(viewer.user)
				if g.rules.CanHit(h) {
					st.Actions = append(st.Actions, "hit")
				}
				st.Actions = append(st.Actions, "stand")
				if g.rules.CanDouble(h) && bal >= h.Bet {
					st.Actions = append(st.Actions, "double")
				}
				if g.rules.CanSplit(h, len(viewer.hands)) && bal >= h.Bet {
					st.Actions = append(st.Actions, "split")
				}
				if g.rules.CanSurrender(h, len(viewer.hands)) {
					st.Actions = append(st.Actions, "surrender")
				}
			}
		}
		if g.store.Balance(viewer.user) < g.cfg.MinBet && !viewer.inHand && g.phase != proto.PhaseResults {
			next := g.cfg.NextRefill(time.Now()).In(g.cfg.loc)
			st.Message = fmt.Sprintf("You're out of chips. Daily refill to %d at %s.", g.cfg.RefillAmount, next.Format("15:04 MST"))
		}
	} else {
		st.Message = "Table full - you're watching from the rail."
	}
	if st.Actions == nil {
		st.Actions = []string{}
	}
	return st
}

func (g *Game) broadcast() {
	for c, p := range g.conns {
		c.send(proto.SState, g.view(p))
	}
}

// ---------------------------------------------------------------- admin helpers

// PlayerList describes who is connected (for the console).
func (g *Game) PlayerList() []string {
	var out []string
	g.DoWait(func() {
		for i, p := range g.seats {
			if p != nil {
				st := g.seatStatus(p)
				out = append(out, fmt.Sprintf("seat %2d: %-16s %-12s balance %d", i+1, p.user, st, g.store.Balance(p.user)))
			}
		}
		for _, p := range g.spectators {
			out = append(out, fmt.Sprintf("rail   : %s", p.user))
		}
		out = append(out, fmt.Sprintf("phase: %s, hand #%d", g.phase, g.handNo))
	})
	return out
}

// Kick disconnects a user.
func (g *Game) Kick(user, reason string) bool {
	ok := false
	g.DoWait(func() {
		if p := g.byUser[strings.ToLower(user)]; p != nil && p.conn != nil {
			p.conn.kick(reason)
			ok = true
		}
	})
	return ok
}

// Notify sends an info toast to a connected user, if present.
func (g *Game) NotifyUser(user, text string) {
	g.Do(func() {
		if p := g.byUser[strings.ToLower(user)]; p != nil && p.conn != nil {
			p.conn.event(proto.Event{Kind: "info", Text: text, Seat: p.seat})
		}
		g.broadcast()
	})
}

// RefreshProfile sends a player their profile again (after admin changes).
func (g *Game) RefreshProfile(user string) {
	g.Do(func() {
		if p := g.byUser[strings.ToLower(user)]; p != nil {
			g.sendProfile(p)
		}
	})
}

// Broadcast pushes fresh state to everyone (after admin changes).
func (g *Game) Broadcast() {
	g.Do(func() {
		g.processUnlocks()
		if g.phase == proto.PhaseWaiting && g.seatedCount() > 0 {
			g.startBetting()
			return
		}
		g.broadcast()
	})
}

// Announce sends a message to everyone at the table.
func (g *Game) Announce(text string) {
	g.Do(func() { g.eventAll(proto.Event{Kind: "info", Text: text, Seat: -1}) })
}

// InHand reports whether a user currently has chips on the table.
func (g *Game) InHand(user string) bool {
	in := false
	g.DoWait(func() {
		if p := g.byUser[strings.ToLower(user)]; p != nil {
			in = p.inHand
		}
	})
	return in
}

// Summary is a one-line description of the table (Discord /table).
func (g *Game) Summary() (phase string, seated int, names []string, hand int) {
	g.DoWait(func() {
		phase, hand = g.phase, g.handNo
		for _, p := range g.seats {
			if p != nil {
				seated++
				names = append(names, p.user)
			}
		}
	})
	return
}

// safely runs f; if it crashes, the crash is logged, the hand in progress is
// called off with every bet returned, and the table carries on.
func (g *Game) safely(what string, f func()) {
	guard(g.log.Printf, what, func() {
		g.crashes++
		if g.crashes > 20 || g.aborting {
			return // something's badly wrong; stop digging
		}
		g.aborting = true
		defer func() { g.aborting = false }()
		guard(g.log.Printf, "recovering the table", nil, g.abortHand)
	}, f)
}

// abortHand calls off the current hand after a crash: bets that are on the
// table go back to their owners and the next hand starts shortly.
func (g *Game) abortHand() {
	refund := g.phase == proto.PhaseDealing || g.phase == proto.PhasePlaying || g.phase == proto.PhaseDealer
	for _, p := range g.seats {
		if p == nil {
			continue
		}
		hands := p.hands
		p.hands = nil // first, so a second crash can't refund twice
		if refund && p.inHand {
			var back int64
			for _, h := range hands {
				back += h.Bet
			}
			if back > 0 {
				g.store.Refund(p.user, back)
			}
		}
	}
	g.dealer = nil
	g.reveal = nil
	g.sayQueue, g.story = nil, nil
	g.eventAll(proto.Event{Kind: "info", Text: "The dealer fumbled the cards. That hand is off - all bets returned."})
	_ = g.store.Save()
	g.after(2*time.Second, g.endRound)
	for _, p := range g.seats {
		if p != nil && p.conn != nil {
			g.sendProfile(p)
		}
	}
	g.broadcast()
}
