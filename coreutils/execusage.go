package coreutils

import "time"

// ExecUsageFunc reports the cumulative resource usage of the children
// the runtime has executed so far, reporting whether such counts are
// available. The reported value counts every child completed since the
// runtime started, so callers take a delta between two reports to
// attribute a share to one execution.
type ExecUsageFunc func() (ExecUsage, bool)

// ExecUsage carries the resource usage of executed children, in the
// portable shape of the rusage counters. All the counters besides MaxRSS
// are cumulative totals, so a delta of two snapshots yields the share of
// the executions between them; MaxRSS is the high-water mark of the
// children, so a delta keeps the larger of the two snapshots. The
// kernel-dependent fields a platform doesn't track are reported as zero.
type ExecUsage struct {
	// UserTime is the user CPU time of the children.
	UserTime time.Duration

	// SysTime is the system CPU time of the children.
	SysTime time.Duration

	// MaxRSS is the maximum resident set size of a child, in bytes.
	MaxRSS uint64

	// MinorFaults and MajorFaults are the page faults the children took,
	// not needing and needing real I/O, respectively.
	MinorFaults, MajorFaults uint64

	// VoluntarySwitches and InvoluntarySwitches are the context switches
	// the children gave up and were preempted through.
	VoluntarySwitches, InvoluntarySwitches uint64

	// FSInputs and FSOutputs are the physical block reads and writes of
	// the children.
	FSInputs, FSOutputs uint64

	// SocketsReceived and SocketsSent are the socket messages the children
	// received and sent.
	SocketsReceived, SocketsSent uint64

	// Swaps are the times the children were swapped out.
	Swaps uint64

	// Signals are the signals the children received.
	Signals uint64
}

// Sub returns the delta of the two snapshots, subtracting each counter
// of other from the one of u, and keeping the larger MaxRSS, as a high
// water mark. Negative counter differences saturate at zero, so reused
// or out-of-order snapshots degrade to zeroed counters rather than
// wrapping.
func (u ExecUsage) Sub(other ExecUsage) ExecUsage {
	return ExecUsage{
		UserTime:            max(u.UserTime-other.UserTime, 0),
		SysTime:             max(u.SysTime-other.SysTime, 0),
		MaxRSS:              max(u.MaxRSS, other.MaxRSS),
		MinorFaults:         delta(u.MinorFaults, other.MinorFaults),
		MajorFaults:         delta(u.MajorFaults, other.MajorFaults),
		VoluntarySwitches:   delta(u.VoluntarySwitches, other.VoluntarySwitches),
		InvoluntarySwitches: delta(u.InvoluntarySwitches, other.InvoluntarySwitches),
		FSInputs:            delta(u.FSInputs, other.FSInputs),
		FSOutputs:           delta(u.FSOutputs, other.FSOutputs),
		SocketsReceived:     delta(u.SocketsReceived, other.SocketsReceived),
		SocketsSent:         delta(u.SocketsSent, other.SocketsSent),
		Swaps:               delta(u.Swaps, other.Swaps),
		Signals:             delta(u.Signals, other.Signals),
	}
}

// delta subtracts two unsigned counters, saturating at zero.
func delta(out, in uint64) uint64 {
	if out < in {
		return 0
	}
	return out - in
}
