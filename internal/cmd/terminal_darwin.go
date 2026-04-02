//go:build darwin

package cmd

import "syscall"

const ioctlGetTermios = syscall.TIOCGETA
