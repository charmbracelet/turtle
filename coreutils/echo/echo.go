// Package echo implements the POSIX echo utility.
//
// As in most modern implementations (GNU, BusyBox, mvdan/sh), a leading
// operand consisting of a '-' followed only by characters from the set
// {'e', 'E', 'n'} acts as flags: '-n' suppresses the trailing newline, while
// '-e' and '-E' enable and disable the interpretation of backslash escapes,
// which is disabled by default.
package echo

import (
	"context"
	"io"
	"strings"

	"charm.land/snail/coreutils"
)

// Prog is the echo core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the given arguments to standard output, separated by single
// spaces and followed by a newline.
func (p *Prog) Run(_ context.Context, args ...string) error {
	newline, expand, operands := parseFlags(args)

	var out strings.Builder
	for i, operand := range operands {
		if i > 0 {
			out.WriteByte(' ')
		}
		if !expand {
			out.WriteString(operand)
			continue
		}
		if !writeEscaped(&out, operand) {
			newline = false
			break
		}
	}
	if newline {
		out.WriteByte('\n')
	}
	if _, err := io.WriteString(p.Stdout, out.String()); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// parseFlags consumes leading operands that act as flags: those made of a '-'
// followed by one or more characters from the set {'n', 'e', 'E'}.
func parseFlags(args []string) (newline, expand bool, operands []string) {
	newline, expand = true, false
	for len(args) > 0 && isFlags(args[0]) {
		for _, flag := range args[0][1:] {
			switch flag {
			case 'n':
				newline = false
			case 'e':
				expand = true
			case 'E':
				expand = false
			}
		}
		args = args[1:]
	}
	return newline, expand, args
}

func isFlags(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for i := 1; i < len(arg); i++ {
		switch arg[i] {
		case 'n', 'e', 'E':
		default:
			return false
		}
	}
	return true
}

// writeEscaped writes operand to out, interpreting the XSI backslash escape
// sequences. It returns false when a '\c' sequence is found, meaning all
// further output should be suppressed.
func writeEscaped(out *strings.Builder, operand string) bool {
	for i := 0; i < len(operand); i++ {
		c := operand[i]
		if c != '\\' || i+1 == len(operand) {
			out.WriteByte(c)
			continue
		}
		i++
		switch c = operand[i]; c {
		case 'a':
			c = '\a'
		case 'b':
			c = '\b'
		case 'c':
			return false
		case 'f':
			c = '\f'
		case 'n':
			c = '\n'
		case 'r':
			c = '\r'
		case 't':
			c = '\t'
		case 'v':
			c = '\v'
		case '\\':
		case '0':
			// \0num: an octal number of up to three digits.
			value := 0
			j := i + 1
			for ; j < len(operand) && j <= i+3 && isOctalDigit(operand[j]); j++ {
				value = value*8 + int(operand[j]-'0')
			}
			out.WriteByte(byte(value))
			i = j - 1
			continue
		default:
			// Unrecognized sequence: keep the backslash literally.
			out.WriteByte('\\')
		}
		out.WriteByte(c)
	}
	return true
}

func isOctalDigit(c byte) bool {
	return c >= '0' && c <= '7'
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
