package ls

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os/user"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/fsx"
)

// followMode is how symbolic links are followed.
type followMode int

const (
	followDefault  followMode = iota // only operand links to directories
	followOperands                   // -H
	followAlways                     // -L
)

// sortKey is what a listing is sorted by.
type sortKey int

const (
	sortName      sortKey = iota
	sortSize              // -S
	sortTime              // -t
	sortNone              // -f, -U
	sortExtension         // -X
	sortVersion           // -v
	sortWidth             // --sort=width
)

// timeKey is the time a listing is written and sorted with.
type timeKey int

const (
	timeModified timeKey = iota
	timeStatus           // -c
	timeAccessed         // -u
)

// layout is the format of a listing that isn't long.
type layout int

const (
	layoutOne     layout = iota // one entry per line, the default and -1
	layoutColumns               // -C
	layoutAcross                // -x
	layoutStream                // -m
)

// classify is the file type indicator written after a name.
type classify byte

const (
	classifyNone classify = iota // no indicator
	classifyDirs                 // -p, --indicator-style=slash: directories only
	classifyFile                 // --file-type: everything but executables
	classifyFull                 // -F, --classify: everything
)

// options are the listing options parsed from the command line.
type options struct {
	all       bool // -A/-a: write hidden entries too
	fast      bool // -f: keep the order of the directory
	inode     bool // -i: write the file serial number
	blocks    bool // -s: write the block count
	kilo      bool // -k: count blocks in 1024-byte units
	recursive bool // -R: list subdirectories
	dirs      bool // -d: write directories as they are
	reverse   bool // -r: reverse the order
	quote     bool // -q: write non-printable characters as question marks
	long      bool // -l/-g/-n/-o: long format
	owner     bool // write the owner
	group     bool // write the group
	numeric   bool // -n: write identifiers as numbers

	follow   followMode
	sort     sortKey
	time     timeKey
	layout   layout
	classify classify
}

// set turns on the option given, applying the argument of a valued long
// option.
func (o *options) set(opt rune, value string) error {
	switch opt {
	case 'A', 'a':
		o.all = true
	case 'C':
		o.layout, o.long = layoutColumns, false
	case 'F':
		o.classify = classifyFull
	case 'H':
		o.follow = followOperands
	case 'L':
		o.follow = followAlways
	case 'R':
		o.recursive, o.dirs = true, false
	case 'S':
		o.sort = sortSize
	case 'U':
		o.sort = sortNone
	case 'X':
		o.sort = sortExtension
	case 'c':
		o.time = timeStatus
	case 'd':
		o.dirs, o.recursive = true, false
	case 'f':
		o.fast, o.all = true, true
	case 'g':
		o.long, o.owner = true, false
	case 'i':
		o.inode = true
	case 'k':
		o.kilo = true
	case 'l':
		o.long = true
	case 'm':
		o.layout, o.long = layoutStream, false
	case 'n':
		o.long, o.numeric = true, true
	case 'o':
		o.long, o.group = true, false
	case 'p':
		o.classify = classifyDirs
	case 'q':
		o.quote = true
	case 'r':
		o.reverse = true
	case 's':
		o.blocks = true
	case 't':
		o.sort = sortTime
	case 'u':
		o.time = timeAccessed
	case 'v':
		o.sort = sortVersion
	case 'x':
		o.layout, o.long = layoutAcross, false
	case '1':
		// unlike --format=single-column, -1 keeps the long format
		o.layout = layoutOne
	case optFormat:
		return o.setFormat(value)
	case optFileType:
		o.classify = classifyFile
	case optIndicatorStyle:
		return o.setIndicatorStyle(value)
	case optSort:
		return o.setSort(value)
	case optTime:
		return o.setTime(value)
	default:
		return getopt.InvalidOption(opt)
	}
	return nil
}

// setFormat applies the word --format stands for: the layout it selects.
func (o *options) setFormat(word string) error {
	switch word {
	case "long", "verbose":
		o.long = true
	case "vertical":
		o.layout, o.long = layoutColumns, false
	case "across", "horizontal":
		o.layout, o.long = layoutAcross, false
	case "commas":
		o.layout, o.long = layoutStream, false
	case "single-column":
		o.layout, o.long = layoutOne, false
	default:
		return invalidWord("format", word)
	}
	return nil
}

// setIndicatorStyle applies the word --indicator-style stands for: the file
// type indicator written after a name.
func (o *options) setIndicatorStyle(word string) error {
	switch word {
	case "none":
		o.classify = classifyNone
	case "slash":
		o.classify = classifyDirs
	case "file-type":
		o.classify = classifyFile
	case "classify":
		o.classify = classifyFull
	default:
		return invalidWord("indicator-style", word)
	}
	return nil
}

// setSort applies the word --sort stands for: the key a listing is sorted by.
func (o *options) setSort(word string) error {
	switch word {
	case "name":
		o.sort = sortName
	case "time":
		o.sort = sortTime
	case "size":
		o.sort = sortSize
	case "none":
		o.sort = sortNone
	case "extension":
		o.sort = sortExtension
	case "version":
		o.sort = sortVersion
	case "width":
		o.sort = sortWidth
	default:
		return invalidWord("sort", word)
	}
	return nil
}

// setTime applies the word --time stands for: the time a listing is written
// and sorted with.
func (o *options) setTime(word string) error {
	switch word {
	case "atime", "access", "use":
		o.time = timeAccessed
	case "ctime", "status":
		o.time = timeStatus
	case "mtime", "modification":
		o.time = timeModified
	default:
		return invalidWord("time", word)
	}
	return nil
}

// invalidWord builds the error of a word a valued long option doesn't take.
func invalidWord(option, word string) error {
	return fmt.Errorf("invalid argument '%s' for '--%s'", word, option)
}

// longOptions maps the long options of ls to the short ones they stand for.
// The ones without a short equivalent map to runes above the bytes a group of
// short options can hold, keeping them out of the short syntax.
var longOptions = map[string]rune{
	"all":                      'a',
	"almost-all":               'A',
	"classify":                 'F',
	"dereference":              'L',
	"dereference-command-line": 'H',
	"directory":                'd',
	"file-type":                optFileType,
	"format":                   optFormat,
	"hide-control-chars":       'q',
	"indicator-style":          optIndicatorStyle,
	"inode":                    'i',
	"kibibytes":                'k',
	"numeric-uid-gid":          'n',
	"recursive":                'R',
	"reverse":                  'r',
	"size":                     's',
	"sort":                     optSort,
	"time":                     optTime,
}

// The long options which have no short equivalent, taking the word which
// selects what each stands for.
const (
	optFormat rune = 0x100 + iota
	optIndicatorStyle
	optSort
	optTime
	optFileType
)

// valuedLong is the Valued set of the long options which have no short
// equivalent.
const valuedLong = string(optFormat) + string(optIndicatorStyle) + string(optSort) + string(optTime)

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{owner: true, group: true}
	operands, err := getopt.Set{Valued: valuedLong, Long: longOptions}.Parse(args, func(opt rune, value string) error {
		return opts.set(opt, value)
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// list writes a listing of entries, preceded by the total number of blocks
// when the options ask for one and this is a listing of a directory.
func (l *lister) list(ctx context.Context, entries []entry, dir bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dir && (l.opts.long || l.opts.blocks) {
		fmt.Fprintf(l.prog.Stdout, "total %d\n", totalBlocks(entries, l.opts))
		l.printed = true
	}
	if len(entries) == 0 {
		return nil
	}

	if l.opts.long {
		for _, e := range entries {
			fmt.Fprintf(l.prog.Stdout, "%s\n", l.longLine(e))
		}
	} else {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = l.shortName(e)
		}
		switch l.opts.layout {
		case layoutColumns:
			writeColumns(l.prog.Stdout, names, l.width, false)
		case layoutAcross:
			writeColumns(l.prog.Stdout, names, l.width, true)
		case layoutStream:
			writeStream(l.prog.Stdout, names, l.width)
		default:
			for _, name := range names {
				fmt.Fprintf(l.prog.Stdout, "%s\n", name)
			}
		}
	}
	l.printed = true
	return nil
}

// longLine writes the fields of the long format in the order the
// specification gives them: the file serial number and the number of blocks,
// the file mode, the number of links, the owner and the group, the size, the
// date and time, and the pathname.
func (l *lister) longLine(e entry) string {
	fields := make([]string, 0, 9)
	if l.opts.inode {
		fields = append(fields, strconv.FormatUint(inodeOf(e.fi), 10))
	}
	if l.opts.blocks {
		fields = append(fields, strconv.FormatUint(blocksOf(e.fi, l.opts), 10))
	}
	fields = append(fields, modeString(e.fi.Mode()), strconv.FormatUint(nlinkOf(e.fi), 10))
	if l.opts.owner {
		fields = append(fields, l.ownerOf(e.fi))
	}
	if l.opts.group {
		fields = append(fields, l.groupOf(e.fi))
	}
	return strings.Join(append(fields,
		strconv.FormatInt(e.fi.Size(), 10),
		formatTime(l.timeOf(e), l.now),
		l.nameOf(e),
	), " ")
}

// shortName writes the name of an entry, preceded by the file serial number
// and the number of blocks the options ask for.
func (l *lister) shortName(e entry) string {
	name := l.nameOf(e)
	if l.opts.blocks {
		name = strconv.FormatUint(blocksOf(e.fi, l.opts), 10) + " " + name
	}
	if l.opts.inode {
		name = strconv.FormatUint(inodeOf(e.fi), 10) + " " + name
	}
	return name
}

// nameOf returns the name of an entry, with the file type indicator the
// options ask for and, in the long format, the contents of a symbolic link
// written as itself.
func (l *lister) nameOf(e entry) string {
	name := quoteName(e.name, l.opts.quote)
	if e.link == nil || e.followed {
		return name + indicator(e.fi, l.opts.classify)
	}
	if l.opts.classify >= classifyFile {
		name += "@"
	}
	if !l.opts.long {
		return name
	}
	return name + " -> " + quoteName(e.target, l.opts.quote)
}

// writeColumns writes names in columns of a constant width, sorted down the
// columns or across them.
func writeColumns(w io.Writer, names []string, width int, across bool) {
	longest := 0
	for _, name := range names {
		longest = max(longest, textWidth(name))
	}
	columns := min(max(width/(longest+2), 1), len(names))
	rows := (len(names) + columns - 1) / columns
	for row := range rows {
		var cells []string
		for column := range columns {
			index := column*rows + row
			if across {
				index = row*columns + column
			}
			if index < len(names) {
				cells = append(cells, names[index])
			}
		}
		line := strings.Builder{}
		for i, cell := range cells {
			line.WriteString(cell)
			if i < len(cells)-1 {
				line.WriteString(strings.Repeat(" ", longest-textWidth(cell)+2))
			}
		}
		fmt.Fprintf(w, "%s\n", line.String())
	}
}

// writeStream writes names across the width, separated by a comma and a
// space, wrapping to the next line when the next name doesn't fit.
func writeStream(w io.Writer, names []string, width int) {
	line := 0
	for i, name := range names {
		n := textWidth(name)
		if i > 0 {
			if line+2+n > width {
				fmt.Fprint(w, ",\n")
				line = 0
			} else {
				fmt.Fprint(w, ", ")
				line += 2
			}
		}
		fmt.Fprint(w, name)
		line += n
	}
	fmt.Fprint(w, "\n")
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

// modeString returns the file mode of a file, as ls writes it.
func modeString(m fs.FileMode) string {
	var b strings.Builder
	b.WriteByte(typeChar(m))
	b.WriteString(permissions(m, 0o400, 0o200, 0o100, fs.ModeSetuid, 's', 'S'))
	b.WriteString(permissions(m, 0o040, 0o020, 0o010, fs.ModeSetgid, 's', 'S'))
	sticky := fs.ModeSticky
	if m&fs.ModeDir == 0 {
		// The sticky flag is only written for directories.
		sticky = 0
	}
	b.WriteString(permissions(m, 0o004, 0o002, 0o001, sticky, 't', 'T'))
	return b.String()
}

// typeChar returns the character that describes the type of a file.
func typeChar(m fs.FileMode) byte {
	switch {
	case m&fs.ModeDir != 0:
		return 'd'
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m&fs.ModeNamedPipe != 0:
		return 'p'
	case m&fs.ModeSocket != 0:
		return 's'
	case m&fs.ModeDevice != 0 && m&fs.ModeCharDevice != 0:
		return 'c'
	case m&fs.ModeDevice != 0:
		return 'b'
	case m&fs.ModeIrregular != 0:
		return '?'
	}
	return '-'
}

// permissions returns the three permission characters of one class, with the
// flag of the class in the third position when it's set.
func permissions(m, read, write, execute, flag fs.FileMode, set, unset byte) string {
	chars := []byte{'-', '-', '-'}
	if m&read != 0 {
		chars[0] = 'r'
	}
	if m&write != 0 {
		chars[1] = 'w'
	}
	switch {
	case m&execute != 0:
		chars[2] = 'x'
	case m&flag != 0:
		chars[2] = unset
	}
	if m&execute != 0 && m&flag != 0 {
		chars[2] = set
	}
	return string(chars)
}

// quoteName replaces the non-printable characters of a name with question
// marks, as -q asks for.
func quoteName(name string, quote bool) string {
	if !quote || !hasNonPrintable(name) {
		return name
	}
	quoted := []byte(name)
	for i, c := range quoted {
		if isNonPrintable(c) {
			quoted[i] = '?'
		}
	}
	return string(quoted)
}

// hasNonPrintable reports whether a name holds a non-printable character.
func hasNonPrintable(name string) bool {
	for i := 0; i < len(name); i++ {
		if isNonPrintable(name[i]) {
			return true
		}
	}
	return false
}

// isNonPrintable reports whether a byte is a control character.
func isNonPrintable(c byte) bool {
	return c < 0x20 || c == 0x7f
}

// textWidth returns the width a name takes in a column.
func textWidth(name string) int {
	return utf8.RuneCountInString(name)
}

// formatTime returns the date and time field of a long listing, with the year
// instead of the time of day for a file modified more than six months ago or
// in the future.
func formatTime(t, now time.Time) string {
	if t.After(now) || t.Before(now.AddDate(0, -6, 0)) {
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}

// timeOf returns the time a listing writes for an entry, which -c and -u
// change. A file system that doesn't report the requested time is written
// with its modification time.
func (l *lister) timeOf(e entry) time.Time {
	switch l.opts.time {
	case timeStatus:
		if t, ok := sysCtime(e.fi.Sys()); ok {
			return t
		}
	case timeAccessed:
		if t, ok := fsx.Atime(e.fi); ok {
			return t
		}
	}
	return e.fi.ModTime()
}

// totalBlocks returns the number of blocks the entries of a listing use.
func totalBlocks(entries []entry, opts options) uint64 {
	var total uint64
	for _, e := range entries {
		total += blocksOf(e.fi, opts)
	}
	return total
}

// blocksOf returns the number of file system blocks a file uses, in the units
// that -k chooses. A file system that doesn't report them is measured by the
// size of the file.
func blocksOf(fi fs.FileInfo, opts options) uint64 {
	unit := uint64(512)
	if opts.kilo {
		unit = 1024
	}
	if st, ok := statOf(fi); ok {
		return (st.blocks*512 + unit - 1) / unit
	}
	size := max(fi.Size(), 0)
	return (uint64(size) + unit - 1) / unit
}

// inodeOf returns the file serial number of a file, or zero when its file
// system doesn't report one.
func inodeOf(fi fs.FileInfo) uint64 {
	st, _ := statOf(fi)
	return st.ino
}

// nlinkOf returns the number of links a file has, defaulting to one when its
// file system doesn't report it.
func nlinkOf(fi fs.FileInfo) uint64 {
	st, ok := statOf(fi)
	if !ok || st.nlink == 0 {
		return 1
	}
	return st.nlink
}

// ownerOf returns the owner of a file: its name, or the identifier that
// stands for it when -n was given or the name can't be determined.
func (l *lister) ownerOf(fi fs.FileInfo) string {
	st, ok := statOf(fi)
	if !ok {
		return "0"
	}
	if l.opts.numeric {
		return strconv.FormatUint(st.uid, 10)
	}
	return l.names.user(st.uid)
}

// groupOf returns the group of a file: its name, or the identifier that
// stands for it when -n was given or the name can't be determined.
func (l *lister) groupOf(fi fs.FileInfo) string {
	st, ok := statOf(fi)
	if !ok {
		return "0"
	}
	if l.opts.numeric {
		return strconv.FormatUint(st.gid, 10)
	}
	return l.names.group(st.gid)
}

// names resolves the names of users and groups, remembering what it looked
// up, and falling back to the identifier when there is no name for it.
type names struct {
	users  map[uint64]string
	groups map[uint64]string
}

// user returns the name of a user identifier.
func (n *names) user(id uint64) string {
	if name, ok := n.users[id]; ok {
		return name
	}
	name := strconv.FormatUint(id, 10)
	if u, err := user.LookupId(name); err == nil && u.Username != "" {
		name = u.Username
	}
	if n.users == nil {
		n.users = make(map[uint64]string)
	}
	n.users[id] = name
	return name
}

// group returns the name of a group identifier.
func (n *names) group(id uint64) string {
	if name, ok := n.groups[id]; ok {
		return name
	}
	name := strconv.FormatUint(id, 10)
	if g, err := user.LookupGroupId(name); err == nil && g.Name != "" {
		name = g.Name
	}
	if n.groups == nil {
		n.groups = make(map[uint64]string)
	}
	n.groups[id] = name
	return name
}
