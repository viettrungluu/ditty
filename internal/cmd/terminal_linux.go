//go:build linux

package cmd

import "syscall"

const ioctlGetTermios = syscall.TCGETS
