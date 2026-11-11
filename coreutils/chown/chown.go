// Package chown implements the POSIX chown utility.
package chown

import (
	"context"
	"errors"
	"io/fs"
	"os/user"
	"path"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the chown core utility.
type Prog struct {
	*coreutils.Runtime

	// lookupUser, lookupGroup, lookupUserID and lookupGroupID resolve names
	// and IDs through the host user database. When nil, the os/user package
	// is used.
	lookupUser    func(string) (*user.User, error)
	lookupGroup   func(string) (*user.Group, error)
	lookupUserID  func(string) (*user.User, error)
	lookupGroupID func(string) (*user.Group, error)
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run changes the owner and group of the given files, as the owner operand
// or the --reference file selects. With -R, directories have their whole
// hierarchy changed.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "chown", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "chown", err)
	}
	if err := p.prepare(&opts); err != nil {
		stdopts.Errorf(p.Runtime, "chown", "%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	switch {
	case len(operands) == 0:
		stdopts.Errorf(p.Runtime, "chown", "missing operand")
		stdopts.TryHelp(p.Runtime, "chown")
		return &coreutils.ExitError{Code: 2}
	case p.FS == nil:
		stdopts.Errorf(p.Runtime, "chown", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	cfs, ok := p.FS.(fsx.ChownFS)
	if !ok {
		stdopts.Errorf(p.Runtime, "chown", "changing ownership is not supported")
		return &coreutils.ExitError{Code: 1}
	}
	if opts.fromSpec != nil {
		from, err := p.parseFrom(*opts.fromSpec)
		if err != nil {
			stdopts.Errorf(p.Runtime, "chown", "%v", err)
			return &coreutils.ExitError{Code: 1}
		}
		opts.from = &from
	}

	target, files, err := p.target(opts, operands)
	if err != nil {
		return err
	}
	if err := p.preserveRoot(opts, files); err != nil {
		return err
	}

	ok = true
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		failed, err := p.chown(ctx, file, target, opts, cfs)
		if err != nil {
			return err
		}
		ok = ok && !failed
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// target builds the ownership a run sets, and returns the files it applies
// to. Without --reference, the first operand is the owner; with it, every
// operand is a file, as chmod treats its mode operand.
func (p *Prog) target(opts options, operands []string) (ownership, []string, error) {
	if opts.reference == nil {
		if len(operands) == 1 {
			stdopts.Errorf(p.Runtime, "chown", "missing file operand")
			stdopts.TryHelp(p.Runtime, "chown")
			return ownership{}, nil, &coreutils.ExitError{Code: 2}
		}
		target, err := p.parseOwner(operands[0])
		if err != nil {
			stdopts.Errorf(p.Runtime, "chown", "%v", err)
			return ownership{}, nil, &coreutils.ExitError{Code: 1}
		}
		return target, operands[1:], nil
	}
	target, err := p.referenceTarget(*opts.reference)
	if err != nil {
		stdopts.Errorf(p.Runtime, "chown", "failed to get attributes of %s: %v", *opts.reference, patherr.Unwrap(err))
		return ownership{}, nil, &coreutils.ExitError{Code: 1}
	}
	return target, operands, nil
}

// referenceTarget returns the owner and group of the reference file, which a
// symbolic link reference doesn't stand for, as GNU asks.
func (p *Prog) referenceTarget(name string) (ownership, error) {
	resolved := p.FsPath(name)
	fi, err := fs.Stat(p.FS, resolved)
	if err != nil {
		return ownership{}, err
	}
	uid, gid, ok := fsx.Owner(fi)
	if !ok {
		return ownership{uid: -1, gid: -1}, nil
	}
	return ownership{
		uid:       uid,
		gid:       gid,
		userSpec:  p.uidName(uid),
		groupSpec: p.gidName(gid),
	}, nil
}

// preserveRoot refuses a recursive run on '/', as --preserve-root asks:
// without -R the option has no effect, as the GNU manual defines. With -H
// and -L, a command line symbolic link pointing at the root stands for it,
// so what it refers to is checked too.
func (p *Prog) preserveRoot(opts options, files []string) error {
	if !opts.recursive || !opts.preserveRoot {
		return nil
	}
	for _, file := range files {
		name := p.FsPath(file)
		if name == "/" {
			return p.refuseRoot()
		}
		if opts.links == linksPhysical {
			continue
		}
		fi, err := p.lstat(name)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		resolved, _, err := p.resolveChain(name)
		if err != nil {
			continue
		}
		// A resolved "." is the root of the file system, as an absolute
		// link target names it, and a resolved "/" the operating system
		// one.
		if resolved == "/" || resolved == "." {
			return p.refuseRoot()
		}
	}
	return nil
}

// refuseRoot reports the failsafe of --preserve-root.
func (p *Prog) refuseRoot() error {
	stdopts.Errorf(p.Runtime, "chown", "it is dangerous to operate recursively on '/'")
	stdopts.Errorf(p.Runtime, "chown", "use --no-preserve-root to override this failsafe")
	return &coreutils.ExitError{Code: 1}
}

// chown changes the ownership of a single operand, reporting a diagnostic
// for every file it fails to change. It reports whether any file failed, and
// only returns an error when the run must be aborted.
func (p *Prog) chown(ctx context.Context, operand string, target ownership, opts options, cfs fsx.ChownFS) (bool, error) {
	root := p.FsPath(operand)
	return p.visit(ctx, operand, root, root, target, opts, true, make(map[string]bool), cfs)
}

// visit chowns the file at name, shown as given in diagnostics, and walks
// the hierarchy below it when the traversal reaches a directory. It reports
// whether any change failed.
func (p *Prog) visit(ctx context.Context, operand, root, name string, target ownership, opts options, operandFirst bool, visited map[string]bool, cfs fsx.ChownFS) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	shown := display(operand, root, name)

	fi, err := p.lstat(name)
	if err != nil {
		if !opts.quiet {
			stdopts.Errorf(p.Runtime, "chown", "cannot access '%s': %v", shown, patherr.Unwrap(err))
		}
		return true, nil
	}

	link := fi.Mode()&fs.ModeSymlink != 0
	if link && p.follow(opts, operandFirst) {
		name, fi, err = p.resolveChain(name)
		if err != nil {
			if !opts.quiet {
				stdopts.Errorf(p.Runtime, "chown", "cannot dereference '%s': %v", shown, patherr.Unwrap(err))
			}
			return true, nil
		}
	}

	traverse := opts.recursive && fi.IsDir()
	if traverse && link && opts.links == linksOperand && !operandFirst {
		traverse = false
	}
	if traverse && visited[name] {
		if !opts.quiet {
			stdopts.Errorf(p.Runtime, "chown", "cycle in directory tree '%s'", shown)
		}
		return true, nil
	}

	failed := false
	if err := p.apply(name, shown, fi, target, opts, cfs); err != nil {
		if !opts.quiet {
			if target.uid >= 0 {
				stdopts.Errorf(p.Runtime, "chown", "changing ownership of '%s': %v", shown, patherr.Unwrap(err))
			} else {
				stdopts.Errorf(p.Runtime, "chown", "changing group of '%s': %v", shown, patherr.Unwrap(err))
			}
		}
		failed = true
	}

	if !traverse {
		return failed, nil
	}
	visited[name] = true
	defer delete(visited, name)

	entries, err := fs.ReadDir(p.FS, name)
	if err != nil {
		if !opts.quiet {
			stdopts.Errorf(p.Runtime, "chown", "cannot read directory '%s': %v", shown, patherr.Unwrap(err))
		}
		return true, nil
	}
	for _, entry := range entries {
		f, err := p.visit(ctx, operand, root, path.Join(name, entry.Name()), target, opts, false, visited, cfs)
		if err != nil {
			return failed, err
		}
		failed = failed || f
	}
	return failed, nil
}

// apply chowns the file described by fi at name, and reports the change when
// the verbosity asks for it.
func (p *Prog) apply(name, shown string, fi fs.FileInfo, target ownership, opts options, cfs fsx.ChownFS) error {
	if fi.Mode()&fs.ModeSymlink != 0 {
		// A symbolic link the run would act on itself is left alone: ChownFS
		// has no link-aware operation, and a Chown which follows links, like
		// the operating system one, would change the referent instead of the
		// link, as chgrp documents too.
		return nil
	}
	oldUid, oldGid, known := fsx.Owner(fi)
	if opts.from != nil && !opts.from.matches(oldUid, oldGid, known) {
		return nil
	}
	if err := cfs.Chown(name, target.uid, target.gid); err != nil && !unsupportedOwnership(err) {
		return err
	}
	if known {
		p.describe(shown, oldUid, oldGid, target, opts.verbosity)
	}
	return nil
}

// follow reports whether the final symbolic link of a name is resolved, to
// operate on the file it refers to. Without it, the link itself is changed.
func (p *Prog) follow(opts options, operandFirst bool) bool {
	if !opts.recursive {
		return opts.deref == derefYes
	}
	if opts.links == linksTree {
		return true
	}
	if opts.links == linksOperand && operandFirst {
		return true
	}
	return opts.deref == derefYes
}

// resolveChain follows the symbolic links at name, returning the path and
// entry of the file the last one refers to. It fails when a link of the
// chain dangles or the chain loops.
func (p *Prog) resolveChain(name string) (string, fs.FileInfo, error) {
	lfs, ok := p.FS.(lstatFS)
	if !ok {
		fi, err := fs.Stat(p.FS, name)
		return name, fi, err
	}
	for range 40 {
		fi, err := lfs.Lstat(name)
		if err != nil {
			return name, nil, err
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return name, fi, nil
		}
		target, err := p.FS.ReadLink(name)
		if err != nil {
			return name, nil, err
		}
		name = linkTarget(name, target)
	}
	return name, nil, &fs.PathError{Op: "lstat", Path: name, Err: errLoop}
}

// linkTarget joins the target of the link at name to its directory. An
// absolute target is relative to the root of the file system, as the memory
// file system resolves it.
func linkTarget(name, target string) string {
	if after, ok := strings.CutPrefix(target, "/"); ok {
		return path.Clean(after)
	}
	return path.Join(path.Dir(name), target)
}

// errLoop reports a chain of symbolic links that doesn't reach an entry.
var errLoop = errors.New("too many levels of symbolic links")

// lstatFS is the Lstat subset that the fsx file systems implement, which
// describes a name without following a final symbolic link.
type lstatFS interface {
	Lstat(name string) (fs.FileInfo, error)
}

// lstat describes the name without following a final symbolic link. A
// file system without Lstat describes what the name refers to.
func (p *Prog) lstat(name string) (fs.FileInfo, error) {
	if lfs, ok := p.FS.(lstatFS); ok {
		return lfs.Lstat(name)
	}
	return fs.Stat(p.FS, name)
}

// display returns the path to show in diagnostics: the operand as given, with
// any sub-path walked below it appended.
func display(operand, resolved, walked string) string {
	if walked == resolved {
		return operand
	}
	if rel, ok := strings.CutPrefix(walked, resolved+"/"); ok {
		return path.Join(operand, rel)
	}
	return path.Join(operand, walked)
}

// The verbosity levels of a run, selected by -c and -v.
const (
	quiet = iota
	changesOnly
	noisy
)

// How a run treats a symbolic link, selected by -h and --dereference.
const (
	derefUnset = iota
	derefNo
	derefYes
)

// The traversal policies of -R, selected by -H, -L and -P. The last of them
// given wins, as POSIX asks.
type links int

const (
	linksPhysical links = iota
	linksOperand
	linksTree
)

// The runes which --from, --reference, --dereference and the preserve-root
// options map to. They're kept well above the range where a short option can
// reach.
const (
	optFrom rune = 0x100 + iota
	optReference
	optDereference
	optPreserveRoot
	optNoPreserveRoot
)

// longOptions maps the long options of chown to the characters they stand
// for.
var longOptions = map[string]rune{
	"recursive":        'R',
	"changes":          'c',
	"silent":           'f',
	"quiet":            'f',
	"no-dereference":   'h',
	"verbose":          'v',
	"dereference":      optDereference,
	"from":             optFrom,
	"reference":        optReference,
	"preserve-root":    optPreserveRoot,
	"no-preserve-root": optNoPreserveRoot,
}

// optSet describes the options of chown.
var optSet = getopt.Set{
	Valued: string(optFrom) + string(optReference),
	Long:   longOptions,
}

// usage describes the options of chown.
const usage = `Usage: chown [OPTION]... [OWNER][:[GROUP]] FILE...
  or:  chown [OPTION]... --reference=RFILE FILE...
Change the owner and/or group of each FILE to OWNER and/or GROUP.

  -c, --changes
         verbosely describe the action for each file whose ownership actually
         changes
  -f, --silent, --quiet
         suppress most error messages
      --from=CURRENT_OWNER:CURRENT_GROUP
         change the owner and/or group of each file only if its current owner
         and/or group match those specified here
  -h, --no-dereference
         affect symbolic links instead of any referenced file, which are
         left alone, as no file system here can change a link itself
      --no-preserve-root
         do not treat '/' specially, which is the default
      --preserve-root
         fail to operate recursively on '/'
      --dereference
         affect the referent of each symbolic link, rather than the symbolic
         link itself
  -H
         if a command line argument is a symbolic link to a directory,
         traverse it
  -L
         traverse every symbolic link to a directory encountered
  -P
         do not traverse any symbolic links
  -R, --recursive
         operate on files and directories recursively
      --reference=REF_FILE
         use the owner and group of REF_FILE instead of an owner operand
  -v, --verbose
         output a diagnostic for every file processed
` + stdopts.Tail

// options holds what a run of chown selects.
type options struct {
	recursive bool
	// links is the traversal policy, the last of -H, -L and -P given.
	links links
	// deref is how a symbolic link is treated, the last of -h and
	// --dereference given.
	deref     int
	quiet     bool
	verbosity int
	// fromSpec is the --from operand, parsed into from before the run.
	fromSpec *string
	from     *ownership
	// preserveRoot refuses a recursive run on '/', as --preserve-root asks;
	// the default, as GNU defines it for chown, is not to.
	preserveRoot bool
	// reference names the file whose ownership is used instead of an owner
	// operand, or is nil without --reference.
	reference *string
}

// prepare resolves the dereference defaults, and refuses -R --dereference
// without a traversal mode, as GNU does.
func (p *Prog) prepare(opts *options) error {
	if !opts.recursive {
		if opts.deref == derefUnset {
			opts.deref = derefYes
		}
		return nil
	}
	if opts.links != linksPhysical {
		if opts.deref == derefUnset {
			opts.deref = derefYes
		}
		return nil
	}
	switch opts.deref {
	case derefYes:
		return errors.New("-R --dereference requires either -H or -L")
	case derefUnset:
		opts.deref = derefNo
	}
	return nil
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'R':
			opts.recursive = true
		case 'H':
			opts.links = linksOperand
		case 'L':
			opts.links = linksTree
		case 'P':
			opts.links = linksPhysical
		case 'h':
			opts.deref = derefNo
		case optDereference:
			opts.deref = derefYes
		case 'f':
			opts.quiet = true
		case 'c':
			opts.verbosity = changesOnly
		case 'v':
			opts.verbosity = noisy
		case optFrom:
			opts.fromSpec = &value
		case optReference:
			opts.reference = &value
		case optPreserveRoot:
			opts.preserveRoot = true
		case optNoPreserveRoot:
			opts.preserveRoot = false
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
