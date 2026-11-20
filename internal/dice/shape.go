// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package dice

import (
	"math"
	"sort"
)

// The dice are the fair Platonic solids: d6 (cube), d8 (octahedron), d12
// (dodecahedron) and d20 (icosahedron). Every face is the same, so every
// number is equally likely, and a die always comes to rest on a face with
// another face flat on top: the top face is the roll. Opposite faces add
// up to sides+1, like real dice.

// Face is one face of a die.
type Face struct {
	N     V3      // outward direction, on the die
	D     float64 // distance from the centre to the face
	Val   int     // the number on it
	Verts []int   // corners, in order around the face
}

// Shape is a kind of die.
type Shape struct {
	Sides  int
	Verts  []V3 // corners, on the die
	Faces  []Face
	Edges  []V3    // the distinct edge directions
	Radius float64 // centre to corner
	Inner  float64 // centre to face
	invI   float64 // 1 / moment of inertia (mass 1, treated as the same about any axis)
}

// Sides lists the dice there are.
var Sides = []int{6, 8, 12, 20}

var shapes = map[int]*Shape{}

func init() {
	phi := (1 + math.Sqrt(5)) / 2
	var cube, octa, dodeca, icosa []V3
	for _, x := range []float64{-1, 1} {
		for _, y := range []float64{-1, 1} {
			for _, z := range []float64{-1, 1} {
				cube = append(cube, V3{x, y, z})
			}
		}
	}
	for _, a := range []float64{-1, 1} {
		octa = append(octa, V3{X: a}, V3{Y: a}, V3{Z: a})
	}
	dodeca = append(dodeca, cube...)
	for _, a := range []float64{-1, 1} {
		for _, b := range []float64{-1, 1} {
			dodeca = append(dodeca, V3{0, a * phi, b / phi}, V3{a / phi, 0, b * phi}, V3{a * phi, b / phi, 0})
			icosa = append(icosa, V3{0, a, b * phi}, V3{a, b * phi, 0}, V3{a * phi, 0, b})
		}
	}
	// a solid's face directions are its dual's corners
	shapes[6] = build(6, cube, octa, 0.5*math.Sqrt(3))
	shapes[8] = build(8, octa, cube, 0.95)
	shapes[12] = build(12, dodeca, icosa, 0.92)
	shapes[20] = build(20, icosa, dodeca, 0.95)
	// the cube keeps its exact numbers and inertia
	shapes[6].invI = invInert
}

// ShapeFor is the die with that many sides (a d6 for anything unknown).
func ShapeFor(sides int) *Shape {
	if s := shapes[sides]; s != nil {
		return s
	}
	return shapes[6]
}

// build makes a shape from its corners and face directions, scaled so the
// corners are radius from the centre.
func build(sides int, verts, normals []V3, radius float64) *Shape {
	s := &Shape{Sides: sides, Radius: radius}
	for _, v := range verts {
		s.Verts = append(s.Verts, v.Norm().Mul(radius))
	}
	for _, n := range normals {
		n = n.Norm()
		best := math.Inf(-1)
		for _, v := range s.Verts {
			best = math.Max(best, v.Dot(n))
		}
		f := Face{N: n, D: best}
		for i, v := range s.Verts {
			if v.Dot(n) > best-1e-6 {
				f.Verts = append(f.Verts, i)
			}
		}
		// order the corners around the face
		u := s.Verts[f.Verts[0]].Sub(n.Mul(best)).Norm()
		w := n.Cross(u)
		sort.Slice(f.Verts, func(i, j int) bool {
			pi, pj := s.Verts[f.Verts[i]], s.Verts[f.Verts[j]]
			return math.Atan2(pi.Dot(w), pi.Dot(u)) < math.Atan2(pj.Dot(w), pj.Dot(u))
		})
		s.Faces = append(s.Faces, f)
		s.Inner = best
	}
	// numbers: opposite faces add to sides+1
	used := make([]bool, len(s.Faces))
	next := 1
	for i := range s.Faces {
		if used[i] {
			continue
		}
		for j := range s.Faces {
			if !used[j] && j != i && s.Faces[j].N.Dot(s.Faces[i].N) < -0.999 {
				s.Faces[i].Val, s.Faces[j].Val = next, sides+1-next
				used[i], used[j] = true, true
				next++
				break
			}
		}
	}
	if sides == 6 {
		// the classic layout: 1 up, 2 toward you, 3 right
		for i := range s.Faces {
			for v := 1; v <= 6; v++ {
				if s.Faces[i].N.Dot(FaceAxes[v]) > 0.999 {
					s.Faces[i].Val = v
				}
			}
		}
	}
	for _, f := range s.Faces {
		for k := range f.Verts {
			e := s.Verts[f.Verts[(k+1)%len(f.Verts)]].Sub(s.Verts[f.Verts[k]]).Norm()
			dup := false
			for _, o := range s.Edges {
				if math.Abs(o.Dot(e)) > 0.999 {
					dup = true
					break
				}
			}
			if !dup {
				s.Edges = append(s.Edges, e)
			}
		}
	}
	r := (s.Inner + s.Radius) / 2
	s.invI = 1 / (0.4 * r * r)
	return s
}

// FaceOf is the face with that number.
func (s *Shape) FaceOf(val int) *Face {
	for i := range s.Faces {
		if s.Faces[i].Val == val {
			return &s.Faces[i]
		}
	}
	return nil
}
