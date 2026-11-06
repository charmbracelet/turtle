// Package xargs implements the POSIX xargs utility.
//
// It reads arguments from the standard input or the file of the -a option,
// delimited by unquoted blanks, newlines, null bytes or a given delimiter,
// and invokes the utility operand with them, in groups that fit the count
// and length limits given by the options.
//
// The GNU extensions are supported as well: -0 (--null), -a (--arg-file),
// -d (--delimiter), --replace, --max-lines, --max-args, --max-chars, --eof,
// --no-run-if-empty, --interactive, --exit and --verbose. The -P
// (--max-procs), -o (--open-tty), --show-limits and --process-slot-var
// options are not implemented.
package xargs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
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

// Run invokes the utility operand with the arguments read from the input
// source, as many per invocation as the options allow. The utility defaults
// to echo, and an invocation that exits with 255 or 125 stops the
// processing, as do the utilities that cannot be run at all.
//
// Invocations stop when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "xargs", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		if p.Stderr != nil {
			stdopts.TryHelp(p.Runtime, "xargs")
		}
		return &coreutils.ExitError{Code: 2}
	}
	if p.Exec == nil {
		p.errorf("no command execution available")
		return &coreutils.ExitError{Code: 1}
	}
	if !opts.argGiven || opts.argFile == "-" {
		if p.Stdin == nil {
			p.errorf("no standard input available")
			return &coreutils.ExitError{Code: 1}
		}
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
		p.errorf("%v", err)
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
		status, err := p.Exec(ctx, coreutils.ExecOpts{Name: utility, Args: args, Env: p.EnvList()})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			p.errorf("%s: %v", utility, err)
			return &coreutils.ExitError{Code: status}
		}
		switch status {
		case 0:
		case 125:
			// An exit status of 125 means the utility was terminated by
			// a signal, so like 255 it stops the processing.
			p.errorf("%s exited with status 125", utility)
			return &coreutils.ExitError{Code: 125}
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

// readArgs reads the arguments of the invocations from the input source,
// the file of the -a option or the standard input, delimited as the options
// require, and stops at the logical end-of-file string when it's enabled.
// Null- and delimiter-separated input isn't checked for it, as the -0 and
// -d options disable the string.
func (p *Prog) readArgs(ctx context.Context, opts options) ([]string, error) {
	in, closer, err := p.openInput(opts.argFile, opts.argGiven)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	data, err := io.ReadAll(ctxio.NewReader(ctx, in))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", inputName(opts.argFile, opts.argGiven), err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	eof := opts.eof
	if opts.zero || opts.delimSet {
		eof = ""
	}
	switch {
	case opts.delimSet:
		return splitDelim(ctx, data, opts.delim)
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

// openInput returns the reader the items are read from, and the closer to
// call when it's done: the standard input, or the file of the -a option,
// "-" denoting the standard input.
func (p *Prog) openInput(file string, given bool) (io.Reader, io.Closer, error) {
	if !given || file == "-" {
		return p.Stdin, nil, nil
	}
	if p.FS == nil {
		return nil, nil, fmt.Errorf("%s: no file system available", file)
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(file)))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", file, patherr.Unwrap(err))
	}
	return f, f, nil
}

// inputName returns the name the input source has in diagnostics.
func inputName(file string, given bool) string {
	if !given || file == "-" {
		return "standard input"
	}
	return file
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
	eof      string // -E, "" when disabled
	zero     bool   // -0
	argFile  string // -a, "-" meaning the standard input
	argGiven bool   // whether -a was given, so an empty name is a name
	delimSet bool   // -d, also set (and zero cleared) by the last of the two
	delim    byte   // -d
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

// The long options --eof, --replace and --max-lines take an optional
// option-argument, which the short forms -E, -I and -L require. The long
// forms have no short equivalent with those semantics, so they map to
// pseudo-runes kept above the bytes a group of short options can hold.
const (
	optEof      rune = 0x100 + iota // --eof[=eof-str]
	optReplace                      // --replace[=replace-str]
	optMaxLines                     // --max-lines[=max-lines]
)

// longOptions maps the long options of xargs to the short ones they stand
// for.
var longOptions = map[string]rune{
	"null":            '0',
	"arg-file":        'a',
	"delimiter":       'd',
	"eof":             optEof,
	"replace":         optReplace,
	"max-lines":       optMaxLines,
	"max-args":        'n',
	"interactive":     'p',
	"no-run-if-empty": 'r',
	"max-chars":       's',
	"verbose":         't',
	"exit":            'x',
}

// optSet describes the options of xargs.
var optSet = getopt.Set{
	Valued:   "aEdILns",
	Optional: string(optEof) + string(optReplace) + string(optMaxLines),
	Long:     longOptions,
}

// usage describes the options of xargs.
const usage = `Usage: xargs [OPTION]... [COMMAND [INITIAL-ARG]...]
Run COMMAND with arguments read from the input; the utility defaults to echo.

  -0, --null
         items are separated by a null character, not whitespace
  -a, --arg-file=FILE
         read the items from FILE instead of the standard input
  -d, --delimiter=CHARACTER
         items are separated by CHARACTER, not whitespace
  -E, --eof[=EOF-STR]
         set the logical end-of-file string; an empty string disables it
  -I, --replace=R
         replace R in the initial arguments with each input item; R defaults to
         {}
  -L, --max-lines=MAX-LINES
         use at most MAX-LINES non-blank input lines per command line
  -n, --max-args=MAX-ARGS
         use at most MAX-ARGS arguments per command line
  -p, --interactive
         prompt before running each command line
  -r, --no-run-if-empty
         do not run the utility when the input holds no items
  -s, --max-chars=MAX-CHARS
         limit each command line to MAX-CHARS characters
  -t, --verbose
         print each command line before running it
  -x, --exit
         exit when a command line exceeds the size limit
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options and operands. The -I, -L and -n options are
// mutually exclusive, and the last one given wins. Option parsing stops at
// the first operand, the utility to invoke, or at "--".
func parseFlags(args []string) (options, []string, error) {
	var sizeSet bool
	opts := options{size: maxLen}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'E', optEof:
			opts.eof = value
		case 'I', optReplace:
			if opt == optReplace && value == "" {
				// The option-argument of --replace is optional,
				// defaulting to "{}".
				value = "{}"
			}
			if value == "" {
				return errors.New("value for -I option should not be empty")
			}
			opts.replstr = value
			opts.maxArgs, opts.maxLines = 0, 0
		case 'L', optMaxLines:
			if opt == optMaxLines && value == "" {
				// The option-argument of --max-lines is optional,
				// defaulting to one line.
				value = "1"
			}
			n, err := parseNumber(optName(opt), value)
			if err != nil {
				return err
			}
			opts.maxLines = n
			opts.replstr, opts.maxArgs = "", 0
		case 'n':
			n, err := parseNumber(optName(opt), value)
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
			opts.zero, opts.delimSet = true, false
		case 'a':
			opts.argFile, opts.argGiven = value, true
		case 'd':
			d, err := parseDelim(value)
			if err != nil {
				return err
			}
			opts.delimSet, opts.delim, opts.zero = true, d, false
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

// optName returns the name an option has in the option-argument diagnostics.
func optName(opt rune) string {
	switch opt {
	case optMaxLines:
		return "--max-lines"
	case 'n':
		return "-n"
	default:
		return "-" + string(opt)
	}
}

// parseNumber parses the option-argument of the -L and -n options: a
// positive decimal integer.
func parseNumber(name, value string) (int, error) {
	n, err := parseDigits(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("value for %s option should be a positive integer", name)
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

// parseDelim parses the option-argument of the -d option: a single
// character, or a backslash escape understood as the printf command
// understands them. Multibyte characters are not supported.
func parseDelim(value string) (byte, error) {
	if len(value) == 1 {
		return value[0], nil
	}
	if len(value) > 1 && value[0] == '\\' {
		if b, err := parseEscape(value); err == nil {
			return b, nil
		}
	}
	return 0, errors.New("value for -d option should be a single character or an escape sequence")
}

// parseEscape parses a backslash escape sequence: the single-character
// escapes, or an octal or hexadecimal character code.
func parseEscape(value string) (byte, error) {
	invalid := fmt.Errorf("invalid escape sequence %s in the -d option value", value)
	if len(value) == 2 {
		switch value[1] {
		case 'a':
			return '\a', nil
		case 'b':
			return '\b', nil
		case 'f':
			return '\f', nil
		case 'n':
			return '\n', nil
		case 'r':
			return '\r', nil
		case 't':
			return '\t', nil
		case 'v':
			return '\v', nil
		case '\\':
			return '\\', nil
		}
	}
	digits := value[1:]
	base := 8
	if value[1] == 'x' {
		digits = value[2:]
		base = 16
	}
	if digits == "" {
		return 0, invalid
	}
	for i := 0; i < len(digits); i++ {
		if !isDigit(digits[i], base) {
			return 0, invalid
		}
	}
	n, err := strconv.ParseUint(digits, base, 8)
	if err != nil {
		return 0, invalid
	}
	return byte(n), nil
}

// isDigit reports whether c is a digit of the given base.
func isDigit(c byte, base int) bool {
	switch {
	case c >= '0' && c <= '9':
		return int(c-'0') < base
	case base == 16 && c >= 'a' && c <= 'f':
		return true
	case base == 16 && c >= 'A' && c <= 'F':
		return true
	default:
		return false
	}
}

// splitDelim splits the input at every occurrence of the delimiter byte of
// the -d option, as GNU xargs does: every character is taken literally, each
// delimiter terminates an argument, and a final run of bytes without one is
// still used as the last argument. Parsing stops when ctx is done.
func splitDelim(ctx context.Context, data []byte, delim byte) ([]string, error) {
	var items []string
	start := 0
	for i := range data {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if data[i] == delim {
			items = append(items, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		items = append(items, string(data[start:]))
	}
	return items, nil
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

var _ coreutils.Program = (*Prog)(nil)
