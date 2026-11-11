// Package grep implements the POSIX grep utility with its GNU extensions.
package grep

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/walk"
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
	if err := p.readSelectors(ctx, &opts); err != nil {
		return err
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
	col := p.colors(opts)
	if opts.dirs == 'R' {
		return p.runRecursive(ctx, m, opts, files, col)
	}
	selected, failed, err := p.runInputs(ctx, m, opts, files, col)
	if err != nil {
		return err
	}
	if opts.output == 'q' && selected {
		return nil
	}
	return exitStatus(selected, failed)
}

// runInputs searches the given files, or the standard input when there are
// none, without descending into directories.
func (p *Prog) runInputs(ctx context.Context, m *matcher, opts options, files []string, col *colors) (selected, failed bool, err error) {
	inputs := input.List(files)
	multiple := opts.filename == 1 || opts.filename == 0 && len(inputs) > 1
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return selected, failed, err
		}
		if !in.Stdin && (opts.dirs == 's' && p.isDirOperand(opts, in.Name) || !opts.keep(in.Name, 0)) {
			continue
		}
		si := searchInput{name: in.Name, stdin: in.Stdin}
		if !in.Stdin {
			si.fsName = slashPath(p, in.Name)
		}
		matched, err := p.searchFile(ctx, m, si, opts, multiple, col)
		if err != nil {
			if ctx.Err() != nil {
				return selected, failed, err
			}
			p.reportAccess(fileLabel(in, opts), err, opts)
			failed = true
		}
		if matched {
			selected = true
		}
		if opts.output == 'q' && selected {
			return selected, failed, nil
		}
	}
	return selected, failed, nil
}

// exitStatus returns the error standing for the exit status of a search.
func exitStatus(selected, failed bool) error {
	switch {
	case failed:
		return &coreutils.ExitError{Code: 2}
	case !selected:
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// reportAccess reports an error of opening or reading an input, which -s
// suppresses.
func (p *Prog) reportAccess(label string, err error, opts options) {
	if accessErr, ok := errors.AsType[*accessError](err); ok {
		err = patherr.Unwrap(accessErr.err)
	}
	if !opts.suppress {
		p.errorf("%s: %v", label, err)
	}
}

// readSelectors reads the patterns of the --exclude-from options, which are
// skipped like the files they come from.
func (p *Prog) readSelectors(ctx context.Context, opts *options) error {
	for _, name := range opts.excludeFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := p.readPatternFile(ctx, name)
		if err != nil {
			return err
		}
		for pattern := range strings.SplitSeq(string(data), "\n") {
			if pattern != "" && !strings.HasPrefix(pattern, "#") {
				opts.addSelector(pattern, false)
			}
		}
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

// fileLabel returns the name to use for an input on the output, honoring
// the label of the standard input.
func fileLabel(in input.Input, opts options) string {
	if in.Stdin {
		if opts.label != "" {
			return opts.label
		}
		return "(standard input)"
	}
	return in.Name
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "grep: "+format+"\n", args...)
}

// isDirOperand reports whether a file operand names a directory. The
// symbolic links are followed as the options ask for.
func (p *Prog) isDirOperand(opts options, name string) bool {
	if p.FS == nil {
		return false
	}
	fi, err := walk.Stat(p.FS, opts.follow, slashPath(p, name), true)
	return err == nil && fi.IsDir()
}

// slashPath returns the name the file system is asked for a file operand.
func slashPath(p *Prog, name string) string {
	if p.FS == nil {
		return name
	}
	return p.FsName(name)
}

var _ coreutils.Program = (*Prog)(nil)
