package client

import (
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Table chat: short messages in a bubble by the player's portrait. Admins
// can type /commands, which the server runs as if typed at its console.

const chatMaxLen = 80

type chatBubble struct {
	text  string
	admin bool
	at    time.Time
}

func (b chatBubble) visible(now time.Time) bool {
	return now.Sub(b.at) < time.Duration(5000+60*len(b.text))*time.Millisecond
}

func (a *App) onChat(c proto.Chat) {
	if a.chats == nil {
		a.chats = map[int]chatBubble{}
	}
	a.chatLog = append(a.chatLog, c)
	if len(a.chatLog) > 60 {
		a.chatLog = a.chatLog[len(a.chatLog)-60:]
	}
	if c.Seat >= 0 {
		a.chats[c.Seat] = chatBubble{text: c.Text, admin: c.Admin, at: a.now}
	} else {
		a.toast(c.From+": "+c.Text, a.A.C("text"), 5, 1)
	}
	a.sound("chat")
}

func (a *App) onConsole(c proto.Console) {
	a.console = append(a.console, c.Lines...)
	if len(a.console) > 400 {
		a.console = a.console[len(a.console)-400:]
	}
	a.consoleAt = a.now
	a.adminConsoleScroll = 0
}

func chatButtonRect() image.Rectangle {
	r := logButtonRect()
	return image.Rect(r.Max.X+4, r.Min.Y, r.Max.X+4+30, r.Max.Y-4)
}

// chatPreInput runs before the table is drawn: Enter and Escape belong to
// the chat box while it's open, not to the buttons under it.
func (a *App) chatPreInput() {
	if !a.chatOpen || a.blocked {
		return
	}
	keys := a.in.Keys[:0]
	for _, k := range a.in.Keys {
		switch k {
		case KeyEnter:
			a.sendChat()
			continue
		case KeyEscape:
			a.closeChat()
			continue
		}
		keys = append(keys, k)
	}
	a.in.Keys = keys
}

func (a *App) sendChat() {
	t := strings.TrimSpace(a.chatDraft)
	if t != "" {
		a.send(proto.CChat, proto.Chat{Text: t})
	}
	a.closeChat()
}

func (a *App) closeChat() {
	a.chatOpen = false
	a.chatDraft = ""
	if a.focusID == "chat" {
		a.focusID = ""
	}
}

func (a *App) openChat(prefix string) {
	a.chatOpen = true
	a.chatDraft = prefix
	a.focusID = "chat"
	a.kbAsk = true
	a.field("chat").cursor = len(prefix)
}

func (a *App) chatAllowed(st *proto.TableState) bool {
	return st.Settings.TableChat || a.profile.Admin
}

// drawChatBar draws the chat button, or the box while typing.
func (a *App) drawChatBar(st *proto.TableState) {
	if !a.chatAllowed(st) {
		a.chatOpen = false
		return
	}
	if !a.chatOpen {
		r := chatButtonRect()
		if a.button("chat_btn", r, "SAY", btn{enabled: a.overlay == ovNone, hotkey: 'y'}) {
			a.openChat("")
		}
		if a.overlay == ovNone && !a.blocked && a.focusID == "" && a.profile.Admin && a.in.hotkey('/') {
			a.openChat("/")
		}
		if a.hovered(r) && !a.blocked {
			a.text(r.Min.X+r.Dx()/2, r.Max.Y+5, "CHAT (Y)", a.A.C("text"), alignCenter)
		}
		a.drawConsolePeek()
		return
	}
	w := min(320, ScreenW-20)
	y := logButtonRect().Max.Y + 6
	r := image.Rect(ScreenW/2-w/2, y, ScreenW/2+w/2, y+18)
	fillRect(a.fb, r.Inset(-3), color.RGBA{0, 0, 0, 150})
	max := chatMaxLen
	ph := "Say something... (Enter sends, Esc cancels)"
	if a.profile.Admin {
		max = 240
		ph = "Say something, or /command  (Enter sends)"
	}
	send := image.Rect(r.Max.X-40, r.Min.Y, r.Max.X, r.Max.Y)
	field := image.Rect(r.Min.X, r.Min.Y, send.Min.X-3, r.Max.Y)
	a.textField("chat", field, &a.chatDraft, false, max, ph, nil)
	if a.button("chat_send", send, "SEND", btn{enabled: true, style: styleGold}) {
		a.sendChat()
	}
	if strings.HasPrefix(a.chatDraft, "/") && !a.profile.Admin {
		a.text(r.Min.X, r.Max.Y+5, "Only admins can use / commands.", a.A.C("lose"), alignLeft)
	}
	// typing elsewhere (or tapping away) closes it
	if a.focusID != "chat" && a.in.Pressed && !a.hovered(r.Inset(-3)) {
		a.closeChat()
	}
	a.drawConsolePeek()
}

// drawConsolePeek shows an admin command's output for a few seconds.
func (a *App) drawConsolePeek() {
	if len(a.console) == 0 || a.overlay == ovAdmin || a.now.Sub(a.consoleAt) > 9*time.Second {
		return
	}
	lines := a.console
	start := len(lines) - 1
	for start > 0 && !strings.HasPrefix(lines[start], "> ") && len(lines)-start < 10 {
		start--
	}
	lines = lines[start:]
	if len(lines) > 10 {
		lines = lines[len(lines)-10:]
	}
	w := min(420, ScreenW-20)
	y := logButtonRect().Max.Y + 30
	r := image.Rect(ScreenW/2-w/2, y, ScreenW/2+w/2, y+len(lines)*9+8)
	fillRect(a.fb, r, color.RGBA{0, 0, 0, 200})
	strokeRect(a.fb, r, a.A.C("gold"))
	for i, l := range lines {
		c := a.A.C("text")
		if strings.HasPrefix(l, "> ") {
			c = a.A.C("gold")
		}
		a.A.Font.Draw(a.fb, r.Min.X+5, r.Min.Y+4+i*9, fitText(a.A.Font, l, w-10), c)
	}
}

// drawChatBubbles draws recent messages over the seats.
func (a *App) drawChatBubbles(st *proto.TableState) {
	n := len(st.Seats)
	for seat, b := range a.chats {
		if !b.visible(a.now) {
			delete(a.chats, seat)
			continue
		}
		if seat >= n || st.Seats[seat].Username == "" {
			continue
		}
		x, cy := a.seatPos(seat, n)
		lines := a.wrap(b.text, 110)
		if len(lines) > 3 {
			lines = lines[:3]
			lines[2] = fitText(a.A.Font, lines[2]+"...", 110)
		}
		w := 0
		for _, l := range lines {
			w = max(w, a.A.Font.Width(l))
		}
		w += 8
		h := len(lines)*9 + 5
		// above the cards (which sit over the hat), or the hat if no cards
		ar := image.Rect(x-16, cy-16, x+16, cy+16)
		cardTop := ar.Min.Y - 4 - a.A.Theme.HatHeight + a.A.Theme.HatOverlap - a.A.Theme.CardHeight + 6
		bottom := cardTop - 4
		if len(st.Seats[seat].Hands) == 0 {
			bottom = ar.Min.Y - a.A.Theme.HatHeight + a.A.Theme.HatOverlap - 2
		}
		r := image.Rect(x-w/2, bottom-h, x-w/2+w, bottom)
		if r.Min.X < 2 {
			r = r.Add(image.Pt(2-r.Min.X, 0))
		}
		if r.Max.X > ScreenW-2 {
			r = r.Sub(image.Pt(r.Max.X-ScreenW+2, 0))
		}
		if r.Min.Y < 2 {
			r = r.Add(image.Pt(0, 2-r.Min.Y))
		}
		bg, fg := color.RGBA{250, 248, 236, 255}, color.RGBA{30, 30, 30, 255}
		if b.admin {
			bg = color.RGBA{255, 232, 160, 255} // admins get a gold bubble
		}
		fillRect(a.fb, r.Add(image.Pt(1, 1)), color.RGBA{0, 0, 0, 80})
		fillRect(a.fb, r, bg)
		strokeRect(a.fb, r, color.RGBA{0, 0, 0, 255})
		// tail down toward the portrait
		tx := min(max(x, r.Min.X+4), r.Max.X-5)
		for i := 0; i < 3; i++ {
			fillRect(a.fb, image.Rect(tx-2+i, r.Max.Y+i, tx+3-i, r.Max.Y+i+1), bg)
		}
		for i, l := range lines {
			a.A.Font.Draw(a.fb, r.Min.X+4, r.Min.Y+3+i*9, l, fg)
		}
	}
}
