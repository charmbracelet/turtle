// Package csplit implements the POSIX csplit utility.
//
// It writes sections of a text file into numbered output files, as directed
// by line number and basic regular expression operands, and prints the byte
// count of each file created.
package csplit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/bre"
	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/fsx"
)

// Prog is the csplit core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run splits the input file according to the operands, printing the size of
// each file created, unless -s is given. By default, files created are
// removed when an error occurs; -k keeps them.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 0 {
		p.errorf("missing operand")
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 1 {
		p.errorf("missing operand after '%s'", operands[0])
		return &coreutils.ExitError{Code: 1}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	controls, err := parseControls(operands[1:])
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if len(baseName(opts.prefix))+max(opts.digits, 10) > nameMax {
		p.errorf("'%s': file name too long", opts.prefix)
		return &coreutils.ExitError{Code: 1}
	}

	r, err := p.openInput(ctx, operands[0])
	if err != nil {
		p.errorf("%s: %v", operands[0], unwrapPathError(err))
		return &coreutils.ExitError{Code: 1}
	}
	lines, err := readLines(ctx, r)
	r.Close()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("%s: %v", operands[0], unwrapPathError(err))
		return &coreutils.ExitError{Code: 1}
	}

	s := &splitter{p: p, opts: opts, lines: lines, current: 1}
	if err := s.run(ctx, controls); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// nameMax is the assumed NAME_MAX limit of the output directory.
const nameMax = 255

// options holds the parsed command line options.
type options struct {
	keep   bool
	silent bool
	prefix string
	digits int
}

// control is a parsed operand: a line number, a regular expression to
// search for, or one to skip to without creating a file.
type control struct {
	arg    string // the operand text, for diagnostics
	re     *regexp.Regexp
	line   int64
	offset int64
	ignore bool
	repeat int64
}

// splitter holds the state of a run in progress.
type splitter struct {
	p       *Prog
	opts    options
	lines   [][]byte
	current int64 // the current line, 1-based
	created []string
	out     fsx.File
	outName string
	written int64
	failed  bool // whether writing to out failed
	regexp  bool // whether a regular expression was already applied
}

// run processes each control and then the rest of the input.
func (s *splitter) run(ctx context.Context, controls []control) error {
	for _, c := range controls {
		for rep := int64(0); rep <= c.repeat; rep++ {
			var err error
			if c.re != nil {
				err = s.processRegexp(ctx, c, rep)
			} else {
				err = s.processLineNumber(ctx, c, rep)
			}
			if err != nil {
				return s.fail(err)
			}
		}
	}
	if err := s.createFile(); err != nil {
		return s.fail(err)
	}
	if err := s.writeTo(ctx, int64(len(s.lines))+1); err != nil {
		return s.fail(err)
	}
	return s.closeFile()
}

// processLineNumber handles a line number operand: it creates a file with
// the lines from the current line up to, but not including, the target.
// When the target is past the end of the file, the rest of the input is
// written before the error is reported.
func (s *splitter) processLineNumber(ctx context.Context, c control, rep int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if rep > 0 && c.line > math.MaxInt64/(rep+1) {
		return &controlError{arg: strconv.FormatInt(c.line, 10), rep: rep}
	}
	target := c.line * (rep + 1)
	if target < s.current {
		return &controlError{arg: strconv.FormatInt(c.line, 10), rep: rep}
	}
	if err := s.createFile(); err != nil {
		return err
	}
	last := int64(len(s.lines))
	if target > last {
		if err := s.writeTo(ctx, last+1); err != nil {
			return err
		}
		return &controlError{arg: strconv.FormatInt(c.line, 10), rep: rep}
	}
	if err := s.writeTo(ctx, target); err != nil {
		return err
	}
	return s.closeFile()
}

// processRegexp handles the /rexp/ and %rexp% operands: it searches for a
// line matching the expression, applies the offset, and creates a file with
// the lines before the resulting line.
func (s *splitter) processRegexp(ctx context.Context, c control, rep int64) error {
	start := s.current + 1
	if s.current == 1 && !s.regexp {
		start = 1
	}
	s.regexp = true
	found := int64(-1)
	for line := start; line <= int64(len(s.lines)); line++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.re.Match(trimNewline(s.lines[line-1])) {
			found = line
			break
		}
	}
	if found < 0 {
		if !c.ignore {
			if err := s.createFile(); err != nil {
				return err
			}
			if err := s.writeTo(ctx, int64(len(s.lines))+1); err != nil {
				return err
			}
			if err := s.closeFile(); err != nil {
				return err
			}
		}
		return &matchError{arg: c.arg, rep: rep}
	}
	boundary := found + c.offset
	if boundary < s.current {
		return &controlError{arg: c.arg, rep: rep}
	}
	last := int64(len(s.lines))
	if !c.ignore {
		if err := s.createFile(); err != nil {
			return err
		}
		if err := s.writeTo(ctx, min(boundary, last+1)); err != nil {
			return err
		}
		if err := s.closeFile(); err != nil {
			return err
		}
	}
	if boundary > last+1 {
		return &controlError{arg: c.arg, rep: rep}
	}
	s.current = boundary
	return nil
}

// writeTo writes the lines from the current line up to, but not including,
// the target line to the current output file.
func (s *splitter) writeTo(ctx context.Context, target int64) error {
	for s.current < target {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := s.lines[s.current-1]
		if _, err := s.out.Write(line); err != nil {
			s.failed = true
			return &writeError{name: s.outName, err: err}
		}
		s.written += int64(len(line))
		s.current++
	}
	return nil
}

// createFile opens the next output file.
func (s *splitter) createFile() error {
	name := s.opts.prefix + fmt.Sprintf("%0*d", s.opts.digits, len(s.created))
	f, err := s.p.FS.OpenFile(filepath.ToSlash(s.p.ResolvePath(name)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^s.p.Umask.Perm())
	if err != nil {
		return &writeError{name: name, err: err}
	}
	s.out = f
	s.outName = name
	s.failed = false
	s.created = append(s.created, name)
	return nil
}

// closeFile closes the current output file, printing its size.
func (s *splitter) closeFile() error {
	if s.out == nil {
		return nil
	}
	err := s.out.Close()
	s.out = nil
	if err != nil {
		return &writeError{name: s.outName, err: err}
	}
	if !s.opts.silent && !s.failed {
		fmt.Fprintf(s.p.Stdout, "%d\n", s.written)
	}
	s.written = 0
	return nil
}

// fail reports err, closes the current file, removes the files created
// unless -k was given, and returns err.
func (s *splitter) fail(err error) error {
	if writeErr, ok := errors.AsType[*writeError](err); ok {
		s.p.errorf("%s: %v", writeErr.name, unwrapPathError(writeErr.err))
	} else {
		s.p.errorf("%v", err)
	}
	s.closeFile()
	if !s.opts.keep {
		for _, name := range s.created {
			s.p.FS.Remove(filepath.ToSlash(s.p.ResolvePath(name)))
		}
	}
	return err
}

// controlError reports a line number operand that doesn't reference a line
// between the current position and the end of the file.
type controlError struct {
	arg string
	rep int64
}

// Error implements the error interface.
func (e *controlError) Error() string {
	if e.rep > 0 {
		return fmt.Sprintf("'%s': line number out of range on repetition %d", e.arg, e.rep)
	}
	return fmt.Sprintf("'%s': line number out of range", e.arg)
}

// matchError reports a regular expression that matched no line.
type matchError struct {
	arg string
	rep int64
}

// Error implements the error interface.
func (e *matchError) Error() string {
	if e.rep > 0 {
		return fmt.Sprintf("'%s': match not found on repetition %d", e.arg, e.rep)
	}
	return fmt.Sprintf("'%s': match not found", e.arg)
}

// readLines reads the whole input as lines, keeping the newline of each.
// The last line may not have one.
func readLines(ctx context.Context, r io.Reader) ([][]byte, error) {
	br := bufio.NewReader(r)
	var lines [][]byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lines = append(lines, line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	return lines, nil
}

// trimNewline drops a trailing newline.
func trimNewline(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		return line[:len(line)-1]
	}
	return line
}

// openInput returns a reader for the given operand. Reads stop early when
// ctx is done. The standard input isn't owned by the caller, so it's wrapped
// on a no-op closer.
func (p *Prog) openInput(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == "-" {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxio.NewReader(ctx, p.Stdin), nil
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(name)))
	if err != nil {
		return nil, err
	}
	return ctxio.ReadCloser{Ctx: ctx, RC: f}, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "csplit: "+format+"\n", args...)
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{prefix: "xx", digits: 2}
	operands, err := (getopt.Set{Valued: "fn"}).Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'k':
			opts.keep = true
		case 's':
			opts.silent = true
		case 'f':
			opts.prefix = value
		case 'n':
			n, err := strconv.ParseInt(value, 10, 0)
			if err != nil || n < 0 || !isDecimal(value) {
				return fmt.Errorf("invalid number: '%s'", value)
			}
			opts.digits = int(n)
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// parseControls parses the operand arguments, each a line number, a
// /rexp/[offset], a %rexp%[offset], or a {num} repeat count applying to the
// previous operand.
func parseControls(args []string) ([]control, error) {
	var controls []control
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var c control
		switch {
		case strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "%"):
			var err error
			c, err = parseRegexp(arg)
			if err != nil {
				return nil, err
			}
		case strings.HasPrefix(arg, "{"):
			return nil, fmt.Errorf("'%s': invalid pattern", arg)
		default:
			n, err := strconv.ParseInt(arg, 10, 64)
			if err != nil || !isDecimal(arg) {
				return nil, fmt.Errorf("'%s': invalid pattern", arg)
			}
			if n == 0 {
				return nil, fmt.Errorf("%s: line number must be greater than zero", arg)
			}
			c = control{arg: arg, line: n}
		}
		if i+1 < len(args) && strings.HasPrefix(args[i+1], "{") {
			repeat, err := parseRepeat(args[i+1])
			if err != nil {
				return nil, err
			}
			c.repeat = repeat
			i++
		}
		controls = append(controls, c)
	}
	return controls, nil
}

// parseRegexp parses a /rexp/[offset] or %rexp%[offset] operand. The
// closing delimiter is its last occurrence in the operand, so the escaped
// delimiter sequences "\/" and "\%" work inside the expression.
func parseRegexp(arg string) (control, error) {
	delim := arg[0]
	closing := strings.LastIndexByte(arg[1:], delim)
	if closing < 0 {
		return control{}, fmt.Errorf("%s: closing delimiter '%c' missing", arg, delim)
	}
	closing++
	c := control{arg: arg, ignore: delim == '%'}
	re, err := bre.Compile(arg[1:closing])
	if err != nil {
		return control{}, fmt.Errorf("'%s': %v", arg, err)
	}
	c.re = re
	if rest := arg[closing+1:]; rest != "" {
		offset, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return control{}, fmt.Errorf("'%s': integer expected after delimiter", arg)
		}
		c.offset = offset
	}
	return c, nil
}

// parseRepeat parses a {num} repeat count.
func parseRepeat(arg string) (int64, error) {
	if !strings.HasSuffix(arg, "}") {
		return 0, fmt.Errorf("'%s': '}' is required in repeat count", arg)
	}
	n, err := strconv.ParseInt(arg[1:len(arg)-1], 10, 64)
	if err != nil || !isDecimal(arg[1:len(arg)-1]) {
		return 0, fmt.Errorf("'%s': integer required between '{' and '}'", arg)
	}
	return n, nil
}

// isDecimal reports whether s holds only decimal digits.
func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// baseName returns the last path element of prefix.
func baseName(prefix string) string {
	return path.Base(filepath.ToSlash(prefix))
}

// writeError marks an error as coming from writing to an output file.
type writeError struct {
	name string
	err  error
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

var _ coreutils.Program = (*Prog)(nil)
