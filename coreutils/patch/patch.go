// Package patch implements the POSIX patch utility.
//
// It reads a patch file holding unified or context diffs and applies them
// to the files they name, or to the file operand, searching around the
// stated places and giving up context lines until the hunks fit.
package patch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/diff"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the patch core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run applies the patches of the patch file to their files, returning an
// exit status of 1 when hunks fail or get ignored, and 2 when any other
// error happens.
//
// Reading is interrupted when ctx is done, and the context error is
// returned as-is.
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
	operand := ""
	if len(operands) == 1 {
		operand = filepath.ToSlash(operands[0])
	}

	data, err := p.readInput(ctx, opts)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return p.failRead(err, "the patch input")
	}

	format := detectFormat(string(data), opts)
	if format == diff.UnknownPatch {
		p.errorf("only garbage was found in the patch input")
		return &coreutils.ExitError{Code: 2}
	}
	patches, err := parsePatches(string(data), format)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(patches) == 0 {
		p.errorf("only garbage was found in the patch input")
		return &coreutils.ExitError{Code: 2}
	}

	r := &runner{p: p, opts: opts, format: format, backed: map[string]bool{}, intermediate: map[string][]string{}}
	status := false
	for _, pr := range patches {
		if opts.reverse {
			pr = diff.Reverse(pr)
		}
		failed, err := r.patchOne(ctx, pr, operand)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if pathErr, ok := errors.AsType[*pathError](err); ok {
				p.errorf("%s: %v", pathErr.label, unwrapPathError(pathErr.err))
				return &coreutils.ExitError{Code: 2, Err: pathErr.err}
			}
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 2}
		}
		status = status || failed
	}
	if status {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// failRead wraps the error of reading an input file, reporting it under the
// given label.
func (p *Prog) failRead(err error, label string) error {
	if pathErr, ok := errors.AsType[*pathError](err); ok {
		p.errorf("%s: %v", pathErr.label, unwrapPathError(pathErr.err))
		return &coreutils.ExitError{Code: 2, Err: pathErr.err}
	}
	p.errorf("%v", err)
	return &coreutils.ExitError{Code: 2}
}

// readInput reads the patch data, from the file of the -i option or from
// the standard input. Reads stop early when ctx is done.
func (p *Prog) readInput(ctx context.Context, opts options) ([]byte, error) {
	var r io.Reader
	switch {
	case opts.input != "":
		if p.FS == nil {
			return nil, errNoFS
		}
		f, err := p.FS.Open(p.resolvePath(opts.input, opts))
		if err != nil {
			return nil, &pathError{label: opts.input, err: err}
		}
		defer f.Close()
		r = f
	case p.Stdin == nil:
		return nil, errors.New("no standard input available")
	default:
		r = p.Stdin
	}
	return readAll(ctx, r)
}

// detectFormat picks the patch format, as the -c or -u options force, or as
// the data looks.
func detectFormat(data string, opts options) diff.PatchFormat {
	switch {
	case opts.context:
		return diff.ContextPatch
	case opts.unified:
		return diff.UnifiedPatch
	default:
		return diff.DetectPatch(data)
	}
}

// parsePatches parses the patch data in the given format.
func parsePatches(data string, format diff.PatchFormat) ([]diff.Patch, error) {
	if format == diff.ContextPatch {
		return diff.ParseContext(data)
	}
	return diff.ParseUnified(data)
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "patch: "+format+"\n", args...)
}

// messagef writes an informative message to standard error.
func (p *Prog) messagef(format string, args ...any) {
	fmt.Fprintf(p.Stderr, format, args...)
}

// ask writes the prompt to standard error and reads one answer line from
// the standard input.
func (p *Prog) ask(prompt string) string {
	fmt.Fprint(p.Stderr, prompt)
	if p.Stdin == nil {
		return ""
	}
	r := bufio.NewReader(p.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSpace(line)
}

// yes reports whether an answer line accepts.
func yes(answer string) bool {
	return strings.HasPrefix(strings.ToLower(answer), "y")
}

// errNoFS marks the lack of a file system to reach paths through.
var errNoFS = errors.New("no file system available")

// options holds the parsed command line options.
type options struct {
	backup  bool   // -b
	context bool   // -c
	dir     string // -d
	input   string // -i
	blank   bool   // -l
	ignore  bool   // -N
	output  string // -o
	strip   int    // -p, negative for the basename default
	reject  string // -r
	reverse bool   // -R
	unified bool   // -u
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{strip: -1}
	operands, err := getopt.Set{Valued: "diopr"}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.backup = true
		case 'c':
			opts.context = true
		case 'd':
			opts.dir = value
		case 'i':
			opts.input = value
		case 'l':
			opts.blank = true
		case 'N':
			opts.ignore = true
		case 'o':
			opts.output = value
		case 'p':
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return fmt.Errorf("invalid -p argument '%s'", value)
			}
			opts.strip = n
		case 'r':
			opts.reject = value
		case 'R':
			opts.reverse = true
		case 'u':
			opts.unified = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if opts.context && opts.unified {
		return options{}, nil, errors.New("options -c and -u are incompatible")
	}
	return opts, operands, nil
}

// readAll reads all of r, stopping with the context error once ctx is done,
// so long inputs don't delay returning.
func readAll(ctx context.Context, r io.Reader) ([]byte, error) {
	var out []byte
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// pathError marks an error as coming from reaching a file.
type pathError struct {
	label string
	err   error
}

// Error implements the error interface.
func (e *pathError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make pathError unwrapable.
func (e *pathError) Unwrap() error {
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

// resolvePath resolves a patch file name to a file system path, under the
// directory of the -d option when it has one.
func (p *Prog) resolvePath(name string, opts options) string {
	name = filepath.ToSlash(name)
	if opts.dir != "" && !path.IsAbs(name) {
		name = path.Join(filepath.ToSlash(p.ResolvePath(opts.dir)), name)
	}
	return filepath.ToSlash(p.ResolvePath(name))
}

// stripName removes n leading components of a patch file path name, keeping
// only the basename when n is negative.
func stripName(name string, n int) string {
	var comps []string
	for c := range strings.SplitSeq(name, "/") {
		if c != "" {
			comps = append(comps, c)
		}
	}
	switch {
	case n < 0:
		if len(comps) == 0 {
			return ""
		}
		return comps[len(comps)-1]
	case path.IsAbs(name):
		if n == 0 {
			return name
		}
		n-- // the leading slashes count as the first component
	}
	if n > len(comps) {
		return ""
	}
	return strings.Join(comps[n:], "/")
}

var _ coreutils.Program = (*Prog)(nil)
