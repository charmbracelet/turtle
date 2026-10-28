package pr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/patherr"
)

// printer paginates and prints the input files.
type printer struct {
	prog *Prog
	opts options
	w    *bufio.Writer
	err  bool // a file couldn't be processed
}

// run prints all the operands, dispatching on the output mode.
func (p *printer) run(ctx context.Context, operands []string) error {
	var err error
	switch {
	case p.opts.merged:
		err = p.printMerged(ctx, operands)
	case p.opts.cols > 1 && p.opts.across:
		err = p.printFiles(ctx, operands, p.acrossFile)
	case p.opts.cols > 1:
		err = p.printFiles(ctx, operands, p.columnsFile)
	default:
		err = p.printFiles(ctx, operands, p.singleFile)
	}
	if ferr := p.w.Flush(); err == nil && ferr != nil {
		err = &writeError{err: ferr}
	}
	return err
}

// printFiles opens and prints each operand with the given engine. Files that
// can't be opened or read are reported and skipped.
func (p *printer) printFiles(ctx context.Context, operands []string, engine func(context.Context, *source) error) error {
	if len(operands) == 0 {
		operands = []string{""}
	}
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		src, err := p.openSource(ctx, name)
		if err != nil {
			p.diagnose(srcLabel(name), err)
			continue
		}
		err = engine(ctx, src)
		src.rc.Close()
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, ok := errors.AsType[*writeError](err); ok {
			return err
		}
		// A read error ends the processing of the file.
		p.diagnose(src.label, err)
	}
	return nil
}

// diagnose reports a file processing error, unless -r silenced it.
func (p *printer) diagnose(label string, err error) {
	p.err = true
	if !p.opts.noDiag {
		p.prog.errorf("%s: %v", label, patherr.Unwrap(err))
	}
}

// openSource opens one operand and resolves its header fields.
func (p *printer) openSource(ctx context.Context, name string) (*source, error) {
	src := &source{label: srcLabel(name)}
	stdin := name == "" || name == "-"
	if stdin {
		if p.prog.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		src.rc = ctxio.NewReader(ctx, p.prog.Stdin)
	} else {
		if p.prog.FS == nil {
			return nil, errors.New("no file system available")
		}
		f, err := p.prog.FS.Open(filepath.ToSlash(p.prog.ResolvePath(name)))
		if err != nil {
			return nil, err
		}
		src.rc = ctxio.ReadCloser{Ctx: ctx, RC: f}
		src.file = f
	}
	src.br = bufio.NewReader(src.rc)

	// The header shows the file's modification time, or the current time
	// for the standard input and for merged output.
	if !p.opts.noHead {
		switch {
		case p.opts.merged || stdin:
			src.date = p.prog.now().Format(dateLayout)
		default:
			fi, err := src.file.Stat()
			if err != nil {
				src.rc.Close()
				return nil, err
			}
			src.date = fi.ModTime().In(p.prog.location()).Format(dateLayout)
		}
	}

	switch {
	case p.opts.hasHead:
		src.disp = p.opts.header
	case p.opts.merged || stdin:
		src.disp = ""
	default:
		src.disp = name
	}
	return src, nil
}

// now returns the current time in the header time zone.
func (p *Prog) now() time.Time {
	return time.Now().In(p.location())
}

// srcLabel returns the name to use on diagnostic messages.
func srcLabel(name string) string {
	if name == "" {
		return "standard input"
	}
	return name
}

// source is an input file being printed.
type source struct {
	label string // operand name, for diagnostics
	disp  string // header field
	date  string // header date
	file  fs.File
	rc    io.ReadCloser
	br    *bufio.Reader
	alive bool // merge: the file still has data
}

// next returns the next input line, expanded and truncated to the column
// width. ok is false at the end of the file.
func (s *source) next(ctx context.Context, o *options) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	line, err := s.br.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	if len(line) == 0 {
		return nil, false, nil
	}
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return expandLine(line, o), true, nil
}

// skipLines discards n lines, reporting false if the file ended first.
func (s *source) skipLines(ctx context.Context, n int) (bool, error) {
	for ; n > 0; n-- {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		line, err := s.br.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return len(line) > 0 && n == 1, nil
			}
			return false, err
		}
	}
	return true, nil
}

// expandLine expands the input tab characters of the line and truncates it
// to the column width in columns.
func expandLine(line []byte, o *options) []byte {
	if !o.expand && o.colWidth == 0 {
		return line
	}
	width := o.colWidth
	out := make([]byte, 0, len(line)+defGap)
	col := 0
	for _, c := range line {
		if o.expand && c == o.inChar {
			n := o.inGap - col%o.inGap
			if width > 0 && col+n > width {
				// A tab crossing the edge becomes blanks up to it; the
				// rest of the line is discarded.
				return appendSpaces(out, width-col)
			}
			out = appendSpaces(out, n)
			col += n
			continue
		}
		if width > 0 && col >= width {
			break
		}
		out = append(out, c)
		col++
	}
	return out
}

// singleFile prints one file in the default single-column mode.
func (p *printer) singleFile(ctx context.Context, src *source) error {
	o := &p.opts
	if ok, err := src.skipLines(ctx, (o.page-1)*o.bodyLines); err != nil || !ok {
		return err
	}
	page, lineNo := o.page, 0
	for {
		count := 0
		done := false
		for count < o.bodyLines {
			line, ok, err := src.next(ctx, o)
			if err != nil {
				return err
			}
			if !ok {
				done = true
				break
			}
			if count == 0 && !o.noHead {
				if err := p.header(page, src.disp, src.date); err != nil {
					return err
				}
			}
			lineNo++
			row := appendSpaces(nil, o.offset)
			row = append(row, p.numPrefix(lineNo)...)
			if err := p.writeLine(append(row, line...)); err != nil {
				return err
			}
			count++
		}
		if count > 0 {
			if err := p.trailer(o.bodyLines - count); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
		page++
	}
}

// columnsFile prints one file in multiple columns filled down the page.
func (p *printer) columnsFile(ctx context.Context, src *source) error {
	o := &p.opts
	if ok, err := src.skipLines(ctx, (o.page-1)*o.bodyLines*o.cols); err != nil || !ok {
		return err
	}
	page, lineNo := o.page, 0
	for {
		var cells []cell
		done := false
		for c := 0; c < o.cols && !done; c++ {
			for r := 0; r < o.bodyLines; r++ {
				text, ok, err := src.next(ctx, o)
				if err != nil {
					return err
				}
				if !ok {
					done = true
					break
				}
				lineNo++
				cells = append(cells, p.cellOf(lineNo, text))
			}
		}
		if len(cells) == 0 {
			return nil
		}
		rows := o.bodyLines
		if done && o.bodyLines*o.cols-len(cells) >= o.cols {
			// Balance the last page into the minimum number of lines.
			rows = (len(cells) + o.cols - 1) / o.cols
		}
		if !o.noHead {
			if err := p.header(page, src.disp, src.date); err != nil {
				return err
			}
		}
		for r := 0; r < rows; r++ {
			row := appendSpaces(nil, o.offset)
			for c := 0; c < o.cols; c++ {
				i := c*rows + r
				if i >= len(cells) {
					break
				}
				row = append(row, cells[i].data()...)
				if c < o.cols-1 {
					row = p.padCell(row, len(cells[i].text))
				}
			}
			if err := p.writeLine(row); err != nil {
				return err
			}
		}
		if err := p.trailer(o.bodyLines - rows); err != nil {
			return err
		}
		if done {
			return nil
		}
		page++
	}
}

// acrossFile prints one file in multiple columns filled across the page.
func (p *printer) acrossFile(ctx context.Context, src *source) error {
	o := &p.opts
	if ok, err := src.skipLines(ctx, (o.page-1)*o.bodyLines*o.cols); err != nil || !ok {
		return err
	}
	page, lineNo := o.page, 0
	for {
		rows := 0
		done := false
		for rows < o.bodyLines {
			row := appendSpaces(nil, o.offset)
			var prev *cell
			filled := 0
			for c := 0; c < o.cols; c++ {
				text, ok, err := src.next(ctx, o)
				if err != nil {
					return err
				}
				if !ok {
					done = true
					break
				}
				lineNo++
				cur := p.cellOf(lineNo, text)
				if prev != nil {
					row = p.padCell(row, len(prev.text))
				}
				row = append(row, cur.data()...)
				prev = &cur
				filled++
			}
			if filled > 0 {
				if rows == 0 && !o.noHead {
					if err := p.header(page, src.disp, src.date); err != nil {
						return err
					}
				}
				if err := p.writeLine(row); err != nil {
					return err
				}
				rows++
			}
			if done {
				break
			}
		}
		if rows > 0 {
			if err := p.trailer(o.bodyLines - rows); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
		page++
	}
}

// printMerged prints the operands side by side, one line of each per row.
func (p *printer) printMerged(ctx context.Context, operands []string) error {
	o := &p.opts
	var files []*source
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		src, err := p.openSource(ctx, name)
		if err != nil {
			p.diagnose(srcLabel(name), err)
			continue
		}
		files = append(files, src)
	}
	defer func() {
		for _, f := range files {
			f.rc.Close()
		}
	}()
	if len(files) == 0 {
		return nil
	}

	// The merge layout puts one line number before all the columns.
	o.cols = len(files)
	if o.number {
		o.colWidth = (o.width - o.cols*sepLen - o.numWidth) / o.cols
	} else {
		o.colWidth = (o.width + sepLen - o.cols*sepLen) / o.cols
	}
	if o.colWidth < 1 {
		p.prog.errorf("page width too narrow")
		p.err = true
		return nil
	}

	alive := 0
	for _, f := range files {
		ok, err := f.skipLines(ctx, (o.page-1)*o.bodyLines)
		if err != nil {
			return err
		}
		f.alive = ok
		if ok {
			alive++
		}
	}

	date := ""
	if !o.noHead {
		date = p.prog.now().Format(dateLayout)
	}
	disp := ""
	if o.hasHead {
		disp = o.header
	}

	page, lineNo := o.page, 0
	for alive > 0 {
		rows := 0
		for rows < o.bodyLines {
			row := appendSpaces(nil, o.offset)
			if o.number {
				lineNo++
				row = append(row, p.numField(lineNo)...)
			}
			end := o.offset
			fproc := 0
			for _, f := range files {
				cnt := 0
				if f.alive {
					text, ok, err := f.next(ctx, o)
					if err != nil {
						return err
					}
					if ok {
						row = append(row, text...)
						end = len(row)
						cnt = len(text)
						fproc++
					} else {
						f.alive = false
						alive--
					}
				}
				if fproc >= alive {
					break
				}
				row = p.padCell(row, cnt)
			}
			if end <= o.offset {
				break
			}
			if rows == 0 && !o.noHead {
				if err := p.header(page, disp, date); err != nil {
					return err
				}
			}
			if err := p.writeLine(row[:end]); err != nil {
				return err
			}
			rows++
			if alive == 0 {
				break
			}
		}
		if rows > 0 {
			if err := p.trailer(o.bodyLines - rows); err != nil {
				return err
			}
		}
		page++
	}
	return nil
}

// cell is a text column cell: an optional line number and the content.
type cell struct {
	num  []byte
	text []byte
}

// cellOf builds a cell for the given content, numbering it when -n is on.
func (p *printer) cellOf(lineNo int, text []byte) cell {
	c := cell{text: text}
	if p.opts.number {
		c.num = p.numField(lineNo)
	}
	return c
}

// data returns the cell contents: the number field followed by the text.
func (c cell) data() []byte {
	return append(c.num, c.text...)
}

// numPrefix returns the line number field of a single-column line.
func (p *printer) numPrefix(lineNo int) []byte {
	if !p.opts.number {
		return nil
	}
	return p.numField(lineNo)
}

// numField renders the line number right justified in the -n width, keeping
// the low-order digits when the number is wider, followed by the separator
// character.
func (p *printer) numField(n int) []byte {
	s := strconv.Itoa(n)
	if w := p.opts.numWidth; len(s) > w {
		s = s[len(s)-w:]
	} else {
		s = strings.Repeat(" ", w-len(s)) + s
	}
	return append([]byte(s), p.opts.numChar)
}

// padCell appends the padding that follows a cell of the given content
// width: the separator character when -s is in use, blanks up to the next
// cell otherwise.
func (p *printer) padCell(line []byte, cnt int) []byte {
	if p.opts.hasSep {
		return append(line, p.opts.sep)
	}
	return appendSpaces(line, p.opts.pitch()-cnt)
}

// header writes the five-line page header.
func (p *printer) header(page int, disp, date string) error {
	if err := p.writeString("\n\n"); err != nil {
		return err
	}
	if p.opts.offset > 0 {
		if err := p.writeOffset(p.opts.offset); err != nil {
			return err
		}
	}
	return p.writeString(fmt.Sprintf("%s %s Page %d\n\n\n", date, disp, page))
}

// trailer fills the page to its length with newlines, or a form feed with
// -f, and adds the five-line trailer, unless suppressed.
func (p *printer) trailer(missing int) error {
	o := &p.opts
	if o.noHead {
		// Without a header there's no padding, but the form feed
		// that terminates each page is still written.
		if o.formfeed {
			return p.writeByte('\f')
		}
		return nil
	}
	if o.formfeed {
		return p.writeByte('\f')
	}
	n := missing
	if o.dspace {
		n *= 2
	}
	if o.addOne {
		n++
	}
	n += tailLen
	for ; n > 0; n-- {
		if err := p.writeByte('\n'); err != nil {
			return err
		}
	}
	return nil
}

// writeLine writes one output line and its terminator, double-spaced when
// requested.
func (p *printer) writeLine(line []byte) error {
	var err error
	if p.opts.contract {
		err = p.writeContracted(line, false)
	} else if len(line) > 0 {
		err = p.writeBytes(line)
	}
	if err != nil {
		return err
	}
	if err := p.writeByte('\n'); err != nil {
		return err
	}
	if p.opts.dspace {
		return p.writeByte('\n')
	}
	return nil
}

// writeOffset writes the left margin of a header line.
func (p *printer) writeOffset(offset int) error {
	if p.opts.contract {
		return p.writeContracted(spaces(offset), true)
	}
	return p.writeBytes(spaces(offset))
}

// writeContracted writes the line replacing runs of blanks that reach a tab
// stop by the output tab character. Trailing blanks are dropped unless flush
// is set.
func (p *printer) writeContracted(line []byte, flush bool) error {
	gap := p.opts.outGap
	ips, ops := 0, 0
	fill := func() error {
		// Tab only where it doesn't overshoot and saves a character.
		for ips-ops > 1 {
			next := ops + gap - ops%gap
			if next > ips {
				break
			}
			if err := p.writeByte(p.opts.outChar); err != nil {
				return err
			}
			ops = next
		}
		for ; ops < ips; ops++ {
			if err := p.writeByte(' '); err != nil {
				return err
			}
		}
		return nil
	}
	for _, c := range line {
		switch c {
		case ' ':
			ips++
		case p.opts.outChar:
			ips += gap - ips%gap
		default:
			if err := fill(); err != nil {
				return err
			}
			if err := p.writeByte(c); err != nil {
				return err
			}
			ips++
			ops++
		}
	}
	if flush {
		return fill()
	}
	return nil
}

func (p *printer) writeBytes(b []byte) error {
	if _, err := p.w.Write(b); err != nil {
		return &writeError{err: err}
	}
	return nil
}

func (p *printer) writeByte(c byte) error {
	if err := p.w.WriteByte(c); err != nil {
		return &writeError{err: err}
	}
	return nil
}

func (p *printer) writeString(s string) error {
	if _, err := p.w.WriteString(s); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// appendSpaces appends n blanks to the line.
func appendSpaces(line []byte, n int) []byte {
	for ; n > 0; n-- {
		line = append(line, ' ')
	}
	return line
}

// spaces returns n blanks.
func spaces(n int) []byte {
	return appendSpaces(nil, n)
}
