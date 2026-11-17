package mobile

import (
	"testing"
)

type fakeHost struct{ kb []bool }

func (f *fakeHost) PlaySound(string)       {}
func (f *fakeHost) PickImage()             {}
func (f *fakeHost) ClipboardText() string  { return "" }
func (f *fakeHost) ShowKeyboard(show bool) { f.kb = append(f.kb, show) }
func (f *fakeHost) Quit()                  {}

func TestBridge(t *testing.T) {
	dir := t.TempDir()
	if err := Start(dir, dir+"/assets", &fakeHost{}); err != nil {
		t.Fatal(err)
	}
	px := Frame()
	if len(px) != Width()*Height()*4 {
		t.Fatalf("frame size %d", len(px))
	}
	if SoundData("card") == nil || SoundNames() == "" {
		t.Fatal("sounds missing")
	}
	// tap the username field and type
	Touch(TouchDown, 300, 175)
	Touch(TouchUp, 300, 175)
	Frame()
	Text("dale")
	KeyChar('x')
	Key(KeyBackspace)
	Frame()
	mu.Lock()
	user := app.Cfg.Username
	mu.Unlock()
	_ = user
	// drag scroll produces wheel events
	Touch(TouchDown, 100, 100)
	Touch(TouchMove, 100, 160)
	mu.Lock()
	w := in.Wheel
	mu.Unlock()
	if w == 0 {
		t.Fatal("drag did not scroll")
	}
	Touch(TouchUp, 100, 160)
	Frame()
}
