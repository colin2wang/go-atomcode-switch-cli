//go:build linux

package cli

import "golang.org/x/sys/unix"

const (
	rawTermiosGet = unix.TCGETS
	rawTermiosSet = unix.TCSETS
)
