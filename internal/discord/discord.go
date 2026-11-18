// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package discord posts the leaderboard and announcements to Discord and
// answers slash commands. It uses only the standard library: REST calls go
// through net/http and the gateway uses a small built-in websocket client.
package discord

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// apiBase is a variable so tests can point it at a fake server.
var apiBase = "https://discord.com/api/v10"

const (
	userAgent = "DiscordBot (https://github.com/Cmacle/TheHouseAlwaysWins, 1.0)"
	colorGold = 0xF2C14E
	colorRed  = 0xE04848
	colorTeal = 0x3FB68B
)

// Config is the subset of server settings the bot needs.
type Config struct {
	BotToken             string
	LeaderboardChannelID string
	AnnounceChannelID    string
	WebhookURL           string
	SlashCommands        bool
	GuildID              string
	LeaderboardSize      int
	UpdateInterval       time.Duration
	AnnounceRefills      bool
	StateFile            string
}

// Data is what the bot reads from the game.
type Data interface {
	ServerName() string
	Leaderboard(n int, rich bool) []proto.Profile
	Profile(name string) (proto.Profile, bool)
	TableSummary() string
	House() proto.HouseStats
}

// Bot is the Discord integration.
type Bot struct {
	cfg  Config
	data Data
	log  *log.Logger
	hc   *http.Client

	changed  chan struct{}
	announce chan embed

	mu    sync.Mutex
	state struct {
		LeaderboardMessageID string `json:"leaderboard_message_id"`
		LeaderboardChannelID string `json:"leaderboard_channel_id"`
		WebhookMessageID     string `json:"webhook_message_id"`
	}
	lastBoard string
}

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []embedField `json:"fields,omitempty"`
	Footer      *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// New creates the bot. Call Start to run it.
func New(cfg Config, data Data, lg *log.Logger) *Bot {
	b := &Bot{
		cfg: cfg, data: data, log: lg,
		hc:       &http.Client{Timeout: 20 * time.Second},
		changed:  make(chan struct{}, 1),
		announce: make(chan embed, 64),
	}
	if raw, err := os.ReadFile(cfg.StateFile); err == nil {
		_ = json.Unmarshal(raw, &b.state)
	}
	return b
}

func (b *Bot) useBot() bool { return b.cfg.BotToken != "" }

// Start runs the bot's background loops.
func (b *Bot) Start(stop <-chan struct{}) {
	if b.cfg.BotToken == "" && b.cfg.WebhookURL == "" {
		b.log.Printf("discord: enabled but no bot_token or webhook_url set - disabled")
		return
	}
	go b.boardLoop(stop)
	go b.announceLoop(stop)
	if b.useBot() && b.cfg.SlashCommands {
		go b.gatewayLoop(stop)
	}
	b.log.Printf("discord: integration started")
}

// LeaderboardChanged implements server.Notifier.
func (b *Bot) LeaderboardChanged() {
	select {
	case b.changed <- struct{}{}:
	default:
	}
}

// BigWin implements server.Notifier.
func (b *Bot) BigWin(user string, amount int64) {
	b.queue(embed{
		Title:       "Big win!",
		Description: fmt.Sprintf("**%s** just won **+%s** chips in a single hand.", esc(user), commas(amount)),
		Color:       colorTeal,
	})
}

// Unlocked implements server.Notifier: epic and better unlocks are announced.
func (b *Bot) Unlocked(user string, it *cosmetics.Item) {
	if it.Tier < cosmetics.Epic {
		return
	}
	kind := map[string]string{cosmetics.Hat: "hat", cosmetics.Frame: "frame", cosmetics.Title: "title"}[it.Kind]
	b.queue(embed{
		Title:       "✨ " + cosmetics.TierNames[it.Tier] + " unlock!",
		Description: fmt.Sprintf("**%s** unlocked the **%s** %s.\n*%s* (%s)", esc(user), it.Name, kind, it.Desc, it.Requirement()),
		Color:       tierColor(it.Tier),
	})
}

// NewTop implements server.Notifier: someone took the crown.
func (b *Bot) NewTop(user string) {
	b.queue(embed{
		Title:       "👑 New king of the table",
		Description: fmt.Sprintf("**%s** now has the most chips and wears the crown.", esc(user)),
		Color:       colorGold,
	})
	b.LeaderboardChanged()
}

func tierColor(t int) int {
	var c int
	fmt.Sscanf(strings.TrimPrefix(cosmetics.TierColors[t], "#"), "%x", &c)
	return c
}

// Refilled implements server.Notifier.
func (b *Bot) Refilled(users []string, amount int64) {
	if !b.cfg.AnnounceRefills || len(users) == 0 {
		return
	}
	names := make([]string, 0, len(users))
	for i, u := range users {
		if i == 15 {
			names = append(names, fmt.Sprintf("and %d more", len(users)-15))
			break
		}
		names = append(names, esc(u))
	}
	b.queue(embed{
		Title: "Daily refill",
		Description: fmt.Sprintf("%d player(s) were topped back up to **%s** chips (the refill counts against their lifetime total):\n%s",
			len(users), commas(amount), strings.Join(names, ", ")),
		Color: colorGold,
	})
}

func (b *Bot) queue(e embed) {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	select {
	case b.announce <- e:
	default:
	}
}

func (b *Bot) saveState() {
	b.mu.Lock()
	raw, _ := json.MarshalIndent(b.state, "", "  ")
	b.mu.Unlock()
	_ = os.WriteFile(b.cfg.StateFile, raw, 0o644)
}

// ---------------------------------------------------------------- REST

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("discord api %d: %s", e.Status, e.Body) }

func (b *Bot) do(method, url string, auth bool, body any, out any) error {
	for attempt := 0; attempt < 3; attempt++ {
		var rd io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			rd = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, url, rd)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if auth {
			req.Header.Set("Authorization", "Bot "+b.cfg.BotToken)
		}
		resp, err := b.hc.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			var rl struct {
				RetryAfter float64 `json:"retry_after"`
			}
			_ = json.Unmarshal(data, &rl)
			wait := time.Duration(rl.RetryAfter*1000) * time.Millisecond
			if wait <= 0 || wait > time.Minute {
				wait = 5 * time.Second
			}
			time.Sleep(wait)
			continue
		}
		if resp.StatusCode >= 300 {
			return &apiError{Status: resp.StatusCode, Body: truncate(string(data), 300)}
		}
		if out != nil && len(data) > 0 {
			return json.Unmarshal(data, out)
		}
		return nil
	}
	return errors.New("discord api: rate limited")
}

func (b *Bot) api(method, path string, body, out any) error {
	return b.do(method, apiBase+path, true, body, out)
}

type message struct {
	ID string `json:"id"`
}

// post sends an embed message to a channel (bot) or the webhook.
func (b *Bot) post(channel string, e embed) (string, error) {
	var m message
	payload := map[string]any{"embeds": []embed{e}, "allowed_mentions": map[string]any{"parse": []string{}}}
	if b.useBot() && channel != "" {
		err := b.api("POST", "/channels/"+channel+"/messages", payload, &m)
		return m.ID, err
	}
	if b.cfg.WebhookURL != "" {
		err := b.do("POST", webhookURL(b.cfg.WebhookURL)+"?wait=true", false, payload, &m)
		return m.ID, err
	}
	return "", errors.New("no channel configured")
}

func webhookURL(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	return strings.TrimRight(u, "/")
}

// ---------------------------------------------------------------- leaderboard

func (b *Bot) boardLines(n int, rich bool) string {
	entries := b.data.Leaderboard(n, rich)
	var sb strings.Builder
	if len(entries) == 0 {
		sb.WriteString("No players yet. Pull up a chair!\n")
	}
	for i, p := range entries {
		rank := fmt.Sprintf("`%2d.`", i+1)
		switch {
		case i == 0 && rich:
			rank = "👑"
		case i == 0:
			rank = "🥇"
		case i == 1:
			rank = "🥈"
		case i == 2:
			rank = "🥉"
		}
		online := ""
		if p.Online {
			online = " 🟢"
		}
		title := ""
		if it, ok := cosmetics.Get(p.Title); ok && p.Title != "t_rookie" {
			title = " *" + it.Name + "*"
		}
		if rich {
			fmt.Fprintf(&sb, "%s **%s**%s%s — `%s chips` · lifetime %s\n", rank, esc(p.Username), title, online, commas(p.Balance), signed(p.Net))
		} else {
			fmt.Fprintf(&sb, "%s **%s**%s%s — `%s` · %s chips · %d hands\n", rank, esc(p.Username), title, online, signed(p.Net), commas(p.Balance), p.HandsPlayed)
		}
	}
	return sb.String()
}

// boardEmbed builds the leaderboard. which is "both", "richest" or "lifetime".
func (b *Bot) boardEmbed(n int, which string) embed {
	e := embed{Color: colorGold, Title: "🏆 " + b.data.ServerName() + " — Leaderboards"}
	var sb strings.Builder
	if which != "lifetime" {
		sb.WriteString("**💰 Richest right now**\n")
		sb.WriteString(b.boardLines(n, true))
	}
	if which == "both" {
		sb.WriteString("\n")
	}
	if which != "richest" {
		sb.WriteString("**📈 Lifetime gain / loss**\n")
		sb.WriteString(b.boardLines(n, false))
	}
	if h := b.data.House(); h.HandsDealt > 0 {
		sb.WriteString("\n")
		sb.WriteString(houseLine(h))
	}
	e.Description = truncate(sb.String(), 4000)
	e.Footer = &struct {
		Text string `json:"text"`
	}{Text: "Lifetime = won minus lost, minus daily refills · 👑 = the crown"}
	return e
}

func houseLine(h proto.HouseStats) string {
	return fmt.Sprintf("**🎩 The House:** `%s` lifetime over %s rounds · won %s · lost %s · dealer blackjacks %d · dealer busts %d\n",
		signed(h.Net), commas(int64(h.HandsDealt)), commas(h.Won), commas(h.Lost), h.DealerBlackjacks, h.DealerBusts)
}

func (b *Bot) boardLoop(stop <-chan struct{}) {
	if b.cfg.LeaderboardChannelID == "" && !(b.cfg.WebhookURL != "" && !b.useBot()) {
		// no live leaderboard target; just drain change signals
		for {
			select {
			case <-stop:
				return
			case <-b.changed:
			}
		}
	}
	tick := time.NewTicker(b.cfg.UpdateInterval)
	defer tick.Stop()
	b.updateBoard(true)
	last := time.Now()
	pending := false
	for {
		var throttle <-chan time.Time
		if pending {
			throttle = time.After(time.Until(last.Add(20 * time.Second)))
		}
		select {
		case <-stop:
			return
		case <-b.changed:
			pending = true
		case <-throttle:
			b.updateBoard(false)
			last, pending = time.Now(), false
		case <-tick.C:
			b.updateBoard(true)
			last, pending = time.Now(), false
		}
	}
}

func (b *Bot) updateBoard(force bool) {
	e := b.boardEmbed(b.cfg.LeaderboardSize, "both")
	key := e.Description
	if !force && key == b.lastBoard {
		return
	}
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	payload := map[string]any{"embeds": []embed{e}, "allowed_mentions": map[string]any{"parse": []string{}}}

	b.mu.Lock()
	msgID := b.state.LeaderboardMessageID
	if b.state.LeaderboardChannelID != b.cfg.LeaderboardChannelID {
		msgID = ""
	}
	whID := b.state.WebhookMessageID
	b.mu.Unlock()

	var err error
	if b.useBot() {
		ch := b.cfg.LeaderboardChannelID
		if msgID != "" {
			err = b.api("PATCH", "/channels/"+ch+"/messages/"+msgID, payload, nil)
			var ae *apiError
			if errors.As(err, &ae) && ae.Status == 404 {
				msgID, err = "", nil
			}
		}
		if msgID == "" && err == nil {
			var id string
			id, err = b.post(ch, e)
			if err == nil {
				b.mu.Lock()
				b.state.LeaderboardMessageID, b.state.LeaderboardChannelID = id, ch
				b.mu.Unlock()
				b.saveState()
			}
		}
	} else {
		wh := webhookURL(b.cfg.WebhookURL)
		if whID != "" {
			err = b.do("PATCH", wh+"/messages/"+whID, false, payload, nil)
			var ae *apiError
			if errors.As(err, &ae) && ae.Status == 404 {
				whID, err = "", nil
			}
		}
		if whID == "" && err == nil {
			var id string
			id, err = b.post("", e)
			if err == nil {
				b.mu.Lock()
				b.state.WebhookMessageID = id
				b.mu.Unlock()
				b.saveState()
			}
		}
	}
	if err != nil {
		b.log.Printf("discord: leaderboard update failed: %v", err)
		return
	}
	b.lastBoard = key
}

func (b *Bot) announceLoop(stop <-chan struct{}) {
	ch := b.cfg.AnnounceChannelID
	if ch == "" {
		ch = b.cfg.LeaderboardChannelID
	}
	for {
		select {
		case <-stop:
			return
		case e := <-b.announce:
			if b.useBot() && ch == "" {
				continue
			}
			if _, err := b.post(ch, e); err != nil {
				b.log.Printf("discord: announcement failed: %v", err)
			}
			time.Sleep(time.Second)
		}
	}
}

// ---------------------------------------------------------------- helpers

func esc(s string) string {
	r := strings.NewReplacer("_", "\\_", "*", "\\*", "~", "\\~", "`", "\\`", "|", "\\|", ">", "\\>")
	return r.Replace(s)
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var sb strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(c)
	}
	if neg {
		return "-" + sb.String()
	}
	return sb.String()
}

func signed(n int64) string {
	if n > 0 {
		return "+" + commas(n)
	}
	return commas(n)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
