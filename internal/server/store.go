// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/imgutil"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/poker"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// AvatarSize is the stored avatar resolution (square, pixels).
const AvatarSize = 32

const pbkdfIter = 120000

// Account is a persisted player account.
type Account struct {
	Username string   `json:"username"`
	Salt     string   `json:"salt"`
	PassHash string   `json:"pass_hash"`
	Tokens   []string `json:"tokens,omitempty"` // sha256 of remember-me tokens

	Balance     int64 `json:"balance"`
	Net         int64 `json:"net"`  // lifetime gain/loss (refills subtracted)
	Won         int64 `json:"won"`  // lifetime winnings from hands
	Lost        int64 `json:"lost"` // lifetime losses from hands
	HandsPlayed int   `json:"hands_played"`
	HandsWon    int   `json:"hands_won"`
	HandsLost   int   `json:"hands_lost"`
	HandsPushed int   `json:"hands_pushed"`
	Blackjacks  int   `json:"blackjacks"`
	BiggestWin  int64 `json:"biggest_win"`
	RefillTotal int64 `json:"refill_total"`
	// Admins can run server commands from the game (/command in chat).
	Admin bool `json:"admin,omitempty"`
	// Chips sent to and received from other players. Received chips count
	// like a refill (subtracted from Net), sent ones are added back, so
	// lifetime gain stays what you actually won from the house.
	GiftsIn      int64  `json:"gifts_in,omitempty"`
	GiftsOut     int64  `json:"gifts_out,omitempty"`
	GiftPeriod   string `json:"gift_period,omitempty"`
	GiftPeriodIn int64  `json:"gift_period_in,omitempty"` // received this refill period
	// Chips added or removed by an admin console command (not part of Net).
	AdminAdjust int64 `json:"admin_adjust"`

	// Stats used for cosmetic unlocks.
	Wagered         int64 `json:"wagered"`
	PeakBalance     int64 `json:"peak_balance"`
	LowestNet       int64 `json:"lowest_net"`
	WinStreak       int   `json:"win_streak"`
	LoseStreak      int   `json:"lose_streak"`
	BestWinStreak   int   `json:"best_win_streak"`
	WorstLoseStreak int   `json:"worst_lose_streak"`
	Busts           int   `json:"busts"`
	DoublesWon      int   `json:"doubles_won"`
	Splits          int   `json:"splits"`
	RefillCount     int   `json:"refill_count"`
	PokerHands      int   `json:"poker_hands,omitempty"`
	// Chips sitting at a poker table (bought in, not yet cashed out). They're
	// saved so a crash can't lose them: at startup they go back to Balance.
	TableStack int64  `json:"table_stack,omitempty"`
	TableID    string `json:"table_id,omitempty"`
	// Poker stats, for unlocks.
	PokerWins       int   `json:"poker_wins,omitempty"`
	PokerFlushes    int   `json:"poker_flushes,omitempty"`
	PokerFullHouses int   `json:"poker_full_houses,omitempty"`
	PokerQuads      int   `json:"poker_quads,omitempty"`
	PokerStrFlushes int   `json:"poker_straight_flushes,omitempty"`
	PokerRoyals     int   `json:"poker_royals,omitempty"`
	PokerBluffs     int   `json:"poker_bluffs,omitempty"`
	PokerBeatDale   int   `json:"poker_beat_dale,omitempty"`
	PokerBiggestPot int64 `json:"poker_biggest_pot,omitempty"`
	BiggestBet      int64 `json:"biggest_bet,omitempty"` // most riding on one hand, any game

	Unlocked []string `json:"unlocked,omitempty"`
	Hat      string   `json:"hat,omitempty"`
	Frame    string   `json:"frame,omitempty"`
	Title    string   `json:"title,omitempty"`

	LastRefill    string    `json:"last_refill"`
	AvatarVersion string    `json:"avatar_version"`
	Created       time.Time `json:"created"`
	LastSeen      time.Time `json:"last_seen"`
}

func (a *Account) owned() map[string]bool {
	m := make(map[string]bool, len(a.Unlocked))
	for _, id := range a.Unlocked {
		m[id] = true
	}
	return m
}

// stats builds unlock stats; caller holds s.mu.
func (s *Store) stats(a *Account) cosmetics.Stats {
	key := strings.ToLower(a.Username)
	return cosmetics.Stats{
		Wagered: a.Wagered, Peak: a.PeakBalance, Net: a.Net, LowestNet: a.LowestNet, BiggestWin: a.BiggestWin,
		Hands: a.HandsPlayed, Blackjacks: a.Blackjacks, BestWinStreak: a.BestWinStreak, WorstLoseStreak: a.WorstLoseStreak,
		Busts: a.Busts, DoublesWon: a.DoublesWon, Splits: a.Splits, Refills: a.RefillCount,
		RankRich: key != "" && key == s.topRich, RankNet: key != "" && key == s.topNet,
		RankHands: key != "" && key == s.topHands, RankBet: key != "" && key == s.topBet,
		PkHands: a.PokerHands, PkFlush: a.PokerFlushes, PkFullHouse: a.PokerFullHouses, PkQuads: a.PokerQuads,
		PkStrFlush: a.PokerStrFlushes, PkRoyal: a.PokerRoyals, PkBluffs: a.PokerBluffs, PkBeatDale: a.PokerBeatDale,
		PkBigPot: a.PokerBiggestPot,
	}
}

// look returns the cosmetics a player is showing; caller holds s.mu.
func (s *Store) look(a *Account) (hat, frame, title string) {
	st := s.stats(a)
	own := a.owned()
	switch {
	case st.RankRich:
		hat = "crown"
	case st.RankNet:
		hat = "laurel"
	case st.RankHands:
		hat = "nightcap"
	case st.RankBet:
		hat = "chip_tower"
	case a.Hat != "" && cosmetics.Has(a.Hat, st, own):
		hat = a.Hat
	}
	frame = "frame_wood"
	if a.Frame != "" && cosmetics.Has(a.Frame, st, own) {
		frame = a.Frame
	}
	title = "t_rookie"
	if a.Title != "" && cosmetics.Has(a.Title, st, own) {
		title = a.Title
	}
	return
}

// profile builds the public profile; caller holds s.mu.
func (s *Store) profileOf(a *Account) proto.Profile {
	hat, frame, title := s.look(a)
	st := s.stats(a)
	return proto.Profile{
		Username: a.Username, Balance: a.Balance + a.TableStack, AtTable: a.TableStack, Net: a.Net, Won: a.Won, Lost: a.Lost,
		HandsPlayed: a.HandsPlayed, HandsWon: a.HandsWon, HandsLost: a.HandsLost,
		HandsPushed: a.HandsPushed, Blackjacks: a.Blackjacks, BiggestWin: a.BiggestWin,
		RefillTotal: a.RefillTotal, AvatarVersion: a.AvatarVersion,
		Wagered: a.Wagered, PeakBalance: a.PeakBalance, LowestNet: a.LowestNet,
		BestWinStreak: a.BestWinStreak, WorstLoseStreak: a.WorstLoseStreak, Busts: a.Busts,
		DoublesWon: a.DoublesWon, Splits: a.Splits, RefillCount: a.RefillCount,
		Unlocked: append([]string(nil), a.Unlocked...), Hat: hat, Frame: frame, Title: title,
		EquippedHat: a.Hat, RankRich: st.RankRich, RankNet: st.RankNet, RankHands: st.RankHands, RankBet: st.RankBet, BiggestBet: a.BiggestBet,
		PokerHands: a.PokerHands, PokerFlushes: a.PokerFlushes, PokerFullHouses: a.PokerFullHouses,
		PokerQuads: a.PokerQuads, PokerStrFlushes: a.PokerStrFlushes, PokerRoyals: a.PokerRoyals,
		PokerBluffs: a.PokerBluffs, PokerBeatDale: a.PokerBeatDale, PokerBiggestPot: a.PokerBiggestPot,
		Admin: a.Admin, GiftsIn: a.GiftsIn, GiftsOut: a.GiftsOut,
		Online: s.online[strings.ToLower(a.Username)],
	}
}

// Unlock is a newly earned cosmetic.
type Unlock struct {
	User string
	Item *cosmetics.Item
}

// touched updates derived stats, the #1 spots and unlocks after a change to
// an account; caller holds s.mu.
func (s *Store) touched(a *Account) {
	if a.Balance+a.TableStack > a.PeakBalance {
		a.PeakBalance = a.Balance + a.TableStack
	}
	if a.Net < a.LowestNet {
		a.LowestNet = a.Net
	}
	s.recomputeRanks()
	for _, it := range cosmetics.Evaluate(s.stats(a), a.owned()) {
		a.Unlocked = append(a.Unlocked, it.ID)
		s.unlocks = append(s.unlocks, Unlock{User: a.Username, Item: it})
	}
	s.dirty = true
}

// recomputeRanks finds the unique #1 richest (above the starting amount) and
// #1 lifetime (above zero); caller holds s.mu.
func (s *Store) recomputeRanks() {
	var rich, net, hands string
	var bestBal, bestNet int64
	bestHands := 0
	richTie, netTie, handsTie := false, false, false
	for k, a := range s.accounts {
		switch n := a.HandsPlayed + a.PokerHands; {
		case hands == "" || n > bestHands:
			hands, bestHands, handsTie = k, n, false
		case n == bestHands:
			handsTie = true
		}
		switch {
		case rich == "" || a.Balance+a.TableStack > bestBal:
			rich, bestBal, richTie = k, a.Balance+a.TableStack, false
		case a.Balance+a.TableStack == bestBal:
			richTie = true
		}
		switch {
		case net == "" || a.Net > bestNet:
			net, bestNet, netTie = k, a.Net, false
		case a.Net == bestNet:
			netTie = true
		}
	}
	if richTie || bestBal <= s.CrownMin {
		rich = ""
	}
	if netTie || bestNet <= 0 {
		net = ""
	}
	if handsTie || bestHands == 0 {
		hands = ""
	}
	// the biggest bet is a record: whoever holds it keeps it until someone
	// beats it outright (a tie with someone else at load time: nobody)
	var bet string
	var bestBet int64
	betTie := false
	for k, a := range s.accounts {
		switch {
		case a.BiggestBet > bestBet:
			bet, bestBet, betTie = k, a.BiggestBet, false
		case a.BiggestBet == bestBet && bestBet > 0:
			betTie = true
		}
	}
	if h, ok := s.accounts[s.topBet]; ok && h.BiggestBet == bestBet && bestBet > 0 {
		bet, betTie = s.topBet, false
	}
	if betTie || bestBet == 0 {
		bet = ""
	}
	s.topBet = bet
	s.topRich, s.topNet, s.topHands = rich, net, hands
}

// TakeUnlocks returns and clears unlocks earned since the last call.
func (s *Store) TakeUnlocks() []Unlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.unlocks
	s.unlocks = nil
	return u
}

// Tops returns the current #1 richest and #1 lifetime usernames ("" if none).
// TopBet is who holds the biggest-bet record, and how big it is.
func (s *Store) TopBet() (string, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[s.topBet]; ok {
		return a.Username, a.BiggestBet
	}
	return "", 0
}

// Records lists who holds the record-based cosmetics and their numbers.
func (s *Store) Records() proto.Records {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out proto.Records
	h := proto.Record{Metric: cosmetics.MRankHands}
	if a, ok := s.accounts[s.topHands]; ok {
		h.User, h.Value = a.Username, int64(a.HandsPlayed+a.PokerHands)
	}
	b := proto.Record{Metric: cosmetics.MRankBet}
	if a, ok := s.accounts[s.topBet]; ok {
		b.User, b.Value = a.Username, a.BiggestBet
	}
	out.Records = append(out.Records, h, b)
	return out
}

// TopHands is who has played the most hands ("" if nobody or a tie).
func (s *Store) TopHands() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[s.topHands]; ok {
		return a.Username
	}
	return ""
}

func (s *Store) Tops() (rich, net string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[s.topRich]; ok {
		rich = a.Username
	}
	if a, ok := s.accounts[s.topNet]; ok {
		net = a.Username
	}
	return
}

// Equip sets a player's hat, frame or title. An empty id removes the hat.
func (s *Store) Equip(username, kind, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return errors.New("no such account")
	}
	if id != "" {
		it, ok := cosmetics.Get(id)
		if !ok || it.Kind != kind {
			return errors.New("unknown item")
		}
		if !cosmetics.Has(id, s.stats(a), a.owned()) {
			return errors.New("you haven't unlocked " + it.Name + " yet")
		}
	}
	switch kind {
	case cosmetics.Hat:
		a.Hat = id
	case cosmetics.Frame:
		a.Frame = id
	case cosmetics.Title:
		a.Title = id
	default:
		return errors.New("unknown item type")
	}
	s.dirty = true
	return nil
}

// Store holds all accounts. It is safe for concurrent use.
type Store struct {
	mu       sync.Mutex
	dir      string
	accounts map[string]*Account // key: lower-case username
	dirty    bool
	online   map[string]bool

	// CrownMin: the crown needs strictly more chips than this (the refill amount).
	CrownMin int64
	house    proto.HouseStats
	topRich  string
	topHands string
	topBet   string
	topNet   string
	unlocks  []Unlock
	changes  int  // successful saves, to know when a backup is due
	backedUp int  // changes at the last backup
	frozen   bool // a backup was restored: stop saving until restart
	logf     func(string, ...any)
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

// OpenStore loads accounts from dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "avatars"), 0o755); err != nil {
		return nil, err
	}
	return OpenStoreLog(dir, func(string, ...any) {})
}

// OpenStoreLog is OpenStore with a logger for recovery messages. A damaged
// accounts.json or house.json is replaced by the previous save's copy or the
// newest good backup.
func OpenStoreLog(dir string, logf func(string, ...any)) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "avatars"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, accounts: map[string]*Account{}, online: map[string]bool{}, logf: logf}
	var list []*Account
	if _, err := loadWithFallback(dir, "accounts.json", func(b []byte) error {
		list = nil
		return json.Unmarshal(b, &list)
	}, logf); err != nil {
		return nil, err
	}
	for _, a := range list {
		if a != nil && a.Username != "" {
			s.accounts[strings.ToLower(a.Username)] = a
		}
	}
	for _, a := range s.accounts {
		s.touched(a) // backfill derived stats and unlocks for older saves
	}
	s.unlocks = nil
	var house proto.HouseStats
	if found, err := loadWithFallback(dir, "house.json", func(b []byte) error {
		house = proto.HouseStats{}
		return json.Unmarshal(b, &house)
	}, logf); err != nil {
		logf("WARNING: %v; starting the house's stats fresh", err)
	} else if found {
		s.house = house
	}
	s.house.Tier = banter.Tier(s.house.Net, s.house.Tier)
	return s, nil
}

// RecordRound adds one settled round to the house's lifetime stats.
// houseDelta is what the house gained (negative when it paid out).
// It returns the updated stats and the dealer's previous wealth tier.
func (s *Store) RecordRound(houseDelta int64, playerHands int, dealerBJ, dealerBust bool, playerBusts int) (proto.HouseStats, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := &s.house
	prevTier := h.Tier
	h.HandsDealt++
	h.PlayerHands += playerHands
	h.Net += houseDelta
	switch {
	case houseDelta > 0:
		h.Won += houseDelta
		h.RoundsWon++
	case houseDelta < 0:
		h.Lost += -houseDelta
		h.RoundsLost++
	}
	if houseDelta > h.BiggestWin {
		h.BiggestWin = houseDelta
	}
	if -houseDelta > h.BiggestLoss {
		h.BiggestLoss = -houseDelta
	}
	if dealerBJ {
		h.DealerBlackjacks++
	}
	if dealerBust {
		h.DealerBusts++
	}
	h.PlayerBusts += playerBusts
	h.Tier = banter.Tier(h.Net, h.Tier)
	s.dirty = true
	return *h, prevTier
}

// House returns the house's lifetime stats.
func (s *Store) House() proto.HouseStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.house
}

// LoseStreak returns a player's current run of losing hands.
func (s *Store) LoseStreak(username string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok {
		return a.LoseStreak
	}
	return 0
}

// Worst returns the username in last place on the lifetime board, if there
// are at least three accounts and they're losing.
func (s *Store) Worst() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.accounts) < 3 {
		return ""
	}
	var worst *Account
	for _, a := range s.accounts {
		if worst == nil || a.Net < worst.Net {
			worst = a
		}
	}
	if worst == nil || worst.Net >= 0 {
		return ""
	}
	return worst.Username
}

// Save writes accounts to disk if anything changed.
func (s *Store) Save() error {
	s.mu.Lock()
	if !s.dirty || s.frozen {
		s.mu.Unlock()
		return nil
	}
	list := make([]*Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		cp := *a
		list = append(list, &cp)
	}
	s.dirty = false
	house := s.house
	s.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username) })
	b, err := json.MarshalIndent(list, "", "  ")
	if err == nil {
		err = writeFileSafe(filepath.Join(s.dir, "accounts.json"), b, true)
	}
	if err == nil {
		var hb []byte
		if hb, err = json.MarshalIndent(house, "", "  "); err == nil {
			err = writeFileSafe(filepath.Join(s.dir, "house.json"), hb, true)
		}
	}
	s.mu.Lock()
	if err != nil {
		s.dirty = true // try again next time instead of forgetting the changes
	} else {
		s.changes++
	}
	s.mu.Unlock()
	return err
}

func hashPassword(pw string, salt []byte) string {
	k, _ := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, 32)
	return base64.StdEncoding.EncodeToString(k)
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Register creates an account. avatar may be nil.
func (s *Store) Register(username, password string, avatar []byte, startBalance int64, period string) (*proto.Profile, string, error) {
	if !usernameRe.MatchString(username) {
		return nil, "", errors.New("username must be 3-16 letters, numbers or _")
	}
	if len(password) < 4 || len(password) > 128 {
		return nil, "", errors.New("password must be 4-128 characters")
	}
	var avatarPNG []byte
	if len(avatar) > 0 {
		var err error
		if avatarPNG, err = NormalizeAvatar(avatar); err != nil {
			return nil, "", err
		}
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	hash := hashPassword(password, salt)
	key := strings.ToLower(username)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[key]; ok {
		return nil, "", errors.New("that username is taken")
	}
	tok := newToken()
	a := &Account{
		Username: username, Salt: base64.StdEncoding.EncodeToString(salt), PassHash: hash,
		Tokens: []string{hashToken(tok)}, Balance: startBalance, LastRefill: period,
		Created: time.Now(), LastSeen: time.Now(),
	}
	if avatarPNG != nil {
		if err := writeFileSafe(s.avatarPath(key), avatarPNG, false); err == nil {
			a.AvatarVersion = avatarVersion(avatarPNG)
		}
	}
	s.accounts[key] = a
	s.dirty = true
	s.touched(a)
	p := s.profileOf(a)
	return &p, tok, nil
}

// Login checks a password or remember-me token. It returns a fresh token.
func (s *Store) Login(username, password, token string) (*proto.Profile, string, error) {
	key := strings.ToLower(username)
	s.mu.Lock()
	a, ok := s.accounts[key]
	var salt []byte
	var want string
	if ok {
		salt, _ = base64.StdEncoding.DecodeString(a.Salt)
		want = a.PassHash
	}
	s.mu.Unlock()
	if !ok {
		time.Sleep(300 * time.Millisecond)
		return nil, "", errors.New("unknown username or wrong password")
	}
	if token != "" {
		th := hashToken(token)
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, t := range a.Tokens {
			if subtle.ConstantTimeCompare([]byte(t), []byte(th)) == 1 {
				// rotate token
				nt := newToken()
				a.Tokens[i] = hashToken(nt)
				a.LastSeen = time.Now()
				s.dirty = true
				p := s.profileOf(a)
				return &p, nt, nil
			}
		}
		return nil, "", errors.New("saved login expired, please log in again")
	}
	got := hashPassword(password, salt)
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		time.Sleep(300 * time.Millisecond)
		return nil, "", errors.New("unknown username or wrong password")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nt := newToken()
	a.Tokens = append(a.Tokens, hashToken(nt))
	if len(a.Tokens) > 5 {
		a.Tokens = a.Tokens[len(a.Tokens)-5:]
	}
	a.LastSeen = time.Now()
	s.dirty = true
	p := s.profileOf(a)
	return &p, nt, nil
}

// RevokeToken removes a remember-me token (logout).
func (s *Store) RevokeToken(username, token string) {
	th := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok {
		for i, t := range a.Tokens {
			if t == th {
				a.Tokens = append(a.Tokens[:i], a.Tokens[i+1:]...)
				s.dirty = true
				return
			}
		}
	}
}

// Canonical returns the stored spelling of a username.
func (s *Store) Canonical(username string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return "", false
	}
	return a.Username, true
}

// Profile returns an account's public profile.
func (s *Store) Profile(username string) (proto.Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return proto.Profile{}, false
	}
	return s.profileOf(a), true
}

// Balance returns an account's balance.
func (s *Store) Balance(username string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok {
		return a.Balance
	}
	return 0
}

// SetAdmin makes an account an admin or not.
func (s *Store) SetAdmin(username string, on bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return false
	}
	a.Admin = on
	s.dirty = true
	return true
}

// IsAdmin reports whether an account is an admin.
func (s *Store) IsAdmin(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	return ok && a.Admin
}

// Admins lists the admin accounts.
func (s *Store) Admins() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, a := range s.accounts {
		if a.Admin {
			out = append(out, a.Username)
		}
	}
	sort.Strings(out)
	return out
}

// GiftedIn reports how many chips an account has received this refill period.
func (s *Store) GiftedIn(username, period string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok && a.GiftPeriod == period {
		return a.GiftPeriodIn
	}
	return 0
}

// Gift moves chips from one account to another. The caller checks the
// limits; Gift only refuses if the sender can't cover it.
func (s *Store) Gift(from, to string, amount int64, period string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok1 := s.accounts[strings.ToLower(from)]
	t, ok2 := s.accounts[strings.ToLower(to)]
	switch {
	case !ok1 || !ok2:
		return errors.New("no such player")
	case f == t:
		return errors.New("you can't send chips to yourself")
	case amount <= 0:
		return errors.New("send at least 1 chip")
	case f.Balance < amount:
		return errors.New("you don't have that many chips")
	}
	f.Balance -= amount
	f.Net += amount
	f.GiftsOut += amount
	t.Balance += amount
	t.Net -= amount
	t.GiftsIn += amount
	if t.GiftPeriod != period {
		t.GiftPeriod, t.GiftPeriodIn = period, 0
	}
	t.GiftPeriodIn += amount
	s.touched(f)
	s.touched(t)
	return nil
}

// Debit removes chips for a wager. It fails if the balance is too low.
func (s *Store) Debit(username string, amount int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok || amount < 0 || a.Balance < amount {
		return false
	}
	a.Balance -= amount
	// ranks are only recomputed when hands settle, so the crown doesn't
	// wobble while chips are on the table
	s.dirty = true
	return true
}

// HandResult records a settled hand: wagered is the total stake on the hand,
// payout what is returned (stake + winnings).
type HandResult struct {
	Wagered   int64
	Payout    int64
	Result    string
	Blackjack bool
	Doubled   bool
	FromSplit bool
}

// Settle credits payouts and updates lifetime stats for one player's hands.
func (s *Store) Settle(username string, hands []HandResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return
	}
	var round int64
	for _, h := range hands {
		a.Balance += h.Payout
		d := h.Payout - h.Wagered
		round += d
		a.Net += d
		if d > 0 {
			a.Won += d
		} else {
			a.Lost += -d
		}
		a.HandsPlayed++
		a.Wagered += h.Wagered
		if h.Wagered > a.BiggestBet {
			a.BiggestBet = h.Wagered
		}
		switch {
		case d > 0:
			a.HandsWon++
			a.WinStreak++
			a.LoseStreak = 0
			if h.Doubled {
				a.DoublesWon++
			}
		case d < 0:
			a.HandsLost++
			a.LoseStreak++
			a.WinStreak = 0
		default:
			a.HandsPushed++
		}
		if a.WinStreak > a.BestWinStreak {
			a.BestWinStreak = a.WinStreak
		}
		if a.LoseStreak > a.WorstLoseStreak {
			a.WorstLoseStreak = a.LoseStreak
		}
		if h.Result == "bust" {
			a.Busts++
		}
		if h.FromSplit {
			a.Splits++ // one per split hand; corrected to splits below
		}
		if h.Blackjack {
			a.Blackjacks++
		}
	}
	// n split hands come from n-1 splits
	splitHands := 0
	for _, h := range hands {
		if h.FromSplit {
			splitHands++
		}
	}
	if splitHands > 0 {
		a.Splits--
	}
	if round > a.BiggestWin {
		a.BiggestWin = round
	}
	s.touched(a)
}

// ApplyRefill tops an account up to amount if this refill period has not been
// processed for it yet. It returns the chips added.
func (s *Store) ApplyRefill(username string, amount int64, period string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok || a.LastRefill == period {
		return 0
	}
	a.LastRefill = period
	s.dirty = true
	if a.Balance+a.TableStack >= amount {
		return 0
	}
	diff := amount - a.Balance - a.TableStack
	a.Balance += diff
	a.Net -= diff
	a.RefillTotal += diff
	a.RefillCount++
	s.touched(a)
	return diff
}

// ForceRefill tops up an account immediately regardless of period (admin).
func (s *Store) ForceRefill(username string, amount int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok || a.Balance+a.TableStack >= amount {
		return 0
	}
	diff := amount - a.Balance - a.TableStack
	a.Balance += diff
	a.Net -= diff
	a.RefillTotal += diff
	a.RefillCount++
	s.touched(a)
	return diff
}

// Usernames lists all accounts.
func (s *Store) Usernames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.accounts))
	for _, a := range s.accounts {
		out = append(out, a.Username)
	}
	sort.Strings(out)
	return out
}

// AdminAdjust adds (or removes) chips without touching the lifetime net.
func (s *Store) AdminAdjust(username string, delta int64) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return 0, false
	}
	if a.Balance+delta < 0 {
		delta = -a.Balance
	}
	a.Balance += delta
	a.AdminAdjust += delta
	s.touched(a)
	return a.Balance, true
}

// ResetStats clears an account's lifetime stats and sets its balance.
func (s *Store) ResetStats(username string, balance int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return false
	}
	a.Balance, a.Net, a.Won, a.Lost = balance, 0, 0, 0
	a.HandsPlayed, a.HandsWon, a.HandsLost, a.HandsPushed, a.Blackjacks = 0, 0, 0, 0, 0
	a.BiggestWin, a.RefillTotal, a.AdminAdjust = 0, 0, 0
	a.Wagered, a.PeakBalance, a.LowestNet = 0, balance, 0
	a.WinStreak, a.LoseStreak, a.BestWinStreak, a.WorstLoseStreak = 0, 0, 0, 0
	a.Busts, a.DoublesWon, a.Splits, a.RefillCount = 0, 0, 0, 0
	a.Unlocked, a.Hat, a.Frame, a.Title = nil, "", "", ""
	s.touched(a)
	return true
}

// Delete removes an account.
func (s *Store) Delete(username string) bool {
	key := strings.ToLower(username)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[key]; !ok {
		return false
	}
	delete(s.accounts, key)
	_ = os.Remove(s.avatarPath(key))
	s.recomputeRanks()
	s.dirty = true
	return true
}

// SetPassword changes an account password and revokes saved logins (admin).
func (s *Store) SetPassword(username, password string) bool {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	h := hashPassword(password, salt)
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return false
	}
	a.Salt, a.PassHash, a.Tokens = base64.StdEncoding.EncodeToString(salt), h, nil
	s.dirty = true
	return true
}

// SetOnline marks a user as connected or not.
func (s *Store) SetOnline(username string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(username)
	if on {
		s.online[key] = true
	} else {
		delete(s.online, key)
	}
	if a, ok := s.accounts[key]; ok {
		a.LastSeen = time.Now()
		s.dirty = true
	}
}

// Leaderboard sort orders.
const (
	SortNet     = "net"     // lifetime gain/loss
	SortBalance = "balance" // most chips right now
)

// Leaderboard returns accounts sorted by lifetime net (SortNet) or current
// chips (SortBalance), best first.
func (s *Store) Leaderboard(limit int, by string) []proto.Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]proto.Profile, 0, len(s.accounts))
	for _, a := range s.accounts {
		out = append(out, s.profileOf(a))
	}
	sort.Slice(out, func(i, j int) bool {
		a1, a2, b1, b2 := out[i].Net, out[j].Net, out[i].Balance, out[j].Balance
		if by == SortBalance {
			a1, a2, b1, b2 = b1, b2, a1, a2
		}
		if a1 != a2 {
			return a1 > a2
		}
		if b1 != b2 {
			return b1 > b2
		}
		return strings.ToLower(out[i].Username) < strings.ToLower(out[j].Username)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Store) avatarPath(key string) string {
	return filepath.Join(s.dir, "avatars", key+".png")
}

// Avatar returns an account's avatar PNG, or nil.
func (s *Store) Avatar(username string) ([]byte, string) {
	key := strings.ToLower(username)
	s.mu.Lock()
	a, ok := s.accounts[key]
	ver := ""
	if ok {
		ver = a.AvatarVersion
	}
	s.mu.Unlock()
	if !ok || ver == "" {
		return nil, ""
	}
	b, err := os.ReadFile(s.avatarPath(key))
	if err != nil {
		return nil, ""
	}
	return b, ver
}

// SetAvatar replaces an account's avatar.
func (s *Store) SetAvatar(username string, data []byte) (string, error) {
	png, err := NormalizeAvatar(data)
	if err != nil {
		return "", err
	}
	key := strings.ToLower(username)
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[key]
	if !ok {
		return "", errors.New("no such account")
	}
	if err := writeFileSafe(s.avatarPath(key), png, false); err != nil {
		return "", err
	}
	a.AvatarVersion = avatarVersion(png)
	s.dirty = true
	return a.AvatarVersion, nil
}

func avatarVersion(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:6])
}

// NormalizeAvatar decodes an uploaded image, center-crops it to a square and
// downsamples it to AvatarSize x AvatarSize, returning PNG bytes.
func NormalizeAvatar(data []byte) ([]byte, error) {
	if len(data) > 4<<20 {
		return nil, errors.New("picture is too large (max 4 MB)")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("unsupported picture (use PNG, JPEG or GIF)")
	}
	if cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width < 1 || cfg.Height < 1 {
		return nil, errors.New("picture dimensions are too large")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("could not read picture")
	}
	out := imgutil.Downsample(img, AvatarSize)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WinStreak returns a player's current run of winning hands.
func (s *Store) WinStreak(username string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok {
		return a.WinStreak
	}
	return 0
}

// Refund gives back chips taken for a wager that never settled.
func (s *Store) Refund(username string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok && amount > 0 {
		a.Balance += amount
		s.dirty = true
	}
}

// ---------------------------------------------------------------- poker

// ErrNotEnough is returned when a buy-in is more than the balance.
var ErrNotEnough = errors.New("you don't have that many chips")

// BuyIn moves chips from a player's balance to their stack at a poker table.
func (s *Store) BuyIn(username, table string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	switch {
	case !ok:
		return errors.New("no such player")
	case amount <= 0:
		return errors.New("buy in for at least 1 chip")
	case a.Balance < amount:
		return ErrNotEnough
	case a.TableStack > 0 && a.TableID != table:
		return errors.New("you still have chips at another table")
	}
	a.Balance -= amount
	a.TableStack += amount
	a.TableID = table
	s.dirty = true
	return nil
}

// CashOut moves a player's poker stack back to their balance. It returns the
// chips moved.
func (s *Store) CashOut(username string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return 0
	}
	n := a.TableStack
	a.Balance += n
	a.TableStack, a.TableID = 0, ""
	s.dirty = true
	s.touched(a)
	return n
}

// TableStack is what a player has at a poker table, and which.
func (s *Store) TableStack(username string) (int64, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok {
		return a.TableStack, a.TableID
	}
	return 0, ""
}

// ReturnTableStacks cashes everyone out (at startup: nobody is at a table, so
// any chips still marked as on one were left there by a crash). It returns
// how many accounts it fixed.
func (s *Store) ReturnTableStacks() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.accounts {
		if a.TableStack != 0 || a.TableID != "" {
			a.Balance += a.TableStack
			a.TableStack, a.TableID = 0, ""
			n++
		}
	}
	if n > 0 {
		s.dirty = true
	}
	return n
}

// PokerHand is one settled Hold'em hand for one player.
type PokerHand struct {
	Delta    int64 // won minus put in
	Stack    int64 // their stack afterwards
	Put      int64 // what they put in (for the biggest-bet record)
	Pot      int64 // the pot they won (0 if they didn't)
	Category int   // their best hand if they saw the river or the showdown, else -1
	Bluff    bool  // won without a showdown holding nothing
	BeatDale bool  // won a showdown Dale was in
}

// RecordPokerHand applies a settled hand to an account.
func (s *Store) RecordPokerHand(username string, h PokerHand) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[strings.ToLower(username)]
	if !ok {
		return
	}
	a.TableStack = h.Stack
	a.Net += h.Delta
	if h.Delta > 0 {
		a.Won += h.Delta
		a.PokerWins++
	} else {
		a.Lost += -h.Delta
	}
	a.PokerHands++
	a.Wagered += h.Put
	if h.Put > a.BiggestBet {
		a.BiggestBet = h.Put
	}
	if h.Delta > a.BiggestWin {
		a.BiggestWin = h.Delta
	}
	if h.Pot > a.PokerBiggestPot {
		a.PokerBiggestPot = h.Pot
	}
	switch h.Category {
	case poker.Flush:
		a.PokerFlushes++
	case poker.FullHouse:
		a.PokerFullHouses++
	case poker.Quads:
		a.PokerQuads++
	case poker.StraightFlush:
		a.PokerStrFlushes++
	case poker.RoyalFlush:
		a.PokerRoyals++
		a.PokerStrFlushes++ // a royal is a straight flush too
	}
	if h.Bluff {
		a.PokerBluffs++
	}
	if h.BeatDale {
		a.PokerBeatDale++
	}
	s.touched(a)
}

// RecordPokerHouse adds Dale's result in one Hold'em hand to the house.
func (s *Store) RecordPokerHouse(delta int64) (proto.HouseStats, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := &s.house
	prev := h.Tier
	h.PokerHands++
	h.PokerNet += delta
	h.Net += delta
	switch {
	case delta > 0:
		h.Won += delta
	case delta < 0:
		h.Lost += -delta
	}
	h.Tier = banter.Tier(h.Net, h.Tier)
	s.dirty = true
	return *h, prev
}

// SetTableStack sets what a player has at a poker table (after a hand is
// called off).
func (s *Store) SetTableStack(username string, stack int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.accounts[strings.ToLower(username)]; ok {
		a.TableStack = stack
		s.dirty = true
	}
}
