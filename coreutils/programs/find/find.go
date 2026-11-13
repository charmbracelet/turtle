// Package find implements the POSIX find utility with its GNU extensions.
package find

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/fileinfo"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/input"
	"charm.land/turtle/coreutils/internal/numparse"
	"charm.land/turtle/coreutils/internal/output"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
	"charm.land/turtle/coreutils/internal/usernames"
	"charm.land/turtle/coreutils/internal/walk"
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
		stdopts.Errorf(p.Runtime, "find", "%v", err)
		stdopts.TryHelp(p.Runtime, "find")
		return &coreutils.ExitError{Code: 1}
	}
	if opts.files0From != "" {
		paths, err = p.readPaths0(ctx, opts.files0From)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			stdopts.Errorf(p.Runtime, "find", "%v", err)
			return &coreutils.ExitError{Code: 1}
		}
	} else if len(paths) == 0 {
		if len(tokens) > 0 {
			stdopts.Errorf(p.Runtime, "find", "paths must precede expression: '%s'", tokens[0])
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
		expr, err = parseExpr(p.Runtime, p.Now(), tokens, opts.follow)
		if err != nil {
			stdopts.Errorf(p.Runtime, "find", "%v", err)
			stdopts.TryHelp(p.Runtime, "find")
			return &coreutils.ExitError{Code: 1}
		}
	}
	action, execs, outFiles := scanExpr(expr, &opts)
	if !action {
		expr = andExpr{expr, printExpr{}}
	}

	s := &state{
		p: p, ctx: ctx, opts: opts, now: p.Now(),
		execs: execs, outFiles: map[string]io.Writer{},
		usernames: usernames.Names{},
	}
	if err := s.openOutFiles(outFiles); err != nil {
		return &coreutils.ExitError{Code: 1}
	}
	// An early return, like a canceled context, still closes the files.
	defer s.closeOutFiles()

	quit := false
	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if quit {
			break
		}
		fsName := p.FsPath(root)
		err := walk.Walk(ctx, p.FS, walk.Options{
			Follow:   opts.follow,
			Depth:    opts.depth,
			MaxDepth: opts.maxDepth,
			MinDepth: opts.minDepth,
			XDev:     opts.xdev,
			Mount:    opts.mount,
			OnError: func(name string, err error) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if opts.ignoreRace && errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				if msg, isDirErr := walk.DirError(name, err); isDirErr {
					stdopts.Errorf(p.Runtime, "find", "%s", msg)
				} else if perr, ok := errors.AsType[*fs.PathError](err); ok {
					stdopts.Errorf(p.Runtime, "find", "'%s': %v", name, perr.Err)
				} else {
					stdopts.Errorf(p.Runtime, "find", "%v", err)
				}
				s.failed = true
				return nil
			},
		}, root, fsName, func(name, fsName string, fi fs.FileInfo, depth int) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			s.pruned = false
			e := file{root: root, name: name, fsName: fsName, fi: fi, depth: depth}
			expr.eval(s, e)
			if s.quit {
				quit = true
				return false, errQuit
			}
			return !s.pruned, nil
		})
		if errors.Is(err, errQuit) {
			break
		}
		if err != nil {
			return err
		}
	}
	s.flush()
	s.flushOutFiles()
	s.closeOutFiles()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.failed || s.execFailed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// errQuit stops the traversal when -quit was evaluated.
var errQuit = errors.New("quit")

// readPaths0 reads the starting points of -files0-from, which are separated
// by null bytes. A name of "-" reads them from the standard input.
func (p *Prog) readPaths0(ctx context.Context, name string) ([]string, error) {
	if name == "-" && p.Stdin == nil {
		return nil, errors.New("no standard input available")
	}
	data, err := input.ReadAll(p.Runtime, ctx, name)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if name == "-" {
			return nil, fmt.Errorf("cannot read the standard input: %v", err)
		}
		if pathErr, ok := errors.AsType[*fs.PathError](err); ok && pathErr.Op == "open" {
			return nil, fmt.Errorf("cannot open '%s': %v", name, patherr.Unwrap(err))
		}
		return nil, fmt.Errorf("cannot read '%s': %v", name, patherr.Unwrap(err))
	}
	if len(data) == 0 {
		return nil, nil
	}
	paths := strings.Split(string(data), "\x00")
	if paths[len(paths)-1] == "" {
		paths = paths[:len(paths)-1]
	}
	if slices.Contains(paths, "") {
		return nil, errors.New("zero-length file name in -files0-from input")
	}
	return paths, nil
}

// openOutFiles opens the files the -fprint family writes to, truncating
// them, as they are created even when nothing is written to them.
func (s *state) openOutFiles(names []string) error {
	for _, name := range names {
		if _, ok := s.outFiles[name]; ok {
			continue
		}
		switch name {
		case "/dev/stdout":
			s.outFiles[name] = s.p.Stdout
			continue
		case "/dev/stderr":
			s.outFiles[name] = s.p.Stderr
			continue
		}
		f, err := output.Create(s.p.Runtime, s.ctx, name)
		if err != nil {
			stdopts.Errorf(s.p.Runtime, "find", "cannot open '%s' for writing: %v", name, patherr.Unwrap(err))
			s.failed = true
			return err
		}
		s.outFiles[name] = f
		s.outClosers = append(s.outClosers, f)
	}
	return nil
}

// closeOutFiles closes the files the -fprint family opened, but never the
// standard output and error it may also write to. Run calls it before
// deciding the exit status, so its failures count, and again, without
// effect, on an early return.
func (s *state) closeOutFiles() {
	closers := s.outClosers
	s.outClosers = nil
	for _, f := range closers {
		if err := f.Close(); err != nil {
			stdopts.Errorf(s.p.Runtime, "find", "close failed: %v", err)
			s.failed = true
		}
	}
}

// flushOutFiles flushes the files the -fprint family wrote to.
func (s *state) flushOutFiles() {
	for _, f := range s.outClosers {
		if flusher, ok := f.(interface{ Flush() error }); ok {
			if err := flusher.Flush(); err != nil {
				stdopts.Errorf(s.p.Runtime, "find", "write error: %v", err)
				s.failed = true
			}
		}
	}
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
	quit       bool // -quit was evaluated
	execs      []*execExpr
	stdin      *bufio.Reader
	outFiles   map[string]io.Writer
	outClosers []io.Closer     // the output files that were opened
	usernames  usernames.Names // the cached user and group names
}

// out returns the writer an output file name stands for, which is the
// standard output for the empty name.
func (s *state) out(name string) io.Writer {
	if name == "" {
		return s.p.Stdout
	}
	return s.outFiles[name]
}

// linkTarget returns what a symbolic link points at, or the empty string
// when it can't be read.
func (s *state) linkTarget(f file) string {
	target, err := fs.ReadLink(s.p.FS, f.fsName)
	if err != nil {
		return ""
	}
	return target
}

// timeOf returns the file time a time primary checks.
func (s *state) timeOf(fi fs.FileInfo, which byte) time.Time {
	return fileTime(fi, which)
}

// fileInfoStat returns the information a file system reports about a file
// beyond what io/fs models.
func fileInfoStat(fi fs.FileInfo) (fileinfo.Info, bool) {
	return fileinfo.Stat(fi)
}

// options holds the options and the primaries that apply to the whole
// traversal.
type options struct {
	follow     byte // 'P', 'H' or 'L'
	depth      bool
	mount      bool
	xdev       bool
	maxDepth   int  // how many levels are descended; zero means all
	minDepth   int  // from which level the files are acted on
	daystart   bool // times are measured from the beginning of today
	ignoreRace bool
	files0From string
}

// optSet describes the options of find. It holds no short options, since
// the -H, -L and -P options are parsed by parseFlags, before the paths.
var optSet = getopt.Set{}

// usage describes the options and primaries of find.
const usage = `Usage: find [-H] [-L] [-P] [-Olevel] [-D debug-features] [path...]
       [expression]
Search the file hierarchy below each path, evaluating an expression on every
file found. The expression combines the primaries below with ! (not), -a
(and), -o (or) and the comma operator, and defaults to -print.

  -H
         follow the symbolic links given as paths, not those met during the
         traversal
  -L
         follow every symbolic link
  -P
         never follow symbolic links
  -amin n
         true when the file was last accessed n minutes ago
  -anewer FILE
         true when the file was last accessed more recently than FILE was
         modified
  -atime n
         true when the file was last accessed n days ago
  -cmin n
         true when the file status last changed n minutes ago
  -cnewer FILE
         true when the file status last changed more recently than FILE was
         modified
  -ctime n
         true when the file status last changed n days ago
  -daystart
         measure the times from the beginning of today
  -delete
         delete the file, empty directories only; implies -depth
  -depth
         act on the entries of a directory before the directory itself
  -empty
         true when the file is an empty regular file or directory
  -exec UTILITY [ARG]... ;
         run UTILITY on the file, true when it succeeds; an ARG of {} is
         the pathname
  -exec UTILITY [ARG]... {} +
         run UTILITY once per set of pathnames
  -execdir UTILITY [ARG]... ;
         like -exec, but run UTILITY in the directory of the file, with {}
         standing for its base name
  -executable
         true when the file can be executed or searched by the user
  -false
         always false
  -files0-from FILE
         read the starting points from FILE, null byte separated
  -fls FILE
         like -ls, but write to FILE
  -fprint FILE
         like -print, but write to FILE
  -fprint0 FILE
         like -print0, but write to FILE
  -fprintf FILE FORMAT
         like -printf, but write to FILE
  -fstype TYPE
         not supported: a runtime file system can't tell its type
  -gid n
         true when the file belongs to the group identifier n
  -group GNAME
         true when the file belongs to the group GNAME
  -ilname PATTERN
         like -lname, but ignoring case distinctions
  -iname PATTERN
         like -name, but ignoring case distinctions
  -inum n
         true when the file has inode number n
  -ipath PATTERN
         like -path, but ignoring case distinctions
  -iregex PATTERN
         like -regex, but ignoring case distinctions
  -iwholename PATTERN
         like -wholename, but ignoring case distinctions
  -links n
         true when the file has n links
  -lname PATTERN
         true when the file is a symbolic link to PATTERN
  -ls
         list the file in the format of ls -dils
  -maxdepth levels
         descend at most levels below the paths
  -mindepth levels
         act only at levels and deeper
  -mmin n
         true when the file was last modified n minutes ago
  -mount
         act only on the files of the same file system, and don't descend
         into the others
  -mtime n
         true when the file was last modified n days ago
  -name PATTERN
         true when the basename of the file matches PATTERN
  -newer FILE
         true when the file was modified more recently than FILE
  -newerXY REFERENCE
         true when time X of the file is newer than time Y of REFERENCE;
         X and Y are a, c or m, and Y can also be t, a literal time
  -nogroup
         true when no group owns the file
  -noleaf
         accepted and ignored: the optimization it turns off isn't done
  -not
         like !, negates the expression that follows
  -nouser
         true when no user owns the file
  -ok UTILITY [ARG]... ;
         like -exec, but asking for a confirmation first
  -okdir UTILITY [ARG]... ;
         like -execdir, but asking for a confirmation first
  -path PATTERN
         true when the whole pathname matches PATTERN
  -perm [-|/]MODE
         true when the permission bits match MODE, at least all of them
         with a '-', and any of them with a '/'
  -print
         write the pathname
  -print0
         write the pathname, terminated by a null byte
  -printf FORMAT
         write FORMAT, interpreting escapes and % directives
  -prune
         don't descend into the directory
  -readable
         true when the file can be read by the user
  -regex PATTERN
         true when the whole pathname matches the expression PATTERN
  -regextype TYPE
         the regular expression syntax of TYPE, which is taken as
         posix-extended unless it is posix-basic, grep or emacs
  -samefile FILE
         true when the file shares an inode with FILE
  -size n[bcwkMG]
         true when the file is n units long, of 512 bytes by default, or of
         bytes, 2-byte words, kibibytes, mebibytes or gibibytes
  -true
         always true
  -type c
         true when the file is of type c: b, c, d, p, f, l or s, or a list
         of them separated by commas
  -uid n
         true when the file belongs to the user identifier n
  -used n
         true when the file was last accessed n days after its status last
         changed
  -user UNAME
         true when the file belongs to the user UNAME
  -wholename PATTERN
         like -path
  -writable
         true when the file can be written by the user
  -xdev
         don't descend into the directories of another file system
  -xtype c
         like -type, but checking the type a symbolic link points to
` + stdopts.Tail

// parseFlags splits the arguments into the -H, -L, -P, -O, -D and
// -files0-from options, the path operands, and the expression tokens.
func parseFlags(args []string) (options, []string, []string, error) {
	opts := options{follow: 'P', maxDepth: -1}
	var i int
options:
	for i = 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-H", arg == "-L", arg == "-P":
			opts.follow = arg[1]
		case arg == "--":
			i++
			break options
		case arg == "-files0-from":
			if i+1 == len(args) {
				return options{}, nil, nil, fmt.Errorf("option '-files0-from' requires an argument")
			}
			i++
			opts.files0From = args[i]
		case strings.HasPrefix(arg, "-O"):
			level := arg[2:]
			if level == "" {
				if i+1 == len(args) {
					return options{}, nil, nil, errors.New("you must specify a digit after the -O option")
				}
				i++
				level = args[i]
			}
			if level == "" || !numparse.AllDigits(level) {
				return options{}, nil, nil, fmt.Errorf("invalid optimisation level '%s'", level)
			}
			// The optimisation only reorders the evaluation, which this
			// implementation doesn't do.
		case arg == "-D":
			if i+1 == len(args) {
				return options{}, nil, nil, errors.New("you must specify a debug option after -D")
			}
			i++
			if err := checkDebug(args[i]); err != nil {
				return options{}, nil, nil, err
			}
			// Debugging output isn't implemented.
		default:
			break options
		}
	}
	var paths []string
	for i < len(args) && !isExprStart(args[i]) {
		if opts.files0From != "" {
			return options{}, nil, nil, errors.New("the -files0-from option cannot be combined with a starting point")
		}
		paths = append(paths, args[i])
		i++
	}
	return opts, paths, args[i:], nil
}

// debugFeatures are the debug options -D accepts.
var debugFeatures = map[string]bool{
	"Help": true, "Tree": true, "Search": true, "Stat": true,
	"OptExprs": true, "Rates": true, "TreeOpt": true,
}

// checkDebug validates a -D option argument, whose feature names are
// matched without case distinctions, like GNU find does.
func checkDebug(value string) error {
	for feature := range strings.SplitSeq(value, ",") {
		known := false
		for name := range debugFeatures {
			if strings.EqualFold(name, feature) {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown debug option '%s'", feature)
		}
	}
	return nil
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

// dirname returns the leading directories of a pathname, which %h writes:
// all but the last component, "." when there is no slash, and nothing for
// the root directory.
func dirname(name string) string {
	i := strings.LastIndex(name, "/")
	switch i {
	case -1:
		return "."
	case 0:
		return ""
	default:
		return name[:i]
	}
}
