package pr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/strftime"
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
		stdopts.Errorf(p.prog.Runtime, "pr", "%s: %v", label, patherr.Unwrap(err))
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
		f, err := p.prog.FS.Open(p.prog.FsName(name))
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
		var t time.Time
		switch {
		case p.opts.merged || stdin:
			t = p.prog.now()
		default:
			fi, err := src.file.Stat()
			if err != nil {
				src.rc.Close()
				return nil, err
			}
			t = fi.ModTime().In(p.prog.location())
		}
		src.date = strftime.Format(p.opts.dateFmt, t)
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
	label    string // operand name, for diagnostics
	disp     string // header field
	date     string // header date
	file     fs.File
	rc       io.ReadCloser
	br       *bufio.Reader
	alive    bool // merge: the file still has data
	held     bool // merge: a form feed holds the file until the next page
	fullPage bool // the last page read filled the body exactly
}

// line is one input line, terminated by a newline or by a form feed.
type line struct {
	text []byte
	ff   bool // terminated by a form feed: a page break follows the line
}

// next returns the next input line, processed for the output: tabs expanded,
// nonprinting characters converted, and the text truncated to the column
// width. ok is false at the end of the file.
//
// A line terminated by a form feed, including an empty one, carries ff,
// meaning the current page ends after it. With -T, form feeds are ignored
// instead. A form feed which follows a page the input filled exactly, as the
// one right after a full page, is dropped.
func (s *source) next(ctx context.Context, o *options) (line, bool, error) {
	if err := ctx.Err(); err != nil {
		return line{}, false, err
	}
	if s.fullPage {
		s.fullPage = false
		s.dropFF()
	}
	var text []byte
	for {
		c, err := s.br.ReadByte()
		if err == io.EOF {
			if len(text) == 0 {
				return line{}, false, nil
			}
			return line{text: processLine(text, o)}, true, nil
		}
		if err != nil {
			return line{}, false, err
		}
		switch c {
		case '\n':
			return line{text: processLine(text, o)}, true, nil
		case '\f':
			// A newline right after a form feed belongs to it.
			if b, err := s.br.Peek(1); err == nil && b[0] == '\n' {
				s.br.ReadByte()
			}
			if o.omitPag {
				continue
			}
			return line{text: processLine(text, o), ff: true}, true, nil
		default:
			text = append(text, c)
		}
	}
}

// dropFF discards a form feed which follows a full page, and the newline
// that may follow it.
func (s *source) dropFF() {
	b, err := s.br.Peek(1)
	if err != nil || b[0] != '\f' {
		return
	}
	s.br.ReadByte()
	if b, err := s.br.Peek(1); err == nil && b[0] == '\n' {
		s.br.ReadByte()
	}
}

// skipPages discards n pages of the source, where a page holds lines lines
// and ends early at a form feed. It returns the number of lines read, for
// the line numbering, and false when the source ended first.
func (s *source) skipPages(ctx context.Context, o *options, n, lines int) (int, bool, error) {
	read := 0
	for ; n > 0; n-- {
		full := true
		for range lines {
			ln, ok, err := s.next(ctx, o)
			if err != nil {
				return 0, false, err
			}
			if !ok {
				return read, false, nil
			}
			// A lone form feed doesn't count as a line.
			if len(ln.text) > 0 || !ln.ff {
				read++
			}
			if ln.ff {
				full = false
				break
			}
		}
		s.fullPage = full
	}
	return read, true, nil
}

// processLine expands the input tab characters, converts the nonprinting
// characters of -c and -v, and truncates the line to the column width.
func processLine(text []byte, o *options) []byte {
	width := o.textBudget
	out := make([]byte, 0, len(text)+defGap)
	col := 0
	for _, c := range text {
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
		clump, w := showChar(c, o)
		if width > 0 && col+w > width {
			break
		}
		out = append(out, clump...)
		col += w
	}
	return out
}

// showChar returns the output form of one input character and its width in
// columns, honoring -c and -v. A TAB is kept as is; it's expanded by
// processLine when the option requires it.
func showChar(c byte, o *options) ([]byte, int) {
	if c == '\t' || (c >= 0x20 && c < 0x7f) {
		return []byte{c}, 1
	}
	if o.showEsc {
		return []byte(fmt.Sprintf("\\%03o", c)), 4
	}
	if o.showCtrl {
		if c < 0x80 {
			return []byte{'^', c ^ 0x40}, 2
		}
		return []byte(fmt.Sprintf("\\%03o", c)), 4
	}
	return []byte{c}, 1
}

// singleFile prints one file in the default single-column mode.
func (p *printer) singleFile(ctx context.Context, src *source) error {
	o := &p.opts
	skipped, ok, err := src.skipPages(ctx, o, o.page-1, o.bodyLines)
	if err != nil || !ok {
		return err
	}
	page, lineNo := o.page, o.startLineNo(skipped)
	for {
		count, done, broke, headed := 0, false, false, false
		for count < o.bodyLines {
			ln, ok, err := src.next(ctx, o)
			if err != nil {
				return err
			}
			if !ok {
				done = true
				break
			}
			if len(ln.text) == 0 && ln.ff {
				// A lone form feed ends the page, which still carries
				// one blank line and the header.
				if count == 0 && !o.noHead {
					if err := p.header(page, src.disp, src.date); err != nil {
						return err
					}
					if err := p.writeLine(nil); err != nil {
						return err
					}
					count, headed = 1, true
				}
				broke = true
				break
			}
			if count == 0 && !headed && !o.noHead {
				if err := p.header(page, src.disp, src.date); err != nil {
					return err
				}
				headed = true
			}
			lineNo++
			row := appendSpaces(nil, o.offset)
			row = append(row, p.numField(lineNo)...)
			if err := p.writeLine(append(row, ln.text...)); err != nil {
				return err
			}
			count++
			if ln.ff {
				broke = true
				break
			}
		}
		// A form feed which follows a page the input filled exactly is
		// dropped, but not one which ended the page itself.
		if count == o.bodyLines && !broke {
			src.fullPage = true
		}
		if count > 0 || broke {
			if err := p.trailer(o.bodyLines - count); err != nil {
				return err
			}
		}
		if broke && o.keepFF() {
			// -t keeps the form feed in place of the page break.
			if err := p.writeByte('\f'); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
		if o.lastPage != 0 && page >= o.lastPage {
			return nil
		}
		page++
	}
}

// columnsFile prints one file in multiple columns filled down the page.
func (p *printer) columnsFile(ctx context.Context, src *source) error {
	o := &p.opts
	capacity := o.bodyLines * o.cols
	skipped, ok, err := src.skipPages(ctx, o, o.page-1, capacity)
	if err != nil || !ok {
		return err
	}
	page, lineNo := o.page, o.startLineNo(skipped)
	for {
		var cells []cell
		done, broke := false, false
		for len(cells) < capacity {
			ln, ok, err := src.next(ctx, o)
			if err != nil {
				return err
			}
			if !ok {
				done = true
				break
			}
			if len(ln.text) == 0 && ln.ff {
				broke = true
				break
			}
			lineNo++
			cells = append(cells, p.cellOf(lineNo, ln.text))
			if ln.ff {
				broke = true
				break
			}
		}
		if len(cells) == capacity && !broke {
			src.fullPage = true
		}
		if len(cells) == 0 && !broke {
			return nil
		}
		// The stored lines are balanced into the minimum number of
		// lines, as GNU pr does with the columns it fills down.
		rows := (len(cells) + o.cols - 1) / o.cols
		if len(cells) == 0 {
			// An empty page still carries one blank line.
			if broke && !o.noHead {
				rows = 1
			} else {
				rows = 0
			}
		}
		if rows > o.bodyLines {
			rows = o.bodyLines
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
				// The separator is written only when the next cell of
				// the row holds a line, even an empty one.
				if c < o.cols-1 && (c+1)*rows+r < len(cells) {
					row = p.padCell(row, len(cells[i].data()))
				}
			}
			if err := p.writeLine(row); err != nil {
				return err
			}
		}
		if err := p.trailer(o.bodyLines - rows); err != nil {
			return err
		}
		if broke && o.keepFF() {
			if err := p.writeByte('\f'); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
		if o.lastPage != 0 && page >= o.lastPage {
			return nil
		}
		page++
	}
}

// acrossFile prints one file in multiple columns filled across the page.
func (p *printer) acrossFile(ctx context.Context, src *source) error {
	o := &p.opts
	skipped, ok, err := src.skipPages(ctx, o, o.page-1, o.bodyLines*o.cols)
	if err != nil || !ok {
		return err
	}
	page, lineNo := o.page, o.startLineNo(skipped)
	for {
		rows, done, broke, headed := 0, false, false, false
		for rows < o.bodyLines {
			row := appendSpaces(nil, o.offset)
			var prev *cell
			filled := 0
			for c := 0; c < o.cols; c++ {
				ln, ok, err := src.next(ctx, o)
				if err != nil {
					return err
				}
				if !ok {
					done = true
					break
				}
				if len(ln.text) == 0 && ln.ff {
					if rows == 0 && !o.noHead {
						// An empty page still carries the header and
						// one blank line.
						if err := p.header(page, src.disp, src.date); err != nil {
							return err
						}
						if err := p.writeLine(nil); err != nil {
							return err
						}
						rows, headed = 1, true
					}
					broke = true
					break
				}
				lineNo++
				cur := p.cellOf(lineNo, ln.text)
				if prev != nil {
					row = p.padCell(row, len(prev.data()))
				}
				row = append(row, cur.data()...)
				prev = &cur
				filled++
				if ln.ff {
					broke = true
					break
				}
			}
			if filled > 0 {
				if rows == 0 && !headed && !o.noHead {
					if err := p.header(page, src.disp, src.date); err != nil {
						return err
					}
					headed = true
				}
				if err := p.writeLine(row); err != nil {
					return err
				}
				rows++
			}
			if done || broke {
				break
			}
		}
		if rows == o.bodyLines && !broke {
			src.fullPage = true
		}
		if rows > 0 || broke {
			if err := p.trailer(o.bodyLines - rows); err != nil {
				return err
			}
		}
		if broke && o.keepFF() {
			if err := p.writeByte('\f'); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
		if o.lastPage != 0 && page >= o.lastPage {
			return nil
		}
		page++
	}
}

// printMerged prints the operands side by side, one line of each per row.
// A form feed in one of the files empties its column for the rest of the
// page, which still shows its padding and separator.
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
	if err := o.setupColumns(len(files)); err != nil {
		stdopts.Errorf(p.prog.Runtime, "pr", "%v", err)
		p.err = true
		return nil
	}

	for _, f := range files {
		_, ok, err := f.skipPages(ctx, o, o.page-1, o.bodyLines)
		if err != nil {
			return err
		}
		f.alive = ok
	}

	date := ""
	if !o.noHead {
		date = strftime.Format(o.dateFmt, p.prog.now())
	}
	disp := ""
	if o.hasHead {
		disp = o.header
	}

	page, lineNo := o.page, o.startLineNo((o.page-1)*o.bodyLines)
	for {
		rows, ffSeen := 0, false
		for rows < o.bodyLines && p.activeFiles(files) > 0 {
			texts := make([][]byte, len(files))
			read := make([]bool, len(files))
			for i, f := range files {
				if !f.alive || f.held {
					continue
				}
				ln, ok, err := f.next(ctx, o)
				if err != nil {
					return err
				}
				if !ok {
					f.alive = false
					continue
				}
				read[i] = true
				if len(ln.text) == 0 && ln.ff {
					f.held, ffSeen = true, true
					continue
				}
				texts[i] = ln.text
				if ln.ff {
					f.held, ffSeen = true, true
				}
			}

			last := -1
			for i, t := range texts {
				if len(t) > 0 {
					last = i
				}
			}
			if !readAnything(read) {
				// No file printed a line at all: the page is over.
				break
			}
			row := appendSpaces(nil, o.offset)
			if last >= 0 && o.number {
				lineNo++
				row = append(row, p.numField(lineNo)...)
			}
			// Every column is rendered, with its padding and separator,
			// even when its file has no line for the row; the trailing
			// blanks end at the last separator, as GNU pr keeps them.
			for i, t := range texts {
				row = append(row, t...)
				if i < len(texts)-1 {
					row = p.padCell(row, len(t))
				}
			}
			if rows == 0 && !o.noHead {
				if err := p.header(page, disp, date); err != nil {
					return err
				}
			}
			if err := p.writeRow(row, false); err != nil {
				return err
			}
			rows++
		}
		if rows == o.bodyLines {
			for _, f := range files {
				if f.alive && !f.held {
					f.fullPage = true
				}
			}
		}
		if rows > 0 {
			if err := p.trailer(o.bodyLines - rows); err != nil {
				return err
			}
		}
		if ffSeen && o.keepFF() {
			if err := p.writeByte('\f'); err != nil {
				return err
			}
		}
		// A held file rejoins the lineup in the next page.
		for _, f := range files {
			f.held = false
		}
		if p.activeFiles(files) == 0 {
			return nil
		}
		if o.lastPage != 0 && page >= o.lastPage {
			return nil
		}
		page++
	}
}

// activeFiles returns the number of merged files which still have a line to
// print in the current page.
func (p *printer) activeFiles(files []*source) int {
	n := 0
	for _, f := range files {
		if f.alive && !f.held {
			n++
		}
	}
	return n
}

// readAnything reports whether any of the merged files printed a line.
func readAnything(read []bool) bool {
	for _, ok := range read {
		if ok {
			return true
		}
	}
	return false
}

// setupColumns settles the number of merged columns and their width for the
// files which could be opened.
func (o *options) setupColumns(n int) error {
	o.cols = n
	if !o.truncate {
		return nil
	}
	used := 0
	if o.number {
		// The number of a merged line stands outside the columns.
		used = o.numFieldW
	}
	o.colWidth = (o.width - used - (n-1)*o.sepLen) / n
	if o.colWidth < 1 {
		return errors.New("page width too narrow")
	}
	o.textBudget = o.colWidth
	o.cuts = o.cuts[:0]
	o.cuts = append(o.cuts, o.offset)
	for k := 1; k < n; k++ {
		o.cuts = append(o.cuts, o.offset+used+k*(o.colWidth+o.sepLen))
	}
	return nil
}

// startLineNo returns the value the line counter starts at, so that the
// first printed line carries the -N number, or the number of the lines of
// the skipped pages plus one, as GNU pr counts them too.
func (o *options) startLineNo(skipped int) int {
	if o.numSet {
		return o.firstNum - 1
	}
	return skipped
}

// keepFF reports whether the input form feeds are kept in the output, as -t
// does, but -T doesn't.
func (o *options) keepFF() bool {
	return o.noHead && !o.omitPag
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

// numField renders the line number right justified in the -n width, keeping
// the low-order digits when the number is wider, followed by the separator
// character.
func (p *printer) numField(n int) []byte {
	if !p.opts.number {
		return nil
	}
	s := strconv.Itoa(n)
	if w := p.opts.numWidth; len(s) > w {
		s = s[len(s)-w:]
	} else {
		s = strings.Repeat(" ", w-len(s)) + s
	}
	return append([]byte(s), p.opts.numChar)
}

// padCell appends the padding and the separator that follow a cell of the
// given width: blanks up to the column width and then the separator, or just
// the separator when the lines are joined.
func (p *printer) padCell(row []byte, cnt int) []byte {
	o := &p.opts
	if o.join || o.colWidth == 0 {
		return append(row, o.sep...)
	}
	row = appendSpaces(row, o.colWidth-cnt)
	return append(row, o.sep...)
}

// header writes the five-line page header: two blank lines, the date at the
// left, the centered file name and the page number at the right, padded to
// the page width, and two more blank lines.
func (p *printer) header(page int, disp, date string) error {
	o := &p.opts
	// The left margin ends the first blank line, as GNU pr writes it.
	if err := p.writeOffset(o.offset); err != nil {
		return err
	}
	if err := p.writeString("\n\n"); err != nil {
		return err
	}
	row := appendSpaces(nil, o.offset)
	row = append(row, p.headerText(page, disp, date)...)
	if err := p.writeBytes(row); err != nil {
		return err
	}
	return p.writeString("\n\n\n")
}

// headerText builds the text line of the header.
func (p *printer) headerText(page int, disp, date string) []byte {
	pageStr := fmt.Sprintf("Page %d", page)
	avail := p.opts.width - len(date) - len(disp) - len(pageStr)
	lhs, rhs := 1, 1
	if avail > 2 {
		// The parts keep at least one blank between them.
		lhs, rhs = avail/2, avail-avail/2
	}
	out := make([]byte, 0, p.opts.width)
	out = append(out, date...)
	out = appendSpaces(out, lhs)
	out = append(out, disp...)
	out = appendSpaces(out, rhs)
	return append(out, pageStr...)
}

// trailer fills the page to its length with newlines, or a form feed with
// -f, and adds the five-line trailer, unless suppressed.
func (p *printer) trailer(missing int) error {
	o := &p.opts
	if o.noHead {
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
	return p.writeRow(line, false)
}

// writeRow writes one output line and its terminator, double-spaced when
// requested. flush keeps the trailing blanks of the line, as the ones of a
// cut off column separator.
func (p *printer) writeRow(line []byte, flush bool) error {
	var err error
	if p.opts.contract {
		err = p.writeContracted(line, flush)
	} else if len(line) > 0 || flush {
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

var debugRuns bool

// writeContracted writes the line replacing runs of blanks that reach a tab
// stop by the output tab character. Trailing blanks are dropped unless flush
// is set. A run which crosses the end of a column separator is split there,
// as GNU pr flushes its pending blanks at each separator.
func (p *printer) writeContracted(line []byte, flush bool) error {
	gap := p.opts.outGap
	ips, ops := 0, 0
	contract := func(end int) error {
		// Tab only where it doesn't overshoot and saves a character.
		for end-ops > 1 {
			next := ops + gap - ops%gap
			if next > end {
				break
			}
			if err := p.writeByte(p.opts.outChar); err != nil {
				return err
			}
			ops = next
		}
		for ; ops < end; ops++ {
			if err := p.writeByte(' '); err != nil {
				return err
			}
		}
		return nil
	}
	fill := func() error {
		if debugRuns {
			fmt.Printf("DBG run [%d,%d) cuts=%v\n", ops, ips, p.opts.cuts)
		}
		start := ops
		for _, cut := range p.opts.cuts {
			if cut > start && cut < ips {
				if err := contract(cut); err != nil {
					return err
				}
				start = cut
			}
		}
		return contract(ips)
	}
	for _, c := range line {
		switch c {
		case ' ':
			ips++
		case p.opts.outChar:
			// A TAB is written as is, counting one column, as the
			// separator tabs of GNU pr do, after the pending blanks.
			if err := fill(); err != nil {
				return err
			}
			if err := p.writeByte(c); err != nil {
				return err
			}
			ips++
			ops++
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
	// The blanks which GNU pr flushes at the column separators are kept,
	// up to the last one; only the padding of the last column is dropped.
	keep := 0
	for _, cut := range p.opts.cuts {
		if cut > keep && cut <= ips {
			keep = cut
		}
	}
	if keep > ops {
		ips = keep
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
