package digest

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/input"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// The format of the checksum lines seen until now. A line without the
// character which tells the input mode comes from a different convention, and
// GNU refuses to mix the two so that renaming a file with a leading blank
// can't change which file gets verified.
const (
	formatUnknown = iota - 1
	formatStandard
	formatReversed
)

// errCheckRead reports a checksum file that couldn't be read to its end.
var errCheckRead = errors.New("read error")

// checker verifies the checksum lines of a file, holding the counters which
// are reported once it was all read.
type checker struct {
	a    Algorithm
	rt   *coreutils.Runtime
	opts options

	// hexLen is how many hexadecimal characters a digest of the algorithm has.
	hexLen int

	// reversed is shared by every checksum file of a run, like GNU keeps it.
	reversed int

	// What happened while reading the checksum file.
	misformatted int
	readFailures int
	mismatches   int
	valid        bool
	matched      bool
}

// verify reads the checksum lines of each operand and compares them with the
// digest of the files they name.
func (a Algorithm) verify(ctx context.Context, rt *coreutils.Runtime, opts options, operands []string) error {
	c := &checker{
		a:        a,
		rt:       rt,
		opts:     opts,
		hexLen:   2 * a.New().Size(),
		reversed: formatUnknown,
	}

	var failed bool
	for _, name := range operands {
		ok, err := c.file(ctx, name)
		if err != nil {
			return err
		}
		failed = failed || !ok
	}
	if failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// file verifies the checksum lines of a single operand, which is either a
// pathname or "-" for the standard input, and returns whether they all held.
func (c *checker) file(ctx context.Context, name string) (bool, error) {
	c.misformatted, c.readFailures, c.mismatches = 0, 0, 0
	c.valid, c.matched = false, false

	label, stdin := name, name == "-"
	if stdin {
		label = "standard input"
	}

	r, err := input.Open(c.rt, ctx, input.New(name))
	if err != nil {
		stdopts.Errorf(c.rt, c.a.Name, "%s: %v", label, patherr.Unwrap(err))
		return false, nil
	}
	defer r.Close()

	if err := c.lines(ctx, r, label, stdin); err != nil {
		if errors.Is(err, errCheckRead) {
			return false, nil
		}
		return false, err
	}
	return c.report(label), nil
}

// lines reads the checksum lines of r and verifies each one of them.
func (c *checker) lines(ctx context.Context, r io.Reader, label string, stdin bool) error {
	br := bufio.NewReader(r)
	for number := 1; ; number++ {
		line, err := br.ReadString('\n')
		atEOF := errors.Is(err, io.EOF)
		if err != nil && !atEOF {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			stdopts.Errorf(c.rt, c.a.Name, "%s: read error: %v", label, err)
			return errCheckRead
		}
		if line != "" {
			if err := c.verifyLine(ctx, label, number, line, stdin); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if atEOF {
			return nil
		}
	}
}

// verifyLine verifies a single line of a checksum file. Blank lines and
// comments are ignored, as GNU does.
func (c *checker) verifyLine(ctx context.Context, label string, number int, line string, stdin bool) error {
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" || line[0] == '#' {
		return nil
	}

	e, ok := c.parseEntry(line)
	if !ok || (stdin && e.name == "-") {
		c.misformatted++
		if c.opts.warn {
			stdopts.Errorf(c.rt, c.a.Name, "%s: %d: improperly formatted %s checksum line",
				label, number, c.a.Tag)
		}
		return nil
	}
	c.valid = true

	sum, err := c.digestOf(ctx, e.name)
	switch {
	case err != nil && ctx.Err() != nil:
		return ctx.Err()
	case err != nil && errors.Is(err, fs.ErrNotExist) && c.opts.ignoreMissing:
		// A file which isn't there is skipped without any mention.
	case err != nil:
		stdopts.Errorf(c.rt, c.a.Name, "%s: %v", e.name, patherr.Unwrap(err))
		c.readFailures++
		if !c.opts.status {
			c.statusf("%s: FAILED open or read", e.name)
		}
	case strings.EqualFold(e.sum, hex.EncodeToString(sum)):
		c.matched = true
		if !c.opts.status && !c.opts.quiet {
			c.statusf("%s: OK", e.name)
		}
	default:
		c.mismatches++
		if !c.opts.status {
			c.statusf("%s: FAILED", e.name)
		}
	}
	return nil
}

// digestOf returns the digest of a file named by a checksum line.
func (c *checker) digestOf(ctx context.Context, name string) ([]byte, error) {
	r, err := input.Open(c.rt, ctx, input.New(name))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return c.a.digestStream(ctx, r)
}

// report writes the summary of a checksum file and returns whether every
// checksum it held was verified.
func (c *checker) report(label string) bool {
	if !c.valid {
		stdopts.Errorf(c.rt, c.a.Name, "%s: no properly formatted checksum lines found", label)
		return false
	}
	if !c.opts.status {
		if c.misformatted > 0 {
			c.warnf(c.misformatted, "line is improperly formatted",
				"lines are improperly formatted")
		}
		if c.readFailures > 0 {
			c.warnf(c.readFailures, "listed file could not be read",
				"listed files could not be read")
		}
		if c.mismatches > 0 {
			c.warnf(c.mismatches, "computed checksum did NOT match",
				"computed checksums did NOT match")
		}
		if c.opts.ignoreMissing && !c.matched {
			stdopts.Errorf(c.rt, c.a.Name, "%s: no file was verified", label)
		}
	}
	return c.matched && c.mismatches == 0 && c.readFailures == 0 &&
		(!c.opts.strict || c.misformatted == 0)
}

// statusf writes the status of a verified file to the standard output.
func (c *checker) statusf(format string, args ...any) {
	fmt.Fprintf(c.rt.Stdout, format+"\n", args...)
}

// warnf writes the diagnostic about a number of lines or files, in the singular
// or the plural form GNU uses.
func (c *checker) warnf(n int, singular, plural string) {
	warning := "WARNING: " + strconv.Itoa(n) + " " + plural
	if n == 1 {
		warning = "WARNING: 1 " + singular
	}
	stdopts.Errorf(c.rt, c.a.Name, "%s", warning)
}

// entry is a checksum line: the digest to compare and the file it was taken
// from.
type entry struct {
	name string
	sum  string
}

// parseEntry splits a checksum line into its digest and file name. It takes both
// the format the utility writes, which tells the input mode after the digest,
// and the BSD-style format it writes with --tag.
func (c *checker) parseEntry(s string) (entry, bool) {
	s = strings.TrimLeft(s, " \t")

	// A line whose file name was escaped begins with a backslash.
	escaped := false
	if rest, ok := strings.CutPrefix(s, `\`); ok {
		s, escaped = rest, true
	}

	if rest, ok := strings.CutPrefix(s, c.a.Tag); ok {
		if name, ok := strings.CutPrefix(strings.TrimPrefix(rest, " "), "("); ok {
			return c.parseBSDEntry(name, escaped)
		}
	}

	if len(s) < c.hexLen+2 {
		return entry{}, false
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return entry{}, false
	}
	e := entry{sum: s[:i]}
	if !validHex(e.sum, c.hexLen) {
		return entry{}, false
	}

	// What follows the blank which ends the digest starts with the mode
	// character, unless the line comes from the other convention.
	rest := s[i+1:]
	switch {
	case len(rest) == 1 || (rest[0] != ' ' && rest[0] != '*'):
		if c.reversed == formatStandard {
			return entry{}, false
		}
		c.reversed = formatReversed
		e.name = rest
	default:
		if c.reversed != formatReversed {
			c.reversed = formatStandard
			rest = rest[1:]
		}
		e.name = rest
	}

	if escaped {
		name, ok := unescapeName(e.name)
		if !ok {
			return entry{}, false
		}
		e.name = name
	}
	return e, true
}

// parseBSDEntry splits what follows the "(" of a BSD-style line, which holds the
// file name up to its last ")", and the digest which follows the "=".
func (c *checker) parseBSDEntry(s string, escaped bool) (entry, bool) {
	end := strings.LastIndexByte(s, ')')
	if end < 0 {
		return entry{}, false
	}

	rest, ok := strings.CutPrefix(strings.TrimLeft(s[end+1:], " \t"), "=")
	if !ok {
		return entry{}, false
	}
	e := entry{name: s[:end], sum: strings.TrimLeft(rest, " \t")}
	if !validHex(e.sum, c.hexLen) {
		return entry{}, false
	}

	if escaped {
		name, ok := unescapeName(e.name)
		if !ok {
			return entry{}, false
		}
		e.name = name
	}
	return e, true
}

// unescapeName translates the escape sequences of a file name written by the
// utility. A name where a backslash is followed by anything other than the
// characters it escapes, or which ends with one, is invalid.
func unescapeName(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch {
		case i == len(s):
			return "", false
		case s[i] == 'n':
			b.WriteByte('\n')
		case s[i] == 'r':
			b.WriteByte('\r')
		case s[i] == '\\':
			b.WriteByte('\\')
		default:
			return "", false
		}
	}
	return b.String(), true
}

// validHex reports whether s is a digest of the algorithm, holding exactly
// length hexadecimal characters.
func validHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for i := range length {
		switch c := s[i]; {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
