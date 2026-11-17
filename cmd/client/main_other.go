//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "The House Always Wins client currently runs on Windows only. Build with GOOS=windows.")
	os.Exit(1)
}
