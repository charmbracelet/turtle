// Package sed implements the POSIX sed utility.
//
// The script is parsed into a flat list of commands, where a block of
// commands is delimited by a `{` command holding the index of the command
// following its matching `}`. Regular expressions are basic regular
// expressions by default, and extended ones when -E is given. Backreferences
// in patterns, collating symbols and equivalence classes are not supported.
package sed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
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
// given by the operands and the -e and -f options.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}

	script, files, err := p.assembleScript(opts, operands)
	if err != nil {
		p.errorf("%v", err)
		if errors.Is(err, errMissingScript) {
			return &coreutils.ExitError{Code: 2}
		}
		return &coreutils.ExitError{Code: 1}
	}

	cmds, err := parseScript(script, opts.extended)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}

	if err := p.run(ctx, cmds, files, opts.quiet || strings.HasPrefix(script, "#n")); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
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

type options struct {
	quiet    bool
	extended bool
	parts    []scriptPart
}

func parseFlags(args []string) (*options, []string, error) {
	opts := &options{}
	operands, err := (getopt.Set{Valued: "ef"}).Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'n':
			opts.quiet = true
		case 'E':
			opts.extended = true
		case 'e':
			opts.parts = append(opts.parts, scriptPart{text: value})
		case 'f':
			opts.parts = append(opts.parts, scriptPart{file: value, fromFile: true})
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

// assembleScript builds the script of editing commands from the -e and -f
// options and, when neither was given, from the first operand, which then
// isn't a file. The remaining operands name the input files.
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
			content, err := p.readFile(part.file)
			if err != nil {
				return "", nil, fmt.Errorf("%s: %v", part.file, unwrapPathError(err))
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

func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var _ coreutils.Program = (*Prog)(nil)
