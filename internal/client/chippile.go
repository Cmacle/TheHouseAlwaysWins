// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// Each player's chips sit in a little heap beside their portrait: bigger the
// more they have, coloured by what it's made of, with the amount on top.

const pileChip = 12 // chip size in the pile

// shortChips writes an amount compactly: 950, 1.2K, 34K, 1.5M.
func shortChips(v int64) string {
	switch {
	case v >= 10_000_000:
		return fmt.Sprintf("%dM", v/1_000_000)
	case v >= 1_000_000:
		return trimZero(fmt.Sprintf("%.1f", float64(v)/1e6)) + "M"
	case v >= 10_000:
		return fmt.Sprintf("%dK", v/1000)
	case v >= 1000:
		return trimZero(fmt.Sprintf("%.1f", float64(v)/1e3)) + "K"
	}
	return fmt.Sprint(v)
}

func trimZero(s string) string {
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}

// pileChips picks which chip sprites make up a pile for an amount, bottom
// first: a handful sampled from the amount broken into chips, biggest first.
func (a *App) pileChips(balance int64) []int {
	if balance <= 0 {
		return nil
	}
	vals := a.A.Theme.ChipValues
	n := 1 + int(math.Round(2.2*math.Log10(float64(balance)/float64(max(1, vals[0]))+1)))
	n = min(max(n, 1), 6)
	var all []int
	left := balance
	for i := len(vals) - 1; i >= 0 && len(all) < 400; i-- {
		for left >= vals[i] && len(all) < 400 {
			all = append(all, i)
			left -= vals[i]
		}
	}
	if len(all) == 0 {
		return []int{0} // less than the smallest chip: one lonely chip
	}
	n = min(n, len(all)) // never more chips than it really is
	out := make([]int, n)
	for k := range out {
		out[k] = all[k*len(all)/n]
	}
	return out
}

func (a *App) smallChip(i int) *image.RGBA {
	if a.pileImgs == nil {
		a.pileImgs = map[int]*image.RGBA{}
	}
	if img, ok := a.pileImgs[i]; ok {
		return img
	}
	img := scaleImage(a.A.ChipImage(i), pileChip, pileChip)
	a.pileImgs[i] = img
	return img
}

// drawChipPile draws a seat's pile: top-down chips heaped in a little
// pyramid that fills out as the stack grows, with the amount on top. Its
// bottom is at bottom; x is the side next to the portrait, and the heap and
// its amount lean away from the portrait (leftwards when the pile is left of
// it, at the right edge of the screen).
func (a *App) drawChipPile(x, bottom int, balance int64, leftOfPortrait bool) {
	chips := a.pileChips(balance)
	if len(chips) == 0 {
		return
	}
	// bottom row of three, then two, then one on top
	spots := [][2]int{{0, 0}, {6, 0}, {3, -4}, {12, 0}, {9, -4}, {6, -8}}
	cs := a.smallChip(0).Rect.Dx()
	top := bottom
	for j := 0; j < min(len(chips), len(spots)); j++ {
		dx, dy := spots[j][0]-2, spots[j][1]
		cx := x + dx
		if leftOfPortrait {
			cx = x + pileChip - dx - cs
		}
		blit(a.fb, a.smallChip(chips[j]), cx, bottom-cs+dy)
		top = min(top, bottom-cs+dy)
	}
	al, lx := alignLeft, x
	if leftOfPortrait {
		al, lx = alignRight, x+pileChip
	}
	a.textOutlined(lx, top-9, shortChips(balance), a.A.C("gold"), al)
}

// textOutlined draws small text with a dark outline so it reads on
// the felt.
func (a *App) textOutlined(x, y int, s string, c color.RGBA, al align) {
	sh := color.RGBA{0, 0, 0, 255}
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		a.text(x+d[0], y+d[1], s, sh, al)
	}
	a.text(x, y, s, c, al)
}
