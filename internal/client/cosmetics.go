// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

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
	s := a.A.Theme.FrameSize
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

// drawFrame draws an avatar frame around the 32x32 box at (x, y).
func (a *App) drawFrame(id string, x, y int) {
	s := a.A.Theme.FrameSize
	pad := (s - 32) / 2
	spr := a.frameSprite(id)
	if spr == nil {
		strokeRect(a.fb, image.Rect(x-2, y-2, x+34, y+34), a.A.C("shadow"))
		return
	}
	blit(a.fb, spr, x-pad, y-pad)
	if id == "frame_rainbow" {
		// animate: recolour the visible ring by perimeter angle
		off := float64(a.frame) / 90
		for yy := 0; yy < s; yy++ {
			for xx := 0; xx < s; xx++ {
				if spr.Pix[spr.PixOffset(spr.Rect.Min.X+xx, spr.Rect.Min.Y+yy)+3] == 0 {
					continue
				}
				ang := (math.Atan2(float64(yy)-float64(s)/2, float64(xx)-float64(s)/2) + math.Pi) / (2 * math.Pi)
				c := hsv(ang + off)
				px, py := x-pad+xx, y-pad+yy
				if image.Pt(px, py).In(a.fb.Rect) {
					o := a.fb.PixOffset(px, py)
					a.fb.Pix[o], a.fb.Pix[o+1], a.fb.Pix[o+2], a.fb.Pix[o+3] = c.R, c.G, c.B, 255
				}
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
	blit(a.fb, spr, hx, hy)
	if id == "crown" {
		// twinkle
		pts := [][2]int{{8, 9}, {16, 6}, {24, 9}, {12, 14}, {20, 14}}
		for i, p := range pts {
			ph := (a.frame/8 + i*3) % 10
			if ph < 2 {
				px, py := hx+p[0], hy+p[1]
				c := color.RGBA{255, 255, 255, 255}
				fillRect(a.fb, image.Rect(px, py, px+1, py+1), c)
				if ph == 0 {
					fillRect(a.fb, image.Rect(px-1, py, px+2, py+1), c)
					fillRect(a.fb, image.Rect(px, py-1, px+1, py+2), c)
				}
			}
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
		RankRich: p.RankRich, RankNet: p.RankNet,
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
						blit(a.fb, spr, hx, hy)
						if it.ID == "crown" {
							a.drawHat("crown", hx, hy+a.A.Theme.HatHeight-a.A.Theme.HatOverlap)
						}
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
					silhouette(a.fb, spr, ax-2, ay-2, color.RGBA{70, 62, 58, 255})
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
	a.text(ib.Max.X-8, ib.Min.Y+6, status, sc, alignRight)
	a.text(ib.Min.X+8, ib.Min.Y+32, req, a.A.C("gold"), alignLeft)
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
