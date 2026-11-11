package od

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// The long options of od which have no short form stand for private runes.
const (
	optEndian rune = 0x100 + iota
	optTraditional
)

// optSet describes the options of od. The arguments of -w and -S are
// optional, but only in their long forms: a short -w without an attached
// argument never takes the next one, the way GNU reads it, and a short -S
// takes it, the way GNU reads that one.
var optSet = getopt.Set{
	Valued:   "AjNt" + string(optEndian),
	Optional: "wS",
	Long: map[string]rune{
		"address-radix":     'A',
		"endian":            optEndian,
		"format":            't',
		"output-duplicates": 'v',
		"read-bytes":        'N',
		"skip-bytes":        'j',
		"strings":           'S',
		"traditional":       optTraditional,
		"width":             'w',
	},
}

// usage describes the options of od.
const usage = `Usage: od [OPTION]... [FILE]...
  or:  od [-abcdfilosx]... [FILE] [[+]OFFSET[.][b]]
  or:  od --traditional [OPTION]... [FILE] [[+]OFFSET[.][b] [+][LABEL[.][b]]]
Write an unambiguous representation of each FILE, or standard input, to
standard output. Each line of output consists of the offset in the input,
followed by groups of data from the file. With more than one FILE
argument, concatenate them in the listed order to form the input. With no
FILE, or when FILE is -, read standard input.

If first and second call formats both apply, the second format is assumed
if the last operand begins with + or (if there are 2 operands) a digit.
An OFFSET operand means -j OFFSET. LABEL is the pseudo-address at first
byte printed, incremented when dump is progressing. For OFFSET and LABEL,
a 0x or 0X prefix indicates hexadecimal; suffixes may be . for octal and b
for multiply by 512.

  -A, --address-radix=RADIX
         select the base in which file offsets are printed: d for decimal,
         o for octal, x for hexadecimal or n for none
      --endian=ORDER
         reorder the input bytes of each unit according to ORDER, either
         big or little
  -j, --skip-bytes=BYTES
         skip BYTES input bytes before formatting and writing
  -N, --read-bytes=BYTES
         output at most BYTES input bytes
  -S, --strings[=BYTES]
         output only the NUL terminated strings of at least BYTES printable
         characters, 3 when BYTES is not given
  -t, --format=TYPE
         select the format of the output data; TYPE may combine a, c, d, f,
         o, u and x, each optionally followed by a size and z to append a
         printable character column
  -v, --output-duplicates
         output consecutive lines that are identical, instead of replacing
         them with '*'
  -w, --width[=BYTES]
         output BYTES input bytes per output line, 32 when BYTES is not
         given
      --traditional
         accept the OFFSET and LABEL operands of the third form above
  -a
         same as -t a
  -b
         same as -t o1
  -c
         same as -t c
  -d
         same as -t u2
  -f
         same as -t fF
  -i
         same as -t dI
  -l
         same as -t dL
  -o
         same as -t o2
  -s
         same as -t d2
  -x
         same as -t x2
` + stdopts.Tail

type options struct {
	address     byte // the radix of the offsets, as -A selects
	skip        uint64
	count       int64 // the input bytes to output, -1 for no limit
	all         bool  // -v
	types       []outputType
	stringMin   int // -S, -1 when not dumping strings
	width       int // -w, 0 when not given
	swap        bool
	traditional bool
	label       uint64
	labelSet    bool
	// modern records whether an option which hides the offset and label
	// operands of the other forms was given.
	modern bool
}

// expandBare turns the short options whose argument getopt would read from
// the next argument into their long forms with an empty value, so that the
// argument which follows them stays an operand: a -w wherever it appears,
// the way GNU reads it, and a -S which ends the arguments, where the next
// argument GNU's own -S would take does not exist.
func expandBare(args []string) []string {
	out := make([]string, 0, len(args))
	scanning, value := true, false
	for k, arg := range args {
		switch {
		case value:
			value = false // the argument of the option before it
			out = append(out, arg)
		case !scanning || arg == "-" || arg == "--" || !strings.HasPrefix(arg, "-"):
			// Options ended: an operand which looks like one stays one.
			scanning = false
			out = append(out, arg)
		default:
			if parts, ok := splitBareOption(arg, k == len(args)-1); ok {
				out = append(out, parts...)
				continue
			}
			out = append(out, arg)
			value = takesNext(arg)
		}
	}
	return out
}

// takesNext reports whether the option token arg reads the argument which
// follows it, because it takes one and has none attached.
func takesNext(arg string) bool {
	if strings.HasPrefix(arg, "--") {
		name, _, given := strings.Cut(arg[2:], "=")
		opt, ok := optSet.Long[name]
		return ok && !given && strings.ContainsRune(optSet.Valued, opt)
	}
	for i := 1; i < len(arg); i++ {
		if strings.ContainsRune(optSet.Valued, rune(arg[i])) ||
			strings.ContainsRune(optSet.Optional, rune(arg[i])) {
			return i == len(arg)-1
		}
	}
	return false
}

// splitBareOption splits the short option cluster arg when it ends in a -w
// without an argument, or in a -S which ends the arguments, as "-vw" into
// "-v" and "--width=".
func splitBareOption(arg string, last bool) ([]string, bool) {
	if len(arg) < 2 || arg[1] == '-' {
		return nil, false
	}
	for i := 1; i < len(arg); i++ {
		long := ""
		switch arg[i] {
		case 'w':
			long = "width"
		case 'S':
			if last {
				long = "strings"
			}
		}
		if long == "" {
			if strings.ContainsRune(optSet.Valued, rune(arg[i])) {
				return nil, false // the rest of the cluster is its argument
			}
			continue
		}
		if i != len(arg)-1 {
			return nil, false // an argument is attached to the option
		}
		if i == 1 {
			return []string{"--" + long + "="}, true
		}
		return []string{arg[:i], "--" + long + "="}, true
	}
	return nil, false
}

func parseFlags(args []string) (options, []string, error) {
	opts := options{address: 'o', count: -1, stringMin: -1}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'A':
			if len(value) != 1 || !strings.ContainsRune("doxn", rune(value[0])) {
				return errors.New("invalid address base")
			}
			opts.address, opts.modern = value[0], true
		case 'j':
			n, err := parseBytes(value)
			if err != nil {
				return errors.New("invalid skip count")
			}
			opts.skip, opts.modern = n, true
		case 'N':
			n, err := parseBytes(value)
			if err != nil || n > math.MaxInt64 {
				return errors.New("invalid byte count")
			}
			opts.count, opts.modern = int64(n), true
		case 'S':
			opts.stringMin, opts.modern = 3, true
			if value != "" {
				n, err := parseBytes(value)
				if err != nil || n > math.MaxInt-1 {
					return errors.New("invalid string length")
				}
				opts.stringMin = int(n)
			}
		case 't':
			types, err := parseTypes(value)
			if err != nil {
				return err
			}
			opts.types = append(opts.types, types...)
			opts.modern = true
		case 'v':
			opts.all, opts.modern = true, true
		case 'w':
			opts.modern = true
			if value == "" {
				opts.width = 32
				break
			}
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid width '%s'", value)
			}
			opts.width = n
		case optEndian:
			opts.modern = true
			switch value {
			case "big":
				opts.swap = true
			case "little":
				opts.swap = false
			default:
				return fmt.Errorf("invalid --endian value '%s'", value)
			}
		case optTraditional:
			opts.traditional = true
		case 'a':
			opts.types = append(opts.types, outputType{kind: 'a', size: 1})
		case 'b':
			opts.types = append(opts.types, outputType{kind: 'o', size: 1})
		case 'c':
			opts.types = append(opts.types, outputType{kind: 'c', size: 1})
		case 'd':
			opts.types = append(opts.types, outputType{kind: 'u', size: 2})
		case 'f':
			opts.types = append(opts.types, outputType{kind: 'f', size: 4})
		case 'i':
			opts.types = append(opts.types, outputType{kind: 'd', size: 4})
		case 'l':
			opts.types = append(opts.types, outputType{kind: 'd', size: 8})
		case 'o':
			opts.types = append(opts.types, outputType{kind: 'o', size: 2})
		case 's':
			opts.types = append(opts.types, outputType{kind: 'd', size: 2})
		case 'x':
			opts.types = append(opts.types, outputType{kind: 'x', size: 2})
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	// The strings mode takes no type, so it leaves the default out and the
	// check for one happens in Run.
	if len(opts.types) == 0 && opts.stringMin < 0 {
		opts.types = []outputType{{kind: 'o', size: 2}}
	}
	return opts, operands, nil
}

// typeSizeNames maps the C type letters which follow an output type to the
// number of bytes they stand for.
var (
	intSizeNames   = map[byte]int{'C': 1, 'S': 2, 'I': 4, 'L': 8}
	floatSizeNames = map[byte]int{'B': 2, 'H': 2, 'F': 4, 'D': 8, 'L': 16}
)

func isIntSize(size int) bool {
	return size == 1 || size == 2 || size == 4 || size == 8
}

func isFloatSize(size int) bool {
	return size == 2 || size == 4 || size == 8 || size == 16
}

func parseTypes(value string) ([]outputType, error) {
	var types []outputType
	for i := 0; i < len(value); {
		typ := outputType{kind: value[i]}
		i++
		var valid func(int) bool
		switch typ.kind {
		case 'a', 'c':
			typ.size = 1
		case 'd', 'o', 'u', 'x':
			size, next, err := typeSize(value, i, intSizeNames, 4)
			if err != nil {
				return nil, err
			}
			typ.size, i = size, next
			valid = isIntSize
		case 'f':
			if i < len(value) && value[i] == 'B' {
				typ.bfloat = true
			}
			size, next, err := typeSize(value, i, floatSizeNames, 8)
			if err != nil {
				return nil, err
			}
			typ.size, i = size, next
			valid = isFloatSize
		default:
			return nil, errors.New("invalid type string")
		}
		if valid != nil && !valid(typ.size) {
			return nil, errors.New("unsupported type size")
		}
		if i < len(value) && value[i] == 'z' {
			typ.z, i = true, i+1
		}
		types = append(types, typ)
	}
	if len(types) == 0 {
		return nil, errors.New("empty type string")
	}
	return types, nil
}

// typeSize reads the size of an output type at i in value: one of the C
// type letters of names, a number of bytes, or defaultSize when nothing
// follows the type.
func typeSize(value string, i int, names map[byte]int, defaultSize int) (int, int, error) {
	if i < len(value) {
		if size, ok := names[value[i]]; ok {
			return size, i + 1, nil
		}
	}
	start := i
	for i < len(value) && isDigit(value[i]) {
		i++
	}
	if i == start {
		return defaultSize, i, nil
	}
	size, err := strconv.Atoi(value[start:i])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid type size '%s'", value[start:i])
	}
	return size, i, nil
}

// parseBytes parses a count of input bytes: a number in decimal, in
// hexadecimal with a 0x or 0X prefix, or in octal with a leading 0,
// optionally followed by a multiplicative suffix, as "b" for 512, "K" for
// 1024, "KB" for 1000 and "KiB" for 1024 again, and so on up to "Y". A
// suffix without a number stands for one unit of it.
func parseBytes(s string) (uint64, error) {
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base, s = 16, s[2:]
	} else if len(s) > 1 && s[0] == '0' {
		base = 8
	}
	i := 0
	for i < len(s) && digitValue(s[i]) < base {
		i++
	}
	digits, suffix := s[:i], s[i:]
	n := uint64(1)
	if digits == "" {
		if base == 16 || suffix == "" {
			return 0, errors.New("invalid number")
		}
	} else {
		var err error
		if n, err = strconv.ParseUint(digits, base, 64); err != nil {
			return 0, errors.New("invalid number")
		}
	}
	mult, err := byteMultiplier(suffix)
	if err != nil {
		return 0, err
	}
	if n > math.MaxUint64/mult {
		return 0, errors.New("number too large")
	}
	return n * mult, nil
}

// byteMultiplier returns what a size suffix stands for: 512 for "b", a
// power of 1024 for "K", "M" and so on, of 1000 for "KB", "MB" and so on,
// and of 1024 again for "KiB" and so on. Like GNU od, it takes the letters
// of the binary prefixes in upper case only, except "k" and "m".
func byteMultiplier(suffix string) (uint64, error) {
	if suffix == "" {
		return 1, nil
	}
	if suffix == "b" {
		return 512, nil
	}
	power := 0
	switch suffix[0] {
	case 'E':
		power = 6
	case 'G':
		power = 3
	case 'K', 'k':
		power = 1
	case 'M', 'm':
		power = 2
	case 'P':
		power = 5
	case 'Q':
		power = 10
	case 'R':
		power = 9
	case 'T':
		power = 4
	case 'Y':
		power = 8
	case 'Z':
		power = 7
	default:
		return 0, fmt.Errorf("invalid suffix '%s'", suffix)
	}
	base := uint64(1024)
	switch suffix[1:] {
	case "":
	case "B", "D":
		base = 1000
	case "iB":
	default:
		return 0, fmt.Errorf("invalid suffix '%s'", suffix)
	}
	mult := uint64(1)
	for i := 0; i < power; i++ {
		if mult > math.MaxUint64/base {
			return 0, errors.New("suffix too large")
		}
		mult *= base
	}
	return mult, nil
}

// parseOldOffset parses the [+OFFSET[.][b]] operand of the traditional
// syntax: an octal number by default, a decimal one with a trailing dot,
// and a hexadecimal one with a 0x or 0X prefix, which a trailing b
// multiplies by 512. It reports whether the operand is an offset at all.
func parseOldOffset(s string) (uint64, bool) {
	if s != "" && s[0] == '+' {
		s = s[1:]
	}
	if s == "" || !isDigit(s[0]) {
		return 0, false
	}
	base := 8
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		if rest := s[dot+1:]; rest == "" || rest == "b" || rest == "B" {
			base, s = 10, s[:dot]+s[dot+1:]
		}
	}
	if base != 10 && len(s) > 1 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		base, s = 16, s[2:]
	}
	i := 0
	for i < len(s) && digitValue(s[i]) < base {
		i++
	}
	if i == 0 {
		return 0, false
	}
	if i == len(s) {
		n, err := strconv.ParseUint(s, base, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	if (s[i] != 'b' && s[i] != 'B') || i+1 != len(s) {
		return 0, false
	}
	n, err := strconv.ParseUint(s[:i], base, 64)
	if err != nil || n > math.MaxUint64/512 {
		return 0, false
	}
	return n * 512, true
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 99
}
