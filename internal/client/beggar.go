// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Beggars Table: five dice thrown into a tray, bouncing off the walls
// and each other until they settle. The server has already decided how they
// land; the throw is simulated from the server's seed, with the felt's
// friction tuned so the dice come to rest when the server says the roll is
// over.

const (
	dieSize  = 17.0
	diceStep = 1.0 / 240 // physics step, seconds
)

type die struct {
	x, y, z    float64 // z: height above the felt
	vx, vy, vz float64
	ang, va    float64
	face       int
	target     int
	rolled     float64 // distance since the face last changed
	settled    bool
}

type diceRoll struct {
	id      int
	dice    []die
	start   time.Time
	simT    float64 // seconds simulated so far
	fric    float64
	rng     *rand.Rand
	tray    image.Rectangle
	done    bool
	win     bool
	faces   []int
	clackAt float64
	clacks  int // collisions this step, for the sound
	chimed  bool
}

func (a *App) onBeggar(bs *proto.BeggarState) {
	a.clockOffset = bs.ServerTime - time.Now().UnixMilli()
	shell := &proto.TableState{TableID: bs.Table, TableName: bs.Name, Phase: "begging", YourSeat: -1, ActiveSeat: -1,
		ServerTime: bs.ServerTime, Settings: bs.Settings, DealerSay: bs.DealerSay, DealerSayID: bs.DealerSayID,
		DealerSayUntil: bs.DealerSayUntil, DealerSayCont: bs.DealerSayCont, DealerSayPara: bs.DealerSayPara,
		DealerLog: bs.DealerLog, HouseNet: bs.HouseNet, HouseTier: bs.HouseTier}
	a.onDealerLines(shell)
	a.bs, a.st = bs, shell
}

// diceTray is where the dice roll.
func (a *App) diceTray() image.Rectangle {
	w := min(300, ScreenW-40)
	h := 104
	top := a.L.cardsY + 30
	if a.L.portrait {
		top = a.L.msgY + 30
		h = 120
	}
	return image.Rect(ScreenW/2-w/2, top, ScreenW/2+w/2, top+h)
}

func (a *App) onDice(r proto.DiceRoll) {
	a.computeLayout()
	tray := a.diceTray()
	dr := &diceRoll{id: r.ID, start: a.now, tray: tray, win: r.Win, faces: r.Faces}
	want := float64(r.MS)/1000 - 0.15
	// find the friction that settles the dice on time
	lo, hi := 0.2, 12.0
	for i := 0; i < 18; i++ {
		mid := (lo + hi) / 2
		if newThrow(r, tray, mid).settleTime() > want {
			lo = mid
		} else {
			hi = mid
		}
	}
	t := newThrow(r, tray, (lo+hi)/2)
	dr.dice, dr.fric, dr.rng = t.dice, t.fric, t.rng
	a.roll = dr
	a.sound("shuffle")
}

// newThrow sets up the dice as they leave the hand: from the bottom right of
// the tray, flying up and left, spinning.
func newThrow(r proto.DiceRoll, tray image.Rectangle, fric float64) *diceRoll {
	rng := rand.New(rand.NewSource(r.Seed))
	t := &diceRoll{tray: tray, fric: fric, rng: rng}
	for i := 0; i < len(r.Faces); i++ {
		d := die{
			x:      float64(tray.Max.X) - 14 - rng.Float64()*30,
			y:      float64(tray.Max.Y) - 12 - float64(i)*7 - rng.Float64()*6,
			z:      18 + rng.Float64()*14,
			vx:     -(260 + rng.Float64()*200),
			vy:     -(40 + rng.Float64()*120) + float64(i)*18,
			vz:     30 + rng.Float64()*50,
			ang:    rng.Float64() * math.Pi * 2,
			va:     (rng.Float64()*2 - 1) * 22,
			face:   1 + rng.Intn(6),
			target: r.Faces[i],
		}
		t.dice = append(t.dice, d)
	}
	return t
}

// settleTime simulates the whole throw and says when the last die stopped.
func (t *diceRoll) settleTime() float64 {
	for s := 0; s < 240*20; s++ {
		if t.step() {
			return t.simT
		}
	}
	return t.simT
}

// step advances the throw by one physics step; true once every die rests.
func (t *diceRoll) step() bool {
	dt := diceStep
	t.simT += dt
	t.clacks = 0
	half := dieSize / 2
	minX, maxX := float64(t.tray.Min.X)+half+2, float64(t.tray.Max.X)-half-2
	minY, maxY := float64(t.tray.Min.Y)+half+2, float64(t.tray.Max.Y)-half-2
	all := true
	for i := range t.dice {
		d := &t.dice[i]
		if d.settled {
			continue
		}
		// height: gravity and bounces
		d.vz -= 520 * dt
		d.z += d.vz * dt
		if d.z <= 0 {
			d.z = 0
			if d.vz < -40 {
				d.vz = -d.vz * 0.42
				d.va += (t.rng.Float64()*2 - 1) * 6
				t.clacks++
			} else {
				d.vz = 0
			}
		}
		// sliding: friction only on the felt
		if d.z == 0 {
			k := math.Exp(-t.fric * dt)
			d.vx *= k
			d.vy *= k
			d.va *= math.Exp(-t.fric * 1.3 * dt)
			// a little rolling resistance so they actually stop
			sp := math.Hypot(d.vx, d.vy)
			if sp > 0 {
				slow := math.Min(sp, 9*t.fric*dt)
				d.vx -= d.vx / sp * slow
				d.vy -= d.vy / sp * slow
			}
		}
		d.x += d.vx * dt
		d.y += d.vy * dt
		d.ang += d.va * dt
		if d.x < minX {
			d.x, d.vx = minX, -d.vx*0.6
			d.va += d.vy * 0.03
			t.clacks++
		}
		if d.x > maxX {
			d.x, d.vx = maxX, -d.vx*0.6
			d.va -= d.vy * 0.03
			t.clacks++
		}
		if d.y < minY {
			d.y, d.vy = minY, -d.vy*0.6
			d.va -= d.vx * 0.03
			t.clacks++
		}
		if d.y > maxY {
			d.y, d.vy = maxY, -d.vy*0.6
			d.va += d.vx * 0.03
			t.clacks++
		}
		// tumbling changes the face; slowing down, it shows where it lands
		sp := math.Hypot(d.vx, d.vy)
		d.rolled += sp*dt + math.Abs(d.vz)*dt*0.5
		if sp < 45 && d.z == 0 {
			d.face = d.target
		} else if d.rolled > dieSize*0.9 {
			d.rolled = 0
			f := 1 + t.rng.Intn(5)
			if f >= d.face {
				f++
			}
			d.face = f
		}
		if sp < 4 && math.Abs(d.va) < 0.4 && d.z == 0 && d.vz == 0 {
			d.vx, d.vy, d.va = 0, 0, 0
			d.face = d.target
			d.settled = true
		}
		if !d.settled {
			all = false
		}
	}
	// dice bump into each other
	r := dieSize * 1.05
	for i := range t.dice {
		for j := i + 1; j < len(t.dice); j++ {
			p, q := &t.dice[i], &t.dice[j]
			if math.Abs(p.z-q.z) > dieSize {
				continue
			}
			dx, dy := q.x-p.x, q.y-p.y
			dist := math.Hypot(dx, dy)
			if dist >= r || dist == 0 {
				continue
			}
			nx, ny := dx/dist, dy/dist
			push := (r - dist) / 2
			p.x, p.y = p.x-nx*push, p.y-ny*push
			q.x, q.y = q.x+nx*push, q.y+ny*push
			rel := (q.vx-p.vx)*nx + (q.vy-p.vy)*ny
			if rel < 0 {
				imp := -rel * 0.85
				p.vx, p.vy = p.vx-nx*imp, p.vy-ny*imp
				q.vx, q.vy = q.vx+nx*imp, q.vy+ny*imp
				p.va += imp * 0.04
				q.va -= imp * 0.04
				if imp > 30 {
					p.settled, q.settled = false, false
					t.clacks++
				}
			}
		}
	}
	return all
}

// pips for each face, in die coordinates from -1 to 1
var diePips = [7][][2]float64{
	1: {{0, 0}},
	2: {{-0.5, -0.5}, {0.5, 0.5}},
	3: {{-0.5, -0.5}, {0, 0}, {0.5, 0.5}},
	4: {{-0.5, -0.5}, {0.5, -0.5}, {-0.5, 0.5}, {0.5, 0.5}},
	5: {{-0.5, -0.5}, {0.5, -0.5}, {0, 0}, {-0.5, 0.5}, {0.5, 0.5}},
	6: {{-0.5, -0.55}, {0.5, -0.55}, {-0.5, 0}, {0.5, 0}, {-0.5, 0.55}, {0.5, 0.55}},
}

// drawDie rasterizes a turned die: rounded square, shaded edge, pips.
func (a *App) drawDie(d die, glow bool) {
	scale := 1 + d.z/70
	half := dieSize / 2 * scale
	cx, cy := d.x, d.y-d.z*0.6
	// shadow on the felt
	sh := int(half)
	fillRect(a.fb, image.Rect(int(d.x)-sh+2, int(d.y)-sh+3, int(d.x)+sh+2, int(d.y)+sh+3), color.RGBA{0, 0, 0, 70})
	cs, sn := math.Cos(-d.ang), math.Sin(-d.ang)
	ext := int(math.Ceil(half*1.45)) + 1
	body := color.RGBA{246, 242, 230, 255}
	edge := color.RGBA{190, 182, 165, 255}
	pip := color.RGBA{30, 24, 24, 255}
	if d.face == 6 && glow {
		pip = color.RGBA{200, 30, 30, 255}
	}
	if glow {
		body = color.RGBA{255, 246, 200, 255}
	}
	corner := half * 0.32
	for py := int(cy) - ext; py <= int(cy)+ext; py++ {
		for px := int(cx) - ext; px <= int(cx)+ext; px++ {
			fx, fy := float64(px)+0.5-cx, float64(py)+0.5-cy
			lx, ly := fx*cs-fy*sn, fx*sn+fy*cs
			ax, ay := math.Abs(lx), math.Abs(ly)
			if ax > half || ay > half {
				continue
			}
			// rounded corners
			if ax > half-corner && ay > half-corner {
				if math.Hypot(ax-(half-corner), ay-(half-corner)) > corner {
					continue
				}
			}
			c := body
			if ax > half-1.2 || ay > half-1.2 {
				c = edge
			}
			u, v := lx/half, ly/half
			for _, p := range diePips[min(max(d.face, 1), 6)] {
				if math.Hypot(u-p[0], v-p[1]) < 0.2 {
					c = pip
					break
				}
			}
			if image.Pt(px, py).In(a.fb.Rect) {
				a.fb.SetRGBA(px, py, c)
			}
		}
	}
}

// ---------------------------------------------------------------- screen

func (a *App) drawBeggar() {
	a.chatPreInput()
	a.computeLayout()
	a.drawTableBG()
	L := a.L
	bs, st := a.bs, a.st
	if bs == nil || st == nil {
		a.text(ScreenW/2, ScreenH/2-10, "Joining the table...", a.A.C("text"), alignCenter)
		a.drawOverlays()
		return
	}
	overlayOpen := a.overlay != ovNone
	updating := a.blocked
	a.blocked = overlayOpen || updating
	a.drawDealer(st)

	tray := a.diceTray()
	felt := a.A.C("felt_text")
	title := strings.ToUpper(bs.Name)
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(title)/2, tray.Min.Y-22, title, a.A.C("gold"))
	sub := fmt.Sprintf("ROLL FIVE SIXES TO WIN %s CHIPS  -  1 IN %s", commas(bs.Prize), commas(int64(bs.Odds)))
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(sub)/2, tray.Min.Y-12, fitText(a.A.Font, sub, ScreenW-20), felt)

	// the tray: a wooden box with a darker felt bed
	fillRect(a.fb, tray.Inset(-5), color.RGBA{92, 54, 28, 255})
	strokeRect(a.fb, tray.Inset(-5), color.RGBA{50, 28, 14, 255})
	fillRect(a.fb, image.Rect(tray.Min.X-4, tray.Min.Y-4, tray.Max.X+4, tray.Min.Y-3), color.RGBA{140, 90, 50, 255})
	fillRect(a.fb, tray, color.RGBA{18, 72, 44, 255})
	strokeRect(a.fb, tray, color.RGBA{10, 40, 24, 255})

	r := a.roll
	if r != nil {
		// run the physics up to now
		want := a.now.Sub(r.start).Seconds()
		for r.simT < want && !r.done {
			r.done = r.step()
			if r.clacks > 0 && r.simT-r.clackAt > 0.09 {
				r.clackAt = r.simT
				a.sound("chip")
			}
		}
		if r.done && r.win && !r.chimed {
			r.chimed = true
			a.sound("blackjack")
		}
		sixes := r.done && r.win
		// draw low dice first so the airborne ones are on top
		order := make([]int, len(r.dice))
		for i := range order {
			order[i] = i
		}
		for i := 1; i < len(order); i++ {
			for j := i; j > 0 && r.dice[order[j]].z < r.dice[order[j-1]].z; j-- {
				order[j], order[j-1] = order[j-1], order[j]
			}
		}
		for _, i := range order {
			a.drawDie(r.dice[i], sixes)
		}
		if r.done {
			msg, c := "", a.A.C("text")
			if r.win {
				msg, c = "FIVE SIXES!", a.A.C("gold")
				if (a.frame/10)%2 == 0 {
					c = a.A.C("highlight")
				}
				tw := a.A.Font.Width(msg)*2 + 16
				fillRect(a.fb, image.Rect(ScreenW/2-tw/2, tray.Min.Y+tray.Dy()/2-12, ScreenW/2+tw/2, tray.Min.Y+tray.Dy()/2+12), color.RGBA{0, 0, 0, 150})
				a.textBig(ScreenW/2, tray.Min.Y+tray.Dy()/2-8, msg, c, 2, alignCenter)
			} else {
				six := 0
				for _, f := range r.faces {
					if f == 6 {
						six++
					}
				}
				msg = []string{"No sixes.", "One six.", "Two sixes.", "Three sixes!", "Four sixes! So close."}[min(six, 4)]
				if six >= 3 {
					c = a.A.C("highlight")
				}
				a.textOutlined(ScreenW/2, tray.Max.Y+8, msg, c, alignCenter)
			}
		}
	} else {
		a.text(ScreenW/2, tray.Min.Y+tray.Dy()/2-4, "Press ROLL to throw the dice", a.A.C("felt_text"), alignCenter)
	}

	// what's been happening, and who else is here
	y := tray.Max.Y + 20
	for i, f := range bs.Feed {
		if i < len(bs.Feed)-3 {
			continue
		}
		a.text(ScreenW/2, y, fitText(a.A.Font, f, ScreenW-20), a.A.C("gold"), alignCenter)
		y += 10
	}
	var others []string
	for _, p := range bs.Players {
		if !strings.EqualFold(p, a.username) {
			others = append(others, p)
		}
	}
	if len(others) > 0 && y < L.hudTop-12 {
		a.text(ScreenW/2, y, fitText(a.A.Font, "Also begging: "+strings.Join(others, ", "), ScreenW-20), a.A.C("text_dim"), alignCenter)
	}

	a.drawSpeech(st)
	a.drawLogButton(st)
	a.drawTopBar(st)
	a.drawBeggarHUD(bs)
	a.drawChatBar(st)
	a.blocked = updating
	a.drawOverlays()
}

func (a *App) drawBeggarHUD(bs *proto.BeggarState) {
	L := a.L
	hudTop := L.hudTop
	a.panel(image.Rect(0, hudTop, ScreenW, ScreenH))
	ar := image.Rect(10, hudTop+12, 42, hudTop+44)
	a.drawFrame(a.profile.Frame, ar.Min.X, ar.Min.Y)
	blit(a.fb, a.avatarImg(a.username, a.profile.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
	a.text(48, hudTop+7, fitText(a.A.Font, a.username, 110), a.A.C("gold"), alignLeft)
	a.text(48, hudTop+18, "CHIPS", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+18, commas(bs.Balance), a.A.C("text"), alignLeft)
	a.text(48, hudTop+29, "ROLLS", a.A.C("text_dim"), alignLeft)
	a.text(80, hudTop+29, commas(bs.Rolls), a.A.C("text"), alignLeft)
	if bs.Wins > 0 {
		a.text(48, hudTop+44, fmt.Sprintf("five sixes x%d", bs.Wins), a.A.C("win"), alignLeft)
	}

	cx0, cx1 := L.cx0, L.cx1
	top := L.center
	if L.portrait {
		fillRect(a.fb, image.Rect(6, top-3, ScreenW-6, top-2), color.RGBA{0, 0, 0, 90})
	}
	rolling := a.roll != nil && !a.roll.done
	can := bs.CanRoll && !rolling
	bw := min(160, cx1-cx0)
	br := image.Rect((cx0+cx1)/2-bw/2, top+10, (cx0+cx1)/2+bw/2, top+34)
	label := "ROLL THE DICE"
	if rolling || bs.Rolling {
		label = "ROLLING..."
	}
	if a.button("bg_roll", br, label, btn{enabled: can, style: styleGold, key: KeySpace}) {
		a.send(proto.CDice, nil)
	}
	switch {
	case bs.Reason != "" && !bs.Rolling && !rolling:
		lines := a.wrap(bs.Reason, cx1-cx0-10)
		for i, l := range lines {
			if i < 2 {
				a.text((cx0+cx1)/2, top+38+i*9, l, a.A.C("lose"), alignCenter)
			}
		}
	case !a.isTouch():
		a.text((cx0+cx1)/2, top+40, "SPACE rolls", a.A.C("text_dim"), alignCenter)
	}

	rx0, rx1 := L.rx0, L.rx1
	a.text(rx0, hudTop+7, "BEGGING", a.A.C("gold"), alignLeft)
	a.text(rx0, hudTop+18, fmt.Sprintf("%s rolls here", commas(bs.TableRolls)), a.A.C("text_dim"), alignLeft)
	half := (rx1 - rx0 - 4) / 2
	if a.button("settings", image.Rect(rx1-half, hudTop+32, rx1, hudTop+50), "SETTINGS", btn{enabled: true, hotkey: 'o'}) {
		a.overlay = ovSettings
	}
}
