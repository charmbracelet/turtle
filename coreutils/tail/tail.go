// Package tail implements the POSIX tail utility.
//
// It copies the last part of each given file to the standard output, starting
// at the point given by the -n or -c options, defaulting to the last 10
// lines. When more than one operand is given, a "==> name <==" header
// precedes the output of each file; -q and -v make the headers never or
// always printed. The standard input is used when no operand is given, and
// also when the operand is "-". With -f, the input is monitored for further
// data after the designated part is copied. The obsolete first-operand forms
// tail -NUM and tail +NUM, with an optional b, c or l size letter and a
// trailing f, are accepted too. The --follow=name, -F, --retry, --pid and -s
// options of the GNU manual are not implemented, and -f follows a single
// file operand only.
package tail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/sizeparse"
	"charm.land/snail/coreutils/internal/stdopts"
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

// Run writes the designated part of the given files to the standard output,
// in the same order the operands are given. The standard input is used when
// no operands are given, and also when an operand is "-". With -f, the input
// keeps being copied as further data becomes available, and Run only returns
// once ctx is done. A failing file doesn't stop the others.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "tail", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "tail")
		return &coreutils.ExitError{Code: 2}
	}
	if opts.follow && len(operands) > 1 {
		p.errorf("cannot follow more than one file")
		stdopts.TryHelp(p.Runtime, "tail")
		return &coreutils.ExitError{Code: 2}
	}

	inputs := input.List(operands)
	headers := opts.headers == headerAlways ||
		(opts.headers == headerAuto && len(inputs) > 1)

	ok := true
	first := true
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		rc, f, err := p.open(in)
		if err != nil {
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			ok = false
			continue
		}
		if headers {
			p.writeHeader(in, !first)
			first = false
		}
		err = p.tail(ctx, rc, f, opts)
		rc.Close()
		if err == nil {
			continue
		}
		var writeErr *writeError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// tail copies the designated part of the open input and follows it with -f.
func (p *Prog) tail(ctx context.Context, rc io.ReadCloser, f fsx.SeekFile, opts options) error {
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
	delim := opts.delim()
	switch {
	case opts.reverse:
		return p.reverse(r, opts)
	case opts.lines && opts.fromStart:
		return p.copyFromLine(r, opts.count, delim)
	case opts.lines:
		return p.copyLastLines(r, f, opts.count, delim)
	case opts.fromStart:
		return p.copyFromByte(r, f, opts.count)
	default:
		return p.copyLastBytes(r, f, opts.count)
	}
}

// writeHeader writes the "==> name <==" header which precedes the output of
// each file. The first one has no leading <newline>; the others are
// separated from the output of the previous file by a blank line.
func (p *Prog) writeHeader(in input.Input, leadingNewline bool) {
	if leadingNewline {
		fmt.Fprint(p.Stdout, "\n")
	}
	name := in.Label()
	if in.Stdin {
		name = "standard input"
	}
	fmt.Fprintf(p.Stdout, "==> %s <==\n", name)
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
func (p *Prog) copyLastLines(r io.Reader, f fsx.SeekFile, count int, delim byte) error {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	if f != nil {
		if err := seekLastLines(f, count, delim); err != nil {
			return err
		}
		return p.copyAll(r)
	}
	lines, err := readLastLines(r, count, delim)
	if err != nil {
		return err
	}
	return p.writeLines(lines)
}

// copyFromLine copies everything from the count'th line on. The origin is 1.
func (p *Prog) copyFromLine(r io.Reader, count int, delim byte) error {
	br := bufio.NewReader(r)
	for i := 0; i < count-1; i++ {
		if _, err := br.ReadBytes(delim); err != nil {
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
		lines, err = readLastLines(r, opts.count, opts.delim())
	} else {
		lines, err = readAllLines(r, opts.delim())
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
func (p *Prog) open(in input.Input) (io.ReadCloser, fsx.SeekFile, error) {
	if in.Stdin {
		if p.Stdin == nil {
			return nil, nil, errors.New("no standard input available")
		}
		return io.NopCloser(p.Stdin), nil, nil
	}
	if p.FS == nil {
		return nil, nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.Name)))
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

// parseFlags parses the options and operands. The obsolete first-operand
// forms are recognized first, when they don't conflict with the standard
// syntax. Then the -c and -n options take a decimal number, optionally
// signed and with a multiplicative suffix: a '+' makes copying start
// relative to the beginning of the input instead of the end. The last of the
// two given wins. Option parsing stops at the first operand or at "--".
// longOptions maps the long options of tail to the short ones they stand
// for. The -r option has no long form in GNU.
var longOptions = map[string]rune{
	"bytes":           'c',
	"follow":          'f',
	"lines":           'n',
	"quiet":           'q',
	"silent":          'q',
	"verbose":         'v',
	"zero-terminated": 'z',
}

// optSet describes the options of tail.
var optSet = getopt.Set{Valued: "cn", Long: longOptions}

// usage describes the options of tail.
const usage = `Usage: tail [OPTION]... [FILE]...
Print the last 10 lines of each FILE to standard output.
With more than one FILE, precede each with a header giving the file name.

With no FILE, or when FILE is -, read standard input.

  -c, --bytes=[+]NUM     output the last NUM bytes; or use -c +NUM to output
                         starting with byte NUM of each file
  -f, --follow           output appended data as the file grows; a single
                         file operand only
  -n, --lines=[+]NUM     output the last NUM lines, instead of the last 10;
                         or use -n +NUM to output starting with line NUM
  -q, --quiet, --silent  never print headers giving file names
  -r                     output the lines in reverse, starting from the end
  -v, --verbose          always print headers giving file names
  -z, --zero-terminated  line delimiter is NUL, not newline
      --help     display this help and exit
      --version  output version information and exit
`

func parseFlags(args []string) (options, []string, error) {
	var opts options
	if len(args) > 0 {
		if obsolete, rest, ok := parseObsolete(args); ok {
			opts, args = obsolete, rest
		}
	}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
			opts.follow = true
		case 'r':
			opts.reverse = true
		case 'q':
			opts.headers = headerNever
		case 'v':
			opts.headers = headerAlways
		case 'z':
			opts.zero = true
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
			// A digit can only be left when the obsolete form didn't
			// apply, like on "tail -n 2 -5".
			if isDigit(byte(opt)) {
				return fmt.Errorf("option used in invalid context -- '%c'", opt)
			}
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
// integer, optionally signed and optionally followed by a multiplicative
// suffix, as "1K" or "5KB". A '+' sign makes copying start relative to the
// beginning of the input and is required to be non-zero. A '-' sign counts
// from the end, like no sign at all.
func parseCount(value string) (count int, fromStart bool, err error) {
	n, _, err := sizeparse.ParseNumber(value)
	if err != nil || n > math.MaxInt {
		return 0, false, fmt.Errorf("invalid number: '%s'", value)
	}
	fromStart = strings.HasPrefix(value, "+")
	if fromStart && n == 0 {
		return 0, false, fmt.Errorf("invalid number: '%s'", value)
	}
	return int(n), fromStart, nil
}

// parseObsolete parses the obsolete first-operand form of the GNU manual,
// tail -[NUM][bcl][f] or tail +[NUM][bcl][f]: NUM counts lines, or 512-byte
// blocks with 'b', bytes with 'c', and a trailing 'f' has the meaning of -f.
// A leading '+' counts from the beginning of the input. The form is
// recognized only when it is the single option and at most one file operand
// follows it, and only when it doesn't conflict with the standard syntax. It
// returns the options it stands for with the arguments left after it, and
// ok=false when the obsolete form doesn't apply.
func parseObsolete(args []string) (options, []string, bool) {
	if !obsoleteShape(args) {
		return options{}, nil, false
	}

	arg := args[0]
	if len(arg) < 2 {
		return options{}, nil, false
	}
	rest := arg[1:]
	opts := options{lines: true}
	switch arg[0] {
	case '+':
		opts.fromStart = true
	case '-':
		// "-c" is missing its argument.
		if rest == "c" {
			return options{}, nil, false
		}
	default:
		return options{}, nil, false
	}

	// The count defaults to the default number of lines.
	n, i := 10, 0
	if i < len(rest) && isDigit(rest[i]) {
		n = 0
		for ; i < len(rest) && isDigit(rest[i]); i++ {
			if d := int(rest[i] - '0'); n <= (math.MaxInt-d)/10 {
				n = n*10 + d
			} else {
				n = math.MaxInt
			}
		}
	}
	switch {
	case i < len(rest) && rest[i] == 'b':
		if n > math.MaxInt/512 {
			n = math.MaxInt
		} else {
			n *= 512
		}
		opts.lines = false
		i++
	case i < len(rest) && (rest[i] == 'c' || rest[i] == 'l'):
		opts.lines = rest[i] == 'l'
		i++
	}
	if i < len(rest) && rest[i] == 'f' {
		opts.follow = true
		i++
	}
	if i < len(rest) {
		return options{}, nil, false
	}

	opts.count = n
	opts.countSet = true
	return opts, args[1:], true
}

// obsoleteShape reports whether args can hold the obsolete form: the
// obsolete option alone, or followed by a file operand, by "--", or by "--"
// and a file operand.
func obsoleteShape(args []string) bool {
	switch {
	case len(args) == 1:
		return true
	case len(args) == 2:
		return args[1] == "--" || !(len(args[1]) > 1 && args[1][0] == '-')
	case len(args) == 3:
		return args[1] == "--"
	}
	return false
}

func isDigit(b byte) bool {
	return '0' <= b && b <= '9'
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
func seekLastLines(f fsx.SeekFile, count int, delim byte) error {
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
		if start+int64(len(b)) == size && b[i] == delim {
			i--
		}
		for ; i >= 0; i-- {
			if b[i] != delim {
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
func readLastLines(r io.Reader, count int, delim byte) ([][]byte, error) {
	if count <= 0 {
		_, err := io.Copy(io.Discard, r)
		return nil, err
	}
	var lines [][]byte
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes(delim)
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
func readAllLines(r io.Reader, delim byte) ([][]byte, error) {
	var lines [][]byte
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes(delim)
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
	headers   headerMode // -q and -v: when the file headers are printed
	zero      bool       // -z: items end with NUL instead of <newline>
}

// headerMode tells when the file headers are printed.
type headerMode int

const (
	headerAuto   headerMode = iota // only with more than one operand
	headerNever                    // -q: never
	headerAlways                   // -v: always
)

// delim returns the byte which ends an item of the input.
func (opts options) delim() byte {
	if opts.zero {
		return 0
	}
	return '\n'
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

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
