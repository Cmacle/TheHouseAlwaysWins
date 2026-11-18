// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package main

import (
	"image"
	"image/color"
	"math"
)

// Dealer sprite sheet: 9 wealth tiers (-3 destitute .. +5 emperor) in two
// rows: row 0 the regular dealer, row 1 the vulgar scumbag. Cells are 48x40;
// the sprite's feet sit on the bottom edge. Cells are 48x48 (8px headroom for
// crowns and top hats). In the vulgar row the cigar tip is always at (41, 26)
// so the client knows where smoke starts.

const dW, dH, dTop = 48, 40, 8

func ell(x, y int, cx, cy, rx, ry float64) bool {
	dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
	return dx*dx+dy*dy < 1
}

func genDealers() {
	img := newImg(dW*9, (dH+dTop)*2)
	for row := 0; row < 2; row++ {
		for t := -3; t <= 5; t++ {
			spr := dealerSprite(row == 1, t)
			ox, oy := (t+3)*dW, row*(dH+dTop)+dTop
			for y := -dTop; y < dH; y++ {
				for x := 0; x < dW; x++ {
					if c := spr.NRGBAAt(x, y); c.A != 0 {
						img.SetNRGBA(ox+x, oy+y, c)
					}
				}
			}
		}
	}
	save("dealers.png", img)
}

func dealerSprite(vulgar bool, tier int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, -dTop, dW, dH))
	cx := 24.0
	headRX, headRY, headCY := 7.0, 8.0, 15.0
	bodyRX := 15.0
	if vulgar {
		headRX, headRY, headCY = 10.0, 9.5, 15.0
		bodyRX = 21.0
	}
	// Head and body masks. The vulgar dealer is heavy: a wide jaw with jowls
	// and a double chin that sits straight on broad, high shoulders, so there's
	// no visible neck. The regular dealer keeps an oval head on a slim neck.
	inHead := func(x, y int) bool { return ell(x, y, cx, headCY, headRX, headRY) }
	inBody := func(x, y int) bool { return ell(x, y, cx, 46, bodyRX, 17) }
	if vulgar {
		const hcy, hry, hrx = 15.5, 10.5, 10.0
		inHead = func(x, y int) bool {
			t := (float64(y) - hcy) / hry
			if t < -1 || t > 1 {
				return false
			}
			var hw float64
			if t <= 0 {
				hw = hrx * math.Sqrt(1-t*t) // round dome
			} else {
				hw = hrx * math.Cbrt(1-t*t*t) * (1 + 0.15*t) // jowls: widest low, square-ish jaw
			}
			return math.Abs(float64(x)-cx+0.5) <= hw
		}
		inBody = func(x, y int) bool {
			if y < 24 {
				return false
			}
			d := 30.0 - float64(y)
			if d < 0 {
				d = 0
			}
			hw := bodyRX * math.Sqrt(math.Max(0, 1-(d/7)*(d/7)))
			return math.Abs(float64(x)-cx+0.5) <= hw
		}
	}
	skin, skinD := hex("f0c39a"), hex("d39a6e")
	if vulgar {
		skin, skinD = hex("eeb08a"), hex("c98a62")
	}
	if tier <= -2 {
		skin, skinD = shadeC(skin, 0.88), shadeC(skinD, 0.85) // sallow
	}

	// ---------------- clothes palette by tier
	type outfit struct {
		jacket, jacketD, shirt, tie      color.NRGBA
		openShirt, tank, rags, fur, cape bool
	}
	var o outfit
	switch tier {
	case -3:
		o = outfit{jacket: hex("6b5a3a"), jacketD: hex("4a3d26"), shirt: hex("8a7a5a"), rags: true}
	case -2:
		o = outfit{shirt: hex("e8e2c8"), tank: true}
	case -1:
		o = outfit{jacket: hex("5a5a62"), jacketD: hex("3e3e46"), shirt: hex("d8d4c4"), tie: hex("6a4a3a")}
	case 0:
		o = outfit{jacket: hex("1d1d26"), jacketD: hex("101016"), shirt: hex("f4f4f0"), tie: hex("c62a3d")}
	case 1:
		o = outfit{jacket: hex("2a2140"), jacketD: hex("1a1430"), shirt: hex("f4f4f0"), tie: hex("d9a227")}
	case 2:
		o = outfit{jacket: hex("f0ece0"), jacketD: hex("c8c2b0"), shirt: hex("1d1d26"), tie: hex("d9a227")}
	case 3:
		o = outfit{jacket: hex("5a3a22"), jacketD: hex("3e2614"), shirt: hex("f4f4f0"), tie: hex("c62a3d"), fur: true}
	case 4:
		o = outfit{jacket: hex("15151c"), jacketD: hex("0a0a10"), shirt: hex("f4f4f0"), tie: hex("7fe9ff"), fur: true}
	case 5:
		o = outfit{jacket: hex("b01e30"), jacketD: hex("7a1020"), shirt: hex("f2c14e"), tie: hex("ffffff"), cape: true}
	}
	if vulgar && tier >= -1 && tier <= 1 {
		o.openShirt = true
		if tier == 0 {
			o.jacket, o.jacketD = hex("6a1a2a"), hex("4a0f1c")
			o.shirt = hex("e6dfc6")
		}
	}

	// ---------------- cape (behind body)
	if o.cape {
		for y := 22; y < dH; y++ {
			for x := 0; x < dW; x++ {
				if ell(x, y, cx, 46, bodyRX+5, 24) {
					set(img, x, y, hex("8a0f1e"))
				}
			}
		}
	}
	// ---------------- body
	for y := 0; y < dH; y++ {
		for x := 0; x < dW; x++ {
			if !inBody(x, y) {
				continue
			}
			dx := float64(x) - cx
			var c color.NRGBA
			switch {
			case o.tank:
				c = o.shirt
				if math.Abs(dx) > 6 && y < 33 { // bare shoulders
					c = skinD
				}
				if c == o.shirt && ((x*5+y*3)%9 == 0 || (x > 24 && y > 33 && (x+y)%4 == 0)) {
					c = hex("cfc27a") // stains
				}
			case o.rags:
				c = o.jacket
				if dx > 4 {
					c = o.jacketD
				}
				if (x*7+y*5)%13 == 0 {
					c = hex("2a2016") // holes
				}
				if x >= 26 && x <= 29 && y >= 33 && y <= 36 {
					c = hex("9a3a3a") // patch
				}
			default:
				c = o.jacket
				if dx > bodyRX*0.45 {
					c = o.jacketD
				}
				half := 2.5 + float64(y-26)/2.2
				if o.openShirt {
					half += 1.5
				}
				if math.Abs(dx) <= half && y >= 26 {
					c = o.shirt
					if o.openShirt && y >= 29 && y <= 33 && (x+y)%3 == 0 && math.Abs(dx) < 2.5 {
						c = hex("8a6a50") // chest hair
					}
				}
			}
			set(img, x, y, c)
		}
	}
	// fur collar
	if o.fur {
		furC := hex("f0ece4")
		if tier == 3 {
			furC = hex("c8a070")
		}
		for y := 24; y < dH; y++ {
			for x := 0; x < dW; x++ {
				d := math.Abs(float64(x)-cx) - (2.5 + float64(y-26)/2.2)
				if inBody(x, y) && d > 0 && d < 4 && y < 36 {
					c := furC
					if (x+y)%3 == 0 {
						c = shadeC(furC, 0.82)
					}
					set(img, x, y, c)
				}
			}
		}
	}
	if o.cape {
		// ermine collar
		for x := 10; x < 38; x++ {
			for y := 25; y < 28; y++ {
				if ell(x, y, cx, 46, bodyRX+2, 21) {
					c := hex("ffffff")
					if (x+y)%4 == 0 {
						c = hex("15151a")
					}
					set(img, x, y, c)
				}
			}
		}
	}
	// tie / bowtie
	if o.tie.A != 0 && !o.openShirt {
		ty := 27
		if tier == -1 {
			// loosened, crooked tie
			fill(img, 24, ty, 2, 1, o.tie)
			fill(img, 24, ty+1, 2, 5, o.tie)
			set(img, 26, ty+5, o.tie)
		} else {
			bitmap(img, 21, ty, []string{"##..##", "######", "##..##"}, o.tie, false)
		}
	}
	// neck (regular dealer)
	if !vulgar {
		fill(img, 22, 22, 5, 5, skinD)
	}
	// ---------------- head
	for y := 0; y < dH; y++ {
		for x := 0; x < dW; x++ {
			if inHead(x, y) {
				c := skin
				if float64(x) > cx+headRX*0.65 && y > 9 {
					c = skinD
				}
				if vulgar && y >= 18 && y <= 23 && (x+y)%2 == 0 {
					c = hex("a98468") // stubble
				}
				set(img, x, y, c)
			}
		}
	}
	if vulgar {
		// double chin: a soft crease under the jaw, then a lighter fold
		for x := int(cx) - 6; x <= int(cx)+5; x++ {
			set(img, x, 24, skinD)
		}
		for x := int(cx) - 4; x <= int(cx)+3; x++ {
			set(img, x, 25, skin)
		}
		// jowl shading
		set(img, int(cx)-9, 21, skinD)
		set(img, int(cx)-9, 22, skinD)
		set(img, int(cx)+8, 21, skinD)
		set(img, int(cx)+8, 22, skinD)
	}
	// grime for the poorest
	if tier <= -2 {
		set(img, 19, 18, hex("8a6a50"))
		set(img, 28, 13, hex("8a6a50"))
		set(img, 21, 21, hex("8a6a50"))
	}
	// beard when destitute
	if tier == -3 {
		for y := 16; y < 27; y++ {
			for x := 0; x < dW; x++ {
				if inHead(x, y) && ell(x, y, cx, 20, headRX+0.5, 8) && y >= 17 && (x*3+y)%4 != 0 {
					set(img, x, y, hex("8a8a90"))
				}
			}
		}
	}
	// ---------------- face
	eyeY := 14
	lx, rx := 21, 27
	if vulgar {
		lx, rx = 19, 29
	}
	sunglasses := vulgar && tier >= -1 || (!vulgar && tier == 3)
	switch {
	case sunglasses:
		w := 4
		if vulgar {
			w = 7
		}
		fill(img, lx-w/2, eyeY-1, w, 3, hex("111116"))
		fill(img, rx-w/2, eyeY-1, w, 3, hex("111116"))
		fill(img, lx+w/2, eyeY-1, rx-lx-w+1, 1, hex("111116"))
		set(img, lx-w/2+1, eyeY-1, hex("ffffff"))
		set(img, rx-w/2+1, eyeY-1, hex("ffffff"))
		if tier >= 4 {
			fill(img, lx-w/2, eyeY-2, w*2+rx-lx-w+1, 1, hex("f2c14e")) // gold frames
		}
	default:
		set(img, lx, eyeY, hex("1a1a1a"))
		set(img, rx-1, eyeY, hex("1a1a1a"))
		if tier <= -1 {
			set(img, lx, eyeY+1, hex("6a5a7a")) // eye bags
			set(img, rx-1, eyeY+1, hex("6a5a7a"))
		}
	}
	if tier == 4 && !sunglasses {
		// monocle
		for a := 0.0; a < 2*math.Pi; a += 0.3 {
			set(img, rx-1+int(math.Round(2*math.Cos(a))), eyeY+int(math.Round(2*math.Sin(a))), hex("f2c14e"))
		}
		fill(img, rx+1, eyeY+2, 1, 8, hex("f2c14e"))
	}
	// nose
	if vulgar {
		fill(img, 23, 16, 3, 2, hex("d86a5a"))
	} else {
		set(img, 24, 17, skinD)
	}
	// mouth / mustache
	mY := 20
	if vulgar {
		fill(img, 20, mY, 6, 1, hex("5a2a20"))
		set(img, 26, mY-1, hex("5a2a20"))
		if tier >= 3 {
			set(img, 22, mY, hex("f2c14e")) // gold tooth
			set(img, 23, mY, hex("f2c14e"))
		}
		if tier <= -2 {
			fill(img, 20, mY, 6, 1, hex("5a2a20"))
			set(img, 20, mY+1, hex("5a2a20")) // frown
			set(img, 25, mY+1, hex("5a2a20"))
		}
	} else {
		must := hex("2a1a10")
		if tier == -3 {
			must = hex("8a8a90")
		}
		bitmap(img, 20, 18, []string{"#########", ".##...##."}, must, false)
		if tier >= 3 {
			set(img, 24, 20, hex("f2c14e"))
		}
	}
	// ---------------- hair & headwear
	hair := hex("2a1a10")
	if vulgar {
		hair = hex("2e2016")
	}
	if tier <= -2 {
		hair = hex("6a6a70")
	}
	topY := int(headCY - headRY)
	switch {
	case vulgar && tier <= 2 && tier >= -1:
		for x := 15; x <= 33; x++ { // greasy comb-over
			if x%3 != 1 {
				set(img, x, topY+2, hair)
			}
			if x%2 == 0 {
				set(img, x+1, topY+3, hair)
			}
		}
		fill(img, 13, 12, 2, 6, hair)
		fill(img, 34, 12, 2, 6, hair)
	case vulgar && tier == -2:
		fill(img, 13, 13, 2, 4, hair) // bald, tufts
		fill(img, 34, 13, 2, 4, hair)
	case !vulgar && tier >= -2:
		for y := 0; y < dH; y++ {
			for x := 0; x < dW; x++ {
				if ell(x, y, cx, headCY-3, headRX+0.5, headRY-2.5) && y < topY+5 {
					set(img, x, y, hair)
				}
			}
		}
	}
	visor := func(c, edge color.NRGBA, torn bool) {
		fill(img, int(cx-headRX), topY+4, int(headRX*2)+1, 2, c)
		fill(img, int(cx-headRX)-2, topY+6, int(headRX*2)+5, 1, edge)
		if torn {
			set(img, int(cx)+3, topY+6, color.NRGBA{})
			set(img, int(cx)+4, topY+6, color.NRGBA{})
			set(img, int(cx)+4, topY+5, color.NRGBA{})
		}
	}
	switch tier {
	case -3:
		// ratty beanie
		for y := 0; y < dH; y++ {
			for x := 0; x < dW; x++ {
				if ell(x, y, cx, float64(topY+4), headRX+1, 5) && y <= topY+5 {
					c := hex("5a4a6a")
					if (x+y)%5 == 0 {
						c = hex("3a2e46")
					}
					set(img, x, y, c)
				}
			}
		}
		set(img, int(cx)-2, topY, color.NRGBA{})
		set(img, int(cx)+3, topY+1, color.NRGBA{})
	case -1:
		if !vulgar {
			visor(hex("5a7a5a"), hex("3a4a3a"), true)
		}
	case 0:
		if !vulgar {
			visor(hex("2e9d4f"), hex("1f6e37"), false)
		}
	case 1:
		if !vulgar {
			visor(hex("2e9d4f"), hex("d9a227"), false)
		}
	case 4:
		// top hat
		fill(img, int(cx)-7, topY-8, 15, 10, hex("15151c"))
		fill(img, int(cx)-7, topY-1, 15, 2, hex("c62a3d"))
		fill(img, int(cx)-11, topY+1, 23, 2, hex("15151c"))
		fill(img, int(cx)-6, topY-8, 1, 7, hex("3a3a48"))
	case 5:
		// crown
		cy := topY - 6
		gold, goldD := hex("f2c14e"), hex("b07a18")
		fill(img, int(cx)-9, cy+4, 19, 5, gold)
		fill(img, int(cx)-9, cy+8, 19, 1, goldD)
		for _, px := range []int{-9, -4, 1, 6} {
			bitmap(img, int(cx)+px-0, cy, []string{"..#..", ".###.", "#####", "#####"}, gold, false)
		}
		set(img, int(cx)-7, cy-1, hex("e0283c"))
		set(img, int(cx)+3, cy-1, hex("3a7bff"))
		set(img, int(cx)+8, cy-1, hex("e0283c"))
		fill(img, int(cx), cy+5, 2, 2, hex("e0283c"))
	}
	// ---------------- jewelry & props
	gold := hex("f2c14e")
	chain := func(n int, droop float64) {
		for k := 0; k < n; k++ {
			for x := 18; x <= 30; x++ {
				dy := int(math.Round(droop * (1 - math.Pow((float64(x)-24)/6, 2))))
				set(img, x, 26+dy+k*2, gold)
			}
		}
	}
	switch {
	case vulgar && tier == 0:
		chain(1, 2.5)
		fill(img, 23, 29, 3, 3, gold)
	case tier == 1:
		chain(1, 2.5)
		fill(img, 23, 29, 3, 3, gold)
	case tier == 2:
		chain(2, 2.5)
		fill(img, 23, 31, 3, 3, gold)
		fill(img, 31, 31, 3, 2, hex("ff5c8a")) // pocket square
	case tier == 3:
		chain(2, 3)
		fill(img, 22, 31, 5, 4, gold)
		set(img, 24, 32, hex("ffffff"))
	case tier == 4:
		chain(1, 2.5)
		bitmap(img, 22, 31, []string{".#.", "###", ".#."}, hex("7fe9ff"), false) // diamond pin
		set(img, 23, 32, hex("ffffff"))
	case tier == 5:
		chain(3, 3)
		fill(img, 22, 33, 5, 4, gold)
		set(img, 24, 34, hex("e0283c"))
		// scepter in hand
		fill(img, 41, 18, 2, 22, goldD())
		bitmap(img, 39, 14, []string{".###.", "#####", "##R##", "#####", ".###."}, gold, false)
		set(img, 41, 16, hex("e0283c"))
	}
	// rings on a visible fist for the rich
	if tier >= 3 && tier <= 4 {
		fill(img, 5, 33, 5, 4, skin)
		set(img, 6, 33, gold)
		set(img, 8, 33, hex("7fe9ff"))
	}
	if tier == -3 {
		// cardboard sign
		fill(img, 2, 27, 15, 10, hex("b08a5a"))
		rectOutline(img, 2, 27, 15, 10, hex("6a4a2a"))
		for _, ln := range []int{29, 31, 33} {
			for x := 4; x < 15; x++ {
				if (x*7+ln)%4 != 0 {
					set(img, x, ln, hex("2a1a10"))
				}
			}
		}
	}
	// ---------------- smoking (vulgar: tip always at 41,18)
	if vulgar {
		switch {
		case tier <= -2: // cigarette butt
			fill(img, 27, 19, 4, 1, hex("f4f4f0"))
			set(img, 31, 19, hex("ff5a1a"))
			fill(img, 32, 18, 10, 1, color.NRGBA{}) // keep tip coordinate meaningful below
			set(img, 41, 18, color.NRGBA{})
		case tier >= 3: // fat Cuban
			fill(img, 27, 18, 14, 3, hex("6a3a1a"))
			fill(img, 27, 18, 14, 1, hex("8a5228"))
			fill(img, 31, 18, 2, 3, hex("f2c14e"))
			fill(img, 41, 18, 1, 3, hex("ff5a1a"))
		default:
			fill(img, 27, 19, 14, 2, hex("7a4a22"))
			fill(img, 27, 19, 14, 1, hex("9a6232"))
			fill(img, 31, 19, 1, 2, hex("d9a227"))
			fill(img, 41, 18, 1, 3, hex("ff5a1a"))
		}
	} else if tier == 5 || tier == 4 {
		// classy cigar for the rich clean dealer too
		fill(img, 27, 19, 12, 2, hex("6a3a1a"))
		fill(img, 39, 19, 1, 2, hex("ff5a1a"))
	}
	// ---------------- outline
	out := image.NewNRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	for y := -dTop; y < dH; y++ {
		for x := 0; x < dW; x++ {
			if img.NRGBAAt(x, y).A != 0 {
				continue
			}
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if image.Pt(nx, ny).In(img.Rect) && img.NRGBAAt(nx, ny).A != 0 {
					out.SetNRGBA(x, y, hex("15151a"))
					break
				}
			}
		}
	}
	return out
}

func goldD() color.NRGBA { return hex("b07a18") }
