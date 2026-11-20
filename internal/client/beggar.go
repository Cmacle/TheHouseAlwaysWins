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
// recorded throw; the client shakes them in a cup, tips it over, then
// replays the throw exactly, drawn as shaded cubes in a wooden tray. Whatever faces the
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
	paid    map[string]bool // who was under the limit when it was thrown
	sides   int
	goal    dice.Goal // the goal it was thrown for (a win brings a new one)
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
	x0 := ScreenW/2 - w/2
	if a.L.portrait {
		top = a.L.msgY + 30
		h = 150
		w = ScreenW - 92 // room on the right for the cup between throws
		x0 = 12
	}
	return image.Rect(x0, top, x0+w, top+h)
}

func (a *App) onDice(r proto.DiceRoll) {
	tr, err := dice.ReadTrack(r.Dice, r.Track)
	if err != nil || len(r.Faces) != r.Dice {
		return
	}
	dr := &diceRoll{id: r.ID, by: r.By, track: tr, faces: r.Faces, win: r.Win, sides: r.Sides, goal: a.bsGoal(),
		start: a.now.Add(-time.Duration(r.Elapsed) * time.Millisecond), shake: float64(r.ShakeMS) / 1000}
	dr.paid = map[string]bool{}
	if a.bs != nil {
		for _, sv := range a.bs.Seats {
			if sv.CanRoll {
				dr.paid[strings.ToLower(sv.Username)] = true
			}
		}
	}
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

// dieColors gives each kind of die its own colour.
func dieColors(sides int) dieLook {
	switch sides {
	case 8:
		return dieLook{color.RGBA{176, 208, 250, 255}, color.RGBA{20, 34, 80, 255}}
	case 12:
		return dieLook{color.RGBA{180, 230, 180, 255}, color.RGBA{18, 60, 26, 255}}
	case 20:
		return dieLook{color.RGBA{250, 196, 150, 255}, color.RGBA{90, 30, 10, 255}}
	}
	return dieLook{color.RGBA{246, 242, 230, 255}, color.RGBA{30, 24, 24, 255}}
}

// drawDie3D draws one die: the faces turned toward the camera, shaded by
// the light, with pips (d6) or the number on top.
func (a *App) drawDie3D(cam camera, p dice.Pose, sh *dice.Shape, look dieLook) {
	top, topZ := 0, -2.0
	for _, f := range sh.Faces {
		if z := p.Rot.Rotate(f.N).Z; z > topZ {
			top, topZ = f.Val, z
		}
	}
	for _, f := range sh.Faces {
		n := p.Rot.Rotate(f.N)
		centre := p.Pos.Add(n.Mul(f.D))
		toCam := cam.pos.Sub(centre)
		facing := n.Dot(toCam) / toCam.Len()
		if facing <= 0 {
			continue
		}
		poly := make([][2]float64, 0, len(f.Verts))
		for _, vi := range f.Verts {
			poly = append(poly, cam.pt(p.Pos.Add(p.Rot.Rotate(sh.Verts[vi]))))
		}
		k := 0.58 + 0.5*math.Max(0, n.Dot(lightDir))
		fillPoly(a.fb, poly, shade(look.body, k))
		strokePoly(a.fb, poly, shade(look.body, k*0.72))
		ink := shade(look.pip, 0.7+0.3*k)
		if sh.Sides == 6 {
			// pips, laid out across the face
			u := p.Rot.Rotate(sh.Verts[f.Verts[0]]).Sub(n.Mul(f.D)).Norm()
			v := n.Cross(u)
			// turn the pip grid to line up with the square's sides
			u, v = u.Add(v).Norm(), v.Sub(u).Norm()
			for _, s := range pipSpots[f.Val] {
				c := centre.Add(u.Mul(s[0] * 0.25)).Add(v.Mul(s[1] * 0.25))
				var dot [][2]float64
				for i := 0; i < 8; i++ {
					sn, cs := math.Sincos(float64(i) * math.Pi / 4)
					dot = append(dot, cam.pt(c.Add(u.Mul(cs*0.095)).Add(v.Mul(sn*0.095))))
				}
				fillPoly(a.fb, dot, ink)
			}
			continue
		}
		if f.Val == top && facing > 0.2 {
			// only the top face is numbered, the way the die is read
			c := cam.pt(centre)
			txt := strconv.Itoa(f.Val)
			if f.Val == 6 || f.Val == 9 {
				txt += "." // tells 6 from 9, like real dice
			}
			a.A.Font.Draw(a.fb, int(c[0])-a.A.Font.Width(txt)/2, int(c[1])-3, txt, ink)
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

// The dice cup: shaken upright over the right end of the tray, then tipped
// so its mouth points left and the dice pour out, then lifted away.
const (
	cupR    = 1.75 // radius, die widths
	cupH    = 3.4  // height
	cupTip  = 0.35 // seconds to tip it over
	cupTilt = 1.85 // radians it tips to (about 106 degrees)
)

// cupRest is where the cup stands between throws: on the table just right
// of the tray, mouth up.
var cupRest = dice.V3{X: dice.TrayW + 0.45 + 0.35 + cupR, Y: dice.TrayD * 0.55, Z: 0}

// cupPose is where the cup's base is, which way its mouth points, and how
// solid it is, t seconds into the throw (negative while shaking). It's
// picked up, shaken, tipped to pour, then set back down upright in the same
// spot, so it never leaves the table.
func cupPose(t float64, shake float64) (base, axis dice.V3, alpha float64) {
	up := dice.V3{Z: 1}
	tilt := dice.V3{X: -math.Sin(cupTilt), Z: math.Cos(cupTilt)}
	tippedBase := dice.CupMouth.Sub(tilt.Mul(cupH))
	held := dice.V3{X: dice.TrayW - 1.6, Y: dice.TrayD / 2, Z: 2.2} // shaken here
	ease := func(f float64) float64 { f = math.Max(0, math.Min(1, f)); return f * f * (3 - 2*f) }
	tipTo := func(f float64) dice.V3 { ang := cupTilt * f; return dice.V3{X: -math.Sin(ang), Z: math.Cos(ang)} }
	switch {
	case t < -cupTip:
		// picked up from the table, then shaken: quick jiggles side to
		// side and up and down
		since := t + shake
		lift := ease(since / 0.3)
		k := lift
		j := dice.V3{X: 0.35 * math.Sin(since*38), Y: 0.2 * math.Sin(since*29+1), Z: 0.3 * math.Abs(math.Sin(since*19))}.Mul(k)
		wob := 0.16 * math.Sin(since*38+0.6) * k
		return cupRest.Add(held.Sub(cupRest).Mul(lift)).Add(j), dice.V3{X: -math.Sin(wob), Z: math.Cos(wob)}, 1
	case t < 0:
		// tipping over to pour
		f := ease((t + cupTip) / cupTip)
		return held.Add(tippedBase.Sub(held).Mul(f)), tipTo(f), 1
	case t < 0.45:
		return tippedBase, tilt, 1
	case t < 1.25:
		// turned back upright and set down where it started
		f := ease((t - 0.45) / 0.8)
		arc := dice.V3{Z: 1.2 * math.Sin(f*math.Pi)} // lifted a little on the way
		return tippedBase.Add(cupRest.Sub(tippedBase).Mul(f)).Add(arc), tipTo(1 - f), 1
	}
	return cupRest, up, 1
}

// drawCup draws the cup as a shaded leather cylinder with a brass rim.
func drawCup(dst *image.RGBA, cam camera, base, axis dice.V3, alpha float64) {
	if alpha <= 0 {
		return
	}
	const sides = 16
	// two directions across the cup
	u := axis.Cross(dice.V3{Y: 1})
	if u.Len() < 0.1 {
		u = axis.Cross(dice.V3{X: 1})
	}
	u = u.Norm()
	v := axis.Cross(u).Norm()
	mouth := base.Add(axis.Mul(cupH))
	ring := func(c dice.V3, r float64, i int) dice.V3 {
		sn, cs := math.Sincos(float64(i) * 2 * math.Pi / sides)
		return c.Add(u.Mul(cs * r)).Add(v.Mul(sn * r))
	}
	a8 := uint8(255 * alpha)
	leather := color.RGBA{142, 38, 34, a8}
	brass := color.RGBA{214, 170, 70, a8}
	tint := func(c color.RGBA, k float64) color.RGBA { out := shade(c, k); out.A = c.A; return out }
	var polys []struct {
		pts   [][2]float64
		depth float64
		c     color.RGBA
	}
	add := func(ws []dice.V3, n dice.V3, c color.RGBA, inside bool) {
		mid := dice.V3{}
		for _, w := range ws {
			mid = mid.Add(w)
		}
		mid = mid.Mul(1 / float64(len(ws)))
		facing := n.Dot(cam.pos.Sub(mid)) > 0
		if facing == inside {
			return
		}
		var pts [][2]float64
		for _, w := range ws {
			pts = append(pts, cam.pt(w))
		}
		_, _, d := cam.project(mid)
		if c == brass {
			d -= 0.05 // bands sit on the leather
		}
		k := 0.55 + 0.5*math.Max(0, n.Dot(lightDir))
		if inside {
			k = 0.35 + 0.2*math.Max(0, n.Mul(-1).Dot(lightDir))
		}
		polys = append(polys, struct {
			pts   [][2]float64
			depth float64
			c     color.RGBA
		}{pts, d, tint(c, k)})
	}
	for i := 0; i < sides; i++ {
		p0, p1 := ring(base, cupR*0.92, i), ring(base, cupR*0.92, i+1)
		q0, q1 := ring(mouth, cupR, i), ring(mouth, cupR, i+1)
		mid := ring(base.Add(axis.Mul(cupH/2)), 1, i).Add(ring(base.Add(axis.Mul(cupH/2)), 1, i+1)).Mul(0.5)
		n := mid.Sub(base.Add(axis.Mul(cupH / 2))).Norm()
		add([]dice.V3{p0, p1, q1, q0}, n, leather, false) // outside
		add([]dice.V3{p0, p1, q1, q0}, n, leather, true)  // the inside, seen through the mouth
		// brass bands near the mouth and the base
		r0, r1 := ring(mouth.Sub(axis.Mul(0.35)), cupR*0.985, i), ring(mouth.Sub(axis.Mul(0.35)), cupR*0.985, i+1)
		add([]dice.V3{r0, r1, q1, q0}, n, brass, false)
		b0, b1 := ring(base.Add(axis.Mul(0.3)), cupR*0.93, i), ring(base.Add(axis.Mul(0.3)), cupR*0.93, i+1)
		add([]dice.V3{p0, p1, b1, b0}, n, brass, false)
	}
	var bottom []dice.V3
	for i := 0; i < sides; i++ {
		bottom = append(bottom, ring(base, cupR*0.92, i))
	}
	add(bottom, axis.Mul(-1), leather, false) // the base, from outside
	add(bottom, axis.Mul(-1), leather, true)  // the floor of the cup, seen inside
	// inside surfaces first, then the outside, each far to near
	sort.SliceStable(polys, func(i, j int) bool { return polys[i].depth > polys[j].depth })
	for _, p := range polys {
		fillPoly(dst, p.pts, p.c)
	}
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
		drawCup(a.fb, cam, cupRest, dice.V3{Z: 1}, 1)
		return
	}
	poses, t := r.poses(a.now)
	base, axis, alpha := cupPose(t, r.shake)
	if t < 0 {
		// the dice are in the cup
		drawTray3D(a.fb, cam, true)
		drawCup(a.fb, cam, base, axis, alpha)
		return
	}
	mouth := base.Add(axis.Mul(cupH))
	inCup := func(p dice.V3) bool { return t < 0.45 && p.Sub(mouth).Dot(axis) < 0.2 }
	resting := t >= 0.45 // on its way back to (or standing on) the table beside the tray
	// shadows on the felt, cast away from the light
	sh := dice.ShapeFor(r.sides)
	for _, p := range poses {
		var pts [][2]float64
		for _, c := range sh.Verts {
			w := p.Pos.Add(p.Rot.Rotate(c))
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
	sort.Slice(order, func(i, j int) bool {
		ci, cj := inCup(poses[order[i]].Pos), inCup(poses[order[j]].Pos)
		if ci != cj {
			return ci
		}
		return dist[order[i]] > dist[order[j]]
	})
	g := r.goal
	lit := g.Counts(r.faces)
	cupDrawn := resting // drawn last, after the front wall
	for _, i := range order {
		if !cupDrawn && !inCup(poses[i].Pos) {
			// dice still inside the cup were drawn before it; the rest go in front
			drawCup(a.fb, cam, base, axis, alpha)
			cupDrawn = true
		}
		look := dieColors(sh.Sides)
		if r.done {
			switch {
			case r.win:
				look.body = color.RGBA{255, 238, 170, 255}
				look.pip = color.RGBA{190, 30, 30, 255}
			case lit[i]:
				look.pip = color.RGBA{190, 30, 30, 255}
			}
		}
		a.drawDie3D(cam, poses[i], sh, look)
	}
	if !cupDrawn {
		drawCup(a.fb, cam, base, axis, alpha)
	}
	if resting {
		drawTray3D(a.fb, cam, true)
		drawCup(a.fb, cam, base, axis, alpha)
		return
	}
	drawTray3D(a.fb, cam, true)
}

// bsGoal is the table's goal as the server described it.
func (a *App) bsGoal() dice.Goal {
	if a.bs == nil {
		return dice.Goal{}
	}
	return dice.Goal{Dice: a.bs.Dice, Sides: a.bs.Sides, Kind: a.bs.GoalKind, Need: a.bs.GoalNeed, Face: a.bs.GoalFace, P: a.bs.Chance}
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
	head := strings.ToUpper(bs.Name)
	if g.P > 0 && g.Dice > 0 {
		head += fmt.Sprintf("  -  1 IN %s (%s)", commas(int64(math.Round(g.Odds()))), pct(g.P))
	}
	a.A.Font.Draw(a.fb, ScreenW/2-a.A.Font.Width(fitText(a.A.Font, head, ScreenW-20))/2, tray.Min.Y-22, fitText(a.A.Font, head, ScreenW-20), a.A.C("gold"))
	sub := fmt.Sprintf("THROW %d %s: %s WINS %s CHIPS FOR EVERYONE HERE", g.Dice, g.Die(), g.Describe(), commas(bs.Prize))
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
		a.textOutlined(ScreenW/2, tray.Max.Y+8, fitText(a.A.Font, r.by+" throws...", a.newsW()), a.A.C("text"), alignCenter)
	case r.win:
		msg, c := "WINNER!", a.A.C("gold")
		if (a.frame/10)%2 == 0 {
			c = a.A.C("highlight")
		}
		tw := a.A.Font.Width(msg)*2 + 16
		mid := tray.Min.Y + tray.Dy()/2
		fillRect(a.fb, image.Rect(ScreenW/2-tw/2, mid-12, ScreenW/2+tw/2, mid+12), color.RGBA{0, 0, 0, 150})
		a.textBig(ScreenW/2, mid-8, msg, c, 2, alignCenter)
		a.textOutlined(ScreenW/2, tray.Max.Y+8, fitText(a.A.Font, fmt.Sprintf("%s rolled %s", r.by, a.scoreText(r.goal, r.faces)), a.newsW()), a.A.C("gold"), alignCenter)
	default:
		a.textOutlined(ScreenW/2, tray.Max.Y+8, fitText(a.A.Font, fmt.Sprintf("%s rolled %s. Needed %s.", r.by, a.scoreText(r.goal, r.faces), strings.ToLower(r.goal.Describe())), a.newsW()), a.A.C("text"), alignCenter)
	}

	// what's been happening, and who else is here
	y := tray.Max.Y + 20
	for i, f := range bs.Feed {
		if i < len(bs.Feed)-3 {
			continue
		}
		a.text(ScreenW/2, y, fitText(a.A.Font, f, a.newsW()), a.A.C("gold"), alignCenter)
		y += 10
	}
	a.drawBeggarSeats(bs)

	a.drawSpeech(st)
	a.drawLogButton(st)
	a.drawTopBar(st)
	a.drawBeggarHUD(bs)
	a.drawChatBar(st)
	a.blocked = updating
	a.drawOverlays()
}

// beggarSeatPos is where the i-th of n people sits: around the rail in
// landscape, in rows under the tray's news on a phone.
func (a *App) beggarSeatPos(i, n int) (int, int) {
	if !a.L.portrait {
		return a.seatPos(beggarSlots[i], 8)
	}
	const perRow = 4
	row, k := i/perRow, i%perRow
	m := min(perRow, n-row*perRow)
	y := a.diceTray().Max.Y + 78 + row*58
	return ScreenW * (2*k + 1) / (2 * m), y
}

// newsW is how wide the lines under the tray can be without running into
// the seats beside them.
func (a *App) newsW() int {
	if a.L.portrait {
		return ScreenW - 20
	}
	return min(ScreenW-20, 280)
}

// beggarSlots is the order seats fill around the rail: the ends first,
// working in, so the middle under the tray stays clear for the news.
var beggarSlots = []int{0, 7, 1, 6, 2, 5, 3, 4}

// drawBeggarSeats draws everyone at the table around the rail, with their
// chips. Whoever threw the dice in the air is marked; a win pays everyone
// under the limit, so they get the +chips sign when it lands.
func (a *App) drawBeggarSeats(bs *proto.BeggarState) {
	const slots = 8
	seatW := a.seatSpacing(slots)
	type seatHat struct {
		id   string
		x, y int
	}
	var hats []seatHat
	r := a.roll
	for i, sv := range bs.Seats {
		if i >= slots {
			break
		}
		x, cy := a.beggarSeatPos(i, len(bs.Seats))
		ar := image.Rect(x-16, cy-16, x+16, cy+16)
		if sv.Throwing || r != nil && !r.done && strings.EqualFold(r.by, sv.Username) {
			strokeRect(a.fb, ar.Inset(-10), a.A.C("gold"))
		}
		a.drawFrame(sv.Frame, ar.Min.X, ar.Min.Y)
		blit(a.fb, a.avatarImg(sv.Username, sv.AvatarVersion, 32), ar.Min.X, ar.Min.Y)
		if !sv.CanRoll {
			fillRect(a.fb, ar, color.RGBA{0, 0, 0, 110}) // too rich to play here
		}
		hats = append(hats, seatHat{sv.Hat, ar.Min.X, ar.Min.Y})
		px, left := ar.Max.X+3, false
		if px+pileChip+20 > ScreenW-2 {
			px, left = ar.Min.X-3-pileChip, true
		}
		a.drawChipPile(px, ar.Max.Y, sv.Balance, left)
		nameCol := a.A.C("text")
		if strings.EqualFold(sv.Username, a.username) {
			nameCol = a.A.C("gold")
		}
		label := func(y int, s string, c color.RGBA) {
			hw := a.A.Font.Width(s)/2 + 2
			a.text(min(max(x, hw), ScreenW-hw), y, s, c, alignCenter)
		}
		label(ar.Max.Y+4, fitText(a.A.Font, sv.Username, seatW), nameCol)
		if name, c, ok := titleOf(sv.Title); ok {
			label(ar.Max.Y+13, fitText(a.A.Font, name, seatW+20), c)
		}
		top := cy - 16 - a.A.Theme.HatHeight + a.A.Theme.HatOverlap - 10
		switch {
		case r != nil && r.done && r.win && r.paid[strings.ToLower(sv.Username)] && (a.frame/8)%4 != 0:
			a.textOutlined(x, top, "+"+commas(bs.Prize), a.A.C("gold"), alignCenter)
		case r != nil && !r.done && strings.EqualFold(r.by, sv.Username):
			a.textOutlined(x, top, "THROWING", a.A.C("gold"), alignCenter)
		}
	}
	for _, h := range hats {
		a.drawHat(h.id, h.x, h.y)
	}
	if extra := len(bs.Seats) - slots; extra > 0 {
		a.text(ScreenW-6, a.L.hudTop-12, fmt.Sprintf("+%d more here", extra), a.A.C("text_dim"), alignRight)
	}
}

// scoreText says what a throw made, in terms of its goal: "a total of 31",
// "3 sixes", "2 of a kind", "4 different".
func (a *App) scoreText(g dice.Goal, faces []int) string {
	n := g.Score(faces)
	switch g.Kind {
	case dice.GoalFaces:
		word := fmt.Sprintf("%ds", g.Face)
		switch {
		case g.Face == 6 && g.S() == 6 && n == 1:
			word = "six"
		case g.Face == 6 && g.S() == 6:
			word = "sixes"
		case g.Face < g.S()-1:
			word = fmt.Sprintf("of %d or higher", g.Face)
		case g.Face == g.S()-1:
			word = fmt.Sprintf("%ds or %ds", g.Face, g.S())
		}
		return fmt.Sprintf("%d %s", n, word)
	case dice.GoalKind:
		if n < 2 {
			return "no matches"
		}
		return fmt.Sprintf("%d of a kind", n)
	case dice.GoalUnique, dice.GoalStraight:
		vals := append([]int(nil), faces...)
		sort.Ints(vals)
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = strconv.Itoa(v)
		}
		return strings.Join(parts, " ")
	case dice.GoalOdd:
		return fmt.Sprintf("%d odd of %d", n, len(faces))
	case dice.GoalEven:
		return fmt.Sprintf("%d even of %d", n, len(faces))
	}
	return fmt.Sprintf("a total of %d", n)
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
