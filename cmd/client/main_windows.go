// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Command client is the "The House Always Wins" game client for Windows.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/selfupdate"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/win32"
)

func main() {
	selfupdate.Cleanup()
	dir := "."
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
	win32.OpenLog(dir)
	win32.Logf("The House Always Wins %s starting in %s", client.Version, dir)
	defer func() {
		if r := recover(); r != nil {
			win32.Logf("CRASH: %v\n%s", r, debug.Stack())
			win32.MessageBox("The House Always Wins", fmt.Sprintf("The game crashed:\n%v\n\nDetails are in %s", r, win32.LogPath()))
			os.Exit(2)
		}
	}()
	cfg := client.LoadConfig(dir)
	if err := client.ExportDefaults(cfg.AssetsDir); err != nil {
		win32.Logf("could not write default assets to %s: %v", cfg.AssetsDir, err)
	}
	assets, err := client.LoadAssets(cfg.AssetsDir)
	if err != nil {
		win32.Logf("could not load assets: %v", err)
		win32.MessageBox("The House Always Wins", "Could not load assets:\n"+err.Error())
		os.Exit(1)
	}
	win32.Logf("assets loaded from %s", cfg.AssetsDir)
	if err := win32.Run(cfg, assets, assets.Icon); err != nil {
		win32.Logf("could not open the window: %v", err)
		win32.MessageBox("The House Always Wins", "Could not open the game window:\n"+err.Error())
		os.Exit(1)
	}
}
