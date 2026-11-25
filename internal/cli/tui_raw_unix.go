//go:build linux || darwin

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

func enableRawModeExtras() {
	fd := int(os.Stdin.Fd())
	termios, err := unix.IoctlGetTermios(fd, rawTermiosGet)
	if err != nil {
		return
	}
	termios.Oflag |= unix.OPOST | unix.ONLCR
	_ = unix.IoctlSetTermios(fd, rawTermiosSet, termios)
}
