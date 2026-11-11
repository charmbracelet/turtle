package sed

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/snail/coreutils/internal/output"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/textutil"
	"charm.land/snail/fsx"
)

// lDefaultWidth is the column at which the l command folds long lines, when
// neither its own argument nor the -l option says otherwise.
const lDefaultWidth = 70

// The names of the standard streams, which the commands writing and reading
// files accept besides the names of the file system.
const (
	devStdin  = "/dev/stdin"
	devStdout = "/dev/stdout"
	devStderr = "/dev/stderr"
)

// ctrl tells how a cycle ended.
type ctrl int

const (
	ctrlEnd  ctrl = iota // the end of the script was reached
	ctrlNext             // the next cycle starts, reading a new line
	ctrlSkip             // the next cycle starts without reading a line
	ctrlQuit             // the script quits, writing the pattern space
	ctrlDone             // the script quits without writing the pattern space
)

// runResult holds what a run of the script asks the program to exit with:
// the status of a q or Q command, and the failure to open an input file,
// which takes precedence over it.
type runResult struct {
	exit       int
	openFailed bool
}

// pendingOut is the text or the file an a, r or R command scheduled for
// output, by kind: the text of an a command, the whole file of an r one, or
// a single line read by an R one.
type pendingOut struct {
	kind byte
	text string
	file string
}

// source reads records from the input files, or the standard input when no
// files are given. A one-record lookahead tells whether the current record
// is the last one, for the `$` address. An input file that can't be opened
// is reported and skipped, as GNU sed does.
type source struct {
	p     *Prog
	ctx   context.Context
	res   *runResult
	paths []string
	idx   int
	r     *bufio.Reader
	file  io.Closer
	sep   byte
	// name is the file the reader holds, and readName the one the record
	// last fetched came from, which the lookahead of another file may
	// already have moved past.
	name     string
	readName string

	buf     string
	bufName string
	has     bool
	bufTerm bool
	// err holds an error hit by the lookahead fetch, so that the record
	// read before it isn't lost: it is returned on the following fetches
	// until the input ends.
	err error
}

func (p *Prog) newSource(ctx context.Context, res *runResult, files []string, o *options) *source {
	paths := make([]string, len(files))
	copy(paths, files)
	if len(paths) == 0 {
		paths = []string{"-"}
	}
	sep := textutil.Delim(o.nullData)
	return &source{p: p, ctx: ctx, res: res, paths: paths, sep: sep}
}

// next returns the next record of input, without its terminating separator,
// the name of the file it came from, and whether it is the last one. An
// error hit by the lookahead fetch of the previous call keeps the record
// read before it: the record is returned, and the error follows it on the
// next fetch.
func (s *source) next() (line, name string, last, hasTerm bool, err error) {
	line, hasTerm, err = s.fetch()
	if err != nil {
		return "", "", false, false, err
	}
	name = s.readName
	peek, peekTerm, perr := s.fetch()
	switch perr {
	case nil:
		s.buf, s.has, s.bufTerm, s.bufName = peek, true, peekTerm, s.readName
		return line, name, false, hasTerm, nil
	case io.EOF:
		return line, name, true, hasTerm, nil
	default:
		s.err = perr // the record was read fine, so the error can wait
		return line, name, false, hasTerm, nil
	}
}

// fetch returns the next record of input and whether it had a terminating
// separator, taking the lookahead record first and moving to the next input
// file when the current one ends.
func (s *source) fetch() (string, bool, error) {
	if s.err != nil {
		return "", false, s.err // the stashed lookahead error is due
	}
	if s.has {
		s.has = false
		line, term, name := s.buf, s.bufTerm, s.bufName
		s.buf, s.bufTerm, s.bufName = "", false, ""
		s.readName = name
		return line, term, nil
	}
	for {
		if s.r == nil {
			if s.idx >= len(s.paths) {
				return "", false, io.EOF
			}
			name := s.paths[s.idx]
			s.idx++
			rc, err := s.open(name)
			if err != nil {
				stdopts.Errorf(s.p.Runtime, "sed", "%v", err)
				s.res.openFailed = true
				continue // the remaining files still get their turn
			}
			s.name = name
			s.r = bufio.NewReader(rc)
			s.file = rc
		}
		line, err := s.r.ReadBytes(s.sep)
		switch err {
		case nil:
			s.readName = s.name
			return string(line[:len(line)-1]), true, nil
		case io.EOF:
			if len(line) > 0 {
				s.readName = s.name
				return string(line), false, nil // the file ends without one
			}
			s.close()
		default:
			return "", false, fmt.Errorf("read error: %v", err)
		}
	}
}

// open returns a reader for an input file, with the standard input used when
// the name is "-" or one of its aliases.
func (s *source) open(name string) (io.ReadCloser, error) {
	if name == "-" || name == devStdin {
		if s.p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return io.NopCloser(s.p.Stdin), nil
	}
	if s.p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := s.p.FS.Open(s.p.FsName(name))
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
	s.name = ""
}

// run executes the script against the input, writing the output to out, or
// the standard output when it's nil. With -s every file runs as its own
// stream, restarting the line numbers, the ranges and the hold space.
func (p *Prog) run(ctx context.Context, cmds []*command, files []string, o *options, quiet bool, out io.Writer) (runResult, error) {
	if out == nil {
		out = p.Stdout
	}
	res := &runResult{}
	r := &runner{
		p:      p,
		ctx:    ctx,
		cmds:   cmds,
		res:    res,
		quiet:  quiet,
		out:    out,
		sep:    "\n",
		lWidth: o.lineLength,
	}
	if o.nullData {
		r.sep = "\x00"
	}
	defer r.closeFiles()
	// The writers are closed even when opening one of them fails, so the
	// ones already open don't leak.
	if err := r.openWriters(); err != nil {
		return *res, err
	}

	groups := [][]string{files}
	if o.separate && len(files) > 0 {
		groups = make([][]string, len(files))
		for i, f := range files {
			groups[i] = []string{f}
		}
	}
	for _, group := range groups {
		r.src = p.newSource(ctx, res, group, o)
		if o.separate {
			r.resetFile()
		}
		quit, err := r.cycles()
		r.src.close()
		if err != nil {
			return *res, err
		}
		if quit {
			return *res, nil
		}
	}
	return *res, nil
}

// runInPlace edits each of the given files with the script. A file that
// fails doesn't keep the ones after it from being edited, and the first
// error is returned.
//
// The script is parsed again for each file, so line numbers, ranges and
// branches start over for every one of them, as they do for GNU sed, which
// handles each file like a separate invocation.
func (p *Prog) runInPlace(ctx context.Context, script string, o *options, files []string, quiet bool) (runResult, error) {
	var res runResult
	var firstErr error
	for _, name := range files {
		cmds, err := parseScript(script, o) // validated above, so it can't fail
		if err != nil {
			return res, err
		}
		one, err := p.editInPlace(ctx, cmds, name, o, quiet)
		if one.openFailed {
			res.openFailed = true
		}
		if one.exit != 0 && res.exit == 0 {
			res.exit = one.exit
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return res, firstErr
}

// editInPlace edits the file named name with the script, collecting its
// output in a temporary file next to it and renaming it over the original
// when the script is done. When a suffix is given, the original contents
// first move to the file named name plus the suffix, or to the name the
// suffix makes when it holds a `*`, which one of them alone asks for no
// backup at all. With --follow-symlinks, a symbolic link keeps pointing to
// the file which is edited.
func (p *Prog) editInPlace(ctx context.Context, cmds []*command, name string, o *options, quiet bool) (runResult, error) {
	if p.FS == nil {
		return runResult{}, errors.New("no file system available")
	}
	target := p.FsName(name)
	if o.followSymlinks {
		target = p.followSymlink(target)
	}
	tmp, tmpPath, err := p.openTemp(target)
	if err != nil {
		return runResult{}, err
	}
	res, err := p.run(ctx, cmds, []string{name}, o, quiet, tmp)
	tmp.Close()
	if err != nil || res.openFailed {
		p.FS.Remove(tmpPath) // the original stays as it was
		return res, err
	}

	if fi, err := fs.Stat(p.FS, target); err == nil {
		if err := p.FS.Chmod(tmpPath, fi.Mode().Perm()); err != nil {
			p.FS.Remove(tmpPath)
			return res, fmt.Errorf("can't set mode of %s: %v", name, patherr.Unwrap(err))
		}
	}
	if o.suffix != "" && o.suffix != "*" {
		backup, display := p.backupPath(name, target, o.suffix)
		if err := p.FS.Rename(target, backup); err != nil {
			p.FS.Remove(tmpPath)
			return res, fmt.Errorf("can't create %s: %v", display, patherr.Unwrap(err))
		}
	}
	if err := p.FS.Rename(tmpPath, target); err != nil {
		return res, fmt.Errorf("can't create %s: %v", name, patherr.Unwrap(err))
	}
	return res, nil
}

// backupPath returns the file system path of the backup of the target, and
// the name to report it with. A suffix holding a `*` is a whole file name,
// into which every `*` writes the name being edited, which GNU sed lets put
// the backup in another directory.
func (p *Prog) backupPath(name, target, suffix string) (string, string) {
	if strings.Contains(suffix, "*") {
		display := strings.ReplaceAll(suffix, "*", name)
		return p.FsName(display), display
	}
	return target + suffix, name + suffix
}

// followSymlink resolves the chain of symbolic links path starts at, so that
// in-place editing writes through the link instead of replacing it. The path
// is returned as it is when it isn't a link, when the file system can't read
// one, or when the chain grows too long.
func (p *Prog) followSymlink(slashPath string) string {
	links, ok := p.FS.(fs.ReadLinkFS)
	if !ok {
		return slashPath
	}
	for range 40 {
		target, err := links.ReadLink(slashPath)
		if err != nil {
			return slashPath // not a symbolic link
		}
		if !strings.HasPrefix(target, "/") {
			target = pathpkg.Join(pathpkg.Dir(slashPath), target)
		}
		slashPath = target
	}
	return slashPath
}

// openTemp opens a temporary output file next to the target path, with the
// name it got returned as well. The names are tried in sequence until one
// isn't taken, so the leftovers of a failed run aren't reused.
func (p *Prog) openTemp(target string) (fsx.File, string, error) {
	base := target + ".sedtmp"
	for i := 1; ; i++ {
		tmpPath := fmt.Sprintf("%s%d", base, i)
		f, err := p.FS.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666&^p.Umask.Perm())
		switch {
		case errors.Is(err, fs.ErrExist):
			continue
		case err != nil:
			return nil, "", fmt.Errorf("can't create %s: %v", tmpPath, patherr.Unwrap(err))
		}
		return f, tmpPath, nil
	}
}

// rfile is the reading state an R command keeps, to give one line of its
// file each time it runs.
type rfile struct {
	rd   *bufio.Reader
	done bool
}

// runner is the state of a script running against the input.
type runner struct {
	p           *Prog
	ctx         context.Context
	cmds        []*command
	res         *runResult
	quiet       bool
	out         io.Writer
	src         *source
	sep         string // the record separator, a NUL with -z
	lWidth      int
	pattern     string
	hold        string
	lineNum     int
	atLast      bool
	hasTerm     bool
	substituted bool
	quitting    bool
	pending     []pendingOut
	writers     map[string]io.Writer
	closers     []io.Closer
	rfiles      map[*command]*rfile
	curFile     string
}

// resetFile starts a file as its own stream, the way a separate invocation
// of the script would: the line numbers, the ranges and the hold space all
// start over.
func (r *runner) resetFile() {
	r.lineNum, r.atLast, r.substituted, r.hold = 0, false, false, ""
	for _, c := range r.cmds {
		c.inRange = false
	}
}

// readLine fetches the next record of input, writing the text and files
// scheduled by the a, r and R commands before the attempt.
func (r *runner) readLine() (string, string, bool, bool, error) {
	if err := r.flushPending(); err != nil {
		return "", "", false, false, err
	}
	return r.src.next()
}

// lineOut appends the separator a record had in the input, so that one which
// ends the input without it keeps that.
func (r *runner) lineOut(line string) string {
	if r.hasTerm {
		return line + r.sep
	}
	return line
}

// cycles runs the script against the records of one input stream, until the
// input ends or a command quits, reporting whether the script quit.
func (r *runner) cycles() (bool, error) {
	skip := false // a D command ended the cycle without reading a line
	for {
		if !skip {
			if err := r.ctx.Err(); err != nil {
				return false, err
			}
			line, name, last, hasTerm, err := r.readLine()
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			r.pattern, r.curFile = line, name
			r.lineNum, r.atLast, r.hasTerm, r.substituted = r.lineNum+1, last, hasTerm, false
		}
		skip = false

		done, err := r.exec()
		if err != nil {
			return false, err
		}
		switch done {
		case ctrlEnd:
			// The a, r and R text is written after copying the pattern
			// space when reaching the end of the script.
			if !r.quiet {
				if err := r.write(r.lineOut(r.pattern)); err != nil {
					return false, err
				}
			}
			if err := r.flushPending(); err != nil {
				return false, err
			}
		case ctrlQuit:
			// The a, r and R text is written just before executing q, so
			// before the pattern space is written.
			if err := r.flushPending(); err != nil {
				return false, err
			}
			if !r.quiet {
				if err := r.write(r.lineOut(r.pattern)); err != nil {
					return false, err
				}
			}
			return true, nil
		case ctrlDone:
			if err := r.flushPending(); err != nil {
				return false, err
			}
			return r.quitting, nil
		case ctrlSkip:
			skip = true // the pattern space is kept, and no line is read
		}
	}
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
			if err := r.write(fmt.Sprintf("%d%s", r.lineNum, r.sep)); err != nil {
				return ctrlEnd, err
			}
		case 'a':
			// With a range, the text goes out once its last line ends.
			if c.a2 == nil || c.neg || c.rangeEnd {
				r.pending = append(r.pending, pendingOut{kind: 'a', text: c.text})
			}
		case 'b':
			next = c.jump
		case 'c':
			// The text goes out with a one-address command, at the end
			// of a non-negated two-address range, and on every line a
			// negated two-address range selects.
			if c.a2 == nil || c.neg || c.rangeEnd {
				if err := r.write(c.text + r.sep); err != nil {
					return ctrlEnd, err
				}
			}
			r.pattern = ""
			return ctrlNext, nil
		case 'd':
			r.pattern = ""
			return ctrlNext, nil
		case 'D':
			if idx := strings.IndexByte(r.pattern, r.sep[0]); idx >= 0 {
				r.pattern = r.pattern[idx+1:]
				return ctrlSkip, nil
			}
			r.pattern = ""
			return ctrlNext, nil
		case 'F':
			if err := r.write(r.curFile + r.sep); err != nil {
				return ctrlEnd, err
			}
		case 'g':
			r.pattern = r.hold
		case 'G':
			r.pattern += r.sep + r.hold
		case 'h':
			r.hold = r.pattern
		case 'H':
			r.hold += r.sep + r.pattern
		case 'i':
			// With a range, the text goes out once, before its first line.
			if c.a2 == nil || c.neg || c.rangeStart {
				if err := r.write(c.text + r.sep); err != nil {
					return ctrlEnd, err
				}
			}
		case 'l':
			width := c.width
			if width < 0 {
				width = r.lWidth
			}
			if err := r.write(formatL(r.pattern, width)); err != nil {
				return ctrlEnd, err
			}
		case 'n':
			if !r.quiet {
				if err := r.write(r.lineOut(r.pattern)); err != nil {
					return ctrlEnd, err
				}
			}
			line, name, last, hasTerm, err := r.readLine()
			switch {
			case errors.Is(err, io.EOF):
				return ctrlDone, nil
			case err != nil:
				return ctrlEnd, err
			}
			r.pattern, r.curFile = line, name
			r.lineNum, r.atLast, r.hasTerm, r.substituted = r.lineNum+1, last, hasTerm, false
		case 'N':
			line, _, last, hasTerm, err := r.readLine()
			switch {
			case errors.Is(err, io.EOF):
				return ctrlDone, nil
			case err != nil:
				return ctrlEnd, err
			}
			r.pattern += r.sep + line
			r.lineNum, r.atLast, r.hasTerm, r.substituted = r.lineNum+1, last, hasTerm, false
		case 'p':
			if err := r.write(r.lineOut(r.pattern)); err != nil {
				return ctrlEnd, err
			}
		case 'P':
			line, _, _ := strings.Cut(r.pattern, r.sep)
			if err := r.write(r.lineOut(line)); err != nil {
				return ctrlEnd, err
			}
		case 'q':
			r.setExit(c)
			return ctrlQuit, nil
		case 'Q':
			// The pattern space is left unprinted, but the text scheduled
			// by the a, r and R commands still goes out.
			r.setExit(c)
			r.quitting = true
			return ctrlDone, nil
		case 'r':
			if c.a2 == nil || c.neg || c.rangeEnd {
				r.pending = append(r.pending, pendingOut{kind: 'r', file: c.file})
			}
		case 'R':
			if line, ok := r.nextFileLine(c); ok {
				r.pending = append(r.pending, pendingOut{kind: 'R', text: line})
			}
		case 's':
			if err := r.substitute(c.sub); err != nil {
				return ctrlEnd, err
			}
		case 't':
			if r.substituted {
				r.substituted = false
				next = c.jump
			}
		case 'T':
			// T branches when nothing was substituted, and takes the flag
			// away when something was, as GNU sed does, so a following t
			// or T starts over.
			if r.substituted {
				r.substituted = false
			} else {
				next = c.jump
			}
		case 'w':
			if err := r.writeTo(c.file, r.lineOut(r.pattern)); err != nil {
				return ctrlEnd, err
			}
		case 'W':
			if err := r.writeTo(c.file, r.firstLine()); err != nil {
				return ctrlEnd, err
			}
		case 'x':
			r.pattern, r.hold = r.hold, r.pattern
		case 'y':
			r.pattern = transliterate(c.translit, r.pattern)
		case 'z':
			r.pattern = ""
		}
		i = next
	}
	return ctrlEnd, nil
}

// setExit keeps the status a q or Q command asks the program to exit with.
func (r *runner) setExit(c *command) {
	if c.exit != 0 {
		r.res.exit = c.exit
	}
}

// firstLine renders the first record of the pattern space, as the W command
// writes it.
func (r *runner) firstLine() string {
	line, _, cut := strings.Cut(r.pattern, r.sep)
	if cut {
		return line + r.sep
	}
	return r.lineOut(line)
}

// selected reports whether the addresses of a command select the pattern
// space, updating the state of its range.
func (r *runner) selected(c *command) bool {
	sel := true
	c.rangeEnd = false
	c.rangeStart = false
	switch {
	case c.a1 == nil:
	case c.a2 == nil:
		sel = r.matches(c.a1)
	default:
		if c.inRange {
			if r.matchesEnd(c) {
				c.inRange = false
				c.rangeEnd = true
			}
		} else if r.matches(c.a1) {
			c.inRange = true
			c.rangeStart = true
			// A +N or ~N second address closes on a line counted from the
			// one opening the range, and the 0 first address lets the
			// second one close it on its own opening line.
			switch {
			case c.a2.rel > 0:
				c.endLine = r.lineNum + c.a2.rel
			case c.a2.mod > 0:
				c.endLine = (r.lineNum/c.a2.mod + 1) * c.a2.mod
			}
			if c.a1.zero || c.a2.rel > 0 || c.a2.mod > 0 ||
				(c.a2.re == nil && !c.a2.last && r.lineNum >= c.a2.line) {
				if r.matchesEnd(c) {
					c.inRange = false
					c.rangeEnd = true
				}
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
	case a.zero:
		return r.lineNum == 1 // the 0 address starts a range at the first line
	case a.step > 0:
		return r.lineNum >= a.line && (r.lineNum-a.line)%a.step == 0
	default:
		return r.lineNum == a.line
	}
}

// matchesEnd reports whether the second address of a range closes it on the
// pattern space. A line number closes the range at that line or before it.
func (r *runner) matchesEnd(c *command) bool {
	a := c.a2
	switch {
	case a.last:
		return r.atLast
	case a.re != nil:
		return a.re.MatchString(r.pattern)
	case a.rel > 0 || a.mod > 0:
		return r.lineNum >= c.endLine
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
	case s.nth > 0 && s.global: // every match from the nth one on
		if s.nth <= len(matches) {
			use = matches[s.nth-1:]
		}
	case s.nth > 0:
		if s.nth <= len(matches) {
			use = [][]int{matches[s.nth-1]}
		}
	case s.global:
		use = matches
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
	var c caser
	for _, item := range tmpl {
		switch {
		case item.caseOp != 0:
			c.set(item.caseOp)
		case item.group >= 0:
			if lo := 2 * item.group; lo+1 < len(m) && m[lo] >= 0 {
				c.write(b, line[m[lo]:m[lo+1]])
			}
		case item.group == -1:
			c.write(b, line[m[0]:m[1]])
		default:
			c.write(b, item.literal)
		}
	}
}

// caser applies the \u \l \U \L \E escapes of a replacement to the text the
// replacement writes, be it literal or taken from the match.
type caser struct {
	upper, lower, nextUpper, nextLower bool
}

func (c *caser) set(op byte) {
	switch op {
	case 'U':
		c.upper, c.lower = true, false
	case 'L':
		c.lower, c.upper = true, false
	case 'E':
		*c = caser{}
	case 'u':
		c.nextUpper = true
	case 'l':
		c.nextLower = true
	}
}

func (c *caser) write(b *strings.Builder, s string) {
	if s == "" {
		return
	}
	// The \u and \l escapes take the next character after the running
	// conversion, which then goes on with the rest of the text.
	switch {
	case c.upper:
		s = strings.ToUpper(s)
	case c.lower:
		s = strings.ToLower(s)
	}
	if c.nextUpper || c.nextLower {
		r, size := utf8.DecodeRuneInString(s)
		if c.nextUpper {
			r = unicode.ToUpper(r)
		} else {
			r = unicode.ToLower(r)
		}
		c.nextUpper, c.nextLower = false, false
		s = string(r) + s[size:]
	}
	b.WriteString(s)
}

// flushPending writes the text and files scheduled by the a, r and R
// commands, in the order they were applied.
func (r *runner) flushPending() error {
	for _, out := range r.pending {
		switch out.kind {
		case 'r':
			if err := r.writeReader(out.file); err != nil {
				return err
			}
		case 'R':
			if err := r.write(out.text); err != nil {
				return err
			}
		default:
			if err := r.write(out.text + r.sep); err != nil {
				return err
			}
		}
	}
	r.pending = nil
	return nil
}

// writeReader writes the contents of a file to the standard output. A file
// that is missing or can't be read is treated as an empty one, causing no
// error condition.
func (r *runner) writeReader(name string) error {
	if name == devStdin {
		if r.p.Stdin == nil {
			return nil
		}
		content, err := io.ReadAll(r.p.Stdin)
		if err != nil {
			return nil // an unreadable stream is treated as an empty one
		}
		return r.write(string(content))
	}
	if r.p.FS == nil {
		return nil
	}
	f, err := r.p.FS.Open(r.p.FsName(name))
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

// nextFileLine reads the next line of the file of an R command, keeping the
// reading state of every one of them. A file which is missing or exhausted
// has no line left to give.
func (r *runner) nextFileLine(c *command) (string, bool) {
	state := r.rfiles[c]
	if state == nil {
		if r.rfiles == nil {
			r.rfiles = map[*command]*rfile{}
		}
		state = r.openFileLine(c.file)
		r.rfiles[c] = state
	}
	if state.done {
		return "", false
	}
	line, _ := state.rd.ReadString('\n')
	if line == "" {
		state.done = true // at the end of the file, or reading it failed
		return "", false
	}
	return line, true
}

// openFileLine opens the file an R command reads its lines from, with the
// standard input used for its name.
func (r *runner) openFileLine(name string) *rfile {
	if name == devStdin {
		if r.p.Stdin == nil {
			return &rfile{done: true}
		}
		return &rfile{rd: bufio.NewReader(r.p.Stdin)}
	}
	if r.p.FS == nil {
		return &rfile{done: true}
	}
	f, err := r.p.FS.Open(r.p.FsName(name))
	if err != nil {
		return &rfile{done: true} // a missing file is an empty one
	}
	r.closers = append(r.closers, f)
	return &rfile{rd: bufio.NewReader(f)}
}

// openWriters creates the files written by the w and W commands and the w
// flag of the s command, truncating any existing contents. The names of the
// standard streams write to the streams of the runtime instead.
func (r *runner) openWriters() error {
	r.writers = map[string]io.Writer{}
	for _, c := range r.cmds {
		var names []string
		if c.verb == 'w' || c.verb == 'W' {
			names = append(names, c.file)
		}
		if c.sub != nil && c.sub.file != "" {
			names = append(names, c.sub.file)
		}
		for _, name := range names {
			if name == "" {
				continue
			}
			if _, ok := r.writers[name]; ok {
				continue
			}
			switch name {
			case devStdout:
				r.writers[name] = r.p.Stdout
				continue
			case devStderr:
				r.writers[name] = r.p.Stderr
				continue
			}
			f, err := output.Create(r.p.Runtime, r.ctx, name)
			if err != nil {
				return fmt.Errorf("can't create %s: %v", name, patherr.Unwrap(err))
			}
			r.writers[name] = f
			r.closers = append(r.closers, f)
		}
	}
	return nil
}

func (r *runner) closeFiles() {
	for _, c := range r.closers {
		c.Close()
	}
	r.closers = nil
}

// writeTo appends a line to a file written by the script.
func (r *runner) writeTo(name string, s string) error {
	w, ok := r.writers[name]
	if !ok {
		return fmt.Errorf("can't write to %s", name)
	}
	if _, err := io.WriteString(w, s); err != nil {
		return fmt.Errorf("write error: %v", err)
	}
	return nil
}

// write writes a line to the output of the run.
func (r *runner) write(s string) error {
	if _, err := io.WriteString(r.out, s); err != nil {
		return fmt.Errorf("write error: %v", err)
	}
	return nil
}

// formatL renders the pattern space in the visually unambiguous form of the
// l command, folding the line at the given width, or not at all when it
// isn't positive.
func formatL(s string, width int) string {
	var b strings.Builder
	col := 0
	write := func(str string) {
		if width > 0 && col+len(str) > width {
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
