// Package xargs implements the POSIX xargs utility.
//
// It reads arguments from the standard input, delimited by unquoted blanks,
// newlines or null bytes, and invokes the utility operand with them, in
// groups that fit the count and length limits given by the options.
package xargs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// maxLen is the largest command line length supported. It's the default
// limit, and the cap of the -s option, which POSIX only requires to support
// values up to {LINE_MAX}.
const maxLen = 1 << 20

// lineMax is the command line length used when -n is given without -s, as
// the -n option requires.
const lineMax = 2048

// checkEvery is how often the input parsing loops check the context, so a
// huge single record doesn't delay returning.
const checkEvery = 4096

// Prog is the xargs core utility.
type Prog struct {
	*coreutils.Runtime

	// tty reads the responses of the -p option. It defaults to /dev/tty,
	// opened on first use, and can be replaced by tests.
	tty io.Reader
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run invokes the utility operand with the arguments read from the standard
// input, as many per invocation as the options allow. The utility defaults
// to echo, and an invocation that exits with 255 stops the processing, as
// do the utilities that cannot be run at all.
//
// Invocations stop when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if p.Exec == nil {
		p.errorf("no command execution available")
		return &coreutils.ExitError{Code: 1}
	}
	if p.Stdin == nil {
		p.errorf("no standard input available")
		return &coreutils.ExitError{Code: 1}
	}
	utility := "echo"
	if len(operands) > 0 {
		utility, operands = operands[0], operands[1:]
	}

	items, err := p.readArgs(ctx, opts)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("standard input: %v", err)
		return &coreutils.ExitError{Code: 1}
	}

	// A failure to assemble a command line stops the processing after the
	// invocations that could be assembled have run.
	cmds, splitErr := splitCommands(items, utility, operands, opts)
	if err := ctx.Err(); err != nil {
		return err
	}

	ok := true
	for _, args := range cmds {
		if err := ctx.Err(); err != nil {
			return err
		}
		if opts.trace || opts.prompt {
			p.trace(utility, args, opts.prompt)
		}
		if opts.prompt {
			yes, err := p.ask()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				p.errorf("%v", err)
				return &coreutils.ExitError{Code: 1}
			}
			if !yes {
				continue
			}
		}
		status, err := p.Exec(ctx, utility, args)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			p.errorf("%s: %v", utility, err)
			return &coreutils.ExitError{Code: status}
		}
		switch status {
		case 0:
		case 255:
			p.errorf("%s exited with status 255", utility)
			return &coreutils.ExitError{Code: 124}
		default:
			ok = false
		}
	}
	if splitErr != nil {
		p.errorf("%v", splitErr)
		return &coreutils.ExitError{Code: 1}
	}
	if !ok {
		return &coreutils.ExitError{Code: 123}
	}
	return nil
}

// trace writes the command about to be invoked to the standard error,
// followed by the prompt of the -p option when given.
func (p *Prog) trace(utility string, args []string, prompt bool) {
	if p.Stderr == nil {
		return
	}
	line := strings.Join(append([]string{utility}, args...), " ")
	if prompt {
		line += " ?..."
	}
	fmt.Fprintln(p.Stderr, line)
}

// ask reads a response for the -p option from /dev/tty, reporting whether
// it's affirmative. In the POSIX locale, an affirmative response is one
// starting with a <y> or <Y>.
func (p *Prog) ask() (bool, error) {
	if p.tty == nil {
		tty, err := os.Open(ttyPath())
		if err != nil {
			return false, err
		}
		p.tty = tty
	}
	line, err := readLine(p.tty)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return len(line) > 0 && (line[0] == 'y' || line[0] == 'Y'), nil
}

// ttyPath is the file the -p responses are read from.
func ttyPath() string {
	if runtime.GOOS == "windows" {
		return "CONIN$"
	}
	return "/dev/tty"
}

// readLine reads a single line, without its <newline>.
func readLine(r io.Reader) ([]byte, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				return line, nil
			}
			line = append(line, b[0])
		}
		if err != nil {
			return line, err
		}
	}
}

// readArgs reads the arguments of the invocations from the standard input,
// delimited as the options require, and stops at the logical end-of-file
// string when it's enabled. Null-delimited input isn't checked for it, as
// the combination of -0 and -E is unspecified.
func (p *Prog) readArgs(ctx context.Context, opts options) ([]string, error) {
	data, err := io.ReadAll(ctxReader{ctx: ctx, r: p.Stdin})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	eof := ""
	if !opts.zero {
		eof = opts.eof
	}
	switch {
	case opts.zero:
		return splitNull(ctx, data)
	case opts.replstr != "":
		return splitLines(ctx, data, eof)
	case opts.maxLines > 0:
		return splitLogicalLines(ctx, data, eof)
	default:
		return tokenize(ctx, data, eof)
	}
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	if p.Stderr == nil {
		return
	}
	fmt.Fprintf(p.Stderr, "xargs: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	eof      string // logical end-of-file string, "" when disabled
	zero     bool   // -0
	replstr  string // -I
	maxLines int    // -L
	maxArgs  int    // -n
	prompt   bool   // -p
	noRun    bool   // -r
	size     int    // -s
	trace    bool   // -t
	exit     bool   // -x, given explicitly
}

// exitIfBig reports whether xargs terminates when a constructed command
// line doesn't fit, which -x requires and -I implies.
func (o options) exitIfBig() bool {
	return o.exit || o.replstr != ""
}

// parseFlags parses the options and operands. The -I, -L and -n options are
// mutually exclusive, and the last one given wins. Option parsing stops at
// the first operand, the utility to invoke, or at "--".
func parseFlags(args []string) (options, []string, error) {
	var sizeSet bool
	opts := options{eof: "_", size: maxLen}
	operands, err := getopt.Set{Valued: "EILns"}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'E':
			opts.eof = value
		case 'I':
			if value == "" {
				return errors.New("value for -I option should not be empty")
			}
			opts.replstr = value
			opts.maxArgs, opts.maxLines = 0, 0
		case 'L':
			n, err := parseNumber(opt, value)
			if err != nil {
				return err
			}
			opts.maxLines = n
			opts.replstr, opts.maxArgs = "", 0
		case 'n':
			n, err := parseNumber(opt, value)
			if err != nil {
				return err
			}
			opts.maxArgs = n
			opts.replstr, opts.maxLines = "", 0
		case 'p':
			opts.prompt = true
		case 'r':
			opts.noRun = true
		case 's':
			n, err := parseSize(value)
			if err != nil {
				return err
			}
			opts.size, sizeSet = n, true
		case 't':
			opts.trace = true
		case 'x':
			opts.exit = true
		case '0':
			opts.zero = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if !sizeSet && opts.maxArgs > 0 {
		opts.size = lineMax
	}
	return opts, operands, err
}

// parseNumber parses the option-argument of the -L and -n options: a
// positive decimal integer.
func parseNumber(opt rune, value string) (int, error) {
	n, err := parseDigits(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("value for -%c option should be a positive integer", opt)
	}
	return n, nil
}

// parseSize parses the option-argument of the -s option: a positive decimal
// integer, with values larger than the largest one supported capped to it,
// as POSIX requires.
func parseSize(value string) (int, error) {
	n, err := parseDigits(value)
	if err != nil {
		return 0, errors.New("value for -s option should be a positive integer")
	}
	if n < 1 {
		return 0, errors.New("value for -s option should be a positive integer")
	}
	return min(n, maxLen), nil
}

// parseDigits parses a positive decimal integer, returning the largest
// integer when the digits don't fit in one.
func parseDigits(value string) (int, error) {
	if value == "" {
		return 0, errors.New("not a number")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		var numErr *strconv.NumError
		if errors.As(err, &numErr) && numErr.Err == strconv.ErrRange {
			return n, nil
		}
		return 0, err
	}
	return n, nil
}

// splitCommands splits the input arguments into the argument lists of each
// invocation, as the options require: one argument per invocation with -I,
// and groups bounded by -n, -L and -s otherwise. When no arguments were
// read, the utility runs once with no extra arguments, unless -r is given.
// A command line that can't be assembled stops the splitting, with the
// commands assembled so far returned along with the error.
func splitCommands(items []string, utility string, initial []string, opts options) ([][]string, error) {
	if len(items) == 0 {
		if opts.noRun {
			return nil, nil
		}
		if cmdLen(utility, initial) >= opts.size {
			return nil, errors.New("argument line too long")
		}
		return [][]string{initial}, nil
	}
	if opts.replstr != "" {
		return replaceCommands(items, utility, initial, opts)
	}
	groups, err := groupItems(items, cmdLen(utility, initial), opts)
	cmds := make([][]string, len(groups))
	for i, group := range groups {
		args := make([]string, 0, len(initial)+len(group))
		args = append(args, initial...)
		args = append(args, group...)
		cmds[i] = args
	}
	return cmds, err
}

// replaceCommands builds one invocation per input argument, replacing every
// occurrence of the replacement string in the initial arguments, and
// appending the input argument when none is replaced. Each command must fit
// the size limit, since -I forces -x on; the commands built so far are
// returned along with the error when one doesn't.
func replaceCommands(items []string, utility string, initial []string, opts options) ([][]string, error) {
	cmds := make([][]string, 0, len(items))
	for _, item := range items {
		args, replaced := replaceArgs(initial, opts.replstr, item)
		if !replaced {
			args = append(args, item)
		}
		if cmdLen(utility, args) >= opts.size {
			return cmds, errors.New("argument line too long")
		}
		cmds = append(cmds, args)
	}
	return cmds, nil
}

// replaceArgs replaces every occurrence of replstr in the initial arguments
// with item, reporting whether any replacement happened.
func replaceArgs(initial []string, replstr, item string) ([]string, bool) {
	args := make([]string, len(initial))
	replaced := false
	for i, arg := range initial {
		if strings.Contains(arg, replstr) {
			args[i] = strings.ReplaceAll(arg, replstr, item)
			replaced = true
		} else {
			args[i] = arg
		}
	}
	return args, replaced
}

// groupItems groups the input arguments into the argument lists of each
// invocation, as many as the -n or -L options allow and the -s size fits,
// base being the length of the utility and its initial arguments. An
// argument that can't fit alone fails, and -x also fails a group smaller
// than the -n or -L count while arguments remain. The groups assembled so
// far are returned along with the error.
func groupItems(items []string, base int, opts options) ([][]string, error) {
	limit := max(opts.maxArgs, opts.maxLines)
	var groups [][]string
	for len(items) > 0 {
		var (
			group  []string
			length = base
		)
		for len(items) > 0 && (limit == 0 || len(group) < limit) &&
			length+len(items[0])+1 < opts.size {
			length += len(items[0]) + 1
			group = append(group, items[0])
			items = items[1:]
		}
		if len(group) == 0 {
			return groups, errors.New("argument line too long")
		}
		if opts.exitIfBig() && len(items) > 0 && limit > 0 && len(group) < limit {
			return groups, errors.New("command line too long")
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// cmdLen returns the length of a command line: the size in bytes of the
// utility name and each argument, including a null byte terminator for each
// of them.
func cmdLen(utility string, args []string) int {
	n := len(utility) + 1
	for _, arg := range args {
		n += len(arg) + 1
	}
	return n
}

// splitNull splits the input at every null byte, as the -0 option requires.
// The trailing bytes are dropped when the input doesn't end in a null byte,
// as they can signal incomplete data. Parsing stops when ctx is done.
func splitNull(ctx context.Context, data []byte) ([]string, error) {
	if len(data) > 0 && data[len(data)-1] != 0 {
		if i := bytes.LastIndexByte(data, 0); i >= 0 {
			data = data[:i+1]
		} else {
			data = nil
		}
	}
	var items []string
	start := 0
	for i := 0; i < len(data); i++ {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if data[i] == 0 {
			items = append(items, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		items = append(items, string(data[start:]))
	}
	return items, nil
}

// tokenize splits the input into arguments delimited by unquoted and
// unescaped blanks and newlines. Double quotes and apostrophes quote the
// characters they enclose, a backslash escapes any character, and multiple
// adjacent delimiters are treated as a single one. Reading stops at the
// logical end-of-file string, when given, and when ctx is done.
func tokenize(ctx context.Context, data []byte, eof string) ([]string, error) {
	var (
		args    []string
		arg     []byte
		started bool
		stopped bool
		quote   byte
	)
	flush := func() error {
		if !started {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if eof != "" && string(arg) == eof {
			stopped = true
			return nil
		}
		args = append(args, string(arg))
		arg, started = arg[:0], false
		return nil
	}
	for i := 0; i < len(data) && !stopped; i++ {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		c := data[i]
		switch {
		case quote != 0:
			switch c {
			case '\n': // a <newline> can't be quoted: it ends the argument
				quote = 0
				if err := flush(); err != nil {
					return nil, err
				}
			case quote:
				quote = 0
			default:
				arg = append(arg, c)
			}
		case c == '"' || c == '\'':
			quote, started = c, true
		case c == '\\':
			started = true
			if i+1 < len(data) {
				i++
				arg = append(arg, data[i])
			} else {
				arg = append(arg, '\\')
			}
		case c == ' ' || c == '\t' || c == '\n':
			if err := flush(); err != nil {
				return nil, err
			}
		default:
			started = true
			arg = append(arg, c)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return args, nil
}

// splitLines splits the input into one argument per line, as the -I option
// requires: arguments are delimited only by newlines not escaped by a
// backslash, empty lines are ignored, and the blanks at the beginning of
// each line are ignored. Reading stops at the logical end-of-file string,
// when given, and when ctx is done.
func splitLines(ctx context.Context, data []byte, eof string) ([]string, error) {
	var (
		args    []string
		line    []byte
		stopped bool
	)
	flush := func() error {
		if len(line) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		content, err := trimLeadingBlanks(ctx, line)
		if err != nil {
			return err
		}
		if eof != "" && string(content) == eof {
			stopped = true
			return nil
		}
		args = append(args, string(content))
		line = line[:0]
		return nil
	}
	for i := 0; i < len(data) && !stopped; i++ {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if data[i] == '\n' && !escaped(data, i) {
			if err := flush(); err != nil {
				return nil, err
			}
		} else {
			line = append(line, data[i])
		}
	}
	if !stopped && len(line) > 0 {
		if err := flush(); err != nil {
			return nil, err
		}
	}
	return args, nil
}

// splitLogicalLines splits the input into one argument per non-empty line,
// as the -L option requires: empty lines are ignored, and a line ending in
// unescaped blanks continues on the next non-empty line, dropping those
// blanks and the newlines in between. Reading stops at the logical
// end-of-file string, when given, and when ctx is done.
func splitLogicalLines(ctx context.Context, data []byte, eof string) ([]string, error) {
	var args []string
	lines := rawLines(data)
	for i := 0; i < len(lines); {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if len(lines[i]) == 0 {
			i++
			continue
		}
		var (
			arg     []byte
			checked int
		)
		arg = append(arg, lines[i]...)
		i++
		for {
			trimmed, removed, err := trimTrailingBlanks(ctx, arg)
			if err != nil {
				return nil, err
			}
			if !removed {
				break
			}
			arg = trimmed
			for i < len(lines) && len(lines[i]) == 0 {
				i++
				if i%checkEvery == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
			}
			if i == len(lines) {
				break
			}
			arg = append(arg, lines[i]...)
			i++
			if len(arg)-checked >= checkEvery {
				checked = len(arg)
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if eof != "" && string(arg) == eof {
			break
		}
		args = append(args, string(arg))
	}
	return args, nil
}

// rawLines splits data at each <newline>, keeping a last line that doesn't
// end in one.
func rawLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.Split(data, []byte{'\n'})
	if data[len(data)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// trimLeadingBlanks returns the line without its leading blanks. The scan
// stops when ctx is done.
func trimLeadingBlanks(ctx context.Context, line []byte) ([]byte, error) {
	start := 0
	for start < len(line) && isBlank(line[start]) {
		start++
		if start%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
	}
	return line[start:], nil
}

// trimTrailingBlanks returns the line without its trailing unescaped blanks,
// reporting whether any was removed. The scan stops when ctx is done.
func trimTrailingBlanks(ctx context.Context, line []byte) ([]byte, bool, error) {
	end := len(line)
	for end > 0 && isBlank(line[end-1]) && !escaped(line, end-1) {
		end--
		if end%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
		}
	}
	return line[:end], end != len(line), nil
}

// escaped reports whether the byte at position i is preceded by an odd
// number of backslashes.
func escaped(data []byte, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && data[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

func isBlank(c byte) bool {
	return c == ' ' || c == '\t'
}

// ctxReader stops read operations with the context error once ctx is done,
// so long inputs don't delay returning.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// Read implements io.Reader.
func (r ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

var _ coreutils.Program = (*Prog)(nil)
