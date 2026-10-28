// Package tail implements the POSIX tail utility.
//
// It copies the last part of the given file to the standard output, starting
// at the point given by the -n or -c options, defaulting to the last 10
// lines. The standard input is used when no operand is given, and also when
// the operand is "-". With -f, the input is monitored for further data after
// the designated part is copied.
package tail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/fsx"
)

// followInterval is how long -f waits between checks for further data, like
// the loop described on the POSIX rationale.
const followInterval = time.Second

// Prog is the tail core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the designated part of the input file to the standard output.
// The standard input is used when no operand is given, and also when the
// operand is "-". With -f, the input keeps being copied as further data
// becomes available, and Run only returns once ctx is done.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 1 {
		p.errorf("extra operand '%s'", operands[1])
		return &coreutils.ExitError{Code: 2}
	}

	var in input
	if len(operands) == 1 {
		in = newInput(operands[0])
		if !in.stdin && in.name == "" {
			p.errorf("invalid zero-length file name")
			return &coreutils.ExitError{Code: 1}
		}
	} else {
		in = input{stdin: true}
	}

	if err := p.tail(ctx, in, opts); err != nil {
		var writeErr *writeError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			p.errorf("%s: %v", in.label(), unwrapPathError(err))
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

// tail copies the designated part of the input and follows it with -f.
func (p *Prog) tail(ctx context.Context, in input, opts options) error {
	rc, f, err := p.open(in)
	if err != nil {
		return err
	}
	defer rc.Close()

	r := ctxio.NewReader(ctx, rc)
	if err := p.copyPortion(r, f, opts); err != nil {
		return err
	}
	if opts.follow {
		return p.follow(ctx, r)
	}
	return nil
}

// copyPortion copies the part of the input selected by the options.
func (p *Prog) copyPortion(r io.Reader, f fsx.SeekFile, opts options) error {
	switch {
	case opts.reverse:
		return p.reverse(r, opts)
	case opts.lines && opts.fromStart:
		return p.copyFromLine(r, opts.count)
	case opts.lines:
		return p.copyLastLines(r, f, opts.count)
	case opts.fromStart:
		return p.copyFromByte(r, f, opts.count)
	default:
		return p.copyLastBytes(r, f, opts.count)
	}
}

// copyLastBytes copies the last count bytes, seeking when the input is
// seekable and buffering otherwise.
func (p *Prog) copyLastBytes(r io.Reader, f fsx.SeekFile, count int) error {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	if f != nil {
		if err := seekLastBytes(f, count); err != nil {
			return err
		}
		return p.copyAll(r)
	}
	data, err := readLastBytes(r, count)
	if err != nil {
		return err
	}
	return p.writeBytes(data)
}

// copyFromByte copies everything from the count'th byte on. The origin is 1.
func (p *Prog) copyFromByte(r io.Reader, f fsx.SeekFile, count int) error {
	if f != nil {
		if _, err := f.Seek(int64(count-1), io.SeekStart); err != nil {
			return err
		}
	} else if _, err := io.CopyN(io.Discard, r, int64(count-1)); err != nil &&
		!errors.Is(err, io.EOF) {
		return err
	}
	return p.copyAll(r)
}

// copyLastLines copies the last count lines, seeking backwards when the
// input is seekable and buffering otherwise. A final line without a trailing
// <newline> counts as a line.
func (p *Prog) copyLastLines(r io.Reader, f fsx.SeekFile, count int) error {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	if f != nil {
		if err := seekLastLines(f, count); err != nil {
			return err
		}
		return p.copyAll(r)
	}
	lines, err := readLastLines(r, count)
	if err != nil {
		return err
	}
	return p.writeLines(lines)
}

// copyFromLine copies everything from the count'th line on. The origin is 1.
func (p *Prog) copyFromLine(r io.Reader, count int) error {
	br := bufio.NewReader(r)
	for i := 0; i < count-1; i++ {
		if _, err := br.ReadBytes('\n'); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return p.copyAll(br)
}

// reverse writes the lines of the input in reverse order, last line first,
// concatenated as they are read. With -n, only the last count lines are
// written.
func (p *Prog) reverse(r io.Reader, opts options) error {
	var (
		lines [][]byte
		err   error
	)
	if opts.countSet {
		lines, err = readLastLines(r, opts.count)
	} else {
		lines, err = readAllLines(r)
	}
	if err != nil {
		return err
	}
	for _, line := range slices.Backward(lines) {
		if werr := p.writeBytes(line); werr != nil {
			return werr
		}
	}
	return nil
}

// copyAll copies everything left on r to the standard output.
func (p *Prog) copyAll(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if werr := p.writeBytes(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// follow keeps copying further data as it becomes available, waiting between
// checks for growth, like the loop the POSIX rationale describes.
func (p *Prog) follow(ctx context.Context, r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if werr := p.writeBytes(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			return err
		}
		if err := p.Wait(ctx, followInterval); err != nil {
			return err
		}
	}
}

// writeBytes writes data to the standard output, wrapping write errors.
func (p *Prog) writeBytes(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if _, err := p.Stdout.Write(data); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeLines writes each line to the standard output.
func (p *Prog) writeLines(lines [][]byte) error {
	for _, line := range lines {
		if err := p.writeBytes(line); err != nil {
			return err
		}
	}
	return nil
}

// open returns a reader for the input, along with the file when it's
// seekable: reading from the end then avoids buffering the whole input.
func (p *Prog) open(in input) (io.ReadCloser, fsx.SeekFile, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, nil, errors.New("no standard input available")
		}
		return io.NopCloser(p.Stdin), nil, nil
	}
	if p.FS == nil {
		return nil, nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.name)))
	if err != nil {
		return nil, nil, err
	}
	var seek fsx.SeekFile
	if s, ok := f.(fsx.SeekFile); ok {
		seek = s
	}
	return f, seek, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "tail: "+format+"\n", args...)
}

// parseFlags parses the options and operands. The -c and -n options take a
// decimal number, optionally signed: a '+' makes copying start relative to
// the beginning of the input instead of the end. The last of the two given
// wins. Option parsing stops at the first operand or at "--".
// longOptions maps the long options of tail to the short ones they stand
// for. The -r option has no long form in GNU.
var longOptions = map[string]rune{
	"bytes":  'c',
	"follow": 'f',
	"lines":  'n',
}

func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{Valued: "cn", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
			opts.follow = true
		case 'r':
			opts.reverse = true
		case 'c', 'n':
			count, fromStart, err := parseCount(value)
			if err != nil {
				return err
			}
			opts.lines = opt == 'n'
			opts.countSet = true
			opts.fromStart = fromStart
			opts.count = count
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}

	switch {
	case opts.reverse && opts.follow:
		return options{}, nil, errors.New("the -f option is not valid with -r")
	case opts.reverse && opts.countSet && !opts.lines:
		return options{}, nil, errors.New("the -c option is not valid with -r")
	case opts.reverse && opts.fromStart:
		return options{}, nil, fmt.Errorf("invalid number: '+%d'", opts.count)
	}
	if !opts.countSet && !opts.reverse {
		opts.lines = true
		opts.count = 10
	}
	return opts, operands, nil
}

// parseCount parses the number option-argument of -c and -n: a decimal
// integer, optionally signed. A '+' sign makes copying start relative to the
// beginning of the input and is required to be non-zero. A '-' sign counts
// from the end, like no sign at all.
func parseCount(value string) (count int, fromStart bool, err error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid number: '%s'", value)
	}
	if n < 0 {
		n = -n
	}
	fromStart = strings.HasPrefix(value, "+")
	if fromStart && n == 0 {
		return 0, false, fmt.Errorf("invalid number: '%s'", value)
	}
	return n, fromStart, nil
}

// fileSize returns the current size of f.
func fileSize(f fsx.SeekFile) (int64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// seekLastBytes seeks f to where the last count bytes begin.
func seekLastBytes(f fsx.SeekFile, count int) error {
	size, err := fileSize(f)
	if err != nil {
		return err
	}
	_, err = f.Seek(max(0, size-int64(count)), io.SeekStart)
	return err
}

// seekLastLines seeks f to where the last count lines begin.
//
// The file is scanned backwards in chunks, growing the region until count
// <newline> separators are found or the start of the file is reached. The
// scan starts before a trailing <newline>, which ends the last line rather
// than separating it.
func seekLastLines(f fsx.SeekFile, count int) error {
	size, err := fileSize(f)
	if err != nil {
		return err
	}

	const chunk = 4096
	found := 0
	start := size
	for start > 0 {
		low := max(0, start-chunk)
		b := make([]byte, start-low)
		if _, err := f.Seek(low, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.ReadFull(f, b); err != nil {
			return err
		}
		start = low

		i := len(b) - 1
		if start+int64(len(b)) == size && b[i] == '\n' {
			i--
		}
		for ; i >= 0; i-- {
			if b[i] != '\n' {
				continue
			}
			found++
			if found == count {
				_, err := f.Seek(start+int64(i)+1, io.SeekStart)
				return err
			}
		}
	}
	_, err = f.Seek(0, io.SeekStart)
	return err
}

// readLastBytes returns the last count bytes of r.
func readLastBytes(r io.Reader, count int) ([]byte, error) {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return nil, err
	}
	var buf []byte
	tmp := make([]byte, 32*1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if len(buf) > count {
			buf = buf[len(buf)-count:]
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return buf, nil
			}
			return nil, err
		}
	}
}

// readLastLines returns the last count lines of r, each including its
// trailing <newline> when the input has one.
func readLastLines(r io.Reader, count int) ([][]byte, error) {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return nil, err
	}
	var lines [][]byte
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lines = append(lines, line)
			if len(lines) > count {
				lines = lines[len(lines)-count:]
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
	}
}

// readAllLines returns all lines of r, each including its trailing <newline>
// when the input has one.
func readAllLines(r io.Reader) ([][]byte, error) {
	var lines [][]byte
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lines = append(lines, line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
	}
}

// options holds the part of the input to copy.
type options struct {
	follow    bool // -f: keep copying as further data becomes available
	reverse   bool // -r: write the lines in reverse order
	lines     bool // -n instead of -c
	countSet  bool // -c or -n given
	fromStart bool // '+' sign: count from the beginning
	count     int
}

// input describes an input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// newInput maps an operand to an input, reading "-" as the standard input.
func newInput(operand string) input {
	return input{name: operand, stdin: operand == "-"}
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
}

// writeError marks an error as coming from writing to the standard output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
