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

type fieldState struct {
	cursor int // insertion point, 0..len(value)
	scroll int // first visible character
	last   int // value length last frame (detects outside changes)
}

func (a *App) field(id string) *fieldState {
	if a.fields == nil {
		a.fields = map[string]*fieldState{}
	}
	f := a.fields[id]
	if f == nil {
		f = &fieldState{cursor: -1}
		a.fields[id] = f
	}
	return f
}

func isWordChar(c byte) bool {
	return c != ' ' && c != '.' && c != ':' && c != '/' && c != '_' && c != '-' && c != '@'
}

// textField is a single-line text box with a movable cursor. Returns true
// when Enter is pressed in it.
func (a *App) textField(id string, r image.Rectangle, value *string, masked bool, maxLen int, placeholder string, allowed func(rune) bool) bool {
	a.focusOrder = append(a.focusOrder, id)
	f := a.field(id)
	v := *value
	// value changed from outside (cleared, loaded, tidied): put cursor at end
	if f.cursor < 0 || len(v) != f.last && a.focusID != id {
		f.cursor = len(v)
	}
	if f.cursor > len(v) {
		f.cursor = len(v)
	}
	shownOf := func(s string) string {
		if masked {
			return strings.Repeat("*", len(s))
		}
		return s
	}
	font := a.A.Font
	tx := r.Min.X + 5
	maxW := r.Dx() - 10

	if a.in.Pressed && !a.blocked {
		if a.hovered(r) {
			if a.focusID != id {
				a.focusID = id
			}
			a.kbAsk = true
			// place the cursor at the nearest character boundary
			shown := shownOf(v)
			if f.scroll > len(shown) {
				f.scroll = 0
			}
			x := tx
			pos := len(v)
			for i := f.scroll; i < len(shown); i++ {
				w := font.Width(shown[i:i+1]) + 1
				if a.in.MX < x+w/2 {
					pos = i
					break
				}
				x += w
			}
			f.cursor = pos
			a.caretAt = a.frame
		} else if a.focusID == id {
			a.focusID = ""
		}
	}
	focused := a.focusID == id && !a.blocked
	if focused {
		a.focusRect = r
	}
	enter := false
	if focused {
		moved := false
		insert := func(text string) {
			for _, c := range text {
				if c < 32 || c > 126 || (allowed != nil && !allowed(c)) || len(v) >= maxLen {
					continue
				}
				v = v[:f.cursor] + string(c) + v[f.cursor:]
				f.cursor++
			}
			moved = true
		}
		insert(string(a.in.Chars))
		for _, k := range a.in.Keys {
			switch k {
			case KeyPaste:
				insert(a.P.ClipboardText())
			case KeyLeft:
				if a.in.Ctrl {
					for f.cursor > 0 && !isWordChar(v[f.cursor-1]) {
						f.cursor--
					}
					for f.cursor > 0 && isWordChar(v[f.cursor-1]) {
						f.cursor--
					}
				} else if f.cursor > 0 {
					f.cursor--
				}
				moved = true
			case KeyRight:
				if a.in.Ctrl {
					for f.cursor < len(v) && isWordChar(v[f.cursor]) {
						f.cursor++
					}
					for f.cursor < len(v) && !isWordChar(v[f.cursor]) {
						f.cursor++
					}
				} else if f.cursor < len(v) {
					f.cursor++
				}
				moved = true
			case KeyHome:
				f.cursor = 0
				moved = true
			case KeyEnd:
				f.cursor = len(v)
				moved = true
			case KeyBackspace:
				if a.in.Ctrl {
					v = v[f.cursor:]
					f.cursor = 0
				} else if f.cursor > 0 {
					v = v[:f.cursor-1] + v[f.cursor:]
					f.cursor--
				}
				moved = true
			case KeyDelete:
				if a.in.Ctrl {
					v = v[:f.cursor]
				} else if f.cursor < len(v) {
					v = v[:f.cursor] + v[f.cursor+1:]
				}
				moved = true
			case KeyEnter:
				enter = true
			}
		}
		if moved {
			a.caretAt = a.frame
		}
		*value = v
	}
	f.last = len(v)

	spr := a.A.Field
	if focused {
		spr = a.A.FieldFocus
	}
	nineSlice(a.fb, spr, r, a.A.Theme.NineSliceEdge)
	ty := r.Min.Y + (r.Dy()-font.Height()+1)/2
	shown := shownOf(v)
	// keep the cursor in view
	if f.scroll > f.cursor {
		f.scroll = f.cursor
	}
	for f.scroll < f.cursor && font.Width(shown[f.scroll:f.cursor]) > maxW-2 {
		f.scroll++
	}
	for f.scroll > 0 && font.Width(shown[f.scroll-1:]) <= maxW {
		f.scroll-- // scroll back when text gets shorter
	}
	vis := shown[f.scroll:]
	for font.Width(vis) > maxW && len(vis) > 0 {
		vis = vis[:len(vis)-1]
	}
	if v == "" && !focused {
		font.Draw(a.fb, tx, ty, placeholder, a.A.C("text_dim"))
	} else {
		font.Draw(a.fb, tx, ty, vis, a.A.C("text"))
	}
	if focused && ((a.frame-a.caretAt)/30)%2 == 0 {
		cx := tx
		if f.cursor > f.scroll {
			cx += font.Width(shown[f.scroll:f.cursor]) + 1
		}
		fillRect(a.fb, image.Rect(cx, ty-1, cx+1, ty+font.Height()-1), a.A.C("gold"))
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
