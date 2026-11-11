package grep

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/walk"
)

// errQuit stops the recursion once -q has found a match.
var errQuit = errors.New("quit")

// runRecursive searches the given files and the directories below them.
// Without a file operand, the working directory is searched.
func (p *Prog) runRecursive(ctx context.Context, m *matcher, opts options, files []string, col *colors) error {
	implicit := len(files) == 0
	if implicit {
		files = []string{"."}
	}
	// The name of an input prefixes the output lines with more than one
	// starting point, or any directory one.
	multiple := opts.filename == 1
	if opts.filename == 0 && len(files) > 1 {
		multiple = true
	}
	var selected, failed bool
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "-" {
			// The standard input is searched as it is, without recursing.
			matched, err := p.searchFile(ctx, m, searchInput{name: name, stdin: true}, opts, multiple, col)
			if err != nil {
				if ctx.Err() != nil {
					return err
				}
				p.reportAccess("(standard input)", err, opts)
				failed = true
			}
			selected = selected || matched
			continue
		}
		if !multiple && opts.filename == 0 {
			multiple = p.isDirOperand(opts, name)
		}
		err := p.walkOperand(ctx, m, opts, name, implicit, multiple, col, &selected, &failed)
		if errors.Is(err, errQuit) {
			return nil
		}
		if err != nil {
			return err
		}
		if opts.output == 'q' && selected {
			return nil
		}
	}
	return exitStatus(selected, failed)
}

// walkOperand searches one starting point and the files below it. When
// the starting point is the implicit working directory, the names of the
// files below it are given without its "./" prefix.
func (p *Prog) walkOperand(ctx context.Context, m *matcher, opts options, name string, implicit, multiple bool, col *colors, selected, failed *bool) error {
	trim := func(name string) string {
		if implicit {
			return strings.TrimPrefix(name, "./")
		}
		return name
	}
	err := walk.Walk(ctx, p.FS, walk.Options{
		Follow:   opts.follow,
		MaxDepth: -1,
		OnError: func(name string, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !opts.suppress {
				stdopts.Errorf(p.Runtime, "grep", "%s: %v", trim(name), patherr.Unwrap(err))
			}
			*failed = true
			return nil
		},
	}, name, slashPath(p, name), func(name, fsName string, fi fs.FileInfo, depth int) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if fi.IsDir() {
			return !opts.skipDir(name, depth), nil
		}
		if depth > 0 && opts.follow == 'H' && fi.Mode()&fs.ModeSymlink != 0 {
			// The symbolic links met below the command line ones are
			// skipped, not searched.
			return true, nil
		}
		if opts.skipDevice(fi, depth) {
			return true, nil
		}
		if !opts.keep(name, depth) {
			return true, nil
		}
		name = trim(name)
		matched, err := p.searchFile(ctx, m, searchInput{name: name, fsName: fsName}, opts, multiple, col)
		if err != nil {
			if ctx.Err() != nil {
				return false, err
			}
			p.reportAccess(name, err, opts)
			*failed = true
		}
		if matched {
			*selected = true
		}
		if opts.output == 'q' && *selected {
			return false, errQuit
		}
		return true, nil
	})
	return err
}
