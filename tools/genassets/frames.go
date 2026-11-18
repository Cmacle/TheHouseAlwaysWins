// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package main

import (
	"image/color"
	"math"
)

// Frames: 48x48 cells around the 32x32 avatar, which sits at (8,8)-(40,40)
// and is never drawn over. Common frames are a plain ring; the rarer the
// frame, the more it spills into the 8 pixels around it. The client animates
// the inferno, diamond, rainbow, marathon and vault frames.
const (
	frameS = 48
	avLo   = 8  // first avatar pixel
	avHi   = 39 // last avatar pixel
)

type canvas struct {
	px [frameS][frameS]color.NRGBA
}

// dist is how far a pixel is outside the avatar box (0 = inside it).
func dist(x, y int) int {
	dx := max(avLo-x, x-avHi, 0)
	dy := max(avLo-y, y-avHi, 0)
	return max(dx, dy)
}

func (c *canvas) set(x, y int, col color.NRGBA) {
	if x < 0 || y < 0 || x >= frameS || y >= frameS || dist(x, y) == 0 {
		return // never over the picture
	}
	c.px[y][x] = col
}

func (c *canvas) get(x, y int) color.NRGBA {
	if x < 0 || y < 0 || x >= frameS || y >= frameS {
		return color.NRGBA{}
	}
	return c.px[y][x]
}

// ring fills every pixel 1..t outside the avatar using f.
func (c *canvas) ring(t int, f func(x, y, d int) color.NRGBA) {
	for y := 0; y < frameS; y++ {
		for x := 0; x < frameS; x++ {
			if d := dist(x, y); d >= 1 && d <= t {
				c.set(x, y, f(x, y, d))
			}
		}
	}
}

// bevel colours a ring light on the top and left, dark on the bottom and
// right, with an outline outside.
func (c *canvas) bevel(t int, hi, mid, lo, line color.NRGBA) {
	c.ring(t, func(x, y, d int) color.NRGBA {
		if d == t {
			return line
		}
		if d == 1 {
			return shadeC(mid, 0.6)
		}
		top := y < avLo || (x < avLo && y <= avHi)
		if top && !(y > avHi) {
			return hi
		}
		if y > avHi || x > avHi {
			return lo
		}
		return mid
	})
}

// sprite stamps a small bitmap; '.' is clear, 'k' the outline colour.
func (c *canvas) sprite(x, y int, rows []string, pal map[rune]color.NRGBA) {
	for j, r := range rows {
		for i, ch := range r {
			if ch == '.' {
				continue
			}
			col, ok := pal[ch]
			if ch == 'k' && !ok {
				col, ok = hex("15151a"), true
			}
			if ok {
				c.set(x+i, y+j, col)
			}
		}
	}
}

// mirror4 stamps a corner bitmap in all four corners (drawn for top-left).
func (c *canvas) mirror4(rows []string, pal map[rune]color.NRGBA) {
	h := len(rows)
	w := 0
	for _, r := range rows {
		w = max(w, len(r))
	}
	for j, r := range rows {
		for i, ch := range r {
			if ch == '.' {
				continue
			}
			col, ok := pal[ch]
			if ch == 'k' && !ok {
				col, ok = hex("15151a"), true
			}
			if !ok {
				continue
			}
			c.set(i, j, col)
			c.set(frameS-1-i, j, col)
			c.set(i, frameS-1-j, col)
			c.set(frameS-1-i, frameS-1-j, col)
		}
	}
	_, _ = h, w
}

// outline adds a dark edge around everything drawn so far.
func (c *canvas) outline(col color.NRGBA) {
	var add [][2]int
	for y := 0; y < frameS; y++ {
		for x := 0; x < frameS; x++ {
			if c.px[y][x].A != 0 || dist(x, y) == 0 {
				continue
			}
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if c.get(x+d[0], y+d[1]).A != 0 {
					add = append(add, [2]int{x, y})
					break
				}
			}
		}
	}
	for _, p := range add {
		c.set(p[0], p[1], col)
	}
}

func hsvN(h float64) color.NRGBA {
	h = math.Mod(h, 1) * 6
	i := int(h)
	f := h - float64(i)
	q, t := uint8(255*(1-f)), uint8(255*f)
	switch i {
	case 0:
		return color.NRGBA{255, t, 0, 255}
	case 1:
		return color.NRGBA{q, 255, 0, 255}
	case 2:
		return color.NRGBA{0, 255, t, 255}
	case 3:
		return color.NRGBA{0, q, 255, 255}
	case 4:
		return color.NRGBA{t, 0, 255, 255}
	}
	return color.NRGBA{255, 0, q, 255}
}

func genFrames() {
	K := hex("15151a")
	styles := []func(c *canvas){
		// 0 wood (common): a plain wooden ring with nail heads
		func(c *canvas) {
			c.ring(3, func(x, y, d int) color.NRGBA {
				switch {
				case d == 3:
					return hex("2b170b")
				case (x+y*3)%7 == 0:
					return hex("8a5a30")
				}
				return hex("6b4220")
			})
			for _, p := range [][2]int{{6, 6}, {41, 6}, {6, 41}, {41, 41}} {
				c.set(p[0], p[1], hex("c9c6ba"))
			}
		},
		// 1 felt (common): green baize with brass tacks
		func(c *canvas) {
			c.ring(3, func(x, y, d int) color.NRGBA {
				switch {
				case d == 3:
					return hex("0e3f24")
				case d == 1:
					return hex("f2c14e")
				}
				return hex("1f7d48")
			})
			c.mirror4([]string{
				"",
				"",
				"",
				"",
				"....kk",
				"...kyYk",
				"...kYYk",
				"....kk",
			}, map[rune]color.NRGBA{'y': hex("f2c14e"), 'Y': hex("fff2b8")})
		},
		// 2 silver (uncommon): bevelled, with round rivets on the corners
		func(c *canvas) {
			c.bevel(4, hex("ffffff"), hex("c4c9d4"), hex("7d8494"), hex("3a3f4b"))
			c.mirror4([]string{
				"......",
				"..kkk.",
				".kWwsk",
				".kwwsk",
				".kssdk",
				"..kkk.",
			}, map[rune]color.NRGBA{'W': hex("ffffff"), 'w': hex("d8dce6"), 's': hex("a2a8b6"), 'd': hex("5a5f6b")})
		},
		// 3 gold (rare): a bevelled gold frame with scrollwork corners and a
		// gem on top
		func(c *canvas) {
			c.bevel(4, hex("fff2b8"), hex("f2c14e"), hex("b8861f"), hex("6a440c"))
			c.mirror4([]string{
				"..kkkk...",
				".kyyyyk..",
				"kyYkkyyk.",
				"kyk..kyk.",
				"kyk...kk.",
				"kyyk.....",
				".kyk.....",
				"..k......",
			}, map[rune]color.NRGBA{'y': hex("f2c14e"), 'Y': hex("fff2b8")})
			c.sprite(21, 0, []string{
				"..kkkk..",
				".kbBbbk.",
				"kbBbbbbk",
				".kbbbdk.",
				"..kddk..",
				"...kk...",
			}, map[rune]color.NRGBA{'b': hex("3fb6d8"), 'B': hex("dff8ff"), 'd': hex("1f6f8a")})
		},
		// 4 ruby (rare): gold with cut rubies on the corners and sides
		func(c *canvas) {
			c.bevel(4, hex("fff2b8"), hex("e0a92e"), hex("9a6a14"), hex("4a2a08"))
			ruby := []string{
				"...kk...",
				"..kRrk..",
				".kRrrrk.",
				"kRrrrrdk",
				".krrrdk.",
				"..krdk..",
				"...kk...",
			}
			pal := map[rune]color.NRGBA{'R': hex("ff8a99"), 'r': hex("e0283c"), 'd': hex("7a0c1c")}
			for _, p := range [][2]int{{0, 0}, {40, 0}, {0, 41}, {40, 41}} {
				c.sprite(p[0], p[1], ruby, pal)
			}
			small := []string{".kk.", "kRrk", "krdk", ".kk."}
			for _, p := range [][2]int{{22, 2}, {22, 42}, {2, 22}, {42, 22}} {
				c.sprite(p[0], p[1], small, pal)
			}
		},
		// 5 frozen (rare): ice with frost on top and icicles hanging below
		func(c *canvas) {
			c.ring(4, func(x, y, d int) color.NRGBA {
				switch {
				case d == 4:
					return hex("2f6fa0")
				case (x*7+y*3)%11 < 3:
					return hex("ffffff")
				}
				return hex("a9e4ff")
			})
			// icicles: varying lengths along the bottom edge
			lens := []int{3, 6, 2, 4, 7, 3, 5, 2, 6, 4, 3, 5}
			for i, l := range lens {
				x := 6 + i*3
				for k := 0; k < l; k++ {
					w := hex("d8f4ff")
					if k == l-1 {
						w = hex("ffffff")
					}
					c.set(x, 44+k-1, w)
					if k < l/2 {
						c.set(x+1, 44+k-1, hex("a9e4ff"))
					}
				}
			}
			// frost spikes on top
			for i := 0; i < 10; i++ {
				x := 7 + i*4
				h := 2 + (i*5)%3
				for k := 0; k < h; k++ {
					c.set(x, 3-k, hex("e8faff"))
				}
			}
			flake := []string{"..w..", "w.w.w", ".www.", "w.w.w", "..w.."}
			fp := map[rune]color.NRGBA{'w': hex("ffffff")}
			c.sprite(0, 0, flake, fp)
			c.sprite(43, 0, flake, fp)
			c.outline(hex("1f4f78"))
		},
		// 6 bone (epic): a ring of bone with skulls on the corners and crossed
		// bones above
		func(c *canvas) {
			c.ring(4, func(x, y, d int) color.NRGBA {
				switch {
				case d == 4:
					return hex("4a4234")
				case (x+y)%6 == 0:
					return hex("b8ad90")
				}
				return hex("e6dcbc")
			})
			skull := []string{
				"..kkkkk..",
				".kwwwwwk.",
				"kwwwwwwwk",
				"kwkkwkkwk",
				"kwkkwkkwk",
				"kwwwkwwwk",
				".kwwwwwk.",
				"..kwkwk..",
				"...kkk...",
			}
			sp := map[rune]color.NRGBA{'w': hex("f0e6c8")}
			for _, p := range [][2]int{{0, 0}, {39, 0}, {0, 39}, {39, 39}} {
				c.sprite(p[0], p[1], skull, sp)
			}
			c.sprite(15, 0, []string{
				"kk..............kk",
				"kwk............kwk",
				".kwkk........kkwk.",
				"...kwwkk..kkwwk...",
				".....kkwwwwkk.....",
				"...kwwkk..kkwwk...",
			}, sp)
		},
		// 7 inferno (epic): an ember ring with flames licking off it
		// (they flicker in the client)
		func(c *canvas) {
			c.ring(3, func(x, y, d int) color.NRGBA {
				v := (x*13 + y*7) % 5
				switch {
				case d == 3 && v < 2:
					return hex("c8203a")
				case d == 3:
					return hex("ff7a1a")
				case v == 0:
					return hex("fff6c8")
				}
				return hex("ffd23f")
			})
			// flames: tongues around the outside, tallest on top
			for i := 0; i < frameS; i++ {
				h := 2 + int(3*math.Abs(math.Sin(float64(i)*0.9))) + (i*7)%3
				for k := 0; k < h; k++ {
					col := hex("ff7a1a")
					switch {
					case k == h-1:
						col = hex("c8203a")
					case k < h/2:
						col = hex("ffd23f")
					}
					c.set(i, 4-k, col) // top
					if i%2 == 0 && k < h-2 {
						c.set(i, 44+k-1, col) // bottom, smaller
					}
					if k < h-1 {
						c.set(4-k, i, col)  // left
						c.set(43+k, i, col) // right
					}
				}
			}
		},
		// 8 diamond (legendary): a faceted crystal frame studded with big
		// cut diamonds (they sparkle in the client)
		func(c *canvas) {
			c.ring(5, func(x, y, d int) color.NRGBA {
				switch {
				case d == 5:
					return hex("1f6f8a")
				case (x+y)%4 == 0:
					return hex("ffffff")
				case (x/2+y/2)%2 == 0:
					return hex("7fe9ff")
				}
				return hex("3fb6d8")
			})
			gem := []string{
				"..kkkkk..",
				".kWwWwWk.",
				"kWwbwbwWk",
				"kbwbBbwbk",
				".kbwbwbk.",
				"..kbwbk..",
				"...kbk...",
				"....k....",
			}
			gp := map[rune]color.NRGBA{'W': hex("ffffff"), 'w': hex("dff8ff"), 'b': hex("7fe9ff"), 'B': hex("3fb6d8")}
			for _, p := range [][2]int{{0, 0}, {39, 0}, {0, 40}, {39, 40}, {19, 0}, {19, 40}, {0, 20}, {39, 20}} {
				c.sprite(p[0], p[1], gem, gp)
			}
		},
		// 9 rainbow (legendary): a wide rainbow band with a soft glow
		// around it (the colours run round in the client)
		func(c *canvas) {
			per := func(x, y int) float64 {
				return (math.Atan2(float64(y)-23.5, float64(x)-23.5) + math.Pi) / (2 * math.Pi)
			}
			c.ring(8, func(x, y, d int) color.NRGBA {
				col := hsvN(per(x, y))
				switch {
				case d <= 4:
					if d == 1 {
						return shadeC(col, 0.7)
					}
					return col
				case d <= 6:
					if (x+y)%2 == 0 {
						return col
					}
					return color.NRGBA{}
				default:
					if (x+y)%4 == 0 {
						return col
					}
					return color.NRGBA{}
				}
			})
		},
		// 10 marathon (unique): a chequered finish line with gold laurels up
		// the sides, crossed flags on top and a winner's medal below
		func(c *canvas) {
			c.ring(4, func(x, y, d int) color.NRGBA {
				if d == 4 {
					return hex("8a5a12")
				}
				if (x/2+y/2)%2 == 0 {
					return hex("f4f4f0")
				}
				return hex("1a1a22")
			})
			leaf := map[rune]color.NRGBA{'g': hex("f2c14e"), 'G': hex("fff2b8"), 'd': hex("b8861f")}
			for i := 0; i < 5; i++ {
				y := 14 + i*6
				c.sprite(0, y, []string{".kk.", "kGgk", "kgdk", ".kk."}, leaf)
				c.sprite(44, y, []string{".kk.", "kGgk", "kgdk", ".kk."}, leaf)
			}
			flag := []string{
				"kkkkkkkk.",
				"kwbwbwbk.",
				"kbwbwbwk.",
				"kwbwbwbk.",
				"kkkkkkkk.",
				"k........",
				"k........",
				"k........",
			}
			fp := map[rune]color.NRGBA{'w': hex("f4f4f0"), 'b': hex("1a1a22")}
			c.sprite(1, 0, flag, fp)
			// mirrored flag on the right
			for j, r := range flag {
				for i, ch := range r {
					if ch == '.' {
						continue
					}
					col := K
					if v, ok := fp[ch]; ok {
						col = v
					}
					c.set(frameS-2-i, j, col)
				}
			}
			c.sprite(19, 39, []string{
				"..kkkkkk..",
				".kGGgggGk.",
				"kGgg11ggdk",
				"kgggk1ggdk",
				"kggggkggdk",
				".kgggggdk.",
				"..kkkkkk..",
			}, map[rune]color.NRGBA{'g': hex("f2c14e"), 'G': hex("fff2b8"), 'd': hex("b8861f"), '1': hex("8a5a12")})
		},
		// 11 vault (unique): a steel vault door with bolts all round, the
		// dial on top and gold bars stacked in the bottom corners (a gleam
		// sweeps across it in the client)
		func(c *canvas) {
			c.bevel(5, hex("dfe4ec"), hex("9aa3b2"), hex("5f6878"), hex("23272f"))
			// bolts around the door
			for i := 0; i < 8; i++ {
				o := 9 + i*4
				for _, p := range [][2]int{{o, 2}, {o, 45}, {2, o}, {45, o}} {
					c.set(p[0], p[1], hex("f4f6fa"))
					c.set(p[0]+1, p[1], hex("6a7282"))
				}
			}
			c.sprite(17, 0, []string{
				"....kkkkkk....",
				"..kkssssssk...",
				".ksskkkkksskk.",
				"kssk.kyyk.kssk",
				"ksk.kyyyyk.ksk",
				"ksk.kyykyk.ksk",
				".ksk.kkkk.ksk.",
			}, map[rune]color.NRGBA{'s': hex("c4c9d4"), 'y': hex("f2c14e")})
			bars := []string{
				"...kkkkkk.",
				"..kGgggdk.",
				".kkkkkkkkk",
				"kGgggdkGgk",
				"kkkkkkkkkk",
			}
			bp := map[rune]color.NRGBA{'g': hex("f2c14e"), 'G': hex("fff2b8"), 'd': hex("b8861f")}
			c.sprite(0, 43, bars, bp)
			c.sprite(38, 43, bars, bp)
			// coins on the sides
			for _, y := range []int{16, 30} {
				c.sprite(0, y, []string{".kk.", "kGgk", "kgdk", ".kk."}, bp)
				c.sprite(44, y, []string{".kk.", "kGgk", "kgdk", ".kk."}, bp)
			}
		},
	}
	img := newImg(frameS*len(styles), frameS)
	for i, st := range styles {
		var c canvas
		st(&c)
		for y := 0; y < frameS; y++ {
			for x := 0; x < frameS; x++ {
				if col := c.px[y][x]; col.A != 0 {
					set(img, i*frameS+x, y, col)
				}
			}
		}
	}
	save("frames.png", img)
}
