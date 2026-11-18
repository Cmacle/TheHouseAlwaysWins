// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package server

import (
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/banter"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// voice is Dale's speech bubble for tables other than blackjack: a line at a
// time (long ones in pages), more important lines interrupting less
// important ones, a short breather between small talk, and a log. The table
// owns it and calls tick from its loop.
type voice struct {
	pick   *banter.Picker
	cur    speech
	queue  []banter.Page
	prio   int
	timer  *time.Timer
	log    []proto.DealerLine
	talked int // lines this hand
	budget int
}

func newVoice() *voice { return &voice{pick: banter.NewPicker(), budget: 4} }

// timerC is the page timer's channel (nil when idle).
func (v *voice) timerC() <-chan time.Time {
	if v.timer == nil {
		return nil
	}
	return v.timer.C
}

// say picks a line for an event and says it, if the dice and his manners
// allow. It returns whether he spoke.
func (v *voice) say(ev string, vulgar bool, vars banter.Vars, chance float64, prio int) bool {
	now := time.Now()
	if v.busy(prio, now) {
		return false
	}
	if prio < prioHigh && v.talked >= v.budget {
		return false
	}
	if v.pick.Chance() >= chance {
		return false
	}
	line := v.pick.Line(ev, vulgar, vars)
	if line == "" {
		return false
	}
	v.talked++
	v.speak(line, prio)
	return true
}

func (v *voice) busy(prio int, now time.Time) bool {
	cur := v.cur
	if now.Before(cur.until) || len(v.queue) > 0 {
		age := now.Sub(cur.at)
		finish := banter.SpeakTime(cur.text)
		if prio < cur.prio || (prio == cur.prio && age < finish+800*time.Millisecond) || age < 1200*time.Millisecond {
			return true
		}
	}
	return prio < prioHigh && now.Before(cur.until.Add(2500*time.Millisecond))
}

func (v *voice) speak(text string, prio int) {
	v.queue = banter.Pages(text)
	v.prio = prio
	v.next(false)
}

func (v *voice) next(cont bool) {
	if len(v.queue) == 0 {
		return
	}
	pg := v.queue[0]
	v.queue = v.queue[1:]
	now := time.Now()
	speak := banter.SpeakTime(pg.Text)
	n := time.Duration(len(pg.Text))
	d := max(speak+min(3500*time.Millisecond+n*25*time.Millisecond, 9*time.Second), 5*time.Second)
	if len(v.queue) > 0 {
		d = speak + 1800*time.Millisecond + n*18*time.Millisecond
	}
	v.cur = speech{text: pg.Text, page: pg, id: v.cur.id + 1, prio: v.prio, at: now, until: now.Add(d), cont: cont}
	v.log = append(v.log, proto.DealerLine{ID: v.cur.id, Text: pg.Text, Cont: cont, Para: cont && pg.Para, At: now.UnixMilli()})
	if len(v.log) > 80 {
		v.log = append([]proto.DealerLine(nil), v.log[len(v.log)-80:]...)
	}
	if v.timer != nil {
		v.timer.Stop()
	}
	v.timer = time.NewTimer(d)
}

// tick runs when the page timer fires; it reports whether the bubble changed.
func (v *voice) tick() bool {
	v.timer = nil
	if len(v.queue) > 0 {
		v.next(true)
		return true
	}
	return false
}

// fill puts the bubble into a poker state.
func (v *voice) fill(st *proto.PokerState, withLog bool) {
	if v.cur.text != "" {
		st.DealerSay, st.DealerSayID, st.DealerSayUntil = v.cur.text, v.cur.id, v.cur.until.UnixMilli()
		st.DealerSayCont, st.DealerSayPara = v.cur.cont, v.cur.cont && v.cur.page.Para
	}
	if withLog {
		st.DealerLog = v.log
	}
}
