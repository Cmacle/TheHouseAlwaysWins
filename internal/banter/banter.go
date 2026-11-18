// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package banter is the dealer's trash talk: event-keyed line pools and a
// picker that avoids repeating itself.
package banter

import (
	crand "crypto/rand"
	"math/rand/v2"
	"strconv"
	"strings"
)

// Events the dealer can react to.
const (
	EvJoin       = "join"
	EvJoinNew    = "join_new"
	EvJoinDown   = "join_down"
	EvJoinUp     = "join_up"
	EvRoastDown  = "roast_down"
	EvRoastUp    = "roast_up"
	EvRoastWorst = "roast_worst"
	EvRoastCrown = "roast_crown"
	EvBust       = "bust"
	EvBustGreedy = "bust_greedy"
	EvCoward     = "coward"
	EvStandTiny  = "stand_tiny"
	EvHit17      = "hit_17"
	EvDouble     = "double"
	EvDoubleLose = "double_lose"
	EvSplitTens  = "split_tens"
	EvSurrender  = "surrender"
	EvPlayerBJ   = "player_bj"
	EvDealerBJ   = "dealer_bj"
	EvDealerBust = "dealer_bust"
	EvBigLoss    = "big_loss"
	EvBigWin     = "big_win"
	EvSweep      = "sweep"
	EvLoseStreak = "lose_streak"
	EvBroke      = "broke"
	EvAllIn      = "all_in"
	EvMinBet     = "min_bet"
	EvMaxBet     = "max_bet"
	EvNoBet      = "no_bet"
	EvShuffle    = "shuffle"
	EvCrown      = "crown"
	EvIdle       = "idle"
	EvHouseUp    = "house_up"
	EvHouseDown  = "house_down"
	EvUpgrade    = "upgrade"   // tier went up (positive): {item} he bought, {name} who paid
	EvDowngrade  = "downgrade" // tier went down but still >= 0: {item} he lost
	EvFallen     = "fallen"    // fell into a negative tier: {item} he lost
	EvRecover    = "recover"   // climbed back toward even: {item} he got back
	EvFlex       = "flex"      // bragging about current riches: {item}
	EvSympathy   = "sympathy"  // while down: begging, badly
	EvPetty      = "petty"     // house lost a round: {name} won {amount}

	// betting
	EvDare     = "dare"      // dare {name} to bet {amount}
	EvBetSlow  = "bet_slow"  // betting clock running out on {name}
	EvBetUp    = "bet_up"    // {name} raised their bet a lot ({amount})
	EvBetDown  = "bet_down"  // {name} shrank their bet after a loss
	EvChase    = "chase"     // {name} raised right after losing {amount}
	EvBetBig   = "bet_big"   // {name} bet a big chunk ({amount}) of their stack
	EvFirstBet = "first_bet" // {name} bet first
	EvLastBet  = "last_bet"  // {name} bet last, everyone waiting
	EvBetOpen  = "bet_open"  // taunting the table to bet big ({name} optional)

	// playing
	EvTurnSlow   = "turn_slow"   // {name}'s turn clock running out
	EvTwentyOne  = "twenty_one"  // {name} hit to 21
	EvFiveCard   = "five_card"   // {name} has five cards and hasn't busted ({total})
	EvStandGood  = "stand_good"  // {name} stood on 20 (correctly, annoyingly)
	EvSplit      = "split"       // {name} split something other than tens
	EvDoubleWin  = "double_win"  // {name} doubled and won {amount}
	EvPush       = "push"        // {name} tied him
	EvDealerAce  = "dealer_ace"  // he's showing an ace
	EvDealerWeak = "dealer_weak" // he's showing a 4, 5 or 6 ({up})
	EvDealer21   = "dealer_21"   // he drew to 21 and won
	EvWinStreak  = "win_streak"  // {name} won {streak} in a row
	EvLeave      = "leave"       // {name} walked away
	EvRefill     = "refill"      // {name} got the daily refill of {amount}
	EvGift       = "gift"        // {name} sent {item} {amount} chips

	// Texas Hold'em ({name} is a player, {amount} chips, {item} a hand name)
	EvPkHello     = "pk_hello"      // {name} sat down at his poker table
	EvPkSolo      = "pk_solo"       // just him and {name}, heads up
	EvPkLeave     = "pk_leave"      // {name} left with {amount}
	EvPkFold      = "pk_fold"       // {name} folded (having put in {amount})
	EvPkCheck     = "pk_check"      // {name} checked
	EvPkCall      = "pk_call"       // {name} called a big bet of {amount}
	EvPkRaise     = "pk_raise"      // {name} bet or raised to {amount}
	EvPkAllIn     = "pk_allin"      // {name} went all in ({amount})
	EvPkWin       = "pk_win"        // {name} won {amount} ({item} at a showdown)
	EvPkBigPot    = "pk_big_pot"    // {name} took down a big pot of {amount}
	EvPkMonster   = "pk_monster"    // {name} showed {item} (a full house or better)
	EvPkRoyal     = "pk_royal"      // {name} made a royal flush
	EvPkBadBeat   = "pk_bad_beat"   // {name} lost a strong hand to him ({item} won)
	EvPkBust      = "pk_bust"       // {name} is out of chips at the table
	EvPkRebuy     = "pk_rebuy"      // {name} added {amount} chips
	EvPkSlow      = "pk_slow"       // {name} is taking forever
	EvPkFlop      = "pk_flop"       // a scary flop (all one suit, or paired)
	EvPkDaleRaise = "pk_dale_raise" // he bets or raises
	EvPkDaleCall  = "pk_dale_call"  // he calls a big bet
	EvPkDaleCheck = "pk_dale_check" // he checks
	EvPkDaleFold  = "pk_dale_fold"  // he folds
	EvPkDaleAllIn = "pk_dale_allin" // he shoves
	EvPkDaleWin   = "pk_dale_win"   // he won {amount} (from {name})
	EvPkDaleLose  = "pk_dale_lose"  // {name} beat him with {item} for {amount}
	EvPkDaleBluff = "pk_dale_bluff" // he stole the pot and shows the bluff

	// baccarat
	EvBacHello   = "bac_hello"    // {name} sat down at baccarat
	EvBacDeal    = "bac_deal"     // betting's closed, here come the cards
	EvBacNatural = "bac_natural"  // {item} (Player or Banker) has a natural {amount}
	EvBacPlayer  = "bac_player"   // Player wins
	EvBacBanker  = "bac_banker"   // Banker wins
	EvBacTie     = "bac_tie"      // a tie
	EvBacBigWin  = "bac_big_win"  // {name} won {amount}
	EvBacBigLose = "bac_big_lose" // {name} lost {amount}
	EvBacStreak  = "bac_streak"   // {item} has won {amount} in a row

	// Red Dog
	EvRdHello   = "rd_hello"   // {name} sat down at Red Dog
	EvRdSpread  = "rd_spread"  // a spread of {amount}, paying {item}
	EvRdConsec  = "rd_consec"  // consecutive cards: a push
	EvRdPair    = "rd_pair"    // a pair: drawing for three of a kind
	EvRdTrips   = "rd_trips"   // three of a kind, 11 to 1
	EvRdRaise   = "rd_raise"   // {name} doubled their bet
	EvRdWin     = "rd_win"     // {name} won {amount}
	EvRdMiss    = "rd_miss"    // the third card missed: the house wins
	EvRdGutshot = "rd_gutshot" // a one-card spread hit

	// the Beggars Table
	EvBgHello   = "bg_hello"    // {name} came begging
	EvBgRoll    = "bg_roll"     // {name} rolls again (now and then)
	EvBgMany    = "bg_many"     // {name} has rolled {amount} times
	EvBgWin     = "bg_win"      // {name} rolled five sixes and gets {amount}
	EvBgTooRich = "bg_too_rich" // {name} has {amount} chips: too many to beg

	// between hands, other ways to run his mouth
	EvAlone    = "alone"    // only {name} at the table
	EvHeadline = "headline" // fake breaking news (often about {name})
	EvRating   = "rating"   // rates {name}
	EvTip      = "tip"      // terrible advice
	EvPhone    = "phone"    // on the phone with somebody from his past
	EvComp     = "comp"     // talking to Comp the iguana (about {name})
	EvRule     = "rule"     // made-up house rules
	EvAffirm   = "affirm"   // daily affirmations
	EvSong     = "song"     // a bit of a lounge song (his own)
)

// Vars fill the placeholders in a line.
type Vars struct {
	Name    string
	Amount  int64
	Total   int
	Net     int64
	Balance int64
	Streak  int
	Up      string
	Item    string
}

// Picker hands out lines without repeating: each event's pool is dealt like a
// shuffled deck, and recently used lines are skipped across events.
type Picker struct {
	rng    *rand.Rand
	decks  map[string][]string
	recent []string
}

// NewPicker creates a picker.
func NewPicker() *Picker {
	var seed [32]byte
	_, _ = crand.Read(seed[:])
	return &Picker{rng: rand.New(rand.NewChaCha8(seed)), decks: map[string][]string{}}
}

// Pool returns the lines available for an event.
func Pool(ev string, vulgarOn bool) []string {
	if ev == EvStory || ev == EvResume {
		// stories have a clean and a vulgar telling of the same thing
		if vulgarOn {
			return append([]string(nil), vulgar[ev]...)
		}
		return append([]string(nil), clean[ev]...)
	}
	out := append([]string(nil), clean[ev]...)
	if vulgarOn {
		// vulgar mode leans hard on the filthy lines
		out = append(out, vulgar[ev]...)
		out = append(out, vulgar[ev]...)
	}
	return out
}

// Count reports how many distinct lines exist (for docs and tests).
func Count() (cleanN, vulgarN int) {
	for _, l := range clean {
		cleanN += len(l)
	}
	for _, l := range vulgar {
		vulgarN += len(l)
	}
	return
}

// Events lists every event key that has lines.
func Events() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string][]string{clean, vulgar} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// Chance returns a random float in [0,1).
func (p *Picker) Chance() float64 { return p.rng.Float64() }

// Intn returns a random int in [0,n).
func (p *Picker) Intn(n int) int { return p.rng.IntN(n) }

// Line picks and fills a line for an event, or "" if there are none.
func (p *Picker) Line(ev string, vulgarOn bool, v Vars) string {
	key := ev
	if vulgarOn {
		key += "/v"
	}
	for tries := 0; tries < 6; tries++ {
		deck := p.decks[key]
		if len(deck) == 0 {
			deck = Pool(ev, vulgarOn)
			if len(deck) == 0 {
				return ""
			}
			p.rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		}
		line := deck[len(deck)-1]
		p.decks[key] = deck[:len(deck)-1]
		if p.isRecent(line) && tries < 5 {
			continue
		}
		p.remember(line)
		return Fill(line, v)
	}
	return ""
}

func (p *Picker) isRecent(l string) bool {
	for _, r := range p.recent {
		if r == l {
			return true
		}
	}
	return false
}

func (p *Picker) remember(l string) {
	p.recent = append(p.recent, l)
	if len(p.recent) > 60 {
		p.recent = p.recent[len(p.recent)-60:]
	}
}

// Money formats an amount like $1,234 (or -$1,234).
func Money(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-$" + b.String()
	}
	return "$" + b.String()
}

func signedMoney(n int64) string {
	if n > 0 {
		return "+" + Money(n)
	}
	return Money(n)
}

// Fill replaces placeholders.
func Fill(line string, v Vars) string {
	r := strings.NewReplacer(
		"{name}", v.Name,
		"{amount}", Money(v.Amount),
		"{total}", strconv.Itoa(v.Total),
		"{net}", signedMoney(v.Net),
		"{balance}", Money(v.Balance),
		"{streak}", strconv.Itoa(v.Streak),
		"{up}", v.Up,
		"{item}", v.Item,
	)
	return r.Replace(line)
}
