package filehdr

import (
	"errors"
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		mode     Mode
		multiple bool
		want     bool
	}{
		{Auto, false, false},
		{Auto, true, true},
		{Never, false, false},
		{Never, true, false},
		{Always, false, true},
		{Always, true, true},
	}
	for _, test := range tests {
		if got := test.mode.Enabled(test.multiple); got != test.want {
			t.Errorf("%v.Enabled(%v) = %v, want %v", test.mode, test.multiple, got, test.want)
		}
	}
}

func TestWrite(t *testing.T) {
	t.Run("first", func(t *testing.T) {
		var b strings.Builder
		h := New(&b)
		h.Write("a.txt")
		if want := "==> a.txt <==\n"; b.String() != want {
			t.Errorf("Write = %q, want %q", b.String(), want)
		}
	})
	t.Run("separated", func(t *testing.T) {
		var b strings.Builder
		h := New(&b)
		h.Write("a.txt")
		h.Write("standard input")
		if want := "==> a.txt <==\n\n==> standard input <==\n"; b.String() != want {
			t.Errorf("writes = %q, want %q", b.String(), want)
		}
	})
}

// errWriter fails every write, so Write ignores the errors like the
// utilities which report them from the copy of the file output instead.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("no")
}

func TestWriteError(t *testing.T) {
	h := New(errWriter{})
	h.Write("a.txt")
	h.Write("b.txt")
}
