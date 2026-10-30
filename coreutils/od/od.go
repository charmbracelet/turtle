// Package od implements the POSIX od utility.
package od

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the od core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the input data in the requested representation.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "od", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "od")
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	var data []byte
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := input.Open(p.Runtime, ctx, input.New(name))
		if err != nil {
			p.errorf("%s: %v", name, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		part, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.errorf("%s: %v", name, patherr.Unwrap(readErr))
			return &coreutils.ExitError{Code: 1}
		}
		if closeErr != nil {
			p.errorf("%s: %v", name, patherr.Unwrap(closeErr))
			return &coreutils.ExitError{Code: 1}
		}
		data = append(data, part...)
	}
	if opts.skip > uint64(len(data)) {
		p.errorf("cannot skip past end of input")
		return &coreutils.ExitError{Code: 1}
	}
	start := int(opts.skip)
	data = data[start:]
	if opts.count >= 0 && int64(len(data)) > opts.count {
		data = data[:opts.count]
	}
	if err := p.dump(ctx, data, uint64(start), opts); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("write error: %v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

func (p *Prog) dump(ctx context.Context, data []byte, base uint64, opts options) error {
	if len(data) == 0 {
		if opts.address != 'n' {
			_, err := fmt.Fprintln(p.Stdout, formatAddress(base, opts.address))
			return err
		}
		return nil
	}
	previous := ""
	starred := false
	for offset := 0; offset < len(data); offset += 16 {
		if err := ctx.Err(); err != nil {
			return err
		}
		block := data[offset:min(offset+16, len(data))]
		var rendered []string
		for _, typ := range opts.types {
			row := renderType(block, typ)
			if row != "" {
				rendered = append(rendered, row)
			}
		}
		key := strings.Join(rendered, "\n")
		if !opts.all && key == previous {
			if !starred {
				if _, err := fmt.Fprintln(p.Stdout, "*"); err != nil {
					return err
				}
				starred = true
			}
		} else {
			for i, row := range rendered {
				if i == 0 && opts.address != 'n' {
					if _, err := fmt.Fprintf(p.Stdout, "%s ", formatAddress(base+uint64(offset), opts.address)); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintln(p.Stdout, row); err != nil {
					return err
				}
			}
			previous = key
			starred = false
		}
	}
	if opts.address != 'n' {
		_, err := fmt.Fprintln(p.Stdout, formatAddress(base+uint64(len(data)), opts.address))
		return err
	}
	return nil
}

type outputType struct {
	kind byte
	size int
}

func renderType(data []byte, typ outputType) string {
	var fields []string
	for i := 0; i < len(data); i += typ.size {
		end := min(i+typ.size, len(data))
		unit := make([]byte, typ.size)
		copy(unit, data[i:end])
		fields = append(fields, renderUnit(unit, typ.kind))
	}
	return strings.Join(fields, " ")
}

func renderUnit(b []byte, kind byte) string {
	value := binary.LittleEndian.Uint64(pad8(b))
	switch kind {
	case 'o':
		return fmt.Sprintf("%0*o", len(b)*3, value)
	case 'x':
		return fmt.Sprintf("%0*x", len(b)*2, value)
	case 'u':
		return strconv.FormatUint(value, 10)
	case 'd':
		bits := uint(len(b) * 8)
		if bits < 64 && value&(uint64(1)<<(bits-1)) != 0 {
			value |= ^uint64(0) << bits
		}
		return strconv.FormatInt(int64(value), 10)
	case 'f':
		if len(b) == 4 {
			return strconv.FormatFloat(float64(math.Float32frombits(uint32(value))), 'g', 7, 32)
		}
		return strconv.FormatFloat(math.Float64frombits(value), 'g', 16, 64)
	case 'c':
		return renderCharacter(b[0])
	case 'a':
		return namedCharacter(b[0])
	default:
		return ""
	}
}

func pad8(b []byte) []byte {
	var padded [8]byte
	copy(padded[:], b)
	return padded[:]
}

func renderCharacter(b byte) string {
	switch b {
	case 0:
		return "\\0"
	case '\\':
		return "\\\\"
	case '\a':
		return "\\a"
	case '\b':
		return "\\b"
	case '\f':
		return "\\f"
	case '\n':
		return "\\n"
	case '\r':
		return "\\r"
	case '\t':
		return "\\t"
	case '\v':
		return "\\v"
	default:
		if b >= 0x20 && b <= 0x7e {
			return string(b)
		}
		return fmt.Sprintf("%03o", b)
	}
}

func namedCharacter(b byte) string {
	names := [...]string{"nul", "soh", "stx", "etx", "eot", "enq", "ack", "bel", "bs", "ht", "lf", "vt", "ff", "cr", "so", "si", "dle", "dc1", "dc2", "dc3", "dc4", "nak", "syn", "etb", "can", "em", "sub", "esc", "fs", "gs", "rs", "us"}
	if b < 32 {
		return names[b]
	}
	if b == 32 {
		return "sp"
	}
	if b == 127 {
		return "del"
	}
	return string(b)
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "od: "+format+"\n", args...)
}

type options struct {
	address byte
	skip    uint64
	count   int64
	all     bool
	types   []outputType
}

// optSet describes the options of od.
var optSet = getopt.Set{Valued: "AjNt"}

// usage describes the options of od.
const usage = `Usage: od [OPTION]... [FILE]...
Write an unambiguous representation of each FILE, or standard input, to
standard output. Each line of output consists of the offset in the input,
followed by groups of data from the file.

  -A RADIX   select the base in which file offsets are printed: d for
             decimal, o for octal, x for hexadecimal or n for none
  -j BYTES   skip BYTES input bytes before formatting and writing
  -N BYTES   output at most BYTES input bytes
  -t TYPE    select the format of the output data; TYPE may combine a, c, d,
             f, o, u and x, each optionally followed by a size
  -v         output consecutive lines that are identical, instead of
             replacing them with '*'
  -b         same as -t o1
  -c         same as -t c
  -d         same as -t u2
  -o         same as -t o2
  -s         same as -t d2
  -x         same as -t x2
      --help
         display this help and exit
      --version
         output version information and exit
`

func parseFlags(args []string) (options, []string, error) {
	opts := options{address: 'o', count: -1}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'A':
			if len(value) != 1 || !strings.ContainsRune("doxn", rune(value[0])) {
				return errors.New("invalid address base")
			}
			opts.address = value[0]
		case 'j':
			n, err := parseNumber(value, true)
			if err != nil {
				return errors.New("invalid skip count")
			}
			opts.skip = n
		case 'N':
			n, err := parseNumber(value, false)
			if err != nil || n > uint64(^uint64(0)>>1) {
				return errors.New("invalid byte count")
			}
			opts.count = int64(n)
		case 't':
			types, err := parseTypes(value)
			if err != nil {
				return err
			}
			opts.types = append(opts.types, types...)
		case 'v':
			opts.all = true
		case 'b':
			opts.types = append(opts.types, outputType{'o', 1})
		case 'c':
			opts.types = append(opts.types, outputType{'c', 1})
		case 'd':
			opts.types = append(opts.types, outputType{'u', 2})
		case 'o':
			opts.types = append(opts.types, outputType{'o', 2})
		case 's':
			opts.types = append(opts.types, outputType{'d', 2})
		case 'x':
			opts.types = append(opts.types, outputType{'x', 2})
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if len(opts.types) == 0 {
		opts.types = []outputType{{'o', 2}}
	}
	return opts, operands, nil
}

func parseTypes(value string) ([]outputType, error) {
	var types []outputType
	for i := 0; i < len(value); {
		kind := value[i]
		if !strings.ContainsRune("acdfoux", rune(kind)) {
			return nil, errors.New("invalid type string")
		}
		i++
		if kind == 'a' || kind == 'c' {
			types = append(types, outputType{kind, 1})
			continue
		}
		start := i
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
		}
		size := 4
		if i > start {
			n, err := strconv.Atoi(value[start:i])
			if err != nil {
				return nil, errors.New("invalid type size")
			}
			size = n
		} else if i < len(value) {
			switch value[i] {
			case 'C', 'F':
				size = 1
			case 'S':
				size = 2
			case 'I':
				size = 4
			case 'L', 'D':
				size = 8
			default:
				if kind == 'f' {
					size = 8
				} else {
					return nil, errors.New("invalid type size")
				}
			}
			i++
		} else if kind == 'f' {
			size = 8
		}
		if size != 1 && size != 2 && size != 4 && size != 8 {
			return nil, errors.New("unsupported type size")
		}
		types = append(types, outputType{kind, size})
	}
	if len(types) == 0 {
		return nil, errors.New("empty type string")
	}
	return types, nil
}

func parseNumber(value string, suffix bool) (uint64, error) {
	multiplier := uint64(1)
	if suffix && len(value) > 0 {
		switch value[len(value)-1] {
		case 'b':
			multiplier = 512
			value = value[:len(value)-1]
		case 'k':
			multiplier = 1024
			value = value[:len(value)-1]
		case 'm':
			multiplier = 1024 * 1024
			value = value[:len(value)-1]
		}
	}
	base := 10
	number := value
	if strings.HasPrefix(number, "0x") || strings.HasPrefix(number, "0X") {
		base, number = 16, number[2:]
	} else if len(number) > 1 && number[0] == '0' {
		base, number = 8, number[1:]
	}
	n, err := strconv.ParseUint(number, base, 64)
	if err != nil || n > ^uint64(0)/multiplier {
		return 0, errors.New("invalid number")
	}
	return n * multiplier, nil
}

func formatAddress(offset uint64, base byte) string {
	switch base {
	case 'd':
		return fmt.Sprintf("%07d", offset)
	case 'x':
		return fmt.Sprintf("%07x", offset)
	default:
		return fmt.Sprintf("%07o", offset)
	}
}

var _ coreutils.Program = (*Prog)(nil)
