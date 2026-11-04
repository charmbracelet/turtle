package find

import (
	"bufio"
	"fmt"
	"strings"
)

// runExec runs one utility invocation, reporting its exit status. A failure
// to run it at all sets the failure flag of the traversal.
func (s *state) runExec(args []string) int {
	if s.p.Exec == nil {
		s.p.errorf("no command execution available")
		s.failed = true
		return 1
	}
	status, err := s.p.Exec(s.ctx, args[0], args[1:], s.p.EnvList())
	if err != nil {
		if s.ctx.Err() != nil {
			return 1
		}
		s.p.errorf("%s: %v", args[0], err)
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
	args := append(append([]string{}, e.args[:len(e.args)-1]...), e.set...)
	e.set = e.set[:0]
	if s.runExec(args) != 0 {
		s.execFailed = true
	}
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
