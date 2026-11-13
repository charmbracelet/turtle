package shuf

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/bits"
)

// rng draws random bytes from a source as they're needed, so a source that
// never ends, like the system's entropy, is consumed without end too.
type rng struct {
	src    *bufio.Reader
	label  string
	closer io.Closer
}

// newRNG wraps a source of random bytes, named by label on diagnostics and
// released by Close when it's a file.
func newRNG(src io.Reader, label string, closer io.Closer) *rng {
	return &rng{src: bufio.NewReader(src), label: label, closer: closer}
}

// Close releases the source of random bytes.
func (g *rng) Close() error {
	if g.closer == nil {
		return nil
	}
	return g.closer.Close()
}

// below returns a uniform random integer in [0, n), rejecting the excess of
// the masked bytes so no value is favored. A range of one always maps to
// zero, without consuming any bytes.
func (g *rng) below(n uint64) (uint64, error) {
	if n < 2 {
		return 0, nil
	}
	mask := ^uint64(0) >> (64 - bits.Len64(n-1))
	for {
		x, err := g.next64()
		if err != nil {
			return 0, err
		}
		if v := x & mask; v < n {
			return v, nil
		}
	}
}

// next64 returns the next 64 random bits, reading 8 bytes from the source.
func (g *rng) next64() (uint64, error) {
	var b [8]byte
	if _, err := io.ReadFull(g.src, b[:]); err != nil {
		return 0, &sourceError{label: g.label, err: readFail(err)}
	}
	return binary.LittleEndian.Uint64(b[:]), nil
}

// readFail maps the end of a source to the GNU diagnostic, keeping the other
// failures as they are.
func readFail(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("end of file")
	}
	return err
}

// sourceError marks an error as coming from the random source, so the
// diagnostics can name it.
type sourceError struct {
	label string
	err   error
}

// Error implements the error interface.
func (e *sourceError) Error() string {
	return e.err.Error()
}

// Unwrap makes the failure of the source reachable with errors.Is and As.
func (e *sourceError) Unwrap() error {
	return e.err
}

// ctxReader stops reads with the context error once ctx is done, so a long
// shuffle can be interrupted while it draws random bytes.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// Read implements the io.Reader interface.
func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// shuffle permutes lines in place with Fisher-Yates, so every ordering is
// equally likely.
func shuffle(ctx context.Context, g *rng, lines []string) error {
	for i := len(lines) - 1; i > 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		j, err := g.below(uint64(i + 1))
		if err != nil {
			return err
		}
		lines[i], lines[j] = lines[j], lines[i]
	}
	return nil
}

// The bounds on the memory shuf spends on the indices of a shuffle, kept at
// sizes machines are expected to be able to allocate. A permutation holds
// one 8-byte index per item, and a sample holds one index plus its map of
// the seen ones, which reserves twice the items at about 16 bytes an entry
// for its capacity hint.
const (
	maxIndexBytes = 1 << 26
	maxPermItems  = maxIndexBytes / 8
	maxSample     = maxIndexBytes / (8 + 16*2)
)

// errTooManyItems reports a range too large to shuffle.
var errTooManyItems = errors.New("too many input lines")

// sampleIndices returns k distinct indices in [0, n) in random order, as a
// uniform random sample without replacement. A sample too large to be held
// in memory is refused instead of being allocated.
func sampleIndices(ctx context.Context, g *rng, n, k uint64) ([]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k = min(k, n)
	if k*2 >= n {
		// A partial Fisher-Yates over all the indices, which needs the whole
		// range held in memory.
		if n > maxPermItems {
			return nil, errTooManyItems
		}
		idx := make([]uint64, n)
		for i := range idx {
			idx[i] = uint64(i)
		}
		for i := uint64(0); i < k; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			j, err := g.below(n - i)
			if err != nil {
				return nil, err
			}
			j += i
			idx[i], idx[j] = idx[j], idx[i]
		}
		return idx[:k], nil
	}
	// Few picks of many: retry until each one is distinct.
	if k > maxSample {
		return nil, errTooManyItems
	}
	seen := make(map[uint64]struct{}, k*2)
	idx := make([]uint64, k)
	for i := range idx {
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			j, err := g.below(n)
			if err != nil {
				return nil, err
			}
			if _, dup := seen[j]; !dup {
				seen[j] = struct{}{}
				idx[i] = j
				break
			}
		}
	}
	return idx, nil
}
