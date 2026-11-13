package shuf

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// endlessReader is a source of random bytes that never ends, like
// /dev/urandom.
type endlessReader struct {
	seed uint64
}

func (r *endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		r.seed = r.seed*6364136223846793005 + 1442695040888963407
		p[i] = byte(r.seed >> 56)
	}
	return len(p), nil
}

func TestRNG(t *testing.T) {
	t.Run("an empty source fails at its end", func(t *testing.T) {
		g := newRNG(strings.NewReader(""), "empty", nil)
		_, err := g.below(4)
		var srcErr *sourceError
		if !errors.As(err, &srcErr) {
			t.Fatalf("below() error = %v, want a source error", err)
		}
		if srcErr.label != "empty" {
			t.Errorf("source error label = %q, want %q", srcErr.label, "empty")
		}
		if got, want := srcErr.err.Error(), "end of file"; got != want {
			t.Errorf("source error = %q, want %q", got, want)
		}
	})

	t.Run("a partially drawn value fails too", func(t *testing.T) {
		g := newRNG(strings.NewReader("0123456"), "short", nil)
		_, err := g.below(4)
		var srcErr *sourceError
		if !errors.As(err, &srcErr) {
			t.Fatalf("below() error = %v, want a source error", err)
		}
		if got, want := srcErr.err.Error(), "end of file"; got != want {
			t.Errorf("source error = %q, want %q", got, want)
		}
	})

	t.Run("a range of one consumes nothing", func(t *testing.T) {
		g := newRNG(strings.NewReader(""), "empty", nil)
		v, err := g.below(1)
		if v != 0 || err != nil {
			t.Errorf("below(1) = %d, %v, want 0, nil", v, err)
		}
	})

	t.Run("reads stop when the context is done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		g := newRNG(&ctxReader{ctx: ctx, r: &endlessReader{}}, "endless", nil)
		if _, err := g.below(4); !errors.Is(err, context.Canceled) {
			t.Errorf("below() error = %v, want context.Canceled", err)
		}
	})
}

func TestRNGEndless(t *testing.T) {
	t.Run("a source that never ends doesn't hang", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		lines := make([]string, 100)
		for i := range lines {
			lines[i] = strconv.Itoa(i)
		}

		done := make(chan error, 1)
		go func() {
			done <- shuffle(t.Context(), g, lines)
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("shuffle() error = %v, want nil", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("shuffle() didn't finish on a source that never ends")
		}

		slices.SortFunc(lines, func(a, b string) int {
			na, _ := strconv.Atoi(a)
			nb, _ := strconv.Atoi(b)
			return na - nb
		})
		for i, line := range lines {
			if n, err := strconv.Atoi(line); err != nil || n != i {
				t.Errorf("shuffled line %d = %q, want %d", i, line, i)
				break
			}
		}
	})
}

func TestSampleIndices(t *testing.T) {
	t.Run("a sample holds distinct indices of the range", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		idx, err := sampleIndices(t.Context(), g, 100, 10)
		if err != nil {
			t.Fatalf("sampleIndices() error = %v, want nil", err)
		}
		if len(idx) != 10 {
			t.Fatalf("sampleIndices() returned %d indices, want 10", len(idx))
		}
		seen := make(map[uint64]bool, len(idx))
		for _, i := range idx {
			if i >= 100 {
				t.Errorf("index %d is out of the range", i)
				return
			}
			if seen[i] {
				t.Errorf("index %d repeats", i)
				return
			}
			seen[i] = true
		}
	})

	t.Run("the whole range is returned when k reaches n", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		idx, err := sampleIndices(t.Context(), g, 50, noCount)
		if err != nil {
			t.Fatalf("sampleIndices() error = %v, want nil", err)
		}
		slices.Sort(idx)
		for i, v := range idx {
			if v != uint64(i) {
				t.Errorf("sorted index %d = %d, want %d", i, v, i)
				break
			}
		}
	})

	t.Run("a range too large to permute is refused", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		_, err := sampleIndices(t.Context(), g, maxPermItems+1, noCount)
		if !errors.Is(err, errTooManyItems) {
			t.Errorf("sampleIndices() error = %v, want errTooManyItems", err)
		}
	})

	t.Run("a range representable but unallocatable is refused", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		_, err := sampleIndices(t.Context(), g, 1<<60, noCount)
		if !errors.Is(err, errTooManyItems) {
			t.Errorf("sampleIndices() error = %v, want errTooManyItems", err)
		}
	})

	t.Run("a sample too large to hold is refused", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		_, err := sampleIndices(t.Context(), g, 1<<60, maxSample+1)
		if !errors.Is(err, errTooManyItems) {
			t.Errorf("sampleIndices() error = %v, want errTooManyItems", err)
		}
	})

	t.Run("a huge range with a small sample needs no allocation", func(t *testing.T) {
		g := newRNG(&endlessReader{}, "endless", nil)
		idx, err := sampleIndices(t.Context(), g, 1<<60, 5)
		if err != nil {
			t.Fatalf("sampleIndices() error = %v, want nil", err)
		}
		if len(idx) != 5 {
			t.Fatalf("sampleIndices() returned %d indices, want 5", len(idx))
		}
		seen := make(map[uint64]bool, len(idx))
		for _, i := range idx {
			if i >= 1<<60 {
				t.Errorf("index %d is out of the range", i)
				return
			}
			if seen[i] {
				t.Errorf("index %d repeats", i)
				return
			}
			seen[i] = true
		}
	})

	t.Run("a done context is reported before allocating", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		g := newRNG(&endlessReader{}, "endless", nil)
		_, err := sampleIndices(ctx, g, 1<<60, noCount)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("sampleIndices() error = %v, want context.Canceled", err)
		}
	})

	t.Run("a done context stops the sampling", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		g := newRNG(&endlessReader{}, "endless", nil)
		_, err := sampleIndices(ctx, g, 1000, 10)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("sampleIndices() error = %v, want context.Canceled", err)
		}
	})
}
