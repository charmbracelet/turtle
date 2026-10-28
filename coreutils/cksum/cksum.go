// Package cksum implements the POSIX cksum utility.
package cksum

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
)

// chunkSize is how many bytes of an input are read at a time, which is also
// how often the context is checked while reading.
const chunkSize = 32 * 1024

// Prog is the cksum core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the checksum and size of each file, or of the standard input
// when no file is given.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	operands, err := (getopt.Set{}).Parse(args, func(opt rune, _ string) error {
		return getopt.InvalidOption(opt)
	})
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	// The pathname is left out only when no file was given, so that an
	// explicit "-" names the standard input like the digest utilities do.
	unnamed := len(operands) == 0
	if unnamed {
		operands = []string{"-"}
	}
	var failed bool
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		sum, size, err := p.checksum(ctx, name)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			p.errorf("%s: %v", name, unwrapPathError(err))
			failed = true
			continue
		}
		line := fmt.Sprintf("%d %d", sum, size)
		if !unnamed {
			line += " " + name
		}
		if _, err := io.WriteString(p.Stdout, line+"\n"); err != nil {
			p.errorf("write error: %v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	if failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// checksum returns the checksum and size of the file, reading it in chunks
// so that the context is checked while it runs.
func (p *Prog) checksum(ctx context.Context, name string) (uint32, int64, error) {
	r, err := input.Open(p.Runtime, ctx, input.New(name))
	if err != nil {
		return 0, 0, err
	}
	sum, size, err := p.hash(ctx, r)
	if closeErr := r.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, 0, err
	}
	return sum, size, nil
}

// hash returns the checksum and size of r.
func (p *Prog) hash(ctx context.Context, r io.Reader) (uint32, int64, error) {
	var c crc
	var size int64
	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		n, readErr := r.Read(buf)
		c.write(buf[:n])
		size += int64(n)
		if readErr != nil {
			if readErr != io.EOF {
				return 0, 0, readErr
			}
			break
		}
	}
	c.writeSize(size)
	return ^c.value, size, nil
}

// crc holds the checksum of a stream, as the algorithm of the cksum
// utility computes it.
type crc struct {
	value uint32
}

// write updates the checksum with the given bytes.
func (c *crc) write(data []byte) {
	for _, b := range data {
		c.value = c.value<<8 ^ crcTable[byte(c.value>>24)^b]
	}
}

// writeSize folds the size of the input into the checksum, as the length
// octets of the algorithm, least significant first.
func (c *crc) writeSize(size int64) {
	for size > 0 {
		c.value = c.value<<8 ^ crcTable[byte(c.value>>24)^byte(size)]
		size >>= 8
	}
}

// crcTable is the lookup table of the polynomial of the ISO/IEC 8802-3
// standard, as the cksum specification defines it.
var crcTable = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		c := uint32(i) << 24
		for range 8 {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04c11db7
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
	return table
}()

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "cksum: "+format+"\n", args...)
}

func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var _ coreutils.Program = (*Prog)(nil)
