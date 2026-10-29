//go:build linux

package term

import "syscall"

// ioctlReadTermios is the ioctl request that reads the terminal attributes.
const ioctlReadTermios = syscall.TCGETS
