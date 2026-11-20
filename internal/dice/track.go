// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package dice

import (
	"encoding/binary"
	"errors"
	"math"
)

// TrackHz is how many poses a second the recorded throw keeps.
const TrackHz = 50

const poseBytes = 14 // 3 position + 4 rotation values, int16 each

// Pose is a die's position and rotation.
type Pose struct {
	Pos V3
	Rot Quat
}

// Throw is a finished, recorded throw.
type Throw struct {
	Dice   int
	Faces  []int
	Track  []byte // TrackHz frames of every die's pose
	Hits   []Hit
	Length float64 // seconds until the dice stopped
}

// Roll throws n dice with the seed and records the throw.
func Roll(n int, seed int64) Throw {
	w := NewWorld(n, seed)
	var buf []byte
	record := func() {
		for i := range w.Dice {
			buf = appendPose(buf, Pose{w.Dice[i].Pos, w.Dice[i].Rot})
		}
	}
	record()
	per := int(math.Round(1 / (step * TrackHz)))
	for s := 1; !w.Settled(); s++ {
		w.Step()
		if s%per == 0 {
			record()
		}
	}
	record() // the resting pose
	return Throw{Dice: n, Faces: w.Faces(), Track: buf, Hits: w.Hits, Length: w.T}
}

func q16(v, scale float64) uint16 {
	return uint16(int16(math.Max(-32767, math.Min(32767, math.Round(v*scale)))))
}

func appendPose(b []byte, p Pose) []byte {
	r := p.Rot
	if r.W < 0 { // q and -q are the same turn; keep W positive
		r = Quat{-r.W, -r.X, -r.Y, -r.Z}
	}
	for _, v := range []uint16{q16(p.Pos.X, 1000), q16(p.Pos.Y, 1000), q16(p.Pos.Z, 1000),
		q16(r.W, 32767), q16(r.X, 32767), q16(r.Y, 32767), q16(r.Z, 32767)} {
		b = binary.LittleEndian.AppendUint16(b, v)
	}
	return b
}

func readPose(b []byte) Pose {
	v := func(i int) float64 { return float64(int16(binary.LittleEndian.Uint16(b[i*2:]))) }
	return Pose{
		Pos: V3{v(0) / 1000, v(1) / 1000, v(2) / 1000},
		Rot: Quat{v(3) / 32767, v(4) / 32767, v(5) / 32767, v(6) / 32767}.Norm(),
	}
}

// Track plays back a recorded throw.
type Track struct {
	dice, frames int
	data         []byte
}

// ReadTrack checks a recorded throw of n dice.
func ReadTrack(n int, data []byte) (*Track, error) {
	if n <= 0 || len(data) == 0 || len(data)%(n*poseBytes) != 0 {
		return nil, errors.New("bad dice track")
	}
	return &Track{dice: n, frames: len(data) / (n * poseBytes), data: data}, nil
}

// Length is how long the throw lasts, in seconds.
func (t *Track) Length() float64 { return float64(t.frames-1) / TrackHz }

func (t *Track) pose(frame, die int) Pose {
	o := (frame*t.dice + die) * poseBytes
	return readPose(t.data[o : o+poseBytes])
}

// At is every die's pose sec seconds into the throw.
func (t *Track) At(sec float64) []Pose {
	f := sec * TrackHz
	i := int(math.Floor(f))
	if i < 0 {
		i, f = 0, 0
	}
	if i >= t.frames-1 {
		i, f = t.frames-1, float64(t.frames-1)
	}
	j := min(i+1, t.frames-1)
	k := f - float64(i)
	out := make([]Pose, t.dice)
	for d := range out {
		a, b := t.pose(i, d), t.pose(j, d)
		out[d] = Pose{Pos: a.Pos.Add(b.Pos.Sub(a.Pos).Mul(k)), Rot: Slerp(a.Rot, b.Rot, k)}
	}
	return out
}
