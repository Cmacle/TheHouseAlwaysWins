// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package cosmetics

import "testing"

func TestCatalog(t *testing.T) {
	ids := map[string]bool{}
	sprites := map[string]map[int]bool{Hat: {}, Frame: {}}
	for _, it := range Catalog {
		if ids[it.ID] {
			t.Errorf("duplicate id %s", it.ID)
		}
		ids[it.ID] = true
		if it.Kind != Title {
			if sprites[it.Kind][it.Sprite] {
				t.Errorf("duplicate sprite %s %d", it.Kind, it.Sprite)
			}
			sprites[it.Kind][it.Sprite] = true
		}
		if it.Requirement() == "" {
			t.Errorf("%s has no requirement text", it.ID)
		}
	}
	if len(sprites[Hat]) != 25 || len(sprites[Frame]) != 11 {
		t.Errorf("sprite counts %d hats %d frames", len(sprites[Hat]), len(sprites[Frame]))
	}
}

func TestComeback(t *testing.T) {
	it, _ := Get("t_comeback")
	if it.Met(Stats{LowestNet: -6000, Net: -10}) || !it.Met(Stats{LowestNet: -6000, Net: 0}) || it.Met(Stats{LowestNet: -100, Net: 50}) {
		t.Fatal("comeback logic")
	}
}
