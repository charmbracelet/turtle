// Package durationparse parses the duration operands sleep and timeout
// take: a non-negative number, as strtod would read it, with an optional
// s, m, h or d unit suffix.
package durationparse

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// unitSeconds maps each unit suffix to its length in seconds.
var unitSeconds = map[byte]float64{
	's': 1,
	'm': 60,
	'h': 60 * 60,
	'd': 24 * 60 * 60,
}

// maxSeconds is the largest number of seconds a duration can hold.
const maxSeconds = float64(math.MaxInt64 / int64(time.Second))

// ErrInvalid marks a duration operand which is not a valid one. sleep and
// timeout replace it with the diagnostic of their own.
var ErrInvalid = errors.New("invalid duration")

// Parse converts the duration operand s into a time.Duration, reporting
// an infinite one when infOK is set and the operand is "inf" or a value
// too large to hold. Without it, those operands are invalid instead, as
// sleep rejects them. With infOK, a positive fraction too small to hold
// still elapses, at once, and a negative one is rejected. The returned
// error is always ErrInvalid.
func Parse(s string, infOK bool) (time.Duration, bool, error) {
	mult := 1.0
	if n := len(s); n > 0 {
		if m, ok := unitSeconds[s[n-1]]; ok {
			mult, s = m, s[:n-1]
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if !infOK {
		if err != nil || math.IsNaN(f) || f < 0 ||
			math.IsInf(f, 0) || f > maxSeconds/mult {
			return 0, false, ErrInvalid
		}
		return time.Duration(f * mult * float64(time.Second)), false, nil
	}
	// A range error stands for an overflow to infinity, which strtod
	// accepts; a syntax error does not. An underflow reads as zero, which
	// underflowed below tells from a plain one.
	if err != nil && !errors.Is(err, strconv.ErrRange) || math.IsNaN(f) || f < 0 {
		return 0, false, ErrInvalid
	}
	f *= mult
	if math.IsInf(f, +1) || f > maxSeconds {
		return 0, true, nil
	}
	if f == 0 && underflowed(s) {
		// A positive value too small to hold, like 1e-400, still
		// limits the command, elapsing at once. A negative one is
		// invalid, as GNU rejects it.
		if strings.HasPrefix(s, "-") {
			return 0, false, ErrInvalid
		}
		return 1, false, nil
	}
	d := time.Duration(f * float64(time.Second))
	if f > 0 && d == 0 {
		// A fraction too small to hold still limits the command, at
		// once.
		return 1, false, nil
	}
	return d, false, nil
}

// underflowed reports whether a numeric string which parsed to zero stands
// for a nonzero value, like the 1 of 1e-400 or the a of 0xap-2000. Only
// the mantissa counts, so the digits of an exponent do not.
func underflowed(s string) bool {
	hex := false
	t := s
	if len(t) > 0 && (t[0] == '+' || t[0] == '-') {
		t = t[1:]
	}
	if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
		hex = true
	}
	// The exponent marker is p for hexadecimal and e for decimal. In
	// particular, the e of a hex mantissa is a digit, not a marker.
	marker := "eE"
	if hex {
		marker = "pP"
	}
	if i := strings.IndexAny(t, marker); i >= 0 {
		t = t[:i]
	}
	return strings.ContainsFunc(t, func(r rune) bool {
		switch {
		case r >= '1' && r <= '9':
			return true
		case hex && (r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'):
			return true
		}
		return false
	})
}
