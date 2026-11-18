//go:build linux

package x11

import (
	"bytes"
	"image"
)

// presenter scales the game's canvas up to the window and sends it to the X
// server: through shared memory when the server allows it (fast), or as
// plain image requests otherwise (works over ssh too).
type presenter struct {
	x       *conn
	win, gc uint32

	shm    bool
	shmExt byte
	seg    uint32
	shmID  int
	mem    []byte
	wait   chan []byte // reply that says the server is done reading mem

	buf        []byte
	xmap, ymap []int
	mapKey     [4]int
	last       []byte // the previous frame, to skip unchanged ones
	lastW      int
	lastH      int
	cleared    bool
}

func (p *presenter) init() {
	if major, ok := p.x.queryExtension("MIT-SHM"); ok {
		p.shmExt = major
		p.shm = true
	}
}

// ensureShm makes sure the shared segment holds at least size bytes.
func (p *presenter) ensureShm(size int) bool {
	if !p.shm {
		return false
	}
	if len(p.mem) >= size {
		return true
	}
	p.dropShm()
	id, mem, err := shmCreate(size)
	if err != nil {
		p.shm = false
		return false
	}
	seg := p.x.newID()
	body := make([]byte, 12)
	le.PutUint32(body[0:], seg)
	le.PutUint32(body[4:], uint32(id))
	body[8] = 1                 // read-only for the server
	p.x.send(p.shmExt, 1, body) // ShmAttach
	if err := p.x.sync(); err != nil || p.x.takeError() {
		shmDetach(mem)
		shmRemove(id)
		p.shm = false // e.g. a remote X server: fall back
		return false
	}
	shmRemove(id) // freed automatically once both sides detach
	p.seg, p.shmID, p.mem = seg, id, mem
	return true
}

func (p *presenter) dropShm() {
	if p.mem == nil {
		return
	}
	p.waitDone()
	p.x.send(p.shmExt, 2, u32s(p.seg)) // ShmDetach
	shmDetach(p.mem)
	p.mem = nil
}

func (p *presenter) waitDone() {
	if p.wait != nil {
		select {
		case <-p.wait:
		case <-p.x.dead:
		}
		p.wait = nil
	}
}

func (p *presenter) present(fb *image.RGBA, ww, wh int, force bool) error {
	fw, fh := fb.Rect.Dx(), fb.Rect.Dy()
	if !force && ww == p.lastW && wh == p.lastH && bytes.Equal(fb.Pix, p.last) {
		return nil // nothing changed
	}
	p.last = append(p.last[:0], fb.Pix...)
	sizeChanged := ww != p.lastW || wh != p.lastH
	p.lastW, p.lastH = ww, wh
	dw, dh, ox, oy := fit(fw, fh, ww, wh)
	if sizeChanged || force {
		// black bars (if any) around the picture
		p.x.send(61, 0, append(u32s(p.win), make([]byte, 8)...)) // ClearArea, whole window
	}
	if key := [4]int{fw, fh, dw, dh}; key != p.mapKey {
		p.mapKey = key
		p.xmap, p.ymap = make([]int, dw), make([]int, dh)
		for i := range p.xmap {
			p.xmap[i] = i * fw / dw * 4
		}
		for i := range p.ymap {
			p.ymap[i] = i * fh / dh
		}
	}
	size := dw * dh * 4
	useShm := p.ensureShm(size)
	var out []byte
	if useShm {
		p.waitDone() // don't scribble on what the server is still reading
		out = p.mem[:size]
	} else {
		if cap(p.buf) < size {
			p.buf = make([]byte, size)
		}
		out = p.buf[:size]
	}
	p.scale(fb, out, dw, dh)
	if useShm {
		body := make([]byte, 36)
		le.PutUint32(body[0:], p.win)
		le.PutUint32(body[4:], p.gc)
		le.PutUint16(body[8:], uint16(dw))
		le.PutUint16(body[10:], uint16(dh))
		le.PutUint16(body[16:], uint16(dw))
		le.PutUint16(body[18:], uint16(dh))
		le.PutUint16(body[20:], uint16(ox))
		le.PutUint16(body[22:], uint16(oy))
		body[24] = p.x.rootDepth
		body[25] = 2 // ZPixmap
		le.PutUint32(body[28:], p.seg)
		p.x.send(p.shmExt, 3, body) // ShmPutImage
		// a cheap request with a reply, to know when the server's done
		p.wait = p.x.callAsync(43, 0, nil)
		return p.x.flush()
	}
	// plain PutImage, in strips that fit the server's request limit
	rowBytes := dw * 4
	rows := max(1, (p.x.maxReq-28)/rowBytes)
	for y := 0; y < dh; y += rows {
		n := min(rows, dh-y)
		body := make([]byte, 20, 20+n*rowBytes)
		le.PutUint32(body[0:], p.win)
		le.PutUint32(body[4:], p.gc)
		le.PutUint16(body[8:], uint16(dw))
		le.PutUint16(body[10:], uint16(n))
		le.PutUint16(body[12:], uint16(ox))
		le.PutUint16(body[14:], uint16(oy+y))
		body[17] = p.x.rootDepth
		body = append(body, out[y*rowBytes:(y+n)*rowBytes]...)
		p.x.send(72, 2, body) // PutImage, ZPixmap
	}
	return p.x.flush()
}

// scale copies the canvas into out (dw x dh, the server's pixel format),
// nearest-neighbour.
func (p *presenter) scale(fb *image.RGBA, out []byte, dw, dh int) {
	// byte positions of red, green and blue in the server's 32-bit pixel
	ri, gi, bi := 2, 1, 0
	if p.x.redMask == 0xff {
		ri, bi = 0, 2
	}
	if p.x.msbFirst {
		ri, gi, bi = 3-ri, 3-gi, 3-bi
	}
	pi := ri ^ gi ^ bi // the unused byte (0^1^2^3 is 0)
	row := dw * 4
	prev := -1
	for y := 0; y < dh; y++ {
		dst := out[y*row : (y+1)*row]
		sy := p.ymap[y]
		if sy == prev {
			copy(dst, out[(y-1)*row:y*row])
			continue
		}
		prev = sy
		src := fb.Pix[sy*fb.Stride:]
		o := 0
		for _, sx := range p.xmap {
			dst[o+ri], dst[o+gi], dst[o+bi], dst[o+pi] = src[sx], src[sx+1], src[sx+2], 0
			o += 4
		}
	}
}
