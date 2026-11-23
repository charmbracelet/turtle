//go:build unix

package stat

import (
	"syscall"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

// TestBlocksReportedByTheSystem checks that a block count the system
// reports is written as it is, even when it's zero: a sparse file uses no
// blocks, and its count isn't measured by its size.
func TestBlocksReportedByTheSystem(t *testing.T) {
	p := &Prog{Runtime: &coreutils.Runtime{FS: fsx.MemoryFS{}}}
	r := &renderer{prog: p, loc: time.UTC, style: quoteShellEscape}
	tests := []struct {
		name   string
		blocks int64
		want   string
	}{
		{name: "zero blocks of a sparse file", blocks: 0, want: "0"},
		{name: "a count the system reports", blocks: 8, want: "8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := p.newFileStat("f", "f", &fakeInfo{
				mode: 0o644,
				size: 4096,
				sys:  &syscall.Stat_t{Blocks: test.blocks},
			}, options{})
			out, fail, err := r.render("%b", &st, false, false)
			if err != nil || fail {
				t.Fatalf("render() = %q, %v, %v, want no failure", out, fail, err)
			}
			if out != test.want {
				t.Errorf("render() = %q, want %q", out, test.want)
			}
		})
	}
}
