package ls

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/snail/coreutils/internal/sizeparse"
)

// sizeSpec describes how a size is written: as the count of the units of a
// fixed number of bytes, with the size letters a suffix-only spec stands
// for appended to every number, or scaled to the largest unit of a base it
// reads well in.
type sizeSpec struct {
	unit      int64  // bytes each written number counts, one when scaling
	suffix    string // what a suffix-only spec appends to every number
	autoscale bool   // scale to the largest unit of base
	base      int64  // the base autoscale divides by, 1024 or 1000
}

// byteSpec writes sizes in bytes, and kibibyteSpec in kibibytes, the units
// the sizes of the long format and the block counts default to.
var (
	byteSpec     = sizeSpec{unit: 1}
	kibibyteSpec = sizeSpec{unit: 1024}
)

// humanSpec returns the spec of -h, which scales in the powers of 1024,
// or the one of --si, which uses the powers of 1000.
func humanSpec(si bool) sizeSpec {
	base := int64(1024)
	if si {
		base = 1000
	}
	return sizeSpec{unit: 1, autoscale: true, base: base}
}

// parseBlockSize parses the argument of --block-size and of the block
// size environment variables: the words "human-readable" and "si", a size
// with an optional multiplicative suffix, or a bare suffix, which scales
// by it and appends what it stands for to every number. A leading
// apostrophe, which asks for the thousands separators no locale here has,
// is accepted and ignored.
func parseBlockSize(word string) (sizeSpec, error) {
	word = strings.TrimPrefix(word, "'")
	switch word {
	case "human-readable":
		return humanSpec(false), nil
	case "si":
		return humanSpec(true), nil
	}
	digits, suffix := splitDigits(word)
	if suffix == "" {
		if unit, err := sizeparse.Parse(word); err == nil {
			return fixedSpec(unit, word)
		}
	} else if _, err := sizeparse.Parse("1" + suffix); err == nil {
		if digits == "" {
			return suffixSpec(suffix), nil
		}
		if unit, err := sizeparse.Parse(word); err == nil {
			return fixedSpec(unit, word)
		}
	}
	return sizeSpec{}, fmt.Errorf("invalid --block-size argument '%s'", word)
}

// fixedSpec returns the spec of a plain unit, which a zero isn't.
func fixedSpec(unit int64, word string) (sizeSpec, error) {
	if unit <= 0 {
		return sizeSpec{}, fmt.Errorf("invalid --block-size argument '%s'", word)
	}
	return sizeSpec{unit: unit}, nil
}

// suffixSpec returns the spec of a bare suffix: a fixed unit of the bytes
// it multiplies, with the size letters it stands for appended to every
// number.
func suffixSpec(suffix string) sizeSpec {
	mult, _ := sizeparse.Parse("1" + suffix)
	base, withB := int64(1024), false
	if strings.HasSuffix(suffix, "iB") {
		base, withB = 1024, true
	} else if strings.HasSuffix(suffix, "B") {
		base, withB = 1000, true
	}
	exponent := 0
	for power := int64(1); power < mult; power *= base {
		exponent++
	}
	letters := sizeLetter(exponent, base)
	if withB {
		if base == 1024 {
			letters += "iB"
		} else {
			letters += "B"
		}
	}
	return sizeSpec{unit: mult, suffix: letters}
}

// splitDigits splits a word into its leading decimal digits and the rest.
func splitDigits(word string) (digits, rest string) {
	for i := 0; i < len(word); i++ {
		if word[i] < '0' || word[i] > '9' {
			return word[:i], word[i:]
		}
	}
	return word, ""
}

// powerLetters holds the size letter of each power of 1024 and of 1000,
// the first after a plain byte count. The kilo letter is special: it's
// the lowercase k in the powers of 1000, and the uppercase K in the ones
// of 1024.
const powerLetters = "KMGTPEZYRQ"

// sizeLetter returns the letter of the given power of base, starting from
// one at the first power.
func sizeLetter(exponent int, base int64) string {
	letter := powerLetters[exponent-1 : exponent]
	if base != 1024 && exponent == 1 {
		letter = "k"
	}
	return letter
}

// format writes n bytes as the spec describes, rounding a count that
// doesn't divide by the unit up, the way the GNU tools do.
func (s sizeSpec) format(n int64) string {
	if n < 0 {
		n = 0
	}
	if s.autoscale {
		return humanize(n, s.base)
	}
	count := n / s.unit
	if n%s.unit > 0 {
		count++
	}
	return strconv.FormatInt(count, 10) + s.suffix
}

// humanize writes n bytes as the largest unit of base it reads well in: a
// plain count below the base, then one decimal place under ten and a whole
// number from it on. A fractional count is rounded up to the next tenth,
// and the rounding carries to the next integer and unit.
func humanize(n int64, base int64) string {
	if n < base {
		return strconv.FormatInt(n, 10)
	}
	var tenths int64
	amt, rounding, exponent := n, 0, 0
	for base <= amt && exponent < len(powerLetters) {
		r10 := (amt%base)*10 + tenths
		r2 := (r10%base)*2 + int64(rounding/2)
		amt /= base
		tenths = r10 / base
		if r2 < base {
			rounding = boolInt(r2+int64(rounding) != 0)
		} else {
			rounding = 2 + boolInt(base < r2+int64(rounding))
		}
		exponent++
	}
	if amt < 10 {
		if rounding > 0 {
			tenths++
			rounding = 0
			if tenths == 10 {
				amt++
				tenths = 0
			}
		}
		if amt < 10 {
			return fmt.Sprintf("%d.%d%s", amt, tenths, sizeLetter(exponent, base))
		}
	}
	if tenths+int64(rounding) > 0 {
		amt++
		if amt == base && exponent < len(powerLetters) {
			return "1.0" + sizeLetter(exponent+1, base)
		}
	}
	return strconv.FormatInt(amt, 10) + sizeLetter(exponent, base)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
