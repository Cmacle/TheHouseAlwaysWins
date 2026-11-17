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
	)
	return r.Replace(line)
}
