package grep

import (
	"fmt"
	"strconv"
	"strings"
)

// filePrinter writes the output lines of one input: the prefixes, the
// context lines and the colors the options ask for.
type filePrinter struct {
	p     *Prog
	opts  options
	eol   string // how an output line ends
	multi bool   // whether the name of the input prefixes the lines
	label string
	col   *colors

	err      error        // the first write error
	wrote    bool         // whether any line was written
	printed  int64        // number of the last line written
	pending  []futureLine // the leading context candidates
	after    int64        // trailing context lines still to be written
	numWidth int          // the width the numbers are padded to, with -T
}

// futureLine is a line held as leading context.
type futureLine struct {
	num  int64
	off  int64
	data []byte
}

// newFilePrinter returns the printer of one input.
func newFilePrinter(p *Prog, in searchInput, opts options, label string, multiple bool, col *colors) *filePrinter {
	eol := "\n"
	if opts.nullData {
		eol = "\x00"
	}
	w := &filePrinter{p: p, opts: opts, eol: eol, multi: multiple, label: label, col: col}
	if opts.initialTab {
		// The line numbers and the byte offsets are padded so the lines of
		// an input all start at the same column, like GNU grep does with
		// the size of the input.
		if size, ok := p.inputSize(in); ok {
			if opts.lineNum {
				// The line numbers start at one.
				size++
			}
			for size > 0 {
				w.numWidth++
				size /= 10
			}
		} else {
			w.numWidth = 20
		}
	}
	return w
}

// line writes one line of the input, as selected or as context, with the
// leading and trailing context the options ask for.
func (w *filePrinter) line(selected bool, num, off int64, data []byte, matches [][]int) {
	if !w.opts.context() {
		if selected {
			w.write(selected, num, off, data, matches)
		}
		return
	}
	if w.after > 0 {
		// Trailing context is being written: a selected line joins the
		// group and arms it again.
		w.pending = w.pending[:0]
		w.write(selected, num, off, data, matches)
		if selected {
			w.after = w.opts.after
		} else {
			w.after--
		}
		return
	}
	if selected {
		for _, held := range w.pending {
			if held.num > w.printed {
				w.write(false, held.num, held.off, held.data, nil)
			}
		}
		w.pending = w.pending[:0]
		w.write(true, num, off, data, matches)
		w.after = w.opts.after
		return
	}
	// A non-selected line: hold it as leading context.
	w.pending = append(w.pending, futureLine{num: num, off: off, data: data})
	if int64(len(w.pending)) > w.opts.before {
		w.pending = w.pending[1:]
	}
}

// trailing reports whether trailing context lines are still to be written.
func (w *filePrinter) trailing() bool {
	return w.opts.context() && w.after > 0
}

// match writes one matched part of a line, as the -o option asks for.
func (w *filePrinter) match(num, off int64, data []byte) {
	var b strings.Builder
	w.prefix(&b, true, num, off, len(data))
	b.WriteString(w.col.colorize(w.col.matchColor(true), string(data)))
	b.WriteString(w.eol)
	w.print(b.String())
}

// write writes one output line, with the separator between groups of
// non-adjacent lines the context options ask for.
func (w *filePrinter) write(selected bool, num, off int64, data []byte, matches [][]int) {
	if w.wrote && num != w.printed+1 {
		w.separator()
	}
	w.wrote = true
	w.printed = num
	var b strings.Builder
	w.prefix(&b, selected, num, off, len(data))
	w.content(&b, selected, data, matches)
	b.WriteString(w.eol)
	w.print(b.String())
}

// prefix writes the fields that prefix an output line: the file name, the
// line number and the byte offset, always in that order, and the tab that
// -T puts before the content.
func (w *filePrinter) prefix(b *strings.Builder, selected bool, num, off int64, len int) {
	sep := byte(':')
	if !selected {
		sep = '-'
	}
	fields := 0
	if w.multi {
		sepCh := sep
		if w.opts.nullName {
			sepCh = 0
		}
		w.field(b, w.label, 'f', sepCh)
		fields++
	}
	if w.opts.lineNum {
		w.field(b, w.numString(num), 'l', sep)
		fields++
	}
	if w.opts.byteOffset {
		w.field(b, w.numString(off), 'b', sep)
		fields++
	}
	if w.opts.initialTab && fields > 0 && len > 0 {
		b.WriteByte('\t')
	}
}

// numString returns a line number or byte offset, padded to the width the
// lines of an input are aligned on.
func (w *filePrinter) numString(n int64) string {
	s := strconv.FormatInt(n, 10)
	if pad := w.numWidth - len(s); pad > 0 {
		s = strings.Repeat(" ", pad) + s
	}
	return s
}

// field writes one prefix field, with the separator that follows it.
func (w *filePrinter) field(b *strings.Builder, text string, which, sep byte) {
	b.WriteString(w.col.colorize(w.col.fieldColor(which), text))
	b.WriteString(w.col.sepColor(string(sep)))
}

// content writes the content of an output line, surrounding the parts
// between the matches with the line colors and the matches with the match
// ones.
func (w *filePrinter) content(b *strings.Builder, selected bool, data []byte, matches [][]int) {
	lc := w.col.lineColor(selected)
	mc := w.col.matchColor(selected)
	pos := 0
	for _, loc := range matches {
		if loc[0] < pos {
			continue
		}
		if loc[0] > pos {
			b.WriteString(w.col.colorize(lc, string(data[pos:loc[0]])))
		}
		b.WriteString(w.col.colorize(mc, string(data[loc[0]:loc[1]])))
		pos = loc[1]
	}
	if pos < len(data) {
		b.WriteString(w.col.colorize(lc, string(data[pos:])))
	}
}

// separator writes the line that separates two groups of context lines.
func (w *filePrinter) separator() {
	if w.opts.groupSepNo || !w.opts.context() {
		return
	}
	b := w.col.sepColor(w.opts.groupSep) + w.eol
	w.print(b)
}

// print writes one output line, holding the first write error.
func (w *filePrinter) print(line string) {
	if w.err != nil {
		return
	}
	if _, err := fmt.Fprint(w.p.Stdout, line); err != nil {
		w.err = err
	}
}

// name writes the name of an input, as the -l and -L options ask for.
func (p *Prog) printName(name string, opts options, col *colors) {
	eol := "\n"
	if opts.nullName {
		eol = "\x00"
	}
	fmt.Fprint(p.Stdout, col.colorize(col.fieldColor('f'), name)+eol)
}
