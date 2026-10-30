// Package ls implements the POSIX ls utility.
//
// It writes the name of every file given, and the names of the files contained
// in every directory given, defaulting to the working directory. The default
// output is one entry per line, as specified for output that isn't written to
// a terminal, which isn't detected here.
//
// Names are compared byte by byte, which is the collating sequence of the
// POSIX locale, and the fields of the long format are separated by a single
// space, exactly as the specification writes them, rather than padded to line
// up like GNU and BSD do.
//
// The file system abstraction used here doesn't carry owners, so a file whose
// system information isn't available, like in a memory file system, is written
// with the numeric identifier zero for its owner and group, a link count of
// one, and no file serial number, and has its blocks measured by its size.
// Character and block special files are written with their size, as no device
// information is available either. Directory entries dot and dot-dot aren't
// reported by the Go file systems, so -A and -a are equivalent, and -f keeps
// the order the file system reports when it can, which a memory file system
// never can.
package ls

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the ls core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run lists the given files and the contents of the given directories, or the
// working directory when no operand is given. Listing stops when ctx is done,
// and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "ls", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "ls")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if opts.fast {
		opts.sort = sortNone
		opts.reverse = false
	}
	l := &lister{prog: p, opts: opts, width: p.displayWidth(), now: time.Now()}
	// The listing of an implicit dot operand is written without a header.
	header := len(operands) > 1 || opts.recursive
	if len(operands) == 0 {
		operands = []string{"."}
		header = false
	}

	var files, dirs []entry
	for _, arg := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if arg == "" {
			p.errorf("invalid zero-length file name")
			l.failed = true
			continue
		}
		e, err := l.entryFor(p.resolve(arg), arg, true)
		if err != nil {
			p.errorf("cannot access '%s': %v", arg, patherr.Unwrap(err))
			l.failed = true
			continue
		}
		if e.fi.IsDir() && !opts.dirs {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	// Non-directory operands are written first, and both groups are sorted by
	// the name they were specified with.
	if len(operands) > 1 {
		slices.SortFunc(files, compareNames)
		slices.SortFunc(dirs, compareNames)
	}
	if len(files) > 0 {
		if err := l.list(ctx, files, false); err != nil {
			return err
		}
	}
	for _, d := range dirs {
		if err := l.listDir(ctx, d, header); err != nil {
			return err
		}
	}
	if l.failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// lister holds the state of a single run.
type lister struct {
	prog    *Prog
	opts    options
	width   int       // the width the column formats write within
	now     time.Time // the time the long format measures a file against
	names   names
	printed bool // whether anything was written yet
	visits  map[string]bool
	failed  bool
}

// entry is a file to be written: the name it's written with, the path it was
// read from, and the information chosen for the requested options.
type entry struct {
	path     string      // the path used with the file system
	name     string      // the name as written
	fi       fs.FileInfo // the information of the file, per the -H and -L rules
	link     fs.FileInfo // the information of the link itself, when it's one
	followed bool        // whether fi is that of the file a link refers to
	target   string      // the contents of the link, when it's one
}

// compareNames sorts entries by the name they are written with.
func compareNames(a, b entry) int {
	return strings.Compare(a.name, b.name)
}

// entryFor returns the entry for the file at path, written as name. Operands
// that name a symbolic link to a directory are followed, so that their
// contents can be listed.
func (l *lister) entryFor(path, name string, isOperand bool) (entry, error) {
	e := entry{path: path, name: name}
	lfi, err := fs.Lstat(l.prog.FS, path)
	if err != nil {
		return e, err
	}
	e.fi = lfi
	if lfi.Mode()&fs.ModeSymlink == 0 {
		return e, nil
	}
	e.link = lfi
	if links, ok := l.prog.FS.(fs.ReadLinkFS); ok {
		e.target, _ = links.ReadLink(path)
	}
	if !l.follows(isOperand) {
		return e, nil
	}
	sfi, err := fs.Stat(l.prog.FS, path)
	if err != nil {
		// A link that can't be resolved is written as itself. Only the
		// options that ask for it make it a failure.
		if l.opts.follow != followDefault {
			l.prog.errorf("cannot access '%s': %v", name, patherr.Unwrap(err))
			l.failed = true
		}
		return e, nil
	}
	if l.opts.follow == followDefault && !sfi.IsDir() {
		// The link was only followed to know whether it names a directory.
		return e, nil
	}
	e.fi, e.followed = sfi, true
	return e, nil
}

// follows reports whether the information of a symbolic link must be that of
// the file it refers to.
func (l *lister) follows(isOperand bool) bool {
	switch l.opts.follow {
	case followAlways:
		return true
	case followOperands:
		return isOperand
	}
	// By default only an operand naming a symbolic link is followed, and only
	// when no option asks for the link itself to be written.
	return isOperand && !l.opts.dirs && !l.opts.long && l.opts.classify < classifyFile
}

// listDir writes the entries of a directory, and with -R the listings of the
// directories they contain.
func (l *lister) listDir(ctx context.Context, e entry, header bool) error {
	if id := dirID(e.fi); id != "" {
		if l.visits[id] {
			l.prog.errorf("%s: directory causes a cycle", e.name)
			l.failed = true
			return nil
		}
		if l.visits == nil {
			l.visits = make(map[string]bool)
		}
		l.visits[id] = true
		defer delete(l.visits, id)
	}
	if header {
		l.writeHeader(e.name)
	}
	entries, err := l.readEntries(e.path, e.name)
	if err != nil {
		l.prog.errorf("cannot open directory '%s': %v", e.name, patherr.Unwrap(err))
		l.failed = true
		return nil
	}
	l.sortEntries(entries)
	if err := l.list(ctx, entries, true); err != nil {
		return err
	}
	if !l.opts.recursive {
		return nil
	}
	for _, child := range entries {
		if !child.fi.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		nested := child
		nested.name = joinName(e.name, child.name)
		if err := l.listDir(ctx, nested, true); err != nil {
			return err
		}
	}
	return nil
}

// readEntries returns the entries of the directory at dir, written below the
// display name, dropping the hidden ones unless they were asked for.
func (l *lister) readEntries(dir, display string) ([]entry, error) {
	raw, err := l.readDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]entry, 0, len(raw))
	for _, de := range raw {
		name := de.Name()
		if !l.opts.all && strings.HasPrefix(name, ".") {
			continue
		}
		e, err := l.entryFor(path.Join(dir, name), name, false)
		if err != nil {
			l.prog.errorf("cannot access '%s': %v", joinName(display, name), patherr.Unwrap(err))
			l.failed = true
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// readDir reads the entries of a directory. With -f it asks the file system
// for them in the order it keeps them, which only a real directory can give.
func (l *lister) readDir(dir string) ([]fs.DirEntry, error) {
	if l.opts.sort == sortNone {
		if f, err := l.prog.FS.Open(dir); err == nil {
			defer f.Close()
			if dirFile, ok := f.(fs.ReadDirFile); ok {
				if entries, err := dirFile.ReadDir(-1); err == nil {
					return entries, nil
				}
			}
		}
	}
	return fs.ReadDir(l.prog.FS, dir)
}

// sortEntries orders a listing by the requested key, and the reverse of it.
func (l *lister) sortEntries(entries []entry) {
	if l.opts.sort == sortNone {
		return
	}
	slices.SortFunc(entries, func(a, b entry) int {
		var order int
		switch l.opts.sort {
		case sortSize:
			order = sizeOrder(a, b)
		case sortTime:
			order = timeOrder(l.timeOf, a, b)
		case sortExtension:
			order = extensionOrder(a, b)
		case sortVersion:
			order = versionOrder(a, b)
		case sortWidth:
			order = widthOrder(a, b)
		}
		if order == 0 {
			order = strings.Compare(a.name, b.name)
		}
		if l.opts.reverse {
			return -order
		}
		return order
	})
}

// sizeOrder sorts the largest file first.
func sizeOrder(a, b entry) int {
	switch {
	case a.fi.Size() > b.fi.Size():
		return -1
	case a.fi.Size() < b.fi.Size():
		return 1
	}
	return 0
}

// timeOrder sorts the most recently modified file first.
func timeOrder(times func(entry) time.Time, a, b entry) int {
	ta, tb := times(a), times(b)
	switch {
	case ta.After(tb):
		return -1
	case tb.After(ta):
		return 1
	}
	return 0
}

// extensionOrder sorts by the extension of a name: the part from its last
// dot on, or nothing for a name without a dot, which sorts first.
func extensionOrder(a, b entry) int {
	return strings.Compare(extensionOf(a.name), extensionOf(b.name))
}

// extensionOf returns the extension of a name, from its last dot on.
func extensionOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

// versionOrder sorts names by the versions they hold, as -v asks for.
func versionOrder(a, b entry) int {
	return compareVersion(a.name, b.name)
}

// compareVersion compares two names as versions, the way the GNU filevercmp
// does: runs of decimal digits are taken as numbers, and the comparison is
// made without the suffixes, the dot-extensions ending each name, before
// being made with them. A name with a leading dot sorts first, with "." and
// ".." before the others.
func compareVersion(a, b string) int {
	switch {
	case a == "":
		if b == "" {
			return 0
		}
		return -1
	case b == "":
		return 1
	}
	if a[0] == '.' {
		if b[0] != '.' {
			return -1
		}
		if rankA, rankB := dotRank(a), dotRank(b); rankA != rankB {
			return rankA - rankB
		}
	} else if b[0] == '.' {
		return 1
	}
	prefixA, prefixB := prefixLen(a), prefixLen(b)
	result := verrevcmp(a[:prefixA], b[:prefixB])
	if result != 0 || (prefixA == len(a) && prefixB == len(b)) {
		return result
	}
	return verrevcmp(a, b)
}

// dotRank orders the names with a leading dot: "." first, then "..", then
// the others.
func dotRank(name string) int {
	switch name {
	case ".":
		return 0
	case "..":
		return 1
	}
	return 2
}

// prefixLen returns the length of a name without its suffix: the chain of
// dot-extensions ending it, each a dot followed by a letter or a tilde and
// then letters, digits or tildes.
func prefixLen(s string) int {
	prefix := 0
	for i := 0; i < len(s); {
		i++
		prefix = i
		for i+1 < len(s) && s[i] == '.' && (isAlpha(s[i+1]) || s[i+1] == '~') {
			for i += 2; i < len(s) && (isAlnum(s[i]) || s[i] == '~'); i++ {
			}
		}
	}
	return prefix
}

// verrevcmp compares two versions the Debian way: each run of decimal
// digits is taken as a number, and the other bytes by their rank.
func verrevcmp(a, b string) int {
	ia, ib := 0, 0
	for ia < len(a) || ib < len(b) {
		firstDiff := 0
		for (ia < len(a) && !isDigit(a[ia])) || (ib < len(b) && !isDigit(b[ib])) {
			rankA, rankB := versionRank(a, ia), versionRank(b, ib)
			if rankA != rankB {
				return rankA - rankB
			}
			ia++
			ib++
		}
		for ia < len(a) && a[ia] == '0' {
			ia++
		}
		for ib < len(b) && b[ib] == '0' {
			ib++
		}
		for ia < len(a) && ib < len(b) && isDigit(a[ia]) && isDigit(b[ib]) {
			if firstDiff == 0 {
				firstDiff = int(a[ia]) - int(b[ib])
			}
			ia++
			ib++
		}
		if ia < len(a) && isDigit(a[ia]) {
			return 1
		}
		if ib < len(b) && isDigit(b[ib]) {
			return -1
		}
		if firstDiff != 0 {
			return firstDiff
		}
	}
	return 0
}

// versionRank orders the byte of s at pos: a tilde sorts before everything,
// the end of s before every other byte, and a byte that is neither a letter
// nor a digit sorts after the letters.
func versionRank(s string, pos int) int {
	if pos == len(s) {
		return -1
	}
	c := s[pos]
	switch {
	case isDigit(c):
		return 0
	case isAlpha(c):
		return int(c)
	case c == '~':
		return -2
	default:
		return int(c) + 256
	}
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func isAlpha(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func isAlnum(c byte) bool { return isAlpha(c) || isDigit(c) }

// widthOrder sorts by the width a name is written with, the narrowest first.
func widthOrder(a, b entry) int {
	return textWidth(a.name) - textWidth(b.name)
}

// writeHeader writes the name of a directory before its listing, with the
// blank line that separates it from what came before.
func (l *lister) writeHeader(name string) {
	if l.printed {
		fmt.Fprint(l.prog.Stdout, "\n")
	}
	fmt.Fprintf(l.prog.Stdout, "%s:\n", name)
	l.printed = true
}

// resolve returns the path to use with the file system for a given name.
func (p *Prog) resolve(name string) string {
	return path.Clean(filepath.ToSlash(p.ResolvePath(name)))
}

// displayWidth returns the width the column formats write within, which the
// COLUMNS variable overrides.
func (p *Prog) displayWidth() int {
	const defaultWidth = 80
	v, ok := p.LookupEnv("COLUMNS")
	if !ok {
		return defaultWidth
	}
	width, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || width < 1 {
		return defaultWidth
	}
	return width
}

// joinName appends the name of an entry to the name of its directory.
func joinName(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// dirID returns what identifies a directory for loop detection, or an empty
// string when the file system reports nothing to identify it with.
func dirID(fi fs.FileInfo) string {
	st, ok := statOf(fi)
	if !ok {
		return ""
	}
	return strconv.FormatUint(st.dev, 10) + ":" + strconv.FormatUint(st.ino, 10)
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "ls: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
