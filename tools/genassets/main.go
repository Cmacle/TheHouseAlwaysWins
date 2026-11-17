// Command genassets generates the default pixel-art assets and sounds that are
// embedded in the client. Every generated file can be replaced by dropping a
// file with the same name into the client's assets folder.
//
//	go run ./tools/genassets
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

var outDir string

func main() {
	flag.StringVar(&outDir, "out", "internal/client/defaultassets", "output directory")
	flag.Parse()
	must(os.MkdirAll(outDir, 0o755))
	genFont()
	genCards()
	genCardBack()
	genChips()
	genTable()
	genUI()
	genAvatar()
	genDealers()
	genShoe()
	genLogo()
	genIcon()
	genHats()
	genFrames()
	genSounds()
	fmt.Println("assets written to", outDir)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---------------------------------------------------------------- helpers

func hex(s string) color.NRGBA {
	s = strings.TrimPrefix(s, "#")
	var r, g, b, a uint8 = 0, 0, 0, 255
	if len(s) == 8 {
		fmt.Sscanf(s, "%02x%02x%02x%02x", &r, &g, &b, &a)
	} else {
		fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	}
	return color.NRGBA{r, g, b, a}
}

func newImg(w, h int) *image.NRGBA { return image.NewNRGBA(image.Rect(0, 0, w, h)) }

func set(img *image.NRGBA, x, y int, c color.NRGBA) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.SetNRGBA(x, y, c)
	}
}

func fill(img *image.NRGBA, x, y, w, h int, c color.NRGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			set(img, xx, yy, c)
		}
	}
}

func rectOutline(img *image.NRGBA, x, y, w, h int, c color.NRGBA) {
	for i := x; i < x+w; i++ {
		set(img, i, y, c)
		set(img, i, y+h-1, c)
	}
	for j := y; j < y+h; j++ {
		set(img, x, j, c)
		set(img, x+w-1, j, c)
	}
}

func bitmap(img *image.NRGBA, x, y int, rows []string, c color.NRGBA, flip bool) {
	h := len(rows)
	for j, row := range rows {
		w := len(row)
		for i, ch := range row {
			if ch == '#' {
				if flip {
					set(img, x+w-1-i, y+h-1-j, c)
				} else {
					set(img, x+i, y+j, c)
				}
			}
		}
	}
}

func save(name string, img image.Image) {
	var buf bytes.Buffer
	must(png.Encode(&buf, img))
	must(os.WriteFile(filepath.Join(outDir, name), buf.Bytes(), 0o644))
}

func textWidth(s string) int {
	w := 0
	for _, r := range s {
		w += glyphAdvance(r)
	}
	if w > 0 {
		w--
	}
	return w
}

func glyphAdvance(r rune) int {
	g, ok := glyphs[r]
	if !ok {
		return 6
	}
	if r == ' ' {
		return 4
	}
	maxc := 0
	for _, row := range g {
		for i, ch := range row {
			if ch == '#' && i+1 > maxc {
				maxc = i + 1
			}
		}
	}
	minc := 5
	for _, row := range g {
		for i, ch := range row {
			if ch == '#' && i < minc {
				minc = i
			}
		}
	}
	return maxc - minc + 1
}

func glyphMin(r rune) int {
	g := glyphs[r]
	minc := 5
	for _, row := range g {
		for i, ch := range row {
			if ch == '#' && i < minc {
				minc = i
			}
		}
	}
	if minc == 5 {
		return 0
	}
	return minc
}

// drawText draws proportional text at scale s.
func drawText(img *image.NRGBA, x, y int, s string, scale int, c color.NRGBA) {
	for _, r := range s {
		g, ok := glyphs[r]
		if !ok {
			x += 6 * scale
			continue
		}
		off := glyphMin(r)
		for j, row := range g {
			for i, ch := range row {
				if ch == '#' {
					fill(img, x+(i-off)*scale, y+j*scale, scale, scale, c)
				}
			}
		}
		x += glyphAdvance(r) * scale
	}
}

// ---------------------------------------------------------------- font

func genFont() {
	img := newImg(16*6, 6*8)
	white := hex("ffffff")
	for r := rune(32); r < 127; r++ {
		g, ok := glyphs[r]
		if !ok {
			continue
		}
		i := int(r - 32)
		cx, cy := (i%16)*6, (i/16)*8
		for j, row := range g {
			for k, ch := range row {
				if ch == '#' {
					set(img, cx+k, cy+j, white)
				}
			}
		}
	}
	save("font.png", img)
}

// ---------------------------------------------------------------- cards

const cardW, cardH = 24, 32

var smallSuits = [4][]string{
	{"..#..", ".###.", "#####", "#####", "..#.."}, // spades
	{".#.#.", "#####", "#####", ".###.", "..#.."}, // hearts
	{"..#..", ".###.", "#####", ".###.", "..#.."}, // diamonds
	{".###.", ".###.", "#####", "#.#.#", "..#.."}, // clubs
}

var bigSuits = [4][]string{
	{"....#....", "...###...", "..#####..", ".#######.", "#########", "#########", ".##.#.##.", "....#....", "...###..."},
	{".##...##.", "####.####", "#########", "#########", ".#######.", "..#####..", "...###...", "....#....", "........."},
	{"....#....", "...###...", "..#####..", ".#######.", "#########", ".#######.", "..#####..", "...###...", "....#...."},
	{"...###...", "..#####..", "..#####..", ".##.#.##.", "#########", "#########", ".##.#.##.", "....#....", "...###..."},
}

var faceIcons = map[int][]string{
	11: {"..#...#..", ".###.###.", "#########", ".#######.", "..#####..", "..#.#.#..", "..#####..", "...###...", "........."},
	12: {"....#....", "...#.#...", "#.#...#.#", "#########", ".#######.", ".#.#.#.#.", ".#######.", "..#####..", "........."},
	13: {"#...#...#", "##.###.##", "#########", "#########", ".#######.", ".#.#.#.#.", ".#######.", "..#####..", "........."},
}

var rankNames = []string{"", "A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}

func cardBase(img *image.NRGBA, ox, oy int, face color.NRGBA) {
	outline := hex("1b1b24")
	shade := hex("c9c0ac")
	for y := 0; y < cardH; y++ {
		for x := 0; x < cardW; x++ {
			corner := (x == 0 || x == cardW-1) && (y == 0 || y == cardH-1)
			if corner {
				continue
			}
			edge := x == 0 || y == 0 || x == cardW-1 || y == cardH-1
			if edge {
				set(img, ox+x, oy+y, outline)
			} else {
				set(img, ox+x, oy+y, face)
			}
		}
	}
	// soft inner corners + bottom shade
	set(img, ox+1, oy+1, outline)
	set(img, ox+cardW-2, oy+1, outline)
	set(img, ox+1, oy+cardH-2, outline)
	set(img, ox+cardW-2, oy+cardH-2, outline)
	for x := 2; x < cardW-2; x++ {
		set(img, ox+x, oy+cardH-2, shade)
	}
	for y := 2; y < cardH-2; y++ {
		set(img, ox+cardW-2, oy+y, shade)
	}
}

func drawRank(img *image.NRGBA, x, y int, rank string, c color.NRGBA, flip bool) {
	// rank text in the 5x7 font; "10" is drawn condensed
	var rows [7]string
	if rank == "10" {
		rows = [7]string{
			".#.###.",
			"##.#.#.",
			".#.#.#.",
			".#.#.#.",
			".#.#.#.",
			".#.#.#.",
			".#.###.",
		}
	} else {
		rows = glyphs[rune(rank[0])]
	}
	bitmap(img, x, y, rows[:], c, flip)
}

func genCards() {
	img := newImg(13*cardW, 4*cardH)
	face := hex("f7f2e4")
	red := hex("c62a3d")
	black := hex("23232f")
	gold := hex("d9a227")
	goldDark := hex("8a5a12")
	for s := 0; s < 4; s++ {
		col := black
		if s == 1 || s == 2 {
			col = red
		}
		for r := 1; r <= 13; r++ {
			ox, oy := (r-1)*cardW, s*cardH
			cardBase(img, ox, oy, face)
			rn := rankNames[r]
			drawRank(img, ox+2, oy+2, rn, col, false)
			bitmap(img, ox+2, oy+10, smallSuits[s], col, false)
			// mirrored index bottom-right
			rw := 5
			if rn == "10" {
				rw = 7
			}
			drawRank(img, ox+cardW-3-rw+1, oy+cardH-3-7+1-0, rn, col, true)
			switch {
			case r >= 11:
				// gold framed court card
				fx, fy := ox+7, oy+9
				fill(img, fx, fy, 11, 15, hex("f3e2b3"))
				rectOutline(img, fx, fy, 11, 15, goldDark)
				bitmap(img, fx+1, fy+1, faceIcons[r], gold, false)
				bitmap(img, fx+3, fy+9, smallSuits[s], col, false)
			case r == 1:
				bitmap(img, ox+7, oy+12, bigSuits[s], col, false)
				// little flourish ring
				for _, p := range [][2]int{{6, 11}, {16, 11}, {6, 21}, {16, 21}} {
					set(img, ox+p[0], oy+p[1], gold)
				}
			default:
				bitmap(img, ox+7, oy+12, bigSuits[s], col, false)
			}
		}
	}
	save("cards.png", img)
}

func genCardBack() {
	img := newImg(cardW, cardH)
	cardBase(img, 0, 0, hex("9c1f33"))
	light := hex("c23a4d")
	dark := hex("6e1222")
	white := hex("f7f2e4")
	for y := 3; y < cardH-3; y++ {
		for x := 3; x < cardW-3; x++ {
			if (x+y)%4 == 0 || (x-y+64)%4 == 0 {
				set(img, x, y, light)
			}
		}
	}
	rectOutline(img, 2, 2, cardW-4, cardH-4, white)
	rectOutline(img, 3, 3, cardW-6, cardH-6, dark)
	bitmap(img, 8, 12, bigSuits[2], hex("f2c14e"), false)
	save("card_back.png", img)
}

// ---------------------------------------------------------------- chips

var chipColors = []string{"c8323c", "2f6fd6", "2e9d4f", "26262e", "7a3fc1", "e08a1e"} // 5,10,25,100,500,1000

func genChips() {
	const n = 20
	img := newImg(n*len(chipColors), n)
	for i, cs := range chipColors {
		base := hex(cs)
		dark := shadeC(base, 0.6)
		lite := shadeC(base, 1.35)
		ox := i * n
		cx, cy := 9.5, 9.5
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				dx, dy := float64(x)-cx, float64(y)-cy
				d := math.Hypot(dx, dy)
				if d > 9.6 {
					continue
				}
				c := base
				ang := math.Atan2(dy, dx)
				switch {
				case d > 8.6:
					c = hex("15151a")
				case d > 6.4:
					// edge spots
					seg := int(math.Floor((ang + math.Pi) / (2 * math.Pi) * 8))
					if seg%2 == 0 {
						c = hex("f2efe6")
					} else {
						c = base
					}
				case d > 5.6:
					c = dark
				default:
					c = base
					if dy < -2 && d < 5 {
						c = lite
					}
				}
				set(img, ox+x, y, c)
			}
		}
	}
	save("chips.png", img)
}

func shadeC(c color.NRGBA, f float64) color.NRGBA {
	cl := func(v float64) uint8 {
		if v > 255 {
			return 255
		}
		if v < 0 {
			return 0
		}
		return uint8(v)
	}
	return color.NRGBA{cl(float64(c.R) * f), cl(float64(c.G) * f), cl(float64(c.B) * f), c.A}
}

// ---------------------------------------------------------------- table

// 4x4 Bayer matrix for ordered dithering.
var bayer = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

func genTable() {
	const W, H = 640, 360
	img := newImg(W, H)
	rng := rand.New(rand.NewSource(7))
	feltLevels := []color.NRGBA{hex("0e3f24"), hex("13502e"), hex("176238"), hex("1b7242"), hex("1f7d48")}
	floor := []color.NRGBA{hex("1a0f0a"), hex("24150d"), hex("2d1a10")}
	railDark, rail, railLite, railHi := hex("2b170b"), hex("5a3418"), hex("7b4a24"), hex("a06a38")
	ecx, ecy := 320.0, 40.0
	rx, ry := 420.0, 222.0
	railW := 11.0
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			dx, dy := (float64(x)-ecx)/rx, (float64(y)-ecy)/ry
			e := math.Sqrt(dx*dx + dy*dy) // 1.0 at the inner rail edge
			th := bayer[y%4][x%4] / 16
			var c color.NRGBA
			outer := 1 + railW/ry
			switch {
			case e < 1:
				// felt: brighter in the middle top, darker toward rail
				fx := (float64(x) - 320) / 380
				fy := (float64(y) - 120) / 260
				v := 1 - (fx*fx + fy*fy*1.2)
				v = v*4.2 + (rng.Float64()-0.5)*0.35
				lvl := int(math.Floor(v + th))
				if lvl < 0 {
					lvl = 0
				}
				if lvl >= len(feltLevels) {
					lvl = len(feltLevels) - 1
				}
				c = feltLevels[lvl]
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
				v := float64(y)/H*2 + (rng.Float64()-0.5)*0.4
				lvl := int(math.Floor(v + th))
				if lvl < 0 {
					lvl = 0
				}
				if lvl >= len(floor) {
					lvl = len(floor) - 1
				}
				c = floor[lvl]
				// carpet pattern
				if (x/8+y/8)%2 == 0 && (x+y)%8 == 0 {
					c = shadeC(c, 1.4)
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	// felt decoration: two thin arcs for the "insurance" style text band
	line := hex("3aa363")
	for _, r := range []struct{ rx, ry float64 }{{250, 100}, {262, 112}} {
		for a := 0.18; a < math.Pi-0.18; a += 0.0015 {
			x := int(math.Round(320 + r.rx*math.Cos(a)))
			y := int(math.Round(30 + r.ry*math.Sin(a)))
			set(img, x, y, line)
		}
	}
	// dealer chip tray
	tx, ty := 248, 2
	fill(img, tx, ty, 144, 16, hex("0b0b0f"))
	rectOutline(img, tx, ty, 144, 16, hex("6b6b78"))
	trayColors := []string{"f2efe6", "c8323c", "2f6fd6", "2e9d4f", "26262e", "7a3fc1", "e08a1e", "f2efe6"}
	for i, cs := range trayColors {
		c := hex(cs)
		x0 := tx + 3 + i*17
		for k := 0; k < 4; k++ {
			fill(img, x0+k*4, ty+3, 3, 10, c)
			set(img, x0+k*4, ty+3, shadeC(c, 1.3))
			for yy := ty + 4; yy < ty+13; yy += 2 {
				set(img, x0+k*4+1, yy, shadeC(c, 0.7))
			}
		}
	}
	// discard holder (left)
	fill(img, 62, 22, 26, 34, hex("0b0b0f"))
	rectOutline(img, 62, 22, 26, 34, hex("6b6b78"))
	save("table.png", img)
}

// ---------------------------------------------------------------- UI

func nine(name string, size int, face, top, bottom, border color.NRGBA, inset bool) {
	img := newImg(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			corner := (x == 0 || x == size-1) && (y == 0 || y == size-1)
			if corner {
				continue
			}
			c := face
			switch {
			case x == 0 || y == 0 || x == size-1 || y == size-1:
				c = border
			case y == 1 || x == 1:
				if inset {
					c = bottom
				} else {
					c = top
				}
			case y == size-2 || x == size-2:
				if inset {
					c = top
				} else {
					c = bottom
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	save(name, img)
}

func genUI() {
	nine("button.png", 12, hex("8e2a2a"), hex("c4574a"), hex("5a1616"), hex("1a0a0a"), false)
	nine("button_hover.png", 12, hex("a83a33"), hex("e0776a"), hex("6c1d1d"), hex("1a0a0a"), false)
	nine("button_down.png", 12, hex("6e1f1f"), hex("c4574a"), hex("3e0f0f"), hex("1a0a0a"), true)
	nine("button_disabled.png", 12, hex("4a4a52"), hex("62626c"), hex("33333a"), hex("15151a"), false)
	nine("button_gold.png", 12, hex("c08a1e"), hex("f2c14e"), hex("7a5410"), hex("2a1a05"), false)
	nine("button_gold_hover.png", 12, hex("d9a227"), hex("ffe08a"), hex("8a6214"), hex("2a1a05"), false)
	nine("field.png", 12, hex("0c140f"), hex("2a3a30"), hex("050806"), hex("3b5a46"), true)
	nine("field_focus.png", 12, hex("0f1a13"), hex("2a3a30"), hex("050806"), hex("f2c14e"), true)
	// panel: dark wood with gold trim
	const ps = 16
	p := newImg(ps, ps)
	wood := []color.NRGBA{hex("2a170c"), hex("321c0f"), hex("3a2112")}
	for y := 0; y < ps; y++ {
		for x := 0; x < ps; x++ {
			corner := (x == 0 || x == ps-1) && (y == 0 || y == ps-1)
			if corner {
				continue
			}
			c := wood[(y/2+x/7)%3]
			switch {
			case x == 0 || y == 0 || x == ps-1 || y == ps-1:
				c = hex("0d0704")
			case x == 1 || y == 1 || x == ps-2 || y == ps-2:
				c = hex("b0832c")
			case x == 2 || y == 2 || x == ps-3 || y == ps-3:
				c = hex("5c3b10")
			}
			p.SetNRGBA(x, y, c)
		}
	}
	save("panel.png", p)
	// checkbox 9x9: unchecked / checked
	cb := newImg(18, 9)
	for k := 0; k < 2; k++ {
		fill(cb, k*9, 0, 9, 9, hex("0c140f"))
		rectOutline(cb, k*9, 0, 9, 9, hex("3b5a46"))
	}
	bitmap(cb, 9+2, 2, []string{"....#", "...##", "#.##.", "###..", ".#..."}, hex("f2c14e"), false)
	save("checkbox.png", cb)
	// turn arrow 7x5
	ar := newImg(7, 5)
	bitmap(ar, 0, 0, []string{"#######", ".#####.", "..###..", "...#...", "......."}, hex("ffe066"), false)
	save("arrow.png", ar)
}

// ---------------------------------------------------------------- avatars

func genAvatar() {
	img := newImg(32, 32)
	bg := hex("2d6e64")
	fill(img, 0, 0, 32, 32, bg)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if (x+y)%6 == 0 {
				set(img, x, y, hex("357a6f"))
			}
		}
	}
	body := hex("c9d6d2")
	shadow := hex("9aaba6")
	// head
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := float64(x)-15.5, float64(y)-12
			if dx*dx+dy*dy < 36 {
				set(img, x, y, body)
			}
			ex, ey := (float64(x)-15.5)/11, (float64(y)-31)/9
			if ex*ex+ey*ey < 1 {
				set(img, x, y, body)
				if x > 20 {
					set(img, x, y, shadow)
				}
			}
		}
	}
	save("default_avatar.png", img)
}

func genShoe() {
	img := newImg(30, 22)
	fill(img, 0, 4, 30, 18, hex("15151a"))
	rectOutline(img, 0, 4, 30, 18, hex("5a5a66"))
	// visible card stack
	for i := 0; i < 6; i++ {
		fill(img, 3+i, 1+i/2, 18, 3, hex("9c1f33"))
		set(img, 3+i, 1+i/2, hex("f7f2e4"))
	}
	fill(img, 2, 8, 26, 2, hex("2a2a33"))
	fill(img, 4, 14, 22, 5, hex("0b0b0f"))
	save("shoe.png", img)
}

func genLogo() {
	lines := []string{"THE HOUSE", "ALWAYS WINS"}
	scale := 4
	w := 0
	for _, l := range lines {
		if tw := textWidth(l) * scale; tw > w {
			w = tw
		}
	}
	h := len(lines)*(8*scale+4) + 4
	pad := 6
	img := newImg(w+pad*2, h+pad*2)
	gold := []color.NRGBA{hex("ffe9a3"), hex("f2c14e"), hex("d18e1f"), hex("9a5a10")}
	for li, l := range lines {
		tw := textWidth(l) * scale
		x := pad + (w-tw)/2
		y := pad + li*(8*scale+4)
		// shadow + outline
		for _, o := range [][2]int{{2, 3}, {3, 3}, {3, 4}} {
			drawText(img, x+o[0], y+o[1], l, scale, hex("000000"))
		}
		for _, o := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, 1}, {-1, 1}, {1, -1}} {
			drawText(img, x+o[0], y+o[1], l, scale, hex("3a1c05"))
		}
		tmp := newImg(img.Bounds().Dx(), img.Bounds().Dy())
		drawText(tmp, x, y, l, scale, hex("ffffff"))
		for yy := 0; yy < tmp.Bounds().Dy(); yy++ {
			for xx := 0; xx < tmp.Bounds().Dx(); xx++ {
				if tmp.NRGBAAt(xx, yy).A == 0 {
					continue
				}
				band := (yy - y) * len(gold) / (7 * scale)
				if band < 0 {
					band = 0
				}
				if band >= len(gold) {
					band = len(gold) - 1
				}
				c := gold[band]
				if (yy-y)%scale == 0 && band == 0 {
					c = hex("fff6d6")
				}
				img.SetNRGBA(xx, yy, c)
			}
		}
	}
	save("logo.png", img)
}

func genIcon() {
	img := newImg(32, 32)
	gold, goldD, edge := hex("f2c14e"), hex("a8741a"), hex("2a1a05")
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			d := math.Hypot(float64(x)-15.5, float64(y)-15.5)
			ang := math.Atan2(float64(y)-15.5, float64(x)-15.5)
			switch {
			case d > 15.6:
			case d > 14.4:
				set(img, x, y, edge)
			case d > 11:
				if int(math.Floor((ang+math.Pi)/(2*math.Pi)*12))%2 == 0 {
					set(img, x, y, hex("fff6d6"))
				} else {
					set(img, x, y, gold)
				}
			case d > 10:
				set(img, x, y, goldD)
			default:
				set(img, x, y, gold)
			}
		}
	}
	// spade, 2x scaled 9x9 pip, centered
	for j, row := range bigSuits[0] {
		for i, ch := range row {
			if ch == '#' {
				fill(img, 7+i*2-1, 7+j*2-1, 2, 2, hex("15151a"))
			}
		}
	}
	save("icon.png", img)
}

// ---------------------------------------------------------------- sounds

const sr = 22050

func writeWav(name string, samples []float64) {
	var buf bytes.Buffer
	n := len(samples)
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+n*2))
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint32(sr))
	binary.Write(&buf, binary.LittleEndian, uint32(sr*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(n*2))
	for _, s := range samples {
		if s > 1 {
			s = 1
		}
		if s < -1 {
			s = -1
		}
		binary.Write(&buf, binary.LittleEndian, int16(s*32000))
	}
	must(os.WriteFile(filepath.Join(outDir, name), buf.Bytes(), 0o644))
}

func silence(sec float64) []float64 { return make([]float64, int(sec*sr)) }

func tone(freq, sec, vol float64, wave string, decay float64) []float64 {
	n := int(sec * sr)
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / sr
		ph := math.Mod(t*freq, 1)
		var v float64
		switch wave {
		case "square":
			if ph < 0.5 {
				v = 1
			} else {
				v = -1
			}
			v *= 0.5
		case "tri":
			v = 4*math.Abs(ph-0.5) - 1
		default:
			v = math.Sin(2 * math.Pi * ph)
		}
		env := math.Exp(-t * decay)
		attack := math.Min(1, float64(i)/(0.004*sr))
		release := math.Min(1, float64(n-i)/(0.01*sr))
		out[i] = v * vol * env * attack * release
	}
	return out
}

func mix(a, b []float64, at float64) []float64 {
	off := int(at * sr)
	if len(a) < off+len(b) {
		a = append(a, make([]float64, off+len(b)-len(a))...)
	}
	for i, v := range b {
		a[off+i] += v
	}
	return a
}

func noise(sec, vol, decay float64, seed int64, lp float64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	n := int(sec * sr)
	out := make([]float64, n)
	prev := 0.0
	for i := range out {
		t := float64(i) / sr
		v := rng.Float64()*2 - 1
		prev = prev + lp*(v-prev)
		out[i] = prev * vol * math.Exp(-t*decay)
	}
	return out
}

func genSounds() {
	writeWav("card.wav", noise(0.07, 0.9, 60, 1, 0.45))
	chip := mix(tone(2600, 0.05, 0.35, "sin", 90), tone(3900, 0.04, 0.2, "sin", 110), 0)
	chip = mix(chip, mix(tone(2300, 0.05, 0.3, "sin", 90), tone(3500, 0.04, 0.18, "sin", 110), 0), 0.055)
	writeWav("chip.wav", chip)
	writeWav("click.wav", mix(tone(1800, 0.02, 0.25, "square", 200), silence(0.01), 0.02))
	notes := func(freqs []float64, step, length float64, wave string, vol float64) []float64 {
		out := []float64{}
		for i, f := range freqs {
			out = mix(out, tone(f, length, vol, wave, 6), float64(i)*step)
		}
		return out
	}
	writeWav("win.wav", notes([]float64{523.25, 659.25, 783.99, 1046.5}, 0.08, 0.22, "square", 0.22))
	writeWav("blackjack.wav", notes([]float64{523.25, 659.25, 783.99, 1046.5, 1318.5, 1568.0, 2093.0}, 0.06, 0.3, "square", 0.2))
	writeWav("lose.wav", notes([]float64{392.0, 329.63, 261.63, 196.0}, 0.14, 0.25, "tri", 0.45))
	writeWav("push.wav", notes([]float64{440, 440}, 0.12, 0.12, "tri", 0.4))
	writeWav("bust.wav", mix(tone(150, 0.35, 0.4, "square", 5), tone(110, 0.35, 0.3, "square", 5), 0.12))
	writeWav("turn.wav", mix(tone(880, 0.4, 0.35, "sin", 8), tone(1320, 0.4, 0.2, "sin", 10), 0))
	writeWav("refill.wav", notes([]float64{1318.5, 1568.0, 1975.5, 2637.0, 1975.5, 2637.0}, 0.05, 0.12, "square", 0.15))
	sh := []float64{}
	for i := 0; i < 8; i++ {
		sh = mix(sh, noise(0.05, 0.6, 50, int64(i+3), 0.5), float64(i)*0.045)
	}
	writeWav("shuffle.wav", sh)
	talk := []float64{}
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 5; i++ {
		f := 180 + rng.Float64()*120
		talk = mix(talk, tone(f, 0.05, 0.25, "square", 25), float64(i)*0.055)
	}
	writeWav("talk.wav", talk)
	writeWav("error.wav", mix(tone(220, 0.12, 0.3, "square", 8), tone(180, 0.15, 0.3, "square", 8), 0.1))
}
