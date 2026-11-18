// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// seatOf is the seat part of a card key ("s3_1_0" -> "s3", "d1" -> "d").
func seatOf(k string) string {
	if strings.HasPrefix(k, "d") {
		return "d"
	}
	if i := strings.IndexByte(k, '_'); i > 0 {
		return k[:i]
	}
	return k
}

const maxLogEntries = 40

// addLog adds a line to the "what the dealer said" log. Pages that continue
// a long line or story are joined onto the entry they belong to.
func (a *App) addLog(l proto.DealerLine) {
	a.logID = l.ID
	if l.Cont && len(a.sayLog) > 0 {
		e := &a.sayLog[len(a.sayLog)-1]
		sep := " "
		if l.Para {
			sep = "\n\n"
		}
		e.text += sep + l.Text
		return
	}
	at := time.UnixMilli(l.At)
	if l.At == 0 {
		at = time.Now()
	}
	a.sayLog = append(a.sayLog, logEntry{text: l.Text, at: at})
	if len(a.sayLog) > maxLogEntries {
		a.sayLog = append([]logEntry(nil), a.sayLog[len(a.sayLog)-maxLogEntries:]...)
	}
}

// logButtonRect is the little speech bubble next to the dealer that opens
// the log.
func logButtonRect() image.Rectangle {
	x, y := ScreenW/2-64, 26+layoutTop()
	return image.Rect(x, y, x+22, y+16)
}

func (a *App) drawLogButton(st *proto.TableState) {
	if !st.Settings.DealerTrashTalk && len(a.sayLog) == 0 {
		return
	}
	r := logButtonRect()
	hov := !a.blocked && a.hovered(r)
	bub, edge := a.A.C("bubble"), a.A.C("bubble_txt")
	if !hov {
		bub = color.RGBA{bub.R * 4 / 5, bub.G * 4 / 5, bub.B * 4 / 5, bub.A}
	}
	body := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-4)
	fillRect(a.fb, image.Rect(body.Min.X+1, body.Min.Y+1, body.Max.X+1, body.Max.Y+1), color.RGBA{0, 0, 0, 120})
	fillRect(a.fb, body, bub)
	strokeRect(a.fb, body, edge)
	// corners
	for _, p := range []image.Point{body.Min, {body.Max.X - 1, body.Min.Y}, {body.Min.X, body.Max.Y - 1}, {body.Max.X - 1, body.Max.Y - 1}} {
		fillRect(a.fb, image.Rect(p.X, p.Y, p.X+1, p.Y+1), color.RGBA{})
	}
	// tail pointing right, toward the dealer
	tx, ty := body.Max.X-7, body.Max.Y-1
	for i := 0; i < 4; i++ {
		fillRect(a.fb, image.Rect(tx+i, ty+i, tx+i+2, ty+i+1), bub)
		fillRect(a.fb, image.Rect(tx+i+2, ty+i, tx+i+3, ty+i+1), edge)
	}
	fillRect(a.fb, image.Rect(tx-1, ty, tx, ty+2), edge)
	// "..." or lines of text
	for i := 0; i < 3; i++ {
		fillRect(a.fb, image.Rect(body.Min.X+5+i*5, body.Min.Y+5, body.Min.X+7+i*5, body.Min.Y+7), edge)
	}
	if a.logUnread {
		d := image.Rect(r.Min.X-2, r.Min.Y-2, r.Min.X+3, r.Min.Y+3)
		fillRect(a.fb, d, a.A.C("lose"))
	}
	if hov {
		a.text(r.Min.X+r.Dx()/2, r.Max.Y+1, "LOG (T)", a.A.C("text"), alignCenter)
	}
	open := hov && a.in.Pressed
	if !a.blocked && a.focusID == "" && a.in.hotkey('t') {
		open = true
	}
	if open {
		a.overlay = ovDealerLog
		a.logScroll = 0
		a.logUnread = false
	}
}

// drawDealerLog shows what the dealer has said lately, newest at the bottom.
func (a *App) drawDealerLog() {
	r := ovRect(440, max(316, ScreenH-80))
	if a.overlayFrame(r, "WHAT DALE SAID") {
		return
	}
	a.logUnread = false
	x0 := r.Min.X + 18
	w := r.Dx() - 36
	top, bottom := r.Min.Y+38, r.Max.Y-36
	lh := 9
	type ln struct {
		s   string
		dim bool
	}
	var lines []ln
	for i, e := range a.sayLog {
		if i > 0 {
			lines = append(lines, ln{})
		}
		lines = append(lines, ln{e.at.Format("3:04 PM"), true})
		for _, l := range a.wrap(e.text, w) {
			lines = append(lines, ln{s: l})
		}
	}
	if len(lines) == 0 {
		msg := "He hasn't said anything yet. Give him a minute."
		if a.st != nil && !a.st.Settings.DealerTrashTalk {
			msg = "The dealer is keeping quiet (trash talk is off on this server)."
		}
		a.text(r.Min.X+r.Dx()/2, top+40, msg, a.A.C("text_dim"), alignCenter)
	}
	rows := (bottom - top) / lh
	maxScroll := max(0, len(lines)-rows)
	if a.in.Wheel != 0 {
		a.logScroll += a.in.Wheel * 3
	}
	if a.in.key(KeyUp) {
		a.logScroll++
	}
	if a.in.key(KeyDown) {
		a.logScroll--
	}
	a.logScroll = min(max(a.logScroll, 0), maxScroll)
	end := len(lines) - a.logScroll
	start := max(0, end-rows)
	for i, l := range lines[start:end] {
		c := a.A.C("text")
		if l.dim {
			c = a.A.C("gold")
		}
		a.text(x0, top+i*lh, l.s, c, alignLeft)
	}
	if maxScroll > 0 {
		// scrollbar
		track := image.Rect(r.Max.X-12, top, r.Max.X-9, bottom)
		fillRect(a.fb, track, color.RGBA{0, 0, 0, 120})
		h := max(8, track.Dy()*rows/len(lines))
		pos := track.Dy() - h - (track.Dy()-h)*a.logScroll/maxScroll
		fillRect(a.fb, image.Rect(track.Min.X, track.Min.Y+pos, track.Max.X, track.Min.Y+pos+h), a.A.C("gold"))
		if a.logScroll > 0 {
			a.text(r.Min.X+r.Dx()/2, bottom+2, "scroll down for newer", a.A.C("text_dim"), alignCenter)
		}
	}
	if a.button("log_done", image.Rect(r.Min.X+r.Dx()/2-40, r.Max.Y-26, r.Min.X+r.Dx()/2+40, r.Max.Y-8), "CLOSE", btn{enabled: true, style: styleGold, key: KeyEnter}) {
		a.overlay = ovNone
	}
}
