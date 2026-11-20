// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/dice"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

// The Beggars Table: one shared tray of dice for everyone at the table.
// The server throws the dice in a 3D physics simulation and sends the
// recorded throw; the client rattles them for a moment, then replays the
// throw exactly, drawn as shaded cubes in a wooden tray. Whatever faces the
// simulation came to rest on is the result.

type diceRoll struct {
	id      int
	by      string
	track   *dice.Track
	faces   []int
	win     bool
	start   time.Time // when the rattle began
	shake   float64   // seconds of rattle before the throw
	hits    []float64 // knock times into the throw
	hitsK   []uint8
	nextHit int
	done    bool
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
	w := min(330, ScreenW-30)
	h := 128
	top := a.L.cardsY + 30
	if a.L.portrait {
		top = a.L.msgY + 30
		h = 150
	}
	return image.Rect(ScreenW/2-w/2, top, ScreenW/2+w/2, top+h)
}

func (a *App) onDice(r proto.DiceRoll) {
	tr, err := dice.ReadTrack(r.Dice, r.Track)
	if err != nil || len(r.Faces) != r.Dice {
		return
	}
	dr := &diceRoll{id: r.ID, by: r.By, track: tr, faces: r.Faces, win: r.Win,
		start: a.now.Add(-time.Duration(r.Elapsed) * time.Millisecond), shake: float64(r.ShakeMS) / 1000}
	for i, h := range r.Hits {
		dr.hits = append(dr.hits, float64(h)/1000)
		k := uint8(255)
		if i < len(r.HitsK) {
			k = r.HitsK[i]
		}
		dr.hitsK = append(dr.hitsK, k)
	}
	// someone arriving mid-throw doesn't hear the knocks already past
	for dr.nextHit < len(dr.hits) && dr.hits[dr.nextHit] < a.now.Sub(dr.start).Seconds()-dr.shake {
		dr.nextHit++
	}
	a.roll = dr
	if r.Elapsed < r.ShakeMS {
		a.sound("shuffle")
	}
}

// poses is every die's pose at the moment, and how far into the throw it
// is (negative while the dice are still rattling in the cup).
func (r *diceRoll) poses(now time.Time) ([]dice.Pose, float64) {
	t := now.Sub(r.start).Seconds() - r.shake
	if t >= 0 {
		return r.track.At(t), t
	}
	// rattling: the opening poses, shaken
	ps := r.track.At(0)
	ph := now.Sub(r.start).Seconds()
	for i := range ps {
		f := float64(i)*1.7 + ph*31
		ps[i].Pos = ps[i].Pos.Add(dice.V3{X: 0.18 * math.Sin(f), Y: 0.12 * math.Cos(f*1.3), Z: 0.25 * math.Abs(math.Sin(f*0.9))})
		ax := dice.V3{X: math.Sin(f * 0.7), Y: math.Cos(f * 0.5), Z: 0.4}.Norm()
		ang := ph * (9 + float64(i))
		sn, cs := math.Sincos(ang / 2)
		ps[i].Rot = dice.Quat{W: cs, X: ax.X * sn, Y: ax.Y * sn, Z: ax.Z * sn}.Mul(ps[i].Rot)
	}
	return ps, t
}

// ---------------------------------------------------------------- 3D

// camera looks down into the tray from the front.
type camera struct {
	pos, right, up, fwd dice.V3
	f, ox, oy           float64
}

func newCamera(view image.Rectangle) camera {
	const pitch, dist = 0.98, 34.0 // about 56 degrees down
	c := dice.V3{X: dice.TrayW / 2, Y: dice.TrayD / 2}
	sp, cp := math.Sincos(pitch)
	cam := camera{
		pos:   c.Add(dice.V3{Y: -dist * cp, Z: dist * sp}),
		right: dice.V3{X: 1},
		fwd:   dice.V3{Y: cp, Z: -sp},
		up:    dice.V3{Y: sp, Z: cp},
		f:     1,
	}
	// fit the tray, walls and all, into view
	lo, hi := [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	for _, x := range []float64{-0.4, dice.TrayW + 0.4} {
		for _, y := range []float64{-0.4, dice.TrayD + 0.4} {
			for _, z := range []float64{0, dice.WallH} {
				sx, sy, _ := cam.project(dice.V3{X: x, Y: y, Z: z})
				lo[0], lo[1] = math.Min(lo[0], sx), math.Min(lo[1], sy)
				hi[0], hi[1] = math.Max(hi[0], sx), math.Max(hi[1], sy)
			}
		}
	}
	cam.f = math.Min(float64(view.Dx())/(hi[0]-lo[0]), float64(view.Dy())/(hi[1]-lo[1]))
	cam.ox = float64(view.Min.X+view.Max.X)/2 - (lo[0]+hi[0])/2*cam.f
	cam.oy = float64(view.Min.Y+view.Max.Y)/2 - (lo[1]+hi[1])/2*cam.f
	return cam
}

// project gives a point's screen position and its distance from the camera.
func (c camera) project(p dice.V3) (float64, float64, float64) {
	d := p.Sub(c.pos)
	z := d.Dot(c.fwd)
	return c.ox + c.f*d.Dot(c.right)/z, c.oy - c.f*d.Dot(c.up)/z, z
}

func (c camera) pt(p dice.V3) [2]float64 {
	x, y, _ := c.project(p)
	return [2]float64{x, y}
}

var lightDir = dice.V3{X: -0.35, Y: -0.55, Z: 1}.Norm()

func shade(c color.RGBA, k float64) color.RGBA {
	f := func(v uint8) uint8 { return uint8(math.Min(255, float64(v)*k)) }
	return color.RGBA{f(c.R), f(c.G), f(c.B), c.A}
}

// fillPoly fills a convex polygon, blending if c is see-through.
func fillPoly(dst *image.RGBA, pts [][2]float64, c color.RGBA) {
	if len(pts) < 3 {
		return
	}
	y0, y1 := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		y0, y1 = math.Min(y0, p[1]), math.Max(y1, p[1])
	}
	b := dst.Rect
	for y := max(int(math.Ceil(y0-0.5)), b.Min.Y); y <= min(int(math.Floor(y1-0.5)), b.Max.Y-1); y++ {
		fy := float64(y) + 0.5
		xl, xr := math.Inf(1), math.Inf(-1)
		for i := range pts {
			p, q := pts[i], pts[(i+1)%len(pts)]
			if (p[1] <= fy) == (q[1] <= fy) {
				continue
			}
			x := p[0] + (fy-p[1])*(q[0]-p[0])/(q[1]-p[1])
			xl, xr = math.Min(xl, x), math.Max(xr, x)
		}
		if xl > xr {
			continue
		}
		for x := max(int(math.Ceil(xl-0.5)), b.Min.X); x <= min(int(math.Floor(xr-0.5)), b.Max.X-1); x++ {
			o := dst.PixOffset(x, y)
			px := dst.Pix[o : o+4 : o+4]
			if c.A == 255 {
				px[0], px[1], px[2], px[3] = c.R, c.G, c.B, 255
				continue
			}
			al := uint32(c.A)
			px[0] = uint8((uint32(c.R)*al + uint32(px[0])*(255-al)) / 255)
			px[1] = uint8((uint32(c.G)*al + uint32(px[1])*(255-al)) / 255)
			px[2] = uint8((uint32(c.B)*al + uint32(px[2])*(255-al)) / 255)
		}
	}
}

// strokePoly outlines a polygon one pixel wide.
func strokePoly(dst *image.RGBA, pts [][2]float64, c color.RGBA) {
	for i := range pts {
		p, q := pts[i], pts[(i+1)%len(pts)]
		n := int(math.Max(math.Abs(q[0]-p[0]), math.Abs(q[1]-p[1]))) + 1
		for s := 0; s <= n; s++ {
			t := float64(s) / float64(n)
			x, y := int(math.Floor(p[0]+(q[0]-p[0])*t)), int(math.Floor(p[1]+(q[1]-p[1])*t))
			if image.Pt(x, y).In(dst.Rect) {
				dst.SetRGBA(x, y, c)
			}
		}
	}
}

// hull is the convex hull of points, in order.
func hull(ps [][2]float64) [][2]float64 {
	sort.Slice(ps, func(i, j int) bool { return ps[i][0] < ps[j][0] || ps[i][0] == ps[j][0] && ps[i][1] < ps[j][1] })
	cross := func(o, a, b [2]float64) float64 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }
	var h [][2]float64
	for pass := 0; pass < 2; pass++ {
		start := len(h)
		for _, p := range ps {
			for len(h) >= start+2 && cross(h[len(h)-2], h[len(h)-1], p) <= 0 {
				h = h[:len(h)-1]
			}
			h = append(h, p)
		}
		h = h[:len(h)-1]
		for i, j := 0, len(ps)-1; i < j; i, j = i+1, j-1 {
			ps[i], ps[j] = ps[j], ps[i]
		}
	}
	return h
}

// a die's faces: the face number, its corners on the unit cube (in order
// around the face) and the directions across it for laying out pips
var dieFaces = func() (fs [7]struct {
	u, v dice.V3
}) {
	for f := 1; f <= 6; f++ {
		n := dice.FaceAxes[f]
		u := dice.V3{X: 1}
		if math.Abs(n.X) > 0.5 {
			u = dice.V3{Y: 1}
		}
		fs[f].u, fs[f].v = u, n.Cross(u)
	}
	return
}()

var pipSpots = map[int][][2]float64{
	1: {{0, 0}},
	2: {{-1, -1}, {1, 1}},
	3: {{-1, -1}, {0, 0}, {1, 1}},
	4: {{-1, -1}, {1, -1}, {-1, 1}, {1, 1}},
	5: {{-1, -1}, {1, -1}, {0, 0}, {-1, 1}, {1, 1}},
	6: {{-1, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {1, 1}},
}

type dieLook struct {
	body, pip color.RGBA
}

// drawDie3D draws one die: the faces turned toward the camera, shaded by
// the light, with their pips.
func drawDie3D(dst *image.RGBA, cam camera, p dice.Pose, look dieLook) {
	for f := 1; f <= 6; f++ {
		n := p.Rot.Rotate(dice.FaceAxes[f])
		centre := p.Pos.Add(n.Mul(0.5))
		if n.Dot(cam.pos.Sub(centre)) <= 0 {
			continue
		}
		u, v := p.Rot.Rotate(dieFaces[f].u), p.Rot.Rotate(dieFaces[f].v)
		quad := [][2]float64{
			cam.pt(centre.Add(u.Mul(0.5)).Add(v.Mul(0.5))),
			cam.pt(centre.Add(u.Mul(-0.5)).Add(v.Mul(0.5))),
			cam.pt(centre.Add(u.Mul(-0.5)).Add(v.Mul(-0.5))),
			cam.pt(centre.Add(u.Mul(0.5)).Add(v.Mul(-0.5))),
		}
		k := 0.58 + 0.5*math.Max(0, n.Dot(lightDir))
		body := shade(look.body, k)
		fillPoly(dst, quad, body)
		strokePoly(dst, quad, shade(look.body, k*0.72))
		pip := shade(look.pip, 0.7+0.3*k)
		for _, s := range pipSpots[f] {
			c := centre.Add(u.Mul(s[0] * 0.25)).Add(v.Mul(s[1] * 0.25))
			var dot [][2]float64
			for i := 0; i < 8; i++ {
				sn, cs := math.Sincos(float64(i) * math.Pi / 4)
				dot = append(dot, cam.pt(c.Add(u.Mul(cs*0.095)).Add(v.Mul(sn*0.095))))
			}
			fillPoly(dst, dot, pip)
		}
	}
}

// drawTray3D draws the wooden tray around the felt: the back and side
// walls (front=false) or the front wall, which goes over the dice.
func drawTray3D(dst *image.RGBA, cam camera, front bool) {
	wood := color.RGBA{116, 70, 36, 255}
	box := func(x0, y0, x1, y1 float64) {
		z0, z1 := 0.0, dice.WallH
		v := func(x, y, z float64) dice.V3 { return dice.V3{X: x, Y: y, Z: z} }
		faces := []struct {
			n  dice.V3
			ps []dice.V3
		}{
			{dice.V3{Z: 1}, []dice.V3{v(x0, y0, z1), v(x1, y0, z1), v(x1, y1, z1), v(x0, y1, z1)}},
			{dice.V3{Y: -1}, []dice.V3{v(x0, y0, z0), v(x1, y0, z0), v(x1, y0, z1), v(x0, y0, z1)}},
			{dice.V3{Y: 1}, []dice.V3{v(x0, y1, z0), v(x1, y1, z0), v(x1, y1, z1), v(x0, y1, z1)}},
			{dice.V3{X: -1}, []dice.V3{v(x0, y0, z0), v(x0, y1, z0), v(x0, y1, z1), v(x0, y0, z1)}},
			{dice.V3{X: 1}, []dice.V3{v(x1, y0, z0), v(x1, y1, z0), v(x1, y1, z1), v(x1, y0, z1)}},
		}
		for _, f := range faces {
			mid := f.ps[0].Add(f.ps[2]).Mul(0.5)
			if f.n.Dot(cam.pos.Sub(mid)) <= 0 {
				continue
			}
			var pts [][2]float64
			for _, p := range f.ps {
				pts = append(pts, cam.pt(p))
			}
			k := 0.6 + 0.45*math.Max(0, f.n.Dot(lightDir))
			fillPoly(dst, pts, shade(wood, k))
			strokePoly(dst, pts, shade(wood, k*0.7))
		}
	}
	const t = 0.45 // wall thickness
	W, D := dice.TrayW, dice.TrayD
	if front {
		box(-t, -t, W+t, 0)
		return
	}
	box(-t, D, W+t, D+t)
	box(-t, 0, 0, D)
	box(W, 0, W+t, D)
}

// drawDiceTray draws the tray and the dice in it.
func (a *App) drawDiceTray(view image.Rectangle) {
	cam := newCamera(view)
	W, D := dice.TrayW, dice.TrayD
	felt := [][2]float64{cam.pt(dice.V3{}), cam.pt(dice.V3{X: W}), cam.pt(dice.V3{X: W, Y: D}), cam.pt(dice.V3{Y: D})}
	drawTray3D(a.fb, cam, false)
	fillPoly(a.fb, felt, color.RGBA{20, 78, 48, 255})
	r := a.roll
	if r == nil {
		drawTray3D(a.fb, cam, true)
		return
	}
	poses, _ := r.poses(a.now)
	// shadows on the felt, cast away from the light
	for _, p := range poses {
		var pts [][2]float64
		for _, c := range []dice.V3{{X: 1, Y: 1, Z: 1}, {X: -1, Y: 1, Z: 1}, {X: 1, Y: -1, Z: 1}, {X: -1, Y: -1, Z: 1},
			{X: 1, Y: 1, Z: -1}, {X: -1, Y: 1, Z: -1}, {X: 1, Y: -1, Z: -1}, {X: -1, Y: -1, Z: -1}} {
			w := p.Pos.Add(p.Rot.Rotate(c.Mul(0.5)))
			g := w.Sub(lightDir.Mul(w.Z / lightDir.Z))
			g.X, g.Y = math.Max(0, math.Min(W, g.X)), math.Max(0, math.Min(D, g.Y))
			pts = append(pts, cam.pt(g))
		}
		fillPoly(a.fb, hull(pts), color.RGBA{0, 0, 0, 80})
	}
	// far dice first
	order := make([]int, len(poses))
	dist := make([]float64, len(poses))
	for i, p := range poses {
		order[i] = i
		_, _, dist[i] = cam.project(p.Pos)
	}
	sort.Slice(order, func(i, j int) bool { return dist[order[i]] > dist[order[j]] })
	g := a.bsGoal()
	for _, i := range order {
		look := dieLook{body: color.RGBA{246, 242, 230, 255}, pip: color.RGBA{30, 24, 24, 255}}
		if r.done {
			switch {
			case r.win:
				look.body = color.RGBA{255, 238, 170, 255}
				look.pip = color.RGBA{190, 30, 30, 255}
			case g.Kind == dice.GoalFaces && g.Counts(r.faces[i]):
				look.pip = color.RGBA{190, 30, 30, 255}
			}
		}
		drawDie3D(a.fb, cam, poses[i], look)
	}
	drawTray3D(a.fb, cam, true)
}

// bsGoal is the table's goal as the server described it.
func (a *App) bsGoal() dice.Goal {
	if a.bs == nil {
		return dice.Goal{}
	}
	return dice.Goal{Dice: a.bs.Dice, Kind: a.bs.GoalKind, Need: a.bs.GoalNeed, Face: a.bs.GoalFace, P: a.bs.Chance}
}

// pct writes a chance as a short percentage: 50%, 2.8%, 0.097%.
func pct(p float64) string {
	v := p * 100
	switch {
	case v >= 10:
		return fmt.Sprintf("%.0f%%", v)
	case v <= 0:
		return "0%"
	}
	digits := max(0, 1-int(math.Floor(math.Log10(v))))
	return strconv.FormatFloat(v, 'f', digits, 64) + "%"
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
	g := a.bsGoal()
	head := fmt.Sprintf("%s  -  1 IN %s (%s)", strings.ToUpper(bs.Name), commas(int64(math.Round(g.Odds()))), pct(g.P))
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(fitText(a.A.Font, head, ScreenW-20))/2, tray.Min.Y-22, fitText(a.A.Font, head, ScreenW-20), a.A.C("gold"))
	noun := "DICE"
	if g.Dice == 1 {
		noun = "DIE"
	}
	sub := fmt.Sprintf("THROW %d %s: %s WINS %s CHIPS FOR EVERYONE HERE", g.Dice, noun, g.Describe(), commas(bs.Prize))
	sub = fitText(a.A.Font, sub, ScreenW-20)
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(sub)/2, tray.Min.Y-12, sub, felt)

	r := a.roll
	if r != nil {
		_, t := r.poses(a.now)
		for r.nextHit < len(r.hits) && r.hits[r.nextHit] <= t {
			if r.hitsK[r.nextHit] > 40 {
				a.sound("chip")
			}
			r.nextHit++
		}
		if !r.done && t >= r.track.Length()+0.15 {
			r.done = true
		}
		if r.done && r.win && !r.chimed {
			r.chimed = true
			a.sound("blackjack")
		}
	}
	a.drawDiceTray(tray)
	switch {
	case r == nil:
		a.textOutlined(ScreenW/2, tray.Max.Y+8, "Anyone under the limit can throw", felt, alignCenter)
	case !r.done:
		a.textOutlined(ScreenW/2, tray.Max.Y+8, fitText(a.A.Font, r.by+" throws...", ScreenW-20), a.A.C("text"), alignCenter)
	case r.win:
		msg, c := "WINNER!", a.A.C("gold")
		if (a.frame/10)%2 == 0 {
			c = a.A.C("highlight")
		}
		tw := a.A.Font.Width(msg)*2 + 16
		mid := tray.Min.Y + tray.Dy()/2
		fillRect(a.fb, image.Rect(ScreenW/2-tw/2, mid-12, ScreenW/2+tw/2, mid+12), color.RGBA{0, 0, 0, 150})
		a.textBig(ScreenW/2, mid-8, msg, c, 2, alignCenter)
		a.textOutlined(ScreenW/2, tray.Max.Y+8, fitText(a.A.Font, fmt.Sprintf("%s rolled %s", r.by, a.scoreText(g, r.faces)), ScreenW-20), a.A.C("gold"), alignCenter)
	default:
		got := g.Score(r.faces)
		c := a.A.C("text")
		if float64(got) >= float64(g.Need)*0.85 {
			c = a.A.C("highlight")
		}
		a.textOutlined(ScreenW/2, tray.Max.Y+8, fitText(a.A.Font, fmt.Sprintf("%s rolled %s. Needed %d.", r.by, a.scoreText(g, r.faces), g.Need), ScreenW-20), c, alignCenter)
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

// scoreText says what a throw made: "a total of 31" or "6 counting dice".
func (a *App) scoreText(g dice.Goal, faces []int) string {
	n := g.Score(faces)
	if g.Kind != dice.GoalFaces {
		return fmt.Sprintf("a total of %d", n)
	}
	word := "sixes"
	switch g.Face {
	case 6:
		if n == 1 {
			word = "six"
		}
	case 5:
		word = "5s or 6s"
	default:
		word = fmt.Sprintf("dice of %d or higher", g.Face)
	}
	return fmt.Sprintf("%d %s", n, word)
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
		a.text(48, hudTop+44, fmt.Sprintf("wins x%d", bs.Wins), a.A.C("win"), alignLeft)
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
	label := "THROW THE DICE"
	if rolling || bs.Rolling {
		label = "ROLLING..."
	}
	can = can && !bs.Rolling
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
		a.text((cx0+cx1)/2, top+40, "SPACE throws", a.A.C("text_dim"), alignCenter)
	}

	rx0, rx1 := L.rx0, L.rx1
	a.text(rx0, hudTop+7, "BEGGING", a.A.C("gold"), alignLeft)
	a.text(rx0, hudTop+18, fmt.Sprintf("%s rolls here", commas(bs.TableRolls)), a.A.C("text_dim"), alignLeft)
	half := (rx1 - rx0 - 4) / 2
	if a.button("settings", image.Rect(rx1-half, hudTop+32, rx1, hudTop+50), "SETTINGS", btn{enabled: true, hotkey: 'o'}) {
		a.overlay = ovSettings
	}
}
