package client

import (
	"image"
	"image/color"
	"image/draw"
	"strings"
)

// Virtual screen size. The window scales this up by whole pixels.
const (
	ScreenW = 640
	ScreenH = 360
)

func blit(dst, src *image.RGBA, x, y int) {
	r := image.Rect(x, y, x+src.Rect.Dx(), y+src.Rect.Dy())
	draw.Draw(dst, r, src, src.Rect.Min, draw.Over)
}

func fillRect(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	if c.A == 255 {
		draw.Draw(dst, r, &image.Uniform{c}, image.Point{}, draw.Src)
		return
	}
	draw.Draw(dst, r, &image.Uniform{c}, image.Point{}, draw.Over)
}

func strokeRect(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	fillRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), c)
	fillRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), c)
	fillRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), c)
	fillRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), c)
}

// blitScaled draws src into dr with nearest-neighbour sampling.
func blitScaled(dst, src *image.RGBA, dr image.Rectangle) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := dr.Dx(), dr.Dy()
	if sw == 0 || sh == 0 || dw <= 0 || dh <= 0 {
		return
	}
	clip := dr.Intersect(dst.Rect)
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		sy := src.Rect.Min.Y + (y-dr.Min.Y)*sh/dh
		for x := clip.Min.X; x < clip.Max.X; x++ {
			sx := src.Rect.Min.X + (x-dr.Min.X)*sw/dw
			si := src.PixOffset(sx, sy)
			sa := uint32(src.Pix[si+3])
			if sa == 0 {
				continue
			}
			di := dst.PixOffset(x, y)
			if sa == 255 {
				copy(dst.Pix[di:di+4], src.Pix[si:si+4])
				continue
			}
			inv := 255 - sa
			for k := 0; k < 4; k++ {
				dst.Pix[di+k] = uint8(uint32(src.Pix[si+k]) + uint32(dst.Pix[di+k])*inv/255)
			}
		}
	}
}

// scaleImage returns a resized copy (nearest neighbour for upscales, box
// filter for downscales).
func scaleImage(src *image.RGBA, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if w >= sw {
		blitScaled(out, src, out.Rect)
		return out
	}
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, (y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, (x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var acc [4]uint32
			n := uint32(0)
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					i := src.PixOffset(src.Rect.Min.X+xx, src.Rect.Min.Y+yy)
					for k := 0; k < 4; k++ {
						acc[k] += uint32(src.Pix[i+k])
					}
					n++
				}
			}
			o := out.PixOffset(x, y)
			for k := 0; k < 4; k++ {
				out.Pix[o+k] = uint8(acc[k] / n)
			}
		}
	}
	return out
}

// tile repeats src over dr (pixel art keeps its texture instead of smearing).
func tile(dst, src *image.RGBA, dr image.Rectangle) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if sw == 0 || sh == 0 {
		return
	}
	clip := dr.Intersect(dst.Rect)
	if clip.Empty() {
		return
	}
	sub := dst.SubImage(clip).(*image.RGBA)
	for y := dr.Min.Y; y < dr.Max.Y; y += sh {
		for x := dr.Min.X; x < dr.Max.X; x += sw {
			draw.Draw(sub, image.Rect(x, y, x+sw, y+sh), src, src.Rect.Min, draw.Over)
		}
	}
}

// nineSlice draws a framed sprite over r: b-pixel corners are kept and the
// edges and middle are tiled.
func nineSlice(dst, src *image.RGBA, r image.Rectangle, b int) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if b*2 >= sw || b*2 >= sh || r.Dx() < b*2 || r.Dy() < b*2 {
		blitScaled(dst, src, r)
		return
	}
	sx := []int{0, b, sw - b, sw}
	sy := []int{0, b, sh - b, sh}
	dx := []int{r.Min.X, r.Min.X + b, r.Max.X - b, r.Max.X}
	dy := []int{r.Min.Y, r.Min.Y + b, r.Max.Y - b, r.Max.Y}
	for j := 0; j < 3; j++ {
		for i := 0; i < 3; i++ {
			s := src.SubImage(image.Rect(src.Rect.Min.X+sx[i], src.Rect.Min.Y+sy[j], src.Rect.Min.X+sx[i+1], src.Rect.Min.Y+sy[j+1])).(*image.RGBA)
			tile(dst, s, image.Rect(dx[i], dy[j], dx[i+1], dy[j+1]))
		}
	}
}

// ---------------------------------------------------------------- font

// Font is a bitmap font loaded from font.png.
type Font struct {
	masks []*image.Alpha
	adv   []int
	first rune
	cellH int
}

func newFont(img *image.RGBA, t Theme) *Font {
	cw, ch, cols := t.FontCellWidth, t.FontCellHeight, t.FontColumns
	if cw <= 0 || ch <= 0 || cols <= 0 {
		cw, ch, cols = 6, 8, 16
	}
	f := &Font{first: rune(t.FontFirstChar), cellH: ch}
	rows := img.Rect.Dy() / ch
	n := rows * cols
	for i := 0; i < n; i++ {
		cx, cy := (i%cols)*cw, (i/cols)*ch
		minX, maxX := cw, -1
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				a := img.Pix[img.PixOffset(cx+x, cy+y)+3]
				if a > 0 {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}
		if maxX < 0 || t.FontMonospace {
			minX, maxX = 0, cw-2
		}
		trim := image.NewAlpha(image.Rect(0, 0, maxX-minX+1, ch))
		for y := 0; y < ch; y++ {
			for x := minX; x <= maxX; x++ {
				trim.Pix[trim.PixOffset(x-minX, y)] = img.Pix[img.PixOffset(cx+x, cy+y)+3]
			}
		}
		f.masks = append(f.masks, trim)
		adv := maxX - minX + 2
		if rune(i)+f.first == ' ' {
			adv = t.FontSpaceWidth
			if t.FontMonospace {
				adv = cw
			}
		}
		f.adv = append(f.adv, adv)
	}
	return f
}

func (f *Font) glyph(r rune) int {
	i := int(r - f.first)
	if i < 0 || i >= len(f.masks) {
		i = int('?' - f.first)
		if i < 0 || i >= len(f.masks) {
			return -1
		}
	}
	return i
}

// Width is the pixel width of s at scale 1.
func (f *Font) Width(s string) int {
	w := 0
	for _, r := range s {
		if i := f.glyph(r); i >= 0 {
			w += f.adv[i]
		}
	}
	if w > 0 {
		w--
	}
	return w
}

// Height is the line height.
func (f *Font) Height() int { return f.cellH }

// Draw draws s with its top-left at (x, y).
func (f *Font) Draw(dst *image.RGBA, x, y int, s string, c color.RGBA) {
	u := &image.Uniform{c}
	for _, r := range s {
		i := f.glyph(r)
		if i < 0 {
			continue
		}
		m := f.masks[i]
		rect := image.Rect(x, y, x+m.Rect.Dx(), y+m.Rect.Dy())
		draw.DrawMask(dst, rect, u, image.Point{}, m, image.Point{}, draw.Over)
		x += f.adv[i]
	}
}

// DrawScaled draws s magnified by an integer factor.
func (f *Font) DrawScaled(dst *image.RGBA, x, y int, s string, c color.RGBA, scale int) {
	if scale <= 1 {
		f.Draw(dst, x, y, s, c)
		return
	}
	for _, r := range s {
		i := f.glyph(r)
		if i < 0 {
			continue
		}
		m := f.masks[i]
		for yy := 0; yy < m.Rect.Dy(); yy++ {
			for xx := 0; xx < m.Rect.Dx(); xx++ {
				a := m.Pix[m.PixOffset(xx, yy)]
				if a == 0 {
					continue
				}
				cc := c
				if a < 255 {
					cc = color.RGBA{uint8(uint16(c.R) * uint16(a) / 255), uint8(uint16(c.G) * uint16(a) / 255), uint8(uint16(c.B) * uint16(a) / 255), uint8(uint16(c.A) * uint16(a) / 255)}
				}
				fillRect(dst, image.Rect(x+xx*scale, y+yy*scale, x+(xx+1)*scale, y+(yy+1)*scale), cc)
			}
		}
		x += f.adv[i] * scale
	}
}

// ---------------------------------------------------------------- text helpers

type align int

const (
	alignLeft align = iota
	alignCenter
	alignRight
)

// text draws a string with a 1px drop shadow.
func (a *App) text(x, y int, s string, c color.RGBA, al align) {
	w := a.A.Font.Width(s)
	switch al {
	case alignCenter:
		x -= w / 2
	case alignRight:
		x -= w
	}
	a.A.Font.Draw(a.fb, x+1, y+1, s, a.A.C("shadow"))
	a.A.Font.Draw(a.fb, x, y, s, c)
}

// textBig draws scaled text with a shadow.
func (a *App) textBig(x, y int, s string, c color.RGBA, scale int, al align) {
	w := a.A.Font.Width(s) * scale
	switch al {
	case alignCenter:
		x -= w / 2
	case alignRight:
		x -= w
	}
	a.A.Font.DrawScaled(a.fb, x+scale, y+scale, s, a.A.C("shadow"), scale)
	a.A.Font.DrawScaled(a.fb, x, y, s, c, scale)
}

// wrap splits text into lines no wider than w.
func (a *App) wrap(s string, w int) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, word := range words {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if a.A.Font.Width(try) > w && line != "" {
				lines = append(lines, line)
				line = word
			} else {
				line = try
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func fitText(f *Font, s string, w int) string {
	if f.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && f.Width(string(r)+"..") > w {
		r = r[:len(r)-1]
	}
	return string(r) + ".."
}
