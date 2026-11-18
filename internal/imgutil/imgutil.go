// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package imgutil holds small image helpers shared by client and server.
package imgutil

import "image"

// Downsample center-crops src to a square and box-filters it to size x size.
func Downsample(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	ox := b.Min.X + (b.Dx()-side)/2
	oy := b.Min.Y + (b.Dy()-side)/2
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		y0 := oy + y*side/size
		y1 := oy + (y+1)*side/size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < size; x++ {
			x0 := ox + x*side/size
			x1 := ox + (x+1)*side/size
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r += uint64(cr)
					g += uint64(cg)
					bl += uint64(cb)
					a += uint64(ca)
					n++
				}
			}
			if a == 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(r * 255 / a)
			dst.Pix[i+1] = uint8(g * 255 / a)
			dst.Pix[i+2] = uint8(bl * 255 / a)
			dst.Pix[i+3] = uint8(a / n >> 8)
		}
	}
	return dst
}
