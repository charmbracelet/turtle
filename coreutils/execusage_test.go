package coreutils

import (
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestExecUsageSub(t *testing.T) {
	snapshot := func(fields ...uint64) ExecUsage {
		if len(fields) != 13 {
			t.Fatalf("snapshot needs 13 counters, got %d", len(fields))
		}
		return ExecUsage{
			UserTime:            time.Duration(fields[0]),
			SysTime:             time.Duration(fields[1]),
			MaxRSS:              fields[2],
			MinorFaults:         fields[3],
			MajorFaults:         fields[4],
			VoluntarySwitches:   fields[5],
			InvoluntarySwitches: fields[6],
			FSInputs:            fields[7],
			FSOutputs:           fields[8],
			SocketsReceived:     fields[9],
			SocketsSent:         fields[10],
			Swaps:               fields[11],
			Signals:             fields[12],
		}
	}

	t.Run("the counts subtract and the rss keeps the higher mark", func(t *testing.T) {
		before := snapshot(1, 2, 4096, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14)
		after := snapshot(3, 5, 6144, 9, 8, 11, 10, 11, 15, 12, 14, 16, 20)
		want := snapshot(2, 3, 6144, 4, 2, 4, 2, 2, 5, 1, 2, 3, 6)
		if got := after.Sub(before); !reflect.DeepEqual(got, want) {
			t.Errorf("delta = %+v, want %+v", got, want)
		}
	})

	t.Run("a lower later snapshot saturates at zero", func(t *testing.T) {
		before := snapshot(10, 10, 9000, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
		after := snapshot(3, 3, 1000, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3)
		if got := after.Sub(before); got != (ExecUsage{MaxRSS: 9000}) {
			t.Errorf("delta = %+v, want the zeroed counters with MaxRSS 9000", got)
		}
	})

	t.Run("equal snapshots yield zeros and the rss", func(t *testing.T) {
		one := snapshot(1, 1, 2048, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1)
		if got := one.Sub(one); got != (ExecUsage{MaxRSS: 2048}) {
			t.Errorf("delta = %+v, want the zeroed counters with MaxRSS 2048", got)
		}
	})
}

func TestOsExecUsage(t *testing.T) {
	t.Run("the unix hook reports the children counts", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows has no rusage counts")
		}
		if _, ok := osExecUsage(); !ok {
			t.Errorf("osExecUsage = ok false, want the unix counts available")
		}
	})
}
