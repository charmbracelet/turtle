// Package ln implements the POSIX ln utility.
//
// Hard links are created by default, and symbolic links with -s. The -L and
// -P options choose what a symbolic link source is linked as when creating a
// hard link: -P, the default, links the link itself, as on systems whose
// link(2) doesn't follow symbolic links. On systems whose link(2) does follow
// them, such as macOS, -P behaves like -L.
package ln

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
)

// Prog is the ln core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run links each source file to the destination named by the last operand: an
// existing directory to create the links into, or the new name of the single
// source.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	switch len(operands) {
	case 0:
		p.errorf("missing file operand")
		return &coreutils.ExitError{Code: 2}
	case 1:
		p.errorf("missing destination file operand after '%s'", operands[0])
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if slices.Contains(operands, "") {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	sources, target := operands[:len(operands)-1], operands[len(operands)-1]

	dst := p.resolve(target)
	fi, err := fs.Stat(p.FS, dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		p.errorf("cannot stat '%s': %v", target, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	targetIsDir := err == nil && fi.IsDir()
	slash := strings.HasSuffix(filepath.ToSlash(target), "/")
	if !targetIsDir && (len(sources) > 1 || slash) {
		p.errorf("target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := pair{src: p.resolve(source), srcName: source, dst: dst, dstName: target}
		if targetIsDir {
			base := baseName(source)
			e.dst = path.Join(dst, base)
			e.dstName = path.Join(target, base)
		}
		if err := p.link(e, opts); err != nil {
			p.errorf("%v", err)
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// link creates a single link, after the checks and the removal of an existing
// destination that -f allows. An error is returned ready to be reported.
func (p *Prog) link(e pair, opts options) error {
	fi, err := p.FS.Lstat(e.dst)
	switch {
	case err == nil && e.src == e.dst:
		return fmt.Errorf("'%s' and '%s' are the same file", e.srcName, e.dstName)
	case err == nil && fi.IsDir():
		return fmt.Errorf("cannot create %s '%s': is a directory", opts.kind(), e.dstName)
	case err == nil && !opts.force:
		return fmt.Errorf("cannot create %s '%s': file exists", opts.kind(), e.dstName)
	case err == nil:
		if err := p.FS.Remove(e.dst); err != nil {
			return fmt.Errorf("cannot remove '%s': %v", e.dstName, patherr.Unwrap(err))
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, patherr.Unwrap(err))
	}

	if opts.symbolic {
		if err := p.FS.Symlink(filepath.ToSlash(e.srcName), e.dst); err != nil {
			return fmt.Errorf("cannot create symbolic link '%s' to '%s': %v", e.dstName, e.srcName, unwrapLinkError(err))
		}
		return nil
	}

	src := e.src
	if opts.logical {
		src, err = p.evalSymlinks(e.src)
		if err != nil {
			return fmt.Errorf("cannot stat '%s': %v", e.srcName, err)
		}
	}
	if err := p.FS.Link(src, e.dst); err != nil {
		return fmt.Errorf("cannot create link '%s' to '%s': %v", e.dstName, e.srcName, unwrapLinkError(err))
	}
	return nil
}

// evalSymlinks resolves name to the file it names, following symbolic links,
// for -L, which hard links the file a link refers to instead of the link.
func (p *Prog) evalSymlinks(name string) (string, error) {
	for range maxSymlinks {
		fi, err := p.FS.Lstat(name)
		if err != nil {
			return "", patherr.Unwrap(err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return name, nil
		}
		target, err := p.FS.ReadLink(name)
		if err != nil {
			return "", patherr.Unwrap(err)
		}
		if path.IsAbs(target) || filepath.IsAbs(target) {
			name = filepath.ToSlash(target)
		} else {
			name = path.Join(path.Dir(name), filepath.ToSlash(target))
		}
	}
	return "", errLoop
}

// pair holds a source and its destination, both as the path used with the
// file system and as the name shown in diagnostics. srcName keeps the source
// as typed, since -s stores it in the link.
type pair struct {
	src, dst         string
	srcName, dstName string
}

// options holds the ln flags. logical is the -L option; -P is the default, and
// the last of the two wins. Both are ignored with -s.
type options struct {
	symbolic bool
	force    bool
	logical  bool
}

// kind names what the options create, for diagnostics.
func (o options) kind() string {
	if o.symbolic {
		return "symbolic link"
	}
	return "link"
}

// longOptions maps the long options of ln to the short ones they stand for.
var longOptions = map[string]rune{
	"force":    'f',
	"logical":  'L',
	"physical": 'P',
	"symbolic": 's',
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'f':
			opts.force = true
		case 's':
			opts.symbolic = true
		case 'L':
			opts.logical = true
		case 'P':
			opts.logical = false
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

// resolve converts an operand into the slash-separated path used with the
// file system, relative to the working directory.
func (p *Prog) resolve(name string) string {
	return path.Clean(filepath.ToSlash(p.ResolvePath(name)))
}

// baseName returns the last component of an operand as typed, so a trailing
// slash is ignored.
func baseName(name string) string {
	return path.Base(filepath.ToSlash(name))
}

// maxSymlinks is the number of symbolic links that evalSymlinks follows before
// reporting a loop.
const maxSymlinks = 40

// errLoop is returned when following symbolic links doesn't reach an entry.
var errLoop = errors.New("too many levels of symbolic links")

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "ln: "+format+"\n", args...)
}

// unwrapLinkError unwraps the errors of a link creation, which *os.LinkError
// wraps with both names instead.
func unwrapLinkError(err error) error {
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return linkErr.Err
	}
	return patherr.Unwrap(err)
}
