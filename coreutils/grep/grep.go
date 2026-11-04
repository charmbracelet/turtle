// Package grep implements the POSIX grep utility.
package grep

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/bre"
	"charm.land/snail/coreutils/internal/ere"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the grep core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the input lines matching the patterns.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "grep", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "grep")
		return &coreutils.ExitError{Code: 2}
	}
	patterns, files, err := p.collectPatterns(ctx, opts, operands)
	if err != nil {
		return err
	}
	if len(patterns) == 0 {
		p.errorf("no pattern given")
		stdopts.TryHelp(p.Runtime, "grep")
		return &coreutils.ExitError{Code: 2}
	}
	m, err := newMatcher(patterns, opts)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	inputs := input.List(files)
	multiple := len(inputs) > 1
	var selected, failed bool
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		matched, err := p.searchFile(ctx, m, in, opts, multiple)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			if accessErr, ok := errors.AsType[*accessError](err); ok {
				if !opts.suppress {
					p.errorf("%s: %v", fileLabel(in), patherr.Unwrap(accessErr.err))
				}
				failed = true
			} else {
				p.errorf("write error: %v", err)
				return &coreutils.ExitError{Code: 2, Err: err}
			}
		}
		if matched {
			selected = true
		}
		if opts.output == 'q' && selected {
			return nil
		}
	}
	switch {
	case failed:
		return &coreutils.ExitError{Code: 2}
	case !selected:
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// collectPatterns splits the pattern lists of the options and of the first
// operand, which is the pattern only when no -e or -f option was given, and
// returns the remaining operands as the files to search.
func (p *Prog) collectPatterns(ctx context.Context, opts options, operands []string) ([]string, []string, error) {
	var patterns []string
	if len(opts.patternLists) == 0 && len(opts.patternFiles) == 0 && len(operands) > 0 {
		patterns = append(patterns, splitPatterns(operands[0])...)
		operands = operands[1:]
	}
	for _, list := range opts.patternLists {
		patterns = append(patterns, splitPatterns(list)...)
	}
	for _, name := range opts.patternFiles {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		data, err := p.readPatternFile(ctx, name)
		if err != nil {
			return nil, nil, err
		}
		patterns = append(patterns, splitPatterns(string(data))...)
	}
	return patterns, operands, nil
}

// readPatternFile returns the contents of a pattern file.
func (p *Prog) readPatternFile(ctx context.Context, name string) ([]byte, error) {
	r, err := input.Open(p.Runtime, ctx, input.New(name))
	if err != nil {
		p.errorf("%s: %v", name, patherr.Unwrap(err))
		return nil, &coreutils.ExitError{Code: 2}
	}
	data, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		p.errorf("%s: %v", name, patherr.Unwrap(readErr))
		return nil, &coreutils.ExitError{Code: 2}
	}
	if closeErr != nil {
		p.errorf("%s: %v", name, patherr.Unwrap(closeErr))
		return nil, &coreutils.ExitError{Code: 2}
	}
	return data, nil
}

// searchFile searches one input for matching lines, writing the output the
// options ask for, and reports whether any line was selected.
func (p *Prog) searchFile(ctx context.Context, m *matcher, in input.Input, opts options, multiple bool) (matched bool, err error) {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return false, &accessError{err}
	}
	defer func() {
		if cerr := r.Close(); cerr != nil && err == nil {
			err = &accessError{cerr}
		}
	}()
	label := fileLabel(in)
	reader := bufio.NewReader(r)
	var count, lineNum int
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNum++
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if m.matches(line) {
				matched = true
				switch opts.output {
				case 'q':
					return true, nil
				case 'l':
					_, werr := fmt.Fprintf(p.Stdout, "%s\n", label)
					return true, werr
				case 'c':
					count++
				default:
					prefix := ""
					if multiple {
						prefix = label + ":"
					}
					if opts.lineNum {
						prefix += strconv.Itoa(lineNum) + ":"
					}
					if _, werr := fmt.Fprintf(p.Stdout, "%s%s\n", prefix, line); werr != nil {
						return matched, werr
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return matched, &accessError{readErr}
		}
	}
	if opts.output == 'c' {
		prefix := ""
		if multiple {
			prefix = label + ":"
		}
		if _, werr := fmt.Fprintf(p.Stdout, "%s%d\n", prefix, count); werr != nil {
			return matched, werr
		}
	}
	return matched, nil
}

// fileLabel returns the name to use for an input on the output and the
// diagnostic messages.
func fileLabel(in input.Input) string {
	if in.Stdin {
		return "(standard input)"
	}
	return in.Name
}

// splitPatterns splits a pattern list into its patterns, which are separated
// by newlines. A trailing newline terminates the last pattern instead of
// opening an empty one.
func splitPatterns(list string) []string {
	if list == "" {
		return nil
	}
	patterns := strings.Split(list, "\n")
	if strings.HasSuffix(list, "\n") {
		patterns = patterns[:len(patterns)-1]
	}
	return patterns
}

// accessError is an error opening or reading an input file, the messages of
// which -s suppresses.
type accessError struct {
	err error
}

func (e *accessError) Error() string { return e.err.Error() }
func (e *accessError) Unwrap() error { return e.err }

// matcher reports whether a line is selected by any of the patterns.
type matcher struct {
	regexps []*regexp.Regexp
	invert  bool
}

// newMatcher compiles the patterns as the options ask for.
func newMatcher(patterns []string, opts options) (*matcher, error) {
	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		var translated string
		var err error
		switch opts.mode {
		case 'F':
			translated = regexp.QuoteMeta(pattern)
		case 'E':
			translated, err = ere.Translate(pattern)
		default:
			translated, err = bre.Translate(pattern)
		}
		if err != nil {
			return nil, err
		}
		if opts.whole {
			translated = `\A(?:` + translated + `)\z`
		}
		if opts.ignoreCase {
			translated = `(?i)` + translated
		}
		re, err := regexp.Compile(translated)
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression: %v", err)
		}
		regexps = append(regexps, re)
	}
	return &matcher{regexps: regexps, invert: opts.invert}, nil
}

// matches reports whether line is selected.
func (m *matcher) matches(line []byte) bool {
	matched := false
	for _, re := range m.regexps {
		if re.Match(line) {
			matched = true
			break
		}
	}
	if m.invert {
		return !matched
	}
	return matched
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "grep: "+format+"\n", args...)
}

type options struct {
	mode         byte // 'B', 'E' or 'F'
	output       byte // 'o' lines, 'c' count, 'l' file names or 'q' quiet
	ignoreCase   bool
	lineNum      bool
	suppress     bool
	invert       bool
	whole        bool
	patternLists []string
	patternFiles []string
}

// optSet describes the options of grep.
var optSet = getopt.Set{
	Valued: "ef",
	Long: map[string]rune{
		"count":              'c',
		"extended-regexp":    'E',
		"file":               'f',
		"files-with-matches": 'l',
		"fixed-strings":      'F',
		"ignore-case":        'i',
		"line-number":        'n',
		"line-regexp":        'x',
		"no-messages":        's',
		"quiet":              'q',
		"regexp":             'e',
		"revert-match":       'v',
		"silent":             'q',
	},
}

// usage describes the options of grep.
const usage = `Usage: grep [OPTION]... PATTERNS [FILE]...
Search for PATTERNS in each FILE, or the standard input when no file is
given. PATTERNS is one or more patterns separated by newline characters,
and grep prints each line that matches a pattern.

  -E, --extended-regexp
         PATTERNS are extended regular expressions
  -F, --fixed-strings
         PATTERNS are strings
  -c, --count
         print only a count of selected lines
  -e, --regexp=PATTERNS
         use PATTERNS for matching
  -f, --file=FILE
         take PATTERNS from FILE
  -i, --ignore-case
         ignore case distinctions
  -l, --files-with-matches
         print only the names of files with selected lines
  -n, --line-number
         print line number with output lines
  -q, --quiet, --silent
         suppress all output
  -s, --no-messages
         suppress error messages about nonexistent or unreadable files
  -v, --revert-match
         select non-matching lines
  -x, --line-regexp
         match only whole lines
      --help
         display this help and exit
      --version
         output version information and exit
`

func parseFlags(args []string) (options, []string, error) {
	opts := options{mode: 'B', output: 'o'}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'E':
			opts.mode = 'E'
		case 'F':
			opts.mode = 'F'
		case 'c':
			opts.output = 'c'
		case 'e':
			opts.patternLists = append(opts.patternLists, value)
		case 'f':
			opts.patternFiles = append(opts.patternFiles, value)
		case 'i':
			opts.ignoreCase = true
		case 'l':
			opts.output = 'l'
		case 'n':
			opts.lineNum = true
		case 'q':
			opts.output = 'q'
		case 's':
			opts.suppress = true
		case 'v':
			opts.invert = true
		case 'x':
			opts.whole = true
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

var _ coreutils.Program = (*Prog)(nil)
