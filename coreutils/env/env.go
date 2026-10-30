// Package env implements the GNU env utility.
//
// It modifies the current environment according to its options and
// operands, then either writes the resulting environment to the standard
// output or invokes the utility operand with it. The -S option splits its
// argument with shell-like quoting, which supports the #!/usr/bin/env -S
// shebang form, and -C changes the directory the utility runs in.
//
// The -u option is an extension beyond the Issue 8 env specification.
package env

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the env core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run applies the options and operands to the environment, then writes the
// resulting environment to the standard output when no utility operand is
// given, or invokes the utility with it otherwise. The exit status of the
// utility becomes the exit status of env, and the 127, 126 and 125 statuses
// of the execution are passed along with a diagnostic. The failures of env
// itself, including usage and write errors, exit with 125.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "env", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 125, Err: err}
	}
	if opts.chdir != "" {
		if err := p.chdir(opts.chdir); err != nil {
			return err
		}
	}
	if opts.split != "" {
		split, err := p.splitArgs(opts.split)
		if err != nil {
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 125, Err: err}
		}
		operands = append(split, operands...)
	}
	assignments, utility, arguments, given := splitOperands(operands)
	entries := p.buildEnv(opts, assignments)
	if !given {
		return p.printEnv(entries, opts.null)
	}
	return p.invoke(ctx, utility, arguments, entries)
}

// settings holds what a run of env selects.
type settings struct {
	ignore bool     // -i
	unset  []string // -u, in the order given
	null   bool     // -0
	chdir  string   // -C
	split  string   // -S, the arguments to split and prepend
}

// longOptions maps the long options of env to the short ones they stand
// for.
var longOptions = map[string]rune{
	"ignore-environment": 'i',
	"unset":              'u',
	"null":               '0',
	"chdir":              'C',
	"split-string":       'S',
}

// optSet describes the options of env.
var optSet = getopt.Set{Valued: "uCS", Long: longOptions}

// usage describes the options of env.
const usage = `Usage: env [OPTION]... [NAME=VALUE]... [COMMAND [ARG]...]
  or:  env [OPTION]... -S [OPTION]... [NAME=VALUE]... [COMMAND [ARG]...]
Run a command in a modified environment.

With no COMMAND, print the resulting environment.

  -i, --ignore-environment  start with an empty environment
  -u, --unset=NAME           remove NAME from the environment
  -0, --null                 end each output line with NUL, not newline
  -C, --chdir=DIR            change the working directory to DIR
  -S, --split-string=STRING  process and split STRING into separate
                             arguments, used to pass multiple arguments on
                             shebang lines
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the options and operands. The -u option may repeat,
// and option parsing stops at the first operand or at "--".
func parseFlags(args []string) (settings, []string, error) {
	var set settings
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'i':
			set.ignore = true
		case 'u':
			if !validName(value) {
				return fmt.Errorf("cannot unset %q: invalid name", value)
			}
			set.unset = append(set.unset, value)
		case '0':
			set.null = true
		case 'C':
			set.chdir = value
		case 'S':
			set.split = value
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return set, operands, err
}

// chdir changes the directory the utility runs in, checking that it is a
// directory first.
func (p *Prog) chdir(dir string) error {
	resolved := path.Clean(filepath.ToSlash(p.ResolvePath(dir)))
	fi, err := fs.Stat(p.FS, resolved)
	if err != nil {
		p.errorf("cannot change directory to %s: %v", dir, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 125, Err: err}
	}
	if !fi.IsDir() {
		p.errorf("cannot change directory to %s: not a directory", dir)
		return &coreutils.ExitError{Code: 125}
	}
	p.WorkingDir = resolved
	return nil
}

// splitArgs splits an -S string into its arguments, with the shell-like
// quoting the GNU manual documents: whitespace separates the arguments,
// single quotes preserve their content (except \' and \\), double quotes
// process the escapes, an unquoted hash at the start of an argument or the
// \c escape ignores the rest of the string, and ${VAR} expands to the
// value the environment held before the options changed it.
func (p *Prog) splitArgs(s string) ([]string, error) {
	var args []string
	var cur []byte
	inArg := false
	var quote byte
	flush := func() {
		if inArg {
			args = append(args, string(cur))
			cur, inArg = cur[:0], false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote == '\'' {
			switch {
			case c == '\'':
				quote = 0
				continue
			case c == '\\' && i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '\\'):
				i++
				c = s[i]
			}
			cur = append(cur, c)
			continue
		}
		switch c {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			if quote != 0 {
				cur = append(cur, c)
				continue
			}
			flush()
		case '\'':
			quote, inArg = '\'', true
		case '"':
			if quote == '"' {
				quote = 0
			} else {
				quote, inArg = '"', true
			}
		case '#':
			// An unquoted hash only starts a comment at the start of an
			// argument.
			if quote == 0 && !inArg {
				return args, nil
			}
			cur = append(cur, c)
			inArg = true
		case '$':
			if i+1 >= len(s) || s[i+1] != '{' {
				return nil, errors.New("only ${VAR} expansion is supported")
			}
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				return nil, errors.New("missing '}' in ${VAR}")
			}
			name := s[i+2 : i+2+end]
			i += 2 + end
			value, _ := p.LookupEnv(name)
			cur = append(cur, value...)
			inArg = true
		case '\\':
			if i+1 >= len(s) {
				return nil, errors.New("string ends in a backslash")
			}
			i++
			switch e := s[i]; e {
			case 'c':
				if quote == '"' {
					cur = append(cur, 'c')
					continue
				}
				flush()
				return args, nil
			case 'f':
				cur = append(cur, '\f')
			case 'n':
				cur = append(cur, '\n')
			case 'r':
				cur = append(cur, '\r')
			case 't':
				cur = append(cur, '\t')
			case 'v':
				cur = append(cur, '\v')
			case '_':
				if quote == '"' {
					cur = append(cur, ' ')
				} else {
					flush()
				}
			case '#', '$', '"', '\\':
				cur = append(cur, e)
			case '\'':
				// Outside single quotes a backslash before a quote keeps
				// the quote from opening one.
				cur = append(cur, e)
			default:
				// The backslash escapes any character, dropping itself.
				cur = append(cur, e)
			}
			inArg = true
		default:
			cur = append(cur, c)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return args, nil
}

// splitOperands splits the leading name=value assignments from the utility
// operand and its arguments, reporting whether the utility operand is given.
// An empty utility operand is given as any other, and fails to be invoked.
func splitOperands(operands []string) (assignments []string, utility string, args []string, given bool) {
	for i, operand := range operands {
		if !isAssignment(operand) {
			return operands[:i], operand, operands[i+1:], true
		}
	}
	return operands, "", nil, false
}

// isAssignment reports whether an operand is of the name=value form.
func isAssignment(operand string) bool {
	name, _, found := strings.Cut(operand, "=")
	return found && validName(name)
}

// buildEnv builds the resulting environment: the one enumerated by the
// runtime, unless -i ignores it, minus the names unset with -u, with the
// name=value assignments applied in order.
func (p *Prog) buildEnv(opts settings, assignments []string) []string {
	var inherited []string
	if !opts.ignore {
		inherited = p.EnvList()
	}
	entries := make([]string, 0, len(inherited)+len(assignments))
	entries = append(entries, inherited...)
	for _, name := range opts.unset {
		entries = unsetEnv(entries, name)
	}
	for _, assignment := range assignments {
		name, value, _ := strings.Cut(assignment, "=")
		entries = setEnv(entries, name, value)
	}
	return entries
}

// printEnv writes the environment to the standard output, one name=value
// pair per line, each terminated with a newline or a NUL with -0.
func (p *Prog) printEnv(entries []string, null bool) error {
	if p.Stdout == nil {
		p.errorf("no standard output available")
		return &coreutils.ExitError{Code: 125}
	}
	term := "\n"
	if null {
		term = "\x00"
	}
	for _, entry := range entries {
		if _, err := fmt.Fprint(p.Stdout, entry+term); err != nil {
			return &coreutils.ExitError{Code: 125, Err: err}
		}
	}
	return nil
}

// invoke runs the utility with the arguments and the built environment. The
// exit status of the utility becomes the exit status of env, and the 127,
// 126 or 125 status the execution returns is passed along with a
// diagnostic.
func (p *Prog) invoke(ctx context.Context, utility string, args, entries []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Exec == nil {
		p.errorf("no command execution available")
		return &coreutils.ExitError{Code: 125}
	}
	status, err := p.Exec(ctx, utility, args, entries)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: status, Err: err}
	}
	if status != 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

// unsetEnv removes every entry of the given name, in place.
func unsetEnv(entries []string, name string) []string {
	out := entries[:0]
	for _, entry := range entries {
		if key, _, _ := strings.Cut(entry, "="); key != name {
			out = append(out, entry)
		}
	}
	return out
}

// setEnv sets the entry of the given name to name=value, replacing its
// first occurrence and dropping any others, or appending it, in place.
func setEnv(entries []string, name, value string) []string {
	entry := name + "=" + value
	set := false
	out := entries[:0]
	for _, e := range entries {
		if key, _, _ := strings.Cut(e, "="); key == name {
			if !set {
				out = append(out, entry)
				set = true
			}
			continue
		}
		out = append(out, e)
	}
	if !set {
		out = append(out, entry)
	}
	return out
}

// validName reports whether name is a valid environment variable name: it
// starts with a letter or an underscore, and the rest is made of letters,
// digits and underscores.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	if p.Stderr == nil {
		return
	}
	fmt.Fprintf(p.Stderr, "env: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
