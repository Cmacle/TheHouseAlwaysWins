// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/server"
)

func startServer(t *testing.T) string { return startServerWith(t, 39517, nil) }

func startServerWith(t *testing.T, port int, mut func(*server.Config)) string {
	return startServerS(t, port, mut).Addr().String()
}

func startServerS(t *testing.T, port int, mut func(*server.Config)) *server.Server {
	cfg := server.DefaultConfig()
	cfg.ListenAddress = "127.0.0.1"
	cfg.Port = port
	cfg.DataDir = t.TempDir()
	cfg.DealDelayMS, cfg.DealerDelayMS = 5, 5
	cfg.ResultsTimeSeconds = 1
	if mut != nil {
		mut(cfg)
	}
	s, err := server.New(cfg, "", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	s.RunTables(stop)
	go s.Serve()
	t.Cleanup(func() { s.Close(); close(stop) })
	return s
}

// pumpWait is how long pump waits; a full poker hand with reveals is slower.
var pumpWait = 15 * time.Second

func pump(t *testing.T, apps []*App, inputs map[*App]Input, until func() bool, what string) {
	deadline := time.Now().Add(pumpWait)
	for first := true; first || !until(); first = false {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		for _, a := range apps {
			in := inputs[a]
			if in.MX == 0 && in.MY == 0 {
				in.MX, in.MY = -1, -1
			}
			a.Frame(in)
		}
		inputs = map[*App]Input{}
		time.Sleep(10 * time.Millisecond)
	}
}

func newTestApp(t *testing.T, addr, user string) *App {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{Server: addr, Sound: true}, assets)
	a.fServer, a.fUser, a.fPass, a.fPass2 = addr, user, "pass1234", "pass1234"
	a.loginMode = modeRegister
	a.doRegister()
	return a
}

// joinFirst waits for the lobby and joins its first table (blackjack).
func joinFirst(t *testing.T, apps ...*App) {
	pump(t, apps, nil, func() bool {
		for _, a := range apps {
			if a.screen != scrLobby || !a.lobbyKnown {
				return false
			}
		}
		return true
	}, "lobby")
	for _, a := range apps {
		a.joinTable(a.lobby.Tables[0])
	}
}

func TestEndToEnd(t *testing.T) {
	addr := startServer(t)
	a := newTestApp(t, addr, "alice")
	b := newTestApp(t, addr, "bob_99")
	apps := []*App{a, b}
	joinFirst(t, a, b)
	pump(t, apps, nil, func() bool { return a.screen == scrTable && b.screen == scrTable && a.st != nil && b.st != nil }, "both at table")
	pump(t, apps, nil, func() bool { return a.st.CanBet && b.st.CanBet }, "betting")

	// alice clicks the 10 chip three times and presses Enter; bob uses the keyboard after clicking 25
	chip := func(i int) Input {
		cs := a.A.Theme.ChipSize
		return Input{MX: 168 + 4 + i*(cs+4) + 5, MY: ScreenH - 62 + 7 + 5, Pressed: true, Down: true, MouseInside: true}
	}
	for k := 0; k < 3; k++ {
		pump(t, apps, map[*App]Input{a: chip(1), b: chip(2)}, func() bool { return true }, "")
		pump(t, apps, nil, func() bool { return true }, "")
	}
	if a.betDraft != 30 || b.betDraft != 75 {
		t.Fatalf("bet drafts %d %d", a.betDraft, b.betDraft)
	}
	pump(t, apps, map[*App]Input{a: {Keys: []Key{KeyEnter}}, b: {Keys: []Key{KeyEnter}}}, func() bool { return true }, "")
	pump(t, apps, nil, func() bool {
		return a.st.Phase == proto.PhasePlaying || a.st.Phase == proto.PhaseDealer || a.st.Phase == proto.PhaseResults
	}, "deal")
	t.Logf("alice seat %d bet %v", a.st.YourSeat, a.st.Seats[a.st.YourSeat].Hands)

	// whoever's turn it is presses S (stand) until the round ends
	pump(t, apps, nil, func() bool {
		for _, x := range apps {
			if len(x.st.Actions) > 0 {
				x.Frame(Input{MX: -1, MY: -1, Chars: []rune{'s'}})
			}
		}
		return a.st.Phase == proto.PhaseResults
	}, "results")
	pump(t, apps, nil, func() bool { return a.profile.HandsPlayed == 1 && b.profile.HandsPlayed == 1 }, "profile update")
	t.Logf("alice net %d balance %d, bob net %d; dealer %v; seats %+v", a.profile.Net, a.profile.Balance, b.profile.Net, a.st.Dealer.Cards, a.st.Seats[:2])
	if a.profile.Balance != 1000+a.profile.Net {
		t.Fatalf("alice balance mismatch")
	}

	// leaderboard + avatars
	a.openLeaderboard()
	pump(t, apps, nil, func() bool { return len(a.leaderboard) == 2 }, "leaderboard")

	// log out and log back in with the password
	a.logout()
	a.fPass = "pass1234"
	a.doLogin()
	joinFirst(t, a)
	pump(t, apps, nil, func() bool { return a.screen == scrTable && a.st != nil }, "re-login")
	// next round starts for both
	pump(t, apps, nil, func() bool { return a.st.CanBet }, "second betting round")
}

func TestServerPassword(t *testing.T) {
	addr := startServerWith(t, 39518, func(c *server.Config) { c.ServerPassword = "hunter2" })
	try := func(user, sp string) *App {
		assets, _ := LoadAssets("")
		a := New(fakePlatform{}, &Config{Server: addr}, assets)
		a.fServer, a.fUser, a.fPass, a.fPass2, a.fServerPass = addr, user, "pass1234", "pass1234", sp
		a.loginMode = modeRegister
		a.doRegister()
		return a
	}
	none := try("nopass", "")
	pump(t, []*App{none}, nil, func() bool { return none.statusErr }, "refused without password")
	if !strings.Contains(none.status, "needs a server password") || !none.serverInfo.NeedsPassword {
		t.Fatalf("status %q", none.status)
	}
	wrong := try("wrongpass", "hunter3")
	pump(t, []*App{wrong}, nil, func() bool { return wrong.statusErr }, "refused with wrong password")
	if !strings.Contains(wrong.status, "Wrong server password") {
		t.Fatalf("status %q", wrong.status)
	}
	right := try("rightpass", "hunter2")
	pump(t, []*App{right}, nil, func() bool { return right.screen == scrLobby && right.lobbyKnown }, "account created")
}

func TestPokerEndToEnd(t *testing.T) {
	s := startServerS(t, 39519, func(c *server.Config) { c.CardRevealMinSeconds, c.CardRevealMaxSeconds = 0.2, 0.3 })
	pumpWait = 40 * time.Second
	defer func() { pumpWait = 15 * time.Second }()
	var out strings.Builder
	for _, kv := range []string{"between_seconds 1", "dale_think_ms 50", "action_seconds 5"} {
		s.Exec("table set holdem "+kv, &out)
	}
	a := newTestApp(t, s.Addr().String(), "alice")
	apps := []*App{a}
	pump(t, apps, nil, func() bool { return a.screen == scrLobby && a.lobbyKnown && len(a.lobby.Tables) == 3 }, "lobby")
	var holdem proto.TableInfo
	for _, ti := range a.lobby.Tables {
		if ti.Game == proto.GameHoldem {
			holdem = ti
		}
	}
	a.openBuyIn(holdem, false)
	if a.buyAmt != "1000" {
		t.Fatalf("buy-in starts at %q", a.buyAmt)
	}
	a.buyAmt = "500"
	pump(t, apps, map[*App]Input{a: {Keys: []Key{KeyEnter}}}, func() bool { return true }, "")
	pump(t, apps, nil, func() bool { return a.screen == scrPoker && a.pokerMe() != nil }, "seated at poker")
	if me := a.pokerMe(); me.Stack != 500 || a.ps.Balance != 500 {
		t.Fatalf("stack %d balance %d", me.Stack, a.ps.Balance)
	}
	// check or call every time it's our turn, for a few hands
	sawReveal := false
	play := func(until func() bool, what string) {
		pump(t, apps, nil, func() bool {
			sawReveal = sawReveal || a.pkRev != nil
			if a.ps.CanDealIn {
				a.Frame(Input{MX: -1, MY: -1, Chars: []rune{'d'}})
			}
			if len(a.ps.Actions) > 0 {
				a.Frame(Input{MX: -1, MY: -1, Chars: []rune{'c'}})
			}
			return until()
		}, what)
	}
	for h := 2; h <= 3; h++ {
		play(func() bool { return a.ps.HandNumber >= h }, fmt.Sprintf("hand %d", h))
	}
	play(func() bool { return a.ps.Phase == "showdown" }, "third showdown")
	if !sawReveal {
		t.Fatal("no card reveal at the poker table")
	}
	t.Logf("after %d hands: stack %d, dale %d, house %d", a.ps.HandNumber, a.pokerMe().Stack, a.ps.Seats[0].Stack, a.ps.HouseNet)
	stack, bank := a.pokerMe().Stack, a.ps.Balance
	a.leaveToLobby()
	pump(t, apps, nil, func() bool { return a.screen == scrLobby && a.ps == nil }, "back in the lobby")
	pump(t, apps, nil, func() bool { return a.profile.Balance == stack+bank }, "chips back in the bank")
	if a.profile.PokerHands < 3 {
		t.Fatalf("poker hands %d", a.profile.PokerHands)
	}
}

func TestBeggarEndToEnd(t *testing.T) {
	s := startServerS(t, 39520, func(c *server.Config) { c.RefillAmount = 20 })
	var out strings.Builder
	for _, kv := range []string{"odds 1", "roll_min_ms 600", "roll_max_ms 600"} {
		s.Exec("table set beggars "+kv, &out)
	}
	a := newTestApp(t, s.Addr().String(), "pauper")
	apps := []*App{a}
	pump(t, apps, nil, func() bool { return a.screen == scrLobby && a.lobbyKnown && len(a.lobby.Tables) == 3 }, "lobby")
	a.joinTable(a.lobby.Tables[2])
	pump(t, apps, nil, func() bool { return a.screen == scrBeggar && a.bs != nil && a.bs.CanRoll }, "at the Beggars Table")
	pump(t, apps, map[*App]Input{a: {Keys: []Key{KeySpace}}}, func() bool { return true }, "")
	pump(t, apps, nil, func() bool { return a.roll != nil }, "dice thrown")
	pump(t, apps, nil, func() bool { return a.roll.done && a.profile.Balance == 70 }, "five sixes paid")
	if a.profile.Net != 0 || a.profile.BeggarWins != 1 {
		t.Fatalf("net %d, wins %d", a.profile.Net, a.profile.BeggarWins)
	}
	for i, d := range a.roll.dice {
		if d.face != 6 {
			t.Fatalf("die %d shows %d", i, d.face)
		}
	}
	pump(t, apps, nil, func() bool { return a.bs != nil && !a.bs.CanRoll }, "too rich to beg")
}
