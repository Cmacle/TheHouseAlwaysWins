package main

import (
	"os"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/x11"
)

func guiAvailable() bool { return os.Getenv("DISPLAY") != "" && x11.Available() }

func runWindow(title string, icon []byte, mk func(client.Platform) *client.App) error {
	return x11.RunApp(title, &client.Config{}, icon, mk)
}

func hideOwnConsole() bool { return false }

func showError(string) {}
