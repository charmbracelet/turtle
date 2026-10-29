//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package coreutils

import "syscall"

// ioctlReadTermios is the ioctl request that reads the terminal attributes.
const ioctlReadTermios = syscall.TIOCGETA
