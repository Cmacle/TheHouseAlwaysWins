// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"testing"
	"time"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/dice"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func throwMsg(id, n int, seed int64, elapsed int64) proto.DiceRoll {
	th := dice.Roll(n, seed)
	return proto.DiceRoll{ID: id, By: "deb", Dice: n, Faces: th.Faces, MS: 1000 + int64(th.Length*1000), ShakeMS: 1000,
		Track: th.Track, Elapsed: elapsed}
}

// The replayed throw ends on the server's faces, and someone arriving
// mid-throw picks it up where it is.
func TestDiceReplay(t *testing.T) {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	a := New(fakePlatform{}, &Config{}, assets)
	a.now = time.Now()
	for seed := int64(1); seed <= 20; seed++ {
		n := int(seed%dice.MaxDice) + 1
		m := throwMsg(int(seed), n, seed, 0)
		a.onDice(m)
		r := a.roll
		if r == nil {
			t.Fatal("throw not taken")
		}
		ps, _ := r.poses(a.now.Add(time.Duration(m.MS+500) * time.Millisecond))
		for i, p := range ps {
			b := dice.Body{Rot: p.Rot}
			if f, _ := b.Up(); f != m.Faces[i] {
				t.Errorf("seed %d die %d ends on %d, server says %d", seed, i, f, m.Faces[i])
			}
		}
	}
	m := throwMsg(99, 5, 99, 1800)
	a.onDice(m)
	if _, at := a.roll.poses(a.now); at < 0.7 || at > 0.9 {
		t.Errorf("joining 1.8s in should be 0.8s into the throw, got %.2f", at)
	}
	a.onDice(proto.DiceRoll{ID: 100, Dice: 3, Faces: []int{1, 2}, Track: []byte{1, 2, 3}})
	if a.roll.id != 99 {
		t.Error("a bad throw replaced the good one")
	}
}
