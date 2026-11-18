// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package selfupdate swaps the running desktop client for a new version.
package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Dir is the folder the program is in (where updates are downloaded).
func Dir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

// startExe is where the program was when it started. It's remembered
// because after Swap, Linux reports the renamed old file as the executable.
var startExe, startErr = findExe()

func findExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}

func exePath() (string, error) { return startExe, startErr }

// Replace puts newFile in place of the running program and starts it. The
// caller should quit right after. The old program is kept as <name>.old
// until the next start (Windows can't delete a running program, but it can
// rename one).
func Replace(newFile string) error {
	if err := Swap(newFile); err != nil {
		return err
	}
	if err := Restart(); err != nil {
		Unswap(newFile)
		return err
	}
	return nil
}

// Swap puts newFile where the running program is, keeping the running one
// as <name>.old. Nothing is started.
func Swap(newFile string) error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	if err := os.Chmod(newFile, 0o755); err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(newFile, exe); err != nil {
		os.Rename(old, exe) // put it back
		return err
	}
	return nil
}

// Unswap undoes Swap (newFile gets the new version back).
func Unswap(newFile string) {
	exe, err := exePath()
	if err != nil {
		return
	}
	os.Rename(exe, newFile)
	os.Rename(exe+".old", exe)
}

// Restart starts the program again (the new version after Swap) with the
// same arguments, sharing this one's console.
func Restart() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Start()
}

// Cleanup removes what an earlier update left behind. Run it at startup.
func Cleanup() {
	exe, err := exePath()
	if err != nil {
		return
	}
	go func() {
		// the old program may still be closing
		for i := 0; i < 20; i++ {
			if err := os.Remove(exe + ".old"); err == nil || os.IsNotExist(err) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		ents, _ := os.ReadDir(filepath.Dir(exe))
		for _, e := range ents {
			if filepath.Ext(e.Name()) == ".download" {
				os.Remove(filepath.Join(filepath.Dir(exe), e.Name()))
			}
		}
	}()
}
