package server

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
)

func storyGame(t *testing.T) *Game {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	st, err := OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	g := NewGame(cfg, st, nil, log.New(io.Discard, "", 0))
	g.seats[0] = &player{user: "alice", conn: &conn{}, seat: 0}
	return g
}

// age pretends the current page went up long ago.
func age(g *Game) { g.speech.at = g.speech.at.Add(-time.Minute) }

func TestStoryInterruptedAndResumed(t *testing.T) {
	g := storyGame(t)
	if !g.tellStory(g.seats[0]) {
		t.Fatal("no story")
	}
	first := g.speech.text
	if g.speech.prio != prioStory || len(g.sayQueue) < 5 {
		t.Fatalf("story didn't start: prio %d, %d pages left", g.speech.prio, len(g.sayQueue))
	}
	// small talk can't interrupt a story
	age(g)
	if g.say(banter.EvIdle, nil, banter.Vars{}, 1, prioNormal) {
		t.Fatal("small talk interrupted the story")
	}
	g.onSayTimer() // page 2
	age(g)
	// a bust can
	g.forceSay = true // skip the dice
	if !g.say(banter.EvBust, g.seats[0], banter.Vars{Total: 25}, 1, prioHigh) {
		t.Fatal("bust didn't interrupt")
	}
	g.forceSay = false
	if g.story == nil || g.story.held == nil {
		t.Fatal("story wasn't put on hold")
	}
	held := len(g.story.held)
	for len(g.sayQueue) > 0 {
		g.onSayTimer()
	}
	g.onSayTimer() // back to the story
	if g.story == nil || g.story.held != nil {
		t.Fatal("story didn't resume")
	}
	pages := 1
	for g.story != nil {
		g.onSayTimer()
		pages++
		if pages > 100 {
			t.Fatal("story never ends")
		}
	}
	if pages < held {
		t.Fatalf("story cut short: %d pages after resuming, %d were held", pages, held)
	}
	// the log has the story joined up and the interruption separately
	starts := 0
	for _, l := range g.sayLog {
		if !l.Cont {
			starts++
		}
	}
	if starts != 3 || g.sayLog[0].Text != first {
		t.Fatalf("log entries: %d (want story, bust, resumed story)", starts)
	}
}

func TestStoryAbandonedWhenEveryoneLeaves(t *testing.T) {
	g := storyGame(t)
	g.tellStory(nil)
	g.seats[0] = nil
	for i := 0; i < 100 && g.story != nil; i++ {
		g.onSayTimer()
	}
	if g.story != nil {
		t.Fatal("story still going")
	}
}

func TestQuieterAtNormalPace(t *testing.T) {
	g := storyGame(t)
	g.speak("Something short.", prioLow)
	g.speech.until = time.Now().Add(-time.Second) // finished a second ago
	if !g.busyTalking(prioLow, time.Now()) {
		t.Fatal("normal pace should leave a gap after a line")
	}
	g.cfg.TurboMode = true
	if g.busyTalking(prioLow, time.Now()) {
		t.Fatal("turbo has no gap")
	}
}

func TestLongLinesStayUpLonger(t *testing.T) {
	g := storyGame(t)
	short := g.pageTime("Bust.", true)
	long := g.pageTime(strings.Repeat("word ", 22), true)
	if short < 5*time.Second || long <= short+2*time.Second {
		t.Fatalf("short %v long %v", short, long)
	}
}

func TestPaceMigration(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "server.json")
	old := `{"port": 7777, "betting_time_seconds": 15, "turn_time_seconds": 33, "deal_delay_ms": 350, "dealer_delay_ms": 700, "results_time_seconds": 5, "play_time_seconds": 30}`
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.DealDelayMS != 700 || c.BettingTimeSeconds != 20 || c.TurnTimeSeconds != 33 || c.TurboMode {
		t.Fatalf("migrated: deal %d bet %d turn %d turbo %v", c.DealDelayMS, c.BettingTimeSeconds, c.TurnTimeSeconds, c.TurboMode)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"turbo_mode": false`) {
		t.Fatal("turbo_mode not written to the file")
	}
	// once turbo_mode exists, values are left alone
	c2, _, _ := LoadConfig(p)
	if c2.DealDelayMS != 700 {
		t.Fatal("changed on second load")
	}
	c2.TurboMode = true
	if c2.Timing().Deal != 350*time.Millisecond || c2.Timing().Betting != 15*time.Second {
		t.Fatal("turbo timing")
	}
}
