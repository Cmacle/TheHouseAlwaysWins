//go:build linux

package x11

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestFakeInput drives a running client with XTEST (manual check: set
// X11_INPUT_TEST=1 and DISPLAY to a server where the client is open).
func TestFakeInput(t *testing.T) {
	if os.Getenv("X11_INPUT_TEST") == "" {
		t.Skip("manual test")
	}
	x, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	xtest, ok := x.queryExtension("XTEST")
	if !ok {
		t.Fatal("no XTEST")
	}
	fake := func(typ, detail byte, rx, ry int) {
		b := make([]byte, 32)
		b[0], b[1] = typ, detail
		le.PutUint32(b[8:], x.root)
		le.PutUint16(b[20:], uint16(rx))
		le.PutUint16(b[22:], uint16(ry))
		x.send(xtest, 2, b)
		x.sync()
		time.Sleep(30 * time.Millisecond)
	}
	per, syms := x.keyboardMapping()
	code := func(r rune) (byte, bool) {
		for i := 0; i+per <= len(syms); i += per {
			if syms[i] == uint32(r) {
				return x.minKeycode + byte(i/per), false
			}
			if per > 1 && syms[i+1] == uint32(r) {
				return x.minKeycode + byte(i/per), true
			}
		}
		t.Fatalf("no key for %q", r)
		return 0, false
	}
	shiftCode, _ := code(0xffe1)
	typeText := func(s string) {
		for _, r := range s {
			c, shift := code(r)
			if shift {
				fake(2, shiftCode, 0, 0)
			}
			fake(2, c, 0, 0)
			fake(3, c, 0, 0)
			if shift {
				fake(3, shiftCode, 0, 0)
			}
		}
	}
	for _, step := range strings.Split(os.Getenv("X11_INPUT_TEST"), ";") {
		f := strings.SplitN(step, ":", 2)
		switch f[0] {
		case "click":
			var px, py int
			for i, n := range strings.Split(f[1], ",") {
				v := 0
				for _, c := range n {
					v = v*10 + int(c-'0')
				}
				if i == 0 {
					px = v
				} else {
					py = v
				}
			}
			fake(6, 0, px, py)
			fake(4, 1, 0, 0)
			fake(5, 1, 0, 0)
		case "type":
			typeText(f[1])
		case "key":
			c, _ := code(map[string]rune{"tab": 0xff09, "enter": 0xff0d, "left": 0xff51, "bs": 0xff08}[f[1]])
			fake(2, c, 0, 0)
			fake(3, c, 0, 0)
		case "resize":
			var wh [2]int
			for i, n := range strings.Split(f[1], ",") {
				for _, c := range n {
					wh[i] = wh[i]*10 + int(c-'0')
				}
			}
			r, err := x.call(15, 0, u32s(x.root)) // QueryTree
			if err != nil {
				t.Fatal(err)
			}
			n := int(le.Uint16(r[16:]))
			for i := 0; i < n; i++ {
				win := le.Uint32(r[32+4*i:])
				body := make([]byte, 16)
				le.PutUint32(body[0:], win)
				le.PutUint16(body[4:], 0x4|0x8)
				le.PutUint32(body[8:], uint32(wh[0]))
				le.PutUint32(body[12:], uint32(wh[1]))
				x.send(12, 0, body) // ConfigureWindow
			}
			x.sync()
		case "wait":
			time.Sleep(500 * time.Millisecond)
		}
	}
}
