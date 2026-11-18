// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// TestPreviewNew renders the card reveal, chat, gift and admin screens.
func TestPreviewNew(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Username: "You_There", Sound: true}
	a := New(fakePlatform{}, cfg, assets)
	if sz := os.Getenv("PREVIEW_SIZE"); sz != "" {
		var w, h int
		fmt.Sscanf(sz, "%dx%d", &w, &h)
		a.SetSize(w, h)
		defer a.SetSize(640, 360)
	}
	a.screen = scrTable
	a.username = "You_There"
	a.profile = proto.Profile{Username: "You_There", Balance: 1048, Admin: true}
	cols := []color.RGBA{{200, 60, 60, 255}, {60, 120, 220, 255}, {60, 180, 90, 255}, {200, 160, 40, 255}, {150, 70, 200, 255}, {40, 180, 180, 255}, {220, 110, 40, 255}}
	for i, n := range []string{"Cmacle", "LuckyLou", "Big_Tony", "You_There", "Deb", "card_shark", "Mo"} {
		fakeAvatar(a, n, cols[i])
	}
	frame := func() { a.Frame(Input{MX: -1, MY: -1}) }

	st := baseState(7)
	st.Settings.TableChat, st.Settings.ChipGifts, st.Settings.CardReveal = true, true, true
	st.Phase = proto.PhasePlaying
	st.Dealer.Cards = append(st.Dealer.Cards, card(10, 0), card(0, 0))
	for i := range st.Seats {
		if st.Seats[i].Username != "" {
			st.Seats[i].Hands = []proto.HandView{{Bet: 100, Cards: []bj.Card{card(9, 1), card(7, 2)}, Total: 16}}
			st.Seats[i].Status = "playing"
		}
	}
	st.YourSeat = 4
	st.ActiveSeat = 4
	me := &st.Seats[4]
	me.Hands[0].Cards = append(me.Hands[0].Cards, card(0, 0))
	st.Reveal = &proto.RevealView{Seat: 4, Hand: 0, Card: 2, Until: time.Now().Add(2 * time.Second).UnixMilli()}
	a.onState(st)
	for i := 0; i < 3; i++ {
		frame()
	}
	a.rev.start = a.rev.start.Add(-time.Second)
	frame()
	save(t, dir, "20_reveal_wait.png", a.fb)

	// the flip
	st2 := *st
	st2.Reveal = nil
	me2 := st2.Seats[4]
	me2.Hands = []proto.HandView{{Bet: 100, Cards: []bj.Card{card(9, 1), card(7, 2), card(4, 3)}, Total: 20}}
	seats := append([]proto.SeatView(nil), st2.Seats...)
	seats[4] = me2
	st2.Seats = seats
	a.onEvent(proto.Event{Kind: "reveal", Seat: 4, Text: "great"})
	a.onState(&st2)
	a.rev.flippedAt = a.rev.flippedAt.Add(-130 * time.Millisecond)
	frame()
	save(t, dir, "21_reveal_flip.png", a.fb)
	a.rev.flippedAt = a.rev.flippedAt.Add(-500 * time.Millisecond)
	frame()
	save(t, dir, "22_reveal_face.png", a.fb)
	a.rev = nil

	// chat bubbles
	a.onChat(proto.Chat{From: "Cmacle", Seat: 0, Text: "hit me baby one more time"})
	a.onChat(proto.Chat{From: "Big_Tony", Seat: 3, Text: "dealer is definitely counting cards, I'm telling you right now", Admin: true})
	a.onChat(proto.Chat{From: "Deb", Seat: 6, Text: "gg"})
	a.openChat("/set card_reveal_max_seconds 4")
	frame()
	save(t, dir, "23_chat.png", a.fb)
	a.closeChat()
	a.onConsole(proto.Console{Lines: []string{"> /players", "  Cmacle seat 0  1,234 chips", "  LuckyLou seat 2  20 chips", "  You_There seat 4 (admin)"}})
	frame()
	save(t, dir, "24_console_peek.png", a.fb)

	// gift dialog
	a.giftTo = "LuckyLou"
	a.giftInfo = proto.GiftInfo{To: "LuckyLou", Max: 980, Balance: 20, Reason: "Up to 980: that brings them to 1,000."}
	a.giftAmt = "500"
	a.overlay = ovGift
	frame()
	save(t, dir, "25_gift.png", a.fb)

	// admin
	a.overlay = ovAdmin
	a.onSettings(proto.SettingsList{Settings: []proto.SettingInfo{
		{Key: "server_name", Label: "Server name", Kind: "string", Value: "The House Always Wins"},
		{Key: "port", Label: "Port", Kind: "int", Value: "7777"},
		{Key: "card_reveal", Label: "Card reveal", Kind: "bool", Value: "true"},
		{Key: "card_reveal_min_seconds", Label: "Card reveal min seconds", Kind: "float", Value: "0.5"},
		{Key: "card_reveal_max_seconds", Label: "Card reveal max seconds", Kind: "float", Value: "3"},
		{Key: "chip_gifts", Label: "Chip gifts", Kind: "bool", Value: "true"},
		{Key: "table_chat", Label: "Table chat", Kind: "bool", Value: "false"},
		{Key: "turbo_mode", Label: "Turbo mode", Kind: "bool", Value: "false"},
		{Key: "discord.enabled", Label: "Enabled", Kind: "bool", Value: "false", Group: "discord"},
	}, Changed: "card_reveal"})
	frame()
	save(t, dir, "26_admin_settings.png", a.fb)
	a.adminTab = 1
	frame()
	save(t, dir, "27_admin_console.png", a.fb)
}

// TestPreviewPiles renders the chip heap at a range of amounts.
func TestPreviewPiles(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{}, assets)
	a.SetSize(640, 360)
	a.computeLayout()
	a.drawTableBG()
	for i, amt := range []int64{5, 40, 350, 1300, 9800, 45000, 250000} {
		x := 60 + i*80
		for k, left := range []bool{false, true} {
			y := 100 + k*80
			ar := image.Rect(x-16, y, x+16, y+32)
			blit(a.fb, a.A.DefaultAvatar, ar.Min.X, ar.Min.Y)
			px := ar.Max.X + 3
			if left {
				px = ar.Min.X - 3 - pileChip
			}
			a.drawChipPile(px, ar.Max.Y, amt, left)
		}
	}
	save(t, dir, "30_piles.png", a.fb)
}

// TestPreviewUniqueHats draws the most-hands and biggest-bet sets on a portrait.
func TestPreviewUniqueHats(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{}, assets)
	a.SetSize(640, 360)
	a.computeLayout()
	a.drawTableBG()
	for i, h := range []string{"crown", "laurel", "nightcap", "chip_tower", "visor", "sailor", "fedora", "jester", "bowler", "shark_fin", "pharaoh", "turban"} {
		x, y := 30+(i%6)*100, 70+(i/6)*120
		frame := "frame_gold"
		if h == "nightcap" {
			frame = "frame_marathon"
		}
		a.drawFrame(frame, x-2, y-2)
		blit(a.fb, a.A.DefaultAvatar, x, y)
		a.drawHat(h, x, y)
		a.text(x+16, y+40, h, a.A.C("text"), alignCenter)
	}
	save(t, dir, "31_unique_hats.png", a.fb)
}

// TestPreviewWardrobeRecords shows the holder of a record cosmetic on hover.
func TestPreviewWardrobeRecords(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{}, assets)
	a.screen = scrTable
	a.username = "You_There"
	a.profile = proto.Profile{Username: "You_There", HandsPlayed: 812, BiggestBet: 400}
	a.records = []proto.Record{{Metric: "rank_hands", User: "LuckyLou", Value: 1204}, {Metric: "rank_bet", User: "Big_Tony", Value: 5000}}
	a.recordsAt = time.Now()
	a.st = baseState(7)
	a.overlay = ovWardrobe
	a.wardTab = "hat"
	a.wardSel = "nightcap"
	a.Frame(Input{MX: -1, MY: -1})
	save(t, dir, "32_wardrobe_record.png", a.fb)
	a.wardSel = "chip_tower"
	a.Frame(Input{MX: -1, MY: -1})
	save(t, dir, "33_wardrobe_record2.png", a.fb)
}
