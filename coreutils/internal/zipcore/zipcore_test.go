package zipcore

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSafeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{name: "plain", in: "dir/file.txt", want: "dir/file.txt"},
		{name: "a directory entry", in: "dir/", want: "dir"},
		{name: "redundant separators", in: "dir//./sub/x", want: "dir/sub/x"},
		{name: "the root", in: ".", want: ""},
		{name: "empty", in: "", want: ""},
		{name: "leading slashes", in: "//tmp/target", want: "tmp/target"},
		{name: "backslash separators", in: `dir\file.txt`, want: "dir/file.txt"},
		{name: "a drive prefix", in: "C:/victim", want: "victim"},
		{name: "a drive prefix behind a dot", in: "./C:/victim", want: "victim"},
		{name: "a drive prefix without a slash", in: "D:victim", want: "victim"},
		{name: "a drive prefix below a directory", in: "dir/C:/x", want: "dir/C:/x"},
		{name: "dotdot is refused", in: "../evil", err: ErrDotDot},
		{name: "a dotdot component is refused", in: "a/../b", err: ErrDotDot},
		{name: "a backslash dotdot is refused", in: `..\evil`, err: ErrDotDot},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SafeName(test.in)
			if !errors.Is(err, test.err) {
				t.Errorf("SafeName(%q) error = %v, want %v", test.in, err, test.err)
			}
			if got != test.want {
				t.Errorf("SafeName(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestLineReader(t *testing.T) {
	t.Run("reads the lines, trimming their endings", func(t *testing.T) {
		lines := NewLineReader(strings.NewReader("one\r\ntwo\nthree"))
		ctx := t.Context()

		for _, want := range []string{"one", "two", "three"} {
			if got, err := lines.ReadLine(ctx); got != want || err != nil {
				t.Errorf("ReadLine() = %q, %v, want %q, nil", got, err, want)
			}
		}
		if _, err := lines.ReadLine(ctx); !errors.Is(err, io.EOF) {
			t.Errorf("ReadLine() error = %v, want %v", err, io.EOF)
		}
	})

	t.Run("keeps the whitespace of a line", func(t *testing.T) {
		lines := NewLineReader(strings.NewReader("  spaced  \n"))
		if got, err := lines.ReadLine(t.Context()); got != "  spaced  " || err != nil {
			t.Errorf("ReadLine() = %q, %v, want the line as typed", got, err)
		}
	})

	t.Run("keeps a trailing carriage return of a newline-terminated line", func(t *testing.T) {
		lines := NewLineReader(strings.NewReader("name\r\r\n"))
		if got, err := lines.ReadLine(t.Context()); got != "name\r" || err != nil {
			t.Errorf("ReadLine() = %q, %v, want name with its carriage return", got, err)
		}
	})

	t.Run("keeps the carriage return of a final line without a newline", func(t *testing.T) {
		lines := NewLineReader(strings.NewReader("name\r"))
		if got, err := lines.ReadLine(t.Context()); got != "name\r" || err != nil {
			t.Errorf("ReadLine() = %q, %v, want the line as typed", got, err)
		}
	})

	t.Run("skips an empty line as empty, not as the end", func(t *testing.T) {
		lines := NewLineReader(strings.NewReader("\nlast\n"))
		ctx := t.Context()

		if got, err := lines.ReadLine(ctx); got != "" || err != nil {
			t.Errorf("ReadLine() = %q, %v, want an empty line", got, err)
		}
		if got, err := lines.ReadLine(ctx); got != "last" || err != nil {
			t.Errorf("ReadLine() = %q, %v, want last", got, err)
		}
	})

	t.Run("a cancelled context stops a blocked read and leaves no goroutine behind", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		// The source blocks until the test releases it, standing for a
		// stream a closer can interrupt, which the pipe of the reader is.
		release := make(chan struct{})
		var releaseOnce sync.Once
		t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
		before := runtime.NumGoroutine()
		lines := NewLineReader(blockedReader{release: release})

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		done := make(chan error, 1)
		go func() {
			_, err := lines.ReadLine(ctx)
			done <- err
		}()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("ReadLine() error = %v, want %v", err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ReadLine() didn't return after the cancellation")
		}
		// The line read ends with the closing of the pipe the cancellation
		// did, and the copy of the source with the release below.
		releaseOnce.Do(func() { close(release) })
		deadline := time.Now().Add(time.Second)
		for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if got := runtime.NumGoroutine(); got > before {
			t.Errorf("goroutines = %d, want at most the %d the test started with", got, before)
		}
	})

	t.Run("the context wins when the line arrives with the cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		// The line only becomes readable once the context is done, so the
		// read and the cancellation arrive together.
		lines := NewLineReader(gatedReader{done: ctx.Done()})

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		if _, err := lines.ReadLine(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("ReadLine() error = %v, want %v", err, context.Canceled)
		}
	})
}

// blockedReader delivers nothing until its release channel closes.
type blockedReader struct {
	release <-chan struct{}
}

// Read implements io.Reader, blocking until the release.
func (b blockedReader) Read([]byte) (int, error) {
	<-b.release
	return 0, io.EOF
}

// gatedReader delivers one line once done closes.
type gatedReader struct {
	done <-chan struct{}
}

// Read implements io.Reader.
func (g gatedReader) Read(p []byte) (int, error) {
	<-g.done
	return copy(p, "y\n"), nil
}
