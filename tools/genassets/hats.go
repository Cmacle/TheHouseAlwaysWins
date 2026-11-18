// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package main

import (
	"image/color"
	"math"
)

// Hat sprites: each cell is 32x20. The bottom 6 rows overlap the top of the
// 32x32 avatar. Order must match the Sprite indexes in internal/cosmetics.

const hatW, hatH = 32, 20

type sprite struct {
	rows []string
	pal  map[rune]string
}

var hatSprites = []sprite{
	// 0 party hat
	{[]string{
		".....w.....",
		"....www....",
		".....k.....",
		"....kpk....",
		"....kyk....",
		"...kpppk...",
		"...kyyyk...",
		"..kpppppk..",
		"..kyyyyyk..",
		".kpppppppk.",
		".kyyyyyyyk.",
		"kpppppppppk",
		"kkkkkkkkkkk",
	}, map[rune]string{'p': "e83a7a", 'y': "ffd23f", 'w': "ffffff"}},
	// 1 ball cap
	{[]string{
		".....kkkkkk.......",
		"...kkbbbbbbkk.....",
		"..kbbbbwbbbbbk....",
		".kbbbbbbbbbbbbk...",
		".kbbbbbbbbbbbbbk..",
		"kbbbbbbbbbbbbbbkkk",
		"kddddddddddddddddk",
		".kkkkkkkkkkkkkkkk.",
	}, map[rune]string{'b': "2f6fd6", 'd': "1d4a99", 'w': "ffffff"}},
	// 2 beanie
	{[]string{
		".......kkk.......",
		"......kwwwk......",
		".......kkk.......",
		"....kkrrrrrkk....",
		"...krrrrrrrrrk...",
		"..krrrrrrrrrrrk..",
		"..kRRRRRRRRRRRk..",
		"..kRrRrRrRrRrRk..",
		"..kkkkkkkkkkkkk..",
	}, map[rune]string{'r': "c8323c", 'R': "8e1f28", 'w': "f4f4f0"}},
	// 3 propeller beanie
	{[]string{
		"yyyyyy.k.yyyyyy",
		".yyyy.kak.yyyy.",
		"......kak......",
		"....kkrgbkk....",
		"...krrrgbbbk...",
		"..krrrrgbbbbk..",
		"..krrrrgbbbbk..",
		"..kyyyyyyyyyk..",
		"..kkkkkkkkkkk..",
	}, map[rune]string{'y': "ffd23f", 'a': "bbbbbb", 'r': "e04848", 'g': "3fb68b", 'b': "3a7bff"}},
	// 4 cowboy hat
	{[]string{
		"........kkkkkkkk........",
		".......kttttkttttk......",
		"......kttttttttttttk....",
		"......kttttttttttttk....",
		"......kBBBBBBBBBBBBk....",
		"kk....kttttttttttttk..kk",
		"ktkkkkkttttttttttttkkktk",
		".kttttttttttttttttttttk.",
		"..kkkkkkkkkkkkkkkkkkkk..",
	}, map[rune]string{'t': "a0703a", 'B': "3a2414"}},
	// 5 tin foil hat
	{[]string{
		"......k......",
		".....kwk.....",
		"....kssgk....",
		"...kgsswsk...",
		"...ksgssgk...",
		"..ksswsgssk..",
		"..kgssgswgk..",
		".ksgswssgssk.",
		".kgssgsswsgk.",
		"kkkkkkkkkkkkk",
	}, map[rune]string{'s': "b8bcc8", 'g': "7d828f", 'w': "f4f6fb"}},
	// 6 sombrero
	{[]string{
		"..........kkkkkkkk..........",
		".........kyyyyyyyyk.........",
		"........kyyyyyyyyyyk........",
		"........kyyyyyyyyyyk........",
		"........krgrgrgrgrgk........",
		".kkkkkkkkyyyyyyyyyyykkkkkkk.",
		"kyyyyyyyyyyyyyyyyyyyyyyyyyyk",
		"kyrgyrgyrgyrgyrgyrgyrgyrgyyk",
		".kkkkkkkkkkkkkkkkkkkkkkkkkk.",
	}, map[rune]string{'y': "e8c25a", 'r': "c8323c", 'g': "2e9d4f"}},
	// 7 traffic cone
	{[]string{
		"......kk......",
		".....kook.....",
		".....kwwk.....",
		"....kwwwwk....",
		"....kooook....",
		"...kooooook...",
		"...kwwwwwwk...",
		"..kwwwwwwwwk..",
		"..koooooooook.",
		".kkkkkkkkkkkkk",
		"kooooooooooook",
		"kkkkkkkkkkkkkk",
	}, map[rune]string{'o': "ff7a1a", 'w': "f4f4f0"}},
	// 8 top hat
	{[]string{
		"...kkkkkkkkkk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kKhKKKKKKk...",
		"...kRRRRRRRRk...",
		"...kKKKKKKKKk...",
		"kkkkKKKKKKKKkkkk",
		"kKKKKKKKKKKKKKKk",
		".kkkkkkkkkkkkkk.",
	}, map[rune]string{'K': "2a2a33", 'h': "4f4f60", 'R': "c62a3d"}},
	// 9 fez
	{[]string{
		"....kkkkkkk..",
		"...krrrrrrrkk",
		"...krrrrrrrkY",
		"..krrrrrrrrkY",
		"..krrrrrrrrkY",
		".krrrrrrrrrkY",
		".krrrrrrrrrk.",
		"kRRRRRRRRRRRk",
		"kkkkkkkkkkkkk",
	}, map[rune]string{'r': "c8203a", 'R': "8e1726", 'Y': "ffd23f"}},
	// 10 pirate hat
	{[]string{
		".....kkkkkkkk.....",
		"...kkKKKKKKKKkk...",
		"..kKKKKKwwKKKKKk..",
		".kKKKKKwwwwKKKKKk.",
		"kKKKKKKKwwKKKKKKKk",
		"kKKKKKKwKKwKKKKKKk",
		"kyyyyyyyyyyyyyyyyk",
		".kkkkkkkkkkkkkkkk.",
	}, map[rune]string{'K': "1d1d26", 'w': "f4f4f0", 'y': "d9a227"}},
	// 11 dunce cap
	{[]string{
		"......k......",
		".....kwk.....",
		".....kwk.....",
		"....kwwwk....",
		"....kwwwk....",
		"...kwkkwwk...",
		"...kwkwkwk...",
		"..kwwkwkwwk..",
		"..kwwkkwwwk..",
		".kwwwwwwwwwk.",
		".kbbbbbbbbbk.",
		"kkkkkkkkkkkkk",
	}, map[rune]string{'w': "f4f4f0", 'b': "3a7bff"}},
	// 12 viking helmet
	{[]string{
		"w..............w",
		"wk............kw",
		"wwk...kkkk...kww",
		".wwk.kssssk.kww.",
		"..wwksshsssskww.",
		"...ksshssssssk..",
		"..kssshsssssssk.",
		"..kyyyyyyyyyyyk.",
		"..kyryyryyryyrk.",
		"..kkkkkkkkkkkkk.",
	}, map[rune]string{'w': "f0e6c8", 's': "9aa0ab", 'h': "d8dde6", 'y': "c8a040", 'r': "8a5a12"}},
	// 13 halo (floats)
	{[]string{
		"....kkkkkkkk....",
		"..kkyyyyyyyykk..",
		".kyyk......kyyk.",
		".kyyk......kyyk.",
		"..kkyyyyyyyykk..",
		"....kkkkkkkk....",
		"................",
		"................",
		"................",
		"................",
		"................",
	}, map[rune]string{'y': "ffe066"}},
	// 14 devil horns
	{[]string{
		"k........................k",
		"rk......................kr",
		"rrk....................krr",
		"rrrk..................krrr",
		".rRrk................krRr.",
		".rRRrk..............krRRr.",
		"..rRRrk............krRRr..",
		"...rRRk............kRRr...",
	}, map[rune]string{'r': "e0283c", 'R': "8e1726"}},
	// 15 wizard hat
	{[]string{
		"...........kk....",
		"..........kbbk...",
		".........kbbk....",
		"........kbbbk....",
		".......kbbybk....",
		"......kbbbbbk....",
		"......kbybbbk....",
		".....kbbbbbybk...",
		"....kbbbbbbbbk...",
		"...kbbybbbbbbbk..",
		"...kyyyyyyyyyyk..",
		".kkbbbbbbbbbbbbkk",
		"kbbbbbbbbbbbbbbbk",
		".kkkkkkkkkkkkkkk.",
	}, map[rune]string{'b': "3b3fb8", 'y': "ffd23f"}},
	// 16 unicorn horn
	{[]string{
		".....k.....",
		"....kwk....",
		"....kpk....",
		"...kwwpk...",
		"...kppwk...",
		"..kwwppwk..",
		"..kppwwpk..",
		".kwwppwwpk.",
		".kppwwppwk.",
		"kkkkkkkkkkk",
	}, map[rune]string{'w': "fff6fb", 'p': "f59ad9"}},
	// 17 graduation cap
	{[]string{
		".......kk.......",
		".....kkKKkk.....",
		"...kkKKKKKKkk...",
		".kkKKKKKKKKKKkk.",
		"kKKKKKKyKKKKKKKk",
		".kkKKKKyKKKKKkk.",
		"...kkKKyKKKkk...",
		"....kKKyKKKk....",
		"....kKKyKKKkY...",
		"....kKKKKKKkY...",
		"....kkkkkkkkY...",
	}, map[rune]string{'K': "23232f", 'y': "ffd23f", 'Y': "ffd23f"}},
	// 18 golden top hat
	{[]string{
		"...kkkkkkkkkk...",
		"...kGwGGGGGDk...",
		"...kGwGGGGGDk...",
		"...kGwGGGGGDk...",
		"...kGwGGGGGDk...",
		"...kGGGGGGGDk...",
		"...kRRrRRrRRk...",
		"...kGGGGGGGDk...",
		"kkkkGGGGGGGGkkkk",
		"kGGGGGGGGGGGGGDk",
		".kkkkkkkkkkkkkk.",
	}, map[rune]string{'G': "f2c14e", 'D': "a8741a", 'w': "fff2b8", 'R': "c62a3d", 'r': "7fe9ff"}},
	// 19 diamond tiara
	{[]string{
		".........c.........",
		"........cWc........",
		"...c.....c.....c...",
		"..cWc...kSk...cWc..",
		"...c...kSSSk...c...",
		"...k..kSScSSk..k...",
		"..kSkkSSSSSSSkkSk..",
		".kSSSSSSSSSSSSSSSk.",
		".kSpSSScSSSpSSScSk.",
		"..kkkkkkkkkkkkkkk..",
	}, map[rune]string{'c': "7fe9ff", 'W': "ffffff", 'S': "d8dde6", 'p': "ff5c8a"}},
	// 20 hair on fire
	{[]string{
		"....r.....r.......",
		"...ro....ror...r..",
		"...roo..rooo..ror.",
		"..rooy.roooy.rooo.",
		"..royyrooyyyrooyy.",
		".royyyyoyyyyyoyyyo",
		".ryyyyyyWyyyyyyyyr",
		"royyyWyyyyyyWyyyor",
		"rrooooooooooooooor",
	}, map[rune]string{'r': "c8203a", 'o': "ff7a1a", 'y': "ffd23f", 'W': "fff6c8"}},
	// 21 space helmet
	{[]string{
		"...........R...........",
		"...........k...........",
		"........kkkkkkk........",
		".....kkkcccccccckkk....",
		"...kkcccWWcccccccccckk.",
		"..kcccWWcccccccccccccck",
		".kcccWccccccccccccccccc",
		".kccccccccccccccccccccc",
		"ksssssssssssssssssssssk",
		"kskkskkskkskkskkskkssk.",
	}, map[rune]string{'R': "ff3030", 'c': "9fd8ffa0", 'W': "ffffffd0", 's': "c8ccd6"}},
	// 22 crown
	{[]string{
		"....R.......R.......R....",
		"...kyk.....kyk.....kyk...",
		"...kyyk...kyyyk...kyyk...",
		"..kyyyyk.kyyyyyk.kyyyyk..",
		"..kyyyyykyyyyyyykyyyyyk..",
		"..kyyyyyyyyyyyyyyyyyyyk..",
		"..kyyByyyyyRyyyyyyByyyk..",
		"..kyBBByyyRRRyyyyBBByyk..",
		"..kyyByyyyyRyyyyyyByyyk..",
		"..kYYYYYYYYYYYYYYYYYYYk..",
		"..kkkkkkkkkkkkkkkkkkkkk..",
	}, map[rune]string{'y': "f2c14e", 'Y': "b07a18", 'R': "e0283c", 'B': "3a7bff"}},
	// 23 laurel wreath (gold)
	{[]string{
		".gg....................gg.",
		"gGgg..................ggGg",
		".gGgg................ggGg.",
		"..ggGg..............gGgg..",
		"...gggGg..........gGggg...",
		".....ggggg......ggggg.....",
		".......gggg....gggg.......",
	}, map[rune]string{'g': "f2c14e", 'G': "a8741a"}},
}

func genHats() {
	cols := 8
	rows := (len(hatSprites) + cols - 1) / cols
	img := newImg(cols*hatW, rows*hatH)
	for i, sp := range hatSprites {
		ox, oy := (i%cols)*hatW, (i/cols)*hatH
		w := 0
		for _, r := range sp.rows {
			if len(r) > w {
				w = len(r)
			}
		}
		x0 := ox + (hatW-w)/2
		y0 := oy + hatH - len(sp.rows)
		for j, r := range sp.rows {
			for k, ch := range r {
				var c color.NRGBA
				switch ch {
				case '.':
					continue
				case 'k':
					c = hex("15151a")
				default:
					h, ok := sp.pal[ch]
					if !ok {
						continue
					}
					c = hex(h)
				}
				set(img, x0+k, y0+j, c)
			}
		}
	}
	save("hats.png", img)
}

// Frames: 36x36 cells, a 2px ring around the 32x32 avatar.
const frameS = 36

func genFrames() {
	type style func(x, y, d int, t float64) (color.NRGBA, bool)
	ring := func(x, y int) (int, bool) {
		d := x
		if y < d {
			d = y
		}
		if frameS-1-x < d {
			d = frameS - 1 - x
		}
		if frameS-1-y < d {
			d = frameS - 1 - y
		}
		return d, d < 2
	}
	corner := func(x, y int) bool {
		return (x < 4 || x > frameS-5) && (y < 4 || y > frameS-5)
	}
	per := func(x, y int) float64 { // 0..1 around the perimeter
		return (math.Atan2(float64(y)-17.5, float64(x)-17.5) + math.Pi) / (2 * math.Pi)
	}
	hsv := func(h float64) color.NRGBA {
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
	styles := []style{
		// 0 wood
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if d == 0 {
				return hex("2b170b"), true
			}
			if (x+y*3)%7 == 0 {
				return hex("8a5a30"), true
			}
			return hex("6b4220"), true
		},
		// 1 felt
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if d == 0 {
				return hex("0e3f24"), true
			}
			if corner(x, y) {
				return hex("f2c14e"), true
			}
			return hex("1f7d48"), true
		},
		// 2 silver
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if d == 0 {
				return hex("5a5f6b"), true
			}
			if (x+y)%9 < 2 {
				return hex("ffffff"), true
			}
			return hex("c4c9d4"), true
		},
		// 3 gold with studs
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if corner(x, y) {
				return hex("fff2b8"), true
			}
			if d == 0 {
				return hex("8a5a12"), true
			}
			if (x+y)%8 < 2 {
				return hex("ffe9a3"), true
			}
			return hex("f2c14e"), true
		},
		// 4 ruby
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if corner(x, y) {
				if (x+y)%2 == 0 {
					return hex("ff6b7d"), true
				}
				return hex("e0283c"), true
			}
			if d == 0 {
				return hex("4a0a14"), true
			}
			return hex("9c1f33"), true
		},
		// 5 ice
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if d == 0 {
				return hex("4a8fc2"), true
			}
			if (x*7+y*3)%11 < 3 {
				return hex("ffffff"), true
			}
			return hex("a9e4ff"), true
		},
		// 6 bone
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if corner(x, y) {
				if (x == 1 || x == frameS-2) && (y == 1 || y == frameS-2) {
					return hex("15151a"), true
				}
				return hex("f0e6c8"), true
			}
			if d == 0 {
				return hex("6a604c"), true
			}
			if (x+y)%6 == 0 {
				return hex("b8ad90"), true
			}
			return hex("e6dcbc"), true
		},
		// 7 inferno
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			v := (x*13 + y*7) % 5
			switch {
			case d == 0 && v < 2:
				return hex("c8203a"), true
			case d == 0:
				return hex("ff7a1a"), true
			case v == 0:
				return hex("fff6c8"), true
			}
			return hex("ffd23f"), true
		},
		// 8 diamond
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			if (x+y)%4 == 0 {
				return hex("ffffff"), true
			}
			if (x/2+y/2)%2 == 0 {
				return hex("7fe9ff"), true
			}
			return hex("3fb6d8"), true
		},
		// 9 rainbow (animated in the client)
		func(x, y, d int, t float64) (color.NRGBA, bool) {
			c := hsv(t)
			if d == 0 {
				c = shadeC(c, 0.7)
			}
			return c, true
		},
	}
	img := newImg(frameS*len(styles), frameS)
	for i, st := range styles {
		for y := 0; y < frameS; y++ {
			for x := 0; x < frameS; x++ {
				d, on := ring(x, y)
				if !on {
					continue
				}
				if c, ok := st(x, y, d, per(x, y)); ok {
					set(img, i*frameS+x, y, c)
				}
			}
		}
	}
	save("frames.png", img)
}
