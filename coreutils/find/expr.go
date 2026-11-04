package find

import (
	"fmt"
	"io/fs"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils/internal/fileinfo"
	"charm.land/snail/coreutils/internal/filemode"

	"charm.land/snail/fsx"
)

// expr is one node of the expression tree evaluated on every file.
type expr interface {
	eval(s *state, name string, fi fs.FileInfo) bool
}

// andExpr is the conjunction of two expressions, the second one skipped
// when the first is false.
type andExpr struct {
	left, right expr
}

func (e andExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	if !e.left.eval(s, name, fi) {
		return false
	}
	return e.right.eval(s, name, fi)
}

// orExpr is the alternation of two expressions, the second one skipped when
// the first is true.
type orExpr struct {
	left, right expr
}

func (e orExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	if e.left.eval(s, name, fi) {
		return true
	}
	return e.right.eval(s, name, fi)
}

// notExpr negates an expression.
type notExpr struct {
	inner expr
}

func (e notExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	return !e.inner.eval(s, name, fi)
}

// nameExpr is the -name and -iname primary: the basename matches a pattern.
type nameExpr struct {
	re *regexp.Regexp
}

func (e nameExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	return e.re.MatchString(basename(name))
}

// pathMatch is the -path primary: the whole pathname matches a pattern.
type pathMatch struct {
	re *regexp.Regexp
}

func (e pathMatch) eval(s *state, name string, fi fs.FileInfo) bool {
	return e.re.MatchString(name)
}

// typeExpr is the -type primary.
type typeExpr struct {
	typ fs.FileMode
}

func (e typeExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	return fi.Mode().Type() == e.typ
}

// permBits is the set of file mode bits a -perm primary matches.
const permBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// permExpr is the -perm primary. With all set, at least the bits of the
// template must match instead of all of them.
type permExpr struct {
	spec filemode.Spec
	all  bool
}

func (e permExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	want := e.spec(0, fi.IsDir(), 0) & permBits
	have := fi.Mode() & permBits
	if e.all {
		return have&want == want
	}
	return have == want
}

// linksExpr is the -links primary.
type linksExpr struct {
	n num
}

func (e linksExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	nlink := int64(1)
	if st, ok := fileinfo.Stat(fi); ok && st.Nlink != 0 {
		nlink = int64(st.Nlink)
	}
	return e.n.match(nlink)
}

// userExpr is the -user primary.
type userExpr struct {
	uid int
}

func (e userExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	uid, _, ok := fsx.Owner(fi)
	return ok && uid == e.uid
}

// groupExpr is the -group primary.
type groupExpr struct {
	gid int
}

func (e groupExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	_, gid, ok := fsx.Owner(fi)
	return ok && gid == e.gid
}

// nouserExpr is the -nouser primary.
type nouserExpr struct{}

func (e nouserExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	uid, _, ok := fsx.Owner(fi)
	if !ok {
		return false
	}
	_, err := user.LookupId(strconv.Itoa(uid))
	return err != nil
}

// nogroupExpr is the -nogroup primary.
type nogroupExpr struct{}

func (e nogroupExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	_, gid, ok := fsx.Owner(fi)
	if !ok {
		return false
	}
	_, err := user.LookupGroupId(strconv.Itoa(gid))
	return err != nil
}

// sizeExpr is the -size primary.
type sizeExpr struct {
	n     num
	bytes bool
}

func (e sizeExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	size := fi.Size()
	if e.bytes {
		return e.n.match(size)
	}
	return e.n.match((size + 511) / 512)
}

// timeExpr is the -atime, -ctime and -mtime primary.
type timeExpr struct {
	n     num
	which byte
}

func (e timeExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	days := int64(s.now.Sub(s.timeOf(fi, e.which)).Seconds() / 86400)
	return e.n.match(days)
}

// newerExpr is the -newer primary.
type newerExpr struct {
	mtime time.Time
}

func (e newerExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	return fi.ModTime().After(e.mtime)
}

// printExpr is the -print and -print0 primary.
type printExpr struct {
	zero bool
}

func (e printExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	ending := "\n"
	if e.zero {
		ending = "\x00"
	}
	if _, err := fmt.Fprint(s.p.Stdout, name, ending); err != nil {
		s.p.errorf("write error: %v", err)
		s.failed = true
	}
	return true
}

// pruneExpr is the -prune primary.
type pruneExpr struct{}

func (e pruneExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	s.pruned = true
	return true
}

// depthExpr is the -depth primary, always true, which the traversal reads
// from the options instead.
type depthExpr struct{}

func (e depthExpr) eval(s *state, name string, fi fs.FileInfo) bool { return true }

// mountExpr is the -mount primary, always true, which the traversal reads
// from the options instead.
type mountExpr struct{}

func (e mountExpr) eval(s *state, name string, fi fs.FileInfo) bool { return true }

// xdevExpr is the -xdev primary, always true, which the traversal reads
// from the options instead.
type xdevExpr struct{}

func (e xdevExpr) eval(s *state, name string, fi fs.FileInfo) bool { return true }

// execExpr is the -exec and -ok primary. In the aggregating form, the last
// argument is replaced by the set of pathnames it holds.
type execExpr struct {
	ok   bool
	plus bool
	args []string
	set  []string
}

func (e *execExpr) eval(s *state, name string, fi fs.FileInfo) bool {
	if e.plus {
		e.set = append(e.set, name)
		if e.setSize() > execSetLimit {
			s.flushExec(e)
		}
		return true
	}
	args := make([]string, len(e.args))
	for i, a := range e.args {
		args[i] = strings.ReplaceAll(a, "{}", name)
	}
	if e.ok && !s.confirm(args) {
		return false
	}
	return s.runExec(args) == 0
}

// setSize returns the size of the set of pathnames aggregated so far.
func (e *execExpr) setSize() int {
	n := 0
	for _, name := range e.set {
		n += len(name) + 1
	}
	return n
}

// execSetLimit is the size at which a set of aggregated pathnames is
// flushed, keeping the command lines within the usual ARG_MAX limits.
const execSetLimit = 128 * 1024
