//go:build linux

package x11

import (
	"unicode"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
)

var specialKeys = map[uint32]client.Key{
	0xff08: client.KeyBackspace,
	0xff09: client.KeyTab,
	0xfe20: client.KeyTab, // ISO_Left_Tab (shift+tab)
	0xff0d: client.KeyEnter,
	0xff8d: client.KeyEnter, // keypad enter
	0xff1b: client.KeyEscape,
	0xffff: client.KeyDelete,
	0xff9f: client.KeyDelete,
	0xff50: client.KeyHome,
	0xff95: client.KeyHome,
	0xff51: client.KeyLeft,
	0xff96: client.KeyLeft,
	0xff52: client.KeyUp,
	0xff97: client.KeyUp,
	0xff53: client.KeyRight,
	0xff98: client.KeyRight,
	0xff54: client.KeyDown,
	0xff99: client.KeyDown,
	0xff55: client.KeyPageUp,
	0xff9a: client.KeyPageUp,
	0xff56: client.KeyPageDown,
	0xff9b: client.KeyPageDown,
	0xff57: client.KeyEnd,
	0xff9c: client.KeyEnd,
	0xffbe: client.KeyF1,
	0xffc8: client.KeyF11,
}

// keypad keys that type characters when Num Lock is on
var keypadChars = map[uint32]rune{
	0xffb0: '0', 0xffb1: '1', 0xffb2: '2', 0xffb3: '3', 0xffb4: '4',
	0xffb5: '5', 0xffb6: '6', 0xffb7: '7', 0xffb8: '8', 0xffb9: '9',
	0xffae: '.', 0xffab: '+', 0xffad: '-', 0xffaa: '*', 0xffaf: '/',
}

// keysym is what a key means with the current modifiers.
func (w *window) keysym(code byte, state uint16) uint32 {
	if w.keyPer == 0 || code < w.x.minKeycode {
		return 0
	}
	i := (int(code) - int(w.x.minKeycode)) * w.keyPer
	if i >= len(w.keySyms) {
		return 0
	}
	lower := w.keySyms[i]
	upper := uint32(0)
	if w.keyPer > 1 {
		upper = w.keySyms[i+1]
	}
	if upper == 0 {
		upper = lower
		if lower < 0x100 && unicode.IsLetter(rune(lower)) {
			upper = uint32(unicode.ToUpper(rune(lower)))
		}
	}
	shift := state&1 != 0
	caps := state&2 != 0
	// keypad with Num Lock: the second keysym is the digit
	if state&0x10 != 0 && upper >= 0xffaa && upper <= 0xffb9 {
		if shift {
			return lower
		}
		return upper
	}
	letter := lower < 0x100 && unicode.IsLetter(rune(lower))
	if shift != (caps && letter) {
		return upper
	}
	return lower
}

func (w *window) key(code byte, state uint16) {
	sym := w.keysym(code, state)
	ctrl := state&4 != 0
	alt := state&8 != 0
	w.in.Shift, w.in.Ctrl = state&1 != 0, ctrl
	if k, ok := specialKeys[sym]; ok {
		if k == client.KeyEnter && alt {
			k = client.KeyF11 // Alt+Enter: fullscreen, like Windows
		}
		w.in.Keys = append(w.in.Keys, k)
		return
	}
	if ctrl {
		if sym == 'v' || sym == 'V' {
			w.in.Keys = append(w.in.Keys, client.KeyPaste)
		}
		return
	}
	var r rune
	switch {
	case sym == ' ':
		w.in.Keys = append(w.in.Keys, client.KeySpace)
		r = ' '
	case sym >= 0x20 && sym <= 0x7e, sym >= 0xa0 && sym <= 0xff:
		r = rune(sym)
	case sym >= 0x01000000 && sym <= 0x0110ffff:
		r = rune(sym - 0x01000000)
	default:
		r = keypadChars[sym]
	}
	if r != 0 {
		w.in.Chars = append(w.in.Chars, r)
	}
}
