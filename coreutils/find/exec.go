package find

import (
	"bufio"
	"fmt"
	"path"
	"strings"
)

// runExec runs one utility invocation, reporting its exit status. A failure
// to run it at all sets the failure flag of the traversal.
func (s *state) runExec(dir string, args []string) int {
	if s.p.Exec == nil {
		s.p.errorf("no command execution available")
		s.failed = true
		return 1
	}
	status, err := s.p.Exec(s.ctx, dir, args[0], args[1:], s.p.EnvList())
	return execResult(s, args[0], status, err)
}

// execResult reports the status of an invocation, reporting the errors of
// running it at all.
func execResult(s *state, name string, status int, err error) int {
	if err != nil {
		if s.ctx.Err() != nil {
			return 1
		}
		s.p.errorf("%s: %v", name, err)
		s.failed = true
		return 1
	}
	return status
}

// flushExec runs the utility of the aggregating form of -exec on the set of
// pathnames it holds. A nonzero exit status makes find exit nonzero.
func (s *state) flushExec(e *execExpr) {
	if len(e.set) == 0 {
		return
	}
	if !e.dir {
		args := append(append([]string{}, e.args[:len(e.args)-1]...), itemNames(e.set)...)
		e.set = e.set[:0]
		if s.runExec("", args) != 0 {
			s.execFailed = true
		}
		return
	}
	// The files of one invocation share the directory the utility runs in,
	// so the set is split by directory.
	dirs := map[string][]string{}
	var order []string
	for _, item := range e.set {
		dir := path.Dir(item.fsName)
		if _, ok := dirs[dir]; !ok {
			order = append(order, dir)
		}
		dirs[dir] = append(dirs[dir], "./"+path.Base(item.name))
	}
	e.set = e.set[:0]
	for _, dir := range order {
		args := append(append([]string{}, e.args[:len(e.args)-1]...), dirs[dir]...)
		if s.runExec(dir, args) != 0 {
			s.execFailed = true
		}
	}
}

// itemNames returns the pathnames of a set of files.
func itemNames(set []execItem) []string {
	names := make([]string, len(set))
	for i, item := range set {
		names[i] = item.name
	}
	return names
}

// flush runs the utility of every aggregating -exec primary on the set of
// pathnames it still holds.
func (s *state) flush() {
	for _, e := range s.execs {
		s.flushExec(e)
	}
}

// confirm asks for the confirmation of a -ok invocation, reading one line
// from the standard input. An affirmative response in the C locale starts
// with y or Y.
func (s *state) confirm(args []string) bool {
	if s.p.Stdin == nil {
		return false
	}
	if s.stdin == nil {
		s.stdin = bufio.NewReader(s.p.Stdin)
	}
	fmt.Fprint(s.p.Stderr, strings.Join(args, " "), " ? ")
	line, err := s.stdin.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	return len(line) > 0 && (line[0] == 'y' || line[0] == 'Y')
}

// execItem is one file of the set of an aggregating -exec primary.
type execItem struct {
	name   string
	fsName string
}
