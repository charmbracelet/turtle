package chmod

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// ModeSpec computes the new mode of a file from its current mode, whether
// it's a directory, and the file mode creation mask.
type ModeSpec func(cur fs.FileMode, isDir bool, umask fs.FileMode) fs.FileMode

// errInvalidMode is returned for mode operands that are neither a valid octal
// number nor a valid symbolic mode expression.
var errInvalidMode = errors.New("invalid mode")

// ParseMode parses a mode operand, which is either a non-negative octal
// number or a symbolic mode expression.
func ParseMode(s string) (ModeSpec, error) {
	if s == "" {
		return nil, fmt.Errorf("invalid mode: %q", s)
	}
	var (
		spec ModeSpec
		err  error
	)
	if isDigit(s[0]) {
		var mode fs.FileMode
		if mode, err = parseOctal(s); err == nil {
			spec = func(fs.FileMode, bool, fs.FileMode) fs.FileMode { return mode }
		}
	} else {
		var clauses []clause
		if clauses, err = parseSymbolic(s); err == nil {
			spec = func(cur fs.FileMode, isDir bool, umask fs.FileMode) fs.FileMode {
				mode := cur
				for _, c := range clauses {
					mode = c.apply(mode, cur, isDir, umask)
				}
				return mode
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid mode: %q", s)
	}
	return spec, nil
}

// parseOctal parses an absolute mode of one to four octal digits, mapping the
// fourth digit to the set-user-ID, set-group-ID and sticky bits.
func parseOctal(s string) (fs.FileMode, error) {
	if len(s) > 4 {
		return 0, errInvalidMode
	}
	var n uint32
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '7' {
			return 0, errInvalidMode
		}
		n = n<<3 | uint32(s[i]-'0')
	}
	mode := fs.FileMode(n & 0o777)
	if n&0o4000 != 0 {
		mode |= fs.ModeSetuid
	}
	if n&0o2000 != 0 {
		mode |= fs.ModeSetgid
	}
	if n&0o1000 != 0 {
		mode |= fs.ModeSticky
	}
	return mode, nil
}

const (
	whoU uint8 = 1 << iota
	whoG
	whoO
)

// clause is one comma-separated part of a symbolic mode: an optional who and
// the actions applied to it.
type clause struct {
	who     uint8 // a bit set of whoU, whoG and whoO; zero when omitted
	actions []action
}

// action is a single operation of a clause.
type action struct {
	op    byte   // '+', '-' or '='
	perms string // any of "rwxXst"
	copy  byte   // a permcopy of 'u', 'g' or 'o', or zero
}

// parseSymbolic parses a symbolic mode expression into its clauses.
func parseSymbolic(s string) ([]clause, error) {
	var clauses []clause
	for part := range strings.SplitSeq(s, ",") {
		c, err := parseClause(part)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, c)
	}
	return clauses, nil
}

// parseClause parses a clause: an optional wholist followed by one or more
// actions.
func parseClause(s string) (clause, error) {
	var c clause
	i := 0
who:
	for i < len(s) {
		switch s[i] {
		case 'u':
			c.who |= whoU
		case 'g':
			c.who |= whoG
		case 'o':
			c.who |= whoO
		case 'a':
			c.who = whoU | whoG | whoO
		default:
			break who
		}
		i++
	}
	for i < len(s) {
		a, next, err := parseAction(s, i)
		if err != nil {
			return c, err
		}
		c.actions = append(c.actions, a)
		i = next
	}
	if len(c.actions) == 0 {
		return c, errInvalidMode
	}
	return c, nil
}

// parseAction parses one action starting at i: an op optionally followed by a
// perm list or by a single permcopy letter. It returns the index right after
// the action.
func parseAction(s string, i int) (action, int, error) {
	a := action{op: s[i]}
	if !isOp(a.op) {
		return a, i, errInvalidMode
	}
	i++
	switch {
	case i < len(s) && isPermCopy(s[i]):
		a.copy = s[i]
		i++
	case i < len(s) && isPerm(s[i]):
		for i < len(s) && isPerm(s[i]) {
			a.perms += s[i : i+1]
			i++
		}
	}
	if i < len(s) && !isOp(s[i]) {
		return a, i, errInvalidMode
	}
	return a, i, nil
}

// apply applies the actions of the clause to mode, in order.
func (c clause) apply(mode, orig fs.FileMode, isDir bool, umask fs.FileMode) fs.FileMode {
	for _, a := range c.actions {
		mode = a.apply(c.who, mode, orig, isDir, umask)
	}
	return mode
}

// apply applies the action to mode, returning the result. permcopy reads the
// current mode, while X checks the original one.
func (a action) apply(who uint8, mode, orig fs.FileMode, isDir bool, umask fs.FileMode) fs.FileMode {
	var bits, special fs.FileMode
	if a.copy != 0 {
		src := (mode.Perm() >> shift(a.copy)) & 7
		for _, s := range classShifts(who) {
			bits |= src << s
		}
	} else {
		for _, p := range a.perms {
			switch p {
			case 'r':
				bits |= 0o444
			case 'w':
				bits |= 0o222
			case 'x':
				bits |= 0o111
			case 'X':
				if isDir || orig.Perm()&0o111 != 0 {
					bits |= 0o111
				}
			case 's':
				if who&whoU != 0 || who == 0 {
					special |= fs.ModeSetuid
				}
				if who&whoG != 0 || who == 0 {
					special |= fs.ModeSetgid
				}
			case 't':
				special |= fs.ModeSticky
			}
		}
		bits &= whoMask(who)
	}
	if who == 0 {
		bits &^= umask
	}

	switch a.op {
	case '+':
		mode |= bits | special
	case '-':
		mode &^= bits | special
	case '=':
		if who == 0 {
			mode &^= fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
		} else {
			mode &^= whoMask(who)
		}
		mode |= bits | special
	}
	return mode
}

// whoMask returns the permission bits of the classes selected by who, or of
// all classes when no who was given.
func whoMask(who uint8) fs.FileMode {
	if who == 0 {
		return fs.ModePerm
	}
	var mask fs.FileMode
	if who&whoU != 0 {
		mask |= 0o700
	}
	if who&whoG != 0 {
		mask |= 0o070
	}
	if who&whoO != 0 {
		mask |= 0o007
	}
	return mask
}

// classShifts returns the bit shift of each class selected by who, or of all
// classes when no who was given.
func classShifts(who uint8) []uint {
	if who == 0 {
		return []uint{6, 3, 0}
	}
	var shifts []uint
	if who&whoU != 0 {
		shifts = append(shifts, 6)
	}
	if who&whoG != 0 {
		shifts = append(shifts, 3)
	}
	if who&whoO != 0 {
		shifts = append(shifts, 0)
	}
	return shifts
}

// shift returns the bit shift of a class letter.
func shift(class byte) uint {
	switch class {
	case 'u':
		return 6
	case 'g':
		return 3
	}
	return 0
}

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isOp(c byte) bool       { return c == '+' || c == '-' || c == '=' }
func isPerm(c byte) bool     { return strings.ContainsRune("rwxXst", rune(c)) }
func isPermCopy(c byte) bool { return c == 'u' || c == 'g' || c == 'o' }
