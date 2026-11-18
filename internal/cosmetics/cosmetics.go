// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

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
	MRefills    = "refills"    // daily refills received
	MComeback   = "comeback"   // was down Goal or more, then got back to even
	MRankRich   = "rank_rich"  // currently #1 richest (unique)
	MRankNet    = "rank_net"   // currently #1 lifetime (unique)
	MRankHands  = "rank_hands" // currently played the most hands, all games (unique)
	MRankBet    = "rank_bet"   // holds the record for the biggest single bet (unique)
	MRankPeak   = "rank_peak"  // holds the record for the most chips ever held at once (unique)
	// Texas Hold'em
	MPkHands     = "pk_hands"      // Hold'em hands played
	MPkFlush     = "pk_flush"      // flushes made (seen to the river or a showdown)
	MPkFullHouse = "pk_full_house" // full houses made
	MPkQuads     = "pk_quads"      // four of a kinds made
	MPkStrFlush  = "pk_str_flush"  // straight flushes made (royals count)
	MPkRoyal     = "pk_royal"      // royal flushes made
	MPkBluffs    = "pk_bluffs"     // pots won without a showdown holding nothing
	MPkBeatDale  = "pk_beat_dale"  // showdowns won against Dale
	MPkBigPot    = "pk_big_pot"    // biggest pot won
	// Baccarat
	MBacRounds  = "bac_rounds"  // baccarat rounds bet on
	MBacNatural = "bac_natural" // bets won on a natural (an 8 or 9 in two cards)
	MBacTie     = "bac_tie"     // tie bets won
	// Red Dog
	MRdWins    = "rd_wins"    // Red Dog bets won
	MRdGutshot = "rd_gutshot" // bets won on a one-card spread (5 to 1)
	MRdTrips   = "rd_trips"   // three of a kind hit (11 to 1)
	MFree      = "free"       // everyone has it
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
	RankRich, RankNet, RankHands, RankBet, RankPeak                                       bool
	PkHands, PkFlush, PkFullHouse, PkQuads, PkStrFlush, PkRoyal, PkBluffs, PkBeatDale     int
	PkBigPot                                                                              int64
	BacRounds, BacNaturals, BacTies, RdWins, RdGutshots, RdTrips                          int
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
	{ID: "moneybag", Kind: Hat, Name: "The Moneybags", Desc: "Worn by whoever has held the most chips at once, ever. Heavy is the head.", Tier: Unique, Metric: MRankPeak, Sprite: 34},
	{ID: "chip_tower", Kind: Hat, Name: "The Chip Tower", Desc: "Worn by whoever has made the biggest single bet. Don't sneeze.", Tier: Unique, Metric: MRankBet, Sprite: 25},
	{ID: "nightcap", Kind: Hat, Name: "The Nightcap", Desc: "Worn by whoever has played the most hands. Do you ever sleep?", Tier: Unique, Metric: MRankHands, Sprite: 24},

	// Texas Hold'em
	{ID: "visor", Kind: Hat, Name: "Green Visor", Desc: "Twenty-five hands of Hold'em. Card room regular.", Tier: Common, Metric: MPkHands, Goal: 25, Sprite: 26},
	{ID: "sailor", Kind: Hat, Name: "Sailor Cap", Desc: "Made a flush. Smooth sailing.", Tier: Uncommon, Metric: MPkFlush, Goal: 1, Sprite: 27},
	{ID: "fedora", Kind: Hat, Name: "Fedora", Desc: "Made a full house. Tip of the hat.", Tier: Rare, Metric: MPkFullHouse, Goal: 1, Sprite: 28},
	{ID: "jester", Kind: Hat, Name: "Jester Hat", Desc: "Won ten pots holding absolutely nothing.", Tier: Rare, Metric: MPkBluffs, Goal: 10, Sprite: 29},
	{ID: "bowler", Kind: Hat, Name: "Bowler Hat", Desc: "Made four of a kind. Frightfully dapper.", Tier: Epic, Metric: MPkQuads, Goal: 1, Sprite: 30},
	{ID: "shark_fin", Kind: Hat, Name: "Shark Fin", Desc: "Beat Dale at the showdown 25 times. Something smells like chum.", Tier: Epic, Metric: MPkBeatDale, Goal: 25, Sprite: 31},
	{ID: "pharaoh", Kind: Hat, Name: "Pharaoh's Headdress", Desc: "Made a straight flush. Ancient, terrible magic.", Tier: Legendary, Metric: MPkStrFlush, Goal: 1, Sprite: 32},
	{ID: "turban", Kind: Hat, Name: "Sultan's Turban", Desc: "Made a royal flush. All bow.", Tier: Legendary, Metric: MPkRoyal, Goal: 1, Sprite: 33},
	{ID: "beret", Kind: Hat, Name: "Beret", Desc: "Twenty-five rounds of baccarat. Tres continental.", Tier: Uncommon, Metric: MBacRounds, Goal: 25, Sprite: 35},
	{ID: "fascinator", Kind: Hat, Name: "Feathered Fascinator", Desc: "Twenty-five bets won on a natural. Darling.", Tier: Epic, Metric: MBacNatural, Goal: 25, Sprite: 36},
	{ID: "dog_ears", Kind: Hat, Name: "Floppy Dog Ears", Desc: "Twenty-five Red Dog bets won. Good boy.", Tier: Uncommon, Metric: MRdWins, Goal: 25, Sprite: 37},
	{ID: "hot_dog", Kind: Hat, Name: "Hot Dog", Desc: "Hit three of a kind at Red Dog. Relish it.", Tier: Epic, Metric: MRdTrips, Goal: 1, Sprite: 38},

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
	{ID: "frame_marathon", Kind: Frame, Name: "Marathon Frame", Desc: "Played more hands than anyone. The finish line is a myth.", Tier: Unique, Metric: MRankHands, Sprite: 10},
	{ID: "frame_velvet", Kind: Frame, Name: "Velvet Rope Frame", Desc: "Won five tie bets. The VIP section is this way.", Tier: Epic, Metric: MBacTie, Goal: 5, Sprite: 12},
	{ID: "frame_vault", Kind: Frame, Name: "Vault Frame", Desc: "Held the most chips anyone ever has. Lock it up.", Tier: Unique, Metric: MRankPeak, Sprite: 11},

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
	{ID: "t_grinder", Kind: Title, Name: "Grinder", Desc: "Play 250 hands of Hold'em.", Tier: Uncommon, Metric: MPkHands, Goal: 250},
	{ID: "t_pot_committed", Kind: Title, Name: "Pot Committed", Desc: "Win a pot of $5,000 or more.", Tier: Rare, Metric: MPkBigPot, Goal: 5_000},
	{ID: "t_bluffer", Kind: Title, Name: "Stone Cold Bluffer", Desc: "Win 25 pots holding nothing.", Tier: Rare, Metric: MPkBluffs, Goal: 25},
	{ID: "t_quad_god", Kind: Title, Name: "Quad God", Desc: "Make four of a kind three times.", Tier: Epic, Metric: MPkQuads, Goal: 3},
	{ID: "t_dale_slayer", Kind: Title, Name: "Dale Slayer", Desc: "Beat Dale at the showdown 100 times.", Tier: Epic, Metric: MPkBeatDale, Goal: 100},
	{ID: "t_royalty", Kind: Title, Name: "Royalty", Desc: "Make a royal flush.", Tier: Legendary, Metric: MPkRoyal, Goal: 1},
	{ID: "t_high_society", Kind: Title, Name: "High Society", Desc: "Bet on 100 rounds of baccarat.", Tier: Uncommon, Metric: MBacRounds, Goal: 100},
	{ID: "t_natural", Kind: Title, Name: "A Natural", Desc: "Win 10 baccarat bets on a natural.", Tier: Rare, Metric: MBacNatural, Goal: 10},
	{ID: "t_tie_breaker", Kind: Title, Name: "Tie Breaker", Desc: "Win a tie bet at baccarat.", Tier: Rare, Metric: MBacTie, Goal: 1},
	{ID: "t_top_dog", Kind: Title, Name: "Top Dog", Desc: "Win 100 Red Dog bets.", Tier: Rare, Metric: MRdWins, Goal: 100},
	{ID: "t_gutshot", Kind: Title, Name: "Gutshot", Desc: "Win 5 Red Dog bets on a one-card spread.", Tier: Epic, Metric: MRdGutshot, Goal: 5},
	{ID: "t_lucky_dog", Kind: Title, Name: "Lucky Dog", Desc: "Hit three of a kind at Red Dog.", Tier: Epic, Metric: MRdTrips, Goal: 1},
	{ID: "t_owns_house", Kind: Title, Name: "Owns The House", Desc: "Have the most chips right now.", Tier: Unique, Metric: MRankRich},
	{ID: "t_goat", Kind: Title, Name: "The G.O.A.T.", Desc: "Be #1 on the lifetime leaderboard.", Tier: Unique, Metric: MRankNet},
	{ID: "t_high_water", Kind: Title, Name: "High Water Mark", Desc: "Hold more chips at once than anyone ever has.", Tier: Unique, Metric: MRankPeak},
	{ID: "t_bet_the_farm", Kind: Title, Name: "Bet The Farm", Desc: "Make the biggest bet anyone has made.", Tier: Unique, Metric: MRankBet},
	{ID: "t_furniture", Kind: Title, Name: "Part Of The Furniture", Desc: "Play more hands than anyone.", Tier: Unique, Metric: MRankHands},
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
func (it *Item) Dynamic() bool {
	return it.Metric == MRankRich || it.Metric == MRankNet || it.Metric == MRankHands || it.Metric == MRankBet || it.Metric == MRankPeak
}

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
	case MRankHands:
		if s.RankHands {
			return 1, 1
		}
		return 0, 1
	case MRankBet:
		if s.RankBet {
			return 1, 1
		}
		return 0, 1
	case MRankPeak:
		if s.RankPeak {
			return 1, 1
		}
		return 0, 1
	case MPkHands:
		return int64(s.PkHands), it.Goal
	case MPkFlush:
		return int64(s.PkFlush), it.Goal
	case MPkFullHouse:
		return int64(s.PkFullHouse), it.Goal
	case MPkQuads:
		return int64(s.PkQuads), it.Goal
	case MPkStrFlush:
		return int64(s.PkStrFlush), it.Goal
	case MPkRoyal:
		return int64(s.PkRoyal), it.Goal
	case MPkBluffs:
		return int64(s.PkBluffs), it.Goal
	case MPkBeatDale:
		return int64(s.PkBeatDale), it.Goal
	case MPkBigPot:
		return s.PkBigPot, it.Goal
	case MBacRounds:
		return int64(s.BacRounds), it.Goal
	case MBacNatural:
		return int64(s.BacNaturals), it.Goal
	case MBacTie:
		return int64(s.BacTies), it.Goal
	case MRdWins:
		return int64(s.RdWins), it.Goal
	case MRdGutshot:
		return int64(s.RdGutshots), it.Goal
	case MRdTrips:
		return int64(s.RdTrips), it.Goal
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
	case MRankHands:
		return "Play more hands than anyone (all games)"
	case MRankBet:
		return "Make the biggest single bet (all games)"
	case MRankPeak:
		return "Hold the most chips at once, ever"
	case MPkHands:
		return fmt.Sprintf("Play %s hands of Hold'em", num(it.Goal))
	case MPkFlush:
		return times("Make a flush", it.Goal)
	case MPkFullHouse:
		return times("Make a full house", it.Goal)
	case MPkQuads:
		return times("Make four of a kind", it.Goal)
	case MPkStrFlush:
		return times("Make a straight flush", it.Goal)
	case MPkRoyal:
		return times("Make a royal flush", it.Goal)
	case MPkBluffs:
		return fmt.Sprintf("Win %s pots holding nothing (no showdown)", num(it.Goal))
	case MPkBeatDale:
		return fmt.Sprintf("Beat Dale at the showdown %s times", num(it.Goal))
	case MPkBigPot:
		return "Win a Hold'em pot of " + m(it.Goal)
	case MBacRounds:
		return fmt.Sprintf("Bet on %s rounds of baccarat", num(it.Goal))
	case MBacNatural:
		return times("Win a baccarat bet on a natural", it.Goal)
	case MBacTie:
		return times("Win a tie bet at baccarat", it.Goal)
	case MRdWins:
		return fmt.Sprintf("Win %s Red Dog bets", num(it.Goal))
	case MRdGutshot:
		return times("Win a Red Dog bet on a one-card spread", it.Goal)
	case MRdTrips:
		return times("Hit three of a kind at Red Dog", it.Goal)
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

// times is "Make a flush" or "Make a flush 3 times".
func times(what string, n int64) string {
	if n <= 1 {
		return what
	}
	return fmt.Sprintf("%s %s times", what, num(n))
}
