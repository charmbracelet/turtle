// Package printfmt holds the pieces of printf-style format strings shared
// by the printf utility and the -printf and -fprintf primaries of find.
package printfmt

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/turtle/coreutils/internal/numparse"
)

// Spec is the argument number, flags, width and precision of a conversion
// or directive, as scanned by ScanSpec.
type Spec struct {
	// ArgNum is the "%n$" argument number, 0 when absent.
	ArgNum int64
	// Flags holds the flag characters, in the order they appear.
	Flags string
	// Width is the field width, -1 when absent.
	Width int64
	// Prec is the precision, -1 when absent.
	Prec int64
	// Raw holds the scanned specification without the leading '%', so
	// it includes the argument number when one is scanned.
	Raw string
}

// ScanSpec scans the argument number, flags, width and precision of the
// conversion specification at the start of s, which must begin with '%'.
// It returns them and the offset of the conversion character, which equals
// len(s) when the format ends before it. Unless argNum is set, a "%n$"
// argument number is not taken, leaving its digits to a width.
func ScanSpec(s string, argNum bool) (Spec, int) {
	spec := Spec{Width: -1, Prec: -1}
	i := 1
	if argNum {
		start := i
		for i < len(s) && numparse.IsDigit(s[i]) {
			i++
		}
		if i > start && i < len(s) && s[i] == '$' {
			spec.ArgNum = parseNumber(s[start:i])
			i++
		} else {
			i = start
		}
	}
	start := i
	for i < len(s) && strings.IndexByte("-+ #0", s[i]) >= 0 {
		i++
	}
	spec.Flags = s[start:i]
	start = i
	for i < len(s) && numparse.IsDigit(s[i]) {
		i++
	}
	if i > start {
		spec.Width = parseNumber(s[start:i])
	}
	if i < len(s) && s[i] == '.' {
		i++
		spec.Prec = 0
		start = i
		for i < len(s) && numparse.IsDigit(s[i]) {
			i++
		}
		if i > start {
			spec.Prec = parseNumber(s[start:i])
		}
	}
	spec.Raw = s[1:i]
	return spec, i
}

// Octal selects the octal escape sequences an Escapes set takes.
type Octal int8

const (
	// OctalNone takes no octal escape.
	OctalNone Octal = iota
	// OctalPlain takes the "\ddd" escapes, with one to three digits.
	OctalPlain
	// OctalZero takes the "\0ddd" escapes of the b conversion, with the
	// leading zero and up to three digits.
	OctalZero
)

// Escapes selects the escape sequences ScanEscape and Expand take.
type Escapes struct {
	// Octal selects the octal escape sequences.
	Octal Octal
	// Extended also takes the GNU "\e", "\"", "\xHH", "\uHHHH" and
	// "\UHHHHHHHH" escapes.
	Extended bool
}

// Esc is one escape sequence, without the backslash that starts it.
type Esc struct {
	// Lit is what the escape stands for, or the escape as it stands,
	// backslash included, when Unknown is set.
	Lit string
	// Stop is the "\c" escape, which stops all further output.
	Stop bool
	// Unknown marks an escape the Escapes set doesn't take.
	Unknown bool
}

// ScanEscape scans the escape sequence at the start of s, which must hold
// the character following a backslash, so it is never empty. It returns
// the escape and how many bytes of s it spans. An octal value beyond a
// byte is truncated like the C printf function does. The hexadecimal and
// universal escapes are unknown ones for an Escapes set without its
// Extended ones.
func ScanEscape(s string, es Escapes) (Esc, int, error) {
	var esc Esc
	switch c := s[0]; c {
	case 'a':
		esc.Lit = "\a"
	case 'b':
		esc.Lit = "\b"
	case 'c':
		esc.Stop = true
	case 'f':
		esc.Lit = "\f"
	case 'n':
		esc.Lit = "\n"
	case 'r':
		esc.Lit = "\r"
	case 't':
		esc.Lit = "\t"
	case 'v':
		esc.Lit = "\v"
	case '\\':
		esc.Lit = "\\"
	case '"':
		if !es.Extended {
			return unknown(c), 1, nil
		}
		esc.Lit = `"`
	case 'e':
		if !es.Extended {
			return unknown(c), 1, nil
		}
		esc.Lit = "\x1b"
	case 'x':
		if !es.Extended {
			return unknown(c), 1, nil
		}
		v, n := scanHex(s[1:], 2)
		if n == 0 {
			return esc, 1, errors.New("missing hexadecimal number in escape")
		}
		esc.Lit = string([]byte{byte(v)})
		return esc, 1 + n, nil
	case 'u', 'U':
		if !es.Extended {
			return unknown(c), 1, nil
		}
		r, err := unicodeEscape(c, s[1:])
		if err != nil {
			return esc, 0, err
		}
		esc.Lit = string(r)
		n := 4
		if c == 'U' {
			n = 8
		}
		return esc, 1 + n, nil
	default:
		if es.Octal == OctalNone || !numparse.IsOctalDigit(c) ||
			es.Octal == OctalZero && c != '0' {
			return unknown(c), 1, nil
		}
		v := int(c - '0')
		lim := 3
		if es.Octal == OctalZero {
			lim = 4
		}
		n := 1
		for n < lim && n < len(s) && numparse.IsOctalDigit(s[n]) {
			v = v*8 + int(s[n]-'0')
			n++
		}
		esc.Lit = string([]byte{byte(v)})
		return esc, n, nil
	}
	return esc, 1, nil
}

// Expand writes s to out, taking its escape sequences. It reports whether
// a "\c" escape stopped the output, and stops after writing max bytes
// when max is not negative. Escape sequences the set doesn't take are
// written as they stand.
func (es Escapes) Expand(out *strings.Builder, s string, max int) (bool, error) {
	written := 0
	for i := 0; i < len(s); i++ {
		if max >= 0 && written >= max {
			return false, nil
		}
		if s[i] != '\\' || i+1 == len(s) {
			out.WriteByte(s[i])
			written++
			continue
		}
		esc, n, err := ScanEscape(s[i+1:], es)
		if err != nil {
			return false, err
		}
		if esc.Stop {
			return true, nil
		}
		i += n
		n = len(esc.Lit)
		if max >= 0 && written+n > max {
			n = max - written
		}
		out.WriteString(esc.Lit[:n])
		written += n
	}
	return false, nil
}

// unknown returns the Esc of an escape the set doesn't take, which stands
// as it is, backslash included.
func unknown(c byte) Esc {
	return Esc{Lit: "\\" + string(c), Unknown: true}
}

// unicodeEscape evaluates a \u or \U escape, whose escape character is
// followed by exactly four or eight hexadecimal digits. Surrogates are
// invalid universal character names, and values beyond the Unicode range
// are not representable in UTF-8.
func unicodeEscape(esc byte, s string) (rune, error) {
	lim := 4
	if esc == 'U' {
		lim = 8
	}
	v, n := scanHex(s, lim)
	if n < lim {
		return 0, errors.New("missing hexadecimal number in escape")
	}
	if v >= 0xd800 && v <= 0xdfff || v > utf8.MaxRune {
		return 0, fmt.Errorf(`invalid universal character name \%c%0*x`, esc, lim, v)
	}
	return rune(v), nil
}

// scanHex reads up to lim hexadecimal digits from the start of s,
// returning their value and how many digits it read.
func scanHex(s string, lim int) (int64, int) {
	var v int64
	n := 0
	for n < lim && n < len(s) && numparse.IsHexDigit(s[n]) {
		d := s[n]
		switch {
		case d >= '0' && d <= '9':
			d = d - '0'
		case d >= 'a' && d <= 'f':
			d = d - 'a' + 10
		default:
			d = d - 'A' + 10
		}
		v = v*16 + int64(d)
		n++
	}
	return v, n
}

// parseNumber parses digits as an argument number, width or precision,
// saturating only values beyond the range a C int can hold.
func parseNumber(digits string) int64 {
	if digits == "" {
		return 0
	}
	if n, err := strconv.ParseInt(digits, 10, 64); err == nil && n <= math.MaxInt32 {
		return n
	}
	return math.MaxInt32
}
