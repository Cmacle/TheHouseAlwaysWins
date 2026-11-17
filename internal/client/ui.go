package client

import (
	"image"
	"strings"
	"unicode"
)

// Key identifies a non-character key press.
type Key int

const (
	KeyNone Key = iota
	KeyEnter
	KeyBackspace
	KeyTab
	KeyEscape
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyDelete
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyF1
	KeyF11
	KeyPaste // Ctrl+V
	KeySpace
)

// Input is one frame of input, in virtual screen coordinates.
type Input struct {
	MX, MY       int
	Down         bool // left button held
	Pressed      bool // left button went down this frame
	Released     bool // left button went up this frame
	RightPressed bool
	Wheel        int // +1 per notch up, -1 per notch down
	Chars        []rune
	Keys         []Key
	Shift, Ctrl  bool
	MouseInside  bool
}

func (in *Input) key(k Key) bool {
	for _, x := range in.Keys {
		if x == k {
			return true
		}
	}
	return false
}

// hotkey reports whether a letter key was typed this frame (case-insensitive).
func (in *Input) hotkey(r rune) bool {
	r = unicode.ToLower(r)
	for _, c := range in.Chars {
		if unicode.ToLower(c) == r {
			return true
		}
	}
	return false
}

type btnStyle int

const (
	styleRed btnStyle = iota
	styleGold
)

type btn struct {
	enabled bool
	style   btnStyle
	hotkey  rune
	key     Key
}

func (a *App) hovered(r image.Rectangle) bool {
	return !a.blocked && a.in.MouseInside && image.Pt(a.in.MX, a.in.MY).In(r)
}

// button draws a button and returns true when it is clicked.
func (a *App) button(id string, r image.Rectangle, label string, o btn) bool {
	hov := a.hovered(r) && o.enabled
	if hov && a.in.Pressed {
		a.activeID = id
	}
	held := a.activeID == id && a.in.Down
	clicked := false
	if a.in.Released && a.activeID == id {
		if hov {
			clicked = true
		}
	}
	if o.enabled && !a.blocked && a.focusID == "" {
		if o.hotkey != 0 && a.in.hotkey(o.hotkey) {
			clicked = true
		}
	}
	if o.enabled && !a.blocked && o.key != KeyNone && a.in.key(o.key) {
		clicked = true
	}
	var spr = a.A.Button
	switch {
	case !o.enabled:
		spr = a.A.ButtonDisabled
	case held && hov:
		spr = a.A.ButtonDown
	case o.style == styleGold && hov:
		spr = a.A.ButtonGoldHover
	case o.style == styleGold:
		spr = a.A.ButtonGold
	case hov:
		spr = a.A.ButtonHover
	}
	nineSlice(a.fb, spr, r, a.A.Theme.NineSliceEdge)
	tc := a.A.C("button_txt")
	if !o.enabled {
		tc = a.A.C("text_dim")
	}
	off := 0
	if held && hov {
		off = 1
	}
	fh := a.A.Font.Height() - 1
	a.text(r.Min.X+r.Dx()/2, r.Min.Y+(r.Dy()-fh)/2+off, label, tc, alignCenter)
	if clicked {
		a.sound("click")
	}
	return clicked
}

// textField is a single-line text box. Returns true when Enter is pressed in it.
func (a *App) textField(id string, r image.Rectangle, value *string, masked bool, maxLen int, placeholder string, allowed func(rune) bool) bool {
	a.focusOrder = append(a.focusOrder, id)
	if a.in.Pressed && !a.blocked {
		if a.hovered(r) {
			a.focusID = id
		} else if a.focusID == id {
			a.focusID = ""
		}
	}
	focused := a.focusID == id && !a.blocked
	enter := false
	if focused {
		for _, c := range a.in.Chars {
			if c < 32 || c > 126 {
				continue
			}
			if allowed != nil && !allowed(c) {
				continue
			}
			if len(*value) < maxLen {
				*value += string(c)
			}
		}
		if a.in.key(KeyPaste) {
			for _, c := range a.P.ClipboardText() {
				if c >= 32 && c <= 126 && (allowed == nil || allowed(c)) && len(*value) < maxLen {
					*value += string(c)
				}
			}
		}
		if a.in.key(KeyBackspace) && len(*value) > 0 {
			if a.in.Ctrl {
				*value = ""
			} else {
				*value = (*value)[:len(*value)-1]
			}
		}
		if a.in.key(KeyEnter) {
			enter = true
		}
	}
	spr := a.A.Field
	if focused {
		spr = a.A.FieldFocus
	}
	nineSlice(a.fb, spr, r, a.A.Theme.NineSliceEdge)
	shown := *value
	if masked {
		shown = strings.Repeat("*", len(*value))
	}
	ty := r.Min.Y + (r.Dy()-a.A.Font.Height()+1)/2
	maxW := r.Dx() - 10
	for a.A.Font.Width(shown) > maxW && len(shown) > 0 {
		shown = shown[1:]
	}
	if shown == "" && !focused {
		a.A.Font.Draw(a.fb, r.Min.X+5, ty, placeholder, a.A.C("text_dim"))
	} else {
		a.A.Font.Draw(a.fb, r.Min.X+5, ty, shown, a.A.C("text"))
	}
	if focused && (a.frame/30)%2 == 0 {
		cx := r.Min.X + 5 + a.A.Font.Width(shown) + 1
		fillRect(a.fb, image.Rect(cx, ty-1, cx+1, ty+a.A.Font.Height()-1), a.A.C("gold"))
	}
	return enter
}

// checkbox toggles *v when clicked.
func (a *App) checkbox(id string, x, y int, label string, v *bool) {
	w := 9 + 4 + a.A.Font.Width(label)
	r := image.Rect(x, y, x+w, y+9)
	if a.hovered(r) && a.in.Pressed {
		*v = !*v
		a.sound("click")
	}
	idx := 0
	if *v {
		idx = 1
	}
	sub := a.A.Checkbox.SubImage(image.Rect(idx*9, 0, idx*9+9, 9)).(*image.RGBA)
	blit(a.fb, sub, x, y)
	c := a.A.C("text")
	if a.hovered(r) {
		c = a.A.C("gold")
	}
	a.text(x+13, y+1, label, c, alignLeft)
}

// panel draws a framed panel.
func (a *App) panel(r image.Rectangle) {
	nineSlice(a.fb, a.A.Panel, r, a.A.Theme.PanelEdge)
}

// handleTab moves focus between text fields.
func (a *App) handleTab() {
	if !a.in.key(KeyTab) || len(a.focusOrder) == 0 {
		return
	}
	idx := -1
	for i, id := range a.focusOrder {
		if id == a.focusID {
			idx = i
		}
	}
	if a.in.Shift {
		idx--
		if idx < 0 {
			idx = len(a.focusOrder) - 1
		}
	} else {
		idx = (idx + 1) % len(a.focusOrder)
	}
	a.focusID = a.focusOrder[idx]
}
