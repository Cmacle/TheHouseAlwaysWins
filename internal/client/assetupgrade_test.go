// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/cosmetics"
)

// An old copy of the art that nobody edited is updated; edited art stays.
func TestExportDefaultsUpgradesUntouchedFiles(t *testing.T) {
	old, err := exec.Command("git", "show", "v1.7.0:internal/client/defaultassets/hats.png").Output()
	if err != nil || len(old) == 0 {
		t.Skip("no git history here")
	}
	dir := t.TempDir()
	custom := []byte("my own frames")
	os.WriteFile(filepath.Join(dir, "hats.png"), old, 0o644)
	os.WriteFile(filepath.Join(dir, "frames.png"), custom, 0o644)
	if err := ExportDefaults(dir); err != nil {
		t.Fatal(err)
	}
	cur, _ := defaultFS.ReadFile("defaultassets/hats.png")
	if got, _ := os.ReadFile(filepath.Join(dir, "hats.png")); !bytes.Equal(got, cur) {
		t.Fatal("an untouched old hats.png wasn't updated")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "frames.png")); !bytes.Equal(got, custom) {
		t.Fatal("a player's own frames.png was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "cards.png")); err != nil {
		t.Fatal("missing files should still be written")
	}
	// next time: a file we wrote and nobody touched is ours to update
	os.WriteFile(filepath.Join(dir, "hats.png"), cur, 0o644)
	if err := ExportDefaults(dir); err != nil {
		t.Fatal(err)
	}
	a, err := LoadAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	app := New(fakePlatform{}, &Config{}, a)
	for _, id := range []string{"beret", "hot_dog", "moneybag", "fedora"} {
		if app.hatSprite(id) == nil {
			t.Fatalf("hat %s has no sprite", id)
		}
	}
}

// Every title fits under a seat without being cut short (the narrowest seat
// gives a title 80 pixels).
func TestTitlesFit(t *testing.T) {
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range cosmetics.OfKind("title") {
		if w := assets.Font.Width(it.Name); w > 80 {
			t.Errorf("title %q is %dpx wide; it gets cut off at seats", it.Name, w)
		}
	}
}
