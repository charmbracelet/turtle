// Package find implements the POSIX find utility.
package find

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/fileinfo"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"

	"charm.land/snail/fsx"
)

// Prog is the find core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run evaluates an expression on every file of the hierarchy below each
// path operand.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "find", usage, optSet, args); done {
		return err
	}
	opts, paths, tokens, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "find")
		return &coreutils.ExitError{Code: 1}
	}
	if len(paths) == 0 {
		if len(tokens) > 0 {
			p.errorf("paths must precede expression: '%s'", tokens[0])
			stdopts.TryHelp(p.Runtime, "find")
			return &coreutils.ExitError{Code: 1}
		}
		paths = []string{"."}
	}
	for i := range paths {
		paths[i] = filepath.ToSlash(paths[i])
	}
	var expr expr = printExpr{}
	if len(tokens) > 0 {
		var err error
		expr, err = parseExpr(p.Runtime, tokens)
		if err != nil {
			p.errorf("%v", err)
			stdopts.TryHelp(p.Runtime, "find")
			return &coreutils.ExitError{Code: 1}
		}
	}
	action, execs := scanExpr(expr, &opts)
	if !action {
		expr = andExpr{expr, printExpr{}}
	}

	s := &state{p: p, ctx: ctx, opts: opts, now: p.Now(), loops: map[string]string{}, execs: execs}
	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		fsName := path.Clean(filepath.ToSlash(p.ResolvePath(root)))
		fi, err := p.stat(s, fsName, true)
		if err != nil {
			p.errorf("'%s': %v", root, patherr.Unwrap(err))
			s.failed = true
			continue
		}
		st, _ := fileinfo.Stat(fi)
		if err := p.walk(ctx, s, expr, root, fsName, fi, st.Dev); err != nil {
			return err
		}
	}
	s.flush()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.failed || s.execFailed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// walk evaluates the expression on one file and, when it's a directory
// that isn't pruned, on the files below it. The name is the pathname as
// given, while fsName is what the file system is asked for, since it takes
// only cleaned, slash-separated paths resolved against the working
// directory.
func (p *Prog) walk(ctx context.Context, s *state, e expr, name, fsName string, fi fs.FileInfo, rootDev uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st, stOK := fileinfo.Stat(fi)
	offRoot := stOK && st.Dev != rootDev
	// -mount doesn't act on the files of another file system, and with -xdev
	// it still applies, as the spec says both together behave like -mount
	// alone.
	if s.opts.mount && offRoot {
		return nil
	}
	descend := func() error {
		if s.pruned || !fi.IsDir() || (s.opts.xdev && offRoot) {
			return nil
		}
		return p.descend(ctx, s, e, name, fsName, fi, rootDev)
	}
	s.pruned = false
	if !s.opts.depth {
		e.eval(s, name, fi)
		return descend()
	}
	if err := descend(); err != nil {
		return err
	}
	e.eval(s, name, fi)
	return nil
}

// descend evaluates the expression on the entries of one directory.
func (p *Prog) descend(ctx context.Context, s *state, e expr, dir, fsDir string, fi fs.FileInfo, rootDev uint64) error {
	if s.opts.follow == 'L' {
		// Only the directories of the current chain of ancestors count: a
		// directory met again once its ancestor is left is a revisit, not a
		// loop.
		if id, ok := dirID(fi); ok {
			if first, seen := s.loops[id]; seen {
				p.errorf("file system loop detected; '%s' is part of the same file system loop as '%s'", dir, first)
				s.failed = true
				return nil
			}
			s.loops[id] = dir
			defer delete(s.loops, id)
		}
	}
	entries, err := fs.ReadDir(p.FS, fsDir)
	if err != nil {
		p.errorf("cannot read directory '%s': %v", dir, patherr.Unwrap(err))
		s.failed = true
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := joinPath(dir, entry.Name())
		fsChild := path.Join(fsDir, entry.Name())
		cfi, err := p.stat(s, fsChild, false)
		if err != nil {
			p.errorf("'%s': %v", child, patherr.Unwrap(err))
			s.failed = true
			continue
		}
		if err := p.walk(ctx, s, e, child, fsChild, cfi, rootDev); err != nil {
			return err
		}
	}
	return nil
}

// stat describes one file, following symbolic links as the options ask for.
// A link whose target doesn't exist is described as itself, while any other
// failure to reach the target, like a symlink loop, is reported.
func (p *Prog) stat(s *state, name string, operand bool) (fs.FileInfo, error) {
	if s.opts.follow == 'L' || (operand && s.opts.follow == 'H') {
		fi, err := fs.Stat(p.FS, name)
		if err == nil {
			return fi, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			if lfi, lerr := fs.Lstat(p.FS, name); lerr == nil {
				return lfi, nil
			}
		}
		return nil, err
	}
	return fs.Lstat(p.FS, name)
}

// joinPath concatenates a directory and an entry name, keeping a single
// slash between them no matter how the directory was given.
func joinPath(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// dirID returns what identifies a directory for loop detection, reporting
// false when the file system reports nothing to identify it with.
func dirID(fi fs.FileInfo) (string, bool) {
	st, ok := fileinfo.Stat(fi)
	if !ok || st.Dev == 0 && st.Ino == 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), true
}

// state is the evaluation state shared by the whole traversal.
type state struct {
	p          *Prog
	ctx        context.Context
	opts       options
	now        time.Time
	pruned     bool // the last -prune seen
	failed     bool // an error was seen
	execFailed bool // a -exec {} + invocation exited nonzero
	loops      map[string]string
	execs      []*execExpr
	stdin      *bufio.Reader
}

// timeOf returns the file time a -atime, -ctime or -mtime primary checks,
// falling back to the modification time, which a file system always reports.
func (s *state) timeOf(fi fs.FileInfo, which byte) time.Time {
	switch which {
	case 'a':
		if t, ok := fsx.Atime(fi); ok {
			return t
		}
	case 'c':
		if t, ok := fileinfo.Ctime(fi); ok {
			return t
		}
	}
	return fi.ModTime()
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "find: "+format+"\n", args...)
}

// options holds the options and the primaries that apply to the whole
// traversal.
type options struct {
	follow byte // 'P', 'H' or 'L'
	depth  bool
	mount  bool
	xdev   bool
}

// optSet describes the options of find. It holds no short options, since
// the -H, -L and -P options are parsed by parseFlags, before the paths.
var optSet = getopt.Set{}

// usage describes the options and primaries of find.
const usage = `Usage: find [-H] [-L] [-P] path... [expression]
Search the file hierarchy below each path, evaluating an expression on every
file found. The expression combines the primaries below with ! (not), -a
(and) and -o (or), and defaults to -print.

  -H
         follow the symbolic links given as paths, not those met during the
         traversal
  -L
         follow every symbolic link
  -P
         never follow symbolic links
  -name PATTERN
         true when the basename of the file matches PATTERN
  -iname PATTERN
         like -name, but ignoring case distinctions
  -path PATTERN
         true when the whole pathname matches PATTERN
  -nouser
         true when no user owns the file
  -nogroup
         true when no group owns the file
  -mount
         act only on the files of the same file system, and don't descend
         into the others
  -xdev
         don't descend into the directories of another file system
  -prune
         don't descend into the directory
  -perm [-]MODE
         true when the permission bits match MODE, at least all of them
         with a '-'
  -type c
         true when the file is of type c: b, c, d, l, p, f or s
  -links n
         true when the file has n links
  -user UNAME
         true when the file belongs to the user UNAME
  -group GNAME
         true when the file belongs to the group GNAME
  -size n[c]
         true when the file is n units long, of 512 bytes, or of one byte
         with c
  -atime n
         true when the file was last accessed n days ago
  -ctime n
         true when the file status last changed n days ago
  -mtime n
         true when the file was last modified n days ago
  -newer FILE
         true when the file was modified more recently than FILE
  -exec UTILITY [ARG]... ;
         run UTILITY on the file, true when it succeeds; an ARG of {} is
         the pathname
  -exec UTILITY [ARG]... {} +
         run UTILITY once per set of pathnames
  -ok UTILITY [ARG]... ;
         like -exec, but asking for a confirmation first
  -print
         write the pathname
  -print0
         write the pathname, terminated by a null byte
  -depth
         act on the entries of a directory before the directory itself
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags splits the arguments into the -H, -L and -P options, the path
// operands, and the expression tokens.
func parseFlags(args []string) (options, []string, []string, error) {
	opts := options{follow: 'P'}
	var i int
options:
	for i = 0; i < len(args); i++ {
		switch args[i] {
		case "-H", "-L", "-P":
			opts.follow = args[i][1]
		case "--":
			i++
			break options
		default:
			break options
		}
	}
	var paths []string
	for i < len(args) && !isExprStart(args[i]) {
		paths = append(paths, args[i])
		i++
	}
	return opts, paths, args[i:], nil
}

// isExprStart reports whether an argument begins the expression: the first
// one that starts with a '-' or is '!' or '(' does.
func isExprStart(arg string) bool {
	return len(arg) > 1 && arg[0] == '-' || arg == "!" || arg == "("
}

var _ coreutils.Program = (*Prog)(nil)

// basename returns the name the -name and -iname primaries match, which is
// the last component of the pathname.
func basename(name string) string {
	return path.Base(name)
}
