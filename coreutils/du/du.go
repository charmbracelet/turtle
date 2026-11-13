// Package du implements the POSIX du utility with the GNU extensions its
// manual describes for the options implemented here.
//
// It writes the file space allocated to each file operand, to every
// subdirectory of a directory operand, and, with -a, to every file in the
// hierarchy below it, each one as a line holding its size, a space and
// its name, in the units the size options ask for: 512-byte ones rounded
// up by default.
//
// The file space a file allocates comes from the blocks its file system
// reports. A file system that reports none, like the memory one, measures a
// file by its size rounded up to whole 512-byte blocks: a symbolic link
// counts the length of the target it holds, and a directory its own size,
// which the memory file system reports as zero.
//
// A file that occurs multiple times is counted and written once, as the
// specification requires, but only when its file system reports an identity
// for it, by device and serial number: one without any, like the memory file
// system, counts every occurrence. The devices that -x compares come from
// the same identity, so such a file system never skips a directory.
package du

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"strconv"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/fileinfo"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
	"charm.land/turtle/coreutils/internal/walk"
)

// Prog is the du core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the amount of file space allocated to each file operand and to
// each subdirectory of the hierarchy below each directory operand,
// defaulting to the working directory when no operand is given. The
// traversal stops when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "du", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		stdopts.Errorf(p.Runtime, "du", "%v", err)
		stdopts.TryHelp(p.Runtime, "du")
		return &coreutils.ExitError{Code: 1}
	}
	if !p.checkConflicts(opts) {
		return &coreutils.ExitError{Code: 1}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "du", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 0 {
		operands = []string{"."}
	}
	for i := range operands {
		operands[i] = filepath.ToSlash(operands[i])
	}
	r := &runner{p: p, opts: opts, track: len(operands) > 1 || opts.follow == 'L'}
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.operand(ctx, name); err != nil {
			return err
		}
	}
	if opts.total {
		fmt.Fprintf(p.Stdout, "%s total\n", opts.spec.Format(r.grand))
	}
	if r.failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// checkConflicts reports whether the options of a run can work together,
// writing the diagnostics of the ones that can't. Summarizing and showing
// every entry are mutually exclusive, and so are summarizing and a maximum
// depth other than zero, while a zero one is the same as summarizing.
func (p *Prog) checkConflicts(opts options) bool {
	switch {
	case opts.all && opts.summarize:
		stdopts.Errorf(p.Runtime, "du", "options --summarize and --all are mutually exclusive")
		stdopts.TryHelp(p.Runtime, "du")
		return false
	case opts.summarize && opts.maxDepthSet && opts.maxDepth != 0:
		stdopts.Errorf(p.Runtime, "du", "warning: summarizing conflicts with --max-depth=%d", opts.maxDepth)
		stdopts.TryHelp(p.Runtime, "du")
		return false
	case opts.summarize && opts.maxDepthSet:
		stdopts.Errorf(p.Runtime, "du", "warning: summarizing is the same as using --max-depth=0")
	}
	return true
}

// runner holds the state of a single run.
type runner struct {
	p    *Prog
	opts options

	// track marks every file as seen, so one that occurs again is counted
	// once. Files are tracked when they can recur: with -L, or under
	// multiple operands. One with a link count above one is tracked
	// regardless.
	track  bool
	seen   map[string]bool
	grand  int64 // the total -c writes, in the units of the run
	failed bool
}

// operand measures the hierarchy of one file operand, writing a line for
// every file the options ask for.
func (r *runner) operand(ctx context.Context, name string) error {
	if name == "" {
		stdopts.Errorf(r.p.Runtime, "du", "cannot access '': %v", fs.ErrNotExist)
		r.failed = true
		return nil
	}
	acc := accumulator{}
	fsName := r.p.FsPath(name)
	return walk.Walk(ctx, r.p.FS, walk.Options{
		Follow:   r.opts.follow,
		Depth:    true,
		MaxDepth: -1,
		XDev:     r.opts.oneFS,
		OnError: func(name string, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if msg, isDirErr := walk.DirError(name, err); isDirErr {
				stdopts.Errorf(r.p.Runtime, "du", "%s", msg)
			} else {
				stdopts.Errorf(r.p.Runtime, "du", "cannot access '%s': %v", name, patherr.Unwrap(err))
			}
			r.failed = true
			return nil
		},
	}, name, fsName, func(name, _ string, fi fs.FileInfo, depth int) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if fi.IsDir() {
			r.dir(fi, name, depth, acc)
		} else {
			r.file(fi, name, depth, acc)
		}
		return true, nil
	})
}

// file adds the size of one non-directory to the directory holding it,
// writing its line with -a.
func (r *runner) file(fi fs.FileInfo, name string, depth int, acc accumulator) {
	if r.counted(fi) {
		return
	}
	size := r.measure(fi)
	if depth > 0 {
		acc.add(depth-1, size, 0)
	}
	if r.opts.total {
		r.grand = addSize(r.grand, size)
	}
	if depth == 0 || r.opts.all && depth <= r.opts.depthLimit() {
		r.print(size, name)
	}
}

// dir writes the line of one directory, the size of everything it holds
// plus its own, or with -S the one of the entries it holds directly. The
// directory above it gets its own size as a direct entry, and the sizes of
// everything below as a subdirectory total. A directory counted already is
// left alone, the same way as any other repeated file.
func (r *runner) dir(fi fs.FileInfo, name string, depth int, acc accumulator) {
	if r.counted(fi) {
		return
	}
	own := r.measure(fi)
	lvl := acc[depth]
	delete(acc, depth)
	total := addSize(own, addSize(lvl.entries, lvl.subdirs))
	if depth > 0 {
		entry := own
		if r.opts.separate {
			entry = 0
		}
		acc.add(depth-1, entry, addSize(lvl.entries, lvl.subdirs))
	}
	if r.opts.total {
		r.grand = addSize(r.grand, own)
	}
	size := total
	if r.opts.separate {
		size = addSize(own, lvl.entries)
	}
	if depth <= r.opts.depthLimit() {
		r.print(size, name)
	}
}

// counted reports whether a file was counted already, marking it seen
// otherwise, so one that occurs multiple times is counted once. Files whose
// file system reports no identity can't be deduplicated.
func (r *runner) counted(fi fs.FileInfo) bool {
	st, ok := fileinfo.Stat(fi)
	if !ok || st.Dev == 0 && st.Ino == 0 {
		return false
	}
	if !r.track && st.Nlink < 2 {
		return false
	}
	id := strconv.FormatUint(st.Dev, 10) + ":" + strconv.FormatUint(st.Ino, 10)
	if r.seen[id] {
		return true
	}
	if r.seen == nil {
		r.seen = make(map[string]bool)
	}
	r.seen[id] = true
	return false
}

// sizeOf returns the file space a file allocates in bytes, or its apparent
// size with -b. A file system that reports no blocks is measured by the
// size of the file rounded up to whole 512-byte blocks.
func (r *runner) sizeOf(fi fs.FileInfo) int64 {
	if r.opts.apparent {
		return fi.Size()
	}
	if st, ok := fileinfo.Stat(fi); ok {
		if st.Blocks > (math.MaxInt64-511)/512 {
			return math.MaxInt64
		}
		return int64(st.Blocks * 512)
	}
	size := max(fi.Size(), 0)
	if size > math.MaxInt64-511 {
		return math.MaxInt64
	}
	return (size + 511) / 512 * 512
}

// measure returns the size of a file as a whole count of the units the run
// writes sizes in, rounded up, the way the GNU utility counts each file
// before summing them.
func (r *runner) measure(fi fs.FileInfo) int64 {
	return ceilUnit(r.sizeOf(fi), r.opts.unit)
}

// print writes the line of one file, as the format the specification
// gives: the size, a space, and the name.
func (r *runner) print(size int64, name string) {
	fmt.Fprintf(r.p.Stdout, "%s %s\n", r.opts.spec.Format(size), name)
}

// depthLimit returns the number of levels below an operand that still get
// their lines written. Summarizing keeps the operand alone, like a maximum
// depth of zero, and no maximum descends the whole hierarchy.
func (o options) depthLimit() int {
	switch {
	case o.summarize:
		return 0
	case o.maxDepth < 0:
		return math.MaxInt
	default:
		return o.maxDepth
	}
}

// addSize adds two sizes, saturating at the largest one an int64 holds
// rather than overflowing.
func addSize(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

// ceilUnit rounds n up to a whole multiple of unit, saturating rather than
// overflowing.
func ceilUnit(n, unit int64) int64 {
	if unit <= 1 {
		return max(n, 0)
	}
	if n < 0 {
		return 0
	}
	if n > math.MaxInt64-unit+1 {
		return math.MaxInt64
	}
	return (n + unit - 1) / unit * unit
}

// level holds the sizes accumulated below one directory: the entries it
// holds directly, and the totals of the subdirectories closed already.
type level struct {
	entries int64
	subdirs int64
}

// accumulator folds the sizes of the files visited in post-order into the
// directories that hold them, one level of the hierarchy per entry.
type accumulator map[int]level

// add folds an entry size and a subdirectory total into the level depth.
func (a accumulator) add(depth int, entry, subdir int64) {
	lvl := a[depth]
	lvl.entries = addSize(lvl.entries, entry)
	lvl.subdirs = addSize(lvl.subdirs, subdir)
	a[depth] = lvl
}
