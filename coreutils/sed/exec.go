package sed

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/fsx"
)

// lWidth is the column at which the l command folds long lines.
const lWidth = 70

// ctrl tells how a cycle ended.
type ctrl int

const (
	ctrlEnd  ctrl = iota // the end of the script was reached
	ctrlNext             // the next cycle starts, reading a new line
	ctrlSkip             // the next cycle starts without reading a line
	ctrlQuit             // the script quits, writing the pattern space
	ctrlDone             // the script quits without writing the pattern space
)

// pendingOut is the text an a or r command scheduled for output.
type pendingOut struct {
	text string
	file string
}

// source reads lines from the input files, or the standard input when no
// files are given. A one-line lookahead tells whether the current line is
// the last one, for the `$` address.
type source struct {
	p     *Prog
	ctx   context.Context
	paths []string
	idx   int
	r     *bufio.Reader
	file  io.Closer

	buf   string
	has   bool
	bufNL bool
	// err holds an error hit by the lookahead fetch, so that the line read
	// before it isn't lost: it is returned on the following fetches until
	// the input ends.
	err error
}

func (p *Prog) newSource(ctx context.Context, files []string) *source {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f
	}
	if len(paths) == 0 {
		paths = []string{"-"}
	}
	return &source{p: p, ctx: ctx, paths: paths}
}

// next returns the next line of input, without its terminating newline, and
// whether it is the last one. An error hit by the lookahead fetch of the
// previous call keeps the line read before it: the line is returned, and the
// error follows it on the next fetch.
func (s *source) next() (line string, last, hasNL bool, err error) {
	line, hasNL, err = s.fetch()
	if err != nil {
		return "", false, false, err
	}
	peek, peekNL, perr := s.fetch()
	switch perr {
	case nil:
		s.buf, s.has, s.bufNL = peek, true, peekNL
		return line, false, hasNL, nil
	case io.EOF:
		return line, true, hasNL, nil
	default:
		s.err = perr // the line was read fine, so the error can wait
		return line, false, hasNL, nil
	}
}

// fetch returns the next line of input and whether it had a terminating
// newline, taking the lookahead line first and moving to the next input file
// when the current one ends.
func (s *source) fetch() (string, bool, error) {
	if s.err != nil {
		return "", false, s.err // the stashed lookahead error is due
	}
	if s.has {
		s.has = false
		line, hasNL := s.buf, s.bufNL
		s.buf, s.bufNL = "", false
		return line, hasNL, nil
	}
	for {
		if s.r == nil {
			if s.idx >= len(s.paths) {
				return "", false, io.EOF
			}
			rc, err := s.open(s.paths[s.idx])
			s.idx++
			if err != nil {
				return "", false, err
			}
			s.r = bufio.NewReader(rc)
			s.file = rc
		}
		line, err := s.r.ReadString('\n')
		switch err {
		case nil:
			return strings.TrimSuffix(line, "\n"), true, nil
		case io.EOF:
			if line := strings.TrimSuffix(line, "\n"); line != "" {
				return line, false, nil // the file ends without a newline
			}
			s.close()
		default:
			return "", false, fmt.Errorf("read error: %v", err)
		}
	}
}

// open returns a reader for an input file, with the standard input used when
// the name is "-".
func (s *source) open(name string) (io.ReadCloser, error) {
	if name == "-" {
		if s.p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return io.NopCloser(s.p.Stdin), nil
	}
	if s.p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := s.p.FS.Open(s.p.path(name))
	if err != nil {
		return nil, fmt.Errorf("can't read %s: %v", name, patherr.Unwrap(err))
	}
	return f, nil
}

// close closes the current input file, when it's owned by the source.
func (s *source) close() {
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	s.r = nil
}

// run executes the script against the input.
func (p *Prog) run(ctx context.Context, cmds []*command, files []string, quiet bool) error {
	r := &runner{
		p:     p,
		ctx:   ctx,
		cmds:  cmds,
		quiet: quiet,
		src:   p.newSource(ctx, files),
	}
	defer r.src.close()
	// The writers are closed even when opening one of them fails, so the
	// ones already open don't leak.
	defer r.closeWriters()
	if err := r.openWriters(); err != nil {
		return err
	}

	skip := false // a D command ended the cycle without reading a line
	for {
		if !skip {
			if err := ctx.Err(); err != nil {
				return err
			}
			line, last, hasNL, err := r.readLine()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			r.pattern = line
			r.lineNum, r.atLast, r.hasNL, r.substituted = r.lineNum+1, last, hasNL, false
		}
		skip = false

		done, err := r.exec()
		if err != nil {
			return err
		}
		switch done {
		case ctrlEnd:
			// The a and r text is written after copying the pattern space
			// when reaching the end of the script.
			if !r.quiet {
				if err := r.write(r.lineOut(r.pattern)); err != nil {
					return err
				}
			}
			if err := r.flushPending(); err != nil {
				return err
			}
		case ctrlQuit:
			// The a and r text is written just before executing q, so before
			// the pattern space is written.
			if err := r.flushPending(); err != nil {
				return err
			}
			if !r.quiet {
				if err := r.write(r.lineOut(r.pattern)); err != nil {
					return err
				}
			}
			return nil
		case ctrlDone:
			if err := r.flushPending(); err != nil {
				return err
			}
			return nil
		case ctrlSkip:
			skip = true // the pattern space is kept, and no line is read
		}
	}
}

// runner is the state of a script running against the input.
type runner struct {
	p           *Prog
	ctx         context.Context
	cmds        []*command
	quiet       bool
	src         *source
	pattern     string
	hold        string
	lineNum     int
	atLast      bool
	hasNL       bool
	substituted bool
	pending     []pendingOut
	writers     map[string]fsx.File
}

// readLine fetches the next line of input, writing the text and files
// scheduled by the a and r commands before the attempt.
func (r *runner) readLine() (string, bool, bool, error) {
	if err := r.flushPending(); err != nil {
		return "", false, false, err
	}
	return r.src.next()
}

// lineOut appends the newline a line had in the input, so that one which
// ends the input without it keeps that.
func (r *runner) lineOut(line string) string {
	if r.hasNL {
		return line + "\n"
	}
	return line
}

// exec runs the script against the pattern space, until a command ends the
// cycle or the end of the script is reached.
func (r *runner) exec() (ctrl, error) {
	for i := 0; i < len(r.cmds); {
		if err := r.ctx.Err(); err != nil {
			return ctrlEnd, err
		}
		c := r.cmds[i]
		if !r.selected(c) {
			if c.verb == '{' {
				i = c.end // the block isn't selected, so it's skipped
			} else {
				i++
			}
			continue
		}
		next := i + 1
		switch c.verb {
		case '{':
			next = i + 1 // the body runs on the following commands
		case ':', '#':
		case '=':
			if err := r.write(fmt.Sprintf("%d\n", r.lineNum)); err != nil {
				return ctrlEnd, err
			}
		case 'a':
			r.pending = append(r.pending, pendingOut{text: c.text})
		case 'b':
			next = c.jump
		case 'c':
			// The text goes out with a one-address command, at the end
			// of a non-negated two-address range, and on every line a
			// negated two-address range selects.
			if c.a2 == nil || c.neg || c.rangeEnd {
				if err := r.write(c.text + "\n"); err != nil {
					return ctrlEnd, err
				}
			}
			r.pattern = ""
			return ctrlNext, nil
		case 'd':
			r.pattern = ""
			return ctrlNext, nil
		case 'D':
			if idx := strings.IndexByte(r.pattern, '\n'); idx >= 0 {
				r.pattern = r.pattern[idx+1:]
				return ctrlSkip, nil
			}
			r.pattern = ""
			return ctrlNext, nil
		case 'g':
			r.pattern = r.hold
		case 'G':
			r.pattern += "\n" + r.hold
		case 'h':
			r.hold = r.pattern
		case 'H':
			r.hold += "\n" + r.pattern
		case 'i':
			if err := r.write(c.text + "\n"); err != nil {
				return ctrlEnd, err
			}
		case 'l':
			if err := r.write(formatL(r.pattern)); err != nil {
				return ctrlEnd, err
			}
		case 'n':
			if !r.quiet {
				if err := r.write(r.lineOut(r.pattern)); err != nil {
					return ctrlEnd, err
				}
			}
			line, last, hasNL, err := r.readLine()
			switch {
			case errors.Is(err, io.EOF):
				return ctrlDone, nil
			case err != nil:
				return ctrlEnd, err
			}
			r.pattern = line
			r.lineNum, r.atLast, r.hasNL, r.substituted = r.lineNum+1, last, hasNL, false
		case 'N':
			line, last, hasNL, err := r.readLine()
			switch {
			case errors.Is(err, io.EOF):
				return ctrlDone, nil
			case err != nil:
				return ctrlEnd, err
			}
			r.pattern += "\n" + line
			r.lineNum, r.atLast, r.hasNL, r.substituted = r.lineNum+1, last, hasNL, false
		case 'p':
			if err := r.write(r.lineOut(r.pattern)); err != nil {
				return ctrlEnd, err
			}
		case 'P':
			line, _, _ := strings.Cut(r.pattern, "\n")
			if err := r.write(r.lineOut(line)); err != nil {
				return ctrlEnd, err
			}
		case 'q':
			return ctrlQuit, nil
		case 'r':
			r.pending = append(r.pending, pendingOut{file: c.file})
		case 's':
			if err := r.substitute(c.sub); err != nil {
				return ctrlEnd, err
			}
		case 't':
			if r.substituted {
				r.substituted = false
				next = c.jump
			}
		case 'w':
			if err := r.writeTo(c.file, r.lineOut(r.pattern)); err != nil {
				return ctrlEnd, err
			}
		case 'x':
			r.pattern, r.hold = r.hold, r.pattern
		case 'y':
			r.pattern = transliterate(c.translit, r.pattern)
		}
		i = next
	}
	return ctrlEnd, nil
}

// selected reports whether the addresses of a command select the pattern
// space, updating the state of its range.
func (r *runner) selected(c *command) bool {
	sel := true
	c.rangeEnd = false
	switch {
	case c.a1 == nil:
	case c.a2 == nil:
		sel = r.matches(c.a1)
	default:
		if c.inRange {
			if r.matchesEnd(c.a2) {
				c.inRange = false
				c.rangeEnd = true
			}
		} else if r.matches(c.a1) {
			c.inRange = true
			// The second address is attempted starting with the line
			// following the first selected one, except for a line number at
			// or before it, which selects just that line.
			if c.a2.re == nil && !c.a2.last && r.lineNum >= c.a2.line {
				c.inRange = false
				c.rangeEnd = true
			}
		} else {
			sel = false
		}
	}
	if c.neg {
		return !sel
	}
	return sel
}

// matches reports whether an address selects the pattern space.
func (r *runner) matches(a *address) bool {
	switch {
	case a.last:
		return r.atLast
	case a.re != nil:
		return a.re.MatchString(r.pattern)
	default:
		return r.lineNum == a.line
	}
}

// matchesEnd reports whether the second address of a range closes it on the
// pattern space. A line number closes the range at that line or before it.
func (r *runner) matchesEnd(a *address) bool {
	switch {
	case a.last:
		return r.atLast
	case a.re != nil:
		return a.re.MatchString(r.pattern)
	default:
		return r.lineNum >= a.line
	}
}

// substitute applies an s command to the pattern space.
func (r *runner) substitute(s *subst) error {
	matches := s.re.FindAllSubmatchIndex([]byte(r.pattern), -1)
	if len(matches) == 0 {
		return nil
	}
	var use [][]int
	switch {
	case s.global:
		use = matches
	case s.nth > 0:
		if s.nth > len(matches) {
			return nil
		}
		use = [][]int{matches[s.nth-1]}
	default:
		use = matches[:1]
	}

	var b strings.Builder
	pos := 0
	for _, m := range use {
		b.WriteString(r.pattern[pos:m[0]])
		expand(&b, s.tmpl, r.pattern, m)
		pos = m[1]
	}
	b.WriteString(r.pattern[pos:])
	r.pattern = b.String()
	r.substituted = true

	if s.print {
		if err := r.write(r.lineOut(r.pattern)); err != nil {
			return err
		}
	}
	if s.file != "" {
		if err := r.writeTo(s.file, r.lineOut(r.pattern)); err != nil {
			return err
		}
	}
	return nil
}

// expand writes the replacement of a substitution for the match m.
func expand(b *strings.Builder, tmpl []replItem, line string, m []int) {
	for _, item := range tmpl {
		switch {
		case item.group >= 0:
			if lo := 2 * item.group; lo+1 < len(m) && m[lo] >= 0 {
				b.WriteString(line[m[lo]:m[lo+1]])
			}
		case item.group == -1:
			b.WriteString(line[m[0]:m[1]])
		default:
			b.WriteString(item.literal)
		}
	}
}

// flushPending writes the text and files scheduled by the a and r commands,
// in the order they were applied.
func (r *runner) flushPending() error {
	for _, out := range r.pending {
		if out.file != "" {
			if err := r.writeReader(out.file); err != nil {
				return err
			}
			continue
		}
		if err := r.write(out.text + "\n"); err != nil {
			return err
		}
	}
	r.pending = nil
	return nil
}

// writeReader writes the contents of a file to the standard output. A file
// that is missing or can't be read is treated as an empty one, causing no
// error condition.
func (r *runner) writeReader(name string) error {
	if r.p.FS == nil {
		return nil
	}
	f, err := r.p.FS.Open(r.p.path(name))
	if err != nil {
		return nil
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		return nil // an unreadable file is treated as an empty one
	}
	return r.write(string(content))
}

// openWriters creates the files written by the w commands and the w flag of
// the s command, truncating any existing contents.
func (r *runner) openWriters() error {
	r.writers = map[string]fsx.File{}
	for _, c := range r.cmds {
		var names []string
		if c.verb == 'w' {
			names = append(names, c.file)
		}
		if c.sub != nil && c.sub.file != "" {
			names = append(names, c.sub.file)
		}
		for _, name := range names {
			if _, ok := r.writers[name]; ok || name == "" {
				continue
			}
			if r.p.FS == nil {
				return errors.New("no file system available")
			}
			f, err := r.p.FS.OpenFile(r.p.path(name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^r.p.Umask.Perm())
			if err != nil {
				return fmt.Errorf("can't create %s: %v", name, patherr.Unwrap(err))
			}
			r.writers[name] = f
		}
	}
	return nil
}

func (r *runner) closeWriters() {
	for _, f := range r.writers {
		f.Close()
	}
}

// writeTo appends a line to a wfile.
func (r *runner) writeTo(name string, s string) error {
	f, ok := r.writers[name]
	if !ok {
		return fmt.Errorf("can't write to %s", name)
	}
	if _, err := f.Write([]byte(s)); err != nil {
		return fmt.Errorf("write error: %v", err)
	}
	return nil
}

// write writes a line to the standard output.
func (r *runner) write(s string) error {
	if _, err := io.WriteString(r.p.Stdout, s); err != nil {
		return fmt.Errorf("write error: %v", err)
	}
	return nil
}

// formatL renders the pattern space in the visually unambiguous form of the
// l command.
func formatL(s string) string {
	var b strings.Builder
	col := 0
	write := func(str string) {
		if col+len(str) > lWidth {
			b.WriteString("\\\n")
			col = 0
		}
		b.WriteString(str)
		col += len(str)
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case escapeSeq[c] != "":
			write(escapeSeq[c])
			i++
		case c < 0x20 || c == 0x7f:
			write(fmt.Sprintf("\\%03o", c))
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				write(fmt.Sprintf("\\%03o", c))
				i++
				continue
			}
			if unicode.IsPrint(r) {
				write(s[i : i+size])
			} else {
				for j := i; j < i+size; j++ {
					write(fmt.Sprintf("\\%03o", s[j]))
				}
			}
			i += size
		}
	}
	b.WriteString("$\n")
	return b.String()
}

var escapeSeq = map[byte]string{
	'\\': `\\`,
	'\a': `\a`,
	'\b': `\b`,
	'\f': `\f`,
	'\r': `\r`,
	'\t': `\t`,
	'\v': `\v`,
	'\n': `\n`,
}

// transliterate replaces the characters of a y command found in s by their
// counterparts, keeping the ones that aren't in its string1.
func transliterate(m map[rune]rune, s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if to, ok := m[r]; ok {
			r = to
		}
		b.WriteRune(r)
	}
	return b.String()
}
