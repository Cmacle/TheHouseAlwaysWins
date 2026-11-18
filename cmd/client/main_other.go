// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

//go:build !windows && !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "The House Always Wins client runs on Windows, Linux and Android.")
	os.Exit(1)
}
