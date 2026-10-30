// Package digest implements the behavior shared by the checksum utilities
// md5sum, sha1sum and the sha*sum family. The utilities differ only in the
// algorithm they use: the options they take, the lines they write and the
// checksum files they verify are all the same. The algorithm itself is given
// by each utility, so that only its hash lands in a build. The shasum
// utility instead selects its algorithm with an -a option, as its Family
// does.
package digest

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Algorithm is a checksum algorithm, as implemented by a {prog}sum utility.
type Algorithm struct {
	// Name is the program name, used on diagnostic messages.
	Name string

	// Tag is the algorithm name of a BSD-style line, like the "MD5" of
	// "MD5 (file) = d41d8cd98f00b204e9800998ecf8427e".
	Tag string

	// New returns a new hash of the algorithm.
	New func() hash.Hash
}

// chunkSize is how many bytes of an input are hashed at a time, which is also
// how often the context is checked while reading.
const chunkSize = 32 * 1024

// Run writes the digest of each operand to the standard output, or verifies
// the checksum lines read from them when -c is given. The standard input is
// used when no operand is given, and also when an operand is "-".
//
// It returns a [*coreutils.ExitError] when an operand failed, and the context
// error when ctx is done.
func (a Algorithm) Run(ctx context.Context, rt *coreutils.Runtime, args []string) error {
	if done, err := stdopts.Check(rt, a.Name, Usage(a.Name), optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		a.errorf(rt, "%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if err := opts.validate(); err != nil {
		a.errorf(rt, "%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	if opts.check {
		return a.verify(ctx, rt, opts, operands)
	}
	return a.digests(ctx, rt, opts, operands)
}

// digests writes a line with the digest of each operand, holding on failure so
// that every operand is still attempted.
func (a Algorithm) digests(ctx context.Context, rt *coreutils.Runtime, opts options, operands []string) error {
	var failed bool
	for _, name := range operands {
		r, err := input.Open(rt, ctx, input.New(name))
		if err != nil {
			a.report(rt, name, err)
			failed = true
			continue
		}
		sum, err := a.digestStream(ctx, r)
		r.Close()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			a.report(rt, name, err)
			failed = true
			continue
		}
		if _, err := io.WriteString(rt.Stdout, a.line(opts, name, sum)); err != nil {
			a.errorf(rt, "write error: %v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// digestStream returns the digest of r, holding it as long as ctx isn't done.
func (a Algorithm) digestStream(ctx context.Context, r io.Reader) ([]byte, error) {
	h := a.New()
	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		h.Write(buf[:n])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return h.Sum(nil), nil
			}
			return nil, err
		}
	}
}

// line returns the line the utility writes for the digest of a file name.
func (a Algorithm) line(opts options, name string, sum []byte) string {
	delim := "\n"
	if opts.zero {
		delim = "\x00"
	}

	// A file name holding a backslash or a line break is escaped, which needs
	// the line to end with a newline: --zero writes the name as it is, like
	// GNU does.
	escaped := !opts.zero && problematicChars(name)
	if escaped {
		name = nameEscaper.Replace(name)
	}

	var b strings.Builder
	b.Grow(len(name) + len(sum)*2 + 8)
	if escaped {
		// The leading backslash tells the whole line is escaped.
		b.WriteByte('\\')
	}
	if opts.tag {
		b.WriteString(a.Tag)
		b.WriteString(" (")
		b.WriteString(name)
		b.WriteString(") = ")
		b.WriteString(hex.EncodeToString(sum))
		b.WriteString(delim)
		return b.String()
	}

	// The character between the digest and the name tells the input mode. It
	// only ever changes the output: reading is always done as bytes are.
	mode := ' '
	if opts.binary {
		mode = '*'
	}
	b.WriteString(hex.EncodeToString(sum))
	b.WriteRune(' ')
	b.WriteRune(mode)
	b.WriteString(name)
	b.WriteString(delim)
	return b.String()
}

// report writes the diagnostic of an operand that couldn't be digested.
func (a Algorithm) report(rt *coreutils.Runtime, name string, err error) {
	a.errorf(rt, "%s: %v", name, patherr.Unwrap(err))
}

// errorf writes a diagnostic message to the standard error of rt.
func (a Algorithm) errorf(rt *coreutils.Runtime, format string, args ...any) {
	fmt.Fprintf(rt.Stderr, a.Name+": "+format+"\n", args...)
}

// nameEscaper translates the characters which are escaped on output.
var nameEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`)

// problematicChars reports whether a file name holds characters which have to
// be escaped for the lines it appears in to be parsed back.
func problematicChars(s string) bool {
	return strings.ContainsAny(s, "\\\n\r")
}

// options holds what the options of the utility select.
type options struct {
	check         bool
	binary        bool
	tag           bool
	zero          bool
	ignoreMissing bool
	quiet         bool
	status        bool
	strict        bool
	warn          bool

	// modeSet tells whether -b or -t was given, which GNU takes as a conflict
	// with both -c and --tag.
	modeSet bool
}

// longOptions maps the long options of the utility to the short ones they
// stand for. The ones without a short equivalent map to runes above the bytes
// a group of options can hold, keeping them out of the short syntax.
var longOptions = map[string]rune{
	"binary":         'b',
	"check":          'c',
	"ignore-missing": optIgnoreMissing,
	"quiet":          optQuiet,
	"status":         optStatus,
	"strict":         optStrict,
	"tag":            optTag,
	"text":           't',
	"warn":           'w',
	"zero":           'z',
}

// The long options which have no short equivalent.
const (
	optIgnoreMissing = 0x100 + iota
	optQuiet
	optStatus
	optStrict
	optTag
)

// optSet describes the options of the {prog}sum utilities.
var optSet = getopt.Set{Long: longOptions}

// Usage returns the usage text of the {prog}sum utility named prog.
func Usage(prog string) string {
	return `Usage: ` + prog + ` [OPTION]... [FILE]...
Print or check checksums.

With no FILE, or when FILE is -, read standard input.

  -b, --binary           read in binary mode
  -c, --check            read checksums from the FILES and check them
      --ignore-missing   don't fail or report status for missing files
      --quiet            don't print OK for each successfully verified file
      --status           don't output anything, status code shows success
      --strict           exit non-zero for improperly formatted checksum lines
      --tag              create a BSD-style checksum
  -t, --text             read in text mode (default)
  -w, --warn             warn about improperly formatted checksum lines
  -z, --zero             end each output line with NUL, not newline
      --help             display this help and exit
      --version          output version information and exit
`
}

// validate reports the options which GNU refuses to take together.
func (o options) validate() error {
	switch {
	case o.zero && o.check:
		return errors.New("the --zero option is not supported when verifying checksums")
	case o.tag && o.check:
		return errors.New("the --tag option is meaningless when verifying checksums")
	case o.modeSet && o.check:
		return errors.New("the --binary and --text options are meaningless when verifying checksums")
	case o.ignoreMissing && !o.check:
		return errors.New("the --ignore-missing option is meaningful only when verifying checksums")
	case o.status && !o.check:
		return errors.New("the --status option is meaningful only when verifying checksums")
	case o.warn && !o.check:
		return errors.New("the --warn option is meaningful only when verifying checksums")
	case o.quiet && !o.check:
		return errors.New("the --quiet option is meaningful only when verifying checksums")
	case o.strict && !o.check:
		return errors.New("the --strict option is meaningful only when verifying checksums")
	case o.tag && o.modeSet && !o.binary:
		return errors.New("--tag does not support --text mode")
	}
	return nil
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.binary, opts.modeSet = true, true
		case 'c':
			opts.check = true
		case 't':
			opts.binary, opts.modeSet = false, true
		case 'w':
			opts.status, opts.quiet = false, false
			opts.warn = true
		case 'z':
			opts.zero = true
		case optIgnoreMissing:
			opts.ignoreMissing = true
		case optQuiet:
			opts.status, opts.warn = false, false
			opts.quiet = true
		case optStatus:
			opts.warn, opts.quiet = false, false
			opts.status = true
		case optStrict:
			opts.strict = true
		case optTag:
			opts.tag = true
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
