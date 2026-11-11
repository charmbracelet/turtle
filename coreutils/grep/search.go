package grep

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/textutil"
)

// searchInput is one input of a search: the standard input, a file operand,
// or a file met while recursing.
type searchInput struct {
	name   string // the name as given
	stdin  bool
	fsName string // the name the file system is asked for
}

// label returns the name of the input as the output writes it.
func (in searchInput) label(opts options) string {
	if in.stdin {
		if opts.label != "" {
			return opts.label
		}
		return "(standard input)"
	}
	return in.name
}

// accessError is an error opening or reading an input file, the messages of
// which -s suppresses.
type accessError struct {
	err error
}

func (e *accessError) Error() string { return e.err.Error() }
func (e *accessError) Unwrap() error { return e.err }

// searchFile searches one input for matching lines, writing the output the
// options ask for, and reports whether any line was selected.
func (p *Prog) searchFile(ctx context.Context, m *matcher, in searchInput, opts options, multiple bool, col *colors) (matched bool, err error) {
	r, err := p.openInput(ctx, in)
	if err != nil {
		return false, &accessError{err}
	}
	defer func() {
		if cerr := r.Close(); cerr != nil && err == nil {
			err = &accessError{cerr}
		}
	}()
	s := &searcher{
		p: p, m: m, in: in, opts: opts, col: col,
		w: newFilePrinter(p, in, opts, in.label(opts), multiple, col),
	}
	if opts.maxCount == 0 {
		// No line is ever selected, but the output is still finalized.
		s.done = true
	}
	reader := bufio.NewReader(r)
	term := textutil.Delim(opts.nullData)
	var offset int64
	for !s.done {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		line, readErr := reader.ReadBytes(term)
		if len(line) > 0 {
			data := line
			if data[len(data)-1] == term {
				data = data[:len(data)-1]
			}
			s.lineNum++
			s.line(data, s.lineNum, offset)
			offset += int64(len(line))
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return s.selected, &accessError{readErr}
		}
	}
	s.finish()
	if s.w.err != nil {
		return s.selected, s.w.err
	}
	return s.selected, nil
}

// inputSize returns the size of a regular input, which -T aligns its
// numbers on. The standard input and the other files report none.
func (p *Prog) inputSize(in searchInput) (int64, bool) {
	if in.stdin || p.FS == nil {
		return 0, false
	}
	fi, err := fs.Stat(p.FS, in.fsName)
	if err != nil || !fi.Mode().IsRegular() {
		return 0, false
	}
	return fi.Size(), true
}

// openInput returns a reader for one input. Reads stop early when ctx is
// done. The standard input isn't owned by the caller, so it's wrapped on a
// no-op closer.
func (p *Prog) openInput(ctx context.Context, in searchInput) (io.ReadCloser, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return io.NopCloser(ctxio.NewReader(ctx, p.Stdin)), nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(in.fsName)
	if err != nil {
		return nil, err
	}
	return ctxio.ReadCloser{Ctx: ctx, RC: f}, nil
}

// searcher holds the state of the search of one input.
type searcher struct {
	p    *Prog
	m    *matcher
	in   searchInput
	opts options
	w    *filePrinter
	col  *colors

	selected bool
	count    int64 // selected lines so far
	lineNum  int64 // lines read so far
	binary   bool  // the input holds null bytes
	done     bool  // the rest of the input is left unread
	maxed    bool  // the max count of selected lines was reached
}

// line processes one line of the input.
func (s *searcher) line(data []byte, num, off int64) {
	if s.maxed {
		// Only the trailing context of the last selected line is written.
		if s.w.trailing() {
			s.w.line(false, num, off, data, nil)
		} else {
			s.done = true
		}
		return
	}
	if !s.opts.nullData && s.opts.binary != 'a' && !s.binary && bytes.IndexByte(data, 0) >= 0 {
		s.binary = true
	}
	if s.binary {
		s.binaryLine(data)
		return
	}
	selected := s.m.matches(data)
	if selected {
		s.selected = true
		s.count++
	}
	switch s.opts.output {
	case 'q':
		if selected {
			s.done = true
		}
	case 'c':
		// The count is written at the end of the input.
	case 'l', 'L':
		if selected {
			if s.opts.output == 'l' {
				s.p.printName(s.in.label(s.opts), s.opts, s.col)
			}
			s.done = true
		}
	default:
		if s.opts.onlyMatching {
			if selected {
				for _, loc := range s.m.find(data) {
					s.w.match(num, off+int64(loc[0]), data[loc[0]:loc[1]])
				}
			}
		} else {
			var matches [][]int
			if s.col != nil {
				matches = s.m.find(data)
			}
			s.w.line(selected, num, off, data, matches)
		}
	}
	if s.opts.maxCount >= 0 && s.count >= s.opts.maxCount {
		// Stop after the selected lines the max count asks for, but write
		// the trailing context that follows them.
		s.maxed = true
		if !s.w.trailing() {
			s.done = true
		}
	}
}

// binaryLine processes one line of an input which null bytes made binary.
func (s *searcher) binaryLine(data []byte) {
	if s.opts.binary == 'I' {
		// Any unprocessed input is taken not to match.
		s.done = true
		return
	}
	// The default keeps finding the selected lines of the rest of the
	// input, but suppresses their output, reporting the input as binary and
	// leaving it unread as soon as one is selected.
	if !s.m.matches(data) {
		return
	}
	s.selected = true
	s.count++
	switch s.opts.output {
	case 'c':
		// The count keeps running through the rest of the input.
	case 'q':
		s.done = true
	case 'l':
		s.p.printName(s.in.label(s.opts), s.opts, s.col)
		s.done = true
	case 'L':
		s.done = true
	default:
		fmt.Fprintf(s.p.Stdout, "Binary file %s matches\n", s.in.label(s.opts))
		s.done = true
	}
}

// finish writes what the output options ask for at the end of an input.
func (s *searcher) finish() {
	switch s.opts.output {
	case 'c':
		sep := ":"
		if s.opts.nullName {
			sep = "\x00"
		}
		prefix := ""
		if s.w.multi {
			prefix = s.col.colorize(s.col.fieldColor('f'), s.in.label(s.opts)) + sep
		}
		fmt.Fprintf(s.p.Stdout, "%s%d\n", prefix, s.count)
	case 'L':
		if !s.selected {
			s.p.printName(s.in.label(s.opts), s.opts, s.col)
		}
	}
}
