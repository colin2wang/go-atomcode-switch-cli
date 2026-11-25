//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

const (
	vtProcessing = 0x0004
	vtInput      = 0x0200
)

func enableRawModeExtras() {
	var mode uint32
	in := windows.Handle(os.Stdin.Fd())
	if windows.GetConsoleMode(in, &mode) == nil {
		_ = windows.SetConsoleMode(in, mode|vtInput)
	}
	out := windows.Handle(os.Stdout.Fd())
	if windows.GetConsoleMode(out, &mode) == nil {
		_ = windows.SetConsoleMode(out, mode|vtProcessing)
	}
}
