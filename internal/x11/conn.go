//go:build linux

// Package x11 is the game's Linux window: a small X11 client written against
// the X protocol directly (no C libraries), so the Linux build is one static
// file. Wayland desktops run it through XWayland.
package x11

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var le = binary.LittleEndian

// conn is a connection to the X server.
type conn struct {
	c   net.Conn
	w   *bufio.Writer
	seq uint16

	mu      sync.Mutex
	replies map[uint16]chan []byte
	errs    map[uint16]byte // errors for requests someone is waiting on
	events  chan []byte
	dead    chan struct{}
	lastErr []byte // the newest error nobody was waiting for

	// from the connection setup
	ridBase, ridMask uint32
	nextID           uint32
	maxReq           int // bytes
	root             uint32
	rootVisual       uint32
	rootDepth        byte
	bpp              int // bits per pixel of ZPixmap at rootDepth
	msbFirst         bool
	redMask          uint32
	screenW, screenH int
	minKeycode       byte
	maxKeycode       byte
}

// displayAddr works out where the X server is from $DISPLAY.
func displayAddr() (network, addr, number string, err error) {
	d := os.Getenv("DISPLAY")
	if d == "" {
		return "", "", "", errors.New("no DISPLAY set (is a desktop running?)")
	}
	host, rest, ok := strings.Cut(d, ":")
	if !ok {
		return "", "", "", fmt.Errorf("can't understand DISPLAY=%q", d)
	}
	number, _, _ = strings.Cut(rest, ".")
	n, err := strconv.Atoi(number)
	if err != nil {
		return "", "", "", fmt.Errorf("can't understand DISPLAY=%q", d)
	}
	if strings.HasPrefix(host, "/") { // macOS-style socket path
		return "unix", host + ":" + number, number, nil
	}
	if host == "" || host == "unix" {
		return "unix", "/tmp/.X11-unix/X" + number, number, nil
	}
	return "tcp", net.JoinHostPort(host, strconv.Itoa(6000+n)), number, nil
}

// xauth finds the MIT-MAGIC-COOKIE-1 for this display in ~/.Xauthority.
func xauth(number string) (name, data []byte) {
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".Xauthority")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	host, _ := os.Hostname()
	be := binary.BigEndian
	read := func() ([]byte, bool) {
		if len(b) < 2 {
			return nil, false
		}
		n := int(be.Uint16(b))
		if len(b) < 2+n {
			return nil, false
		}
		v := b[2 : 2+n]
		b = b[2+n:]
		return v, true
	}
	for len(b) >= 2 {
		family := be.Uint16(b)
		b = b[2:]
		addr, ok1 := read()
		num, ok2 := read()
		nm, ok3 := read()
		dt, ok4 := read()
		if !(ok1 && ok2 && ok3 && ok4) {
			return nil, nil
		}
		if string(nm) != "MIT-MAGIC-COOKIE-1" {
			continue
		}
		if (family == 256 && string(addr) == host || family == 65535 || family == 0) && (string(num) == number || len(num) == 0) {
			return nm, dt
		}
		if name == nil && string(num) == number {
			name, data = nm, dt // close enough: same display, other host name
		}
	}
	return name, data
}

func pad4(n int) int { return (n + 3) &^ 3 }

func dial() (*conn, error) {
	network, addr, number, err := displayAddr()
	if err != nil {
		return nil, err
	}
	c, err := net.Dial(network, addr)
	if err != nil && network == "unix" {
		c, err = net.Dial("unix", "@"+addr) // abstract socket
	}
	if err != nil {
		return nil, fmt.Errorf("can't connect to the X server at %s: %v", addr, err)
	}
	x := &conn{c: c, w: bufio.NewWriterSize(c, 1<<16), replies: map[uint16]chan []byte{}, errs: map[uint16]byte{},
		events: make(chan []byte, 1024), dead: make(chan struct{})}
	authName, authData := xauth(number)
	setup := make([]byte, 12+pad4(len(authName))+pad4(len(authData)))
	setup[0] = 'l'
	le.PutUint16(setup[2:], 11)
	le.PutUint16(setup[6:], uint16(len(authName)))
	le.PutUint16(setup[8:], uint16(len(authData)))
	copy(setup[12:], authName)
	copy(setup[12+pad4(len(authName)):], authData)
	if _, err := c.Write(setup); err != nil {
		c.Close()
		return nil, err
	}
	head := make([]byte, 8)
	if _, err := io.ReadFull(c, head); err != nil {
		c.Close()
		return nil, err
	}
	body := make([]byte, int(le.Uint16(head[6:]))*4)
	if _, err := io.ReadFull(c, body); err != nil {
		c.Close()
		return nil, err
	}
	if head[0] != 1 {
		c.Close()
		reason := string(body)
		if head[0] == 0 && len(body) >= int(head[1]) {
			reason = string(body[:head[1]])
		}
		return nil, fmt.Errorf("the X server refused the connection: %s", strings.TrimSpace(reason))
	}
	if err := x.parseSetup(body); err != nil {
		c.Close()
		return nil, err
	}
	go x.readLoop()
	return x, nil
}

func (x *conn) parseSetup(b []byte) error {
	if len(b) < 32 {
		return errors.New("short X setup")
	}
	x.ridBase = le.Uint32(b[4:])
	x.ridMask = le.Uint32(b[8:])
	vendorLen := int(le.Uint16(b[16:]))
	x.maxReq = int(le.Uint16(b[18:])) * 4
	numScreens := int(b[20])
	numFormats := int(b[21])
	x.msbFirst = b[22] == 1
	x.minKeycode, x.maxKeycode = b[26], b[27]
	off := 32 + pad4(vendorLen)
	type format struct{ depth, bpp byte }
	var formats []format
	for i := 0; i < numFormats; i++ {
		if off+8 > len(b) {
			return errors.New("short X setup")
		}
		formats = append(formats, format{b[off], b[off+1]})
		off += 8
	}
	if numScreens < 1 || off+40 > len(b) {
		return errors.New("X server has no screens")
	}
	s := b[off:]
	x.root = le.Uint32(s[0:])
	x.screenW, x.screenH = int(le.Uint16(s[20:])), int(le.Uint16(s[22:]))
	x.rootVisual = le.Uint32(s[32:])
	x.rootDepth = s[38]
	numDepths := int(s[39])
	p := 40
	for d := 0; d < numDepths && p+8 <= len(s); d++ {
		depth := s[p]
		nv := int(le.Uint16(s[p+2:]))
		p += 8
		for v := 0; v < nv && p+24 <= len(s); v++ {
			if depth == x.rootDepth && le.Uint32(s[p:]) == x.rootVisual {
				x.redMask = le.Uint32(s[p+8:])
			}
			p += 24
		}
	}
	for _, f := range formats {
		if f.depth == x.rootDepth {
			x.bpp = int(f.bpp)
		}
	}
	if x.bpp != 32 || (x.rootDepth != 24 && x.rootDepth != 32) {
		return fmt.Errorf("unsupported display: %d-bit color at %d bits per pixel (needs 24-bit color)", x.rootDepth, x.bpp)
	}
	return nil
}

// newID allocates a resource id (window, GC, ...).
func (x *conn) newID() uint32 {
	x.nextID++
	return x.ridBase | (x.nextID & x.ridMask)
}

// send queues a request; body excludes the 4-byte header. It returns the
// request's sequence number.
func (x *conn) send(opcode, data byte, body []byte) uint16 {
	n := 4 + len(body)
	h := [4]byte{opcode, data}
	le.PutUint16(h[2:], uint16(pad4(n)/4))
	x.w.Write(h[:])
	x.w.Write(body)
	if p := pad4(n) - n; p > 0 {
		x.w.Write(make([]byte, p))
	}
	x.seq++
	return x.seq
}

// call sends a request that has a reply and waits for it.
func (x *conn) call(opcode, data byte, body []byte) ([]byte, error) {
	ch := make(chan []byte, 1)
	x.mu.Lock()
	seq := x.seq + 1
	x.replies[seq] = ch
	x.mu.Unlock()
	x.send(opcode, data, body)
	if err := x.w.Flush(); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		if r == nil {
			x.mu.Lock()
			code := x.errs[seq]
			delete(x.errs, seq)
			x.mu.Unlock()
			return nil, fmt.Errorf("X error %d", code)
		}
		return r, nil
	case <-x.dead:
		return nil, errors.New("lost the connection to the X server")
	}
}

// sync waits until the server has handled everything sent so far, and
// reports an error any of those requests caused since mark.
func (x *conn) sync() error {
	_, err := x.call(43, 0, nil) // GetInputFocus
	return err
}

func (x *conn) flush() error { return x.w.Flush() }

func (x *conn) readLoop() {
	defer close(x.dead)
	r := bufio.NewReaderSize(x.c, 1<<16)
	for {
		buf := make([]byte, 32)
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		switch buf[0] {
		case 0: // error
			seq := le.Uint16(buf[2:])
			x.mu.Lock()
			ch, waiting := x.replies[seq]
			if waiting {
				delete(x.replies, seq)
				x.errs[seq] = buf[1]
			} else {
				x.lastErr = buf
			}
			x.mu.Unlock()
			if waiting {
				ch <- nil
			}
		case 1: // reply
			if extra := int(le.Uint32(buf[4:])) * 4; extra > 0 {
				more := make([]byte, 32+extra)
				copy(more, buf)
				if _, err := io.ReadFull(r, more[32:]); err != nil {
					return
				}
				buf = more
			}
			seq := le.Uint16(buf[2:])
			x.mu.Lock()
			ch := x.replies[seq]
			delete(x.replies, seq)
			x.mu.Unlock()
			if ch != nil {
				ch <- buf
			}
		default: // event
			if buf[0]&0x7f == 35 { // GenericEvent carries more
				if extra := int(le.Uint32(buf[4:])) * 4; extra > 0 {
					io.CopyN(io.Discard, r, int64(extra))
				}
				continue
			}
			select {
			case x.events <- buf:
			default: // the game is behind; drop it
			}
		}
	}
}

// ------------------------------------------------------------ requests

func (x *conn) internAtom(name string) uint32 {
	body := make([]byte, 4+pad4(len(name)))
	le.PutUint16(body, uint16(len(name)))
	copy(body[4:], name)
	r, err := x.call(16, 0, body)
	if err != nil || len(r) < 12 {
		return 0
	}
	return le.Uint32(r[8:])
}

// changeProperty replaces a window property. format is 8, 16 or 32.
func (x *conn) changeProperty(win, prop, typ uint32, format byte, data []byte) {
	unit := int(format) / 8
	body := make([]byte, 20+pad4(len(data)))
	le.PutUint32(body[0:], win)
	le.PutUint32(body[4:], prop)
	le.PutUint32(body[8:], typ)
	body[12] = format
	le.PutUint32(body[16:], uint32(len(data)/unit))
	copy(body[20:], data)
	x.send(18, 0, body)
}

func u32s(v ...uint32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		le.PutUint32(b[4*i:], x)
	}
	return b
}

func (x *conn) queryExtension(name string) (major byte, ok bool) {
	body := make([]byte, 4+pad4(len(name)))
	le.PutUint16(body, uint16(len(name)))
	copy(body[4:], name)
	r, err := x.call(98, 0, body)
	if err != nil || len(r) < 12 || r[8] == 0 {
		return 0, false
	}
	return r[9], true
}

// keyboardMapping returns the keysyms for every keycode.
func (x *conn) keyboardMapping() (perCode int, syms []uint32) {
	count := int(x.maxKeycode) - int(x.minKeycode) + 1
	r, err := x.call(101, 0, []byte{x.minKeycode, byte(count), 0, 0})
	if err != nil || len(r) < 32 {
		return 0, nil
	}
	perCode = int(r[1])
	for i := 32; i+4 <= len(r); i += 4 {
		syms = append(syms, le.Uint32(r[i:]))
	}
	return perCode, syms
}

// callAsync sends a request with a reply and returns a channel that gets the
// reply (or nil on error) later.
func (x *conn) callAsync(opcode, data byte, body []byte) chan []byte {
	ch := make(chan []byte, 1)
	x.mu.Lock()
	x.replies[x.seq+1] = ch
	x.mu.Unlock()
	x.send(opcode, data, body)
	return ch
}

// takeError reports (and forgets) an error from a request nobody waited on.
func (x *conn) takeError() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	e := x.lastErr != nil
	x.lastErr = nil
	return e
}
