package client

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

type soundRec struct {
	fakePlatform
	played []string
	vols   []float64
	wavs   [][]byte
}

func (s *soundRec) PlaySound(name string, wav []byte, vol float64) {
	s.played = append(s.played, name)
	s.vols = append(s.vols, vol)
	s.wavs = append(s.wavs, wav)
}

func newRecApp(t *testing.T) (*App, *soundRec) {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	rec := &soundRec{}
	cfg := &Config{Sound: true, Volume: 100, EffectsVolume: 100, VoiceVolume: 100}
	return New(rec, cfg, assets), rec
}

func TestDealerSpeaksWordByWord(t *testing.T) {
	a, rec := newRecApp(t)
	a.screen = scrTable
	st := baseState(10)
	st.Phase = proto.PhaseBetting
	st.Settings.DealerVulgar = true
	st.DealerSay = "Standing on 13? Grow a pair, you chickenshit."
	st.DealerSayID = 7
	st.DealerSayUntil = time.Now().Add(time.Hour).UnixMilli()
	a.onState(st)
	a.Frame(Input{MX: -1, MY: -1})
	if n := a.wordsShown(); n != 1 {
		t.Fatalf("expected 1 word at the start, got %d", n)
	}
	a.sayAt = time.Now().Add(-a.sayEnd / 2)
	a.Frame(Input{MX: -1, MY: -1})
	mid := a.wordsShown()
	if mid <= 1 || mid >= len(a.sayWords) {
		t.Fatalf("midway shows %d of %d words", mid, len(a.sayWords))
	}
	a.sayAt = time.Now().Add(-time.Minute)
	a.Frame(Input{MX: -1, MY: -1})
	if a.wordsShown() != len(a.sayWords) || a.mouthOpen(st) {
		t.Fatal("line should be finished with the mouth shut")
	}
	low := 0
	for _, s := range rec.played {
		if strings.HasPrefix(s, "talklow") {
			low++
		} else if strings.HasPrefix(s, "talk") {
			t.Fatalf("vulgar dealer used the high voice: %s", s)
		}
	}
	if low < 2 {
		t.Fatalf("expected voice blips, got %v", rec.played)
	}
}

func peak(wav []byte) int {
	f, _, _ := strings.Cut(string(wav), "data")
	d := wav[len(f)+8:]
	m := 0
	for i := 0; i+1 < len(d); i += 2 {
		s := int(int16(binary.LittleEndian.Uint16(d[i:])))
		if s < 0 {
			s = -s
		}
		if s > m {
			m = s
		}
	}
	return m
}

func TestVolumes(t *testing.T) {
	a, rec := newRecApp(t)
	full := peak(a.A.Sounds["chip"])
	a.Cfg.Volume, a.Cfg.EffectsVolume, a.Cfg.VoiceVolume = 50, 100, 0
	a.sound("chip")
	a.sound("talk1") // voice muted
	if len(rec.played) != 1 || rec.played[0] != "chip" {
		t.Fatalf("played %v", rec.played)
	}
	if v := rec.vols[0]; v < 0.49 || v > 0.51 {
		t.Fatalf("volume %v", v)
	}
	if p := peak(rec.wavs[0]); p > full*55/100 || p < full*45/100 {
		t.Fatalf("scaled peak %d of %d", p, full)
	}
	a.Cfg.VoiceVolume = 40
	a.sound("talk1")
	if rec.played[len(rec.played)-1] != "talk1" || rec.vols[len(rec.vols)-1] > 0.21 {
		t.Fatalf("voice volume %v", rec.vols)
	}
	a.Cfg.Sound = false
	n := len(rec.played)
	a.sound("chip")
	if len(rec.played) != n {
		t.Fatal("muted but played")
	}
	if len(a.A.Sounds["chip"]) == 0 || peak(a.A.Sounds["chip"]) != full {
		t.Fatal("original sound was modified")
	}
}

func TestSettingsSlider(t *testing.T) {
	a, _ := newRecApp(t)
	a.screen = scrTable
	st := baseState(10)
	st.Phase = proto.PhaseBetting
	a.onState(st)
	a.overlay = ovSettings
	a.Frame(Input{MX: -1, MY: -1})
	// press on the left end of the master slider and let go
	y := 40 + 44 + 11 + 8
	a.Frame(Input{MX: 176, MY: y, Pressed: true, Down: true, MouseInside: true})
	a.Frame(Input{MX: 176, MY: y, Released: true, MouseInside: true})
	if a.Cfg.Volume > 5 {
		t.Fatalf("master volume %d after dragging to the left", a.Cfg.Volume)
	}
}
