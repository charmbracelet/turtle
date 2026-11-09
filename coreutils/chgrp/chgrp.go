// Package chgrp implements the POSIX chgrp utility.
package chgrp

import (
	"context"
	"fmt"
	"io/fs"
	"os/user"
	"path"
	"path/filepath"
	"strconv"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the chgrp core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run changes the group of the given files, as specified by the group
// operand, a group name or a numeric group ID, or by the group of a
// reference file. With -R, directories have their whole hierarchy changed.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "chgrp", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "chgrp")
		return &coreutils.ExitError{Code: 2}
	}
	switch {
	case len(operands) == 0:
		p.errorf("missing operand")
		stdopts.TryHelp(p.Runtime, "chgrp")
		return &coreutils.ExitError{Code: 2}
	case p.FS == nil:
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	chowner, ok := p.FS.(fsx.ChownFS)
	if !ok {
		p.errorf("group ownership is not supported by this file system")
		return &coreutils.ExitError{Code: 1}
	}
	gid, files, err := p.group(opts, operands)
	if err != nil {
		return err
	}

	r := &run{p: p, opts: opts, gid: gid, chowner: chowner}
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.file(ctx, name); err != nil {
			return err
		}
	}
	if r.failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// group resolves the group operand of a run, and returns the files it
// applies to. Without --reference, the first operand is the group; with it,
// every operand is a file.
func (p *Prog) group(opts options, operands []string) (int, []string, error) {
	if opts.reference == nil {
		if len(operands) == 1 {
			p.errorf("missing file operand")
			stdopts.TryHelp(p.Runtime, "chgrp")
			return 0, nil, &coreutils.ExitError{Code: 2}
		}
		gid, err := parseGroup(operands[0])
		if err != nil {
			p.errorf("%v", err)
			stdopts.TryHelp(p.Runtime, "chgrp")
			return 0, nil, &coreutils.ExitError{Code: 1}
		}
		return gid, operands[1:], nil
	}
	gid, err := p.referenceGroup(*opts.reference)
	if err != nil {
		p.errorf("%v", err)
		return 0, nil, &coreutils.ExitError{Code: 1}
	}
	return gid, operands, nil
}

// referenceGroup returns the group of the reference file, which a symbolic
// link stands for.
func (p *Prog) referenceGroup(name string) (int, error) {
	resolved := path.Clean(filepath.ToSlash(p.ResolvePath(name)))
	fi, err := fs.Stat(p.FS, resolved)
	if err != nil {
		return 0, fmt.Errorf("failed to get attributes of %s: %v", name, patherr.Unwrap(err))
	}
	_, gid, ok := fsx.Owner(fi)
	if !ok {
		return 0, fmt.Errorf("failed to get attributes of %s: group ownership is not available", name)
	}
	return gid, nil
}

// run holds the state of one run: the options, the group to set, and the
// file system which changes it.
type run struct {
	p       *Prog
	opts    options
	gid     int
	chowner fsx.ChownFS
	failed  bool
}

// file changes the group of one operand, shown as given.
func (r *run) file(ctx context.Context, operand string) error {
	resolved := path.Clean(filepath.ToSlash(r.p.ResolvePath(operand)))
	fi, err := r.p.FS.Lstat(resolved)
	if err != nil {
		r.report(operand, err)
		return nil
	}
	if !r.opts.recursive {
		return r.plain(resolved, operand, fi)
	}
	return r.tree(ctx, resolved, operand, fi, true, map[string]string{})
}

// plain changes the group of one file without walking below it, shown as
// given. fi describes it without following a final symbolic link.
func (r *run) plain(name, shown string, fi fs.FileInfo) error {
	if fi.Mode()&fs.ModeSymlink != 0 && r.opts.dereference != linkItself {
		// A symbolic link stands for what it points at, unless -h.
		target, tfi, err := r.p.referent(name)
		if err != nil {
			r.report(shown, err)
			return nil
		}
		if err := r.change(target, shown, tfi); err != nil {
			r.report(shown, err)
		}
		return nil
	}
	if err := r.change(name, shown, fi); err != nil {
		r.report(shown, err)
	}
	return nil
}

// tree changes the group of name and of every file of the hierarchy below
// it, shown as given, as the traversal options ask for. operand tells
// whether name was given on the command line. fi describes it without
// following a final symbolic link. seen holds the shown name of every
// directory of the current descent, to detect file system loops.
func (r *run) tree(ctx context.Context, name, shown string, fi fs.FileInfo, operand bool, seen map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		if err := r.change(name, shown, fi); err != nil {
			r.report(shown, err)
		}
		if !fi.IsDir() {
			return nil
		}
		return r.descend(ctx, name, shown, seen)
	}

	// A symbolic link: followed for the traversal as -H and -L ask for,
	// acted on itself otherwise, unless --dereference asks for its referent.
	followed := r.opts.follow == 'L' || operand && r.opts.follow == 'H'
	if !followed && r.opts.dereference != linkReferent {
		// -h, or the default -P walk: the link itself is acted on, which
		// leaves it alone, since no file system here can change one.
		if err := r.change(name, shown, fi); err != nil {
			r.report(shown, err)
		}
		return nil
	}
	if r.opts.dereference == linkItself {
		// -h: the link itself is acted on, and only the files below a link
		// followed for the traversal are walked: a dangling one has nothing
		// below to walk, and no error to report.
		if err := r.change(name, shown, fi); err != nil {
			r.report(shown, err)
		}
		if !followed {
			return nil
		}
		target, tfi, err := r.p.referent(name)
		if err != nil {
			return nil
		}
		if !tfi.IsDir() {
			return nil
		}
		if r.loop(shown, target, seen) {
			return nil
		}
		return r.descend(ctx, target, shown, seen)
	}
	target, tfi, err := r.p.referent(name)
	if err != nil {
		r.report(shown, err)
		return nil
	}
	if r.loop(shown, target, seen) {
		return nil
	}
	if !followed {
		// --dereference without following: the referent changes, but the
		// link is not walked below.
		if err := r.change(target, shown, tfi); err != nil {
			r.report(shown, err)
		}
		return nil
	}
	// The link stands for what it points at.
	return r.tree(ctx, target, shown, tfi, false, seen)
}

// loop reports a file system loop when the link at name points back at a
// directory of the current descent, recorded in seen. It tells whether the
// walk must stop.
func (r *run) loop(shown, target string, seen map[string]string) bool {
	first, ok := seen[target]
	if !ok {
		return false
	}
	r.report(shown, fmt.Errorf(
		"file system loop detected; '%s' is part of the same file system loop as '%s'",
		shown, first))
	return true
}

// descend walks the entries of the directory at name, shown as given.
func (r *run) descend(ctx context.Context, name, shown string, seen map[string]string) error {
	seen[name] = shown
	defer delete(seen, name)
	entries, err := fs.ReadDir(r.p.FS, name)
	if err != nil {
		r.report(shown, err)
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := path.Join(name, entry.Name())
		childShown := path.Join(shown, entry.Name())
		fi, err := r.p.FS.Lstat(child)
		if err != nil {
			r.report(childShown, err)
			continue
		}
		if err := r.tree(ctx, child, childShown, fi, false, seen); err != nil {
			return err
		}
	}
	return nil
}

// change sets the group of the file at name, shown as given, which fi
// describes. A file whose group the file system doesn't report is left
// alone, like on systems that don't model group ownership, and so is a
// symbolic link: ChownFS has no link-aware operation, and one whose Chown
// follows links, like the operating system one, would change the referent
// instead of the link.
func (r *run) change(name, shown string, fi fs.FileInfo) error {
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil
	}
	_, old, ok := fsx.Owner(fi)
	if !ok {
		return nil
	}
	if old == r.gid {
		if r.opts.verbosity == noisy {
			fmt.Fprintf(r.p.Stdout, "group of '%s' retained as %d\n", shown, r.gid)
		}
		return nil
	}
	if err := r.chowner.Chown(name, -1, r.gid); err != nil {
		return err
	}
	if r.opts.verbosity >= changesOnly {
		fmt.Fprintf(r.p.Stdout, "changed group of '%s' from %d to %d\n", shown, old, r.gid)
	}
	return nil
}

// report writes the diagnostic of a failed file, unless -f silences it.
func (r *run) report(shown string, err error) {
	if !r.opts.silence {
		r.p.errorf("%s: %v", shown, patherr.Unwrap(err))
	}
	r.failed = true
}

// referent returns the path and info of the file that the symbolic link at
// name points at.
func (p *Prog) referent(name string) (string, fs.FileInfo, error) {
	resolved := p.resolveLink(name)
	fi, err := fs.Stat(p.FS, resolved)
	return resolved, fi, err
}

// resolveLink follows the symbolic links at name, returning the path the
// last one points at. A name which is not a symbolic link, or a broken or
// looping chain, is returned as it is.
func (p *Prog) resolveLink(name string) string {
	for range 40 {
		fi, err := p.FS.Lstat(name)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			return name
		}
		target, err := p.FS.ReadLink(name)
		if err != nil {
			return name
		}
		name = path.Join(path.Dir(name), target)
	}
	return name
}

// parseGroup resolves a group operand, a group name or a numeric group ID.
// A numeric operand which also names a group stands for that group, as the
// POSIX specification asks; a leading '+' forces the numeric reading.
func parseGroup(s string) (int, error) {
	if num, err := strconv.Atoi(s); err == nil {
		if num < 0 {
			// A negative ID would tell Chown to keep the group unchanged.
			return 0, fmt.Errorf("invalid group: '%s'", s)
		}
		if g, err := user.LookupGroup(s); err == nil {
			if gid, err := strconv.Atoi(g.Gid); err == nil {
				return gid, nil
			}
		}
		return num, nil
	}
	g, err := user.LookupGroup(s)
	if err != nil {
		return 0, fmt.Errorf("invalid group: '%s'", s)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, fmt.Errorf("invalid group: '%s'", s)
	}
	return gid, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "chgrp: "+format+"\n", args...)
}

// The verbosity levels of a run, selected by -c and -v.
const (
	quiet = iota
	changesOnly
	noisy
)

// How a run treats a symbolic link: the default dereferences it unless -R
// walks, --dereference always does, and -h never.
const (
	linkAuto = iota
	linkReferent
	linkItself
)

// optReference and optDereference are runes which no short option can
// reach, so --reference and --dereference stay long-only.
const (
	optReference rune = 0x100 + iota
	optDereference
)

// longOptions maps the long options of chgrp to the characters they stand
// for.
var longOptions = map[string]rune{
	"recursive":      'R',
	"changes":        'c',
	"silent":         'f',
	"quiet":          'f',
	"verbose":        'v',
	"dereference":    optDereference,
	"no-dereference": 'h',
	"reference":      optReference,
}

// optSet describes the options of chgrp.
var optSet = getopt.Set{Valued: string(optReference), Long: longOptions}

// usage describes the options of chgrp.
const usage = `Usage: chgrp [OPTION]... {GROUP | --reference=REF_FILE} FILE...
Change the group of each FILE to GROUP.

  -c, --changes
         like --verbose but report only when a change is made
  -f, --silent, --quiet
         suppress most error messages
  -v, --verbose
         output a diagnostic for every file processed
      --dereference
         affect the referent of each symbolic link rather than the symbolic
         link itself
  -h, --no-dereference
         affect symbolic links instead of any referenced file; the links
         are left alone when the file system cannot change one
      --reference=REF_FILE
         use the group of REF_FILE instead of a GROUP value
  -R, --recursive
         operate on files and directories recursively
  -H
         with -R, follow a symbolic link to a directory given on the
         command line
  -L
         with -R, follow every symbolic link to a directory encountered
  -P
         with -R, never follow symbolic links
      --help
         display this help and exit
      --version
         output version information and exit
`

// options holds what a run of chgrp selects.
type options struct {
	recursive bool
	// follow is 'H', 'L' or 'P' for a recursive walk; the last one given
	// wins, and the default is 'P'.
	follow byte
	// dereference is one of linkAuto, linkReferent and linkItself; the last
	// of -h and --dereference given wins.
	dereference int
	silence     bool
	// verbosity is one of quiet, changesOnly and noisy; the last of -c and
	// -v given wins.
	verbosity int
	// reference names the file whose group is used instead of a group
	// operand, or is nil without --reference.
	reference *string
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
			opts.follow = 'H'
		case 'L':
			opts.follow = 'L'
		case 'P':
			opts.follow = 'P'
		case 'f':
			opts.silence = true
		case 'c':
			opts.verbosity = changesOnly
		case 'v':
			opts.verbosity = noisy
		case 'h':
			opts.dereference = linkItself
		case optDereference:
			opts.dereference = linkReferent
		case optReference:
			opts.reference = &value
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
