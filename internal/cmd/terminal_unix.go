//go:build !windows

package cmd

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal returns true if the file is a terminal.
func isTerminal(f *os.File) bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		f.Fd(),
		ioctlGetTermios,
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0,
	)
	return err == 0
}
