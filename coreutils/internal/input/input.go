// Package input handles the input operands shared by the core utilities:
// mapping the operands to files or the standard input, and opening them.
package input

import (
	"context"
	"errors"
	"io"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/ctxio"
)

// Input is one input operand: a pathname, or the standard input.
type Input struct {
	Name  string
	Stdin bool
}

// New maps an operand to an Input, reading "-" as the standard input.
func New(name string) Input {
	return Input{Name: name, Stdin: name == "-"}
}

// List maps the operands to inputs, reading "-" as the standard input. With
// no operands, the standard input is used.
func List(operands []string) []Input {
	if len(operands) == 0 {
		return []Input{{Stdin: true}}
	}
	inputs := make([]Input, len(operands))
	for i, operand := range operands {
		inputs[i] = New(operand)
	}
	return inputs
}

// Label returns the name to use on diagnostic messages.
func (in Input) Label() string {
	if in.Stdin && in.Name == "" {
		return "standard input"
	}
	return in.Name
}

// Open returns a reader for the input. Reads stop early when ctx is done.
// The standard input isn't owned by the caller, so it's wrapped on a no-op
// closer.
func Open(rt *coreutils.Runtime, ctx context.Context, in Input) (io.ReadCloser, error) {
	if in.Stdin {
		if rt.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxio.NewReader(ctx, rt.Stdin), nil
	}
	if rt.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := rt.FS.Open(rt.FsName(in.Name))
	if err != nil {
		return nil, err
	}
	return ctxio.ReadCloser{Ctx: ctx, RC: f}, nil
}

// ReadAll reads the whole input: the file named, or the standard input for
// the "-" operand, which the caller also passes when its operand list is
// empty. Reads stop early when ctx is done.
func ReadAll(rt *coreutils.Runtime, ctx context.Context, name string) ([]byte, error) {
	r, err := Open(rt, ctx, New(name))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// ReadError wraps a read failure with the label of the input it came from.
type ReadError struct {
	Label string
	Err   error
}

// Error implements the error interface.
func (e *ReadError) Error() string {
	return e.Err.Error()
}

// Unwrap implements the interface to make ReadError unwrapable.
func (e *ReadError) Unwrap() error {
	return e.Err
}
