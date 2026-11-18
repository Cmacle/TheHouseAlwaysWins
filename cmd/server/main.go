// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Command server is the headless "The House Always Wins" blackjack server.
//
// It is meant to run constantly. With nobody connected the table stops and the
// process just waits for connections.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	_ "time/tzdata" // so "timezone" works on Windows without Go installed

	"github.com/Cmacle/TheHouseAlwaysWins/internal/selfupdate"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/server"
)

func main() {
	def := "server.json"
	if exe, err := os.Executable(); err == nil {
		def = filepath.Join(filepath.Dir(exe), "server.json")
	}
	cfgPath := flag.String("config", def, "path to server.json (created with defaults if missing)")
	noConsole := flag.Bool("noconsole", false, "don't read admin commands from stdin (for services)")
	noGUI := flag.Bool("nogui", false, "don't open the server window, even if there's a desktop")
	version := flag.Bool("version", false, "print version and exit")
	printCfg := flag.Bool("print-config", false, "print the default server.json and exit")
	flag.Parse()
	if *printCfg {
		b, _ := json.MarshalIndent(server.DefaultConfig(), "", "  ")
		fmt.Println(string(b))
		return
	}
	if *version {
		fmt.Println(server.Version)
		return
	}
	selfupdate.Cleanup()
	var err error
	gui := !*noGUI && !*noConsole && guiAvailable()
	if gui {
		console := !hideOwnConsole()
		err = runWithWindow(*cfgPath, console)
	} else {
		err = server.Main(*cfgPath, server.Options{Console: !*noConsole})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if gui {
			showError("The server couldn't start:\n\n" + err.Error())
		}
		os.Exit(1)
	}
}
