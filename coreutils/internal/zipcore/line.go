package zipcore

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// LineReader reads the lines of a reader one by one, stopping with the
// context error when a run is cancelled, even in the middle of a read
// blocked on input. Each line comes without its ending, and the end of the
// input reads as io.EOF.
//
// The source is copied through a pipe on a goroutine of its own, and every
// cancellation, and Close too, closes that pipe: the line being read and
// the copy then stop, leaving nothing behind but a copy already stuck
// reading the source itself, which nothing can interrupt.
type LineReader struct {
	pipe   *io.PipeReader
	reader *bufio.Reader
}

// line is one read of a line: what ReadString returned.
type line struct {
	text string
	err  error
}

// NewLineReader returns a LineReader reading r.
func NewLineReader(r io.Reader) *LineReader {
	pipe, out := io.Pipe()
	lines := &LineReader{pipe: pipe, reader: bufio.NewReader(pipe)}
	go func() {
		_, err := io.Copy(out, r)
		out.CloseWithError(err)
	}()
	return lines
}

// Close stops the reader, ending the copy of its source.
func (l *LineReader) Close() error {
	return l.pipe.Close()
}

// ReadLine reads the next line, without its line ending: a newline, with
// the carriage return preceding it when there's one. A final line without
// its newline keeps every character, as does a name ending in a carriage
// return.
func (l *LineReader) ReadLine(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// The line is read on its own so a cancellation doesn't wait for it,
	// and its goroutine leaves with the closing of the pipe below.
	read := make(chan line, 1)
	go func() {
		text, err := l.reader.ReadString('\n')
		read <- line{text, err}
	}()
	select {
	case <-ctx.Done():
		l.Close()
		return "", ctx.Err()
	case got := <-read:
		// The line and the cancellation can both arrive: the run is over,
		// so the context wins.
		if err := ctx.Err(); err != nil {
			l.Close()
			return "", err
		}
		if got.text == "" {
			return "", got.err
		}
		if text, ok := strings.CutSuffix(got.text, "\n"); ok {
			text, _ = strings.CutSuffix(text, "\r")
			return text, nil
		}
		return got.text, nil
	}
}
