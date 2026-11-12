// Package ln implements the POSIX ln utility.
//
// Hard links are created by default, and symbolic links with -s. The -L and
// -P options choose what a symbolic link source is linked as when creating a
// hard link: -P, the default, links the link itself, as on systems whose
// link(2) doesn't follow symbolic links. On systems whose link(2) does follow
// them, such as macOS, -P behaves like -L.
//
// The GNU options are supported as well: -t and -T control how a directory
// destination is treated, -n treats a symbolic link to a directory as the
// link itself, and -v names each created link on standard output.
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
	"charm.land/snail/coreutils/internal/fsop"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the ln core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run links each source file to the destination: the -t directory if given,
// or an existing directory named by the last operand, or else the new name of
// the single source. -T makes the last operand always a new name, a lone
// source is linked into the current directory, and -n keeps a symbolic link
// destination from being treated as the directory it points to.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "ln", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "ln", err)
	}
	switch {
	case len(operands) == 0:
		stdopts.Errorf(p.Runtime, "ln", "missing file operand")
		stdopts.TryHelp(p.Runtime, "ln")
		return &coreutils.ExitError{Code: 2}
	case opts.noTarget && opts.targetDir != "":
		stdopts.Errorf(p.Runtime, "ln", "cannot combine --target-directory and --no-target-directory")
		stdopts.TryHelp(p.Runtime, "ln")
		return &coreutils.ExitError{Code: 2}
	case opts.noTarget && len(operands) < 2:
		stdopts.Errorf(p.Runtime, "ln", "missing destination file operand after '%s'", operands[0])
		stdopts.TryHelp(p.Runtime, "ln")
		return &coreutils.ExitError{Code: 2}
	case opts.noTarget && len(operands) > 2:
		stdopts.Errorf(p.Runtime, "ln", "extra operand '%s'", operands[2])
		stdopts.TryHelp(p.Runtime, "ln")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "ln", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if slices.Contains(operands, "") {
		stdopts.Errorf(p.Runtime, "ln", "invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	sources, target := destination(opts, operands)

	dst := p.FsPath(target)
	var targetIsDir bool
	switch {
	case opts.noTarget:
		// The last operand names a new file, whatever it is.
	case opts.targetDir != "":
		fi, err := fs.Stat(p.FS, dst)
		if err != nil {
			stdopts.Errorf(p.Runtime, "ln", "failed to access '%s': %v", target, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		if !fi.IsDir() {
			stdopts.Errorf(p.Runtime, "ln", "target '%s' is not a directory", target)
			return &coreutils.ExitError{Code: 1}
		}
		targetIsDir = true
	default:
		fi, err := p.targetStat(opts, dst)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			stdopts.Errorf(p.Runtime, "ln", "cannot stat '%s': %v", target, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		targetIsDir = err == nil && fi.IsDir()
	}
	slash := strings.HasSuffix(filepath.ToSlash(target), "/")
	if !targetIsDir && (len(sources) > 1 || slash) {
		stdopts.Errorf(p.Runtime, "ln", "target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := fsop.Pair{Src: p.FsPath(source), SrcName: source, Dst: dst, DstName: target}
		if targetIsDir {
			base := coreutils.FsBase(source)
			e.Dst = path.Join(dst, base)
			e.DstName = path.Join(target, base)
		}
		if err := p.link(e, opts); err != nil {
			stdopts.Errorf(p.Runtime, "ln", "%v", err)
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
func (p *Prog) link(e fsop.Pair, opts options) error {
	fi, err := p.FS.Lstat(e.Dst)
	switch {
	case err == nil && e.Src == e.Dst:
		return fmt.Errorf("'%s' and '%s' are the same file", e.SrcName, e.DstName)
	case err == nil && fi.IsDir():
		return fmt.Errorf("cannot create %s '%s': is a directory", opts.kind(), e.DstName)
	case err == nil && !opts.force:
		return fmt.Errorf("cannot create %s '%s': file exists", opts.kind(), e.DstName)
	case err == nil:
		if err := p.FS.Remove(e.Dst); err != nil {
			return fmt.Errorf("cannot remove '%s': %v", e.DstName, patherr.Unwrap(err))
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.DstName, patherr.Unwrap(err))
	}

	if opts.symbolic {
		if err := p.FS.Symlink(filepath.ToSlash(e.SrcName), e.Dst); err != nil {
			return fmt.Errorf("cannot create symbolic link '%s' to '%s': %v", e.DstName, e.SrcName, unwrapLinkError(err))
		}
		p.reportLink(e, opts)
		return nil
	}

	src := e.Src
	if opts.logical {
		src, err = p.evalSymlinks(e.Src)
		if err != nil {
			return fmt.Errorf("cannot stat '%s': %v", e.SrcName, err)
		}
	}
	if err := p.FS.Link(src, e.Dst); err != nil {
		return fmt.Errorf("cannot create link '%s' to '%s': %v", e.DstName, e.SrcName, unwrapLinkError(err))
	}
	p.reportLink(e, opts)
	return nil
}

// reportLink names the created link on standard output, as -v asks, with ->
// for a symbolic link and => for a hard link.
func (p *Prog) reportLink(e fsop.Pair, opts options) {
	if !opts.verbose {
		return
	}
	arrow := "=>"
	if opts.symbolic {
		arrow = "->"
	}
	fmt.Fprintf(p.Stdout, "'%s' %s '%s'\n", e.DstName, arrow, e.SrcName)
}

// destination splits the operands into sources and the destination target:
// the -t directory if given, the last operand when there is more than one,
// or the current directory for a lone source, as GNU's ln does.
func destination(opts options, operands []string) ([]string, string) {
	switch {
	case opts.targetDir != "":
		return operands, opts.targetDir
	case opts.noTarget || len(operands) > 1:
		return operands[:len(operands)-1], operands[len(operands)-1]
	default:
		return operands, "."
	}
}

// targetStat describes the destination, following a final symbolic link
// unless -n keeps it from being treated as the directory it points to.
func (p *Prog) targetStat(opts options, dst string) (fs.FileInfo, error) {
	if opts.noDeref {
		return p.FS.Lstat(dst)
	}
	return fs.Stat(p.FS, dst)
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

// options holds the ln flags. logical is the -L option; -P is the default, and
// the last of the two wins. Both are ignored with -s. -n only keeps the last
// operand from being followed when it's a symbolic link to a directory, which
// makes it weaker than -T, which always makes the last operand a new name.
type options struct {
	symbolic  bool
	force     bool
	logical   bool
	noDeref   bool
	noTarget  bool
	targetDir string
	verbose   bool
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
	"force":               'f',
	"logical":             'L',
	"no-dereference":      'n',
	"no-target-directory": 'T',
	"physical":            'P',
	"symbolic":            's',
	"target-directory":    't',
	"verbose":             'v',
}

// optSet describes the options of ln.
var optSet = getopt.Set{Valued: "t", Long: longOptions}

// usage describes the options of ln.
const usage = `Usage: ln [OPTION]... [-T] TARGET LINK_NAME
  or:  ln [OPTION]... TARGET
  or:  ln [OPTION]... TARGET... DIRECTORY
  or:  ln [OPTION]... -t DIRECTORY TARGET...
Link TARGET to LINK_NAME, or link TARGET(s) to DIRECTORY.

  -f, --force
         remove existing destination files
  -L, --logical
         link to the file a symbolic link points to
  -n, --no-dereference
         treat a destination symbolic link to a directory as if it were a normal
         file
  -P, --physical
         link to the symbolic link itself
  -s, --symbolic
         make symbolic links instead of hard links
  -t, --target-directory=DIRECTORY
         specify the destination directory
  -T, --no-target-directory
         treat the last operand as a normal file
  -v, --verbose
         print the name of each file after linking it
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
			opts.force = true
		case 's':
			opts.symbolic = true
		case 'L':
			opts.logical = true
		case 'P':
			opts.logical = false
		case 'n':
			opts.noDeref = true
		case 'T':
			opts.noTarget = true
		case 't':
			if opts.targetDir != "" {
				return errors.New("multiple target directories specified")
			}
			opts.targetDir = value
		case 'v':
			opts.verbose = true
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

// maxSymlinks is the number of symbolic links that evalSymlinks follows before
// reporting a loop.
const maxSymlinks = 40

// errLoop is returned when following symbolic links doesn't reach an entry.
var errLoop = errors.New("too many levels of symbolic links")

// unwrapLinkError unwraps the errors of a link creation, which *os.LinkError
// wraps with both names instead.
func unwrapLinkError(err error) error {
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return linkErr.Err
	}
	return patherr.Unwrap(err)
}
