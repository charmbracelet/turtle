//go:build linux

package coreutils

import "syscall"

// ioctlReadTermios is the ioctl request that reads the terminal attributes.
const ioctlReadTermios = syscall.TCGETS
