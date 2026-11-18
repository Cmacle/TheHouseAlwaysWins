// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Command client is the "The House Always Wins" game client for Linux.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/selfupdate"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/x11"
)

func main() {
	selfupdate.Cleanup()
	dir := "."
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
	// keep settings next to the program if we can write there, otherwise in
	// ~/.config/thehousealwayswins
	if !writable(dir) {
		if cd, err := os.UserConfigDir(); err == nil {
			dir = filepath.Join(cd, "thehousealwayswins")
			os.MkdirAll(dir, 0o755)
		}
	}
	cfg := client.LoadConfig(dir)
	if err := client.ExportDefaults(cfg.AssetsDir); err != nil {
		fmt.Fprintln(os.Stderr, "could not write default assets:", err)
	}
	assets, err := client.LoadAssets(cfg.AssetsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not load assets:", err)
		os.Exit(1)
	}
	if err := x11.Run(cfg, assets, assets.Icon); err != nil {
		fmt.Fprintln(os.Stderr, "The House Always Wins:", err)
		os.Exit(1)
	}
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}
