// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package banter

// The dealer's wealth tiers follow the house's lifetime net, in big steps so
// he doesn't flip-flop. Moving away from even happens as soon as a threshold
// is crossed; moving back toward even needs a buffer past it.

// MinTier and MaxTier bound the dealer's wealth tiers.
const (
	MinTier = -3
	MaxTier = 5
)

var (
	upAt   = []int64{10_000, 50_000, 150_000, 500_000, 1_500_000} // enter tiers 1..5
	downAt = []int64{-5_000, -20_000, -50_000}                    // enter tiers -1..-3
)

// TierNames describe each tier, from -3 to 5.
var TierNames = map[int]string{
	-3: "Destitute", -2: "Broke", -1: "Struggling", 0: "Working Stiff",
	1: "Comfortable", 2: "Rich", 3: "Loaded", 4: "Tycoon", 5: "Emperor",
}

// TierItems is what the dealer bought on reaching a tier (positive tiers),
// or what he lost on falling into it (negative tiers), or what he got back on
// recovering to it (zero and below, see RecoverItems).
var TierItems = map[int]string{
	1:  "velvet jacket and gold chain",
	2:  "white tuxedo and two gold chains",
	3:  "fur coat, gold tooth and diamond rings",
	4:  "top hat and diamond monocle",
	5:  "golden crown, royal cape and a scepter",
	-1: "good jacket",
	-2: "car, the watch and my visor",
	-3: "trailer",
}

// RecoverItems is what he gets back climbing up to a tier at or below zero.
var RecoverItems = map[int]string{
	0:  "jacket back from the pawn shop",
	-1: "shirt with all its buttons",
	-2: "tent back (it's not much, but it's MY tent)",
}

func margin(b int64) int64 {
	if b < 0 {
		b = -b
	}
	if m := b / 10; m > 2000 {
		return m
	}
	return 2000
}

// Tier works out the dealer's wealth tier from the house's lifetime net and
// his previous tier.
func Tier(net int64, prev int) int {
	t := prev
	if t < MinTier {
		t = MinTier
	}
	if t > MaxTier {
		t = MaxTier
	}
	for i := 0; i < 20; i++ {
		switch {
		case t >= 0 && t < MaxTier && net >= upAt[t]:
			t++ // richer: immediately
		case t <= 0 && t > MinTier && net <= downAt[-t]:
			t-- // poorer: immediately
		case t > 0 && net < upAt[t-1]-margin(upAt[t-1]):
			t-- // falling back toward even: needs the buffer
		case t < 0 && net > downAt[-t-1]+margin(downAt[-t-1]):
			t++ // recovering toward even: needs the buffer
		default:
			return t
		}
	}
	return t
}

// Thresholds returns the house net at which the dealer moves up or down from
// tier t (ok=false at the ends).
func Thresholds(t int) (up int64, upOK bool, down int64, downOK bool) {
	switch {
	case t >= 0 && t < MaxTier:
		up, upOK = upAt[t], true
	case t < 0:
		up, upOK = downAt[-t-1]+margin(downAt[-t-1]), true
	}
	switch {
	case t > 0:
		down, downOK = upAt[t-1]-margin(upAt[t-1]), true
	case t <= 0 && t > MinTier:
		down, downOK = downAt[-t], true
	}
	return
}
