// Package cosmetics is the catalogue of unlockable hats, avatar frames and
// titles. It is shared by the server (which decides unlocks) and the client
// (which shows requirements and progress).
//
// Goals are tuned around the default $1,000 daily refill, a $10 minimum and a
// $1,000 maximum bet: common items arrive in the first session, legendary ones
// take serious time (or serious luck).
package cosmetics

import (
	"fmt"
	"strconv"
	"strings"
)

// Kinds of cosmetic.
const (
	Hat   = "hat"
	Frame = "frame"
	Title = "title"
)

// Tiers, easiest to hardest. Unique items belong to one player at a time.
const (
	Common = iota
	Uncommon
	Rare
	Epic
	Legendary
	Unique
)

// TierNames are display names for tiers.
var TierNames = []string{"Common", "Uncommon", "Rare", "Epic", "Legendary", "Unique"}

// TierColors are hex colours for tiers.
var TierColors = []string{"#c8c8c8", "#6fdc6f", "#5aa9ff", "#c77dff", "#ffb02e", "#ff5c8a"}

// Metrics an item can depend on.
const (
	MWagered    = "wagered"     // total amount bet, doubles and splits included
	MPeak       = "peak"        // most chips held at once
	MNetUp      = "net_up"      // lifetime gain at least Goal
	MNetDown    = "net_down"    // lifetime loss at least Goal (net <= -Goal)
	MHands      = "hands"       // hands played
	MBlackjacks = "blackjacks"  // naturals dealt
	MBiggestWin = "biggest_win" // largest single-round win
	MWinStreak  = "win_streak"  // best run of winning hands
	MLoseStreak = "lose_streak" // worst run of losing hands
	MBusts      = "busts"
	MDoublesWon = "doubles_won"
	MSplits     = "splits"
	MRefills    = "refills"   // daily refills received
	MComeback   = "comeback"  // was down Goal or more, then got back to even
	MRankRich   = "rank_rich" // currently #1 richest (unique)
	MRankNet    = "rank_net"  // currently #1 lifetime (unique)
	MFree       = "free"      // everyone has it
)

// Item is one unlockable.
type Item struct {
	ID     string
	Kind   string
	Name   string
	Desc   string // flavour text
	Tier   int
	Metric string
	Goal   int64
	Sprite int // index into hats.png / frames.png
}

// Stats are the numbers unlocks are based on.
type Stats struct {
	Wagered, Peak, Net, LowestNet, BiggestWin                                             int64
	Hands, Blackjacks, BestWinStreak, WorstLoseStreak, Busts, DoublesWon, Splits, Refills int
	RankRich, RankNet                                                                     bool
}

// Catalog lists every cosmetic, in display order.
var Catalog = []Item{
	// ------------------------------------------------ hats
	{ID: "party_hat", Kind: Hat, Name: "Party Hat", Desc: "Every journey starts with a single bet.", Tier: Common, Metric: MWagered, Goal: 1_000, Sprite: 0},
	{ID: "ball_cap", Kind: Hat, Name: "Ball Cap", Desc: "You're a regular now.", Tier: Common, Metric: MHands, Goal: 25, Sprite: 1},
	{ID: "beanie", Kind: Hat, Name: "Beanie", Desc: "Warm, cozy, slightly in debt.", Tier: Common, Metric: MWagered, Goal: 5_000, Sprite: 2},
	{ID: "propeller", Kind: Hat, Name: "Propeller Beanie", Desc: "Hours of strategic thinking.", Tier: Uncommon, Metric: MHands, Goal: 250, Sprite: 3},
	{ID: "cowboy", Kind: Hat, Name: "Cowboy Hat", Desc: "Yeehaw, high roller.", Tier: Uncommon, Metric: MWagered, Goal: 25_000, Sprite: 4},
	{ID: "tinfoil", Kind: Hat, Name: "Tin Foil Hat", Desc: "The deck is rigged. Obviously.", Tier: Uncommon, Metric: MNetDown, Goal: 3_000, Sprite: 5},
	{ID: "sombrero", Kind: Hat, Name: "Sombrero", Desc: "Doubled the refill. Fiesta time.", Tier: Uncommon, Metric: MPeak, Goal: 2_000, Sprite: 6},
	{ID: "cone", Kind: Hat, Name: "Traffic Cone", Desc: "Caution: frequently over 21.", Tier: Uncommon, Metric: MBusts, Goal: 50, Sprite: 7},
	{ID: "top_hat", Kind: Hat, Name: "Top Hat", Desc: "A gentleman of means. Allegedly.", Tier: Rare, Metric: MWagered, Goal: 100_000, Sprite: 8},
	{ID: "fez", Kind: Hat, Name: "Fez", Desc: "Fezzes are cool. So are blackjacks.", Tier: Rare, Metric: MBlackjacks, Goal: 25, Sprite: 9},
	{ID: "pirate", Kind: Hat, Name: "Pirate Hat", Desc: "Plundered five times the refill.", Tier: Rare, Metric: MPeak, Goal: 5_000, Sprite: 10},
	{ID: "dunce", Kind: Hat, Name: "Dunce Cap", Desc: "Ten grand in the hole. Corner, please.", Tier: Rare, Metric: MNetDown, Goal: 10_000, Sprite: 11},
	{ID: "viking", Kind: Hat, Name: "Viking Helmet", Desc: "Six wins in a row. Raid the dealer!", Tier: Rare, Metric: MWinStreak, Goal: 6, Sprite: 12},
	{ID: "halo", Kind: Hat, Name: "Halo", Desc: "Up five thousand lifetime. Blessed.", Tier: Epic, Metric: MNetUp, Goal: 5_000, Sprite: 13},
	{ID: "devil", Kind: Hat, Name: "Devil Horns", Desc: "Ten losses in a row. Sold your soul?", Tier: Epic, Metric: MLoseStreak, Goal: 10, Sprite: 14},
	{ID: "wizard", Kind: Hat, Name: "Wizard Hat", Desc: "Half a million wagered. Pure sorcery.", Tier: Epic, Metric: MWagered, Goal: 500_000, Sprite: 15},
	{ID: "unicorn", Kind: Hat, Name: "Unicorn Horn", Desc: "Won $2,000+ in a single round.", Tier: Epic, Metric: MBiggestWin, Goal: 2_000, Sprite: 16},
	{ID: "grad_cap", Kind: Hat, Name: "Graduation Cap", Desc: "Two thousand hands. Doctorate in 21.", Tier: Epic, Metric: MHands, Goal: 2_000, Sprite: 17},
	{ID: "gold_top_hat", Kind: Hat, Name: "Golden Top Hat", Desc: "Up $25,000 lifetime. Beat the house.", Tier: Legendary, Metric: MNetUp, Goal: 25_000, Sprite: 18},
	{ID: "tiara", Kind: Hat, Name: "Diamond Tiara", Desc: "Held $25,000 at once. Dripping.", Tier: Legendary, Metric: MPeak, Goal: 25_000, Sprite: 19},
	{ID: "flame", Kind: Hat, Name: "Hair On Fire", Desc: "Ten wins in a row. Absolutely ablaze.", Tier: Legendary, Metric: MWinStreak, Goal: 10, Sprite: 20},
	{ID: "space", Kind: Hat, Name: "Space Helmet", Desc: "$2.5 million wagered. To the moon.", Tier: Legendary, Metric: MWagered, Goal: 2_500_000, Sprite: 21},
	{ID: "crown", Kind: Hat, Name: "The Crown", Desc: "Worn by whoever has the most chips right now.", Tier: Unique, Metric: MRankRich, Sprite: 22},
	{ID: "laurel", Kind: Hat, Name: "Laurel Wreath", Desc: "Worn by the #1 lifetime earner.", Tier: Unique, Metric: MRankNet, Sprite: 23},

	// ------------------------------------------------ frames
	{ID: "frame_wood", Kind: Frame, Name: "Plain Frame", Desc: "Simple. Honest. Wooden.", Tier: Common, Metric: MFree, Sprite: 0},
	{ID: "frame_felt", Kind: Frame, Name: "Felt Frame", Desc: "Fresh off the table.", Tier: Common, Metric: MWagered, Goal: 2_500, Sprite: 1},
	{ID: "frame_silver", Kind: Frame, Name: "Silver Frame", Desc: "A hundred hands of commitment.", Tier: Uncommon, Metric: MHands, Goal: 100, Sprite: 2},
	{ID: "frame_gold", Kind: Frame, Name: "Gold Frame", Desc: "Held $3,000 at once.", Tier: Rare, Metric: MPeak, Goal: 3_000, Sprite: 3},
	{ID: "frame_ruby", Kind: Frame, Name: "Ruby Frame", Desc: "Ten blackjacks dealt.", Tier: Rare, Metric: MBlackjacks, Goal: 10, Sprite: 4},
	{ID: "frame_ice", Kind: Frame, Name: "Frozen Frame", Desc: "Seven losses in a row. Ice cold.", Tier: Rare, Metric: MLoseStreak, Goal: 7, Sprite: 5},
	{ID: "frame_bone", Kind: Frame, Name: "Bone Frame", Desc: "Down $25,000 lifetime. Skeleton crew.", Tier: Epic, Metric: MNetDown, Goal: 25_000, Sprite: 6},
	{ID: "frame_fire", Kind: Frame, Name: "Inferno Frame", Desc: "Eight wins in a row.", Tier: Epic, Metric: MWinStreak, Goal: 8, Sprite: 7},
	{ID: "frame_diamond", Kind: Frame, Name: "Diamond Frame", Desc: "Held $10,000 at once.", Tier: Legendary, Metric: MPeak, Goal: 10_000, Sprite: 8},
	{ID: "frame_rainbow", Kind: Frame, Name: "Rainbow Frame", Desc: "A million wagered. Taste the rainbow.", Tier: Legendary, Metric: MWagered, Goal: 1_000_000, Sprite: 9},

	// ------------------------------------------------ titles
	{ID: "t_rookie", Kind: Title, Name: "Rookie", Desc: "Fresh meat.", Tier: Common, Metric: MFree},
	{ID: "t_regular", Kind: Title, Name: "Regular", Desc: "Play 50 hands.", Tier: Common, Metric: MHands, Goal: 50},
	{ID: "t_lucky", Kind: Title, Name: "Lucky", Desc: "Five wins in a row.", Tier: Uncommon, Metric: MWinStreak, Goal: 5},
	{ID: "t_cursed", Kind: Title, Name: "Cursed", Desc: "Seven losses in a row.", Tier: Uncommon, Metric: MLoseStreak, Goal: 7},
	{ID: "t_big_fish", Kind: Title, Name: "Big Fish", Desc: "Hold $2,000 at once.", Tier: Uncommon, Metric: MPeak, Goal: 2_000},
	{ID: "t_in_the_red", Kind: Title, Name: "In The Red", Desc: "Be down $2,000 lifetime.", Tier: Uncommon, Metric: MNetDown, Goal: 2_000},
	{ID: "t_bust_master", Kind: Title, Name: "Bust Master", Desc: "Bust 100 times.", Tier: Uncommon, Metric: MBusts, Goal: 100},
	{ID: "t_splitter", Kind: Title, Name: "Splitter", Desc: "Split 25 times.", Tier: Uncommon, Metric: MSplits, Goal: 25},
	{ID: "t_refill_regular", Kind: Title, Name: "Refill Regular", Desc: "Receive 10 daily refills.", Tier: Uncommon, Metric: MRefills, Goal: 10},
	{ID: "t_shark", Kind: Title, Name: "Shark", Desc: "Be up $2,500 lifetime.", Tier: Rare, Metric: MNetUp, Goal: 2_500},
	{ID: "t_high_roller", Kind: Title, Name: "High Roller", Desc: "Wager $100,000 total.", Tier: Rare, Metric: MWagered, Goal: 100_000},
	{ID: "t_down_bad", Kind: Title, Name: "Down Bad", Desc: "Be down $10,000 lifetime.", Tier: Rare, Metric: MNetDown, Goal: 10_000},
	{ID: "t_double_trouble", Kind: Title, Name: "Double Trouble", Desc: "Win 25 doubled hands.", Tier: Rare, Metric: MDoublesWon, Goal: 25},
	{ID: "t_degenerate", Kind: Title, Name: "Professional Degenerate", Desc: "Receive 30 daily refills.", Tier: Rare, Metric: MRefills, Goal: 30},
	{ID: "t_card_shark", Kind: Title, Name: "Card Shark", Desc: "Get 50 blackjacks.", Tier: Epic, Metric: MBlackjacks, Goal: 50},
	{ID: "t_on_fire", Kind: Title, Name: "On Fire", Desc: "Nine wins in a row.", Tier: Epic, Metric: MWinStreak, Goal: 9},
	{ID: "t_comeback", Kind: Title, Name: "Comeback Kid", Desc: "Fall $5,000 behind lifetime, then climb back to even.", Tier: Epic, Metric: MComeback, Goal: 5_000},
	{ID: "t_rock_bottom", Kind: Title, Name: "Rock Bottom", Desc: "Be down $50,000 lifetime.", Tier: Epic, Metric: MNetDown, Goal: 50_000},
	{ID: "t_iron_butt", Kind: Title, Name: "Iron Butt", Desc: "Play 5,000 hands.", Tier: Epic, Metric: MHands, Goal: 5_000},
	{ID: "t_whale", Kind: Title, Name: "Whale", Desc: "Wager $1,000,000 total.", Tier: Legendary, Metric: MWagered, Goal: 1_000_000},
	{ID: "t_tycoon", Kind: Title, Name: "Tycoon", Desc: "Hold $50,000 at once.", Tier: Legendary, Metric: MPeak, Goal: 50_000},
	{ID: "t_legend", Kind: Title, Name: "Living Legend", Desc: "Be up $50,000 lifetime.", Tier: Legendary, Metric: MNetUp, Goal: 50_000},
	{ID: "t_owns_house", Kind: Title, Name: "Owns The House", Desc: "Have the most chips right now.", Tier: Unique, Metric: MRankRich},
	{ID: "t_goat", Kind: Title, Name: "The G.O.A.T.", Desc: "Be #1 on the lifetime leaderboard.", Tier: Unique, Metric: MRankNet},
}

var byID = func() map[string]*Item {
	m := map[string]*Item{}
	for i := range Catalog {
		m[Catalog[i].ID] = &Catalog[i]
	}
	return m
}()

// Get looks up an item.
func Get(id string) (*Item, bool) { it, ok := byID[id]; return it, ok }

// OfKind lists items of one kind in catalogue order.
func OfKind(kind string) []*Item {
	var out []*Item
	for i := range Catalog {
		if Catalog[i].Kind == kind {
			out = append(out, &Catalog[i])
		}
	}
	return out
}

// Dynamic reports whether an item is held only while a condition stays true
// (the #1 spots). Everything else is unlocked permanently.
func (it *Item) Dynamic() bool { return it.Metric == MRankRich || it.Metric == MRankNet }

// Progress returns how far a player is toward an item.
func (it *Item) Progress(s Stats) (cur, goal int64) {
	switch it.Metric {
	case MWagered:
		return s.Wagered, it.Goal
	case MPeak:
		return s.Peak, it.Goal
	case MNetUp:
		return s.Net, it.Goal
	case MNetDown:
		return -s.Net, it.Goal
	case MHands:
		return int64(s.Hands), it.Goal
	case MBlackjacks:
		return int64(s.Blackjacks), it.Goal
	case MBiggestWin:
		return s.BiggestWin, it.Goal
	case MWinStreak:
		return int64(s.BestWinStreak), it.Goal
	case MLoseStreak:
		return int64(s.WorstLoseStreak), it.Goal
	case MBusts:
		return int64(s.Busts), it.Goal
	case MDoublesWon:
		return int64(s.DoublesWon), it.Goal
	case MSplits:
		return int64(s.Splits), it.Goal
	case MRefills:
		return int64(s.Refills), it.Goal
	case MComeback:
		if -s.LowestNet < it.Goal {
			return -s.LowestNet, it.Goal
		}
		if s.Net >= 0 {
			return it.Goal, it.Goal
		}
		return it.Goal + s.Net, it.Goal // climbing back
	case MRankRich:
		if s.RankRich {
			return 1, 1
		}
		return 0, 1
	case MRankNet:
		if s.RankNet {
			return 1, 1
		}
		return 0, 1
	case MFree:
		return 1, 1
	}
	return 0, 1
}

// Met reports whether the requirement is currently satisfied.
func (it *Item) Met(s Stats) bool {
	cur, goal := it.Progress(s)
	return cur >= goal
}

// Requirement describes how to unlock an item.
func (it *Item) Requirement() string {
	m := money
	switch it.Metric {
	case MWagered:
		return "Bet a total of " + m(it.Goal)
	case MPeak:
		return "Have " + m(it.Goal) + " at once"
	case MNetUp:
		return "Be up " + m(it.Goal) + " lifetime"
	case MNetDown:
		return "Be down " + m(it.Goal) + " lifetime"
	case MHands:
		return fmt.Sprintf("Play %s hands", num(it.Goal))
	case MBlackjacks:
		return fmt.Sprintf("Get %s blackjacks", num(it.Goal))
	case MBiggestWin:
		return "Win " + m(it.Goal) + " in one round"
	case MWinStreak:
		return fmt.Sprintf("Win %d hands in a row", it.Goal)
	case MLoseStreak:
		return fmt.Sprintf("Lose %d hands in a row", it.Goal)
	case MBusts:
		return fmt.Sprintf("Bust %s times", num(it.Goal))
	case MDoublesWon:
		return fmt.Sprintf("Win %s doubled hands", num(it.Goal))
	case MSplits:
		return fmt.Sprintf("Split %s times", num(it.Goal))
	case MRefills:
		return fmt.Sprintf("Receive %s daily refills", num(it.Goal))
	case MComeback:
		return "Fall " + m(it.Goal) + " behind, then get back to even"
	case MRankRich:
		return "Be #1 on the Richest leaderboard"
	case MRankNet:
		return "Be #1 on the Lifetime leaderboard"
	case MFree:
		return "Free for everyone"
	}
	return ""
}

func num(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func money(n int64) string { return "$" + num(n) }

// Evaluate returns the permanent items newly met by s that are not in owned.
func Evaluate(s Stats, owned map[string]bool) []*Item {
	var out []*Item
	for i := range Catalog {
		it := &Catalog[i]
		if it.Dynamic() || it.Metric == MFree || owned[it.ID] {
			continue
		}
		if it.Met(s) {
			out = append(out, it)
		}
	}
	return out
}

// Has reports whether a player may use an item right now.
func Has(id string, s Stats, owned map[string]bool) bool {
	it, ok := Get(id)
	if !ok {
		return false
	}
	if it.Metric == MFree {
		return true
	}
	if it.Dynamic() {
		return it.Met(s)
	}
	return owned[id]
}
