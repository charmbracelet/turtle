package find

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils/internal/fileinfo"
	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/patherr"

	"charm.land/snail/fsx"
)

// file is one file of the traversal, with the names the traversal uses.
type file struct {
	root   string // the starting point as given
	name   string // the pathname as written
	fsName string // the pathname the file system is asked for
	fi     fs.FileInfo
	depth  int
}

// relName returns the pathname with the starting point it was found below
// removed from the beginning, which %P writes.
func (f file) relName() string {
	switch {
	case f.name == f.root:
		return ""
	case f.root == "/":
		return f.name[1:]
	case len(f.name) > len(f.root)+1 && f.name[:len(f.root)] == f.root && f.name[len(f.root)] == '/':
		return f.name[len(f.root)+1:]
	}
	return f.name
}

// expr is one node of the expression tree evaluated on every file.
type expr interface {
	eval(s *state, f file) bool
}

// andExpr is the conjunction of two expressions, the second one skipped
// when the first is false.
type andExpr struct {
	left, right expr
}

func (e andExpr) eval(s *state, f file) bool {
	if !e.left.eval(s, f) {
		return false
	}
	return e.right.eval(s, f)
}

// orExpr is the alternation of two expressions, the second one skipped when
// the first is true.
type orExpr struct {
	left, right expr
}

func (e orExpr) eval(s *state, f file) bool {
	if e.left.eval(s, f) {
		return true
	}
	return e.right.eval(s, f)
}

// notExpr negates an expression.
type notExpr struct {
	inner expr
}

func (e notExpr) eval(s *state, f file) bool {
	return !e.inner.eval(s, f)
}

// listExpr is the comma operator: both sides are always evaluated, and the
// value of the second one is the result.
type listExpr struct {
	left, right expr
}

func (e listExpr) eval(s *state, f file) bool {
	e.left.eval(s, f)
	return e.right.eval(s, f)
}

// trueExpr is -true.
type trueExpr struct{}

func (e trueExpr) eval(s *state, f file) bool { return true }

// falseExpr is -false.
type falseExpr struct{}

func (e falseExpr) eval(s *state, f file) bool { return false }

// nameExpr is the -name and -iname primary: the basename matches a pattern.
type nameExpr struct {
	re *regexp.Regexp
}

func (e nameExpr) eval(s *state, f file) bool {
	return e.re.MatchString(basename(f.name))
}

// pathMatch is the -path, -wholename and their case-insensitive forms: the
// whole pathname matches a pattern.
type pathMatch struct {
	re *regexp.Regexp
}

func (e pathMatch) eval(s *state, f file) bool {
	return e.re.MatchString(f.name)
}

// linkMatch is the -lname and -ilname primary: the target of a symbolic
// link matches a pattern.
type linkMatch struct {
	re *regexp.Regexp
}

func (e linkMatch) eval(s *state, f file) bool {
	if f.fi.Mode()&fs.ModeSymlink == 0 {
		return false
	}
	if s.opts.follow == 'L' {
		// Every followed link but a broken one isn't seen as one.
		return false
	}
	return e.re.MatchString(s.linkTarget(f))
}

// regexMatch is the -regex and -iregex primary: the whole pathname matches
// a regular expression.
type regexMatch struct {
	re *regexp.Regexp
}

func (e regexMatch) eval(s *state, f file) bool {
	return e.re.MatchString(f.name)
}

// typeExpr is the -type primary, which may hold a list of types.
type typeExpr struct {
	typs []fs.FileMode
}

func (e typeExpr) eval(s *state, f file) bool {
	return matchType(e.typs, f.fi.Mode().Type())
}

// matchType reports whether the type of a file is one of the wanted ones.
func matchType(typs []fs.FileMode, typ fs.FileMode) bool {
	return slices.Contains(typs, typ)
}

// xtypeExpr is the -xtype primary: like -type, but checking the type a
// symbolic link points to instead of the link's own.
type xtypeExpr struct {
	typs []fs.FileMode
}

func (e xtypeExpr) eval(s *state, f file) bool {
	if f.fi.Mode()&fs.ModeSymlink == 0 {
		return matchType(e.typs, f.fi.Mode().Type())
	}
	// Under -L only the broken links are seen as ones, and under -H the
	// starting points are followed as -L would.
	followed := s.opts.follow == 'L' || f.depth == 0 && s.opts.follow == 'H'
	if followed {
		return matchType(e.typs, fs.ModeSymlink)
	}
	// Check the type of the target, which a broken or looping link never
	// reaches: it counts as of type l.
	tfi, err := fs.Stat(s.p.FS, f.fsName)
	if err != nil {
		return matchType(e.typs, fs.ModeSymlink)
	}
	return matchType(e.typs, tfi.Mode().Type())
}

// permExpr is the -perm primary. With all set, at least the bits of the
// template must match; with any set, at least one of them must.
type permExpr struct {
	spec filemode.Spec
	all  bool
	any  bool
}

func (e permExpr) eval(s *state, f file) bool {
	want := e.spec(0, f.fi.IsDir(), 0) & filemode.AccessBits
	have := f.fi.Mode() & filemode.AccessBits
	switch {
	case e.all:
		return have&want == want
	case e.any:
		// A template with no bits set matches every file.
		return want == 0 || have&want != 0
	default:
		return have == want
	}
}

// linksExpr is the -links primary.
type linksExpr struct {
	n num
}

func (e linksExpr) eval(s *state, f file) bool {
	nlink := int64(1)
	if st, ok := fileinfo.Stat(f.fi); ok && st.Nlink != 0 {
		nlink = int64(st.Nlink)
	}
	return e.n.match(nlink)
}

// userExpr is the -user primary.
type userExpr struct {
	uid int
}

func (e userExpr) eval(s *state, f file) bool {
	uid, _, ok := fsx.Owner(f.fi)
	return ok && uid == e.uid
}

// groupExpr is the -group primary.
type groupExpr struct {
	gid int
}

func (e groupExpr) eval(s *state, f file) bool {
	_, gid, ok := fsx.Owner(f.fi)
	return ok && gid == e.gid
}

// uidExpr is the -uid primary.
type uidExpr struct {
	n num
}

func (e uidExpr) eval(s *state, f file) bool {
	uid, _, ok := fsx.Owner(f.fi)
	if !ok {
		return false
	}
	return e.n.match(int64(uid))
}

// gidExpr is the -gid primary.
type gidExpr struct {
	n num
}

func (e gidExpr) eval(s *state, f file) bool {
	_, gid, ok := fsx.Owner(f.fi)
	if !ok {
		return false
	}
	return e.n.match(int64(gid))
}

// nouserExpr is the -nouser primary.
type nouserExpr struct{}

func (e nouserExpr) eval(s *state, f file) bool {
	uid, _, ok := fsx.Owner(f.fi)
	if !ok {
		return false
	}
	_, err := user.LookupId(strconv.Itoa(uid))
	return err != nil
}

// nogroupExpr is the -nogroup primary.
type nogroupExpr struct{}

func (e nogroupExpr) eval(s *state, f file) bool {
	_, gid, ok := fsx.Owner(f.fi)
	if !ok {
		return false
	}
	_, err := user.LookupGroupId(strconv.Itoa(gid))
	return err != nil
}

// inumExpr is the -inum primary.
type inumExpr struct {
	n num
}

func (e inumExpr) eval(s *state, f file) bool {
	st, ok := fileinfo.Stat(f.fi)
	if !ok {
		return e.n.match(0)
	}
	return e.n.match(int64(st.Ino))
}

// samefileExpr is the -samefile primary.
type samefileExpr struct {
	dev uint64
	ino uint64
}

func (e samefileExpr) eval(s *state, f file) bool {
	st, ok := fileinfo.Stat(f.fi)
	return ok && st.Dev == e.dev && st.Ino == e.ino
}

// sizeExpr is the -size primary.
type sizeExpr struct {
	n    num
	unit int64
}

func (e sizeExpr) eval(s *state, f file) bool {
	size := f.fi.Size()
	units := (size + e.unit - 1) / e.unit
	return e.n.match(units)
}

// emptyExpr is the -empty primary.
type emptyExpr struct{}

func (e emptyExpr) eval(s *state, f file) bool {
	if f.fi.IsDir() {
		entries, err := fs.ReadDir(s.p.FS, f.fsName)
		return err == nil && len(entries) == 0
	}
	return f.fi.Mode().IsRegular() && f.fi.Size() == 0
}

// fileTime returns the time of a file a time selector checks, falling
// back to the modification time, which a file system always reports.
func fileTime(fi fs.FileInfo, which byte) time.Time {
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

// timeExpr is the -atime, -ctime, -mtime, -amin, -cmin and -mmin primary.
type timeExpr struct {
	n       num
	which   byte  // 'a', 'c' or 'm'
	seconds int64 // how many seconds a unit counts, a day or a minute
}

func (e timeExpr) eval(s *state, f file) bool {
	base := s.now
	if s.opts.daystart {
		// Measure from the beginning of today rather than from now.
		n := s.now.Local()
		base = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	}
	age := base.Sub(s.timeOf(f.fi, e.which)).Seconds()
	return e.n.match(int64(age / float64(e.seconds)))
}

// newerExpr is the -newer, -anewer, -cnewer and -newerXY primaries.
type newerExpr struct {
	which byte      // the time of the file checked: 'a', 'c' or 'm'
	ref   time.Time // the reference time
	never bool      // the reference has no time, so the test always fails
}

func (e newerExpr) eval(s *state, f file) bool {
	if e.never {
		return false
	}
	return s.timeOf(f.fi, e.which).After(e.ref)
}

// usedExpr is the -used primary.
type usedExpr struct {
	n num
}

func (e usedExpr) eval(s *state, f file) bool {
	atime := s.timeOf(f.fi, 'a')
	ctime := s.timeOf(f.fi, 'c')
	return e.n.match(int64(atime.Sub(ctime).Seconds() / 86400))
}

// accessExpr is the -readable, -writable and -executable primary, which is
// approximated from the permission bits, as no access control list can be
// consulted.
type accessExpr struct {
	bit fs.FileMode // 0o400 to read, 0o200 to write, 0o100 to execute
}

func (e accessExpr) eval(s *state, f file) bool {
	uid, gid, ok := fsx.Owner(f.fi)
	if !ok {
		return false
	}
	m := f.fi.Mode()
	if uid == currentUser {
		if currentUser == 0 {
			// The superuser reads and writes everything, and executes
			// whatever anyone can.
			if e.bit == 0o400 || e.bit == 0o200 {
				return true
			}
			return m&0o111 != 0
		}
		return m&e.bit != 0
	}
	if gid == currentGroup {
		return m&(e.bit>>3) != 0
	}
	return m&(e.bit>>6) != 0
}

// currentUser and currentGroup are the identifiers the access primaries
// approximate the checks of.
var (
	currentUser  = os.Getuid()
	currentGroup = os.Getgid()
)

// printExpr is the -print, -print0 and -fprint family primary.
type printExpr struct {
	zero bool
	file string // the file to write to, or the standard output
}

func (e printExpr) eval(s *state, f file) bool {
	ending := "\n"
	if e.zero {
		ending = "\x00"
	}
	if _, err := fmt.Fprint(s.out(e.file), f.name, ending); err != nil {
		s.p.errorf("write error: %v", err)
		s.failed = true
	}
	return true
}

// lsExpr is the -ls and -fls primary.
type lsExpr struct {
	file string // the file to write to, or the standard output
}

func (e lsExpr) eval(s *state, f file) bool {
	if err := listFile(s, s.out(e.file), f); err != nil {
		s.p.errorf("write error: %v", err)
		s.failed = true
	}
	return true
}

// printfExpr is the -printf and -fprintf primary.
type printfExpr struct {
	format printfFormat
	file   string // the file to write to, or the standard output
}

func (e printfExpr) eval(s *state, f file) bool {
	if err := e.format.write(s, s.out(e.file), f); err != nil {
		s.p.errorf("write error: %v", err)
		s.failed = true
	}
	return true
}

// pruneExpr is the -prune primary.
type pruneExpr struct{}

func (e pruneExpr) eval(s *state, f file) bool {
	s.pruned = true
	return true
}

// quitExpr is the -quit primary.
type quitExpr struct{}

func (e quitExpr) eval(s *state, f file) bool {
	s.quit = true
	return true
}

// depthExpr is the -depth primary, always true, which the traversal reads
// from the options instead.
type depthExpr struct{}

func (e depthExpr) eval(s *state, f file) bool { return true }

// mountExpr is the -mount primary, always true, which the traversal reads
// from the options instead.
type mountExpr struct{}

func (e mountExpr) eval(s *state, f file) bool { return true }

// xdevExpr is the -xdev primary, always true, which the traversal reads
// from the options instead.
type xdevExpr struct{}

func (e xdevExpr) eval(s *state, f file) bool { return true }

// daystartExpr is the -daystart option, always true, which the time
// primaries read from the options instead.
type daystartExpr struct{}

func (e daystartExpr) eval(s *state, f file) bool { return true }

// followExpr is the -follow option, always true, which makes the traversal
// follow every symbolic link.
type followExpr struct{}

func (e followExpr) eval(s *state, f file) bool { return true }

// ignoreRaceExpr is the -ignore_readdir_race option, always true, which the
// traversal reads from the options instead.
type ignoreRaceExpr struct{}

func (e ignoreRaceExpr) eval(s *state, f file) bool { return true }

// noIgnoreRaceExpr is the -noignore_readdir_race option, always true, which
// the traversal reads from the options instead.
type noIgnoreRaceExpr struct{}

func (e noIgnoreRaceExpr) eval(s *state, f file) bool { return true }

// maxDepthExpr is the -maxdepth option, always true, which the traversal
// reads from the options instead.
type maxDepthExpr struct {
	n int
}

func (e maxDepthExpr) eval(s *state, f file) bool { return true }

// minDepthExpr is the -mindepth option, always true, which the traversal
// reads from the options instead.
type minDepthExpr struct {
	n int
}

func (e minDepthExpr) eval(s *state, f file) bool { return true }

// deleteExpr is the -delete action, which removes the file and reports
// whether it succeeded. It acts after the entries of a directory, as it
// implies -depth.
type deleteExpr struct{}

func (e deleteExpr) eval(s *state, f file) bool {
	err := s.p.FS.Remove(f.fsName)
	if err == nil {
		return true
	}
	if s.opts.ignoreRace && errors.Is(err, fs.ErrNotExist) {
		return true
	}
	s.p.errorf("cannot delete '%s': %v", f.name, patherr.Unwrap(err))
	s.failed = true
	return false
}

// execExpr is the -exec, -execdir, -ok and -okdir primary. In the
// aggregating form, the last argument is replaced by the set of pathnames
// it holds.
type execExpr struct {
	ok   bool
	dir  bool // run in the directory of the file
	plus bool
	args []string
	set  []execItem
}

func (e *execExpr) eval(s *state, f file) bool {
	if e.plus {
		e.set = append(e.set, execItem{name: f.name, fsName: f.fsName})
		if e.setSize() > execSetLimit {
			s.flushExec(e)
		}
		return true
	}
	args := make([]string, len(e.args))
	for i, a := range e.args {
		args[i] = strings.ReplaceAll(a, "{}", e.replace(f))
	}
	if e.ok && !s.confirm(args) {
		return false
	}
	return s.runExec(e.dirExec(f), args) == 0
}

// replace returns what {} stands for: the pathname, or the base name of
// the file in its directory with -execdir.
func (e *execExpr) replace(f file) string {
	if !e.dir {
		return f.name
	}
	return "./" + basename(f.name)
}

// dirExec returns the directory a utility runs in: the one of the file with
// -execdir, and the working directory otherwise.
func (e *execExpr) dirExec(f file) string {
	if !e.dir {
		return ""
	}
	return path.Dir(f.fsName)
}

// setSize returns the size of the set of pathnames aggregated so far.
func (e *execExpr) setSize() int {
	n := 0
	for _, item := range e.set {
		n += len(item.name) + 1
	}
	return n
}

// execSetLimit is the size at which a set of aggregated pathnames is
// flushed, keeping the command lines within the usual ARG_MAX limits.
const execSetLimit = 128 * 1024
