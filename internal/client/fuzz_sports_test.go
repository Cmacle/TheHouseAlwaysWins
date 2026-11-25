// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"math/rand"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Sportsbook screen survives random clicks, drags and scrolls with
// odd states (empty, broke, one market) at several screen sizes.
func TestFuzzSportsScreen(t *testing.T) {
	for _, size := range [][2]int{{640, 360}, {360, 640}, {800, 360}, {360, 760}} {
		t.Setenv("PREVIEW_DIR", t.TempDir())
		a, frame, _ := pokerPreviewApp(t)
		a.SetSize(size[0], size[1])
		a.onJoined(proto.Joined{ID: "sportsbook", Game: proto.GameSports, Name: "Sportsbook"})
		rng := rand.New(rand.NewSource(1))
		states := []*proto.SportsState{sampleSports(), {Table: "sportsbook", Name: "S"}, {Table: "sportsbook", MinBet: 10, MaxBet: 1000, Balance: 0}}
		one := sampleSports()
		one.Markets, one.Mine, one.Others = one.Markets[:1], one.Mine[:1], one.Others[:1]
		states = append(states, one)
		for i := 0; i < 3000; i++ {
			if i%200 == 0 {
				a.onSports(states[rng.Intn(len(states))])
				a.sbTab = rng.Intn(3)
			}
			a.in = Input{MX: rng.Intn(ScreenW), MY: rng.Intn(ScreenH), MouseInside: true, Wheel: rng.Intn(5) - 2}
			switch rng.Intn(4) {
			case 0:
				a.in.Pressed, a.in.Down = true, true
			case 1:
				a.in.Down = true
			case 2:
				a.in.Released = true
			}
			if rng.Intn(10) == 0 {
				a.sbSel = sbSelection{market: "m1", outcome: rng.Intn(2)}
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("size %v step %d tab %d: %v", size, i, a.sbTab, r)
					}
				}()
				frame()
			}()
		}
	}
}
