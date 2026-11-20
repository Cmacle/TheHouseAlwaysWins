// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The remembered password round-trips through client.json, isn't stored
// as plain text, and the file is private.
func TestRememberPassword(t *testing.T) {
	dir := t.TempDir()
	c := LoadConfig(dir)
	c.SetPassword("hunter2!")
	c.Save()
	b, _ := os.ReadFile(filepath.Join(dir, "client.json"))
	if strings.Contains(string(b), "hunter2") {
		t.Fatal("password saved as plain text")
	}
	if st, err := os.Stat(filepath.Join(dir, "client.json")); err == nil && st.Mode().Perm()&0o077 != 0 {
		t.Errorf("client.json is readable by others: %v", st.Mode())
	}
	if pw := LoadConfig(dir).Password(); pw != "hunter2!" {
		t.Fatalf("got %q back", pw)
	}
	c.SetPassword("")
	if c.Password() != "" || c.SavedPassword != "" {
		t.Fatal("forgetting didn't")
	}
	// a remembered password is filled in on the login form at start
	c.Username, c.RememberMe = "me", true
	c.SetPassword("pw1234")
	assets, err := LoadAssets("")
	if err != nil {
		t.Fatal(err)
	}
	if a := New(fakePlatform{}, c, assets); a.fPass != "pw1234" {
		t.Fatalf("login form has %q", a.fPass)
	}
}
