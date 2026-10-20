// Package tar implements the tar archive utility.
//
// It creates, lists, extracts, appends to and updates archives through the
// archive/tar formats: ustar when it can hold the entry, pax otherwise, with
// pax and GNU extensions understood on input. The -c, -r, -t, -u and -x
// operations and the -b, -e, -f, -h, -m, -o, -p, -v and -w options follow
// the GNU tar interface, including the old option style without dashes, as
// in "tar cf a.tar dir".
//
// Some GNU behaviors are out of scope: operands of -t and -x are literal
// names or directory prefixes, not glob patterns; member ownership is never
// changed, making -o a no-op; -e only ignores files which can't be read
// while an archive is created; and with no -f the archive is the standard
// input or output, taken from the TAPE environment variable when set.
package tar

import (
	"archive/tar"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the tar core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

var _ coreutils.NewFunc = New

// defaultBlocking is the number of 512-byte blocks written per record when
// -b isn't given.
const defaultBlocking = 20

// mode is the operation of the utility, from its function letter.
type mode rune

// The operations of the utility.
const (
	modeCreate  mode = 'c'
	modeAppend  mode = 'r'
	modeList    mode = 't'
	modeUpdate  mode = 'u'
	modeExtract mode = 'x'
)

// options holds the parsed flags of a run.
type options struct {
	mode         mode
	blocking     int
	archive      string
	archiveSet   bool
	ignoreErrors bool
	dereference  bool
	touch        bool
	preserve     bool
	verbose      bool
	interactive  bool
}

// Run operates on the archive following the options given in args.
//
// It returns a [*coreutils.ExitError] holding 2 for usage errors and 1 when
// an operation fails, and the context error when ctx is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	a := &archiver{rt: p.Runtime}
	opts, operands, err := parseFlags(args)
	if err != nil {
		a.errorf("%v", err)
		return &coreutils.ExitError{Code: 2, Err: err}
	}
	a.opts = opts
	if err := ctx.Err(); err != nil {
		return err
	}
	switch opts.mode {
	case modeCreate:
		err = a.create(ctx, operands)
	case modeAppend:
		err = a.append(ctx, operands, false)
	case modeUpdate:
		err = a.append(ctx, operands, true)
	case modeList:
		err = a.list(ctx, operands)
	case modeExtract:
		err = a.extract(ctx, operands)
	}
	return err
}

// longOptions maps the long options of tar to the short ones they stand for.
var longOptions = map[string]rune{
	"append":               'r',
	"blocking-factor":      'b',
	"confirmation":         'w',
	"create":               'c',
	"dereference":          'h',
	"extract":              'x',
	"file":                 'f',
	"get":                  'x',
	"ignore-errors":        'e',
	"interactive":          'w',
	"list":                 't',
	"no-same-owner":        'o',
	"old-archive":          'o',
	"preserve-permissions": 'p',
	"same-permissions":     'p',
	"touch":                'm',
	"update":               'u',
	"verbose":              'v',
}

// parseFlags parses the options and operands of a run, expanding a leading
// old-style key, like "cvf", into the dashed options it stands for.
func parseFlags(args []string) (options, []string, error) {
	opts := options{blocking: defaultBlocking}
	operands, err := getopt.Set{Valued: "bf", Long: longOptions}.Parse(expandOldStyle(args), func(opt rune, value string) error {
		switch opt {
		case 'c', 'r', 't', 'u', 'x':
			if opts.mode != 0 {
				return errors.New("You may not specify more than one '-ctrux' option")
			}
			opts.mode = mode(opt)
		case 'b':
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid blocking factor %q", value)
			}
			opts.blocking = n
		case 'f':
			opts.archive = value
			opts.archiveSet = true
		case 'e':
			// Like GNU's --ignore-failed-read: unreadable files don't fail
			// an archive creation.
			opts.ignoreErrors = true
		case 'h':
			opts.dereference = true
		case 'm':
			opts.touch = true
		case 'o': // accepted, no effect: ownership is never changed
		case 'p':
			opts.preserve = true
		case 'v':
			opts.verbose = true
		case 'w':
			opts.interactive = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if opts.mode == 0 {
		return options{}, nil, errors.New("You must specify one of the '-ctrux' options")
	}
	return opts, operands, nil
}

// expandOldStyle rewrites a leading old-style key, like "cvf", into dashed
// options. The letters b and f take their values from the arguments which
// follow the key, in the order the letters appear in it; without such an
// argument the letter is dropped, keeping the default value.
func expandOldStyle(args []string) []string {
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' || !strings.ContainsRune("crtux", rune(args[0][0])) {
		return args
	}
	var expanded []string
	rest := args[1:]
	for _, letter := range args[0] {
		opt := "-" + string(letter)
		switch letter {
		case 'b', 'f':
			if len(rest) == 0 {
				continue
			}
			expanded = append(expanded, opt, rest[0])
			rest = rest[1:]
		default:
			expanded = append(expanded, opt)
		}
	}
	return append(expanded, rest...)
}

// archiver holds the state shared by the operations of a single run.
type archiver struct {
	rt   *coreutils.Runtime
	opts options

	// failed marks an operation which couldn't complete, deciding the exit
	// status; notedSlash tells whether the note about stripped leading
	// slashes was already written.
	failed     bool
	notedSlash bool
}

// archiveName returns the archive to operate on: the -f argument, the TAPE
// environment variable, or the standard input or output.
func (a *archiver) archiveName() string {
	if a.opts.archiveSet {
		return a.opts.archive
	}
	if tape := a.rt.Getenv("TAPE"); tape != "" {
		return tape
	}
	return "-"
}

// openReader returns the archive to read.
func (a *archiver) openReader() (io.ReadCloser, error) {
	name := a.archiveName()
	if name == "-" {
		if a.rt.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return io.NopCloser(a.rt.Stdin), nil
	}
	if a.rt.FS == nil {
		return nil, errors.New("no file system available")
	}
	return a.rt.FS.Open(a.resolve(name))
}

// openWriter returns the archive to write, truncating an existing one.
func (a *archiver) openWriter() (io.WriteCloser, error) {
	name := a.archiveName()
	if name == "-" {
		if a.rt.Stdout == nil {
			return nil, errors.New("no standard output available")
		}
		return nopCloser{a.rt.Stdout}, nil
	}
	if a.rt.FS == nil {
		return nil, errors.New("no file system available")
	}
	return a.rt.FS.OpenFile(a.resolve(name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^a.rt.Umask.Perm())
}

// nopCloser adapts a writer to io.WriteCloser without closing it.
type nopCloser struct {
	io.Writer
}

// Close implements io.Closer.
func (nopCloser) Close() error { return nil }

// resolve converts a name into the slash-separated path used with the file
// system, relative to the working directory.
func (a *archiver) resolve(name string) string {
	return path.Clean(filepath.ToSlash(a.rt.ResolvePath(name)))
}

// errorf writes a diagnostic message to standard error.
func (a *archiver) errorf(format string, args ...any) {
	fmt.Fprintf(a.rt.Stderr, "tar: "+format+"\n", args...)
}

// fail reports the error of an operation on name, marking the run as failed.
func (a *archiver) fail(name string, err error) {
	if name == "" {
		a.errorf("%v", err)
	} else {
		a.errorf("%s: %v", name, unwrapPathError(err))
	}
	a.failed = true
}

// exit returns a [*coreutils.ExitError] with code when the run failed.
func (a *archiver) exit(code int) error {
	if a.failed {
		return &coreutils.ExitError{Code: code}
	}
	return nil
}

// errNotTar is reported when an input isn't a tar archive: shorter than a
// block, or holding an invalid header.
var errNotTar = errors.New("This does not look like a tar archive")

// newTarReader starts reading an archive, rejecting inputs shorter than one
// block up front.
func newTarReader(r io.Reader) (*tar.Reader, error) {
	br := bufio.NewReader(r)
	block, err := br.Peek(blockSize)
	if err != nil && len(block) < blockSize {
		return nil, errNotTar
	}
	return tar.NewReader(br), nil
}

// confirm asks on the standard error whether the action should happen,
// reading the answer from the standard input. Only a response starting with
// 'y' or 'Y' is taken as affirmative.
func (a *archiver) confirm(action, name string) bool {
	fmt.Fprintf(a.rt.Stderr, "tar: %s '%s'? ", action, name)
	if a.rt.Stdin == nil {
		return false
	}
	var (
		buf    [1]byte
		answer byte
	)
	for {
		n, err := a.rt.Stdin.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if answer == 0 {
				answer = buf[0]
			}
		}
		if err != nil {
			break
		}
	}
	return answer == 'y' || answer == 'Y'
}

// cleanName normalizes an operand for storage or matching: slashes become
// forward slashes, and trailing slashes are dropped.
func cleanName(name string) string {
	name = filepath.ToSlash(name)
	for len(name) > 1 && strings.HasSuffix(name, "/") {
		name = strings.TrimSuffix(name, "/")
	}
	return name
}

// relativeName returns the name stored in the archive for an operand,
// stripping leading slashes, with a one-time note about it. Drive letters,
// which make a name absolute on Windows, are stripped the same way; on other
// systems VolumeName is always empty.
func (a *archiver) relativeName(name string) string {
	stripped := strings.TrimLeft(name, "/")
	if vol := filepath.VolumeName(stripped); vol != "" {
		stripped = strings.TrimLeft(stripped[len(vol):], "/")
	}
	if stripped != name && !a.notedSlash {
		a.errorf("Removing leading '/' from member names")
		a.notedSlash = true
	}
	return stripped
}

// nameMatches reports whether an entry name is selected by the operands: it
// is when no operand is given, when it equals one of them, or when it sits
// inside one of them.
func nameMatches(operands []string, name string) bool {
	if len(operands) == 0 {
		return true
	}
	name = strings.TrimSuffix(name, "/")
	for _, operand := range operands {
		op := cleanName(operand)
		if name == op || strings.HasPrefix(name, op+"/") {
			return true
		}
	}
	return false
}

// reportMissing diagnoses the operands which matched no entry.
func (a *archiver) reportMissing(operands []string, matched map[string]bool) {
	for _, operand := range operands {
		if !matched[cleanName(operand)] {
			a.fail(operand, errors.New("Not found in archive"))
		}
	}
}

// matchTracker marks the operands which match an entry name.
func matchTracker(operands []string) map[string]bool {
	matched := make(map[string]bool, len(operands))
	for _, operand := range operands {
		matched[cleanName(operand)] = false
	}
	return matched
}

// markMatched flags the operands matching an entry name.
func markMatched(matched map[string]bool, name string) {
	name = strings.TrimSuffix(name, "/")
	for op := range matched {
		if name == op || strings.HasPrefix(name, op+"/") {
			matched[op] = true
		}
	}
}

// ctxReader stops read operations with the context error once ctx is done.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// Read implements io.Reader.
func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// countWriter counts the bytes written through it.
type countWriter struct {
	w io.Writer
	n int64
}

// Write implements io.Writer.
func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}
