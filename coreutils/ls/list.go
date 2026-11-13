package ls

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/turtle/coreutils/internal/fileinfo"
	"charm.land/turtle/coreutils/internal/filemode"
	"charm.land/turtle/fsx"
)

// list writes a listing of entries, preceded by the total number of blocks
// when the options ask for one and this is a listing of a directory.
func (l *lister) list(ctx context.Context, entries []entry, dir bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dir && (l.opts.long || l.opts.blocks) {
		fmt.Fprintf(l.prog.Stdout, "total %s%s", l.blocksTotal(entries), l.eol)
		l.printed = true
	}
	if len(entries) == 0 {
		return nil
	}

	lines := make([]line, len(entries))
	for i, e := range entries {
		lines[i] = l.lineOf(e)
	}
	fields := longFields{}
	for _, ln := range lines {
		fields.inode = max(fields.inode, len(ln.inode))
		fields.blocks = max(fields.blocks, len(ln.blocks))
		if l.opts.long {
			fields.nlink = max(fields.nlink, len(ln.nlink))
			fields.owner = max(fields.owner, len(ln.owner))
			fields.group = max(fields.group, len(ln.group))
			fields.author = max(fields.author, len(ln.author))
			fields.size = max(fields.size, len(ln.size))
		}
	}
	if l.alignQuotes() {
		someQuoted := false
		for i := range lines {
			if lines[i].quoted {
				someQuoted = true
				break
			}
		}
		if someQuoted {
			for i := range lines {
				if !lines[i].quoted {
					lines[i].pad = true
				}
			}
		}
	}

	if l.opts.long {
		for _, ln := range lines {
			fmt.Fprintf(l.prog.Stdout, "%s%s", ln.longString(fields, l.opts), l.eol)
		}
	} else {
		names := make([]string, len(lines))
		for i, ln := range lines {
			names[i] = ln.shortString(fields, l.opts)
		}
		switch l.opts.layout {
		case layoutColumns:
			l.writeColumns(names, true)
		case layoutAcross:
			l.writeColumns(names, false)
		case layoutStream:
			l.writeSeparated(names, ',')
		default:
			for _, name := range names {
				fmt.Fprintf(l.prog.Stdout, "%s%s", name, l.eol)
			}
		}
	}
	l.printed = true
	return nil
}

// alignQuotes reports whether the unquoted names of a listing are padded
// with a space when others carry the quotes a shell style would have left
// out, which the long format and the column ones within a width do.
func (l *lister) alignQuotes() bool {
	if l.opts.quoting != quoteShell && l.opts.quoting != quoteShellEscape && l.opts.quoting != quoteCMaybe {
		return false
	}
	if l.opts.long {
		return true
	}
	return (l.opts.layout == layoutColumns || l.opts.layout == layoutAcross) && l.width > 0
}

// line holds the fields of an entry formatted for a listing, before the
// widths they are padded to are known.
type line struct {
	entry

	inode  string
	blocks string
	nlink  string
	owner  string
	group  string
	author string
	size   string
	name   string // the name as written, with its indicator and target
	quoted bool   // whether the name was quoted
	pad    bool   // whether the name is padded for the quoted ones
	time   string // the date and time of the long format
}

// longFields holds the width each field of a long listing is padded to.
type longFields struct {
	inode, blocks, nlink, owner, group, author, size int
}

// lineOf formats the fields of an entry for the options of the listing.
func (l *lister) lineOf(e entry) line {
	ln := line{entry: e}
	if l.opts.inode {
		ln.inode = strconv.FormatUint(inodeOf(e.fi), 10)
	}
	if l.opts.blocks {
		ln.blocks = l.blocksSpec.Format(int64(blocksOf(e.fi)))
	}
	ln.name, ln.quoted = l.nameOf(e)
	if !l.opts.long {
		return ln
	}
	ln.nlink = strconv.FormatUint(nlinkOf(e.fi), 10)
	if l.opts.owner {
		ln.owner = l.ownerOf(e.fi)
	}
	if l.opts.group {
		ln.group = l.groupOf(e.fi)
	}
	if l.opts.author {
		ln.author = l.ownerOf(e.fi)
	}
	ln.size = l.sizeSpec.Format(e.fi.Size())
	ln.time = l.timeString(e)
	return ln
}

// longString writes the line of an entry in the long format: the fields
// right-justified to the widths of the listing, except the names of the
// owner and the group, which are left-justified, and the name of the
// entry, which isn't padded.
func (ln line) longString(fields longFields, opts options) string {
	var b strings.Builder
	if opts.inode {
		b.WriteString(padLeft(ln.inode, fields.inode) + " ")
	}
	if opts.blocks {
		b.WriteString(padLeft(ln.blocks, fields.blocks) + " ")
	}
	b.WriteString(filemode.String(ln.fi.Mode()) + " ")
	b.WriteString(padLeft(ln.nlink, fields.nlink) + " ")
	if opts.owner {
		b.WriteString(userField(ln.owner, fields.owner, opts.numeric) + " ")
	}
	if opts.group {
		b.WriteString(userField(ln.group, fields.group, opts.numeric) + " ")
	}
	if opts.author {
		b.WriteString(userField(ln.author, fields.author, opts.numeric) + " ")
	}
	b.WriteString(padLeft(ln.size, fields.size) + " ")
	b.WriteString(ln.time + " ")
	if ln.pad {
		b.WriteString(" ")
	}
	b.WriteString(ln.name)
	return b.String()
}

// shortString writes the line of an entry in a format that isn't long: the
// file serial number and the block count the options ask for,
// right-justified, then the name. The stream format, which fills a line,
// writes them without the padding.
func (ln line) shortString(fields longFields, opts options) string {
	inodeWidth, blocksWidth := fields.inode, fields.blocks
	if opts.layout == layoutStream {
		inodeWidth, blocksWidth = 0, 0
	}
	var b strings.Builder
	if opts.inode {
		b.WriteString(padLeft(ln.inode, inodeWidth) + " ")
	}
	if opts.blocks {
		b.WriteString(padLeft(ln.blocks, blocksWidth) + " ")
	}
	if ln.pad {
		b.WriteString(" ")
	}
	b.WriteString(ln.name)
	return b.String()
}

// userField writes the name of the owner or of a group: left-justified,
// or right-justified when it's an identifier written as a number.
func userField(name string, width int, numeric bool) string {
	if numeric {
		return padLeft(name, width)
	}
	return padRight(name, width)
}

// nameOf returns the name of an entry, quoted the way the options ask,
// with the file type indicator they ask for and, in the long format, the
// contents of a symbolic link written as itself. It reports whether the
// name was quoted.
func (l *lister) nameOf(e entry) (string, bool) {
	extra := l.nameExtra()
	name, quoted := quoteName(e.name, l.opts.quoting, l.hideControl, extra)
	if e.link == nil || e.followed {
		return name + indicator(e.fi, l.opts.classify), quoted
	}
	if l.opts.classify >= classifyFile {
		name += "@"
	}
	if !l.opts.long {
		return name, quoted
	}
	target, _ := quoteName(e.target, l.opts.quoting, l.hideControl, extra)
	return name + " -> " + target, quoted
}

// nameExtra returns the characters quoted in a name in addition to the
// ones the quoting style quotes: the space the escape style escapes, and
// the file type indicators, so a name holding one isn't taken for a name
// with an indicator appended.
func (l *lister) nameExtra() string {
	extra := ""
	if l.opts.quoting == quoteEscape {
		extra += " "
	}
	switch l.opts.classify {
	case classifyFile:
		extra += "*=>@|"
	case classifyFull:
		extra += "=>@|"
	}
	return extra
}

// writeColumns writes names in columns, sorted down the columns or
// across them, as many as the width fits and each as wide as its longest
// name, indented with tabs where they fit.
func (l *lister) writeColumns(names []string, byColumns bool) {
	if l.width <= 0 {
		// With no width to write within, the entries share one line.
		l.writeSeparated(names, ' ')
		return
	}
	cols := l.columnWidths(names, byColumns)
	rows := (len(names) + len(cols) - 1) / len(cols)
	if byColumns {
		for row := range rows {
			filesno, column, pos := row, 0, 0
			for {
				name := names[filesno]
				fmt.Fprint(l.prog.Stdout, name)
				width := textWidth(name)
				if len(names)-rows <= filesno {
					break
				}
				columnWidth := cols[column]
				filesno += rows
				column++
				pos = l.indent(pos+width, pos+columnWidth)
			}
			fmt.Fprint(l.prog.Stdout, l.eol)
		}
		return
	}
	pos := 0
	for filesno, name := range names {
		if filesno > 0 {
			if filesno%len(cols) == 0 {
				fmt.Fprint(l.prog.Stdout, l.eol)
				pos = 0
			} else {
				prev := (filesno - 1) % len(cols)
				pos = l.indent(pos+textWidth(names[filesno-1]), pos+cols[prev])
			}
		}
		fmt.Fprint(l.prog.Stdout, name)
	}
	fmt.Fprint(l.prog.Stdout, l.eol)
}

// boolInt gives a boolean as the zero or one an arithmetic needs.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// columnWidths returns the widths of the columns a listing fits in the
// width it's written within: the width of the longest name of each column,
// with the two columns the gap between them takes added to all but the
// last one. The widest layout that fits is the one written.
func (l *lister) columnWidths(names []string, byColumns bool) []int {
	type layout struct {
		widths []int
		total  int
		valid  bool
	}
	maxCols := len(names)
	if idx := l.width/2 + boolInt(l.width%2 != 0); idx > 0 && idx < maxCols {
		maxCols = idx
	}
	layouts := make([]layout, maxCols)
	for i := range layouts {
		layouts[i] = layout{widths: make([]int, i+1), valid: true}
	}
	for filesno, name := range names {
		width := textWidth(name)
		for i := range layouts {
			lo := &layouts[i]
			if !lo.valid {
				continue
			}
			column := filesno % (i + 1)
			if byColumns {
				column = filesno / ((len(names) + i) / (i + 1))
			}
			real := width
			if column != i {
				real += 2
			}
			if lo.widths[column] < real {
				lo.total += real - lo.widths[column]
				lo.widths[column] = real
				lo.valid = lo.total < l.width
			}
		}
	}
	for cols := maxCols; cols > 1; cols-- {
		if layouts[cols-1].valid {
			return layouts[cols-1].widths
		}
	}
	return layouts[0].widths
}

// writeSeparated writes names on lines filled with them, separated by the
// given character and a space, wrapping to a new line when the next name
// wouldn't fit the width. No width means the line has no limit.
func (l *lister) writeSeparated(names []string, sep byte) {
	pos := 0
	for filesno, name := range names {
		width := 0
		if l.width > 0 {
			width = textWidth(name)
		}
		if filesno > 0 {
			separator := byte(' ')
			if l.width > 0 && pos+width+2 >= l.width {
				separator = l.eol[0]
				pos = 0
			} else {
				pos += 2
			}
			fmt.Fprintf(l.prog.Stdout, "%c%c", sep, separator)
		}
		fmt.Fprint(l.prog.Stdout, name)
		pos += width
	}
	fmt.Fprint(l.prog.Stdout, l.eol)
}

// indent writes the padding from one column to another: a tab whenever
// it spans a tab stop, and the spaces the rest of the way. It returns the
// column reached.
func (l *lister) indent(from, to int) int {
	var b strings.Builder
	for from < to {
		if l.tabsize > 0 && to/l.tabsize > (from+1)/l.tabsize {
			b.WriteByte('\t')
			from += l.tabsize - from%l.tabsize
		} else {
			b.WriteByte(' ')
			from++
		}
	}
	fmt.Fprint(l.prog.Stdout, b.String())
	return to
}

// indicator returns the file type indicator the given style asks for.
func indicator(fi fs.FileInfo, kind classify) string {
	if fi == nil || kind == classifyNone {
		return ""
	}
	if fi.IsDir() {
		return "/"
	}
	if kind < classifyFile {
		return ""
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return "@"
	case fi.Mode()&fs.ModeNamedPipe != 0:
		return "|"
	case fi.Mode()&fs.ModeSocket != 0:
		return "="
	case kind == classifyFull && fi.Mode()&0o111 != 0:
		return "*"
	}
	return ""
}

// textWidth returns the number of columns a name takes when written: a
// combining mark takes none, and an East Asian wide or fullwidth character
// takes two.
func textWidth(name string) int {
	width := 0
	for _, r := range name {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		case isWide(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

// isWide reports whether r takes two columns, holding for the East Asian
// wide and fullwidth characters.
func isWide(r rune) bool {
	return r >= 0x1100 && (r <= 0x115F || // Hangul Jamo
		r == 0x2329 || r == 0x232A || // angle brackets
		r >= 0x2E80 && r <= 0xA4CF && r != 0x303F || // CJK radicals through Yi
		r >= 0xAC00 && r <= 0xD7A3 || // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF || // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE6F || // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60 || // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6 || // fullwidth signs
		r >= 0x20000 && r <= 0x3FFFD) // CJK extensions
}

// timeString writes the date and time field of the long format for an
// entry, or a question mark for the birth time a file system doesn't
// report.
func (l *lister) timeString(e entry) string {
	if l.opts.time == timeBirth {
		if t, ok := l.birthOf(e); ok {
			return l.timeStyle.format(t, l.now)
		}
		return padLeft("?", l.timeWidth)
	}
	return l.timeStyle.format(l.timeOf(e), l.now)
}

// timeOf returns the time a listing writes for an entry, which -c and -u
// change. A file system that doesn't report the requested time is written
// with its modification time.
func (l *lister) timeOf(e entry) time.Time {
	switch l.opts.time {
	case timeStatus:
		if t, ok := fileinfo.Ctime(e.fi); ok {
			return t
		}
	case timeAccessed:
		if t, ok := fsx.Atime(e.fi); ok {
			return t
		}
	case timeBirth:
		if t, ok := l.birthOf(e); ok {
			return t
		}
	}
	return e.fi.ModTime()
}

// birthOf returns the birth time of an entry, when its file system reports
// one.
func (l *lister) birthOf(e entry) (time.Time, bool) {
	if e.link != nil && !e.followed {
		return fileinfo.Btime(e.link)
	}
	return fileinfo.Btime(e.fi)
}

// blocksTotal returns the number of blocks the entries of a listing use,
// as the block size spec writes it.
func (l *lister) blocksTotal(entries []entry) string {
	total := int64(0)
	for _, e := range entries {
		blocks := blocksOf(e.fi)
		if blocks > uint64(math.MaxInt64-total) {
			total = math.MaxInt64
			break
		}
		total += int64(blocks)
	}
	return l.blocksSpec.Format(total)
}

// blocksOf returns the number of file system bytes a file allocates,
// saturating at the largest count an int64 holds rather than overflowing.
// A file system that doesn't report them is measured by the size of the
// file, rounded up to whole 512-byte blocks.
func blocksOf(fi fs.FileInfo) uint64 {
	alloc := func(bytes uint64) uint64 {
		return min(bytes, 1<<63-1)
	}
	if st, ok := fileinfo.Stat(fi); ok {
		if st.Blocks > (1<<63-1)/512 {
			return alloc(^uint64(0))
		}
		return alloc(st.Blocks * 512)
	}
	size := max(fi.Size(), 0)
	return alloc(uint64((size + 511) / 512 * 512))
}

// inodeOf returns the file serial number of a file, or zero when its file
// system doesn't report one.
func inodeOf(fi fs.FileInfo) uint64 {
	st, _ := fileinfo.Stat(fi)
	return st.Ino
}

// nlinkOf returns the number of links a file has, defaulting to one when its
// file system doesn't report it.
func nlinkOf(fi fs.FileInfo) uint64 {
	st, ok := fileinfo.Stat(fi)
	if !ok || st.Nlink == 0 {
		return 1
	}
	return st.Nlink
}

// ownerOf returns the owner of a file: its name, or the identifier that
// stands for it when -n was given or the name can't be determined.
func (l *lister) ownerOf(fi fs.FileInfo) string {
	st, ok := fileinfo.Stat(fi)
	if !ok {
		return "0"
	}
	if l.opts.numeric {
		return strconv.FormatUint(st.Uid, 10)
	}
	return l.names.User(st.Uid)
}

// groupOf returns the group of a file: its name, or the identifier that
// stands for it when -n was given or the name can't be determined.
func (l *lister) groupOf(fi fs.FileInfo) string {
	st, ok := fileinfo.Stat(fi)
	if !ok {
		return "0"
	}
	if l.opts.numeric {
		return strconv.FormatUint(st.Gid, 10)
	}
	return l.names.Group(st.Gid)
}

// padLeft right-justifies a string within the given width.
func padLeft(s string, width int) string {
	for len(s) < width {
		s = " " + s
	}
	return s
}

// padRight left-justifies a string within the given width.
func padRight(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
