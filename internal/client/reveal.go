package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"time"

	bj "github.com/Cmacle/TheHouseAlwaysWins/internal/blackjack"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// Card reveal: the server holds a freshly drawn card face down, the client
// shows it big in the middle of the table with a suspense sound, and it
// flips when the server says. Then it settles back into the hand.

const (
	revPopSecs  = 0.25 // grows out of the hand
	revFlipSecs = 0.32
	revHoldSecs = 0.75 // face up in the middle after the flip
	revBackSecs = 0.30 // back into the hand
)

// SuspenseStyles and SuspenseLengths name the suspense sounds:
// suspense_<style>_<tenths of a second>.
var (
	SuspenseStyles  = []string{"drum", "rise", "heart"}
	SuspenseLengths = []int{5, 10, 15, 20, 25, 30}
)

type revealFX struct {
	key       string // card key (d1, s0_0_2)
	seat      int
	start     time.Time // big card appears
	flip      time.Time // when it turns over (server time, local clock)
	flipped   bool      // the server has said so
	flippedAt time.Time
	card      bj.Card
	outcome   string
	sound     string
	soundAt   time.Time
	soundOn   bool
	hx, hy    int // where the card sits in its hand
}

// onRevealState starts or follows a reveal from a state update.
func (a *App) onRevealState(st *proto.TableState) {
	if rv := st.Reveal; rv != nil {
		key := revealKey(rv.Seat, rv.Hand, rv.Card)
		if a.rev == nil || a.rev.key != key || a.rev.flipped {
			flip := time.UnixMilli(rv.Until - a.clockOffset)
			a.rev = &revealFX{key: key, seat: rv.Seat, start: a.now, flip: flip, hx: -1}
			// a suspense sound that ends right as the card turns over
			left := flip.Sub(a.now)
			n := 0
			for i, l := range SuspenseLengths {
				if time.Duration(l)*100*time.Millisecond <= left+50*time.Millisecond {
					n = i
				}
			}
			l := SuspenseLengths[n]
			a.rev.sound = fmt.Sprintf("suspense_%s_%d", SuspenseStyles[rand.Intn(len(SuspenseStyles))], l)
			a.rev.soundAt = flip.Add(-time.Duration(l) * 100 * time.Millisecond)
		}
		return
	}
	if a.rev != nil && !a.rev.flipped {
		a.revealFlipped("")
	}
	if a.rev != nil {
		a.rev.card = cardByKey(st, a.rev.key)
	}
}

// onRevealEvent is the moment the card turns over.
func (a *App) onRevealEvent(e proto.Event) {
	if a.rev == nil || a.rev.flipped {
		a.revealSting(e.Text, e.Seat)
		return
	}
	a.revealFlipped(e.Text)
	a.revealSting(e.Text, e.Seat)
}

func (a *App) revealFlipped(outcome string) {
	a.rev.flipped = true
	a.rev.flippedAt = a.now
	a.rev.outcome = outcome
}

// revealSting plays the sound for how the card went.
func (a *App) revealSting(outcome string, seat int) {
	name := map[string]string{
		"bust": "rev_bust", "safe": "rev_safe", "meh": "rev_meh", "good": "rev_good", "great": "rev_great", "21": "rev_21",
		"dealer_bust": "rev_dealer_bust", "dealer_strong": "rev_dealer_strong", "dealer_ok": "rev_dealer_ok", "dealer_more": "rev_dealer_more",
	}[outcome]
	if name != "" {
		a.sound(name)
	}
}

func revealKey(seat, hand, card int) string {
	if seat < 0 {
		return fmt.Sprintf("d%d", card)
	}
	return fmt.Sprintf("s%d_%d_%d", seat, hand, card)
}

func cardByKey(st *proto.TableState, key string) bj.Card {
	var out bj.Card
	cardKeys(st, func(k string, c bj.Card) {
		if k == key {
			out = c
		}
	})
	return out
}

// revealHides reports whether the card with this key is being shown big
// (so it isn't drawn in its hand) and remembers where it belongs.
func (a *App) revealHides(key string, x, y int) bool {
	r := a.rev
	if r == nil || r.key != key {
		return false
	}
	r.hx, r.hy = x, y
	return true
}

// drawReveal draws the big card. It's called after the table so it's on
// top of everything but the overlays.
func (a *App) drawReveal() {
	r := a.rev
	if r == nil {
		return
	}
	if !r.soundOn && !a.now.Before(r.soundAt) && !r.flipped {
		r.soundOn = true
		a.sound(r.sound)
	}
	cw, ch := a.A.Theme.CardWidth, a.A.Theme.CardHeight
	if cw <= 0 || ch <= 0 {
		cw, ch = a.A.CardBack.Rect.Dx(), a.A.CardBack.Rect.Dy()
	}
	k := max(2, min(4, (ScreenH-120)/ch))
	bw, bh := cw*k, ch*k
	cx, cy := ScreenW/2, ScreenH/2-10
	if a.L.portrait {
		cy = ScreenH * 2 / 5
	}
	hx, hy := r.hx+cw/2, r.hy+ch/2
	if r.hx < 0 {
		hx, hy = cx, cy
	}
	scale, px, py := 1.0, float64(cx), float64(cy)
	face := false
	flipK := 1.0 // horizontal squash while turning over
	var done bool
	if t := a.now.Sub(r.start).Seconds(); t < revPopSecs {
		e := easeOut(t / revPopSecs)
		scale = 1/float64(k) + (1-1/float64(k))*e
		px = float64(hx) + (float64(cx)-float64(hx))*e
		py = float64(hy) + (float64(cy)-float64(hy))*e
	}
	if r.flipped {
		t := a.now.Sub(r.flippedAt).Seconds()
		switch {
		case t < revFlipSecs:
			p := t / revFlipSecs
			if p < 0.5 {
				flipK = 1 - p*2
			} else {
				face, flipK = true, (p-0.5)*2
			}
		case t < revFlipSecs+revHoldSecs:
			face = true
		case t < revFlipSecs+revHoldSecs+revBackSecs:
			face = true
			e := easeOut((t - revFlipSecs - revHoldSecs) / revBackSecs)
			scale = 1 - (1-1/float64(k))*e
			px = float64(cx) + (float64(hx)-float64(cx))*e
			py = float64(cy) + (float64(hy)-float64(cy))*e
		default:
			done = true
		}
	} else {
		// a little wobble while everyone waits
		left := r.flip.Sub(a.now).Seconds()
		if left < 0.6 && left > 0 {
			px += math.Sin(a.now.Sub(r.start).Seconds()*60) * (0.6 - left) * 3
		}
	}
	if done {
		a.rev = nil
		return
	}
	// dim the table a little so the card stands out
	if scale > 0.5 {
		fillRect(a.fb, a.fb.Rect, color.RGBA{0, 0, 0, uint8(70 * scale)})
	}
	img := a.A.CardBack
	if face && !r.card.Hidden() {
		img = a.A.CardImage(r.card)
	}
	w := int(float64(bw) * scale * flipK)
	h := int(float64(bh) * scale)
	if w < 1 {
		w = 1
	}
	dr := image.Rect(int(px)-w/2, int(py)-h/2, int(px)-w/2+w, int(py)-h/2+h)
	// glow/shadow behind it
	if scale > 0.6 {
		sh := dr.Add(image.Pt(k, k))
		fillRect(a.fb, sh, color.RGBA{0, 0, 0, 110})
	}
	blitScaled(a.fb, img, dr)
	if face && flipK >= 1 && scale >= 0.99 && r.outcome != "" {
		if label, c := a.revealLabel(r.outcome); label != "" {
			a.textBig(cx, dr.Max.Y+6, label, c, 2, alignCenter)
		}
	}
}

func (a *App) revealLabel(outcome string) (string, color.RGBA) {
	switch outcome {
	case "bust":
		return "BUST", a.A.C("lose")
	case "21":
		return "TWENTY-ONE!", a.A.C("gold")
	case "great":
		return "BEAUTIFUL", a.A.C("win")
	case "good":
		return "NOT BAD", a.A.C("win")
	case "dealer_bust":
		return "DEALER BUSTS!", a.A.C("win")
	case "dealer_strong":
		return "UH OH", a.A.C("lose")
	}
	return "", color.RGBA{}
}
