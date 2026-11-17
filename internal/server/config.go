package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
)

// DiscordConfig configures the Discord integration.
type DiscordConfig struct {
	Enabled bool `json:"enabled"`
	// Bot token from the Discord developer portal (Bot tab).
	BotToken string `json:"bot_token"`
	// Channel where the bot keeps a live leaderboard message. Optional.
	LeaderboardChannelID string `json:"leaderboard_channel_id"`
	// Channel for announcements (daily refills, big wins). Optional;
	// defaults to the leaderboard channel.
	AnnounceChannelID string `json:"announce_channel_id"`
	// Alternative to a bot: a channel webhook URL. Used for the live leaderboard
	// and announcements when no bot token is set.
	WebhookURL string `json:"webhook_url"`
	// Register /leaderboard, /stats and /table slash commands (bot token only).
	SlashCommands bool `json:"slash_commands"`
	// Optional guild (server) ID: slash commands registered to one guild appear
	// instantly; global commands can take a while the first time.
	GuildID               string `json:"guild_id"`
	LeaderboardSize       int    `json:"leaderboard_size"`
	UpdateIntervalMinutes int    `json:"update_interval_minutes"`
	AnnounceRefills       bool   `json:"announce_refills"`
	// Announce single-hand wins of at least this amount. 0 disables.
	BigWinThreshold int64 `json:"big_win_threshold"`
}

// HTTPConfig configures the optional web leaderboard.
type HTTPConfig struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// Config is the server configuration (server.json).
type Config struct {
	ServerName    string `json:"server_name"`
	MOTD          string `json:"motd"`
	ListenAddress string `json:"listen_address"`
	Port          int    `json:"port"`
	DataDir       string `json:"data_dir"`
	MaxPlayers    int    `json:"max_players"`
	MaxSpectators int    `json:"max_spectators"`

	AllowRegistration bool `json:"allow_registration"`

	// Daily refill: once per day at RefillTime, any account below RefillAmount is
	// topped up to RefillAmount and the top-up is subtracted from its lifetime
	// net. New accounts also start with RefillAmount.
	RefillAmount int64  `json:"refill_amount"`
	RefillTime   string `json:"refill_time"` // "HH:MM", 24h
	Timezone     string `json:"timezone"`    // IANA name like "America/Chicago", or "Local"

	MinBet int64 `json:"min_bet"`
	MaxBet int64 `json:"max_bet"`

	// "simultaneous" (everyone bets at once) or "turns" (one seat at a time).
	BettingMode string `json:"betting_mode"`
	// "turns" (one hand at a time, in seat order) or "simultaneous" (everyone
	// plays their hand at the same time).
	PlayMode string `json:"play_mode"`
	// Hide other players' cards until the hand is over.
	HideOtherHands bool `json:"hide_other_hands"`

	Decks            int     `json:"decks"`
	Penetration      float64 `json:"penetration"`
	DealerHitsSoft17 bool    `json:"dealer_hits_soft_17"`
	BlackjackPays    string  `json:"blackjack_pays"` // "3:2", "6:5", ...
	DoubleAfterSplit bool    `json:"double_after_split"`
	DoubleOn         string  `json:"double_on"` // any | 9-11 | 10-11
	MaxSplitHands    int     `json:"max_split_hands"`
	ResplitAces      bool    `json:"resplit_aces"`
	HitSplitAces     bool    `json:"hit_split_aces"`
	AllowSurrender   bool    `json:"allow_surrender"`

	BettingTimeSeconds int `json:"betting_time_seconds"`
	TurnTimeSeconds    int `json:"turn_time_seconds"`
	PlayTimeSeconds    int `json:"play_time_seconds"`
	ResultsTimeSeconds int `json:"results_time_seconds"`
	DealDelayMS        int `json:"deal_delay_ms"`
	DealerDelayMS      int `json:"dealer_delay_ms"`

	// The dealer talks trash: calls out players for busting, standing scared,
	// big losses, lifetime scores and so on, in a speech bubble.
	DealerTrashTalk bool `json:"dealer_trash_talk"`
	// Uncensored dealer: swearing, crude insults, grumpy old-man rants, and a
	// cigar-smoking scumbag sprite. Needs dealer_trash_talk.
	DealerVulgar bool `json:"dealer_vulgar"`

	HTTPLeaderboard HTTPConfig    `json:"http_leaderboard"`
	Discord         DiscordConfig `json:"discord"`

	loc *time.Location
	rh  int
	rm  int
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		ServerName:         "The House Always Wins",
		MOTD:               "Welcome to the table. Remember: the house always wins.",
		ListenAddress:      "0.0.0.0",
		Port:               7777,
		DataDir:            "data",
		MaxPlayers:         10,
		MaxSpectators:      20,
		AllowRegistration:  true,
		RefillAmount:       1000,
		RefillTime:         "00:00",
		Timezone:           "Local",
		MinBet:             10,
		MaxBet:             1000,
		BettingMode:        "simultaneous",
		PlayMode:           "turns",
		HideOtherHands:     false,
		Decks:              6,
		Penetration:        0.75,
		DealerHitsSoft17:   false,
		BlackjackPays:      "3:2",
		DoubleAfterSplit:   true,
		DoubleOn:           "any",
		MaxSplitHands:      4,
		ResplitAces:        false,
		HitSplitAces:       false,
		AllowSurrender:     false,
		BettingTimeSeconds: 15,
		TurnTimeSeconds:    20,
		PlayTimeSeconds:    30,
		ResultsTimeSeconds: 5,
		DealDelayMS:        350,
		DealerDelayMS:      700,
		DealerTrashTalk:    true,
		DealerVulgar:       false,
		HTTPLeaderboard:    HTTPConfig{Enabled: false, Address: "0.0.0.0", Port: 8090},
		Discord: DiscordConfig{
			SlashCommands:         true,
			LeaderboardSize:       10,
			UpdateIntervalMinutes: 5,
			AnnounceRefills:       true,
			BigWinThreshold:       0,
		},
	}
}

// LoadConfig reads path, creating it with defaults if it does not exist.
// Missing fields keep their default values.
func LoadConfig(path string) (*Config, bool, error) {
	cfg := DefaultConfig()
	created := false
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		out, _ := json.MarshalIndent(cfg, "", "  ")
		if werr := os.WriteFile(path, out, 0o644); werr != nil {
			return nil, false, fmt.Errorf("write default config: %w", werr)
		}
		created = true
	} else if err != nil {
		return nil, false, err
	} else if err := json.Unmarshal(b, cfg); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, false, err
	}
	// Settings added in newer versions get written into an existing file
	// (with their defaults) so they're easy to find. Existing values are kept.
	if !created {
		def, _ := json.Marshal(DefaultConfig())
		if missing := missingKeys(b, def, ""); len(missing) > 0 {
			out, _ := json.MarshalIndent(cfg, "", "  ")
			if err := writeAtomic(path, append(out, '\n')); err == nil {
				addedKeys = missing
			}
		}
	}
	return cfg, created, nil
}

// addedKeys lists settings LoadConfig added to an existing file (for logging).
var addedKeys []string

// missingKeys returns the dotted names of keys present in want but absent in
// have, looking inside nested objects.
func missingKeys(have, want []byte, prefix string) []string {
	var h, w map[string]json.RawMessage
	if json.Unmarshal(have, &h) != nil || json.Unmarshal(want, &w) != nil {
		return nil
	}
	var out []string
	for k, wv := range w {
		hv, ok := h[k]
		if !ok {
			out = append(out, prefix+k)
			continue
		}
		if len(wv) > 0 && wv[0] == '{' && len(hv) > 0 && hv[0] == '{' {
			out = append(out, missingKeys(hv, wv, prefix+k+".")...)
		}
	}
	sort.Strings(out)
	return out
}

func (c *Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.MaxPlayers < 1 {
		c.MaxPlayers = 1
	}
	if c.MaxPlayers > 16 {
		return errors.New("max_players can be at most 16")
	}
	if c.MinBet < 1 {
		c.MinBet = 1
	}
	if c.MaxBet < c.MinBet {
		c.MaxBet = c.MinBet
	}
	if c.RefillAmount < 0 {
		c.RefillAmount = 0
	}
	c.BettingMode = normMode(c.BettingMode, "simultaneous")
	c.PlayMode = normMode(c.PlayMode, "turns")
	switch c.DoubleOn {
	case "any", "9-11", "10-11":
	default:
		c.DoubleOn = "any"
	}
	if c.Decks < 1 {
		c.Decks = 1
	}
	if c.Decks > 8 {
		c.Decks = 8
	}
	if c.MaxSplitHands < 1 {
		c.MaxSplitHands = 1
	}
	if c.BettingTimeSeconds < 3 {
		c.BettingTimeSeconds = 3
	}
	if c.TurnTimeSeconds < 3 {
		c.TurnTimeSeconds = 3
	}
	if c.PlayTimeSeconds < 5 {
		c.PlayTimeSeconds = 5
	}
	if c.ResultsTimeSeconds < 1 {
		c.ResultsTimeSeconds = 1
	}
	if c.DealDelayMS < 0 {
		c.DealDelayMS = 0
	}
	if c.DealerDelayMS < 0 {
		c.DealerDelayMS = 0
	}
	if _, _, err := c.bjPays(); err != nil {
		return err
	}
	if c.Timezone == "" || strings.EqualFold(c.Timezone, "local") {
		c.loc = time.Local
	} else {
		loc, err := time.LoadLocation(c.Timezone)
		if err != nil {
			return fmt.Errorf("timezone %q: %w", c.Timezone, err)
		}
		c.loc = loc
	}
	var h, m int
	if _, err := fmt.Sscanf(c.RefillTime, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return fmt.Errorf("refill_time %q must be HH:MM", c.RefillTime)
	}
	c.rh, c.rm = h, m
	if c.Discord.LeaderboardSize <= 0 {
		c.Discord.LeaderboardSize = 10
	}
	if c.Discord.LeaderboardSize > 25 {
		c.Discord.LeaderboardSize = 25
	}
	if c.Discord.UpdateIntervalMinutes <= 0 {
		c.Discord.UpdateIntervalMinutes = 5
	}
	return nil
}

func normMode(s, def string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "simultaneous", "turns":
		return s
	case "turn", "sequential", "taking_turns", "take_turns":
		return "turns"
	case "all", "all_at_once", "simultaneously":
		return "simultaneous"
	}
	return def
}

func (c *Config) bjPays() (int64, int64, error) {
	parts := strings.Split(c.BlackjackPays, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("blackjack_pays %q must look like 3:2", c.BlackjackPays)
	}
	n, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	d, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 {
		return 0, 0, fmt.Errorf("blackjack_pays %q must look like 3:2", c.BlackjackPays)
	}
	return n, d, nil
}

// Rules converts the config into blackjack rules.
func (c *Config) Rules() blackjack.Rules {
	n, d, _ := c.bjPays()
	return blackjack.Rules{
		DealerHitsSoft17: c.DealerHitsSoft17,
		BlackjackNum:     n,
		BlackjackDen:     d,
		DoubleAfterSplit: c.DoubleAfterSplit,
		DoubleOn:         c.DoubleOn,
		MaxHands:         c.MaxSplitHands,
		ResplitAces:      c.ResplitAces,
		HitSplitAces:     c.HitSplitAces,
		Surrender:        c.AllowSurrender,
	}
}

// RefillPeriod returns an identifier for the refill period containing t: the
// date of the most recent refill boundary at or before t.
func (c *Config) RefillPeriod(t time.Time) string {
	lt := t.In(c.loc)
	boundary := time.Date(lt.Year(), lt.Month(), lt.Day(), c.rh, c.rm, 0, 0, c.loc)
	if lt.Before(boundary) {
		boundary = boundary.AddDate(0, 0, -1)
	}
	return boundary.Format("2006-01-02")
}

// NextRefill returns the time of the next refill boundary after t.
func (c *Config) NextRefill(t time.Time) time.Time {
	lt := t.In(c.loc)
	boundary := time.Date(lt.Year(), lt.Month(), lt.Day(), c.rh, c.rm, 0, 0, c.loc)
	if !lt.Before(boundary) {
		boundary = boundary.AddDate(0, 0, 1)
	}
	return boundary
}
