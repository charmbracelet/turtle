// Package expr implements the POSIX expr utility, with the string
// keywords and the + quoting token of the GNU one.
//
// The whole command line is parsed as a single expression, in which each
// token is a separate argument. Evaluation follows the precedence of the
// operators, all of which are left-associative, evaluating immediately and
// short-circuiting the | and & connectives as GNU expr does.
package expr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// Prog is the expr core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run evaluates the expression given as arguments and writes the result to
// the standard output, even when it is null or zero. It exits with status 1
// when the result is null or zero, 2 when the expression is invalid, and 3
// when the evaluation fails, like on an out of range numeric result or a
// failed write.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "expr", usage, getopt.Set{}, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// expr takes no options of its own; a leading "--" is skipped, so the
	// expression may start with a negative integer or a string holding a
	// leading minus.
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		err := errors.New("missing operand")
		stdopts.Errorf(p.Runtime, "expr", "%v", err)
		stdopts.TryHelp(p.Runtime, "expr")
		return &coreutils.ExitError{Code: 2, Err: err}
	}
	v, err := evaluate(args)
	if err != nil {
		status := 2
		if fail, ok := errors.AsType[*failError](err); ok {
			status = fail.status
		}
		stdopts.Errorf(p.Runtime, "expr", "%v", err)
		return &coreutils.ExitError{Code: status, Err: err}
	}
	if _, err := io.WriteString(p.Stdout, v.text()+"\n"); err != nil {
		// GNU reports a failed write through close_stdout, exiting with
		// its EXPR_FAILURE status.
		err = fmt.Errorf("write error: %w", err)
		stdopts.Errorf(p.Runtime, "expr", "%v", err)
		return &coreutils.ExitError{Code: 3, Err: err}
	}
	if isNull(v) {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// failError is a failed evaluation, with the exit status it asks for: 2
// when the expression is invalid, 3 when the evaluation itself failed, as
// on a numeric result out of range.
type failError struct {
	status int
	msg    string
}

func (e *failError) Error() string { return e.msg }

// syntaxErrf builds an invalid expression error, which exits with status 2.
func syntaxErrf(format string, args ...any) error {
	return &failError{status: 2, msg: fmt.Sprintf(format, args...)}
}

// rangeErrf builds an out of range numeric error, which exits with status
// 3. GNU expr avoids it with its multiple-precision arithmetic; when s is
// not empty it names the operand which didn't fit.
func rangeErrf(s string) error {
	if s == "" {
		return &failError{status: 3, msg: "Numerical result out of range"}
	}
	return &failError{status: 3, msg: s + ": Numerical result out of range"}
}

// quote wraps s in single quotes, escaping the inner ones, as GNU quotes
// the arguments of its diagnostics.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// usage describes the expression language of expr.
const usage = `Usage: expr EXPRESSION
  or:  expr OPTION
Print the value of EXPRESSION to standard output.  A blank line below
separates increasing precedence groups.  EXPRESSION may be:

  ARG1 | ARG2       ARG1 if it is neither null nor 0, otherwise ARG2

  ARG1 & ARG2       ARG1 if neither argument is null or 0, otherwise 0

  ARG1 < ARG2       ARG1 is less than ARG2
  ARG1 <= ARG2      ARG1 is less than or equal to ARG2
  ARG1 = ARG2       ARG1 is equal to ARG2
  ARG1 != ARG2      ARG1 is unequal to ARG2
  ARG1 >= ARG2      ARG1 is greater than or equal to ARG2
  ARG1 > ARG2       ARG1 is greater than ARG2

  ARG1 + ARG2       arithmetic sum of ARG1 and ARG2
  ARG1 - ARG2       arithmetic difference of ARG1 and ARG2

  ARG1 * ARG2       arithmetic product of ARG1 and ARG2
  ARG1 / ARG2       arithmetic quotient of ARG1 divided by ARG2
  ARG1 % ARG2       arithmetic remainder of ARG1 divided by ARG2

  STRING : REGEXP   anchored pattern match of REGEXP in STRING

  match STRING REGEXP        same as STRING : REGEXP
  substr STRING POS LENGTH   substring of STRING, POS counted from 1
  index STRING CHARS         index in STRING where any CHARS is found, or 0
  length STRING              length of STRING
  + TOKEN                    interpret TOKEN as a string, even if it is a
                               keyword like 'match' or an operator like '/'

  ( EXPRESSION )             value of EXPRESSION

Comparisons are arithmetic if both ARGs are numbers, else lexicographical.
Pattern matches return the string matched between \( and \) or null; if
\( and \) are not used, they return the number of characters matched or 0.

Exit status is 0 if EXPRESSION is neither null nor 0, 1 if EXPRESSION is null
or 0, 2 if EXPRESSION is syntactically invalid, and 3 if an error occurred.

` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
