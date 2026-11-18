// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func testConn() *conn { return &conn{out: make(chan []byte, 1000), closed: make(chan struct{})} }

func giftGame(t *testing.T) (*Game, *player, *player) {
	g := storyGame(t)
	if err := g.cfg.validate(); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob"} {
		if _, _, err := g.store.Register(u, "pw123456", nil, 1000, ""); err != nil {
			t.Fatal(err)
		}
	}
	a := &player{user: "alice", conn: testConn(), seat: 0}
	b := &player{user: "bob", conn: testConn(), seat: 1}
	g.seats[0], g.seats[1] = a, b
	g.byUser["alice"], g.byUser["bob"] = a, b
	return g, a, b
}

func TestGiftLimits(t *testing.T) {
	g, a, b := giftGame(t)
	// bob has the refill amount already: nothing
	if max, _ := g.giftMax(a, "bob"); max != 0 {
		t.Fatalf("bob at the refill amount can get %d", max)
	}
	g.store.AdminAdjust("bob", -900) // bob has 100
	g.store.AdminAdjust("alice", 5000)
	if max, why := g.giftMax(a, "bob"); max != 900 {
		t.Fatalf("max %d (%s), want 900", max, why)
	}
	// chips on the table count
	b.inHand = true
	b.hands = []*bj.Hand{{Bet: 50}}
	if max, _ := g.giftMax(a, "bob"); max != 850 {
		t.Fatalf("with 50 on the table max %d, want 850", max)
	}
	b.inHand, b.hands = false, nil
	netBefore, _ := g.store.Profile("bob")
	g.onGift(a, "bob", 900)
	if bal := g.store.Balance("bob"); bal != 1000 {
		t.Fatalf("bob has %d after the gift", bal)
	}
	after, _ := g.store.Profile("bob")
	if after.Net != netBefore.Net-900 {
		t.Fatalf("gift counted as winnings: net %d -> %d", netBefore.Net, after.Net)
	}
	// daily cap: bob loses it all again, alice can't refill him today
	g.store.AdminAdjust("bob", -1000)
	if max, why := g.giftMax(a, "bob"); max != 100 {
		t.Fatalf("after 900 today max %d (%s), want the 100 left", max, why)
	}
	// can't send more than you have, or to yourself
	if max, _ := g.giftMax(a, "alice"); max != 0 {
		t.Fatal("gift to self allowed")
	}
	g.cfg.GiftRefillsPerDay = 0
	g.store.AdminAdjust("alice", -g.store.Balance("alice")+30)
	if max, _ := g.giftMax(a, "bob"); max != 30 {
		t.Fatalf("alice has 30, max %d", max)
	}
	g.cfg.ChipGifts = false
	if max, _ := g.giftMax(a, "bob"); max != 0 {
		t.Fatal("gifts off but allowed")
	}
}

func TestChatAndAdminCommands(t *testing.T) {
	g, a, b := giftGame(t)
	g.conns = map[*conn]*player{a.conn: a, b.conn: b}
	var ranv atomic.Value
	ranv.Store("")
	ran := func() string { return ranv.Load().(string) }
	g.exec = func(line string, w io.Writer) { ranv.Store(line); io.WriteString(w, "ok\n") }
	g.onChat(a, "hello table")
	if got := drain(b.conn); !strings.Contains(got, `"hello table"`) {
		t.Fatalf("bob didn't see the chat: %s", got)
	}
	g.onChat(a, "/players")
	if ran() != "" {
		t.Fatal("non-admin ran a command")
	}
	g.store.SetAdmin("alice", true)
	g.onChat(a, "/players")
	for i := 0; i < 100 && ran() == ""; i++ {
		waitABit()
	}
	if ran() != "/players" {
		t.Fatalf("admin command not run: %q", ran())
	}
	// flood control
	drain(b.conn)
	for i := 0; i < 10; i++ {
		g.onChat(b, "spam")
	}
	if n := strings.Count(drain(a.conn), `"spam"`); n != 4 {
		t.Fatalf("%d spam messages got through, want 4", n)
	}
}

func drain(c *conn) string {
	var b strings.Builder
	for {
		select {
		case m := <-c.out:
			b.Write(m)
		default:
			return b.String()
		}
	}
}

func TestSetSetting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")
	cfg, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = dir
	s, err := New(cfg, dir, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	s.configPath = path
	s.reloadFn = func() error {
		nc, _, err := LoadConfig(path)
		if err == nil {
			s.cfgp.Store(nc)
		}
		return err
	}
	if err := s.SetSetting("card_reveal", "off"); err != nil {
		t.Fatal(err)
	}
	if s.cfg().CardReveal {
		t.Fatal("card_reveal still on")
	}
	if err := s.SetSetting("card_reveal_max_seconds", "4.5"); err != nil || s.cfg().CardRevealMaxSeconds != 4.5 {
		t.Fatalf("float setting: %v %v", err, s.cfg().CardRevealMaxSeconds)
	}
	if err := s.SetSetting("discord.leaderboard_size", "7"); err != nil || s.cfg().Discord.LeaderboardSize != 7 {
		t.Fatalf("nested setting: %v", err)
	}
	if err := s.SetSetting("port", "banana"); err == nil {
		t.Fatal("bad number accepted")
	}
	if err := s.SetSetting("port", "99999"); err == nil {
		t.Fatal("invalid port accepted")
	}
	if err := s.SetSetting("nope", "1"); err == nil {
		t.Fatal("unknown key accepted")
	}
	b, _ := os.ReadFile(path)
	var raw map[string]any
	if json.Unmarshal(b, &raw) != nil || raw["card_reveal"] != false || raw["port"].(float64) != 7777 && raw["port"].(float64) == 99999 {
		t.Fatalf("file: %s", b)
	}
	// console commands
	var out bytes.Buffer
	s.Exec("admin nobody", &out)
	if !strings.Contains(out.String(), "no such account") {
		t.Fatal(out.String())
	}
	out.Reset()
	s.Exec("get card_reveal", &out)
	if !strings.Contains(out.String(), "card_reveal = false") {
		t.Fatal(out.String())
	}
	_ = proto.Chat{}
}

func waitABit() { timeSleep(10) }

func timeSleep(ms int) { <-time.After(time.Duration(ms) * time.Millisecond) }

// A seated player mustn't be told the table is full while a card is being
// revealed (the reveal only hides the buttons).
func TestRevealKeepsSeatMessage(t *testing.T) {
	g, a, _ := giftGame(t)
	g.phase = proto.PhasePlaying
	g.reveal = &revealing{seat: 0, hand: 0, card: 2, until: time.Now().Add(time.Second)}
	st := g.view(a)
	if strings.Contains(st.Message, "full") || len(st.Actions) != 0 || st.Reveal == nil {
		t.Fatalf("during a reveal: message %q, actions %v", st.Message, st.Actions)
	}
}
