package discord

import (
	"encoding/json"

	"errors"
	"fmt"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"strings"
	"sync/atomic"
	"time"
)

type gwPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

var errFatal = errors.New("fatal gateway error")

func (b *Bot) gatewayLoop(stop <-chan struct{}) {
	backoff := 5 * time.Second
	registered := false
	for {
		start := time.Now()
		err := b.runGateway(stop, &registered)
		select {
		case <-stop:
			return
		default:
		}
		if errors.Is(err, errFatal) {
			b.log.Printf("discord: %v - slash commands disabled until restart", err)
			return
		}
		if time.Since(start) > 5*time.Minute {
			backoff = 5 * time.Second
		}
		b.log.Printf("discord: gateway disconnected (%v), reconnecting in %s", err, backoff)
		select {
		case <-stop:
			return
		case <-time.After(backoff):
		}
		if backoff < 2*time.Minute {
			backoff *= 2
		}
	}
}

func (b *Bot) runGateway(stop <-chan struct{}, registered *bool) error {
	var gw struct {
		URL string `json:"url"`
	}
	if err := b.api("GET", "/gateway/bot", nil, &gw); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == 401 {
			return fmt.Errorf("%w: bot token rejected (401)", errFatal)
		}
		return err
	}
	if gw.URL == "" {
		gw.URL = "wss://gateway.discord.gg"
	}
	ws, err := dialWS(strings.TrimRight(gw.URL, "/") + "/?v=10&encoding=json")
	if err != nil {
		return err
	}
	defer ws.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-stop:
			ws.Close()
		case <-done:
		}
	}()

	var seq atomic.Int64
	seq.Store(-1)
	var acked atomic.Bool
	acked.Store(true)
	send := func(op int, d any) error {
		raw, _ := json.Marshal(map[string]any{"op": op, "d": d})
		return ws.WriteText(raw)
	}
	heartbeat := func() error {
		s := seq.Load()
		if s < 0 {
			return send(1, nil)
		}
		return send(1, s)
	}

	for {
		raw, err := ws.ReadMessage()
		if err != nil {
			var ce *closeError
			if errors.As(err, &ce) {
				switch ce.Code {
				case 4004:
					return fmt.Errorf("%w: authentication failed (check bot_token)", errFatal)
				case 4010, 4011, 4012, 4013, 4014:
					return fmt.Errorf("%w: %s", errFatal, ce.Error())
				}
			}
			return err
		}
		var p gwPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.S != nil {
			seq.Store(*p.S)
		}
		switch p.Op {
		case 10: // hello
			var h struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			_ = json.Unmarshal(p.D, &h)
			iv := time.Duration(h.HeartbeatInterval) * time.Millisecond
			if iv <= 0 {
				iv = 40 * time.Second
			}
			go func() {
				t := time.NewTicker(iv)
				defer t.Stop()
				for {
					select {
					case <-done:
						return
					case <-t.C:
						if !acked.Load() {
							ws.Close() // zombie connection
							return
						}
						acked.Store(false)
						if heartbeat() != nil {
							return
						}
					}
				}
			}()
			if err := send(2, map[string]any{
				"token":   b.cfg.BotToken,
				"intents": 0,
				"properties": map[string]string{
					"os": "windows", "browser": "thehousealwayswins", "device": "thehousealwayswins",
				},
			}); err != nil {
				return err
			}
		case 1:
			_ = heartbeat()
		case 11:
			acked.Store(true)
		case 7:
			return errors.New("server requested reconnect")
		case 9:
			time.Sleep(3 * time.Second)
			return errors.New("invalid session")
		case 0:
			switch p.T {
			case "READY":
				var r struct {
					User struct {
						Username string `json:"username"`
					} `json:"user"`
					Application struct {
						ID string `json:"id"`
					} `json:"application"`
				}
				_ = json.Unmarshal(p.D, &r)
				b.log.Printf("discord: connected as %s", r.User.Username)
				if !*registered && r.Application.ID != "" {
					if err := b.registerCommands(r.Application.ID); err != nil {
						b.log.Printf("discord: registering slash commands failed: %v", err)
					} else {
						*registered = true
					}
				}
			case "INTERACTION_CREATE":
				go b.handleInteraction(p.D)
			}
		}
	}
}

func (b *Bot) registerCommands(appID string) error {
	cmds := []map[string]any{
		{
			"name": "leaderboard", "description": "Show the richest players and the lifetime gain/loss leaders", "type": 1,
			"options": []map[string]any{
				{
					"type": 3, "name": "board", "description": "Which leaderboard", "required": false,
					"choices": []map[string]string{
						{"name": "Both", "value": "both"},
						{"name": "Richest right now", "value": "richest"},
						{"name": "Lifetime gain/loss", "value": "lifetime"},
					},
				},
				{
					"type": 4, "name": "count", "description": "How many players to show (1-25)",
					"required": false, "min_value": 1, "max_value": 25,
				},
			},
		},
		{
			"name": "stats", "description": "Show a player's blackjack stats", "type": 1,
			"options": []map[string]any{{
				"type": 3, "name": "player", "description": "In-game username", "required": true,
			}},
		},
		{"name": "table", "description": "See who is at the blackjack table right now", "type": 1},
	}
	path := "/applications/" + appID + "/commands"
	if b.cfg.GuildID != "" {
		path = "/applications/" + appID + "/guilds/" + b.cfg.GuildID + "/commands"
	}
	if err := b.api("PUT", path, cmds, nil); err != nil {
		return err
	}
	b.log.Printf("discord: slash commands /leaderboard /stats /table registered")
	return nil
}

func titleSuffix(id string) string {
	if it, ok := cosmetics.Get(id); ok {
		return " — " + it.Name
	}
	return ""
}

func (b *Bot) handleInteraction(raw json.RawMessage) {
	var in struct {
		ID    string `json:"id"`
		Token string `json:"token"`
		Type  int    `json:"type"`
		Data  struct {
			Name    string `json:"name"`
			Options []struct {
				Name  string          `json:"name"`
				Value json.RawMessage `json:"value"`
			} `json:"options"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &in) != nil || in.Type != 2 {
		return
	}
	opt := func(name string) string {
		for _, o := range in.Data.Options {
			if o.Name == name {
				var s string
				if json.Unmarshal(o.Value, &s) == nil {
					return s
				}
				return strings.Trim(string(o.Value), `"`)
			}
		}
		return ""
	}
	var e embed
	switch in.Data.Name {
	case "leaderboard":
		n := b.cfg.LeaderboardSize
		if v := opt("count"); v != "" {
			fmt.Sscanf(v, "%d", &n)
		}
		if n < 1 || n > 25 {
			n = 10
		}
		which := opt("board")
		if which != "richest" && which != "lifetime" {
			which = "both"
		}
		e = b.boardEmbed(n, which)
	case "stats":
		name := opt("player")
		p, ok := b.data.Profile(name)
		if !ok {
			e = embed{Description: fmt.Sprintf("No player named **%s**.", esc(name)), Color: colorRed}
			break
		}
		status := "offline"
		if p.Online {
			status = "🟢 online"
		}
		winRate := "—"
		if p.HandsPlayed > 0 {
			winRate = fmt.Sprintf("%.1f%%", float64(p.HandsWon)*100/float64(p.HandsPlayed))
		}
		e = embed{
			Title: p.Username + titleSuffix(p.Title) + " · " + status,
			Color: colorGold,
			Fields: []embedField{
				{Name: "Lifetime", Value: "`" + signed(p.Net) + "`", Inline: true},
				{Name: "Balance", Value: commas(p.Balance), Inline: true},
				{Name: "Biggest win", Value: commas(p.BiggestWin), Inline: true},
				{Name: "Hands", Value: fmt.Sprintf("%d (W %d / L %d / P %d)", p.HandsPlayed, p.HandsWon, p.HandsLost, p.HandsPushed), Inline: true},
				{Name: "Win rate", Value: winRate, Inline: true},
				{Name: "Blackjacks", Value: fmt.Sprint(p.Blackjacks), Inline: true},
				{Name: "Won / Lost", Value: commas(p.Won) + " / " + commas(p.Lost), Inline: true},
				{Name: "Refills received", Value: commas(p.RefillTotal), Inline: true},
				{Name: "Wagered", Value: commas(p.Wagered), Inline: true},
				{Name: "Most chips held", Value: commas(p.PeakBalance), Inline: true},
				{Name: "Unlocks", Value: fmt.Sprintf("%d / %d", len(p.Unlocked)+2, len(cosmetics.Catalog)-4), Inline: true},
			},
		}
	case "table":
		e = embed{Title: b.data.ServerName(), Description: b.data.TableSummary(), Color: colorTeal}
	default:
		return
	}
	body := map[string]any{"type": 4, "data": map[string]any{"embeds": []embed{e}, "allowed_mentions": map[string]any{"parse": []string{}}}}
	if err := b.do("POST", apiBase+"/interactions/"+in.ID+"/"+in.Token+"/callback", false, body, nil); err != nil {
		b.log.Printf("discord: interaction reply failed: %v", err)
	}
}
