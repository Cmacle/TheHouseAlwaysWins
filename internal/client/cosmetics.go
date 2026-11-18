// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func (a *App) hatSprite(id string) *image.RGBA {
	it, ok := cosmetics.Get(id)
	if !ok || it.Kind != cosmetics.Hat {
		return nil
	}
	w, h := a.A.Theme.HatWidth, a.A.Theme.HatHeight
	cols := a.A.Hats.Rect.Dx() / w
	if cols == 0 {
		return nil
	}
	x, y := (it.Sprite%cols)*w, (it.Sprite/cols)*h
	r := image.Rect(x, y, x+w, y+h)
	if !r.In(a.A.Hats.Rect) {
		return nil
	}
	return a.A.Hats.SubImage(r).(*image.RGBA)
}

func (a *App) frameSprite(id string) *image.RGBA {
	it, ok := cosmetics.Get(id)
	if !ok || it.Kind != cosmetics.Frame {
		it, _ = cosmetics.Get("frame_wood")
	}
	s := a.frameCell()
	r := image.Rect(it.Sprite*s, 0, it.Sprite*s+s, s)
	if !r.In(a.A.Frames.Rect) {
		return nil
	}
	return a.A.Frames.SubImage(r).(*image.RGBA)
}

func hsv(h float64) color.RGBA {
	h = math.Mod(h, 1)
	if h < 0 {
		h++
	}
	h *= 6
	i := int(h)
	f := h - float64(i)
	q, t := uint8(255*(1-f)), uint8(255*f)
	switch i {
	case 0:
		return color.RGBA{255, t, 0, 255}
	case 1:
		return color.RGBA{q, 255, 0, 255}
	case 2:
		return color.RGBA{0, 255, t, 255}
	case 3:
		return color.RGBA{0, q, 255, 255}
	case 4:
		return color.RGBA{t, 0, 255, 255}
	}
	return color.RGBA{255, 0, q, 255}
}

// frameCell is the size of one frame in frames.png (square cells in a row).
func (a *App) frameCell() int {
	if a.A.Frames != nil && a.A.Frames.Rect.Dy() > 0 {
		return a.A.Frames.Rect.Dy()
	}
	return a.A.Theme.FrameSize
}

// drawFrame draws an avatar frame around the 32x32 box at (x, y). The rarest
// frames move: flames flicker, gems sparkle, the rainbow runs round and a
// gleam sweeps over the finish line and the vault.
func (a *App) drawFrame(id string, x, y int) {
	s := a.frameCell()
	pad := (s - 32) / 2
	spr := a.frameSprite(id)
	if spr == nil {
		strokeRect(a.fb, image.Rect(x-2, y-2, x+34, y+34), a.A.C("shadow"))
		return
	}
	var anim func(xx, yy int, c color.RGBA) (color.RGBA, bool)
	t := float64(a.frame)
	switch id {
	case "frame_rainbow":
		off := t / 90
		anim = func(xx, yy int, c color.RGBA) (color.RGBA, bool) {
			ang := (math.Atan2(float64(yy)-float64(s)/2, float64(xx)-float64(s)/2) + math.Pi) / (2 * math.Pi)
			return hsv(ang + off), true
		}
	case "frame_fire":
		anim = func(xx, yy int, c color.RGBA) (color.RGBA, bool) {
			d := max(pad-xx, xx-(pad+31), pad-yy, yy-(pad+31), 0)
			if d <= 3 {
				return c, true
			}
			// flame tips come and go
			n := math.Sin(float64(xx)*1.7+t*0.35) + math.Sin(float64(yy)*1.3-t*0.27)
			if n < -0.9+float64(d-3)*0.35 {
				return c, false
			}
			return c, true
		}
	case "frame_bone":
		// a ghostly glow comes and goes over the bone
		k := math.Max(0, math.Sin(t/30))
		anim = func(xx, yy int, c color.RGBA) (color.RGBA, bool) {
			if c.R > 200 && c.G > 190 {
				return color.RGBA{uint8(float64(c.R) * (1 - 0.25*k)), c.G, uint8(float64(c.B) * (1 - 0.1*k)), 255}, true
			}
			return c, true
		}
	case "frame_velvet":
		anim = func(xx, yy int, c color.RGBA) (color.RGBA, bool) {
			if c.R > 200 && c.G > 150 && c.B < 120 { // the brass
				return gleam(t, 60, xx, yy, c), true
			}
			return c, true
		}
	case "frame_diamond", "frame_marathon", "frame_vault":
		period := 140.0
		if id == "frame_diamond" {
			period = 90
		}
		anim = func(xx, yy int, c color.RGBA) (color.RGBA, bool) {
			g := math.Mod(float64(xx+yy)-t*1.2, period)
			if g < 0 {
				g += period
			}
			if g < 3 {
				return color.RGBA{uint8((int(c.R) + 255*2) / 3), uint8((int(c.G) + 255*2) / 3), uint8((int(c.B) + 255*2) / 3), 255}, true
			}
			if id == "frame_diamond" && (xx*7+yy*13+int(t/8))%97 == 0 {
				return color.RGBA{255, 255, 255, 255}, true // a sparkle
			}
			return c, true
		}
	}
	if anim == nil {
		blit(a.fb, spr, x-pad, y-pad)
		return
	}
	for yy := 0; yy < s; yy++ {
		for xx := 0; xx < s; xx++ {
			o := spr.PixOffset(spr.Rect.Min.X+xx, spr.Rect.Min.Y+yy)
			if spr.Pix[o+3] == 0 {
				continue
			}
			c, ok := anim(xx, yy, color.RGBA{spr.Pix[o], spr.Pix[o+1], spr.Pix[o+2], spr.Pix[o+3]})
			if !ok {
				continue
			}
			px, py := x-pad+xx, y-pad+yy
			if image.Pt(px, py).In(a.fb.Rect) {
				q := a.fb.PixOffset(px, py)
				a.fb.Pix[q], a.fb.Pix[q+1], a.fb.Pix[q+2], a.fb.Pix[q+3] = c.R, c.G, c.B, 255
			}
		}
	}
}

// drawHat draws a hat on a 32x32 avatar whose top-left is (x, y).
func (a *App) drawHat(id string, x, y int) {
	spr := a.hatSprite(id)
	if spr == nil {
		return
	}
	hy := y - a.A.Theme.HatHeight + a.A.Theme.HatOverlap
	hx := x + 16 - a.A.Theme.HatWidth/2
	a.blitHat(id, spr, hx, hy)
}

// hatMotion says how an animated hat moves this frame: each row's sideways
// shift, a bob up or down, and a recolour per pixel (nil to keep it).
type hatMotion struct {
	sway   func(row, rows int) int
	bob    int
	colour func(x, y int, c color.RGBA) color.RGBA
	sparks [][2]int // twinkle points
}

func gleam(t float64, period float64, x, y int, c color.RGBA) color.RGBA {
	g := math.Mod(float64(x+y)-t*0.9, period)
	if g < 0 {
		g += period
	}
	if g < 3 {
		return color.RGBA{uint8((int(c.R) + 255*2) / 3), uint8((int(c.G) + 255*2) / 3), uint8((int(c.B) + 255*2) / 3), 255}
	}
	return c
}

// hatAnim is the motion of the best hats (nil for the rest).
func (a *App) hatAnim(id string) *hatMotion {
	t := float64(a.frame)
	switch id {
	case "crown":
		return &hatMotion{colour: func(x, y int, c color.RGBA) color.RGBA { return gleam(t, 70, x, y, c) },
			sparks: [][2]int{{8, 9}, {16, 6}, {24, 9}, {12, 14}, {20, 14}}}
	case "gold_top_hat", "laurel", "pharaoh":
		return &hatMotion{colour: func(x, y int, c color.RGBA) color.RGBA { return gleam(t, 80, x, y, c) }}
	case "tiara":
		return &hatMotion{colour: func(x, y int, c color.RGBA) color.RGBA { return gleam(t, 60, x, y, c) },
			sparks: [][2]int{{10, 12}, {16, 8}, {22, 12}, {13, 15}, {19, 15}}}
	case "turban":
		return &hatMotion{sparks: [][2]int{{16, 11}, {15, 2}}}
	case "moneybag":
		return &hatMotion{bob: int(math.Round(math.Sin(t/18) * 0.6)),
			colour: func(x, y int, c color.RGBA) color.RGBA { return gleam(t, 90, x, y, c) },
			sparks: [][2]int{{6, 18}, {26, 18}, {14, 3}}}
	case "flame":
		// the fire licks: rows shiver and the colours flicker
		return &hatMotion{
			sway: func(row, rows int) int {
				if row > rows*2/3 {
					return 0
				}
				return int(math.Round(math.Sin(t/4+float64(row)*0.9) * float64(rows-row) / float64(rows) * 1.6))
			},
			colour: func(x, y int, c color.RGBA) color.RGBA {
				if c.R > 200 && c.G > 60 && (x*7+y*3+int(t/3))%5 == 0 {
					return color.RGBA{255, 246, 200, 255}
				}
				return c
			}}
	case "space":
		return &hatMotion{colour: func(x, y int, c color.RGBA) color.RGBA { return gleam(t, 50, x, -y, c) }}
	case "chip_tower":
		// the stack wobbles, more at the top
		return &hatMotion{sway: func(row, rows int) int {
			return int(math.Round(math.Sin(t/14) * float64(rows-row) / float64(rows) * 2))
		}}
	case "nightcap":
		// the long tip sways
		return &hatMotion{sway: func(row, rows int) int {
			if row > rows/2 {
				return 0
			}
			return int(math.Round(math.Sin(t/20) * float64(rows/2-row) / float64(rows/2) * 2))
		}}
	case "halo":
		// a slow glow and a gentle float
		k := 0.5 + 0.5*math.Sin(t/16)
		return &hatMotion{bob: int(math.Round(math.Sin(t/16) * 1)), colour: func(x, y int, c color.RGBA) color.RGBA {
			if c.R < 60 && c.G < 60 {
				return c // the outline stays
			}
			m := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*k*0.6) }
			return color.RGBA{m(c.R), m(c.G), m(c.B), 255}
		}}
	case "wizard":
		return &hatMotion{sparks: [][2]int{{12, 8}, {18, 12}, {15, 4}, {20, 16}}}
	case "unicorn":
		// the horn shimmers through the colours
		return &hatMotion{colour: func(x, y int, c color.RGBA) color.RGBA {
			if c.R > 200 && c.G > 200 && c.B > 200 {
				hc := hsv(float64(y)/20 + t/120)
				return color.RGBA{uint8((int(hc.R) + int(c.R)) / 2), uint8((int(hc.G) + int(c.G)) / 2), uint8((int(hc.B) + int(c.B)) / 2), 255}
			}
			return c
		}}
	case "shark_fin":
		// swimming: up and down, side to side
		return &hatMotion{bob: int(math.Round(math.Sin(t/12) * 1.2)), sway: func(row, rows int) int {
			return int(math.Round(math.Sin(t/24) * 1.5))
		}}
	case "fascinator":
		return &hatMotion{sway: func(row, rows int) int {
			if row > rows/2 {
				return 0
			}
			return int(math.Round(math.Sin(t/10) * float64(rows/2-row) / float64(rows/2) * 1.5))
		}}
	case "devil":
		return &hatMotion{colour: func(x, y int, c color.RGBA) color.RGBA {
			if c.R > 150 && c.G < 80 {
				k := 0.5 + 0.5*math.Sin(t/10)
				return color.RGBA{c.R, uint8(float64(c.G) + 60*k), uint8(float64(c.B) + 30*k), 255}
			}
			return c
		}}
	}
	return nil
}

// blitHat draws a hat sprite at (x, y), animated if it's one of the best.
func (a *App) blitHat(id string, spr *image.RGBA, x, y int) {
	m := a.hatAnim(id)
	if m == nil {
		blit(a.fb, spr, x, y)
		return
	}
	w, h := spr.Rect.Dx(), spr.Rect.Dy()
	// the first row with anything in it, so sway counts from the hat's top
	top := h
	for yy := 0; yy < h && top == h; yy++ {
		for xx := 0; xx < w; xx++ {
			if spr.Pix[spr.PixOffset(spr.Rect.Min.X+xx, spr.Rect.Min.Y+yy)+3] != 0 {
				top = yy
				break
			}
		}
	}
	rows := max(1, h-top)
	for yy := 0; yy < h; yy++ {
		dx := 0
		if m.sway != nil && yy >= top {
			dx = m.sway(yy-top, rows)
		}
		for xx := 0; xx < w; xx++ {
			o := spr.PixOffset(spr.Rect.Min.X+xx, spr.Rect.Min.Y+yy)
			if spr.Pix[o+3] == 0 {
				continue
			}
			c := color.RGBA{spr.Pix[o], spr.Pix[o+1], spr.Pix[o+2], 255}
			if m.colour != nil {
				c = m.colour(xx, yy, c)
			}
			px, py := x+xx+dx, y+yy+m.bob
			if image.Pt(px, py).In(a.fb.Rect) {
				q := a.fb.PixOffset(px, py)
				a.fb.Pix[q], a.fb.Pix[q+1], a.fb.Pix[q+2], a.fb.Pix[q+3] = c.R, c.G, c.B, 255
			}
		}
	}
	for i, p := range m.sparks {
		ph := (a.frame/8 + i*3) % 10
		if ph >= 2 {
			continue
		}
		px, py := x+p[0], y+p[1]+m.bob
		c := color.RGBA{255, 255, 255, 255}
		fillRect(a.fb, image.Rect(px, py, px+1, py+1), c)
		if ph == 0 {
			fillRect(a.fb, image.Rect(px-1, py, px+2, py+1), c)
			fillRect(a.fb, image.Rect(px, py-1, px+1, py+2), c)
		}
	}
}

// drawAvatarFull draws frame, picture and hat at 32x32.
func (a *App) drawAvatarFull(user, ver, hat, frame string, x, y int, dim bool) {
	a.drawFrame(frame, x, y)
	blit(a.fb, a.avatarImg(user, ver, 32), x, y)
	if dim {
		fillRect(a.fb, image.Rect(x, y, x+32, y+32), color.RGBA{0, 0, 0, 140})
	}
	a.drawHat(hat, x, y)
}

// smallHat returns a hat scaled to half size (for the leaderboard).
func (a *App) smallHat(id string) *image.RGBA {
	if a.smallHats == nil {
		a.smallHats = map[string]*image.RGBA{}
	}
	if img, ok := a.smallHats[id]; ok {
		return img
	}
	spr := a.hatSprite(id)
	var img *image.RGBA
	if spr != nil {
		img = scaleImage(toRGBA(spr), spr.Rect.Dx()/2, spr.Rect.Dy()/2)
	}
	a.smallHats[id] = img
	return img
}

func tierColor(t int) color.RGBA {
	return parseHex(cosmetics.TierColors[t])
}

func titleOf(id string) (string, color.RGBA, bool) {
	it, ok := cosmetics.Get(id)
	if !ok {
		return "", color.RGBA{}, false
	}
	return it.Name, tierColor(it.Tier), true
}

func statsOf(p proto.Profile) (cosmetics.Stats, map[string]bool) {
	own := map[string]bool{}
	for _, id := range p.Unlocked {
		own[id] = true
	}
	return cosmetics.Stats{
		Wagered: p.Wagered, Peak: p.PeakBalance, Net: p.Net, LowestNet: p.LowestNet, BiggestWin: p.BiggestWin,
		Hands: p.HandsPlayed, Blackjacks: p.Blackjacks, BestWinStreak: p.BestWinStreak, WorstLoseStreak: p.WorstLoseStreak,
		Busts: p.Busts, DoublesWon: p.DoublesWon, Splits: p.Splits, Refills: p.RefillCount,
		RankRich: p.RankRich, RankNet: p.RankNet, RankHands: p.RankHands, RankBet: p.RankBet, RankPeak: p.RankPeak,
		PkHands: p.PokerHands, PkFlush: p.PokerFlushes, PkFullHouse: p.PokerFullHouses, PkQuads: p.PokerQuads,
		PkStrFlush: p.PokerStrFlushes, PkRoyal: p.PokerRoyals, PkBluffs: p.PokerBluffs, PkBeatDale: p.PokerBeatDale,
		PkBigPot:  p.PokerBiggestPot,
		BacRounds: p.BacRounds, BacNaturals: p.BacNaturals, BacTies: p.BacTies,
		RdWins: p.RdWins, RdGutshots: p.RdGutshots, RdTrips: p.RdTrips,
	}, own
}

func silhouette(dst, src *image.RGBA, x, y int, c color.RGBA) {
	r := image.Rect(x, y, x+src.Rect.Dx(), y+src.Rect.Dy())
	draw.DrawMask(dst, r, &image.Uniform{c}, image.Point{}, src, src.Rect.Min, draw.Over)
}

// ---------------------------------------------------------------- wardrobe

var wardTabs = []struct{ kind, label string }{{cosmetics.Hat, "HATS"}, {cosmetics.Frame, "FRAMES"}, {cosmetics.Title, "TITLES"}}

func (a *App) drawWardrobe() {
	r := ovRect(560, 332)
	narrow := r.Dx() < 500
	if narrow {
		r = ovRect(560, ScreenH-40)
	}
	if a.overlayFrame(r, "WARDROBE") {
		return
	}
	if a.now.Sub(a.recordsAt) > 5*time.Second {
		a.recordsAt = a.now
		a.send(proto.CRecords, nil)
	}
	if a.wardTab == "" {
		a.wardTab = cosmetics.Hat
	}
	st, own := statsOf(a.profile)
	total, have := 0, 0
	for i := range cosmetics.Catalog {
		it := &cosmetics.Catalog[i]
		if it.Dynamic() {
			continue
		}
		total++
		if cosmetics.Has(it.ID, st, own) {
			have++
		}
	}
	a.text(r.Min.X+16, r.Min.Y+14, commas(int64(have))+"/"+commas(int64(total))+" unlocked", a.A.C("text_dim"), alignLeft)

	tx := r.Min.X + 16
	for _, t := range wardTabs {
		w := a.A.Font.Width(t.label) + 20
		style := styleRed
		if a.wardTab == t.kind {
			style = styleGold
		}
		if a.button("wt_"+t.kind, image.Rect(tx, r.Min.Y+30, tx+w, r.Min.Y+46), t.label, btn{enabled: true, style: style}) {
			a.wardTab = t.kind
			a.wardSel = ""
		}
		tx += w + 4
	}
	// your look
	a.text(r.Max.X-60, r.Min.Y+34, "YOU", a.A.C("text_dim"), alignCenter)
	a.drawAvatarFull(a.username, a.profile.AvatarVersion, a.profile.Hat, a.profile.Frame, r.Max.X-76, r.Min.Y+54, false)
	if name, c, ok := titleOf(a.profile.Title); ok {
		a.text(r.Max.X-60, r.Min.Y+92, fitText(a.A.Font, name, 100), c, alignCenter)
	}

	items := cosmetics.OfKind(a.wardTab)
	gx, gy := r.Min.X+16, r.Min.Y+54
	gridW := r.Dx() - 32 - 100 // the "you" preview sits to the right
	if narrow {
		gy, gridW = r.Min.Y+104, r.Dx()-32 // ...or above
	}
	hover := ""
	equipped := map[string]string{cosmetics.Hat: a.profile.EquippedHat, cosmetics.Frame: a.profile.Frame, cosmetics.Title: a.profile.Title}[a.wardTab]
	click := func(it *cosmetics.Item, unlocked bool) {
		a.wardSel = it.ID
		if !unlocked {
			a.sound("error")
			return
		}
		id := it.ID
		if a.wardTab == cosmetics.Hat && equipped == id {
			id = "" // take it off
		}
		a.send(proto.CEquip, proto.Equip{Kind: a.wardTab, ID: id})
		a.sound("chip")
	}
	switch a.wardTab {
	case cosmetics.Hat, cosmetics.Frame:
		cw, ch := 54, 46
		if a.wardTab == cosmetics.Frame {
			cw, ch = 54, 50
		}
		cols := max(1, min(8, (gridW+2)/(cw+2)))
		if !narrow {
			cols = 8
		}
		// shrink the cells until every item fits above the info box
		infoTop := r.Max.Y - 66
		if narrow {
			infoTop = r.Max.Y - 92
		}
		rowsFit := max(1, (infoTop-4-gy)/(ch+2))
		for (len(items)+cols-1)/cols > rowsFit && cw > 36 {
			cw -= 2
			cols = max(1, (gridW+2)/(cw+2))
			if !narrow {
				cols = min(cols, 12)
			}
		}
		for (len(items)+cols-1)/cols > rowsFit && ch > 38 {
			ch -= 2
			rowsFit = max(1, (infoTop-4-gy)/(ch+2))
		}
		for i, it := range items {
			cx, cy := gx+(i%cols)*(cw+2), gy+(i/cols)*(ch+2)
			cell := image.Rect(cx, cy, cx+cw, cy+ch)
			unlocked := cosmetics.Has(it.ID, st, own)
			fillRect(a.fb, cell, color.RGBA{0, 0, 0, 120})
			bc := tierColor(it.Tier)
			if !unlocked {
				bc = color.RGBA{bc.R / 3, bc.G / 3, bc.B / 3, 255}
			}
			strokeRect(a.fb, cell, bc)
			isEq := it.ID == equipped || (a.wardTab == cosmetics.Hat && it.ID == a.profile.Hat)
			if isEq {
				strokeRect(a.fb, cell.Inset(-1), a.A.C("highlight"))
				strokeRect(a.fb, cell.Inset(1), a.A.C("highlight"))
			}
			if a.wardTab == cosmetics.Hat {
				if spr := a.hatSprite(it.ID); spr != nil {
					hx, hy := cx+(cw-spr.Rect.Dx())/2, cy+(ch-spr.Rect.Dy())/2-2
					if unlocked {
						a.blitHat(it.ID, spr, hx, hy)
					} else {
						silhouette(a.fb, spr, hx, hy, color.RGBA{70, 62, 58, 255})
					}
				}
			} else {
				ax, ay := cx+(cw-32)/2, cy+(ch-32)/2
				if unlocked {
					a.drawFrame(it.ID, ax, ay)
					blit(a.fb, a.avatarImg(a.username, a.profile.AvatarVersion, 32), ax, ay)
				} else if spr := a.frameSprite(it.ID); spr != nil {
					pad := (a.frameCell() - 32) / 2
					silhouette(a.fb, spr, ax-pad, ay-pad, color.RGBA{70, 62, 58, 255})
				}
			}
			if !unlocked {
				a.text(cx+cw-6, cy+ch-11, "?", a.A.C("text_dim"), alignCenter)
			}
			if a.hovered(cell) {
				hover = it.ID
				strokeRect(a.fb, cell, a.A.C("text"))
				if a.in.Pressed {
					click(it, unlocked)
				}
			}
		}
	case cosmetics.Title:
		cw, rh := 140, 15
		cols := 3
		if narrow {
			cols = max(1, (gridW+4)/(cw+4))
			cw = (gridW+4)/cols - 4
		}
		perCol := (len(items) + cols - 1) / cols
		for i, it := range items {
			cx, cy := gx+(i/perCol)*(cw+4), gy+(i%perCol)*(rh+2)
			row := image.Rect(cx, cy, cx+cw, cy+rh)
			unlocked := cosmetics.Has(it.ID, st, own)
			fillRect(a.fb, row, color.RGBA{0, 0, 0, 110})
			c := tierColor(it.Tier)
			if !unlocked {
				c = a.A.C("text_dim")
			}
			if it.ID == equipped {
				strokeRect(a.fb, row, a.A.C("highlight"))
			}
			label := it.Name
			if !unlocked {
				label = "? " + label
			}
			a.text(cx+5, cy+4, fitText(a.A.Font, label, cw-10), c, alignLeft)
			if a.hovered(row) {
				hover = it.ID
				strokeRect(a.fb, row, a.A.C("text"))
				if a.in.Pressed {
					click(it, unlocked)
				}
			}
		}
	}

	// info box
	show := hover
	if show == "" {
		show = a.wardSel
	}
	ib := image.Rect(r.Min.X+16, r.Max.Y-66, r.Max.X-16, r.Max.Y-14)
	if narrow {
		ib.Min.Y = r.Max.Y - 92
	}
	fillRect(a.fb, ib, color.RGBA{0, 0, 0, 140})
	strokeRect(a.fb, ib, color.RGBA{0, 0, 0, 255})
	it, ok := cosmetics.Get(show)
	if !ok {
		hint := "Hover an item to see how to unlock it. Click to wear it."
		if a.isTouch() {
			hint = "Tap an item to see how to unlock it, or to wear it."
		}
		for i, l := range a.wrap(hint, ib.Dx()-16) {
			a.text(ib.Min.X+ib.Dx()/2, ib.Min.Y+16+i*10, l, a.A.C("text_dim"), alignCenter)
		}
		return
	}
	if narrow {
		a.wardInfoNarrow(ib, it, st, own)
		return
	}
	unlocked := cosmetics.Has(it.ID, st, own)
	a.text(ib.Min.X+8, ib.Min.Y+6, it.Name, tierColor(it.Tier), alignLeft)
	a.text(ib.Min.X+16+a.A.Font.Width(it.Name), ib.Min.Y+6, strings.ToUpper(cosmetics.TierNames[it.Tier]), a.A.C("text_dim"), alignLeft)
	a.text(ib.Min.X+8, ib.Min.Y+18, it.Desc, a.A.C("text"), alignLeft)
	req := it.Requirement()
	status := "LOCKED"
	sc := a.A.C("lose")
	if unlocked {
		status, sc = "UNLOCKED - click to wear", a.A.C("win")
		if it.Dynamic() {
			status = "YOURS WHILE YOU HOLD #1"
		}
	} else if it.Dynamic() {
		status = "HELD BY THE CURRENT #1"
	}
	if held, ok := a.recordLine(it); ok {
		status = held
	}
	a.text(ib.Max.X-8, ib.Min.Y+6, status, sc, alignRight)
	a.text(ib.Min.X+8, ib.Min.Y+32, req, a.A.C("gold"), alignLeft)
	if mine, ok := a.recordMine(it); ok {
		a.text(ib.Max.X-8, ib.Min.Y+32, mine, a.A.C("text_dim"), alignRight)
	}
	if !it.Dynamic() && it.Metric != cosmetics.MFree {
		cur, goal := it.Progress(st)
		if unlocked {
			cur = goal
		}
		if cur < 0 {
			cur = 0
		}
		bar := image.Rect(ib.Max.X-170, ib.Min.Y+32, ib.Max.X-8, ib.Min.Y+39)
		fillRect(a.fb, bar, color.RGBA{0, 0, 0, 255})
		in := bar.Inset(1)
		frac := float64(cur) / float64(goal)
		if frac > 1 {
			frac = 1
		}
		fillRect(a.fb, image.Rect(in.Min.X, in.Min.Y, in.Min.X+int(float64(in.Dx())*frac), in.Max.Y), tierColor(it.Tier))
		a.text(bar.Min.X-6, ib.Min.Y+32, commas(cur)+" / "+commas(goal), a.A.C("text_dim"), alignRight)
	}
}

// wardInfoNarrow is the item info box laid out for a narrow screen.
func (a *App) wardInfoNarrow(ib image.Rectangle, it *cosmetics.Item, st cosmetics.Stats, own map[string]bool) {
	unlocked := cosmetics.Has(it.ID, st, own)
	x := ib.Min.X + 8
	y := ib.Min.Y + 6
	a.text(x, y, it.Name, tierColor(it.Tier), alignLeft)
	a.text(ib.Max.X-8, y, strings.ToUpper(cosmetics.TierNames[it.Tier]), a.A.C("text_dim"), alignRight)
	y += 11
	for _, l := range a.wrap(it.Desc, ib.Dx()-16) {
		a.text(x, y, l, a.A.C("text"), alignLeft)
		y += 10
	}
	for _, l := range a.wrap(it.Requirement(), ib.Dx()-16) {
		a.text(x, y, l, a.A.C("gold"), alignLeft)
		y += 10
	}
	status, sc := "LOCKED", a.A.C("lose")
	if unlocked {
		status, sc = "UNLOCKED - tap to wear", a.A.C("win")
		if it.Dynamic() {
			status = "YOURS WHILE YOU HOLD #1"
		}
	} else if it.Dynamic() {
		status = "HELD BY THE CURRENT #1"
	}
	if held, ok := a.recordLine(it); ok {
		status = held
	}
	by := ib.Max.Y - 12
	a.text(x, by, status, sc, alignLeft)
	if !it.Dynamic() && it.Metric != cosmetics.MFree {
		cur, goal := it.Progress(st)
		if unlocked {
			cur = goal
		}
		cur = max(cur, 0)
		bar := image.Rect(ib.Max.X-90, by+1, ib.Max.X-8, by+7)
		fillRect(a.fb, bar, color.RGBA{0, 0, 0, 255})
		in := bar.Inset(1)
		frac := min(float64(cur)/float64(goal), 1)
		fillRect(a.fb, image.Rect(in.Min.X, in.Min.Y, in.Min.X+int(float64(in.Dx())*frac), in.Max.Y), tierColor(it.Tier))
	}
}

// recordUnit is how a record's number reads.
func recordUnit(metric string, v int64) string {
	switch metric {
	case cosmetics.MRankHands:
		if v == 1 {
			return "1 hand"
		}
		return commas(v) + " hands"
	case cosmetics.MRankBet:
		return commas(v) + " chip bet"
	case cosmetics.MRankPeak:
		return commas(v) + " chips at once"
	}
	return commas(v)
}

// recordLine says who holds a record-based item ("Held by Lou: 1,204
// hands"). ok is false for other items.
func (a *App) recordLine(it *cosmetics.Item) (string, bool) {
	if it.Metric != cosmetics.MRankHands && it.Metric != cosmetics.MRankBet && it.Metric != cosmetics.MRankPeak {
		return "", false
	}
	for _, r := range a.records {
		if r.Metric != it.Metric {
			continue
		}
		switch {
		case r.User == "":
			return "NOBODY HOLDS IT YET", true
		case strings.EqualFold(r.User, a.username):
			return "YOURS: " + strings.ToUpper(recordUnit(r.Metric, r.Value)), true
		}
		return "HELD BY " + r.User + ": " + strings.ToUpper(recordUnit(r.Metric, r.Value)), true
	}
	return "", false
}

// recordMine is your own number for a record-based item.
func (a *App) recordMine(it *cosmetics.Item) (string, bool) {
	switch it.Metric {
	case cosmetics.MRankHands:
		return "You: " + recordUnit(it.Metric, int64(a.profile.HandsPlayed+a.profile.PokerHands)), true
	case cosmetics.MRankBet:
		return "You: " + recordUnit(it.Metric, a.profile.BiggestBet), true
	case cosmetics.MRankPeak:
		return "You: " + recordUnit(it.Metric, a.profile.PeakBalance), true
	}
	return "", false
}
