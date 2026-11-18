// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package discord

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

type fakeData struct{}

func (fakeData) ServerName() string { return "Test Casino" }
func (fakeData) Leaderboard(n int, rich bool) []proto.Profile {
	return []proto.Profile{{Username: "alice_x", Net: 1500, Balance: 2500, HandsPlayed: 9, Online: true}, {Username: "bob", Net: -300}}
}
func (fakeData) Profile(name string) (proto.Profile, bool) {
	return proto.Profile{Username: name, Net: 5, HandsPlayed: 2, HandsWon: 1}, name == "alice_x"
}
func (fakeData) TableSummary() string    { return "empty" }
func (fakeData) House() proto.HouseStats { return proto.HouseStats{HandsDealt: 10, Net: 1234} }

func TestBoardAndInteraction(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		bodies = append(bodies, string(b))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bot tok" && r.URL.Path != "/interactions/1/abc/callback" {
			t.Errorf("missing auth on %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"999"}`)
	}))
	defer srv.Close()
	apiBase = srv.URL
	b := New(Config{BotToken: "tok", LeaderboardChannelID: "123", LeaderboardSize: 10, StateFile: filepath.Join(t.TempDir(), "s.json")}, fakeData{}, log.New(io.Discard, "", 0))
	b.updateBoard(true)
	b.updateBoard(true)
	raw, _ := json.Marshal(map[string]any{"id": "1", "token": "abc", "type": 2, "data": map[string]any{"name": "stats", "options": []map[string]any{{"name": "player", "value": "alice_x"}}}})
	b.handleInteraction(raw)
	want := []string{"POST /channels/123/messages", "PATCH /channels/123/messages/999", "POST /interactions/1/abc/callback"}
	if len(calls) != 3 {
		t.Fatalf("calls %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call %d = %s want %s", i, calls[i], want[i])
		}
	}
	t.Log(bodies[0])
	t.Log(bodies[2])
}
