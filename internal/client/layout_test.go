// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func TestChooseSize(t *testing.T) {
	cases := []struct{ pw, ph, w, h int }{
		{1920, 1080, 640, 360}, // 16:9
		{2400, 1080, 800, 360}, // phone, sideways
		{1080, 2400, 360, 800}, // phone, upright
		{1920, 1200, 640, 400}, // 16:10
		{1536, 2048, 420, 560}, // tablet, upright
		{3440, 1440, 860, 360}, // ultrawide
		{5120, 1440, 900, 360}, // super-ultrawide: capped
		{1000, 1000, 640, 480}, // square: capped
	}
	for _, c := range cases {
		if w, h := ChooseSize(c.pw, c.ph); w != c.w || h != c.h {
			t.Errorf("%dx%d -> %dx%d, want %dx%d", c.pw, c.ph, w, h, c.w, c.h)
		}
	}
}

// Every screen draws at every canvas shape without panicking, and the seats
// stay on screen.
func TestLayoutsAtManySizes(t *testing.T) {
	a, _ := newRecApp(t)
	defer a.SetSize(640, 360)
	for _, sz := range [][2]int{{640, 360}, {900, 360}, {640, 480}, {360, 560}, {360, 820}, {450, 560}} {
		a.SetSize(sz[0], sz[1])
		a.screen = scrLogin
		a.Frame(Input{MX: -1, MY: -1})
		a.screen = scrTable
		st := baseState(10)
		st.Phase = proto.PhaseBetting
		st.CanBet = true
		st.ServerTime = time.Now().UnixMilli()
		a.onState(st)
		for _, ov := range []int{ovNone, ovLeaderboard, ovProfile, ovHelp, ovWardrobe, ovSettings, ovDealerLog} {
			a.overlay = ov
			fb := a.Frame(Input{MX: -1, MY: -1})
			if fb.Rect.Dx() != sz[0] || fb.Rect.Dy() != sz[1] {
				t.Fatalf("frame is %v, want %v", fb.Rect, sz)
			}
		}
		a.overlay = ovNone
		for i := 0; i < 10; i++ {
			x, y := a.seatPos(i, 10)
			if x < 16 || x > ScreenW-16 || y < 40 || y > a.L.hudTop {
				t.Errorf("%v: seat %d at %d,%d is off the table", sz, i, x, y)
			}
		}
	}
}
