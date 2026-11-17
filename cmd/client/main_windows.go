// Command client is the "The House Always Wins" game client for Windows.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/win32"
)

func main() {
	dir := "."
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
	cfg := client.LoadConfig(dir)
	if err := client.ExportDefaults(cfg.AssetsDir); err != nil {
		fmt.Fprintln(os.Stderr, "could not write default assets:", err)
	}
	assets, err := client.LoadAssets(cfg.AssetsDir)
	if err != nil {
		win32.MessageBox("The House Always Wins", "Could not load assets:\n"+err.Error())
		os.Exit(1)
	}
	if err := win32.Run(cfg, assets, assets.Icon); err != nil {
		win32.MessageBox("The House Always Wins", "Could not open the game window:\n"+err.Error())
		os.Exit(1)
	}
}
