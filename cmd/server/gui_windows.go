package main

import (
	"syscall"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/client"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/win32"
)

func guiAvailable() bool { return true }

func runWindow(title string, icon []byte, mk func(client.Platform) *client.App) error {
	return win32.RunApp(title, &client.Config{}, icon, mk)
}

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	pGetConsoleWnd = kernel32.NewProc("GetConsoleWindow")
	pGetConsoleLst = kernel32.NewProc("GetConsoleProcessList")
	pShowWindow    = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")
)

// hideOwnConsole hides the console window when it was opened just for the
// server (double-clicked), so only the server window shows. It reports
// whether it did; a console you started the server from stays.
func hideOwnConsole() bool {
	var pids [4]uint32
	n, _, _ := pGetConsoleLst.Call(uintptr(unsafePtr(&pids[0])), 4)
	if n != 1 {
		return false
	}
	h, _, _ := pGetConsoleWnd.Call()
	if h == 0 {
		return false
	}
	pShowWindow.Call(h, 0) // SW_HIDE
	return true
}

func showError(msg string) { win32.MessageBox("House Server", msg) }
