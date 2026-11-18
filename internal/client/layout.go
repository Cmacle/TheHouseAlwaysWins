// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"math/rand"
)

// The game draws onto a small pixel-art canvas that the platform scales up to
// fill the window. The canvas size follows the window's shape: 360 pixels on
// the short side in landscape, 360 wide in portrait, so the layout fills any
// window without black bars and every screen anchors to its edges.

// ScreenW and ScreenH are the current canvas size.
var ScreenW, ScreenH = 640, 360

const (
	minLandW, maxLandW = 640, 900
	minLandH, maxLandH = 360, 480
	minPortW, maxPortW = 360, 450
	minPortH, maxPortH = 560, 820
)

// ChooseSize picks the canvas size for a window (or screen) of pw x ph pixels.
func ChooseSize(pw, ph int) (w, h int) {
	if pw <= 0 || ph <= 0 {
		return 640, 360
	}
	aspect := float64(pw) / float64(ph)
	if aspect >= 1 {
		if aspect >= 16.0/9 {
			return clampi(int(math.Round(360*aspect)), minLandW, maxLandW), 360
		}
		return 640, clampi(int(math.Round(640/aspect)), minLandH, maxLandH)
	}
	h = int(math.Round(360 / aspect))
	if h < minPortH {
		// squarish portrait (a tablet): grow the width instead
		return clampi(int(math.Round(minPortH*aspect)), minPortW, maxPortW), minPortH
	}
	return 360, clampi(h, minPortH, maxPortH)
}

func clampi(v, lo, hi int) int { return min(max(v, lo), hi) }

// SetSize changes the canvas size. Platforms call it when the window changes.
func (a *App) SetSize(w, h int) {
	if w <= 0 || h <= 0 || (w == ScreenW && h == ScreenH && a.fb != nil && a.fb.Rect.Dx() == w) {
		return
	}
	ScreenW, ScreenH = w, h
	a.fb = image.NewRGBA(image.Rect(0, 0, w, h))
	a.tableBG = nil
}

// Portrait reports whether the canvas is taller than it is wide.
func Portrait() bool { return ScreenH > ScreenW }

// tableLayout holds where things go on the table screen for the current
// canvas size.
type tableLayout struct {
	portrait bool
	top      int             // shift of the dealer area (portrait has a taller top bar)
	ecx, ecy float64         // center of the table's rail ellipse
	rx, ry   float64         // its radii
	shoe     image.Point     // card shoe (cards fly from here)
	discard  image.Point     // discard holder, top-left
	tray     image.Point     // dealer's chip tray, top-left
	dealerY  int             // bottom of the dealer sprite
	cardsY   int             // top of the dealer's cards
	msgY     int             // table message
	rulesY   int             // the rules printed on the felt
	bubble   image.Rectangle // speech bubble area (portrait: under the dealer)
	noRules  bool            // short portrait: no room for the rules on the felt
	frontY   int             // portrait: avatar centre of the front row
	backY    int             // portrait: avatar centre of the back row
	hudTop   int             // top of the bottom panel
	center   int             // top of the bet/action area inside the panel
	cx0, cx1 int             // bet/action area
	rx0, rx1 int             // phase/timer/buttons area
	felt     [2]struct{ rx, ry float64 }
}

func (a *App) computeLayout() {
	w, h := float64(ScreenW), float64(ScreenH)
	L := tableLayout{portrait: Portrait()}
	if !L.portrait {
		extra := ScreenH - 360
		L.hudTop = ScreenH - 62
		L.center = L.hudTop
		L.cx0, L.cx1 = 168, ScreenW-164
		L.rx0, L.rx1 = ScreenW-156, ScreenW-10
		L.ecx, L.ecy = w/2, 40
		L.rx = 420 * w / 640
		L.ry = 222 + float64(extra)*0.85
		L.shoe = image.Pt(ScreenW-52, 24)
		L.discard = image.Pt(62*ScreenW/640, 22)
		L.tray = image.Pt(ScreenW/2-72, 2)
		L.dealerY = 52
		L.cardsY = 58
		L.msgY = 96 + extra/4
		L.rulesY = 110 + extra/4
		L.felt[0] = struct{ rx, ry float64 }{250 * w / 640, 100 + float64(extra)/4}
		L.felt[1] = struct{ rx, ry float64 }{262 * w / 640, 112 + float64(extra)/4}
	} else {
		L.top = 40
		L.hudTop = ScreenH - 112
		L.center = L.hudTop + 52
		L.cx0, L.cx1 = 8, ScreenW-8
		L.rx0, L.rx1 = ScreenW-150, ScreenW-8
		L.shoe = image.Pt(ScreenW-40, 30+L.top)
		L.discard = image.Pt(14, 26+L.top)
		L.tray = image.Pt(ScreenW/2-72, 2+L.top)
		L.dealerY = 52 + L.top
		L.cardsY = 58 + L.top
		L.bubble = image.Rect(10, L.cardsY+40, ScreenW-10, L.cardsY+40+4*9+8)
		L.msgY = L.bubble.Max.Y + 8
		L.rulesY = L.msgY + 13
		// two rows of seats between the rules and the panel
		L.noRules = ScreenH < 620
		feltBottom := L.rulesY + 30
		if L.noRules {
			feltBottom = L.msgY + 10
		}
		space := L.hudTop - feltBottom
		L.frontY = L.hudTop - 46
		L.backY = L.frontY - max(110, min(space/2+20, 150))
		L.backY = max(L.backY, feltBottom+80) // back row's cards clear the felt text
		L.ecx, L.ecy = w/2, float64(L.top)+20
		L.ry = float64(L.frontY+12) - L.ecy
		L.rx = w * 0.7
		L.felt[0] = struct{ rx, ry float64 }{w * 0.42, float64(L.rulesY+22) - 30 - float64(L.top)}
		L.felt[1] = struct{ rx, ry float64 }{w*0.42 + 12, float64(L.rulesY+34) - 30 - float64(L.top)}
	}
	_ = h
	a.L = L
}

// seatPos is where seat i of n goes: x and the avatar's center y.
func (a *App) seatPos(i, n int) (x, avatarCY int) {
	L := a.L
	if !L.portrait {
		if n <= 1 {
			x = ScreenW / 2
		} else {
			x = 46 + i*(ScreenW-92)/(n-1)
		}
		dx := (float64(x) - L.ecx) / L.rx
		rail := L.ecy + L.ry*math.Sqrt(math.Max(0, 1-dx*dx))
		return x, int(rail) - 10
	}
	front, back := portraitRows(n)
	row, k := back, 0
	y := L.backY
	for j, s := range front {
		if s == i {
			row, k, y = front, j, L.frontY
		}
	}
	if y == L.backY {
		for j, s := range back {
			if s == i {
				k = j
			}
		}
	}
	m := len(row)
	margin := 30
	if m <= 1 {
		return ScreenW / 2, y
	}
	if y == L.frontY && m < len(back) {
		margin = 30 + (ScreenW-60)/(2*(len(back)-1)) // sit between the back row's seats
	}
	x = margin + k*(ScreenW-2*margin)/(m-1)
	if y == L.backY {
		// the back row curves down at the ends, around the table
		d := math.Abs(float64(x)-L.ecx) / (float64(ScreenW) / 2)
		y += int(d * d * 18)
	}
	return x, y
}

// portraitRows splits the seats into a front row (the middle seats, nearest
// you) and a back row (the outer seats), each in seat order.
func portraitRows(n int) (front, back []int) {
	if n <= 5 {
		for i := 0; i < n; i++ {
			front = append(front, i)
		}
		return
	}
	mid := float64(n-1) / 2
	for i := 0; i < n; i++ {
		if math.Abs(float64(i)-mid) < float64(n)/4 {
			front = append(front, i)
		} else {
			back = append(back, i)
		}
	}
	return
}

// seatSpacing is the room each seat has for its cards.
func (a *App) seatSpacing(n int) int {
	if !a.L.portrait {
		if n <= 1 {
			return 60
		}
		return min(60, (ScreenW-92)/(n-1)-2)
	}
	front, back := portraitRows(n)
	m := max(len(front), len(back))
	if m <= 1 {
		return 60
	}
	return min(64, (ScreenW-60)/(m-1)-2)
}

// ---------------------------------------------------------------- table art

// drawTableBG draws the felt, rail and floor for the current canvas. The
// default art is drawn to fit; a replaced table.png is stretched to fit.
func (a *App) drawTableBG() {
	if a.tableBG == nil || a.tableBG.Rect.Dx() != ScreenW || a.tableBG.Rect.Dy() != ScreenH {
		a.computeLayout()
		if a.A.TableCustom || (ScreenW == 640 && ScreenH == 360) {
			a.tableBG = image.NewRGBA(image.Rect(0, 0, ScreenW, ScreenH))
			blitScaled(a.tableBG, a.A.Table, a.tableBG.Rect)
		} else {
			a.tableBG = a.genTable()
		}
	}
	copy(a.fb.Pix, a.tableBG.Pix)
}

var bayer4 = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

func hexc(s string) color.RGBA { return parseHex("#" + s) }

// genTable paints the table like the default table.png, at any size. The
// chip tray and discard holder are copied from table.png.
func (a *App) genTable() *image.RGBA {
	W, H := ScreenW, ScreenH
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	L := a.L
	rng := rand.New(rand.NewSource(7))
	feltLevels := []color.RGBA{hexc("0e3f24"), hexc("13502e"), hexc("176238"), hexc("1b7242"), hexc("1f7d48")}
	floor := []color.RGBA{hexc("1a0f0a"), hexc("24150d"), hexc("2d1a10")}
	railDark, rail, railLite, railHi := hexc("2b170b"), hexc("5a3418"), hexc("7b4a24"), hexc("a06a38")
	railW := 11.0
	outer := 1 + railW/L.ry
	feltCY := float64(L.ecy) + L.ry*0.36
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			dx, dy := (float64(x)-L.ecx)/L.rx, (float64(y)-L.ecy)/L.ry
			e := math.Sqrt(dx*dx + dy*dy)
			th := bayer4[y%4][x%4] / 16
			var c color.RGBA
			switch {
			case e < 1:
				fx := (float64(x) - L.ecx) / (L.rx * 0.9)
				fy := (float64(y) - feltCY) / (L.ry * 1.17)
				v := 1 - (fx*fx + fy*fy*1.2)
				v = v*4.2 + (rng.Float64()-0.5)*0.35
				c = feltLevels[clampi(int(math.Floor(v+th)), 0, len(feltLevels)-1)]
			case e < outer:
				t := (e - 1) / (outer - 1)
				switch {
				case t < 0.12:
					c = railDark
				case t < 0.3:
					c = railHi
				case t < 0.55:
					c = railLite
				case t < 0.88:
					c = rail
				default:
					c = railDark
				}
			default:
				v := float64(y)/float64(H)*2 + (rng.Float64()-0.5)*0.4
				c = floor[clampi(int(math.Floor(v+th)), 0, len(floor)-1)]
				if (x/8+y/8)%2 == 0 && (x+y)%8 == 0 {
					c = color.RGBA{uint8(min(255, int(c.R)*14/10)), uint8(min(255, int(c.G)*14/10)), uint8(min(255, int(c.B)*14/10)), 255}
				}
			}
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	line := hexc("3aa363")
	arcY := float64(30 + L.top)
	for _, r := range L.felt {
		for t := 0.18; t < math.Pi-0.18; t += 0.0015 {
			x := int(math.Round(L.ecx + r.rx*math.Cos(t)))
			y := int(math.Round(arcY + r.ry*math.Sin(t)))
			if image.Pt(x, y).In(img.Rect) {
				img.SetRGBA(x, y, line)
			}
		}
	}
	// chip tray and discard holder from the table art
	src := a.A.Table
	if src.Rect.Dx() >= 640 && src.Rect.Dy() >= 360 {
		copyRegion(img, src, image.Rect(248, 2, 392, 18), L.tray)
		copyRegion(img, src, image.Rect(62, 22, 88, 56), L.discard)
	}
	return img
}

func copyRegion(dst, src *image.RGBA, r image.Rectangle, at image.Point) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p := image.Pt(at.X+x-r.Min.X, at.Y+y-r.Min.Y)
			if p.In(dst.Rect) {
				dst.SetRGBA(p.X, p.Y, src.RGBAAt(x, y))
			}
		}
	}
}

// layoutTop is how far the dealer area is pushed down (portrait).
func layoutTop() int {
	if Portrait() {
		return 40
	}
	return 0
}

// ovRect is a window of up to w x h, centered and kept on screen.
func ovRect(w, h int) image.Rectangle {
	w = min(w, ScreenW-8)
	h = min(h, ScreenH-8)
	x := (ScreenW - w) / 2
	y := (ScreenH - h) / 2
	return image.Rect(x, y, x+w, y+h)
}

// isDefaultAsset reports whether b is the built-in copy of an asset.
func isDefaultAsset(name string, b []byte) bool {
	def, err := defaultFS.ReadFile("defaultassets/" + name)
	return err == nil && bytes.Equal(def, b)
}
