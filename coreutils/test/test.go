// Package test implements the POSIX test utility.
//
// The expression is evaluated with the POSIX algorithm based on the number of
// arguments, falling back to a general grammar with the '!', '-a' and '-o'
// operators and '(' and ')' grouping for the results that algorithm leaves
// unspecified.
package test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// Prog is the test core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run evaluates the given expression, exiting with status 0 when it is true,
// 1 when it is false, and 2 when it is not a valid one.
func (p *Prog) Run(_ context.Context, args ...string) error {
	ok, err := p.evaluate(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// errNoFS is returned when a file primary is evaluated without a file system.
var errNoFS = errors.New("no file system available")

// evaluate computes the result of the expression, following the POSIX
// algorithm based on the number of arguments and falling back to the general
// grammar for the results it leaves unspecified.
func (p *Prog) evaluate(args []string) (bool, error) {
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		return args[0] != "", nil
	case 2:
		switch {
		case args[0] == "!":
			return args[1] == "", nil
		case isUnary(args[0]):
			return p.unary(args[0], args[1])
		}
	case 3:
		switch {
		case isBinary(args[1]):
			return p.binary(args[0], args[1], args[2])
		case args[0] == "!":
			return negate(p.evaluate(args[1:]))
		case args[0] == "(" && args[2] == ")":
			return p.evaluate(args[1:2])
		}
	case 4:
		switch {
		case args[0] == "!":
			return negate(p.evaluate(args[1:]))
		case args[0] == "(" && args[3] == ")":
			return p.evaluate(args[1:3])
		}
	}
	return p.parse(args)
}

func negate(v bool, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	return !v, nil
}

// parser evaluates an expression with the general grammar: '-o' has the
// lowest precedence, '-a' a higher one, and '!', the parentheses and the
// primaries the highest.
type parser struct {
	p    *Prog
	args []string
	pos  int
}

// parse evaluates the whole expression, failing when arguments are left over.
func (p *Prog) parse(args []string) (bool, error) {
	ps := &parser{p: p, args: args}
	v, err := ps.parseOr()
	if err != nil {
		return false, err
	}
	if ps.pos < len(args) {
		return false, fmt.Errorf("unexpected argument %q", args[ps.pos])
	}
	return v, nil
}

// peek returns the next argument and whether one is left.
func (ps *parser) peek() (string, bool) {
	if ps.pos >= len(ps.args) {
		return "", false
	}
	return ps.args[ps.pos], true
}

func (ps *parser) parseOr() (bool, error) {
	left, err := ps.parseAnd()
	if err != nil {
		return false, err
	}
	for arg, ok := ps.peek(); ok && arg == "-o"; arg, ok = ps.peek() {
		ps.pos++
		right, err := ps.parseAnd()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (ps *parser) parseAnd() (bool, error) {
	left, err := ps.parseTerm()
	if err != nil {
		return false, err
	}
	for arg, ok := ps.peek(); ok && arg == "-a"; arg, ok = ps.peek() {
		ps.pos++
		right, err := ps.parseTerm()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (ps *parser) parseTerm() (bool, error) {
	arg, ok := ps.peek()
	if !ok {
		return false, errors.New("argument expected")
	}
	if arg == "!" {
		ps.pos++
		return negate(ps.parseTerm())
	}
	if arg == "(" {
		ps.pos++
		v, err := ps.parseOr()
		if err != nil {
			return false, err
		}
		if arg, ok := ps.peek(); !ok || arg != ")" {
			return false, errors.New("')' expected")
		}
		ps.pos++
		return v, nil
	}
	return ps.parsePrimary()
}

func (ps *parser) parsePrimary() (bool, error) {
	if ps.pos >= len(ps.args) {
		return false, errors.New("argument expected")
	}
	op := ps.args[ps.pos]
	if ps.pos+2 < len(ps.args) && isBinary(ps.args[ps.pos+1]) {
		v, err := ps.p.binary(op, ps.args[ps.pos+1], ps.args[ps.pos+2])
		ps.pos += 3
		return v, err
	}
	if isUnary(op) && ps.pos+1 < len(ps.args) {
		v, err := ps.p.unary(op, ps.args[ps.pos+1])
		ps.pos += 2
		return v, err
	}
	ps.pos++
	return op != "", nil
}

// unaryOps are the unary primary operators.
var unaryOps = map[string]bool{
	"-b": true, "-c": true, "-d": true, "-e": true, "-f": true,
	"-g": true, "-h": true, "-L": true, "-n": true, "-p": true,
	"-r": true, "-s": true, "-S": true, "-t": true, "-u": true,
	"-w": true, "-x": true, "-z": true,
}

// binaryOps are the binary primary operators. The '-a' and '-o' connectives
// are left out, so they are only ever handled by the parser precedence.
var binaryOps = map[string]bool{
	"=": true, "!=": true, "<": true, ">": true,
	"-eq": true, "-ne": true, "-gt": true, "-ge": true, "-lt": true, "-le": true,
	"-nt": true, "-ot": true, "-ef": true,
}

func isUnary(op string) bool { return unaryOps[op] }

func isBinary(op string) bool { return binaryOps[op] }

// unary evaluates a unary primary against its single operand.
func (p *Prog) unary(op, arg string) (bool, error) {
	switch op {
	case "-n":
		return arg != "", nil
	case "-z":
		return arg == "", nil
	case "-t":
		// A descriptor that isn't a number is not a valid one.
		fd, err := parseInteger(arg)
		if err != nil || fd < 0 {
			return false, nil
		}
		return p.Terminal(int(fd)), nil
	}
	if p.FS == nil {
		return false, errNoFS
	}
	name := filepath.ToSlash(p.ResolvePath(arg))
	if op == "-h" || op == "-L" {
		fi, err := p.FS.Lstat(name)
		return err == nil && fi.Mode()&fs.ModeSymlink != 0, nil
	}
	fi, err := fs.Stat(p.FS, name)
	if err != nil {
		return false, nil
	}
	return fileTest(op, fi), nil
}

// fileTest evaluates the file primaries against a stat result, which followed
// any symbolic link: a primary that cannot be resolved is false.
func fileTest(op string, fi fs.FileInfo) bool {
	mode := fi.Mode()
	switch op {
	case "-e":
		return true
	case "-f":
		return mode.IsRegular()
	case "-d":
		return mode.IsDir()
	case "-s":
		return fi.Size() > 0
	case "-b":
		return mode&fs.ModeDevice != 0 && mode&fs.ModeCharDevice == 0
	case "-c":
		return mode&fs.ModeCharDevice != 0
	case "-p":
		return mode&fs.ModeNamedPipe != 0
	case "-S":
		return mode&fs.ModeSocket != 0
	case "-u":
		return mode&fs.ModeSetuid != 0
	case "-g":
		return mode&fs.ModeSetgid != 0
	case "-r":
		return mode.Perm()&0o444 != 0
	case "-w":
		return mode.Perm()&0o222 != 0
	case "-x":
		return mode.Perm()&0o111 != 0
	}
	return false
}

// binary evaluates a binary primary between its two operands.
func (p *Prog) binary(a, op, b string) (bool, error) {
	switch op {
	case "=":
		return a == b, nil
	case "!=":
		return a != b, nil
	case "<":
		return a < b, nil
	case ">":
		return a > b, nil
	case "-eq", "-ne", "-gt", "-ge", "-lt", "-le":
		return numeric(a, op, b)
	}
	return p.fileBinary(a, op, b)
}

// numeric evaluates the integer comparison primaries.
func numeric(a, op, b string) (bool, error) {
	x, err := parseInteger(a)
	if err != nil {
		return false, fmt.Errorf("integer expression expected before %q", op)
	}
	y, err := parseInteger(b)
	if err != nil {
		return false, fmt.Errorf("integer expression expected before %q", op)
	}
	switch op {
	case "-eq":
		return x == y, nil
	case "-ne":
		return x != y, nil
	case "-gt":
		return x > y, nil
	case "-ge":
		return x >= y, nil
	case "-lt":
		return x < y, nil
	case "-le":
		return x <= y, nil
	}
	return false, nil
}

// parseInteger parses an operand of an integer comparison, which the POSIX
// spec defines as only an optional unary minus followed by digits.
func parseInteger(s string) (int64, error) {
	digits := strings.TrimPrefix(s, "-")
	if !isDigits(digits) {
		return 0, errors.New("not an integer")
	}
	return strconv.ParseInt(s, 10, 64)
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// fileBinary evaluates the file comparison primaries, which follow symbolic
// links: an operand that cannot be resolved counts as missing.
func (p *Prog) fileBinary(a, op, b string) (bool, error) {
	if p.FS == nil {
		return false, errNoFS
	}
	fi, fia := fs.Stat(p.FS, filepath.ToSlash(p.ResolvePath(a)))
	fj, fib := fs.Stat(p.FS, filepath.ToSlash(p.ResolvePath(b)))
	switch op {
	case "-nt":
		switch {
		case fia != nil:
			return false, nil
		case fib != nil:
			return true, nil
		}
		return fi.ModTime().After(fj.ModTime()), nil
	case "-ot":
		switch {
		case fib != nil:
			return false, nil
		case fia != nil:
			return true, nil
		}
		return fi.ModTime().Before(fj.ModTime()), nil
	case "-ef":
		if fia != nil || fib != nil {
			return false, nil
		}
		return sameFile(fi, fj), nil
	}
	return false, nil
}

// sameFile reports whether two stat results describe the same file. The
// memory file system shares entries between links, while the operating system
// one compares their device and inode numbers.
func sameFile(a, b fs.FileInfo) bool {
	if ma, ok := a.Sys().(*fsx.MemFile); ok {
		mb, ok := b.Sys().(*fsx.MemFile)
		return ok && ma == mb
	}
	return os.SameFile(a, b)
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "test: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
