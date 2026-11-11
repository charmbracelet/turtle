// Package sed implements the sed utility of POSIX with the GNU extensions
// of its invocation, commands and addresses.
//
// The script is parsed into a flat list of commands, where a block of
// commands is delimited by a `{` command holding the index of the command
// following its matching `}`. Regular expressions are basic regular
// expressions by default, and extended ones when -E is given. Backreferences
// in patterns, collating symbols and equivalence classes are not supported,
// and neither is the e command, which runs a shell.
package sed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// errMissingScript reports that no script of editing commands was given,
// which is a usage error rather than a file or runtime error.
var errMissingScript = errors.New("missing script")

// Prog is the sed core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run edits the standard input or the given files according to the script
// given by the operands and the -e and -f options. With -i, the given files
// are edited in place, their contents replaced by the output of the script.
//
// An input file that can't be opened doesn't keep the ones after it from
// being processed, and the exit status is 2, unless a q or Q command asks
// for another one. Reading is interrupted when ctx is done, and the context
// error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "sed", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "sed")
		return &coreutils.ExitError{Code: 2}
	}

	script, files, err := p.assembleScript(opts, operands)
	if err != nil {
		p.errorf("%v", err)
		if errors.Is(err, errMissingScript) {
			stdopts.TryHelp(p.Runtime, "sed")
			return &coreutils.ExitError{Code: 2}
		}
		return &coreutils.ExitError{Code: 1}
	}

	cmds, err := parseScript(script, opts)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}

	quiet := opts.quiet || strings.HasPrefix(script, "#n")
	var res runResult
	if opts.inPlace {
		if len(files) == 0 || slices.Contains(files, "-") {
			p.errorf("-i may not be used with stdin")
			stdopts.TryHelp(p.Runtime, "sed")
			return &coreutils.ExitError{Code: 2}
		}
		res, err = p.runInPlace(ctx, script, opts, files, quiet)
	} else {
		res, err = p.run(ctx, cmds, files, opts, quiet, nil)
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if res.openFailed {
		return &coreutils.ExitError{Code: 2}
	}
	if res.exit != 0 {
		return &coreutils.ExitError{Code: res.exit}
	}
	return nil
}

// scriptPart is an addition to the script of editing commands, from the
// script operand, a -e option-argument, or the contents of a -f script file.
type scriptPart struct {
	text string
	file string
	// fromFile tells a file part apart from a script-text one, since an
	// empty option-argument is an empty script for -e but a file name for -f.
	fromFile bool
}

// options holds the command line options of an invocation.
type options struct {
	quiet    bool
	extended bool
	inPlace  bool
	suffix   string
	parts    []scriptPart

	separate       bool // -s
	nullData       bool // -z
	unbuffered     bool // -u
	posix          bool
	followSymlinks bool
	lineLength     int // -l, the wrap width of the l command
}

// The runes the long-only options stand for, kept out of the range where
// the short ones are.
const (
	posixOpt rune = iota + 0x100
	followSymlinksOpt
)

// longOptions maps the long options of sed to the short ones they stand for.
var longOptions = map[string]rune{
	"expression":      'e',
	"file":            'f',
	"in-place":        'i',
	"line-length":     'l',
	"quiet":           'n',
	"silent":          'n',
	"separate":        's',
	"unbuffered":      'u',
	"null-data":       'z',
	"zero-terminated": 'z',
	"regexp-extended": 'E',
	"extended-regexp": 'E',
	"posix":           posixOpt,
	"follow-symlinks": followSymlinksOpt,
}

// optSet describes the options of sed.
var optSet = getopt.Set{Valued: "efl", Optional: "i", Long: longOptions}

// usage describes the options of sed.
const usage = `Usage: sed [OPTION]... {script-only-if-no-other-script} [input-file]...
Edit the standard input or the given files with a script of commands.

  -n, --quiet, --silent
         suppress automatic printing of the pattern space
  -e script, --expression=script
         add script to the commands to be executed
  -f script-file, --file=script-file
         add the contents of script-file to the commands to be executed
  -i[SUFFIX], --in-place[=SUFFIX]
         edit files in place, making a backup with SUFFIX
  -l N, --line-length=N
         set the line-wrap length of the l command
  -E, -r, --regexp-extended, --extended-regexp
         use extended regular expressions in the script
  -s, --separate
         consider the files as separate streams, restarting the line
         numbers and the ranges for each one
  -z, --null-data, --zero-terminated
         use the NUL byte instead of the newline as the record separator
  -u, --unbuffered
         load minimal amounts of data and flush the output more often
      --follow-symlinks
         follow symlinks when editing files in place
      --posix
         reject the GNU extensions of the script syntax
      --help
         display this help and exit
      --version
         output version information and exit
`

func parseFlags(args []string) (*options, []string, error) {
	opts := &options{lineLength: lDefaultWidth}
	operands, err := optSet.Parse(expandInPlace(args), func(opt rune, value string) error {
		switch opt {
		case 'n':
			opts.quiet = true
		case 'E', 'r':
			opts.extended = true
		case 'e':
			opts.parts = append(opts.parts, scriptPart{text: value})
		case 'f':
			opts.parts = append(opts.parts, scriptPart{file: value, fromFile: true})
		case 'i':
			opts.inPlace, opts.suffix = true, value
		case 'l':
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return fmt.Errorf("invalid line length: %s", value)
			}
			opts.lineLength = n
		case 's':
			opts.separate = true
		case 'z':
			opts.nullData = true
		case 'u':
			opts.unbuffered = true
		case posixOpt:
			opts.posix = true
		case followSymlinksOpt:
			opts.followSymlinks = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return opts, operands, nil
}

// expandInPlace rewrites the short groups holding an -i option, like
// "-ni.bak", into the long form the parser understands, since the suffix of
// -i must be attached to it, as in GNU sed, rather than taken from the next
// argument. Everything past the first operand is left as it is.
func expandInPlace(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			return append(out, args[i:]...) // the options ended
		}
		if strings.HasPrefix(arg, "--") {
			out = append(out, arg)
			continue
		}
		expanded, valued := expandShortInPlace(arg)
		out = append(out, expanded...)
		if valued {
			i++ // the next argument is the value of the last option
			if i < len(args) {
				out = append(out, args[i])
			}
		}
	}
	return out
}

// expandShortInPlace rewrites one short option group, reporting the options
// it stands for and whether the next argument is the value of its last
// option. A group holding -i splits there, the rest of it being the suffix.
func expandShortInPlace(arg string) ([]string, bool) {
	for j := 1; j < len(arg); j++ {
		switch arg[j] {
		case 'i':
			long := "--in-place"
			if rest := arg[j+1:]; rest != "" {
				long += "=" + rest
			}
			if j > 1 {
				return []string{arg[:j], long}, false
			}
			return []string{long}, false
		case 'e', 'f', 'l': // a valued option, the rest of the group is its value
			return []string{arg}, j+1 == len(arg)
		}
	}
	return []string{arg}, false
}

// assembleScript builds the script of editing commands from the -e and -f
// options and, when neither was given, from the first operand, which then
// isn't a file. The remaining operands name the input files. A -f argument
// of - takes the script from the standard input.
func (p *Prog) assembleScript(opts *options, operands []string) (string, []string, error) {
	parts := opts.parts
	files := operands
	if len(parts) == 0 {
		if len(operands) == 0 {
			return "", nil, errMissingScript
		}
		parts = []scriptPart{{text: operands[0]}}
		files = operands[1:]
	}

	var b strings.Builder
	prevE := false
	for _, part := range parts {
		if prevE {
			b.WriteByte('\n')
		}
		if part.fromFile {
			content, err := p.readScript(part.file)
			if err != nil {
				return "", nil, fmt.Errorf("%s: %v", part.file, patherr.Unwrap(err))
			}
			b.WriteString(content)
			prevE = false
			continue
		}
		b.WriteString(part.text)
		prevE = true
	}
	return b.String(), files, nil
}

// readScript reads the script file of a -f option, with the standard input
// used when the name is "-".
func (p *Prog) readScript(name string) (string, error) {
	if name == "-" {
		if p.Stdin == nil {
			return "", errors.New("no standard input available")
		}
		content, err := io.ReadAll(p.Stdin)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}
	return p.readFile(name)
}

// readFile reads a file of the runtime file system.
func (p *Prog) readFile(name string) (string, error) {
	if p.FS == nil {
		return "", errors.New("no file system available")
	}
	f, err := p.FS.Open(p.path(name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// path resolves a path operand for the runtime file system.
func (p *Prog) path(name string) string {
	return filepath.ToSlash(p.ResolvePath(name))
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "sed: "+format+"\n", args...)
}

var _ coreutils.Program = (*Prog)(nil)
