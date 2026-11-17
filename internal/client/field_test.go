package client

import "testing"

func TestTextFieldCursor(t *testing.T) {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{Server: "example.com:7777"}, assets)
	a.reconnectAt = a.now
	a.reconnectAt = a.reconnectAt.AddDate(10, 0, 0)
	a.focusID = "user"
	f := func(in Input) { in.MX, in.MY = -1, -1; a.Frame(in) }
	f(Input{Chars: []rune("helo")})
	f(Input{Keys: []Key{KeyLeft}})
	f(Input{Chars: []rune("l")})
	if a.fUser != "hello" {
		t.Fatalf("insert at cursor: %q", a.fUser)
	}
	f(Input{Keys: []Key{KeyHome}})
	f(Input{Chars: []rune("X")})
	f(Input{Keys: []Key{KeyEnd, KeyBackspace}})
	f(Input{Keys: []Key{KeyHome, KeyRight, KeyDelete}})
	if a.fUser != "Xell" {
		t.Fatalf("home/end/delete: %q", a.fUser)
	}
	// server box: ctrl+left jumps words
	a.focusID = "server"
	f(Input{Keys: []Key{KeyEnd}})
	f(Input{Keys: []Key{KeyLeft}, Ctrl: true})
	f(Input{Chars: []rune("new")})
	if a.fServer != "example.newcom" {
		t.Fatalf("word jump: %q", a.fServer)
	}
}
