//go:build linux

package x11

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"runtime"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/selfupdate"
)

const (
	evKeyPress      = 2
	evKeyRelease    = 3
	evButtonPress   = 4
	evButtonRelease = 5
	evMotion        = 6
	evEnter         = 7
	evLeave         = 8
	evFocusOut      = 10
	evExpose        = 12
	evConfigure     = 22
	evClientMessage = 33
	evMapping       = 34

	eventMask = 1 | 2 | 4 | 8 | 0x10 | 0x20 | 0x40 | 0x8000 | 0x20000 | 0x200000
)

type window struct {
	title      string
	x          *conn
	id, gc     uint32
	w, h       int // window size in pixels
	quit       bool
	fullscreen bool
	dirty      bool // redraw even if the frame didn't change

	atomProtocols, atomDelete, atomState, atomFullscreen uint32

	keyPer  int
	keySyms []uint32

	in client.Input
	p  presenter
}

// Run opens the game window and runs the game until it's closed.
func Run(cfg *client.Config, assets *client.Assets, icon []byte) error {
	return RunApp("The House Always Wins", cfg, icon, func(p client.Platform) *client.App { return client.New(p, cfg, assets) })
}

// Available reports whether there's an X display to open a window on.
func Available() bool {
	x, err := dial()
	if err != nil {
		return false
	}
	x.c.Close()
	return true
}

// RunApp opens a window titled title and runs the app made by mk in it
// until the window closes.
func RunApp(title string, cfg *client.Config, icon []byte, mk func(client.Platform) *client.App) error {
	x, err := dial()
	if err != nil {
		return err
	}
	defer x.c.Close()
	w := &window{x: x, in: client.Input{MX: -1, MY: -1}, title: title}

	scale := cfg.WindowScale
	if scale <= 0 {
		scale = 1
		for s := 2; s <= 8; s++ {
			if client.ScreenW*s <= x.screenW*9/10 && client.ScreenH*s <= x.screenH*85/100 {
				scale = s
			}
		}
	}
	w.w, w.h = client.ScreenW*scale, client.ScreenH*scale
	if err := w.create(icon); err != nil {
		return err
	}
	w.p = presenter{x: x, win: w.id, gc: w.gc}
	w.p.init()
	if cfg.Fullscreen {
		w.toggleFullscreen()
	}

	app := mk(&platform{w: w})
	defer app.Close()

	const frameTime = time.Second / 60
	next := time.Now()
	for !w.quit {
		w.pump()
		if w.quit {
			break
		}
		lw, lh := client.ChooseSize(w.w, w.h)
		app.SetSize(lw, lh)
		in := w.in
		dw, dh, ox, oy := fit(lw, lh, w.w, w.h)
		in.MX = (w.in.MX - ox) * lw / max(1, dw)
		in.MY = (w.in.MY - oy) * lh / max(1, dh)
		in.MouseInside = w.in.MouseInside && in.MX >= 0 && in.MY >= 0 && in.MX < lw && in.MY < lh
		fb := app.Frame(in)
		w.in.Pressed, w.in.Released, w.in.RightPressed = false, false, false
		w.in.Wheel, w.in.Chars, w.in.Keys = 0, nil, nil
		if err := w.p.present(fb, w.w, w.h, w.dirty); err != nil {
			return err
		}
		w.dirty = false
		select {
		case <-x.dead:
			return errors.New("lost the connection to the X server")
		default:
		}
		next = next.Add(frameTime)
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		} else if d < -100*time.Millisecond {
			next = time.Now()
		}
	}
	return nil
}

// fit places a canvas of lw x lh in a window of ww x wh, as big as it goes.
func fit(lw, lh, ww, wh int) (dw, dh, ox, oy int) {
	s := min(float64(ww)/float64(lw), float64(wh)/float64(lh))
	dw = min(ww, max(1, int(float64(lw)*s+0.5)))
	dh = min(wh, max(1, int(float64(lh)*s+0.5)))
	return dw, dh, (ww - dw) / 2, (wh - dh) / 2
}

func (w *window) create(icon []byte) error {
	x := w.x
	w.id = x.newID()
	body := make([]byte, 28+8)
	le.PutUint32(body[0:], w.id)
	le.PutUint32(body[4:], x.root)
	le.PutUint16(body[12:], uint16(w.w))
	le.PutUint16(body[14:], uint16(w.h))
	le.PutUint16(body[18:], 1) // InputOutput
	le.PutUint32(body[24:], 0x2|0x800)
	le.PutUint32(body[28:], 0) // black background
	le.PutUint32(body[32:], eventMask)
	x.send(1, 0, body) // CreateWindow (depth and visual from the parent)

	w.atomProtocols = x.internAtom("WM_PROTOCOLS")
	w.atomDelete = x.internAtom("WM_DELETE_WINDOW")
	w.atomState = x.internAtom("_NET_WM_STATE")
	w.atomFullscreen = x.internAtom("_NET_WM_STATE_FULLSCREEN")
	utf8 := x.internAtom("UTF8_STRING")
	title := w.title
	x.changeProperty(w.id, 39, 31, 8, []byte(title))                                          // WM_NAME, STRING
	x.changeProperty(w.id, x.internAtom("_NET_WM_NAME"), utf8, 8, []byte(title))              // for modern desktops
	x.changeProperty(w.id, 67, 31, 8, []byte("thehousealwayswins\x00TheHouseAlwaysWins\x00")) // WM_CLASS
	x.changeProperty(w.id, w.atomProtocols, 4, 32, u32s(w.atomDelete))                        // ATOM
	hints := make([]uint32, 18)
	hints[0] = 16 // PMinSize
	hints[5], hints[6] = 320, 180
	x.changeProperty(w.id, 40, 41, 32, u32s(hints...)) // WM_NORMAL_HINTS, WM_SIZE_HINTS
	if img, err := png.Decode(bytes.NewReader(icon)); err == nil {
		b := img.Bounds()
		data := []uint32{uint32(b.Dx()), uint32(b.Dy())}
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for xx := b.Min.X; xx < b.Max.X; xx++ {
				r, g, bl, a := img.At(xx, y).RGBA()
				data = append(data, a>>8<<24|r>>8<<16|g>>8<<8|bl>>8)
			}
		}
		x.changeProperty(w.id, x.internAtom("_NET_WM_ICON"), 6, 32, u32s(data...)) // CARDINAL
	}
	w.gc = x.newID()
	x.send(55, 0, u32s(w.gc, w.id, 0)) // CreateGC
	x.send(8, 0, u32s(w.id))           // MapWindow
	w.keyPer, w.keySyms = x.keyboardMapping()
	return x.sync()
}

func (w *window) toggleFullscreen() {
	w.fullscreen = !w.fullscreen
	action := uint32(0)
	if w.fullscreen {
		action = 1
	}
	ev := make([]byte, 32)
	ev[0], ev[1] = 33, 32 // ClientMessage, format 32
	le.PutUint32(ev[4:], w.id)
	le.PutUint32(ev[8:], w.atomState)
	le.PutUint32(ev[12:], action)
	le.PutUint32(ev[16:], w.atomFullscreen)
	le.PutUint32(ev[24:], 1)
	body := append(u32s(w.x.root, 0x80000|0x100000), ev...)
	w.x.send(25, 0, body) // SendEvent
	w.x.flush()
}

// pump handles every event waiting from the X server.
func (w *window) pump() {
	for {
		select {
		case ev := <-w.x.events:
			w.handle(ev)
		default:
			return
		}
	}
}

func (w *window) handle(ev []byte) {
	i16 := func(o int) int { return int(int16(le.Uint16(ev[o:]))) }
	switch ev[0] & 0x7f {
	case evKeyPress:
		w.key(ev[1], le.Uint16(ev[28:]))
	case evButtonPress:
		w.in.MX, w.in.MY = i16(24), i16(26)
		switch ev[1] {
		case 1:
			w.in.Down, w.in.Pressed = true, true
		case 3:
			w.in.RightPressed = true
		case 4:
			w.in.Wheel++
		case 5:
			w.in.Wheel--
		}
	case evButtonRelease:
		w.in.MX, w.in.MY = i16(24), i16(26)
		if ev[1] == 1 {
			w.in.Down, w.in.Released = false, true
		}
	case evMotion:
		w.in.MX, w.in.MY, w.in.MouseInside = i16(24), i16(26), true
	case evEnter:
		w.in.MX, w.in.MY, w.in.MouseInside = i16(24), i16(26), true
	case evLeave:
		w.in.MouseInside = false
	case evFocusOut:
		if w.in.Down {
			w.in.Down, w.in.Released = false, true
		}
		w.in.Shift, w.in.Ctrl = false, false
	case evExpose:
		w.dirty = true
	case evConfigure:
		nw, nh := int(le.Uint16(ev[20:])), int(le.Uint16(ev[22:]))
		if nw > 0 && nh > 0 && (nw != w.w || nh != w.h) {
			w.w, w.h, w.dirty = nw, nh, true
		}
	case evClientMessage:
		if le.Uint32(ev[8:]) == w.atomProtocols && le.Uint32(ev[12:]) == w.atomDelete {
			w.quit = true
		}
	case evMapping:
		if ev[4] == 1 { // keyboard
			w.keyPer, w.keySyms = w.x.keyboardMapping()
		}
	}
}

// ------------------------------------------------------------ platform

type platform struct{ w *window }

func (p *platform) PlaySound(name string, wav []byte, volume float64) { playSound(wav) }
func (p *platform) OpenImageFile() (string, bool)                     { return pickImage() }
func (p *platform) ClipboardText() string                             { return clipboardText() }
func (p *platform) ToggleFullscreen()                                 { p.w.toggleFullscreen() }
func (p *platform) Quit()                                             { p.w.quit = true }

func (p *platform) UpdateAsset() string             { return "TheHouseAlwaysWins-linux-" + runtime.GOARCH }
func (p *platform) UpdateDir() string               { return selfupdate.Dir() }
func (p *platform) InstallUpdate(path string) error { return selfupdate.Replace(path) }

var _ = image.Rect
