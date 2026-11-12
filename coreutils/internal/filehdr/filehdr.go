// Package filehdr writes the "==> name <==" headers which precede the
// output of each input file, as head and tail print them.
package filehdr

import (
	"fmt"
	"io"
)

// Mode is when a utility writes the "==> name <==" header of an input file.
type Mode byte

const (
	// Auto writes the headers only when there is more than one operand.
	Auto Mode = iota
	// Never never writes them, as with -q.
	Never
	// Always always writes them, as with -v.
	Always
)

// Enabled reports whether the headers are written, given whether there are
// multiple input operands.
func (m Mode) Enabled(multiple bool) bool {
	switch m {
	case Never:
		return false
	case Always:
		return true
	default:
		return multiple
	}
}

// Writer writes the headers of one output, remembering whether one was
// already written: the first header has no leading blank line, and the
// others are separated from the output of the previous file by one.
//
// A Writer is not safe for concurrent use. Utilities which write from
// multiple goroutines keep their own lock around it.
type Writer struct {
	w       io.Writer
	printed bool
}

// New returns a Writer which writes the headers to w.
func New(w io.Writer) Writer {
	return Writer{w: w}
}

// Write writes the header of the input named name. Write errors are
// reported by the copy of the file output, like head and tail do.
func (h *Writer) Write(name string) {
	if h.printed {
		fmt.Fprint(h.w, "\n")
	}
	h.printed = true
	fmt.Fprintf(h.w, "==> %s <==\n", name)
}
