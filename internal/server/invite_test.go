// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Invite codes work once, are given back if the sign-up fails, survive a
// restart, and record who invited whom.
func TestInviteCodes(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	codes := s.MakeInvites(3, "boss", "for the cousins")
	if len(codes) != 3 || len(codes[0]) != InviteLen {
		t.Fatalf("codes %v", codes)
	}
	if _, _, err := s.RegisterInvited("zzzz", "nobody", "pass1234", nil, 100, ""); err == nil {
		t.Fatal("a made-up code worked")
	}
	if _, _, err := s.RegisterInvited(codes[0], "x", "pass1234", nil, 100, ""); err == nil {
		t.Fatal("bad username accepted")
	}
	if len(s.Invites()) != 3 {
		t.Fatal("a failed sign-up used up the code")
	}
	low := strings.ToLower(codes[0][:2]) + "-" + codes[0][2:] // typed sloppily
	if _, iv, err := s.RegisterInvited(low, "cousin", "pass1234", nil, 100, ""); err != nil || iv.By != "boss" {
		t.Fatalf("good code refused: %v", err)
	}
	if _, _, err := s.RegisterInvited(codes[0], "cousin2", "pass1234", nil, 100, ""); err == nil {
		t.Fatal("a code worked twice")
	}
	if n := s.RevokeInvite(codes[1]); n != 1 {
		t.Fatal("revoke failed")
	}
	s.Save()
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if left := s2.Invites(); len(left) != 1 || left[0].Code != codes[2] {
		t.Fatalf("after restart: %+v", left)
	}
	s2.mu.Lock()
	a := s2.accounts["cousin"]
	s2.mu.Unlock()
	if a == nil || a.InvitedBy != "boss" || a.InviteCode != codes[0] {
		t.Fatalf("account %+v", a)
	}
}

// talk connects to the server, says hello as release rel, sends one message
// and returns the reply.
func talk(t *testing.T, s *Server, rel string, tp string, v any) proto.Msg {
	nc, err := net.Dial("tcp", s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	w := bufio.NewWriter(nc)
	w.Write(proto.Encode(proto.CHello, proto.Hello{Version: proto.Version, Lobby: true, Release: rel}))
	w.Write(proto.Encode(tp, v))
	w.Flush()
	sc := bufio.NewScanner(nc)
	sc.Buffer(make([]byte, 64*1024), proto.MaxLine)
	nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	for sc.Scan() {
		var m proto.Msg
		json.Unmarshal(sc.Bytes(), &m)
		if m.Type == proto.SAuthFail || m.Type == proto.SAuthOK {
			return m
		}
	}
	t.Fatal("no answer")
	return proto.Msg{}
}

// With require_invite on, signing up over the network needs a code.
func TestInviteRequired(t *testing.T) {
	s := testServer(t, func(c *Config) { c.RequireInvite = true })
	if m := talk(t, s, Version, proto.CRegister, proto.Register{Username: "freeload", Password: "pass1234"}); m.Type != proto.SAuthFail {
		t.Fatal("signed up without a code")
	}
	code := s.Store.MakeInvites(1, "server", "")[0]
	if m := talk(t, s, Version, proto.CRegister, proto.Register{Username: "welcome", Password: "pass1234", Invite: code}); m.Type != proto.SAuthOK {
		t.Fatalf("code refused: %s", m.Data)
	}
}

// An older game can connect (to be offered the update) but can't log in or
// sign up.
func TestOldClientRefused(t *testing.T) {
	old := Version
	Version = "1.9.5"
	defer func() { Version = old }()
	s := testServer(t, nil)
	for _, rel := range []string{"", "1.9.4"} {
		m := talk(t, s, rel, proto.CRegister, proto.Register{Username: "oldtimer", Password: "pass1234"})
		var f proto.AuthFail
		json.Unmarshal(m.Data, &f)
		if m.Type != proto.SAuthFail || !strings.Contains(f.Reason, "Update the game") {
			t.Fatalf("release %q: %s %s", rel, m.Type, m.Data)
		}
	}
	if m := talk(t, s, "1.9.5", proto.CRegister, proto.Register{Username: "current", Password: "pass1234"}); m.Type != proto.SAuthOK {
		t.Fatalf("current game refused: %s", m.Data)
	}
	c := *s.cfg()
	c.RequireLatestClient = false
	s.cfgp.Store(&c)
	if m := talk(t, s, "1.9.4", proto.CLogin, proto.Login{Username: "current", Password: "pass1234"}); m.Type != proto.SAuthOK {
		t.Fatalf("with the setting off, an old game should log in: %s", m.Data)
	}
}
