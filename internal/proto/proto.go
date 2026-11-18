// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package proto defines the client/server wire protocol.
//
// The transport is TCP. Every message is one line of JSON:
//
//	{"t":"<type>","d":{...}}
package proto

import (
	"encoding/json"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
)

// Version must match between client and server.
const Version = 1

// MaxLine is the largest accepted message line in bytes.
const MaxLine = 512 * 1024

// Msg is a raw protocol message.
type Msg struct {
	Type string          `json:"t"`
	Data json.RawMessage `json:"d,omitempty"`
}

// Encode builds a newline-terminated message.
func Encode(t string, v any) []byte {
	m := Msg{Type: t}
	if v != nil {
		b, err := json.Marshal(v)
		if err == nil {
			m.Data = b
		}
	}
	out, _ := json.Marshal(m)
	return append(out, '\n')
}

// Client -> server message types.
const (
	CHello       = "hello"
	CRegister    = "register"
	CLogin       = "login"
	CLogout      = "logout"
	CBet         = "bet"
	CAction      = "action"
	CSitOut      = "sit_out"
	CLeaderboard = "leaderboard"
	CAvatarGet   = "avatar_get"
	CAvatarSet   = "avatar_set"
	CPing        = "ping"
	CProfile     = "profile"
	CEquip       = "equip"
	CGiftInfo    = "gift_info" // how much can I send this player?
	CGift        = "gift"
	CChat        = "chat"
	CSettings    = "settings"    // admins: list the server settings
	CSetSetting  = "set_setting" // admins: change one
)

// Server -> client message types.
const (
	SServerInfo  = "server_info"
	SAuthOK      = "auth_ok"
	SAuthFail    = "auth_fail"
	SState       = "state"
	SEvent       = "event"
	SError       = "error"
	SLeaderboard = "leaderboard"
	SAvatar      = "avatar"
	SProfile     = "profile"
	SKicked      = "kicked"
	SPong        = "pong"
	SGiftInfo    = "gift_info"
	SChat        = "chat"
	SConsole     = "console" // command output, for admins
	SSettings    = "settings"
)

type Hello struct {
	Version int `json:"version"`
}

type ServerInfo struct {
	Name              string `json:"name"`
	Version           int    `json:"version"`
	AllowRegistration bool   `json:"allow_registration"`
	MOTD              string `json:"motd"`
	// NeedsPassword: creating an account needs the server password.
	NeedsPassword bool `json:"needs_password,omitempty"`
	// ServerVersion is the server's release version ("1.6.0", or "dev");
	// Updates says whether to offer older clients the update.
	ServerVersion string `json:"server_version,omitempty"`
	Updates       bool   `json:"updates,omitempty"`
}

type Register struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Avatar   []byte `json:"avatar,omitempty"` // PNG bytes (base64 in JSON)
	// ServerPassword is needed when the server has one set.
	ServerPassword string `json:"server_password,omitempty"`
}

type Login struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

type AuthOK struct {
	Username string  `json:"username"`
	Token    string  `json:"token"`
	Profile  Profile `json:"profile"`
}

type AuthFail struct {
	Reason string `json:"reason"`
}

// Profile is an account's public stats.
type Profile struct {
	Username      string `json:"username"`
	Balance       int64  `json:"balance"`
	Net           int64  `json:"net"`
	Won           int64  `json:"won"`
	Lost          int64  `json:"lost"`
	HandsPlayed   int    `json:"hands_played"`
	HandsWon      int    `json:"hands_won"`
	HandsLost     int    `json:"hands_lost"`
	HandsPushed   int    `json:"hands_pushed"`
	Blackjacks    int    `json:"blackjacks"`
	BiggestWin    int64  `json:"biggest_win"`
	RefillTotal   int64  `json:"refill_total"`
	AvatarVersion string `json:"avatar_version"`
	Online        bool   `json:"online"`

	Wagered         int64    `json:"wagered"`
	PeakBalance     int64    `json:"peak_balance"`
	LowestNet       int64    `json:"lowest_net"`
	BestWinStreak   int      `json:"best_win_streak"`
	WorstLoseStreak int      `json:"worst_lose_streak"`
	Busts           int      `json:"busts"`
	DoublesWon      int      `json:"doubles_won"`
	Splits          int      `json:"splits"`
	RefillCount     int      `json:"refill_count"`
	Unlocked        []string `json:"unlocked,omitempty"`
	Hat             string   `json:"hat,omitempty"`   // shown hat (crown/laurel override)
	Frame           string   `json:"frame,omitempty"` // shown frame
	Title           string   `json:"title,omitempty"` // shown title
	EquippedHat     string   `json:"equipped_hat,omitempty"`
	RankRich        bool     `json:"rank_rich,omitempty"` // currently #1 richest
	RankNet         bool     `json:"rank_net,omitempty"`  // currently #1 lifetime
	Admin           bool     `json:"admin,omitempty"`
	GiftsIn         int64    `json:"gifts_in,omitempty"`
	GiftsOut        int64    `json:"gifts_out,omitempty"`
}

// Equip asks to wear a cosmetic. Kind is hat, frame or title; an empty ID
// takes the hat off.
type Equip struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// LeaderboardReq picks the leaderboard order: "balance" or "net".
type LeaderboardReq struct {
	Sort string `json:"sort"`
}

type Bet struct {
	Amount int64 `json:"amount"`
}

type Action struct {
	Action string `json:"action"` // hit, stand, double, split, surrender
}

type SitOut struct {
	On bool `json:"on"`
}

type AvatarGet struct {
	Username string `json:"username"`
}

type Avatar struct {
	Username string `json:"username"`
	Version  string `json:"version"`
	Data     []byte `json:"data"`
}

type AvatarSet struct {
	Data []byte `json:"data"`
}

type Leaderboard struct {
	Sort    string     `json:"sort"`
	Entries []Profile  `json:"entries"`
	House   HouseStats `json:"house"`
}

type Event struct {
	Kind  string `json:"kind"` // card, chip, win, lose, push, blackjack, bust, turn, refill, info, shuffle, unlock, crown
	Item  string `json:"item,omitempty"`
	Text  string `json:"text,omitempty"`
	Seat  int    `json:"seat"`
	Delta int64  `json:"delta,omitempty"`
}

// GiftInfo asks (To) or answers (all fields) how many chips can be sent.
type GiftInfo struct {
	To      string `json:"to"`
	Max     int64  `json:"max"`              // 0 = can't send now
	Reason  string `json:"reason,omitempty"` // why not / what limits it
	Balance int64  `json:"balance"`          // their chips
}

// Gift sends chips to another player.
type Gift struct {
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

// Chat is a table chat message. From the client only Text is used; a
// message starting with "/" from an admin is run as a server command.
type Chat struct {
	From  string `json:"from,omitempty"`
	Seat  int    `json:"seat"` // -1 = not seated
	Text  string `json:"text"`
	Admin bool   `json:"admin,omitempty"`
	At    int64  `json:"at,omitempty"` // unix ms
}

// SettingInfo is one server setting, for admins.
type SettingInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // bool, int, float, string, list
	Value string `json:"value"`
	Group string `json:"group,omitempty"`
}

// SettingsList lists the server settings (answering CSettings / CSetSetting).
type SettingsList struct {
	Settings []SettingInfo `json:"settings"`
	Changed  string        `json:"changed,omitempty"` // key that was just set
	Error    string        `json:"error,omitempty"`
}

// SetSetting changes one server setting.
type SetSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Console is the output of a server command run from the client.
type Console struct {
	Lines []string `json:"lines"`
}

type ErrorMsg struct {
	Message string `json:"message"`
}

// Table phases.
const (
	PhaseWaiting = "waiting"
	PhaseBetting = "betting"
	PhaseDealing = "dealing"
	PhasePlaying = "playing"
	PhaseDealer  = "dealer"
	PhaseResults = "results"
)

// Settings are the table settings shown to players.
type Settings struct {
	MaxSeats         int    `json:"max_seats"`
	MinBet           int64  `json:"min_bet"`
	MaxBet           int64  `json:"max_bet"`
	BettingMode      string `json:"betting_mode"` // simultaneous | turns
	PlayMode         string `json:"play_mode"`    // simultaneous | turns
	HideOtherHands   bool   `json:"hide_other_hands"`
	Decks            int    `json:"decks"`
	DealerHitsSoft17 bool   `json:"dealer_hits_soft_17"`
	BlackjackPays    string `json:"blackjack_pays"`
	Surrender        bool   `json:"surrender"`
	RefillAmount     int64  `json:"refill_amount"`
	ServerName       string `json:"server_name"`
	DealerTrashTalk  bool   `json:"dealer_trash_talk"`
	DealerVulgar     bool   `json:"dealer_vulgar"`
	ChipGifts        bool   `json:"chip_gifts,omitempty"`
	TableChat        bool   `json:"table_chat,omitempty"`
	CardReveal       bool   `json:"card_reveal,omitempty"`
}

// HouseStats are the dealer's lifetime results.
type HouseStats struct {
	HandsDealt       int   `json:"hands_dealt"`  // rounds dealt
	PlayerHands      int   `json:"player_hands"` // player hands settled
	Net              int64 `json:"net"`          // house gain (+) or loss (-)
	Won              int64 `json:"won"`          // total from rounds the house won
	Lost             int64 `json:"lost"`         // total from rounds the house lost
	RoundsWon        int   `json:"rounds_won"`
	RoundsLost       int   `json:"rounds_lost"`
	DealerBlackjacks int   `json:"dealer_blackjacks"`
	DealerBusts      int   `json:"dealer_busts"`
	PlayerBusts      int   `json:"player_busts"`
	BiggestWin       int64 `json:"biggest_win"`  // best single round for the house
	BiggestLoss      int64 `json:"biggest_loss"` // worst single round for the house
	Tier             int   `json:"tier"`         // dealer wealth tier, -3 (destitute) .. 5 (emperor)
}

// HandView is a hand as seen by one client.
type HandView struct {
	Cards   []blackjack.Card `json:"cards"`
	Bet     int64            `json:"bet"`
	Total   int              `json:"total"`
	Soft    bool             `json:"soft"`
	Done    bool             `json:"done"`
	Doubled bool             `json:"doubled"`
	Result  string           `json:"result,omitempty"`
	Payout  int64            `json:"payout,omitempty"`
	Natural bool             `json:"natural,omitempty"`
}

// SeatView is a seat as seen by one client.
type SeatView struct {
	Seat          int        `json:"seat"`
	Username      string     `json:"username"`
	AvatarVersion string     `json:"avatar_version"`
	Balance       int64      `json:"balance"`
	Status        string     `json:"status"` // waiting, betting, bet_placed, playing, sitting_out, broke, away
	PendingBet    int64      `json:"pending_bet"`
	Hands         []HandView `json:"hands"`
	ActiveHand    int        `json:"active_hand"` // index of the hand being played, -1 if none
	Connected     bool       `json:"connected"`
	Hat           string     `json:"hat,omitempty"`
	Frame         string     `json:"frame,omitempty"`
	Title         string     `json:"title,omitempty"`
}

// DealerView is the dealer as seen by clients.
type DealerView struct {
	Cards []blackjack.Card `json:"cards"`
	Total int              `json:"total"`
	Soft  bool             `json:"soft"`
}

// TableState is a full personalised table snapshot.
// RevealView is a card being revealed slowly (the "card reveal" setting):
// it's face down in the hand until the reveal ends.
type RevealView struct {
	Seat  int   `json:"seat"` // -1 = the dealer
	Hand  int   `json:"hand"`
	Card  int   `json:"card"`
	Until int64 `json:"until"` // unix ms, server clock
}

type TableState struct {
	Phase      string     `json:"phase"`
	HandNumber int        `json:"hand_number"`
	Deadline   int64      `json:"deadline"`    // unix ms when the current timer expires, 0 = none
	TimerTotal int64      `json:"timer_total"` // timer length in ms
	ServerTime int64      `json:"server_time"` // unix ms, to correct clock skew
	Dealer     DealerView `json:"dealer"`
	Seats      []SeatView `json:"seats"`
	ActiveSeat int        `json:"active_seat"` // seat whose turn it is (turn modes), -1 otherwise
	YourSeat   int        `json:"your_seat"`   // -1 when spectating
	Actions    []string   `json:"actions"`     // actions you may take right now
	CanBet     bool       `json:"can_bet"`
	Message    string     `json:"message"`
	Settings   Settings   `json:"settings"`
	Spectators int        `json:"spectators"`
	ShoeLeft   int        `json:"shoe_left"`

	DealerSay      string `json:"dealer_say,omitempty"`
	DealerSayID    int    `json:"dealer_say_id,omitempty"`
	DealerSayUntil int64  `json:"dealer_say_until,omitempty"` // unix ms
	// DealerSayCont: this bubble continues the previous one (a long line or a
	// story told a page at a time); DealerSayPara: and it starts a paragraph.
	DealerSayCont bool `json:"dealer_say_cont,omitempty"`
	DealerSayPara bool `json:"dealer_say_para,omitempty"`
	// Reveal is set while a card is being revealed.
	Reveal *RevealView `json:"reveal,omitempty"`
	// DealerLog is what the dealer said recently, sent once after you join.
	DealerLog []DealerLine `json:"dealer_log,omitempty"`
	HouseNet  int64        `json:"house_net"`
	HouseTier int          `json:"house_tier"`
}

// DealerLine is one bubble the dealer said, for the "what he said" log.
type DealerLine struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Cont bool   `json:"cont,omitempty"` // continues the previous line
	Para bool   `json:"para,omitempty"` // ...starting a new paragraph
	At   int64  `json:"at"`             // unix ms
}
