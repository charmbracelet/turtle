// Package cmp implements the POSIX and GNU cmp utilities.
//
// It compares two files byte by byte, writing the position of the first
// difference, or every differing byte with -l. An operand of "-" means the
// standard input, which is also compared to when FILE2 is missing. The
// SKIP1 and SKIP2 operands, like the -i option, skip the given number of
// bytes at the beginning of each file.
package cmp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/sizeparse"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the cmp core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run compares two files byte by byte, returning an exit status of 1 when
// they differ and 2 when any other error happens.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "cmp", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "cmp", err)
	}
	if opts.version {
		fmt.Fprint(p.Stdout, stdopts.VersionText("cmp"))
		return nil
	}
	names, err := parseOperands(operands, &opts)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "cmp", err)
	}

	var readers [2]io.Reader
	for i, name := range names {
		r, err := input.Open(p.Runtime, ctx, input.New(name))
		if err != nil {
			if !opts.silent {
				stdopts.Errorf(p.Runtime, "cmp", "%s: %v", name, patherr.Unwrap(err))
			}
			return &coreutils.ExitError{Code: 2}
		}
		readers[i] = r
		defer r.Close()
	}
	// Two files with the same name and the same skip are identical, as
	// the same data would be read from both.
	if names[0] == names[1] && opts.skip[0] == opts.skip[1] {
		return nil
	}

	status, err := p.compare(ctx, readers[0], readers[1], names, opts)
	if err != nil {
		var writeErr *writeError
		var readErr *input.ReadError
		switch {
		case errors.As(err, &writeErr):
			stdopts.Errorf(p.Runtime, "cmp", "write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 2, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &readErr):
			stdopts.Errorf(p.Runtime, "cmp", "%s: %v", readErr.Label, patherr.Unwrap(readErr.Err))
			return &coreutils.ExitError{Code: 2}
		default:
			return err
		}
	}
	if status > 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

// parseOperands maps the operands to the names of both files and their skips.
// The standard input is read for a missing FILE2, and the SKIP1 and SKIP2
// operands stand for the -i option, one file each.
func parseOperands(operands []string, opts *options) ([2]string, error) {
	if len(operands) == 0 {
		return [2]string{}, errors.New("missing operand")
	}
	names := [2]string{operands[0], "-"}
	if len(operands) > 1 {
		names[1] = operands[1]
	}
	// Comparing the standard input with itself is undefined: the stream
	// can't be read twice, as the first read consumes what the second
	// would compare.
	if names[0] == "-" && names[1] == "-" {
		return names, errors.New("the standard input may only be specified once")
	}
	for i := 2; i < len(operands) && i < 4; i++ {
		skip, err := parseCount(operands[i])
		if err != nil {
			return names, fmt.Errorf("invalid --ignore-initial value '%s'", operands[i])
		}
		if skip > opts.skip[i-2] {
			opts.skip[i-2] = skip
		}
	}
	if len(operands) > 4 {
		return names, fmt.Errorf("extra operand '%s'", operands[4])
	}
	return names, nil
}

// bufSize is the size of the blocks read from each file.
const bufSize = 32 * 1024

// compare compares both files block by block, writing the differences to the
// standard output as the options demand. It returns 1 when the files differ,
// and 0 when they are identical.
func (p *Prog) compare(ctx context.Context, r1, r2 io.Reader, names [2]string, opts options) (int, error) {
	// The skipped initial prefixes are read and discarded, as the files
	// may not be seekable.
	for i, r := range [2]io.Reader{r1, r2} {
		if opts.skip[i] > 0 {
			if _, err := io.CopyN(io.Discard, r, opts.skip[i]); err != nil && !errors.Is(err, io.EOF) {
				return 0, &input.ReadError{Label: names[i], Err: err}
			}
		}
	}

	br1 := bufio.NewReader(r1)
	br2 := bufio.NewReader(r2)
	buf1 := make([]byte, bufSize)
	buf2 := make([]byte, bufSize)
	var (
		byteNumber  int64 = 1
		lineNumber  int64 = 1
		atLineStart       = true
		differing         = false
		// remaining counts the bytes still to be compared; it is negative when
		// the comparison is unlimited.
		remaining = opts.bytes
	)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		size := bufSize
		if remaining >= 0 && remaining < int64(bufSize) {
			size = int(remaining)
		}
		remaining -= int64(size)

		n1, err1 := io.ReadFull(br1, buf1[:size])
		if err := readErr(err1); err != nil {
			return 0, &input.ReadError{Label: names[0], Err: err}
		}
		n2, err2 := io.ReadFull(br2, buf2[:size])
		if err := readErr(err2); err != nil {
			return 0, &input.ReadError{Label: names[1], Err: err}
		}

		smaller := min(n1, n2)
		diff := firstDiff(buf1[:smaller], buf2[:smaller])
		if diff < smaller {
			if opts.silent {
				return 1, nil
			}
			if opts.list {
				differing = true
				for i := diff; i < smaller; i++ {
					if buf1[i] != buf2[i] {
						if err := p.writeDiff(byteNumber+int64(i), buf1[i], buf2[i], opts); err != nil {
							return 0, err
						}
					}
				}
			} else {
				lineNumber += countNewlines(buf1[:diff])
				if err := p.writeFirstDiff(names, byteNumber+int64(diff), lineNumber, buf1[diff], buf2[diff], opts); err != nil {
					return 0, err
				}
				return 1, nil
			}
		}
		if !opts.list {
			lineNumber += countNewlines(buf1[:smaller])
			if smaller > 0 {
				atLineStart = buf1[smaller-1] == '\n'
			}
		}
		byteNumber += int64(smaller)

		if n1 != n2 {
			if !opts.silent {
				shorter := names[0]
				if n2 < n1 {
					shorter = names[1]
				}
				p.writeEOF(shorter, byteNumber-1, lineNumber, atLineStart, opts)
			}
			return 1, nil
		}
		if n1 < size || remaining == 0 {
			if differing {
				return 1, nil
			}
			return 0, nil
		}
	}
}

// writeFirstDiff writes the message pointing at the first difference.
func (p *Prog) writeFirstDiff(names [2]string, byteNumber, lineNumber int64, b1, b2 byte, opts options) error {
	var err error
	if opts.printBytes {
		_, err = fmt.Fprintf(p.Stdout, "%s %s differ: byte %d, line %d is %3o %s %3o %s\n",
			names[0], names[1], byteNumber, lineNumber, b1, printable(b1), b2, printable(b2))
	} else {
		_, err = fmt.Fprintf(p.Stdout, "%s %s differ: byte %d, line %d\n",
			names[0], names[1], byteNumber, lineNumber)
	}
	if err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeDiff writes the message of a differing byte for -l.
func (p *Prog) writeDiff(byteNumber int64, b1, b2 byte, opts options) error {
	var err error
	if opts.printBytes {
		_, err = fmt.Fprintf(p.Stdout, "%d %3o %-4s %3o %s\n",
			byteNumber, b1, printable(b1), b2, printable(b2))
	} else {
		_, err = fmt.Fprintf(p.Stdout, "%d %o %o\n", byteNumber, b1, b2)
	}
	if err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeEOF writes the diagnostic of the file with the given name ending
// before the other, after compared bytes were already matched.
func (p *Prog) writeEOF(name string, compared, lineNumber int64, atLineStart bool, opts options) {
	switch {
	case compared == 0:
		stdopts.Errorf(p.Runtime, "cmp", "EOF on '%s' which is empty", name)
	case opts.list:
		stdopts.Errorf(p.Runtime, "cmp", "EOF on '%s' after byte %d", name, compared)
	case atLineStart:
		stdopts.Errorf(p.Runtime, "cmp", "EOF on '%s' after byte %d, line %d", name, compared, lineNumber-1)
	default:
		stdopts.Errorf(p.Runtime, "cmp", "EOF on '%s' after byte %d, in line %d", name, compared, lineNumber)
	}
}

// options holds the parsed command line options.
type options struct {
	list       bool
	silent     bool
	printBytes bool     // -b
	version    bool     // -v
	bytes      int64    // -n; negative when the comparison is unlimited
	skip       [2]int64 // -i and the SKIP operands
}

// longOptions maps the long options of cmp to the short ones they stand for.
var longOptions = map[string]rune{
	"print-bytes":    'b',
	"ignore-initial": 'i',
	"bytes":          'n',
	"verbose":        'l',
	"quiet":          's',
	"silent":         's',
}

// optSet describes the options of cmp.
var optSet = getopt.Set{Valued: "in", Long: longOptions}

// usage describes the options of cmp.
const usage = `Usage: cmp [OPTION]... FILE1 [FILE2 [SKIP1 [SKIP2]]]
Compare two files byte by byte.

The optional SKIP1 and SKIP2 specify the number of bytes to skip
at the beginning of each file (zero by default).

  -b, --print-bytes
         print differing bytes
  -i, --ignore-initial=SKIP
         skip first SKIP bytes of both inputs
  -i, --ignore-initial=SKIP1:SKIP2
         skip first SKIP1 bytes of FILE1 and first SKIP2 bytes of FILE2
  -l, --verbose
         output byte numbers and differing byte values
  -n, --bytes=LIMIT
         compare at most LIMIT bytes
  -s, --quiet, --silent
         suppress all normal output
      --help
         display this help and exit
  -v, --version
         output version information and exit

SKIP values may be followed by the following multiplicative suffixes:
kB 1000, K 1024, MB 1,000,000, M 1,048,576, and so on for G, T, P, E, Z, Y.
If a FILE is '-' or missing, read standard input.
Exit status is 0 if inputs are the same, 1 if different, 2 if trouble.
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{bytes: -1}
	var mode byte
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.printBytes = true
		case 'i':
			if err := parseSkips(value, &opts.skip); err != nil {
				return err
			}
		case 'l':
			if mode != 0 && mode != 'l' {
				return errors.New("options -l and -s are incompatible")
			}
			mode = 'l'
			opts.list = true
		case 's':
			if mode != 0 && mode != 's' {
				return errors.New("options -l and -s are incompatible")
			}
			mode = 's'
			opts.silent = true
		case 'v':
			opts.version = true
		case 'n':
			limit, err := parseLimit(value)
			if err != nil {
				return err
			}
			if opts.bytes < 0 || limit < opts.bytes {
				opts.bytes = limit
			}
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// parseSkips parses the value of -i into the skips of both files: a single
// count applies to both files, while two counts split by a colon apply to
// FILE1 and FILE2 each. Repeated options keep the largest count of a file.
func parseSkips(value string, skip *[2]int64) error {
	first, second, twoCounts := strings.Cut(value, ":")
	n, err := parseCount(first)
	if err != nil {
		return fmt.Errorf("invalid --ignore-initial value '%s'", value)
	}
	if n > skip[0] {
		skip[0] = n
	}
	if twoCounts {
		n, err := parseCount(second)
		if err != nil {
			return fmt.Errorf("invalid --ignore-initial value '%s'", second)
		}
		if n > skip[1] {
			skip[1] = n
		}
	} else if skip[1] < skip[0] {
		skip[1] = skip[0]
	}
	return nil
}

// parseLimit parses the value of -n: a byte count.
func parseLimit(value string) (int64, error) {
	limit, err := parseCount(value)
	if err != nil {
		return 0, fmt.Errorf("invalid --bytes value '%s'", value)
	}
	return limit, nil
}

// errInvalidCount marks a value which is not a byte count.
var errInvalidCount = errors.New("invalid byte count")

// countSuffixes holds the multiplicative suffixes a byte count may take, with
// the multiplier each stands for: "K" for 1024, "kB" for 1000 and "KiB" for
// 1024 again. GNU cmp accepts these letters in this exact case only.
var countSuffixes = func() map[string]int64 {
	suffixes := map[string]int64{"": 1}
	for _, letter := range []string{"k", "K", "M", "G", "T", "P", "E", "Z", "Y"} {
		for _, suffix := range []string{letter, letter + "B", letter + "iB"} {
			// The multiplier comes from the shared size suffixes.
			if mult, err := sizeparse.Parse("1" + suffix); err == nil {
				suffixes[suffix] = mult
			}
		}
	}
	return suffixes
}()

// parseCount parses a byte count as cmp takes it: a decimal, hexadecimal or
// octal number with an optional multiplicative suffix, as "10K" for 10240.
// Leading blanks and a '+' sign are allowed.
func parseCount(value string) (int64, error) {
	value = strings.TrimLeft(value, " \t\n\v\f\r")
	if strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	n, suffix, err := splitCount(value)
	if err != nil {
		return 0, err
	}
	if _, ok := countSuffixes[suffix]; !ok {
		return 0, errInvalidCount
	}
	// sizeparse does the multiplication of the suffix, with its overflow
	// checks. The digits were normalized to decimal by splitCount.
	return sizeparse.Parse(strconv.FormatInt(n, 10) + suffix)
}

// splitCount splits a value into its number and suffix, returning an error
// when the value doesn't start with one. The number may be decimal,
// hexadecimal with a "0x" prefix, or octal with a leading zero.
func splitCount(value string) (n int64, suffix string, err error) {
	if len(value) >= 2 && value[0] == '0' && (value[1] == 'x' || value[1] == 'X') {
		end := 2
		for end < len(value) && isHexDigit(value[end]) {
			end++
		}
		if end > 2 {
			n, err := strconv.ParseInt(value[2:end], 16, 64)
			if err != nil {
				return 0, "", errInvalidCount
			}
			return n, value[end:], nil
		}
		// Without digits, only the leading zero counts as a number.
	}
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, "", errInvalidCount
	}
	base := 10
	if value[0] == '0' {
		// A leading zero makes the number octal, as far as its digits
		// go: "09" counts the "0" only, leaving "9" for the suffix.
		base = 8
		octal := 1
		for octal < end && value[octal] <= '7' {
			octal++
		}
		end = octal
	}
	n, err = strconv.ParseInt(value[:end], base, 64)
	if err != nil {
		return 0, "", errInvalidCount
	}
	return n, value[end:], nil
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// printable returns the byte as GNU cmp writes it, quoting the unprintable
// ones as cat -t does: control bytes as "^X", the meta ones as "M-X" and the
// delete byte as "^?".
func printable(b byte) string {
	var quoted string
	if b >= 0x80 {
		quoted = "M-"
		b -= 0x80
	}
	switch {
	case b < 0x20:
		return quoted + "^" + string(b+'@')
	case b == 0x7f:
		return quoted + "^?"
	}
	return quoted + string(b)
}

// readErr returns the error of a read result, or nil when it only signals
// the end of the input.
func readErr(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil
	}
	return err
}

// firstDiff returns the offset of the first differing byte of b1 and b2, or
// their length when they are equal.
func firstDiff(b1, b2 []byte) int {
	if bytes.Equal(b1, b2) {
		return len(b1)
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			return i
		}
	}
	return len(b1)
}

// countNewlines returns the number of newlines in b.
func countNewlines(b []byte) int64 {
	return int64(bytes.Count(b, []byte{'\n'}))
}

// writeError marks an error as coming from writing to the output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}

var _ coreutils.Program = (*Prog)(nil)
