// Package mobile is the bridge between the Android app and the game client.
// It is compiled with `gomobile bind` into an .aar; the Java side forwards
// touch and keyboard input, asks for frames (RGBA pixels, 640x360) and
// implements Host for sound, the picture picker, the clipboard and the
// on-screen keyboard.
package mobile

import (
	"errors"
	"strings"
	"sync"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
)

// Host is implemented in Java.
type Host interface {
	PlaySound(name string, volume float64)
	PickImage()
	ClipboardText() string
	ShowKeyboard(show bool)
	Quit()
}

// Key codes for Key().
const (
	KeyEnter     = 1
	KeyBackspace = 2
	KeyBack      = 3 // Android back button: closes windows like Escape
	KeyTab       = 4
	KeyLeft      = 5
	KeyRight     = 6
	KeyDelete    = 7
)

var (
	mu       sync.Mutex
	app      *client.App
	assets   *client.Assets
	host     Host
	in       client.Input
	hideNext bool
	dragY    float64
	dragLast float64
	dragging bool
	pending  []string // pictures picked while a frame was running
)

type platform struct{}

func (platform) PlaySound(name string, _ []byte, volume float64) {
	if host != nil {
		go host.PlaySound(name, volume)
	}
}
func (platform) OpenImageFile() (string, bool) { return "", false }
func (platform) PickImageAsync() {
	if host != nil {
		go host.PickImage()
	}
}
func (platform) ClipboardText() string {
	if host != nil {
		return host.ClipboardText()
	}
	return ""
}
func (platform) ToggleFullscreen() {}
func (platform) Quit() {
	if host != nil {
		go host.Quit()
	}
}
func (platform) SetTextInput(on bool) {
	if host != nil {
		go host.ShowKeyboard(on)
	}
}
func (platform) IsTouch() bool { return true }

// Start loads settings from configDir and art from assetsDir (defaults are
// exported there so they can be replaced) and creates the game.
func Start(configDir, assetsDir string, h Host) error {
	mu.Lock()
	defer mu.Unlock()
	if h == nil {
		return errors.New("no host")
	}
	host = h
	if app != nil {
		return nil // activity recreated: keep the running game
	}
	cfg := client.LoadConfig(configDir)
	if assetsDir != "" {
		cfg.AssetsDir = assetsDir
	}
	_ = client.ExportDefaults(cfg.AssetsDir)
	a, err := client.LoadAssets(cfg.AssetsDir)
	if err != nil {
		return err
	}
	assets = a
	app = client.New(platform{}, cfg, a)
	in = client.Input{MX: -1, MY: -1}
	return nil
}

// Width is the frame width in pixels.
func Width() int {
	mu.Lock()
	defer mu.Unlock()
	return client.ScreenW
}

// Height is the frame height in pixels.
func Height() int {
	mu.Lock()
	defer mu.Unlock()
	return client.ScreenH
}

// Layout tells the game how big the screen area is (in device pixels); it
// picks a canvas with the same shape, which Width and Height then return.
func Layout(pw, ph int) {
	mu.Lock()
	defer mu.Unlock()
	w, h := client.ChooseSize(pw, ph)
	if app != nil {
		app.SetSize(w, h)
	} else {
		client.ScreenW, client.ScreenH = w, h
	}
}

// FocusBottom is the bottom of the text box being typed in (canvas pixels),
// or -1, so the screen can pan it above the keyboard.
func FocusBottom() float64 {
	mu.Lock()
	defer mu.Unlock()
	if app == nil {
		return -1
	}
	return float64(app.FocusBottom())
}

// Frame advances the game one frame and returns the picture as RGBA bytes
// (Width*Height*4), ready for Bitmap.copyPixelsFromBuffer.
func Frame() []byte {
	mu.Lock()
	defer mu.Unlock()
	if app == nil {
		return make([]byte, client.ScreenW*client.ScreenH*4)
	}
	for _, p := range pending {
		app.ImagePicked(p)
	}
	pending = nil
	cur := in
	fb := app.Frame(cur)
	// reset per-frame input
	in.Pressed, in.Released, in.RightPressed = false, false, false
	in.Wheel = 0
	in.Chars = nil
	in.Keys = nil
	if hideNext && !in.Down {
		in.MouseInside = false
		in.MX, in.MY = -1, -1
		hideNext = false
	}
	return fb.Pix
}

// Touch actions for Touch().
const (
	TouchDown = 0
	TouchMove = 1
	TouchUp   = 2
)

// Touch reports a finger at (x, y) in frame pixels (the Java side converts
// from screen coordinates). Vertical drags also scroll lists.
func Touch(action int, x, y float64) {
	mu.Lock()
	defer mu.Unlock()
	in.MX, in.MY = int(x), int(y)
	in.MouseInside = true
	switch action {
	case TouchDown:
		in.Down, in.Pressed = true, true
		hideNext = false
		dragY, dragLast, dragging = y, y, false
	case TouchMove:
		if !dragging && (y-dragY > 10 || dragY-y > 10) {
			dragging = true
			dragLast = dragY
		}
		if dragging {
			const notch = 14.0
			for y-dragLast >= notch {
				in.Wheel++ // finger moves down = scroll up, like a list
				dragLast += notch
			}
			for dragLast-y >= notch {
				in.Wheel--
				dragLast -= notch
			}
		}
	case TouchUp:
		in.Down = false
		if dragging {
			// a drag is not a tap: release away from where it started
			in.MX, in.MY = -1, -1
		}
		in.Released = true
		hideNext = true
	}
}

// KeyChar types a character (from the on-screen or a physical keyboard).
func KeyChar(r int32) {
	mu.Lock()
	defer mu.Unlock()
	if r >= 32 && r < 127 {
		in.Chars = append(in.Chars, rune(r))
	}
}

// Text types a string (keyboards that commit whole words).
func Text(s string) {
	mu.Lock()
	defer mu.Unlock()
	for _, r := range s {
		if r == '\n' {
			in.Keys = append(in.Keys, client.KeyEnter)
		} else if r >= 32 && r < 127 {
			in.Chars = append(in.Chars, r)
		}
	}
}

// Key presses a special key (see the Key* constants).
func Key(code int) {
	mu.Lock()
	defer mu.Unlock()
	var k client.Key
	switch code {
	case KeyEnter:
		k = client.KeyEnter
	case KeyBackspace:
		k = client.KeyBackspace
	case KeyBack:
		k = client.KeyEscape
	case KeyTab:
		k = client.KeyTab
	case KeyLeft:
		k = client.KeyLeft
	case KeyRight:
		k = client.KeyRight
	case KeyDelete:
		k = client.KeyDelete
	default:
		return
	}
	in.Keys = append(in.Keys, k)
}

// ImagePicked hands over a picture chosen in the picker (a file path).
func ImagePicked(path string) {
	mu.Lock()
	defer mu.Unlock()
	pending = append(pending, path)
}

// SoundNames lists the sound effects, comma-separated.
func SoundNames() string { return strings.Join(client.SoundNames, ",") }

// SoundData returns a sound effect's WAV bytes (nil if missing).
func SoundData(name string) []byte {
	mu.Lock()
	defer mu.Unlock()
	if assets == nil {
		return nil
	}
	return assets.Sounds[name]
}

// SoundEnabled reports whether the player has sound turned on.
func SoundEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return app != nil && app.Cfg.Sound
}

// Pause saves settings (the app is going to the background).
func Pause() {
	mu.Lock()
	defer mu.Unlock()
	if app != nil {
		app.Cfg.Save()
	}
}
