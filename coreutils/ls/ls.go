// Package ls implements the ls core utility, following the GNU manual.
//
// It writes the name of every file given, and the names of the files
// contained in every directory given, defaulting to the working directory.
// The default output is one entry per line, as specified for output that
// isn't written to a terminal; on a terminal the entries fill the columns
// instead, their nongraphic characters are hidden, and their names are
// quoted the way the shell reads them.
//
// Names are compared byte by byte, which is the collating sequence of the
// C locale, the only one here: no utility of this project implements the
// collating sequences of other locales. The fields of the long format are
// aligned the way GNU aligns them, and the timestamps and the quoted
// names are written as the C locale writes them.
//
// The file system abstraction used here doesn't carry owners, so a file
// whose system information isn't available, like in a memory file system,
// is written with the numeric identifier zero for its owner and group, a
// link count of one, and no file serial number, and has its blocks
// measured by its size. Character and block special files are written
// with their size, as no device numbers are available either, and the
// birth time a system doesn't report is written as a question mark.
// Directory entries dot and dot-dot aren't reported by the Go file
// systems, so -A and -a are equivalent, and -f keeps the order the file
// system reports when it can, which a memory file system never can.
package ls

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/fileinfo"
	"charm.land/snail/coreutils/internal/numparse"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/sizefmt"
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

// Run lists the given files and the contents of the given directories, or
// the working directory when no operand is given. Listing stops when ctx
// is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "ls", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "ls", err)
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "ls", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if opts.fast {
		opts.sort = sortNone
		opts.reverse = false
	}
	l, err := p.newLister(opts)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "ls", err)
	}
	// The listing of an implicit dot operand is written without a header,
	// unless the listing is recursive.
	header := len(operands) > 1 || opts.recursive
	if len(operands) == 0 {
		operands = []string{"."}
	}

	var files, dirs []entry
	for _, arg := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if arg == "" {
			stdopts.Errorf(p.Runtime, "ls", "invalid zero-length file name")
			l.failed = true
			continue
		}
		e, err := l.entryFor(p.FsPath(arg), arg, true)
		if err != nil {
			stdopts.Errorf(p.Runtime, "ls", "cannot access '%s': %v", arg, patherr.Unwrap(err))
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

// lister holds the state of a single run, with the options resolved from
// the environment of the runtime.
type lister struct {
	prog *Prog
	opts options

	width       int       // the width the column formats write within
	now         time.Time // the time the long format measures a file against
	names       names
	printed     bool // whether anything was written yet
	visits      map[string]bool
	failed      bool
	eol         string       // how a line of output ends, NUL with --zero
	tabsize     int          // the distance between the tab stops used to align
	hideControl bool         // write nongraphic characters as question marks
	timeStyle   timeStyle    // how the long format writes a timestamp
	timeWidth   int          // the width a timestamp without one takes
	blocksSpec  sizefmt.Spec // how the block counts are written
	sizeSpec    sizefmt.Spec // how the sizes of the long format are written
}

// newLister builds the lister of a run, filling the options the
// environment and the terminal the standard output is give the defaults
// of. An environment variable with an invalid value is an error, except
// the ones GNU only warns about.
func (p *Prog) newLister(opts options) (*lister, error) {
	tty := p.Terminal(1)
	l := &lister{
		prog:  p,
		opts:  opts,
		width: p.displayWidth(opts),
		now:   p.Now(),
		eol:   "\n",
	}
	if opts.zero {
		l.eol = "\x00"
	}
	if !opts.quotingSet {
		if word, ok := p.LookupEnv("QUOTING_STYLE"); ok {
			if style, found := quotingStyles[word]; found {
				opts.quoting = style
			} else {
				stdopts.Errorf(p.Runtime, "ls", "ignoring invalid value of environment variable QUOTING_STYLE: %s", word)
			}
		} else if tty {
			opts.quoting = quoteShellEscape
		}
	}
	switch opts.hideCtrl {
	case hideAlways:
		l.hideControl = true
	case showAlways:
		l.hideControl = false
	default:
		l.hideControl = tty
	}
	if !opts.layoutSet && !opts.long && !opts.zero && tty {
		opts.layout = layoutColumns
	}
	l.tabsize = opts.tabsize
	if l.tabsize < 0 {
		l.tabsize = 8
		if word, ok := p.LookupEnv("TABSIZE"); ok {
			if size, err := strconv.Atoi(strings.TrimSpace(word)); err == nil && size >= 0 {
				l.tabsize = size
			} else {
				stdopts.Errorf(p.Runtime, "ls", "ignoring invalid tab size in environment variable TABSIZE: %s", word)
			}
		}
	}
	blocks, sizes, err := p.blockSpecs(opts)
	if err != nil {
		return nil, err
	}
	l.blocksSpec, l.sizeSpec = blocks, sizes
	word := opts.timeStyle
	if word == "" {
		word = p.Getenv("TIME_STYLE")
	}
	if word == "" {
		word = "locale"
	}
	style, err := resolveTimeStyle(word, p.lcTime())
	if err != nil {
		return nil, fmt.Errorf("invalid --time-style argument '%s'", word)
	}
	l.timeStyle = style
	// The width a timestamp which can't be shown takes: the one the recent
	// form of the style writes for a sample time.
	sample := time.Date(2005, 7, 1, 12, 0, 0, 0, time.Local)
	l.timeWidth = len(style.format(sample, sample))
	l.opts = opts
	return l, nil
}

// lcTime returns the value the LC_TIME category resolves to: the one
// LC_ALL gives, else LC_TIME, else LANG, empty when none is set.
func (p *Prog) lcTime() string {
	for _, name := range []string{"LC_ALL", "LC_TIME", "LANG"} {
		if v, ok := p.LookupEnv(name); ok {
			return v
		}
	}
	return ""
}

// blockSpecs returns the specs the block counts and the sizes of the long
// format are written with: the one --block-size, -h or --si set, else the
// one -k resets the block counts to, else the one the environment gives,
// which BLOCKSIZE gives the block counts only.
func (p *Prog) blockSpecs(opts options) (blocks, sizes sizefmt.Spec, err error) {
	if opts.blockSizeSet {
		return opts.blockSize, opts.blockSize, nil
	}
	blocks, sizes = sizefmt.KibibyteSpec, sizefmt.ByteSpec
	spec, ok, err := p.envSize("LS_BLOCK_SIZE")
	if err != nil {
		return blocks, sizes, err
	}
	if !ok {
		spec, ok, err = p.envSize("BLOCK_SIZE")
		if err != nil {
			return blocks, sizes, err
		}
	}
	if ok {
		blocks, sizes = spec, spec
	} else if spec, ok, err = p.envSize("BLOCKSIZE"); err != nil {
		return blocks, sizes, err
	} else if ok {
		blocks = spec
	} else if p.Getenv("POSIXLY_CORRECT") != "" {
		blocks = sizefmt.Fixed(512)
	}
	if opts.kilo {
		blocks = sizefmt.KibibyteSpec
	}
	return blocks, sizes, nil
}

// envSize returns the block size a variable of the environment gives,
// reporting whether it's set.
func (p *Prog) envSize(name string) (sizefmt.Spec, bool, error) {
	word, ok := p.LookupEnv(name)
	if !ok {
		return sizefmt.Spec{}, false, nil
	}
	spec, err := sizefmt.Parse(word)
	if err != nil {
		return sizefmt.Spec{}, false, fmt.Errorf("invalid suffix in %s: '%s'", name, word)
	}
	return spec, true, nil
}

// entry is a file to be written: the name it's written with, the path it
// was read from, and the information chosen for the requested options.
type entry struct {
	path     string      // the path used with the file system
	name     string      // the name as written
	fi       fs.FileInfo // the information of the file, per the -H and -L rules
	link     fs.FileInfo // the information of the link itself, when it's one
	followed bool        // whether fi is that of the file a link refers to
	target   string      // the contents of the link, when it's one
	dirLike  bool        // whether --group-directories-first counts it as a directory
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
			stdopts.Errorf(l.prog.Runtime, "ls", "cannot access '%s': %v", name, patherr.Unwrap(err))
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

// follows reports whether the information of a symbolic link must be that
// of the file it refers to.
func (l *lister) follows(isOperand bool) bool {
	switch l.opts.follow {
	case followAlways:
		return true
	case followOperands:
		return isOperand
	}
	// By default only an operand naming a symbolic link is followed, and
	// only when no option asks for the link itself to be written.
	if l.opts.followArgDirs {
		return isOperand
	}
	return isOperand && !l.opts.dirs && !l.opts.long && l.opts.classify < classifyFile
}

// listDir writes the entries of a directory, and with -R the listings of
// the directories they contain.
func (l *lister) listDir(ctx context.Context, e entry, header bool) error {
	if id := dirID(e.fi); id != "" {
		if l.visits[id] {
			stdopts.Errorf(l.prog.Runtime, "ls", "%s: directory causes a cycle", e.name)
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
		stdopts.Errorf(l.prog.Runtime, "ls", "cannot open directory '%s': %v", e.name, patherr.Unwrap(err))
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

// readEntries returns the entries of the directory at dir, written below
// the display name, dropping the hidden ones and the ones the ignore
// patterns match, unless they were asked for.
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
		if l.ignores(name) {
			continue
		}
		e, err := l.entryFor(path.Join(dir, name), name, false)
		if err != nil {
			stdopts.Errorf(l.prog.Runtime, "ls", "cannot access '%s': %v", joinName(display, name), patherr.Unwrap(err))
			l.failed = true
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ignores reports whether a directory entry is dropped: one matching an
// ignore pattern always is, and one matching a hide pattern is unless -a
// or -A was given.
func (l *lister) ignores(name string) bool {
	for _, pattern := range l.opts.ignore {
		if matchName(pattern, name) {
			return true
		}
	}
	if l.opts.all {
		return false
	}
	for _, pattern := range l.opts.hide {
		if matchName(pattern, name) {
			return true
		}
	}
	return false
}

// matchName reports whether a name matches a shell pattern, the way the
// ignore options of ls match: an initial dot in the name isn't matched by
// a wildcard at the start of the pattern.
func matchName(pattern, name string) bool {
	if len(name) > 0 && name[0] == '.' && (len(pattern) == 0 || pattern[0] != '.') {
		return false
	}
	matched, err := path.Match(pattern, name)
	return err == nil && matched
}

// readDir reads the entries of a directory. With -f it asks the file
// system for them in the order it keeps them, which only a real directory
// can give.
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

// sortEntries orders a listing by the requested key, and the reverse of
// it, with the directories first when the option asks for them.
func (l *lister) sortEntries(entries []entry) {
	if l.opts.sort == sortNone {
		return
	}
	groupDirs := l.opts.groupDirs
	if groupDirs {
		for i := range entries {
			entries[i].dirLike = l.isDirLike(entries[i])
		}
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if groupDirs {
			if a.dirLike != b.dirLike {
				if a.dirLike {
					return -1
				}
				return 1
			}
		}
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

// isDirLike reports whether an entry counts as a directory for
// --group-directories-first, which a symbolic link to a directory does.
func (l *lister) isDirLike(e entry) bool {
	if e.fi.IsDir() {
		return true
	}
	if e.link == nil || e.followed {
		return false
	}
	fi, err := fs.Stat(l.prog.FS, e.path)
	return err == nil && fi.IsDir()
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
		for (ia < len(a) && !numparse.IsDigit(a[ia])) || (ib < len(b) && !numparse.IsDigit(b[ib])) {
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
		for ia < len(a) && ib < len(b) && numparse.IsDigit(a[ia]) && numparse.IsDigit(b[ib]) {
			if firstDiff == 0 {
				firstDiff = int(a[ia]) - int(b[ib])
			}
			ia++
			ib++
		}
		if ia < len(a) && numparse.IsDigit(a[ia]) {
			return 1
		}
		if ib < len(b) && numparse.IsDigit(b[ib]) {
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
	case numparse.IsDigit(c):
		return 0
	case isAlpha(c):
		return int(c)
	case c == '~':
		return -2
	default:
		return int(c) + 256
	}
}

func isAlpha(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func isAlnum(c byte) bool { return isAlpha(c) || numparse.IsDigit(c) }

// widthOrder sorts by the width a name is written with, the narrowest first.
func widthOrder(a, b entry) int {
	return textWidth(a.name) - textWidth(b.name)
}

// writeHeader writes the name of a directory before its listing, with the
// blank line that separates it from what came before, quoted with its
// colon so it isn't taken for the header of another listing.
func (l *lister) writeHeader(name string) {
	if l.printed {
		fmt.Fprint(l.prog.Stdout, "\n")
	}
	quoted, _ := quoteName(name, l.opts.quoting, l.hideControl, ":")
	fmt.Fprintf(l.prog.Stdout, "%s:\n", quoted)
	l.printed = true
}

// displayWidth returns the width the column formats write within: the one
// -w sets, else the COLUMNS variable, else 80. A zero -w means no limit.
func (p *Prog) displayWidth(opts options) int {
	if opts.width >= 0 {
		return opts.width
	}
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
	st, ok := fileinfo.Stat(fi)
	if !ok {
		return ""
	}
	return strconv.FormatUint(st.Dev, 10) + ":" + strconv.FormatUint(st.Ino, 10)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
