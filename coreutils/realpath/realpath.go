// Package realpath implements the GNU realpath utility.
//
// It canonicalizes each operand to a name with no symbolic links, "." or
// ".." components, or repeated separators. The resolution goes through the
// runtime file system, so in file systems without an absolute root, like a
// memory one, the resolved names are relative to the file system root.
package realpath

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// The runes which --relative-to and --relative-base stand for, kept well
// above the printable range of short options.
const (
	optRelativeTo rune = 0x100 + iota
	optRelativeBase
)

// Mode tells which components of a name Canonicalize requires to exist.
type Mode int

const (
	// AllButLast requires every component but the last to exist, the
	// default of realpath.
	AllButLast Mode = iota
	// Existing requires every component to exist, as -e does.
	Existing
	// Missing requires no component to exist, treating the missing ones
	// as directories, as -m does.
	Missing
)

// Options holds the canonicalization mode of Canonicalize.
type Options struct {
	Mode Mode
}

// Canonicalize resolves name, relative to the root of the file system,
// to a name with no symbolic links, "." or ".." components, or repeated
// separators, following the symbolic links it holds. The name is used as
// given, without being joined to any working directory. It fails when a
// component the mode requires to exist doesn't, or when the symbolic links
// loop.
func Canonicalize(ctx context.Context, fsys fsx.FS, name string, opts Options) (string, error) {
	return canonicalizeOnce(ctx, fsys, name, existMode(opts.Mode), false, false)
}

// Prog is the realpath core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run prints the canonicalized name of each operand to the standard output,
// one per line, exiting with status 1 when any of them cannot be resolved.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "realpath", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		stdopts.Errorf(p.Runtime, "realpath", "%v", err)
		stdopts.TryHelp(p.Runtime, "realpath")
		return &coreutils.ExitError{Code: 1}
	}
	switch {
	case len(operands) == 0:
		stdopts.Errorf(p.Runtime, "realpath", "missing file operand")
		stdopts.TryHelp(p.Runtime, "realpath")
		return &coreutils.ExitError{Code: 1}
	case slices.Contains(operands, ""):
		stdopts.Errorf(p.Runtime, "realpath", "invalid zero-length file name")
		stdopts.TryHelp(p.Runtime, "realpath")
		return &coreutils.ExitError{Code: 1}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "realpath", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}

	delims := "\n"
	if opts.zero {
		delims = "\x00"
	}

	relTo, relBase, err := p.relativeDirs(ctx, opts)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		stdopts.Errorf(p.Runtime, "realpath", "%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}

	ok := true
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := p.canonicalize(ctx, p.FsAbs(name), opts)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if opts.verbose {
				stdopts.Errorf(p.Runtime, "realpath", "%s: %v", name, patherr.Unwrap(err))
			}
			ok = false
			continue
		}
		out = p.relative(out, relTo, relBase)
		if _, err := io.WriteString(p.Stdout, out+delims); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// canonicalize resolves a name according to the options: without expanding
// symbolic links when they are stripped, and twice, with the ".." of the
// given name processed before them, when logical resolution is asked.
func (p *Prog) canonicalize(ctx context.Context, name string, opts options) (string, error) {
	if opts.logical {
		// The first pass leaves only the logical name, whose ".." are
		// already applied; the second one resolves its symbolic links.
		out, err := canonicalizeOnce(ctx, p.FS, name, opts.exist, true, true)
		if err != nil {
			return "", err
		}
		return canonicalizeOnce(ctx, p.FS, out, opts.exist, false, false)
	}
	return canonicalizeOnce(ctx, p.FS, name, opts.exist, opts.noLinks, false)
}

// canonicalizeOnce resolves a name component by component from the root of
// the file system, following symbolic links by splicing their targets into
// the remaining name, and processing "." and ".." against the path resolved
// so far, as the physical mode of the GNU utility does. When noLinks is set
// the symbolic links are kept as they are, and when deferChecks is set the
// missing components are left for a later pass to check.
func canonicalizeOnce(ctx context.Context, fsys fsx.FS, name string, exist existMode, noLinks, deferChecks bool) (string, error) {
	var root, rest string
	if strings.HasPrefix(name, "/") {
		root, rest = "/", name[1:]
	} else {
		rest = name
	}
	resolved := root
	links := 0
	for rest != "" {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if strings.HasPrefix(rest, "/") {
			rest = strings.TrimLeft(rest, "/")
			continue
		}
		comp := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			comp, rest = rest[:i], rest[i:]
		} else {
			rest = ""
		}
		switch comp {
		case ".", "":
			continue
		case "..":
			resolved = popLast(resolved)
			continue
		}
		cur := joinComp(resolved, comp)
		if noLinks && exist == existNone {
			// -s with -m operates only on the file name, without
			// touching any actual file.
			resolved = cur
			continue
		}
		st, err := fsys.Lstat(cur)
		if err != nil {
			if !tolerateMissing(err, rest, exist, deferChecks) {
				return "", patherr.Unwrap(err)
			}
			resolved = cur
			continue
		}
		if st.Mode()&fs.ModeSymlink != 0 && !noLinks {
			target, err := fsys.ReadLink(cur)
			if err != nil {
				return "", patherr.Unwrap(err)
			}
			if links++; links > maxSymlinks {
				// As GNU does with -m, a loop is only an error when
				// the links are expanded; the component is left as it
				// is otherwise.
				if exist == existNone {
					resolved = cur
					continue
				}
				return "", errLoop
			}
			if strings.HasPrefix(target, "/") {
				resolved, rest = "/", target[1:]+rest
			} else {
				rest = target + rest
			}
			continue
		}
		if err := checkDir(fsys, st, cur, rest, exist); err != nil {
			return "", err
		}
		resolved = cur
	}
	if resolved == "" {
		resolved = "."
	}
	return resolved, nil
}

// tolerateMissing reports whether the failure to look up a component is
// acceptable: with no existence requirement, or deferred to a later pass, or
// on the last component when only it may be missing. The other failures are
// deferred while components still follow, as the lookups of the next ones
// fail the same way.
func tolerateMissing(err error, rest string, exist existMode, deferChecks bool) bool {
	if !errors.Is(err, fs.ErrNotExist) {
		return rest != ""
	}
	if deferChecks || exist == existNone {
		return true
	}
	return exist == existAllButLast && onlySlashes(rest)
}

// checkDir reports whether an entry which exists supports the components
// that follow it: when the remaining name holds only separators, "." and
// ".." components, the entry must resolve to a directory, following its own
// symbolic link.
func checkDir(fsys fsx.FS, st fs.FileInfo, cur, rest string, exist existMode) error {
	if !needsDirCheck(rest) || st.IsDir() {
		return nil
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		fi, err := fs.Stat(fsys, cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) &&
				(exist == existNone || exist == existAllButLast && onlySlashes(rest)) {
				return nil
			}
			return patherr.Unwrap(err)
		}
		if fi.IsDir() {
			return nil
		}
	}
	return errNotDir
}

// relativeDirs canonicalizes the --relative-to and --relative-base
// directories with the options of the operands, checking that they are
// directories when -e asks for existing files. When both are given and the
// base is not a prefix of the target, the names are printed absolute.
func (p *Prog) relativeDirs(ctx context.Context, opts options) (relTo, relBase string, err error) {
	if opts.relTo == "" && opts.relBase == "" {
		return "", "", nil
	}
	if opts.relTo == "" {
		relBase, err = p.canonicalizeDir(ctx, opts.relBase, opts)
		return relBase, relBase, err
	}
	if relTo, err = p.canonicalizeDir(ctx, opts.relTo, opts); err != nil {
		return "", "", err
	}
	if opts.relBase != "" {
		if relBase, err = p.canonicalizeDir(ctx, opts.relBase, opts); err != nil {
			return "", "", err
		}
		if !pathPrefix(relBase, relTo) {
			relTo = ""
		}
	}
	return relTo, relBase, nil
}

// canonicalizeDir resolves a relative option directory, requiring it to be a
// directory when -e asks for existing files.
func (p *Prog) canonicalizeDir(ctx context.Context, name string, opts options) (string, error) {
	dir, err := p.canonicalize(ctx, p.FsAbs(name), opts)
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, patherr.Unwrap(err))
	}
	if opts.exist != existAll {
		return dir, nil
	}
	fi, err := fs.Stat(p.FS, dir)
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, patherr.Unwrap(err))
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%s: %v", name, errNotDir)
	}
	return dir, nil
}

// relative computes the name of the resolved file relative to the
// --relative-to directory, reporting whether it applies: only when the
// --relative-base directory holds it, when one is given.
func (p *Prog) relative(out, relTo, relBase string) string {
	if relTo == "" || relBase != "" && !pathPrefix(relBase, out) {
		return out
	}
	return relFrom(relTo, out)
}

// relFrom returns name relative to base, as two clean slash paths: the
// common prefix is dropped, and the components of base beyond it become
// ".." steps. A root base leaves the name as it is, since it already is
// relative to the root.
func relFrom(base, name string) string {
	baseComps := components(base)
	nameComps := components(name)
	i := 0
	for i < len(baseComps) && i < len(nameComps) && baseComps[i] == nameComps[i] {
		i++
	}
	var rel []string
	for range baseComps[i:] {
		rel = append(rel, "..")
	}
	rel = append(rel, nameComps[i:]...)
	if len(rel) == 0 {
		return "."
	}
	return strings.Join(rel, "/")
}

// existMode tells which components of a name must exist.
type existMode int

const (
	existAllButLast existMode = iota
	existAll
	existNone
)

// options holds the flags of realpath. noLinks is the current symbolic link
// handling, which -L and -s set and -P clears, and logical asks for a second
// resolution pass which expands the links of -L.
type options struct {
	exist   existMode
	noLinks bool
	logical bool
	verbose bool
	zero    bool
	relTo   string
	relBase string
}

// longOptions maps the long options of realpath to the short ones they stand
// for.
var longOptions = map[string]rune{
	"canonicalize":          'E',
	"canonicalize-existing": 'e',
	"canonicalize-missing":  'm',
	"logical":               'L',
	"physical":              'P',
	"quiet":                 'q',
	"strip":                 's',
	"no-symlinks":           's',
	"zero":                  'z',
	"relative-to":           optRelativeTo,
	"relative-base":         optRelativeBase,
}

// optSet describes the options of realpath. The relative options are long
// only, and take their argument attached or in the next argument.
var optSet = getopt.Set{
	Valued: string(optRelativeTo) + string(optRelativeBase),
	Long:   longOptions,
}

// usage describes the options of realpath.
const usage = `Usage: realpath [OPTION]... FILE...
Print the resolved absolute file name;
all but the last component must exist.

  -E, --canonicalize
         all but the last component of the file names must exist; this is
         the default
  -e, --canonicalize-existing
         all components of the file names must exist
  -m, --canonicalize-missing
         no components of the file names need exist
  -L, --logical
         resolve '..' components before symbolic links
  -P, --physical
         resolve symbolic links as encountered (default)
  -q, --quiet
         suppress diagnostics for unresolvable operands
  --relative-to=DIR
         print the resolved file names relative to DIR
  --relative-base=DIR
         print absolute file names unless they are below DIR
  -s, --strip
         don't expand symbolic links
      --no-symlinks
         don't expand symbolic links
  -z, --zero
         end each output line with NUL, not newline
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{
		exist:   existAllButLast,
		verbose: true,
	}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'E':
			opts.exist = existAllButLast
		case 'e':
			opts.exist = existAll
		case 'm':
			opts.exist = existNone
		case 'L':
			opts.noLinks, opts.logical = true, true
		case 'P':
			opts.noLinks, opts.logical = false, false
		case 's':
			opts.noLinks, opts.logical = true, false
		case 'q':
			opts.verbose = false
		case 'z':
			opts.zero = true
		case optRelativeTo:
			opts.relTo = value
		case optRelativeBase:
			opts.relBase = value
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

// needsDirCheck reports whether the remaining name asks the component just
// processed to be a directory: a remainder of separators, "." and ".."
// components only, as in "dir/", "dir/." or "dir/..".
func needsDirCheck(rest string) bool {
	if rest == "" {
		return false
	}
	for rest != "" {
		if strings.HasPrefix(rest, "/") {
			rest = strings.TrimLeft(rest, "/")
			continue
		}
		comp := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			comp, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		if comp != "." && comp != ".." {
			return false
		}
	}
	return true
}

// onlySlashes reports whether the remaining name holds separators only,
// which makes the component just processed the last one.
func onlySlashes(rest string) bool {
	return strings.Trim(rest, "/") == ""
}

// components splits a slash-separated path into its components.
func components(p string) []string {
	var comps []string
	for comp := range strings.SplitSeq(p, "/") {
		if comp != "" {
			comps = append(comps, comp)
		}
	}
	return comps
}

// joinComp appends a component to the path resolved so far.
func joinComp(resolved, comp string) string {
	switch {
	case resolved == "":
		return comp
	case resolved == "/":
		return "/" + comp
	default:
		return resolved + "/" + comp
	}
}

// popLast removes the last component of the path resolved so far, keeping
// the root when there's none left.
func popLast(resolved string) string {
	switch i := strings.LastIndexByte(resolved, '/'); {
	case i < 0:
		return ""
	case i == 0:
		return "/"
	default:
		return resolved[:i]
	}
}

// pathPrefix reports whether name is dir or a descendant of it. The root
// holds every name, including the relative ones of file systems without an
// absolute root.
func pathPrefix(dir, name string) bool {
	if dir == "/" {
		return true
	}
	return name == dir || strings.HasPrefix(name, dir+"/")
}

// maxSymlinks is the number of symbolic links the resolution follows before
// reporting a loop.
const maxSymlinks = 40

var (
	errLoop   = errors.New("too many levels of symbolic links")
	errNotDir = errors.New("not a directory")
)

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
