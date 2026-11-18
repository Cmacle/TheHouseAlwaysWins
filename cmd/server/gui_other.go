// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build !windows && !linux

package main

import (
	"errors"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
)

func guiAvailable() bool { return false }

func runWindow(string, []byte, func(client.Platform) *client.App) error {
	return errors.New("no server window on this system")
}

func hideOwnConsole() bool { return false }

func showError(string) {}
