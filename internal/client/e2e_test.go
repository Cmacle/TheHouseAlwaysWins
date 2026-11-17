package client

import (
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
	go s.Game.Run(stop)
	go s.Serve()
	t.Cleanup(func() { s.Close(); close(stop) })
	return s.Addr().String()
}

func pump(t *testing.T, apps []*App, inputs map[*App]Input, until func() bool, what string) {
	deadline := time.Now().Add(15 * time.Second)
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

func TestEndToEnd(t *testing.T) {
	addr := startServer(t)
	a := newTestApp(t, addr, "alice")
	b := newTestApp(t, addr, "bob_99")
	apps := []*App{a, b}
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
	pump(t, []*App{right}, nil, func() bool { return right.screen == scrTable }, "account created")
}
