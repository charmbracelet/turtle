// Package uuencode implements the POSIX uuencode utility.
//
// It writes an encoded version of the input to the standard output, either
// with the historical algorithm or, with -m, with base64. The output opens
// with a "begin" header holding the mode bits and the name uudecode needs to
// recreate the file, and closes with the trailer of each algorithm.
package uuencode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/basex"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/uux"
)

// Prog is the uuencode core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the encoding of the input to the standard output. The standard
// input is used when no file operand is given, and also when it's "-". The
// header announces the permission bits of the input file, or the default
// file creation mode when reading the standard input.
//
// Reading is interrupted when ctx is done, and the context error is
// returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	base64, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	switch {
	case len(operands) == 0:
		p.errorf("missing operand")
		return &coreutils.ExitError{Code: 2}
	case len(operands) > 2:
		p.errorf("extra operand '%s'", operands[2])
		return &coreutils.ExitError{Code: 2}
	}

	name := operands[len(operands)-1]
	in := input.Input{Stdin: true}
	if len(operands) == 2 {
		in = input.New(operands[0])
	}
	if !in.Stdin && in.Name == "" {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	r, mode, err := p.open(in)
	if err != nil {
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	defer r.Close()

	w := &errWriter{w: p.Stdout}
	err = p.encode(ctx, r, w, mode, name, base64)
	switch {
	case w.err != nil:
		p.errorf("write error: %v", w.err)
		return &coreutils.ExitError{Code: 1, Err: w.err}
	case ctx.Err() != nil:
		return ctx.Err()
	case err == nil:
		return nil
	default:
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
}

// encode writes the encoding of r to w: the "begin" header, the encoded
// lines and the trailer.
func (p *Prog) encode(ctx context.Context, r io.Reader, w io.Writer, mode fs.FileMode, name string, base64 bool) error {
	if _, err := io.WriteString(w, uux.BeginLine(base64, mode, name)); err != nil {
		return err
	}
	var err error
	if base64 {
		err = basex.Base64.Encode(ctx, w, r, uux.Base64Wrap)
	} else {
		err = uux.Encode(ctx, w, r)
	}
	if err != nil {
		return err
	}
	trailer := "`\nend\n"
	if base64 {
		trailer = "====\n"
	}
	_, err = io.WriteString(w, trailer)
	return err
}

// open returns a reader for the input, with the mode the header announces:
// the permission bits of a file, or the default creation mode for the
// standard input.
func (p *Prog) open(in input.Input) (io.ReadCloser, fs.FileMode, error) {
	if in.Stdin {
		if p.Stdin == nil {
			return nil, 0, errors.New("no standard input available")
		}
		return io.NopCloser(p.Stdin), 0o666 &^ p.Umask.Perm(), nil
	}
	if p.FS == nil {
		return nil, 0, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.Name)))
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, fi.Mode().Perm(), nil
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "uuencode: "+format+"\n", args...)
}

// longOptions maps the long options of uuencode to the short ones they
// stand for.
var longOptions = map[string]rune{
	"base64": 'm',
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (base64 bool, operands []string, err error) {
	operands, err = getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		if opt != 'm' {
			return getopt.InvalidOption(opt)
		}
		base64 = true
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return base64, operands, nil
}

// errWriter records the first write error, so a read error isn't mistaken
// for one.
type errWriter struct {
	w   io.Writer
	err error
}

// Write implements io.Writer.
func (w *errWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
