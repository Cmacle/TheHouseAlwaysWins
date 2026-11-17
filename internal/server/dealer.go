package server

import (
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Dealer trash talk. Lines have a priority: a line only interrupts the one
// on screen if it's more important, or the current one has been up a while.
const (
	prioLow = iota
	prioNormal
	prioHigh
	prioStory // only used for the bubble: nothing small interrupts a story
)

type speech struct {
	page  banter.Page
	cont  bool
	text  string
	id    int
	prio  int
	at    time.Time
	until time.Time
}

func cardName(c bj.Card) string {
	switch c.Rank {
	case 1:
		return "Ace"
	case 11:
		return "Jack"
	case 12:
		return "Queen"
	case 13:
		return "King"
	}
	return string("0123456789T"[c.Rank : c.Rank+1])
}

func upCardName(c bj.Card) string {
	if c.Rank == 10 {
		return "10"
	}
	return cardName(c)
}

// say makes the dealer speak, if trash talk is on and the dice allow it.
// It returns true if a line was said.
func (g *Game) say(ev string, p *player, v banter.Vars, chance float64, prio int) bool {
	if !g.cfg.DealerTrashTalk || g.banter == nil {
		return false
	}
	now := time.Now()
	if !g.cfg.TurboMode && !g.forceSay {
		// he runs his mouth a bit less at the normal pace
		switch prio {
		case prioLow:
			chance *= 0.55
		case prioNormal:
			chance *= 0.75
		}
	}
	if !g.forceSay && g.busyTalking(prio, now) {
		return false
	}
	if !g.forceSay && prio < prioHigh && g.handTalk >= g.talkBudget() {
		return false
	}
	if g.banter.Chance() >= chance {
		return false
	}
	line := g.banter.Line(ev, g.cfg.DealerVulgar, g.fillVars(p, v))
	if line == "" {
		return false
	}
	g.handTalk++
	g.speak(line, prio)
	return true
}

func (g *Game) fillVars(p *player, v banter.Vars) banter.Vars {
	if p != nil {
		v.Name = p.user
		if prof, ok := g.store.Profile(p.user); ok {
			v.Net = prof.Net
			v.Balance = prof.Balance
		}
	}
	if v.Up == "" && len(g.dealer) > 0 {
		v.Up = upCardName(g.dealer[0])
	}
	return v
}

// busyTalking reports whether a new line at this priority has to wait.
func (g *Game) busyTalking(prio int, now time.Time) bool {
	cur := g.speech
	// a story only gets interrupted by something big
	if g.story != nil && prio < prioHigh {
		return true
	}
	if cur.prio == prioStory {
		cur.prio = prioLow
	}
	talking := now.Before(cur.until) || len(g.sayQueue) > 0
	if talking {
		age := now.Sub(cur.at)
		// let him finish the sentence unless something more important happens
		finish := banter.SpeakTime(cur.text)
		if prio < cur.prio || (prio == cur.prio && (age < finish+800*time.Millisecond || len(g.sayQueue) > 0)) || age < 1200*time.Millisecond {
			return true
		}
	}
	// at the normal pace he takes a breath between the small stuff
	if !g.cfg.TurboMode && prio < prioHigh && now.Before(cur.until.Add(3*time.Second)) {
		return true
	}
	return false
}

// speak puts a line in the bubble, a page at a time if it's long. If he was
// in the middle of a story, the story is put on hold and picks up afterwards.
func (g *Game) speak(text string, prio int) {
	if g.story != nil && g.story.held == nil {
		rest := g.sayQueue
		// if he got cut off mid-page, he'll say that page again
		if time.Since(g.speech.at) < banter.SpeakTime(g.speech.text) && g.speech.page.Text != "" {
			rest = append([]banter.Page{g.speech.page}, rest...)
		}
		g.story.held = rest
		if g.story.held == nil {
			g.story.held = []banter.Page{}
		}
	}
	g.sayQueue = banter.Pages(text)
	g.sayPrio = prio
	g.nextPage(false)
}

// tellStory starts one of his long stories.
func (g *Game) tellStory(p *player) bool {
	line := g.banter.Line(banter.EvStory, g.cfg.DealerVulgar, g.fillVars(p, banter.Vars{}))
	if line == "" {
		return false
	}
	g.lastStory = time.Now()
	g.handTalk++
	g.story = &storyRun{}
	g.sayQueue = banter.Pages(line)
	g.sayPrio = prioLow
	g.nextPage(false)
	return true
}

// nextPage shows the next page of whatever he's saying. cont marks a page
// that continues the previous one in the log.
func (g *Game) nextPage(cont bool) {
	if len(g.sayQueue) == 0 {
		return
	}
	pg := g.sayQueue[0]
	g.sayQueue = g.sayQueue[1:]
	now := time.Now()
	last := len(g.sayQueue) == 0
	d := g.pageTime(pg.Text, last)
	g.speech = speech{text: pg.Text, page: pg, id: g.speech.id + 1, prio: g.sayPrio, at: now, until: now.Add(d), cont: cont}
	if g.story != nil && g.story.held == nil {
		g.speech.prio = prioStory
	}
	g.logLine(proto.DealerLine{ID: g.speech.id, Text: pg.Text, Cont: cont, Para: cont && pg.Para, At: now.UnixMilli()})
	g.armSayTimer(d)
	g.broadcast()
}

// pageTime is how long a page stays up: the time to say it plus time to
// read it. Long lines stay longer.
func (g *Game) pageTime(text string, last bool) time.Duration {
	speak := banter.SpeakTime(text)
	if g.cfg.TurboMode {
		d := speak + 2500*time.Millisecond
		return min(max(d, 3500*time.Millisecond), 14*time.Second)
	}
	n := time.Duration(len(text))
	if !last {
		return speak + 1800*time.Millisecond + n*18*time.Millisecond
	}
	linger := min(3500*time.Millisecond+n*25*time.Millisecond, 9*time.Second)
	return max(speak+linger, 5*time.Second)
}

func (g *Game) armSayTimer(d time.Duration) {
	if g.sayTimer != nil {
		g.sayTimer.Stop()
	}
	g.sayTimer = time.NewTimer(d)
}

// onSayTimer runs when a page has been up long enough.
func (g *Game) onSayTimer() {
	g.sayTimer = nil
	if !g.cfg.DealerTrashTalk {
		g.sayQueue, g.story = nil, nil
		return
	}
	if len(g.sayQueue) > 0 {
		g.nextPage(true)
		return
	}
	if g.story == nil {
		return
	}
	if g.listeners() == 0 {
		g.story = nil // nobody left to hear it
		return
	}
	if g.story.held != nil {
		// back to the story after an interruption
		rest := g.story.held
		g.story.held = nil
		if len(rest) == 0 {
			g.story = nil
			return
		}
		resume := g.banter.Line(banter.EvResume, g.cfg.DealerVulgar, banter.Vars{})
		g.sayQueue = append(banter.Pages(resume), rest...)
		g.sayPrio = prioLow
		g.nextPage(false)
		return
	}
	g.story = nil
	g.lastStory = time.Now()
}

func (g *Game) listeners() int {
	n := 0
	for _, p := range g.seats {
		if p != nil && p.conn != nil {
			n++
		}
	}
	return n
}

func (g *Game) logLine(l proto.DealerLine) {
	g.sayLog = append(g.sayLog, l)
	if len(g.sayLog) > 80 {
		g.sayLog = append([]proto.DealerLine(nil), g.sayLog[len(g.sayLog)-80:]...)
	}
}

// storyRun is a story in progress. held is set while he's been interrupted:
// the pages still to tell once he's done with the interruption.
type storyRun struct {
	held []banter.Page
}

func (g *Game) greet(p *player) {
	prof, ok := g.store.Profile(p.user)
	if !ok {
		return
	}
	ev := banter.EvJoin
	switch {
	case prof.HandsPlayed == 0:
		ev = banter.EvJoinNew
	case prof.Net <= -1000:
		ev = banter.EvJoinDown
	case prof.Net >= 1000:
		ev = banter.EvJoinUp
	}
	g.say(ev, p, banter.Vars{}, 1, prioNormal)
}

// chatter runs at the start of a betting round: roast somebody's lifetime
// score, brag about the house, or ramble.
func (g *Game) chatter() {
	if !g.cfg.DealerTrashTalk {
		return
	}
	r := g.banter.Chance()
	var seated []*player
	for _, p := range g.seats {
		if p != nil && p.conn != nil {
			seated = append(seated, p)
		}
	}
	pick := func() *player { return seated[g.banter.Intn(len(seated))] }
	// now and then, a long story
	gap, chance := 15*time.Minute, 0.15
	if g.cfg.TurboMode {
		gap, chance = 25*time.Minute, 0.12
	}
	if len(seated) > 0 && g.story == nil && time.Since(g.lastStory) > gap && !time.Now().Before(g.speech.until) &&
		g.banter.Chance() < chance && g.tellStory(pick()) {
		return
	}
	if tier := g.store.House().Tier; tier != 0 && len(seated) > 0 && g.banter.Chance() < 0.3 {
		if tier > 0 {
			g.say(banter.EvFlex, pick(), banter.Vars{Item: banter.TierItems[tier]}, 1, prioLow)
		} else {
			g.say(banter.EvSympathy, nil, banter.Vars{}, 1, prioLow)
		}
		return
	}
	if len(seated) == 1 && g.banter.Chance() < 0.2 {
		g.say(banter.EvAlone, seated[0], banter.Vars{}, 1, prioLow)
		return
	}
	switch {
	case r < 0.16 && len(seated) > 0:
		p := pick()
		prof, _ := g.store.Profile(p.user)
		rich, _ := g.store.Tops()
		switch {
		case strings.EqualFold(g.store.Worst(), p.user):
			g.say(banter.EvRoastWorst, p, banter.Vars{}, 1, prioLow)
		case strings.EqualFold(rich, p.user):
			g.say(banter.EvRoastCrown, p, banter.Vars{}, 1, prioLow)
		case prof.Net <= -500:
			g.say(banter.EvRoastDown, p, banter.Vars{}, 1, prioLow)
		case prof.Net >= 500:
			g.say(banter.EvRoastUp, p, banter.Vars{}, 1, prioLow)
		default:
			g.say(banter.EvInvent, p, banter.Vars{}, 1, prioLow)
		}
	case r < 0.28 && len(seated) > 0:
		g.say(banter.EvInvent, pick(), banter.Vars{}, 1, prioLow)
	case r < 0.34 && len(seated) > 0:
		g.say(banter.EvHeadline, pick(), banter.Vars{}, 1, prioLow)
	case r < 0.39 && len(seated) > 0:
		g.say(banter.EvRating, pick(), banter.Vars{}, 1, prioLow)
	case r < 0.45 && len(seated) > 0:
		g.say(banter.EvBetOpen, pick(), banter.Vars{}, 1, prioLow)
	case r < 0.50:
		h := g.store.House()
		if h.Net >= 0 {
			g.say(banter.EvHouseUp, nil, banter.Vars{Amount: h.Net}, 1, prioLow)
		} else {
			g.say(banter.EvHouseDown, nil, banter.Vars{Amount: -h.Net}, 1, prioLow)
		}
	case r < 0.58:
		g.say(banter.EvBackstory, nil, banter.Vars{}, 1, prioLow)
	case r < 0.66:
		g.say(banter.EvOpinion, nil, banter.Vars{}, 1, prioLow)
	case r < 0.70:
		g.say(banter.EvCryptic, nil, banter.Vars{}, 1, prioLow)
	case r < 0.74:
		g.say(banter.EvIdle, nil, banter.Vars{}, 1, prioLow)
	case r < 0.79:
		g.say(banter.EvTip, nil, banter.Vars{}, 1, prioLow)
	case r < 0.83:
		g.say(banter.EvPhone, nil, banter.Vars{}, 1, prioLow)
	case r < 0.87 && len(seated) > 0:
		g.say(banter.EvComp, pick(), banter.Vars{}, 1, prioLow)
	case r < 0.91:
		g.say(banter.EvRule, nil, banter.Vars{}, 1, prioLow)
	case r < 0.94:
		g.say(banter.EvAffirm, nil, banter.Vars{}, 1, prioLow)
	case r < 0.97:
		g.say(banter.EvSong, nil, banter.Vars{}, 1, prioLow)
	}
}

// heckleTurn sometimes picks on whoever's turn it is.
func (g *Game) heckleTurn(p *player) {
	if g.cfg.DealerTrashTalk {
		g.say(banter.EvInvent, p, banter.Vars{}, 0.1, prioLow)
	}
}

// reactToAction comments on a player's decision. before is the hand total
// (and softness) before the action.
func (g *Game) reactToAction(p *player, act string, h *bj.Hand, before int, soft bool, splitTens bool) {
	if !g.cfg.DealerTrashTalk {
		return
	}
	t, _ := h.Total()
	upVal := 0
	if len(g.dealer) > 0 {
		upVal = g.dealer[0].Value()
		if g.dealer[0].Rank == 1 {
			upVal = 11
		}
	}
	switch act {
	case "stand":
		switch {
		case before >= 20 && !soft:
			g.say(banter.EvStandGood, p, banter.Vars{Total: before}, 0.08, prioLow)
		case before <= 11:
			g.say(banter.EvStandTiny, p, banter.Vars{Total: before}, 0.95, prioHigh)
		case before <= 16 && !soft && upVal >= 7:
			g.say(banter.EvCoward, p, banter.Vars{Total: before}, 0.6, prioNormal)
		case before <= 16 && soft:
			g.say(banter.EvCoward, p, banter.Vars{Total: before}, 0.5, prioNormal)
		}
	case "hit", "double":
		greedy := act == "hit" && before >= 17 && !soft
		switch {
		case t > 21 && greedy:
			g.say(banter.EvBustGreedy, p, banter.Vars{Total: t}, 1, prioHigh)
		case t > 21:
			g.say(banter.EvBust, p, banter.Vars{Total: t}, 0.8, prioHigh)
		case greedy:
			g.say(banter.EvHit17, p, banter.Vars{Total: before}, 0.7, prioNormal)
		case t == 21 && len(h.Cards) >= 3:
			g.say(banter.EvTwentyOne, p, banter.Vars{Total: t}, 0.5, prioNormal)
		case len(h.Cards) >= 5:
			g.say(banter.EvFiveCard, p, banter.Vars{Total: t}, 0.6, prioNormal)
		case act == "double":
			g.say(banter.EvDouble, p, banter.Vars{}, 0.35, prioLow)
		}
	case "split":
		if splitTens {
			g.say(banter.EvSplitTens, p, banter.Vars{}, 0.95, prioHigh)
		} else {
			g.say(banter.EvSplit, p, banter.Vars{}, 0.3, prioLow)
		}
	case "surrender":
		g.say(banter.EvSurrender, p, banter.Vars{}, 0.9, prioNormal)
	}
}

type roundResult struct {
	p          *player
	delta      int64
	before     int64 // balance before the round (stake included)
	doubleLost bool
	doubleWon  bool
	pushed     bool
}

// reactToResults picks the juiciest thing to say about a settled round.
func (g *Game) reactToResults(res []roundResult, houseDelta int64, dealerBust, dealerBJ bool) {
	if !g.cfg.DealerTrashTalk || len(res) == 0 {
		return
	}
	tier := g.store.House().Tier
	// someone went broke
	for _, r := range res {
		if g.store.Balance(r.p.user) < g.cfg.MinBet {
			if g.say(banter.EvBroke, r.p, banter.Vars{}, 0.85, prioHigh) {
				return
			}
		}
	}
	var loser, winner *roundResult
	allLost := true
	for i := range res {
		r := &res[i]
		if loser == nil || r.delta < loser.delta {
			loser = r
		}
		if winner == nil || r.delta > winner.delta {
			winner = r
		}
		if r.delta >= 0 {
			allLost = false
		}
	}
	if loser != nil && loser.delta < 0 {
		lost := -loser.delta
		if lost >= 200 && (lost >= 500 || loser.before <= 0 || float64(lost) >= 0.3*float64(loser.before)) {
			if g.say(banter.EvBigLoss, loser.p, banter.Vars{Amount: lost}, 0.9, prioHigh) {
				return
			}
		}
	}
	// the house lost the round: get petty, extra petty when he's broke
	if houseDelta < 0 && winner != nil && winner.delta > 0 {
		chance := 0.35
		if tier < 0 {
			chance = 0.85
		}
		if g.say(banter.EvPetty, winner.p, banter.Vars{Amount: winner.delta}, chance, prioNormal) {
			return
		}
	}
	if dealerBust && g.say(banter.EvDealerBust, nil, banter.Vars{}, 0.7, prioNormal) {
		return
	}
	for _, r := range res {
		if r.doubleLost && g.say(banter.EvDoubleLose, r.p, banter.Vars{}, 0.6, prioNormal) {
			return
		}
	}
	// he drew out on people: dealer made 21 the hard way and won
	if dt, _ := bj.Total(g.dealer); dt == 21 && len(g.dealer) >= 3 && houseDelta > 0 &&
		g.say(banter.EvDealer21, nil, banter.Vars{}, 0.6, prioNormal) {
		return
	}
	for _, r := range res {
		if r.doubleWon && g.say(banter.EvDoubleWin, r.p, banter.Vars{Amount: r.delta}, 0.45, prioNormal) {
			return
		}
	}
	if winner != nil && winner.delta >= 500 && g.say(banter.EvBigWin, winner.p, banter.Vars{Amount: winner.delta}, 0.6, prioNormal) {
		return
	}
	if allLost && len(res) >= 2 && !dealerBJ && g.say(banter.EvSweep, nil, banter.Vars{}, 0.8, prioNormal) {
		return
	}
	for _, r := range res {
		if s := g.store.LoseStreak(r.p.user); s >= 4 {
			if g.say(banter.EvLoseStreak, r.p, banter.Vars{Streak: s}, 0.5, prioNormal) {
				return
			}
		}
		if s := g.store.WinStreak(r.p.user); s >= 3 {
			if g.say(banter.EvWinStreak, r.p, banter.Vars{Streak: s}, 0.5, prioNormal) {
				return
			}
		}
	}
	for _, r := range res {
		if r.pushed && r.delta == 0 && g.say(banter.EvPush, r.p, banter.Vars{}, 0.35, prioLow) {
			return
		}
	}
	// nothing juicy happened: make something up about somebody
	if g.say(banter.EvInvent, res[g.banter.Intn(len(res))].p, banter.Vars{}, 0.25, prioLow) {
		return
	}
	g.say(banter.EvCryptic, nil, banter.Vars{}, 0.08, prioLow)
}

// tierChanged announces the dealer's new outfit. The biggest loser of the
// round gets thanked for upgrades; the biggest winner gets blamed for
// downgrades.
func (g *Game) tierChanged(from, to int, res []roundResult) {
	var loser, winner *player
	var lo, hi int64
	for _, r := range res {
		if loser == nil || r.delta < lo {
			loser, lo = r.p, r.delta
		}
		if winner == nil || r.delta > hi {
			winner, hi = r.p, r.delta
		}
	}
	name := banter.TierNames[to]
	g.log.Printf("dealer wealth tier %d -> %d (%s)", from, to, name)
	if to > from {
		g.eventAll(proto.Event{Kind: "info", Seat: -1, Text: "The dealer upgraded his look: " + name + "!"})
	} else {
		g.eventAll(proto.Event{Kind: "info", Seat: -1, Text: "The dealer is looking rough: " + name + "."})
	}
	// talking is optional; the outfit changes either way. This beats
	// whatever he was saying.
	g.forceSay = true
	defer func() { g.forceSay = false }()
	switch {
	case to > from && to > 0:
		g.say(banter.EvUpgrade, loser, banter.Vars{Item: banter.TierItems[to]}, 1, prioHigh)
	case to > from:
		g.say(banter.EvRecover, loser, banter.Vars{Item: banter.RecoverItems[to]}, 1, prioHigh)
	case to >= 0:
		g.say(banter.EvDowngrade, winner, banter.Vars{Item: banter.TierItems[from]}, 1, prioHigh)
	default:
		g.say(banter.EvFallen, winner, banter.Vars{Item: banter.TierItems[to]}, 1, prioHigh)
	}
	g.broadcast()
}
