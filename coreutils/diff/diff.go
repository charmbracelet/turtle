// Package diff implements the diff utility.
//
// It compares two files line by line, writing the changes necessary to
// convert the first into the second, in one of the formats diff offers.
// An operand of "-" means the standard input, and a directory operand
// makes diff compare the files of the same name inside it.
package diff

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the diff core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run compares the operands line by line, writing the changes that turn
// each first file into its second. It returns an exit status of 1 when any
// comparison finds differences and 2 when any other error happens.
//
// Comparisons are interrupted when ctx is done, and the context error is
// returned as-is.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "diff", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "diff", err)
	}
	if opts.version {
		return p.printVersion()
	}
	if err := p.loadExcludes(ctx, &opts); err != nil {
		return p.exit(false, err)
	}

	if opts.fromFile != "" {
		if opts.toFile != "" {
			stdopts.Errorf(p.Runtime, "diff", "--from-file and --to-file both specified")
			return &coreutils.ExitError{Code: 2}
		}
		return p.compareAll(ctx, opts, operands, opts.fromFile, true)
	}
	if opts.toFile != "" {
		return p.compareAll(ctx, opts, operands, opts.toFile, false)
	}

	if len(operands) < 2 {
		stdopts.Errorf(p.Runtime, "diff", "missing operand")
		stdopts.TryHelp(p.Runtime, "diff")
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 2 {
		stdopts.Errorf(p.Runtime, "diff", "extra operand '%s'", operands[2])
		stdopts.TryHelp(p.Runtime, "diff")
		return &coreutils.ExitError{Code: 2}
	}
	differ, err := p.compare(ctx, filepath.ToSlash(operands[0]), filepath.ToSlash(operands[1]), opts)
	return p.exit(differ, err)
}

// compareAll compares every operand against the given file, as the
// --from-file and --to-file options ask: with the operand first when the
// file is the from-file, and second when it's the to-file.
func (p *Prog) compareAll(ctx context.Context, opts options, operands []string, other string, fromFile bool) error {
	differ := false
	for _, name := range operands {
		first, second := name, other
		if fromFile {
			first, second = other, name
		}
		d, err := p.compare(ctx, filepath.ToSlash(first), filepath.ToSlash(second), opts)
		if err != nil {
			return p.exit(false, err)
		}
		differ = differ || d
	}
	if differ {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// exit turns the outcome of a comparison into the exit status of diff,
// reporting the errors it can name.
func (p *Prog) exit(differ bool, err error) error {
	if err != nil {
		var writeErr *writeError
		var pathErr *pathError
		switch {
		case errors.As(err, &writeErr):
			stdopts.Errorf(p.Runtime, "diff", "write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 2, Err: writeErr.err}
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return err
		case errors.Is(err, errNoFS):
			stdopts.Errorf(p.Runtime, "diff", "%v", err)
			return &coreutils.ExitError{Code: 2}
		case errors.Is(err, errIfdefDirs):
			stdopts.Errorf(p.Runtime, "diff", "%v", err)
			return &coreutils.ExitError{Code: 2}
		case errors.As(err, &pathErr):
			stdopts.Errorf(p.Runtime, "diff", "%s: %v", pathErr.label, patherr.Unwrap(pathErr.err))
			return &coreutils.ExitError{Code: 2, Err: pathErr.err}
		default:
			return err
		}
	}
	if differ {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// compare resolves both operands and compares them, reporting whether they
// differ.
func (p *Prog) compare(ctx context.Context, name1, name2 string, opts options) (bool, error) {
	op1, err := p.operand(name1, opts, 0, name2)
	if err != nil {
		return false, err
	}
	op2, err := p.operand(name2, opts, 1, name1)
	if err != nil {
		return false, err
	}

	switch {
	case op1.isDir && op2.isDir:
		return p.compareDirs(ctx, op1, op2, opts, nil)
	case op1.isDir:
		base := path.Base(op2.name)
		return p.compareFiles(ctx, pair{
			name1: path.Join(op1.name, base),
			path1: path.Join(op1.path, base),
			name2: op2.name,
			path2: op2.path,
		}, opts)
	case op2.isDir:
		base := path.Base(op1.name)
		return p.compareFiles(ctx, pair{
			name1: op1.name,
			path1: op1.path,
			name2: path.Join(op2.name, base),
			path2: path.Join(op2.path, base),
		}, opts)
	default:
		return p.compareFiles(ctx, pair{
			name1:    op1.name,
			path1:    op1.path,
			missing1: op1.missing,
			name2:    op2.name,
			path2:    op2.path,
			missing2: op2.missing,
		}, opts)
	}
}

// operand holds a resolved command line operand: its name as given, its
// path in the file system, empty for the standard input, whether it names
// a directory, and whether the new file options ask to treat it as a
// missing file, compared against an empty one.
type operand struct {
	name    string
	path    string
	isDir   bool
	missing bool
}

// errNoFS marks the lack of a file system to reach paths through.
var errNoFS = errors.New("no file system available")

// errIfdefDirs marks a merged output asked of a directory comparison.
var errIfdefDirs = errors.New("-D option not supported with directories")

// operand resolves the given operand name, with side telling which of the
// two operands it is and other holding the name of its counterpart.
func (p *Prog) operand(name string, opts options, side int, other string) (operand, error) {
	if name == "-" {
		return operand{name: name}, nil
	}
	if p.FS == nil {
		return operand{}, errNoFS
	}
	op := operand{name: name, path: p.FsName(name)}
	fi, err := fs.Stat(p.FS, op.path)
	if err == nil {
		op.isDir = fi.IsDir()
		return op, nil
	}
	// The new file options treat a missing file as an empty one, when
	// its counterpart does exist: -N on either side, and -P only when
	// the first file is the missing one.
	if (opts.newFile || (side == 0 && opts.unidirectional)) &&
		errors.Is(err, fs.ErrNotExist) && p.exists(other) {
		op.missing = true
		return op, nil
	}
	return operand{}, &pathError{label: name, err: err}
}

// exists reports whether the operand name can be reached, with the
// standard input always counting as present.
func (p *Prog) exists(name string) bool {
	if name == "-" {
		return true
	}
	if p.FS == nil {
		return false
	}
	_, err := fs.Stat(p.FS, p.FsName(name))
	return err == nil
}

// printVersion writes the version text, like the --version option which
// the -v shortcut of diff stands for.
func (p *Prog) printVersion() error {
	if _, err := fmt.Fprint(p.Stdout, stdopts.VersionText("diff")); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// options holds the parsed command line options.
type options struct {
	brief           bool
	reportIdentical bool
	text            bool
	recursive       bool
	format          format
	lines           int
	explicitContext bool
	ocontext        int  // context lines of the obsolete -NUM, -1 when none
	digits          bool // the last option seen was one of those digits

	// White space and case comparisons, and the changes they ignore.
	ignoreCase         bool
	blank              bool // -b
	ignoreAllSpace     bool // -w
	ignoreTabExpansion bool // -E
	ignoreTrailing     bool // -Z
	blankLines         bool // -B
	ignoreRes          []*regexp.Regexp

	labels         [2]string // -L
	newFile        bool      // -N
	unidirectional bool      // -P
	ifdef          string    // -D
	fromFile       string    // --from-file
	toFile         string    // --to-file
	stripCR        bool      // --strip-trailing-cr
	binary         bool      // --binary, a no-op here
	expandTabs     bool      // -t
	initialTab     bool      // -T
	tabSize        int       // --tabsize
	width          int       // -W
	leftColumn     bool      // --left-column
	suppressCommon bool      // --suppress-common-lines
	version        bool      // -v

	excludes    []string // -x patterns
	excludeFrom []string // -X files
	patterns    []string // all the exclude patterns, -x and -X together

	// cmdline holds the command line as given, options and all, which
	// the header of files compared under directories repeats.
	cmdline string
}

// format is the output format of the changes.
type format uint8

const (
	normalFormat format = iota
	contextFormat
	unifiedFormat
	edFormat
	forwardEdFormat
	rcsFormat
	ifdefFormat
	sideFormat
)

// Long-only options, kept well above the range of the option letters.
const (
	optBinary rune = iota + 0x20000
	optFrom
	optLeftColumn
	optNormal
	optStripCR
	optSuppressCommon
	optTabSize
	optTo
	optUniNew
)

// optSet describes the options of diff.
var optSet = getopt.Set{
	Valued:   "CDILUWxX" + string(optFrom) + string(optTabSize) + string(optTo),
	Optional: "CU",
	Long: map[string]rune{
		"brief":                   'q',
		"report-identical-files":  's',
		"context":                 'C',
		"unified":                 'U',
		"ed":                      'e',
		"forward-ed":              'f',
		"rcs":                     'n',
		"side-by-side":            'y',
		"width":                   'W',
		"left-column":             optLeftColumn,
		"suppress-common-lines":   optSuppressCommon,
		"label":                   'L',
		"expand-tabs":             't',
		"initial-tab":             'T',
		"tabsize":                 optTabSize,
		"strip-trailing-cr":       optStripCR,
		"binary":                  optBinary,
		"recursive":               'r',
		"new-file":                'N',
		"unidirectional-new-file": optUniNew,
		"exclude":                 'x',
		"exclude-from":            'X',
		"from-file":               optFrom,
		"to-file":                 optTo,
		"ignore-case":             'i',
		"ignore-tab-expansion":    'E',
		"ignore-trailing-space":   'Z',
		"ignore-space-change":     'b',
		"ignore-all-space":        'w',
		"ignore-blank-lines":      'B',
		"ignore-matching-lines":   'I',
		"text":                    'a',
		"ifdef":                   'D',
		"normal":                  optNormal,
	},
}

// usage describes the options of diff.
const usage = `Usage: diff [OPTION]... FILE1 FILE2
Compare FILE1 with FILE2 line by line, writing the changes which turn
FILE1 into FILE2. An operand of '-' means the standard input, and a
directory operand makes the files of the same name inside it compared.

  -q
         report only when files differ
  -s
         report when two files are the same
  -c
         output 3 lines of copied context
  -C NUM
         output NUM lines of copied context
      --context[=NUM]
         like -C NUM, or 3 lines when NUM is left out
  -u
         output 3 lines of unified context
  -U NUM
         output NUM lines of unified context
      --unified[=NUM]
         like -U NUM, or 3 lines when NUM is left out
  -e
         output an ed script
  -f
         output a forward ed script
  -n
         output an RCS format diff
  -y
         output in two columns
  -W NUM
         output at most NUM (default 130) print columns
      --left-column
         output only the left column of common lines
      --suppress-common-lines
         do not output common lines
  -L LABEL
         use LABEL instead of file name and timestamp (can be repeated)
  -t
         expand tabs to spaces in output
  -T
         make tabs line up by prepending a tab
      --tabsize=NUM
         tab stops every NUM (default 8) print columns
      --strip-trailing-cr
         strip trailing carriage return on input
      --binary
         read and write data in binary mode (a no-op here)
  -r
         compare directories recursively
  -N
         treat absent files as empty
      --unidirectional-new-file
         treat absent first files as empty
  -x PAT
         exclude files that match PAT
  -X FILE
         exclude files that match any pattern in FILE
      --from-file=FILE1
         compare FILE1 to all operands; FILE1 can be a directory
      --to-file=FILE2
         compare all operands to FILE2; FILE2 can be a directory
  -i
         ignore case differences in file contents
  -E
         ignore changes due to tab expansion
  -Z
         ignore white space at line end
  -b
         ignore changes in the amount of white space
  -w
         ignore all white space
  -B
         ignore changes where lines are all blank
  -I RE
         ignore changes where all lines match RE
  -a
         treat all files as text
  -D NAME
         output a merged file to show '#ifdef NAME' differences
      --normal
         output a normal diff (the default)
` + stdopts.Tail

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	opts.ocontext = -1
	opts.cmdline = commandLine(args)

	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		if isDigit(opt) {
			if opts.digits {
				opts.ocontext = 10*opts.ocontext + int(opt-'0')
			} else {
				opts.ocontext = int(opt - '0')
			}
			opts.digits = true
			return nil
		}
		opts.digits = false
		switch opt {
		case 'q':
			opts.brief = true
		case 's':
			opts.reportIdentical = true
		case 'a':
			opts.text = true
		case 'r':
			opts.recursive = true
		case 'b':
			opts.blank = true
		case 'w':
			opts.ignoreAllSpace = true
		case 'i':
			opts.ignoreCase = true
		case 'E':
			opts.ignoreTabExpansion = true
		case 'Z':
			opts.ignoreTrailing = true
		case 'B':
			opts.blankLines = true
		case 't':
			opts.expandTabs = true
		case 'T':
			opts.initialTab = true
		case 'N':
			opts.newFile = true
		case 'P', optUniNew:
			opts.unidirectional = true
		case 'y':
			return opts.setStyle(sideFormat, 0)
		case 'n':
			return opts.setStyle(rcsFormat, 0)
		case 'e':
			return opts.setStyle(edFormat, 0)
		case 'f':
			return opts.setStyle(forwardEdFormat, 0)
		case 'c':
			return opts.setStyle(contextFormat, 3)
		case 'u':
			return opts.setStyle(unifiedFormat, 3)
		case 'D':
			opts.ifdef = value
			return opts.setStyle(ifdefFormat, 0)
		case 'v':
			opts.version = true
		case 'I':
			re, err := regexp.Compile(value)
			if err != nil {
				return err
			}
			opts.ignoreRes = append(opts.ignoreRes, re)
		case 'x':
			opts.excludes = append(opts.excludes, value)
		case 'X':
			opts.excludeFrom = append(opts.excludeFrom, value)
		case 'L':
			return opts.addLabel(value)
		case 'C', 'U':
			f, min := contextFormat, 1
			if opt == 'U' {
				f, min = unifiedFormat, 0
			}
			n := 3
			if value != "" {
				count, err := parseCount(value, min)
				if err != nil {
					return err
				}
				n = count
			}
			if err := opts.setStyle(f, n); err != nil {
				return err
			}
			opts.explicitContext = true
		case 'W':
			n, err := parsePositive(value, "width")
			if err != nil {
				return err
			}
			if opts.width != 0 && opts.width != n {
				return errors.New("conflicting width options")
			}
			opts.width = n
		case optTabSize:
			n, err := parsePositive(value, "tabsize")
			if err != nil {
				return err
			}
			if opts.tabSize != 0 && opts.tabSize != n {
				return errors.New("conflicting tabsize options")
			}
			opts.tabSize = n
		case optFrom:
			opts.fromFile = value
		case optTo:
			opts.toFile = value
		case optBinary:
			opts.binary = true
		case optStripCR:
			opts.stripCR = true
		case optLeftColumn:
			opts.leftColumn = true
		case optSuppressCommon:
			opts.suppressCommon = true
		case optNormal:
			return opts.setStyle(normalFormat, 0)
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	opts.applyObsoleteContext()
	return opts, operands, nil
}

// isDigit reports whether the option is one of the digits of the obsolete
// context syntax, as in "diff -3".
func isDigit(opt rune) bool {
	return '0' <= opt && opt <= '9'
}

// applyObsoleteContext gives the context count of the obsolete -NUM
// syntax to the context and unified formats, as GNU diff does: it raises
// the count, and lowers it only when -C or -U didn't give one.
func (o *options) applyObsoleteContext() {
	if o.ocontext < 0 {
		return
	}
	if o.format != contextFormat && o.format != unifiedFormat {
		return
	}
	if o.lines < o.ocontext || (o.ocontext < o.lines && !o.explicitContext) {
		o.lines = o.ocontext
	}
}

// addLabel records the -L label of one of the two files.
func (o *options) addLabel(label string) error {
	if o.labels[0] == "" {
		o.labels[0] = label
		return nil
	}
	if o.labels[1] == "" {
		o.labels[1] = label
		return nil
	}
	return errors.New("too many file label options")
}

// setStyle picks the output format, rejecting a conflicting one.
func (o *options) setStyle(f format, lines int) error {
	if o.format != normalFormat && o.format != f {
		return errors.New("conflicting output style options")
	}
	o.format = f
	o.lines = lines
	return nil
}

// parseCount parses the option-argument of -C or -U as a line count of at
// least min lines.
func parseCount(value string, min int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min {
		return 0, fmt.Errorf("invalid context length '%s'", value)
	}
	return n, nil
}

// parsePositive parses the option-argument of the width and tab size
// options as a positive number, whose name names for the error.
func parsePositive(value, name string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s '%s'", name, value)
	}
	return n, nil
}

// commandLine builds the command line as the header of files compared
// under directories repeats it: "diff" plus the options as they were
// given, quoting the ones a shell couldn't read back.
func commandLine(args []string) string {
	var b strings.Builder
	b.WriteString("diff")
	for _, arg := range optionArgs(args) {
		b.WriteString(" ")
		b.WriteString(quote(arg))
	}
	return b.String()
}

// optionArgs returns the arguments which hold options, up to the first
// operand or the "--" which ends them, which the option parser also stops
// at. The values of the options which take one belong to them, even when
// held by the next argument.
func optionArgs(args []string) []string {
	var out []string
	nextIsValue := false
	for _, arg := range args {
		if nextIsValue {
			out = append(out, arg)
			nextIsValue = false
			continue
		}
		switch {
		case arg == "--":
			return append(out, arg)
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			return out
		case strings.HasPrefix(arg, "--"):
			out = append(out, arg)
			name, _, given := strings.Cut(arg[2:], "=")
			opt, ok := optSet.Long[name]
			nextIsValue = ok && !given && strings.ContainsRune(optSet.Valued, opt)
		default:
			out = append(out, arg)
			nextIsValue = shortNeedsValue(arg)
		}
	}
	return out
}

// shortNeedsValue reports whether the option group ends with an option
// which takes a value from the next argument.
func shortNeedsValue(arg string) bool {
	for j := 1; j < len(arg); j++ {
		opt := rune(arg[j])
		if !strings.ContainsRune(optSet.Valued, opt) && !strings.ContainsRune(optSet.Optional, opt) {
			continue
		}
		return j+1 == len(arg)
	}
	return false
}

// quote writes the option as a shell word, plain when it needs no
// quoting.
func quote(arg string) string {
	const safe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-"
	for _, c := range arg {
		if !strings.ContainsRune(safe, c) {
			return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		}
	}
	if arg == "" {
		return "''"
	}
	return arg
}

var _ coreutils.Program = (*Prog)(nil)
