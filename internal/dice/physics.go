// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package dice

import (
	"math"
	"math/rand"
)

// The throw is a small rigid-body simulation: unit cubes in an open tray,
// with gravity, bounces off the felt and walls, friction, and dice knocking
// into each other. Contacts are found at the dice's corners (against the
// tray and inside other dice), with an overlap check between centres to
// catch edge-on-edge hits. Lengths are in die widths.

// The tray, in die widths. x runs left to right, y from the front wall to
// the back, z up from the felt.
const (
	TrayW = 18.0
	TrayD = 8.0
	WallH = 1.4

	step     = 1.0 / 480 // simulation step, seconds
	gravity  = 150.0
	half     = 0.5
	invMass  = 1.0
	invInert = 6.0 // a uniform cube: I = m*s*s/6, the same about any axis
	maxTime  = 14.0
)

// V3 is a 3D vector.
type V3 struct{ X, Y, Z float64 }

func (a V3) Add(b V3) V3      { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V3) Sub(b V3) V3      { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V3) Mul(s float64) V3 { return V3{a.X * s, a.Y * s, a.Z * s} }
func (a V3) Dot(b V3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a V3) Cross(b V3) V3    { return V3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X} }
func (a V3) Len() float64     { return math.Sqrt(a.Dot(a)) }
func (a V3) Norm() V3 {
	l := a.Len()
	if l == 0 {
		return a
	}
	return a.Mul(1 / l)
}

// Quat is a rotation.
type Quat struct{ W, X, Y, Z float64 }

func (q Quat) Mul(r Quat) Quat {
	return Quat{
		q.W*r.W - q.X*r.X - q.Y*r.Y - q.Z*r.Z,
		q.W*r.X + q.X*r.W + q.Y*r.Z - q.Z*r.Y,
		q.W*r.Y - q.X*r.Z + q.Y*r.W + q.Z*r.X,
		q.W*r.Z + q.X*r.Y - q.Y*r.X + q.Z*r.W,
	}
}

func (q Quat) Norm() Quat {
	l := math.Sqrt(q.W*q.W + q.X*q.X + q.Y*q.Y + q.Z*q.Z)
	if l == 0 {
		return Quat{W: 1}
	}
	return Quat{q.W / l, q.X / l, q.Y / l, q.Z / l}
}

// Rotate turns v by q.
func (q Quat) Rotate(v V3) V3 {
	u := V3{q.X, q.Y, q.Z}
	t := u.Cross(v).Mul(2)
	return v.Add(t.Mul(q.W)).Add(u.Cross(t))
}

// Inv is the opposite rotation.
func (q Quat) Inv() Quat { return Quat{q.W, -q.X, -q.Y, -q.Z} }

// Slerp blends a to b.
func Slerp(a, b Quat, t float64) Quat {
	d := a.W*b.W + a.X*b.X + a.Y*b.Y + a.Z*b.Z
	if d < 0 {
		b, d = Quat{-b.W, -b.X, -b.Y, -b.Z}, -d
	}
	if d > 0.9995 {
		return Quat{a.W + (b.W-a.W)*t, a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t, a.Z + (b.Z-a.Z)*t}.Norm()
	}
	th := math.Acos(d)
	s := math.Sin(th)
	wa, wb := math.Sin((1-t)*th)/s, math.Sin(t*th)/s
	return Quat{a.W*wa + b.W*wb, a.X*wa + b.X*wb, a.Y*wa + b.Y*wb, a.Z*wa + b.Z*wb}
}

// FaceAxes are each face's outward direction on the die, for faces 1-6.
// Opposite faces add to seven.
var FaceAxes = [7]V3{{}, {0, 0, 1}, {0, -1, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, 0, -1}}

// Body is one die in the simulation.
type Body struct {
	S   *Shape // which die; nil is a d6
	Pos V3
	Rot Quat
	Vel V3
	Ang V3 // angular velocity, world axes

	asleep  bool    // at rest; skipped until something knocks it
	stillT  float64 // how long it has barely moved
	snapPos V3      // where it was at the last stillness check
	snapRot Quat
	floor   bool // touched the felt this step
	anchor  V3   // where it has been loitering since anchorT
	anchorT float64
}

func (b *Body) shape() *Shape {
	if b.S == nil {
		return shapes[6]
	}
	return b.S
}

// Up says which number is on top and how squarely (1 = flat).
func (b *Body) Up() (int, float64) {
	best, bd := 1, -2.0
	for _, f := range b.shape().Faces {
		if d := b.Rot.Rotate(f.N).Z; d > bd {
			best, bd = f.Val, d
		}
	}
	return best, bd
}

type plane struct {
	n V3
	d float64 // points p with p.n >= d are inside
	e float64 // bounciness
}

var walls = []plane{
	{V3{0, 0, 1}, 0, 0.55},       // felt
	{V3{1, 0, 0}, 0, 0.65},       // left wall
	{V3{-1, 0, 0}, -TrayW, 0.65}, // right wall
	{V3{0, 1, 0}, 0, 0.65},       // front wall
	{V3{0, -1, 0}, -TrayD, 0.65}, // back wall
}

// World is a throw in progress.
type World struct {
	Dice    []Body
	T       float64
	Hits    []Hit // knocks loud enough to hear
	rng     *rand.Rand
	calm    float64 // how long everything has been still
	lastHit float64
}

// Hit is a knock: when, and how hard (0-1).
type Hit struct {
	T float64
	K float64
}

// The cup the dice are poured from: it tips over the right end of the
// tray, mouth to the left. CupMouth is where its mouth is when it has
// tipped, and the dice start packed just inside it.
var CupMouth = V3{TrayW - 3.0, TrayD / 2, 4.0}

// NewWorld pours n dice with the given number of sides out of the cup,
// flying left.
func NewWorld(sides, n int, seed int64) *World {
	rng := rand.New(rand.NewSource(seed))
	w := &World{rng: rng}
	sh := ShapeFor(sides)
	gap := 2*sh.Radius*0.82 + 0.06 // packed snugly; any overlap pops apart at once
	across := 3
	if gap > 1.25 {
		across = 2
	}
	for i := 0; i < n; i++ {
		// packed 2 deep, a few across, in layers, with a little jitter
		col, row, layer := i%2, (i/2)%across, i/(2*across)
		b := Body{
			S: sh,
			Pos: V3{CupMouth.X + 0.9 + float64(col)*gap + rng.Float64()*0.04,
				CupMouth.Y + (float64(row)-float64(across-1)/2)*gap + rng.Float64()*0.04,
				CupMouth.Z - 0.4 + float64(layer)*gap + rng.Float64()*0.04},
			Rot: randomRot(rng),
			Vel: V3{-(18 + rng.Float64()*14), (rng.Float64()*2 - 1) * 7, 1 + rng.Float64()*6},
			Ang: V3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}.Norm().Mul(14 + rng.Float64()*20),
		}
		w.Dice = append(w.Dice, b)
	}
	return w
}

// a uniformly random rotation
func randomRot(rng *rand.Rand) Quat {
	u1, u2, u3 := rng.Float64(), rng.Float64(), rng.Float64()
	a, b := math.Sqrt(1-u1), math.Sqrt(u1)
	return Quat{a * math.Sin(2*math.Pi*u2), a * math.Cos(2*math.Pi*u2), b * math.Sin(2*math.Pi*u3), b * math.Cos(2*math.Pi*u3)}
}

// Faces is what each die shows.
func (w *World) Faces() []int {
	out := make([]int, len(w.Dice))
	for i := range w.Dice {
		out[i], _ = w.Dice[i].Up()
	}
	return out
}

// Settled says the dice have stopped.
func (w *World) Settled() bool { return w.calm > 0 || w.T >= maxTime }

// impulse applies an impulse j at world point p to body b.
func (b *Body) impulse(j, p V3) {
	b.Vel = b.Vel.Add(j.Mul(invMass))
	b.Ang = b.Ang.Add(p.Sub(b.Pos).Cross(j).Mul(b.shape().invI))
}

func (b *Body) pointVel(p V3) V3 { return b.Vel.Add(b.Ang.Cross(p.Sub(b.Pos))) }

// contact resolves a touch at point p with normal n (pointing toward a)
// between a and b (b == nil: the tray).
func (w *World) contact(a, b *Body, p, n V3, e, mu float64) {
	rv := a.pointVel(p)
	if b != nil {
		rv = rv.Sub(b.pointVel(p))
	}
	vn := rv.Dot(n)
	if vn >= 0 {
		return
	}
	if -vn > 6 {
		w.hit(-vn)
	}
	if -vn < 1.5 {
		e = 0 // no bouncing at a crawl, so dice can come to rest
	}
	ra := p.Sub(a.Pos).Cross(n)
	k := invMass + ra.Dot(ra)*a.shape().invI
	if b != nil {
		rb := p.Sub(b.Pos).Cross(n)
		k += invMass + rb.Dot(rb)*b.shape().invI
	}
	jn := -(1 + e) * vn / k
	J := n.Mul(jn)
	// friction against the sliding direction
	vt := rv.Sub(n.Mul(vn))
	if s := vt.Len(); s > 1e-6 {
		t := vt.Mul(1 / s)
		rta := p.Sub(a.Pos).Cross(t)
		kt := invMass + rta.Dot(rta)*a.shape().invI
		if b != nil {
			rtb := p.Sub(b.Pos).Cross(t)
			kt += invMass + rtb.Dot(rtb)*b.shape().invI
		}
		jt := math.Min(s/kt, mu*jn)
		J = J.Sub(t.Mul(jt))
	}
	if b != nil && jn > 0.6 {
		a.asleep, b.asleep = false, false // a real knock wakes both
	}
	if !a.asleep {
		a.impulse(J, p)
	}
	if b != nil && !b.asleep {
		b.impulse(J.Mul(-1), p)
	}
}

func (w *World) hit(speed float64) {
	if w.T-w.lastHit < 0.06 {
		return
	}
	w.lastHit = w.T
	w.Hits = append(w.Hits, Hit{T: w.T, K: math.Min(1, speed/30)})
}

// Step advances the throw one simulation step.
func (w *World) Step() {
	w.T += step
	for i := range w.Dice {
		b := &w.Dice[i]
		b.floor = false
		if !b.asleep {
			b.Vel.Z -= gravity * step
		}
	}
	for iter := 0; iter < 4; iter++ {
		for i := range w.Dice {
			b := &w.Dice[i]
			if b.asleep {
				continue
			}
			_, sq := b.Up()
			for k, pl := range walls {
				if k == 0 && sq > 0.985 && b.Vel.Z > -1.5 && b.lowest() < 0.03 {
					// lying flat on the felt: it rests on the whole face,
					// so it can't tip, and the felt drags it to a stop
					b.floor = true
					if iter == 0 {
						b.Vel.Z = math.Max(b.Vel.Z, 0)
						b.Ang.X, b.Ang.Y = 0, 0
						if h := math.Hypot(b.Vel.X, b.Vel.Y); h > 0 {
							k := math.Max(0, h-0.12*gravity*step) / h
							b.Vel.X, b.Vel.Y = b.Vel.X*k, b.Vel.Y*k
						}
						b.Ang.Z *= 1 - 6*step
					}
					continue
				}
				for _, c := range b.shape().Verts {
					p := b.Pos.Add(b.Rot.Rotate(c))
					if p.Dot(pl.n) < pl.d {
						if pl.n.Z != 0 {
							b.floor = true
						}
						mu := 0.45
						if pl.n.Z == 0 {
							mu = 0.2
						}
						w.contact(b, nil, p, pl.n, pl.e, mu)
					}
				}
			}
		}
		for i := range w.Dice {
			for j := i + 1; j < len(w.Dice); j++ {
				w.diePair(&w.Dice[i], &w.Dice[j])
			}
		}
	}
	allAsleep := true
	for i := range w.Dice {
		b := &w.Dice[i]
		if b.asleep {
			continue
		}
		allAsleep = false
		b.Pos = b.Pos.Add(b.Vel.Mul(step))
		if l := b.Ang.Len(); l > 0 {
			ax := b.Ang.Mul(1 / l)
			s, c := math.Sincos(l * step / 2)
			b.Rot = Quat{c, ax.X * s, ax.Y * s, ax.Z * s}.Mul(b.Rot).Norm()
		}
		// push out of the tray's walls and felt
		for _, pl := range walls {
			deep := 0.0
			for _, c := range b.shape().Verts {
				if d := pl.d - b.Pos.Add(b.Rot.Rotate(c)).Dot(pl.n); d > deep {
					deep = d
				}
			}
			if deep > 0 {
				b.Pos = b.Pos.Add(pl.n.Mul(deep * 0.8))
			}
		}
		// a little air drag; on the felt, rolling resistance, and a die
		// lying flat stops turning and sliding quickly
		b.Vel = b.Vel.Mul(1 - 0.05*step)
		b.Ang = b.Ang.Mul(1 - 0.3*step)
		_, sq := b.Up()
		if b.floor {
			b.Ang = b.Ang.Mul(1 - 0.6*step)
			if sq > 0.985 {
				b.Ang = b.Ang.Mul(1 - 4*step)
			}
		}
		// stillness is judged by how far it actually moved, since a die
		// resting on the felt keeps a little contact jitter in its velocity
		b.stillT += step
		if b.stillT >= 0.05 {
			moved := b.Pos.Sub(b.snapPos).Len()
			turned := math.Abs(b.Rot.W*b.snapRot.W + b.Rot.X*b.snapRot.X + b.Rot.Y*b.snapRot.Y + b.Rot.Z*b.snapRot.Z)
			if moved > 0.012 || turned < 0.99998 {
				b.stillT = 0
			}
			b.snapPos, b.snapRot = b.Pos, b.Rot
			if b.stillT >= 0.3 && sq >= 0.97 {
				b.asleep = true
				b.Vel, b.Ang = V3{}, V3{}
			}
		}
		if d := b.Pos.Sub(b.anchor).Len(); d > 0.2 || sq >= 0.97 {
			b.anchor, b.anchorT = b.Pos, w.T
		} else if w.T-b.anchorT > 0.8 {
			// propped on an edge against a wall or another die:
			// jostle it until it falls flat
			b.anchorT = w.T
			b.Vel = b.Vel.Add(V3{(w.rng.Float64()*2 - 1) * 6, (w.rng.Float64()*2 - 1) * 6, 10})
			b.Ang = b.Ang.Add(V3{w.rng.NormFloat64(), w.rng.NormFloat64(), w.rng.NormFloat64()}.Mul(8))
			b.stillT = 0
			for j := range w.Dice { // let the pile it leans on shift too
				if j != i && w.Dice[j].Pos.Sub(b.Pos).Len() < 1.8 {
					w.Dice[j].asleep, w.Dice[j].stillT = false, 0
				}
			}
		}
	}
	// moving can push dice into each other again: push them apart now, so
	// no step ends with one die inside another
	for iter := 0; iter < 3; iter++ {
		clean := true
		for i := range w.Dice {
			for j := i + 1; j < len(w.Dice); j++ {
				a, b := &w.Dice[i], &w.Dice[j]
				if a.Pos.Sub(b.Pos).Len() > a.shape().Radius+b.shape().Radius {
					continue
				}
				if d, n, _, ok := overlap(a, b); ok && d > 0.002 {
					separate(a, b, n.Mul(d))
					clean = false
				}
			}
		}
		if clean {
			break
		}
	}
	if allAsleep {
		w.calm += step
	} else {
		w.calm = 0
	}
}

// diePair handles two dice touching. Corners of one inside the other give
// the resting contacts; on top of that, an exact solid-against-solid test
// (separating axes: every face direction of both dice and every pairing
// of their edges) finds any overlap at all, edge against edge included,
// and pushes the dice apart along the shallowest way out.
func (w *World) diePair(a, b *Body) {
	if a.Pos.Sub(b.Pos).Len() > a.shape().Radius+b.shape().Radius {
		return
	}
	w.cornersInto(a, b)
	w.cornersInto(b, a)
	if depth, n, p, ok := overlap(a, b); ok {
		w.contact(a, b, p, n, 0.3, 0.3)
		separate(a, b, n.Mul(depth))
	}
}

// span is how far the die reaches along dir, both ways, from the origin.
func (b *Body) span(dir V3) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	inv := b.Rot.Inv().Rotate(dir)
	for _, v := range b.shape().Verts {
		d := v.Dot(inv)
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	c := b.Pos.Dot(dir)
	return c + lo, c + hi
}

// support is the corner of b farthest along dir.
func (b *Body) support(dir V3) V3 {
	inv := b.Rot.Inv().Rotate(dir)
	best, bd := V3{}, math.Inf(-1)
	for _, v := range b.shape().Verts {
		if d := v.Dot(inv); d > bd {
			best, bd = v, d
		}
	}
	return b.Pos.Add(b.Rot.Rotate(best))
}

// overlap says how deep two dice overlap, the direction to push a out of
// b, and where they touch. ok is false when they don't overlap.
func overlap(a, b *Body) (depth float64, n, point V3, ok bool) {
	best := math.Inf(1)
	kind := 0
	try := func(L V3, k int) bool {
		l := L.Len()
		if l < 1e-6 {
			return true // parallel edges: covered by the face directions
		}
		L = L.Mul(1 / l)
		alo, ahi := a.span(L)
		blo, bhi := b.span(L)
		o1, o2 := ahi-blo, bhi-alo // a pushed back along -L, or along +L
		o, dir := o2, L
		if o1 < o2 {
			o, dir = o1, L.Mul(-1)
		}
		if o <= 0 {
			return false // a gap along this direction: no overlap
		}
		if k == 2 {
			o += 1e-4 // prefer a face direction when it's a tie
		}
		if o < best {
			best, kind, n = o, k, dir
		}
		return true
	}
	sa, sb := a.shape(), b.shape()
	for _, f := range sa.Faces {
		if !try(a.Rot.Rotate(f.N), 0) {
			return 0, V3{}, V3{}, false
		}
	}
	for _, f := range sb.Faces {
		if !try(b.Rot.Rotate(f.N), 1) {
			return 0, V3{}, V3{}, false
		}
	}
	for _, ea := range sa.Edges {
		wa := a.Rot.Rotate(ea)
		for _, eb := range sb.Edges {
			if !try(wa.Cross(b.Rot.Rotate(eb)), 2) {
				return 0, V3{}, V3{}, false
			}
		}
	}
	switch kind {
	case 0: // a face of a: b's deepest corner into it
		point = b.support(n)
	case 1: // a face of b: a's deepest corner into it
		point = a.support(n.Mul(-1))
	default: // edge against edge: between the two edges' nearest corners
		point = a.support(n.Mul(-1)).Add(b.support(n)).Mul(0.5)
	}
	if kind == 2 {
		best -= 1e-4
	}
	return best, n, point, true
}

// cornersInto pushes a's corners out of b.
func (w *World) cornersInto(a, b *Body) {
	sb := b.shape()
	inv := b.Rot.Inv()
	for _, c := range a.shape().Verts {
		p := a.Pos.Add(a.Rot.Rotate(c))
		l := inv.Rotate(p.Sub(b.Pos))
		depth, face := math.Inf(1), -1
		for i, f := range sb.Faces {
			d := f.D - l.Dot(f.N)
			if d <= 0 {
				face = -1
				break // outside this face, so outside the die
			}
			if d < depth {
				depth, face = d, i
			}
		}
		if face < 0 {
			continue
		}
		n := b.Rot.Rotate(sb.Faces[face].N)
		w.contact(a, b, p, n, 0.3, 0.3)
		separate(a, b, n.Mul(depth*0.8))
	}
}

// separate moves two overlapping dice apart by push (toward a), all of it on
// the awake one if the other is asleep.
func separate(a, b *Body, push V3) {
	switch {
	case a.asleep && b.asleep:
	case a.asleep:
		b.Pos = b.Pos.Sub(push)
	case b.asleep:
		a.Pos = a.Pos.Add(push)
	default:
		a.Pos = a.Pos.Add(push.Mul(0.5))
		b.Pos = b.Pos.Sub(push.Mul(0.5))
	}
}

// lowest is the height of the die's lowest corner.
func (b *Body) lowest() float64 {
	low := math.Inf(1)
	for _, c := range b.shape().Verts {
		low = math.Min(low, b.Pos.Z+b.Rot.Rotate(c).Z)
	}
	return low
}
