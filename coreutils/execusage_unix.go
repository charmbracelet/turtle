//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package coreutils

import (
	"runtime"
	"syscall"
	"time"
)

// osExecUsage reads the cumulative resource usage of the waited children
// of the process, like getrusage(RUSAGE_CHILDREN). The kernel counters
// the platform doesn't track are reported as zero. On darwin the kernel
// reports the resident set size in bytes, while the other systems report
// it in kilobytes, so the value is normalized to bytes here.
func osExecUsage() (ExecUsage, bool) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &ru); err != nil {
		return ExecUsage{}, false
	}
	return ExecUsage{
		UserTime:            duration(ru.Utime),
		SysTime:             duration(ru.Stime),
		MaxRSS:              uint64(ru.Maxrss) * rssUnit,
		MinorFaults:         uint64(ru.Minflt),
		MajorFaults:         uint64(ru.Majflt),
		VoluntarySwitches:   uint64(ru.Nvcsw),
		InvoluntarySwitches: uint64(ru.Nivcsw),
		FSInputs:            uint64(ru.Inblock),
		FSOutputs:           uint64(ru.Oublock),
		SocketsReceived:     uint64(ru.Msgrcv),
		SocketsSent:         uint64(ru.Msgsnd),
		Swaps:               uint64(ru.Nswap),
		Signals:             uint64(ru.Nsignals),
	}, true
}

// duration converts a timeval into a duration.
func duration(t syscall.Timeval) time.Duration {
	return time.Duration(t.Sec)*time.Second + time.Duration(t.Usec)*time.Microsecond
}

// rssUnit converts the kernel's resident set size unit into bytes: the
// kernels of the other systems kept in this file report it in kilobytes,
// while Darwin reports bytes.
var rssUnit = func() uint64 {
	if runtime.GOOS == "darwin" {
		return 1
	}
	return 1024
}()
