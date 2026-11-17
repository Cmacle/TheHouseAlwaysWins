package client

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

type fakePlatform struct{}

func (fakePlatform) PlaySound([]byte)              {}
func (fakePlatform) OpenImageFile() (string, bool) { return "", false }
func (fakePlatform) ClipboardText() string         { return "" }
func (fakePlatform) ToggleFullscreen()             {}
func (fakePlatform) Quit()                         {}

func card(r, s uint8) bj.Card { return bj.Card{Rank: r, Suit: s} }

func fakeAvatar(a *App, name string, c color.RGBA) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			cc := c
			if (x/4+y/4)%2 == 0 {
				cc = color.RGBA{c.R / 2, c.G / 2, c.B / 2, 255}
			}
			dx, dy := x-16, y-12
			if dx*dx+dy*dy < 40 {
				cc = color.RGBA{240, 200, 160, 255}
			}
			img.Set(x, y, cc)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	a.storeAvatar(proto.Avatar{Username: name, Version: "v1", Data: buf.Bytes()})
}

func save(t *testing.T, dir, name string, img *image.RGBA) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

func baseState(n int) *proto.TableState {
	st := &proto.TableState{
		Settings: proto.Settings{MaxSeats: n, MinBet: 10, MaxBet: 1000, BettingMode: "simultaneous", PlayMode: "turns",
			Decks: 6, BlackjackPays: "3:2", RefillAmount: 1000, ServerName: "The House Always Wins", Surrender: true},
		ActiveSeat: -1, YourSeat: 4, ServerTime: time.Now().UnixMilli(),
	}
	names := []string{"Cmacle", "", "LuckyLou", "Big_Tony", "You_There", "", "Deb", "card_shark", "", "Mo"}
	for i := 0; i < n; i++ {
		sv := proto.SeatView{Seat: i, ActiveHand: -1, Hands: []proto.HandView{}}
		if i < len(names) && names[i] != "" {
			sv.Username = names[i]
			sv.AvatarVersion = "v1"
			sv.Balance = int64(500 + i*137)
			sv.Connected = true
			sv.Status = "waiting"
		}
		st.Seats = append(st.Seats, sv)
	}
	looks := [][3]string{{"crown", "frame_gold", "t_owns_house"}, {}, {"party_hat", "frame_felt", "t_regular"}, {"wizard", "frame_rainbow", "t_high_roller"},
		{"cowboy", "frame_silver", "t_lucky"}, {}, {"tinfoil", "frame_bone", "t_down_bad"}, {"laurel", "frame_diamond", "t_goat"}, {}, {"flame", "frame_fire", "t_on_fire"}}
	for i := range st.Seats {
		if i < len(looks) && st.Seats[i].Username != "" {
			st.Seats[i].Hat, st.Seats[i].Frame, st.Seats[i].Title = looks[i][0], looks[i][1], looks[i][2]
		}
	}
	return st
}

func TestPreview(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Server: "play.example.com:7777", Username: "You_There", RememberMe: true, Sound: true}
	a := New(fakePlatform{}, cfg, assets)
	a.reconnectAt = time.Time{}
	frame := func() *image.RGBA { return a.Frame(Input{MX: -1, MY: -1}) }

	a.setStatus("Unknown username or wrong password", true)
	save(t, dir, "01_login.png", frame())
	a.loginMode = modeRegister
	a.fPass, a.fPass2 = "abcd", "abcd"
	a.setStatus("", false)
	save(t, dir, "02_register.png", frame())

	// table
	a.screen = scrTable
	a.username = "You_There"
	a.profile = proto.Profile{Username: "You_There", Balance: 1048, Net: 348, AvatarVersion: "v1",
		Wagered: 31200, PeakBalance: 2400, BestWinStreak: 5, WorstLoseStreak: 4, Busts: 12, Hat: "cowboy", EquippedHat: "cowboy", Frame: "frame_silver", Title: "t_lucky",
		Unlocked: []string{"party_hat", "ball_cap", "beanie", "cowboy", "sombrero", "frame_felt", "frame_silver", "t_regular", "t_lucky", "t_big_fish"}, HandsPlayed: 42, HandsWon: 20, HandsLost: 18, HandsPushed: 4, Blackjacks: 3, BiggestWin: 450, Won: 2100, Lost: 1752, RefillTotal: 0}
	cols := []color.RGBA{{200, 60, 60, 255}, {60, 120, 220, 255}, {60, 180, 90, 255}, {200, 160, 40, 255}, {150, 70, 200, 255}, {40, 180, 180, 255}, {220, 110, 40, 255}}
	for i, n := range []string{"Cmacle", "LuckyLou", "Big_Tony", "You_There", "Deb", "card_shark", "Mo"} {
		fakeAvatar(a, n, cols[i])
	}

	st := baseState(10)
	st.Phase = proto.PhaseBetting
	st.Message = "Place your bets!"
	st.Deadline = time.Now().UnixMilli() + 9000
	st.TimerTotal = 15000
	st.CanBet = true
	st.Seats[0].Status, st.Seats[0].PendingBet = "bet_placed", 100
	st.Seats[2].Status = "betting"
	st.Seats[3].Status, st.Seats[3].PendingBet = "bet_placed", 500
	st.Seats[4].Status = "betting"
	st.Seats[6].Status = "sitting_out"
	st.Seats[7].Status, st.Seats[7].PendingBet = "bet_placed", 25
	st.Seats[9].Status, st.Seats[9].Connected = "away", false
	st.DealerSay = "LuckyLou bets the minimum. Big spender."
	st.DealerSayID = 2
	st.DealerSayUntil = time.Now().Add(time.Hour).UnixMilli()
	a.st = nil
	a.onState(st)
	a.betDraft = 150
	a.lastBet = 100
	save(t, dir, "03_betting.png", frame())

	// playing
	st = baseState(10)
	st.Phase = proto.PhasePlaying
	st.HandNumber = 37
	st.Message = "You_There's turn"
	st.ActiveSeat = 4
	st.Deadline = time.Now().UnixMilli() + 12000
	st.TimerTotal = 20000
	st.Dealer = proto.DealerView{Cards: []bj.Card{card(10, 1), {}}, Total: 10}
	hv := func(bet int64, cards ...bj.Card) proto.HandView {
		t, s := bj.Total(cards)
		return proto.HandView{Cards: cards, Bet: bet, Total: t, Soft: s, Done: false, Natural: bj.IsBlackjack(cards)}
	}
	st.Seats[0].Hands = []proto.HandView{hv(100, card(10, 0), card(7, 2))}
	st.Seats[0].Hands[0].Done = true
	st.Seats[2].Hands = []proto.HandView{hv(50, card(1, 3), card(13, 1))}
	st.Seats[3].Hands = []proto.HandView{hv(500, card(9, 0), card(5, 1), card(10, 2))}
	st.Seats[4].Hands = []proto.HandView{hv(100, card(8, 0), card(3, 2)), hv(100, card(8, 1), card(1, 3))}
	st.Seats[4].Hands[0].Bet = 200
	st.Seats[4].Hands[0].Doubled = true
	st.Seats[4].Hands[0].Cards = append(st.Seats[4].Hands[0].Cards, card(10, 3))
	st.Seats[4].Hands[0].Total = 21
	st.Seats[4].ActiveHand = 1
	st.Seats[7].Hands = []proto.HandView{{Cards: []bj.Card{{}, {}}, Bet: 25}}
	st.Actions = []string{"hit", "stand", "double"}
	st.Settings.DealerVulgar = true
	st.DealerSay = "Standing on 13, Big_Tony? Grow a pair. Hit the goddamn card, you spineless chickenshit."
	st.DealerSayID = 1
	st.DealerSayUntil = time.Now().Add(time.Hour).UnixMilli()
	st.HouseNet = 48210
	for _, i := range []int{0, 2, 3, 4, 7} {
		st.Seats[i].Status = "playing"
	}
	a.onState(st)
	settle := func() {
		for k := range a.cardSeen {
			a.cardSeen[k] = time.Now().Add(-time.Minute)
		}
	}
	settle()
	save(t, dir, "04_playing.png", frame())

	// results
	st2 := *st
	st2.Phase = proto.PhaseResults
	st2.Message = "Dealer busts with 24!"
	st2.ActiveSeat = -1
	st2.Actions = nil
	st2.Dealer = proto.DealerView{Cards: []bj.Card{card(10, 1), card(4, 0), card(10, 2)}, Total: 24}
	st2.Seats = append([]proto.SeatView(nil), st.Seats...)
	res := []struct {
		seat int
		r    []string
		pay  []int64
	}{{0, []string{"win"}, []int64{200}}, {2, []string{"blackjack"}, []int64{125}}, {3, []string{"bust"}, []int64{0}}, {4, []string{"win", "win"}, []int64{400, 200}}, {7, []string{"win"}, []int64{50}}}
	for _, r := range res {
		sv := st2.Seats[r.seat]
		hands := append([]proto.HandView(nil), sv.Hands...)
		for k := range hands {
			hands[k].Result = r.r[k]
			hands[k].Payout = r.pay[k]
			hands[k].Done = true
		}
		if r.seat == 7 {
			hands[0].Cards = []bj.Card{card(6, 2), card(12, 0)}
			hands[0].Total = 16
		}
		sv.Hands = hands
		sv.ActiveHand = -1
		st2.Seats[r.seat] = sv
	}
	st2.Deadline = time.Now().UnixMilli() + 3000
	st2.TimerTotal = 5000
	a.onState(&st2)
	settle()
	a.flipAt = map[string]time.Time{}
	a.toast("YOU WIN +500", a.A.C("win"), 3, 2)
	save(t, dir, "05_results.png", frame())
	a.toasts = nil

	// overlays
	a.leaderboard = nil
	names := []string{"Cmacle", "LuckyLou", "Big_Tony", "You_There", "Deb", "card_shark", "Mo", "whale_watcher", "tiltproof", "dealer_hater", "Ace_Ventura", "zed"}
	for i, n := range names {
		hats := []string{"crown", "wizard", "party_hat", "cowboy", "", "tinfoil", "flame", "", "viking", "dunce", "halo", ""}
		titles := []string{"t_owns_house", "t_high_roller", "t_regular", "t_lucky", "t_rookie", "t_down_bad", "t_on_fire", "t_rookie", "t_shark", "t_rock_bottom", "t_legend", "t_rookie"}
		a.leaderboard = append(a.leaderboard, proto.Profile{Username: n, Net: int64(4200 - i*900), Balance: int64(9000 - i*633), HandsPlayed: 300 - i*20, AvatarVersion: "v1", Online: i%3 == 0, Hat: hats[i], Title: titles[i]})
	}
	a.overlay = ovLeaderboard
	a.lbSort = "balance"
	a.lbAt = time.Now()
	save(t, dir, "06_leaderboard.png", frame())
	a.house = proto.HouseStats{HandsDealt: 1843, PlayerHands: 7210, Net: 48210, Won: 212400, Lost: 164190, RoundsWon: 1011, RoundsLost: 702, DealerBlackjacks: 88, DealerBusts: 512, PlayerBusts: 1290, BiggestWin: 6200, BiggestLoss: 4800}
	a.lbSort = "house"
	save(t, dir, "06b_house.png", frame())
	a.lbSort = "balance"
	a.overlay = ovProfile
	save(t, dir, "07_profile.png", frame())
	a.overlay = ovHelp
	save(t, dir, "08_help.png", frame())
	a.overlay = ovWardrobe
	a.wardSel = "tiara"
	save(t, dir, "09_wardrobe_hats.png", frame())
	a.wardTab = "frame"
	a.wardSel = "frame_gold"
	save(t, dir, "10_wardrobe_frames.png", frame())
	a.wardTab = "title"
	a.wardSel = "t_comeback"
	save(t, dir, "11_wardrobe_titles.png", frame())
	fmt.Println("previews written to", dir)
}
