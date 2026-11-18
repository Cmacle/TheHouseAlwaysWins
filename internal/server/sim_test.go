package server

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func testServer(t *testing.T, mut func(*Config)) *Server {
	cfg := DefaultConfig()
	cfg.ListenAddress = "127.0.0.1"
	cfg.Port = 1
	cfg.BettingTimeSeconds, cfg.TurnTimeSeconds, cfg.PlayTimeSeconds, cfg.ResultsTimeSeconds = 3, 3, 5, 1
	cfg.DealDelayMS, cfg.DealerDelayMS = 1, 1
	cfg.AllowSurrender = true
	cfg.CardRevealMinSeconds, cfg.CardRevealMaxSeconds = 0.01, 0.05
	if mut != nil {
		mut(cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = t.TempDir()
	lg := log.New(io.Discard, "", 0)
	if testing.Verbose() {
		lg = log.New(os.Stderr, "srv ", log.Ltime)
	}
	s, err := New(cfg, "", lg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Port = 0
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go s.Game.Run(stop)
	go s.Serve()
	t.Cleanup(func() { s.Close(); close(stop) })
	return s
}

type bot struct {
	name  string
	nc    net.Conn
	w     *bufio.Writer
	mu    sync.Mutex
	hands atomic.Int32
	errs  atomic.Int32
}

func (b *bot) send(tp string, v any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.w.Write(proto.Encode(tp, v))
	b.w.Flush()
}

func runBot(t *testing.T, addr, name string, stopAfter time.Duration, wg *sync.WaitGroup, quitEarly bool) *bot {
	nc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	b := &bot{name: name, nc: nc, w: bufio.NewWriter(nc)}
	b.send(proto.CHello, proto.Hello{Version: proto.Version})
	b.send(proto.CRegister, proto.Register{Username: name, Password: "secret"})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer nc.Close()
		rng := rand.New(rand.NewSource(int64(len(name)) * time.Now().UnixNano()))
		sc := bufio.NewScanner(nc)
		sc.Buffer(make([]byte, 64*1024), proto.MaxLine)
		deadline := time.Now().Add(stopAfter)
		if quitEarly {
			deadline = time.Now().Add(stopAfter / 3)
		}
		lastBetHand := -1
		for sc.Scan() {
			if time.Now().After(deadline) {
				return
			}
			var m proto.Msg
			json.Unmarshal(sc.Bytes(), &m)
			switch m.Type {
			case proto.SError:
				b.errs.Add(1)
			case proto.SEvent:
				var e proto.Event
				json.Unmarshal(m.Data, &e)
				if e.Kind == "win" || e.Kind == "lose" || e.Kind == "push" || e.Kind == "blackjack" {
					b.hands.Add(1)
				}
			case proto.SState:
				var st proto.TableState
				json.Unmarshal(m.Data, &st)
				if st.CanBet && lastBetHand != st.HandNumber {
					me := st.Seats[st.YourSeat]
					if me.PendingBet == 0 {
						lastBetHand = st.HandNumber
						max := st.Settings.MaxBet
						if me.Balance < max {
							max = me.Balance
						}
						amt := st.Settings.MinBet + rng.Int63n(max-st.Settings.MinBet+1)
						b.send(proto.CBet, proto.Bet{Amount: amt})
					}
				}
				if len(st.Actions) > 0 {
					a := st.Actions[rng.Intn(len(st.Actions))]
					b.send(proto.CAction, proto.Action{Action: a})
				}
			}
		}
	}()
	return b
}

func checkInvariants(t *testing.T, s *Server) {
	for _, u := range s.Store.Usernames() {
		s.Store.mu.Lock()
		a := s.Store.accounts[lower(u)]
		want := s.cfg().RefillAmount + a.Net + a.RefillTotal + a.AdminAdjust
		if a.Balance != want {
			t.Errorf("%s: balance %d != start+net+refills %d", u, a.Balance, want)
		}
		if a.Balance < 0 {
			t.Errorf("%s: negative balance", u)
		}
		if a.Won-a.Lost != a.Net+a.RefillTotal {
			t.Errorf("%s: won-lost %d != net+refill %d", u, a.Won-a.Lost, a.Net+a.RefillTotal)
		}
		s.Store.mu.Unlock()
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func simulate(t *testing.T, mut func(*Config), bots int, dur time.Duration) {
	s := testServer(t, mut)
	var wg sync.WaitGroup
	var list []*bot
	for i := 0; i < bots; i++ {
		list = append(list, runBot(t, s.Addr().String(), "bot_"+string(rune('a'+i)), dur, &wg, i%5 == 4))
		time.Sleep(20 * time.Millisecond)
	}
	wg.Wait()
	// let the table finish the hand in progress and go idle
	idle := false
	for i := 0; i < 100 && !idle; i++ {
		time.Sleep(100 * time.Millisecond)
		s.Game.DoWait(func() { idle = s.Game.phase == proto.PhaseWaiting && len(s.Game.conns) == 0 })
	}
	if !idle {
		t.Errorf("table did not go idle")
	}
	total := int32(0)
	for _, b := range list {
		total += b.hands.Load()
	}
	t.Logf("hands settled across bots: %d, hand number %d", total, s.Game.handNo)
	if s.Game.handNo < 2 || total == 0 {
		t.Errorf("too few hands played: %d hands, %d settled", s.Game.handNo, total)
	}
	checkInvariants(t, s)
	// the house's lifetime net mirrors the players' combined result
	var players int64
	for _, u := range s.Store.Usernames() {
		p, _ := s.Store.Profile(u)
		players += p.Net + p.RefillTotal
	}
	if h := s.Store.House(); h.Net != -players {
		t.Errorf("house net %d, players %d", h.Net, players)
	}
	t.Logf("house: %+v", s.Store.House())
}

func TestSimTurns(t *testing.T) {
	simulate(t, func(c *Config) { c.DealerVulgar = true }, 12, 8*time.Second)
}

func TestSimSimultaneous(t *testing.T) {
	simulate(t, func(c *Config) {
		c.PlayMode = "simultaneous"
		c.HideOtherHands = true
	}, 12, 8*time.Second)
}

func TestSimTurnBetting(t *testing.T) {
	simulate(t, func(c *Config) {
		c.BettingMode = "turns"
		c.DealerHitsSoft17 = true
		c.MaxPlayers = 4
	}, 5, 8*time.Second)
}

func TestRefill(t *testing.T) {
	st, _ := OpenStore(t.TempDir())
	st.Register("alice", "pass", nil, 1000, "2026-01-01")
	st.Settle("alice", []HandResult{{Wagered: 0, Payout: 0}})
	st.Debit("alice", 700)
	st.Settle("alice", []HandResult{{Wagered: 700, Payout: 0}})
	if d := st.ApplyRefill("alice", 1000, "2026-01-01"); d != 0 {
		t.Fatal("same period should not refill")
	}
	if d := st.ApplyRefill("alice", 1000, "2026-01-02"); d != 700 {
		t.Fatalf("refill %d", d)
	}
	p, _ := st.Profile("alice")
	if p.Balance != 1000 || p.Net != -1400 {
		t.Fatalf("balance %d net %d", p.Balance, p.Net)
	}
	if d := st.ApplyRefill("alice", 1000, "2026-01-02"); d != 0 {
		t.Fatal("double refill")
	}
}

func TestRefillPeriod(t *testing.T) {
	c := DefaultConfig()
	c.RefillTime = "06:30"
	c.Timezone = "America/Chicago"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/Chicago")
	if p := c.RefillPeriod(time.Date(2026, 3, 5, 6, 29, 0, 0, loc)); p != "2026-03-04" {
		t.Fatal(p)
	}
	if p := c.RefillPeriod(time.Date(2026, 3, 5, 6, 30, 0, 0, loc)); p != "2026-03-05" {
		t.Fatal(p)
	}
}

func TestLoginToken(t *testing.T) {
	st, _ := OpenStore(t.TempDir())
	_, tok, err := st.Register("Bob_1", "hunter2", nil, 500, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Login("bob_1", "wrong", ""); err == nil {
		t.Fatal("wrong password accepted")
	}
	_, tok2, err := st.Login("BOB_1", "", tok)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Login("bob_1", "", tok); err == nil {
		t.Fatal("rotated token reused")
	}
	if _, _, err := st.Login("bob_1", "", tok2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Register("bob_1", "x1234", nil, 1, "x"); err == nil {
		t.Fatal("dup username")
	}
}
