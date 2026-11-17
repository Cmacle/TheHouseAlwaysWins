package client

import (
	"encoding/binary"
	"image"
	"image/color"
	"strings"
)

// Volume settings live in Config as 0-100. Sound effects and the dealer's
// voice have their own levels on top of the master volume.

func clampPct(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// volumeFor returns the playback volume (0..1) for a sound.
func (a *App) volumeFor(name string) float64 {
	if !a.Cfg.Sound {
		return 0
	}
	cat := a.Cfg.EffectsVolume
	if strings.HasPrefix(name, "talk") {
		cat = a.Cfg.VoiceVolume
	}
	return float64(clampPct(a.Cfg.Volume)) / 100 * float64(clampPct(cat)) / 100
}

// scaledSound returns a WAV with its samples scaled to vol, cached in 5%
// steps so rapid sounds (the dealer's blips) don't allocate every time.
func (a *App) scaledSound(name string, vol float64) []byte {
	b := a.A.Sounds[name]
	if b == nil || vol >= 0.999 {
		return b
	}
	step := int(vol*20 + 0.5) // 0..20
	if step >= 20 {
		return b
	}
	if a.soundCache == nil {
		a.soundCache = map[string][]byte{}
	}
	key := name + "#" + string(rune('A'+step))
	if c, ok := a.soundCache[key]; ok {
		return c
	}
	c := scaleWav(b, float64(step)/20)
	a.soundCache[key] = c
	return c
}

// scaleWav copies a PCM WAV with its volume scaled (8- or 16-bit). Other
// formats are returned unchanged.
func scaleWav(b []byte, vol float64) []byte {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return b
	}
	out := append([]byte(nil), b...)
	bits := 0
	for p := 12; p+8 <= len(out); {
		id := string(out[p : p+4])
		n := int(binary.LittleEndian.Uint32(out[p+4 : p+8]))
		body := p + 8
		if n < 0 || body+n > len(out) {
			n = len(out) - body
		}
		switch id {
		case "fmt ":
			if n >= 16 && binary.LittleEndian.Uint16(out[body:]) == 1 {
				bits = int(binary.LittleEndian.Uint16(out[body+14:]))
			}
		case "data":
			d := out[body : body+n]
			switch bits {
			case 16:
				for i := 0; i+1 < len(d); i += 2 {
					s := int16(binary.LittleEndian.Uint16(d[i:]))
					binary.LittleEndian.PutUint16(d[i:], uint16(int16(float64(s)*vol)))
				}
			case 8:
				for i := range d {
					d[i] = uint8(128 + (float64(d[i])-128)*vol)
				}
			default:
				return b
			}
		}
		p = body + n + n%2
	}
	return out
}

// ---------------------------------------------------------------- settings window

// slider draws a horizontal 0-100 slider; drag or tap to set it. It returns
// true when the value changed this frame.
func (a *App) slider(id string, r image.Rectangle, v *int) bool {
	old := *v
	knobW := 8
	track := image.Rect(r.Min.X+knobW/2, r.Min.Y+r.Dy()/2-2, r.Max.X-knobW/2, r.Min.Y+r.Dy()/2+2)
	if a.hovered(r) && a.in.Pressed {
		a.activeID = id
	}
	if a.activeID == id && (a.in.Down || a.in.Pressed || a.in.Released) && a.in.MX >= 0 {
		p := (a.in.MX - track.Min.X) * 100 / max(1, track.Dx())
		*v = clampPct(p)
	}
	if a.hovered(r) && a.in.Wheel != 0 {
		*v = clampPct(*v + a.in.Wheel*5)
	}
	if a.hovered(r) && a.in.key(KeyLeft) {
		*v = clampPct(*v - 5)
	}
	if a.hovered(r) && a.in.key(KeyRight) {
		*v = clampPct(*v + 5)
	}
	fillRect(a.fb, track.Inset(-1), a.A.C("shadow"))
	fillRect(a.fb, track, color.RGBA{40, 50, 44, 255})
	fill := track
	fill.Max.X = track.Min.X + track.Dx()**v/100
	fillRect(a.fb, fill, a.A.C("gold"))
	kx := track.Min.X + track.Dx()**v/100 - knobW/2
	knob := image.Rect(kx, r.Min.Y, kx+knobW, r.Max.Y)
	spr := a.A.ButtonGold
	if a.activeID == id || a.hovered(r) {
		spr = a.A.ButtonGoldHover
	}
	nineSlice(a.fb, spr, knob, 3)
	return *v != old
}

func (a *App) drawSettings() {
	r := ovRect(340, 280)
	if a.overlayFrame(r, "SETTINGS") {
		a.Cfg.Save()
		return
	}
	x0, x1 := r.Min.X+24, r.Max.X-24
	y := r.Min.Y + 44
	row := func(id, label, hint string, v *int, test string) {
		a.text(x0, y, label, a.A.C("gold"), alignLeft)
		val := commas(int64(*v)) + "%"
		if *v == 0 {
			val = "MUTED"
		}
		a.text(x1, y, val, a.A.C("text"), alignRight)
		if hint != "" {
			a.text(x0+a.A.Font.Width(label)+8, y, hint, a.A.C("text_dim"), alignLeft)
		}
		sr := image.Rect(x0, y+11, x1-50, y+27)
		a.slider(id, sr, v)
		if a.button("t_"+id, image.Rect(x1-44, y+11, x1, y+27), "TEST", btn{enabled: true}) {
			a.sound(test)
		}
		// play a sample when you let go of the slider
		if a.in.Released && a.activeID == id {
			a.sound(test)
			a.Cfg.Save()
		}
		y += 44
	}
	row("vol_master", "MASTER VOLUME", "", &a.Cfg.Volume, "chip")
	row("vol_fx", "SOUND EFFECTS", "cards, chips, wins", &a.Cfg.EffectsVolume, "win")
	voiceTest := "talk2"
	if a.dealerVulgar(a.st) {
		voiceTest = "talklow2"
	}
	row("vol_voice", "DEALER VOICE", "his babbling", &a.Cfg.VoiceVolume, voiceTest)
	mute := !a.Cfg.Sound
	a.checkbox("mute", x0, y+2, "Mute everything (M)", &mute)
	if mute == a.Cfg.Sound {
		a.Cfg.Sound = !mute
		a.Cfg.Save()
	}
	y += 22
	if !a.isTouch() {
		a.text(x0, y, "Tip: hover a slider and use the mouse wheel or arrow keys.", a.A.C("text_dim"), alignLeft)
	}
	if a.button("set_done", image.Rect(r.Min.X+r.Dx()/2-40, r.Max.Y-34, r.Min.X+r.Dx()/2+40, r.Max.Y-14), "DONE", btn{enabled: true, style: styleGold, key: KeyEnter}) {
		a.Cfg.Save()
		a.overlay = ovNone
	}
}
