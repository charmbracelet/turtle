// Package zip implements the zip utility.
//
// It creates, updates, freshens and deletes entries of ZIP archives through
// the standard archive/zip package, deflating at the -0..-9 levels and
// storing an entry when deflating it wouldn't shrink it.
//
// The -@, -D, -d, -f, -i, -j, -m, -q, -r, -u, -v, -x, -y and -z options and
// the patterns of -i and -x, which match the internal archive paths, follow
// the Info-ZIP zip interface, including its exit statuses. Encryption and
// the -e and -P options are out of scope.
package zip

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/ctxio"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/output"
	"charm.land/turtle/coreutils/internal/stdopts"
	"charm.land/turtle/coreutils/internal/zipcore"
)

// Prog is the zip core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

var _ coreutils.NewFunc = New

// mode is what a run of the utility does with the archive: add, freshen and
// update read the files of the file system, while delete works on the
// entries of an existing archive alone.
type mode int

// The operations of the utility.
const (
	modeAdd mode = iota
	modeFreshen
	modeUpdate
	modeDelete
)

// options holds the parsed flags of a run.
type options struct {
	mode     mode
	move     bool // -m
	recurse  bool // -r
	junk     bool // -j
	level    int  // -0..-9, where 0 stores and -1 is the default
	noDirs   bool // -D
	symlinks bool // -y
	quiet    bool
	verbose  bool
	names    bool // -@
	comment  bool // -z
	exclude  []*zipcore.Pattern
	include  []*zipcore.Pattern
}

// longOptions maps the long options of zip to the short ones they stand for.
var longOptions = map[string]rune{
	"archive-comment": 'z',
	"delete":          'd',
	"exclude":         'x',
	"freshen":         'f',
	"include":         'i',
	"junk-paths":      'j',
	"move":            'm',
	"names-stdin":     '@',
	"no-dir-entries":  'D',
	"paths":           'p',
	"quiet":           'q',
	"recurse-paths":   'r',
	"symlinks":        'y',
	"update":          'u',
	"verbose":         'v',
}

// optSet describes the options of zip.
var optSet = getopt.Set{Collect: "xi", Permute: true, Long: longOptions}

// usage describes the options of zip.
const usage = `Usage: zip [OPTION]... ARCHIVE FILE... [-x PATTERN]... [-i PATTERN]...
Create, update, freshen and delete ZIP archives.

  -0
         store the files without compressing them
  -1..-9
         deflate at the given level (the default is 6)
  -@
         read the input file names from the standard input
  -D
         don't store the directory entries
  -d
         delete the entries matching the patterns from the archive
  -f
         freshen: replace only the entries which changed on disk
  -i PATTERN
         include only the files matching PATTERN (repeatable list)
  -j
         junk paths: store the file names without their directories
  -m
         move the files into the archive, deleting them after it's written
  -q
         suppress the informational messages
  -r
         travel the directory structure recursively
  -u
         update: freshen the changed entries and add the missing ones
  -v
         report the entry sizes and the total savings, or the version alone
  -x PATTERN
         exclude the files matching PATTERN (repeatable list)
  -y
         store symbolic links as such, instead of the files they point to
  -z
         ask for an archive comment on the standard input
` + stdopts.Tail

// Run operates on the archive named by the first operand following the
// options given in args, which name the files added to it.
//
// It returns a [*coreutils.ExitError] holding 2 for usage errors and the
// Info-ZIP statuses otherwise: 12 when there's nothing to do, 1 for the
// warnings, 3 for a broken archive, 14 and 15 for write failures, and 18
// when an input couldn't be read. The context error is returned as is when
// ctx is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "zip", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "zip", err)
	}
	if len(args) == 0 {
		// Like Info-ZIP, a bare invocation prints the usage screen.
		fmt.Fprint(p.Stdout, usage)
		return nil
	}
	if opts.verbose && len(operands) == 0 && opts.mode == modeAdd {
		fmt.Fprint(p.Stdout, stdopts.VersionText("zip"))
		return nil
	}
	if len(operands) == 0 {
		// Like Info-ZIP, an invocation without an archive operand prints
		// the usage screen.
		fmt.Fprint(p.Stdout, usage)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "zip", "no file system available")
		return &coreutils.ExitError{Code: 15}
	}

	archive := operands[0]
	inputs := operands[1:]
	if opts.names {
		var stdinErr error
		if inputs, stdinErr = stdinNames(ctx, p.Runtime, inputs); stdinErr != nil {
			if ctx.Err() != nil {
				return stdinErr
			}
			return &coreutils.ExitError{Code: 1, Err: stdinErr}
		}
	}

	a := &archiver{rt: p.Runtime, opts: opts}
	defer a.closeStdin()
	var runErr error
	if opts.mode == modeDelete {
		runErr = a.delete(ctx, archive, inputs)
	} else {
		runErr = a.update(ctx, archive, inputs)
	}
	if runErr != nil {
		return runErr
	}
	return a.status()
}

// parseFlags parses the options and operands of a run.
func parseFlags(args []string) (options, []string, error) {
	opts := options{level: zipcore.DefaultLevel}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			opts.level = int(opt - '0')
		case '@':
			opts.names = true
		case 'D':
			opts.noDirs = true
		case 'd':
			opts.mode = modeDelete
		case 'f':
			opts.mode = modeFreshen
		case 'i':
			opts.include = append(opts.include, zipcore.CompilePattern(value))
		case 'j':
			opts.junk = true
		case 'm':
			opts.move = true
		case 'p': // the default, kept for the long form
		case 'q':
			opts.quiet = true
		case 'r':
			opts.recurse = true
		case 'u':
			opts.mode = modeUpdate
		case 'v':
			opts.verbose = true
		case 'x':
			opts.exclude = append(opts.exclude, zipcore.CompilePattern(value))
		case 'y':
			opts.symlinks = true
		case 'z':
			opts.comment = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// closeStdin stops the reader of the standard input, when one was started.
func (a *archiver) closeStdin() {
	if a.stdin != nil {
		a.stdin.Close()
	}
}

// stdinNames appends the input file names of -@ to names, one per line.
func stdinNames(ctx context.Context, rt *coreutils.Runtime, names []string) ([]string, error) {
	if rt.Stdin == nil {
		return nil, errors.New("no standard input available")
	}
	lines := zipcore.NewLineReader(rt.Stdin)
	defer lines.Close()
	for {
		name, err := lines.ReadLine(ctx)
		if errors.Is(err, io.EOF) {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		// The names are taken as typed, whitespace included, with just the
		// line ending removed.
		if name != "" {
			names = append(names, name)
		}
	}
}

// archiver holds the state of a single utility execution.
type archiver struct {
	rt      *coreutils.Runtime
	opts    options
	archive string

	// tmp is the sibling temporary file the archive is rebuilt through,
	// empty when the archive is created from scratch.
	tmp string

	stdin *zipcore.LineReader

	// writes are the entries to write once the kept members are copied,
	// replacing the ones the archive already holds. replacing are the
	// member names their old copy is dropped for, and scheduled the
	// gathered entries the plan already placed.
	writes    []write
	replacing map[string]bool
	scheduled map[string]bool

	// moved are the entries a move must delete once the archive is written,
	// in the order they were gathered.
	moved []*entry

	// failed marks an input which couldn't be read, and changed whether
	// anything was written. size and compressed count the bytes of the
	// written entries, for the -v summary.
	failed     bool
	changed    bool
	size       uint64
	compressed uint64
}

// write is one entry to write into the archive, with the verb its report
// line uses. data holds its contents, read before the archive is rewritten,
// so an unreadable input leaves the member it's replacing untouched.
type write struct {
	entry *entry
	verb  string
	data  []byte
}

// status returns the exit error matching the problems found so far, following
// the Info-ZIP statuses.
func (a *archiver) status() error {
	if a.failed {
		return &coreutils.ExitError{Code: 18}
	}
	if !a.changed {
		if a.opts.mode != modeFreshen && a.opts.mode != modeUpdate {
			fmt.Fprintf(a.rt.Stderr, "\nzip error: Nothing to do! (%s)\n", a.archive)
		}
		return &coreutils.ExitError{Code: 12}
	}
	return nil
}

// report prints the line of an entry the archive takes, unless the run is
// quiet.
func (a *archiver) report(verb, name string, size, compressed int) {
	if a.opts.quiet {
		return
	}
	method, ratio := "stored", 0
	if compressed < size {
		method, ratio = "deflated", 100-100*compressed/max(size, 1)
	}
	if a.opts.verbose {
		fmt.Fprintf(a.rt.Stdout, "  %s: %s\t(in=%d) (out=%d) (%s %d%%)\n", verb, name, size, compressed, method, ratio)
		return
	}
	fmt.Fprintf(a.rt.Stdout, "  %s: %s (%s %d%%)\n", verb, name, method, ratio)
}

// warn reports an input problem on the standard error, the Info-ZIP way.
func (a *archiver) warn(format string, args ...any) {
	fmt.Fprintf(a.rt.Stderr, "\tzip warning: "+format+"\n", args...)
}

// writeError reports a failure of writing the archive.
func (a *archiver) writeError(name string, err error, code int) error {
	stdopts.Errorf(a.rt, "zip", "%s: %v", name, err)
	return &coreutils.ExitError{Code: code}
}

// readError reports the failure of reading an existing archive, with the
// status 3 the format errors take.
func (a *archiver) readError(name string, err error) error {
	stdopts.Errorf(a.rt, "zip", "%s: %v", name, err)
	return &coreutils.ExitError{Code: 3}
}

// delete removes the entries the input patterns select from the archive,
// rebuilding it through a sibling temporary file.
func (a *archiver) delete(ctx context.Context, operand string, patterns []string) error {
	a.archive = operand
	archive, err := zipcore.Read(ctx, a.rt, operand)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			a.warn("%s not found or empty", operand)
			return nil
		}
		return a.readError(operand, err)
	}
	defer archive.Close()
	zr := archive.Reader
	deleted := make([]*zipcore.Pattern, len(patterns))
	for i, pattern := range patterns {
		deleted[i] = zipcore.CompilePattern(pattern)
	}

	out, writer, err := a.open(ctx, operand)
	if err != nil {
		return err
	}
	writer.SetComment(zr.Comment)
	kept := 0
	for _, file := range zr.File {
		if err := ctx.Err(); err != nil {
			return a.abort(out, err)
		}
		if a.drops(file.Name, deleted) {
			if !a.opts.quiet {
				fmt.Fprintf(a.rt.Stdout, "deleting: %s\n", file.Name)
			}
			a.changed = true
			continue
		}
		if err := writer.Copy(file); err != nil {
			return a.abort(out, a.writeError(operand, err, 14))
		}
		kept++
	}
	if !a.changed {
		return a.abort(out, nil)
	}
	if kept == 0 {
		a.warn("zip file empty")
	}
	if err := writer.Close(); err != nil {
		return a.abort(out, a.writeError(operand, err, 14))
	}
	return a.finish(out)
}

// drops reports whether the patterns remove the member name.
func (a *archiver) drops(name string, patterns []*zipcore.Pattern) bool {
	for _, pattern := range patterns {
		if pattern.Matches(name) {
			return true
		}
	}
	return false
}

// update adds the input files to the archive, replacing the entries which
// already exist under their names, as the add, freshen and update modes ask.
func (a *archiver) update(ctx context.Context, operand string, inputs []string) error {
	a.replacing = map[string]bool{}
	a.scheduled = map[string]bool{}
	name, exists := a.resolve(operand)
	a.archive = name

	var zr *zip.Reader
	if exists {
		archive, err := zipcore.Read(ctx, a.rt, name)
		if err != nil {
			return a.readError(name, err)
		}
		defer archive.Close()
		zr = archive.Reader
	} else if a.opts.mode == modeFreshen {
		a.warn("%s not found or empty", name)
		return nil
	} else if a.opts.mode == modeUpdate {
		a.warn("%s not found or empty", name)
	}
	if zr != nil && a.opts.mode == modeUpdate && len(inputs) == 0 {
		// An update without inputs takes every entry of the archive.
		for _, file := range zr.File {
			inputs = append(inputs, strings.TrimSuffix(file.Name, "/"))
		}
	}

	additions := a.gather(ctx, inputs)
	byName := make(map[string]*entry, len(additions))
	for _, e := range additions {
		byName[e.name] = e
	}
	if zr != nil {
		for _, file := range zr.File {
			if e := byName[trimSlash(file.Name)]; e != nil {
				a.schedule(e, file)
			}
		}
	}
	for _, e := range additions {
		if a.scheduled[e.name] || a.opts.mode == modeFreshen {
			// A freshen never adds: only the entries above change.
			continue
		}
		a.writes = append(a.writes, write{entry: e, verb: "adding"})
	}
	// The reads happen before the archive is rewritten: an input which
	// can't be read leaves the member it would replace untouched.
	var writes []write
	for _, w := range a.writes {
		if w.entry.fi.IsDir() {
			writes = append(writes, w)
			continue
		}
		data, ok := a.read(ctx, w.entry)
		if !ok {
			if err := ctx.Err(); err != nil {
				return err
			}
			delete(a.replacing, w.entry.name)
			continue
		}
		w.data = data
		writes = append(writes, w)
	}
	a.writes = writes
	if len(a.writes) == 0 && !a.opts.comment {
		return nil
	}

	out, writer, err := a.open(ctx, name)
	if err != nil {
		return err
	}
	if zr != nil {
		writer.SetComment(zr.Comment)
		for _, file := range zr.File {
			if err := ctx.Err(); err != nil {
				return a.abort(out, err)
			}
			if a.replacing[trimSlash(file.Name)] {
				continue
			}
			if err := writer.Copy(file); err != nil {
				return a.abort(out, a.writeError(name, err, 14))
			}
		}
	}
	for i := range a.writes {
		if err := ctx.Err(); err != nil {
			return a.abort(out, err)
		}
		if err := a.addEntry(writer, a.writes[i].entry, a.writes[i].verb, a.writes[i].data); err != nil {
			return a.abort(out, err)
		}
	}
	if a.opts.verbose && !a.opts.quiet {
		savings := 0
		if a.size > 0 {
			savings = int(100 - 100*a.compressed/a.size)
		}
		fmt.Fprintf(a.rt.Stdout, "total bytes=%d, compressed=%d -> %d%% savings\n", a.size, a.compressed, savings)
	}
	comment := ""
	if zr != nil {
		comment = zr.Comment
	}
	if a.opts.comment {
		if comment, err = a.setComment(ctx, comment); err != nil {
			return a.abort(out, err)
		}
		a.changed = true
	}
	writer.SetComment(comment)

	if err := writer.Close(); err != nil {
		return a.abort(out, a.writeError(name, err, 14))
	}
	return a.finish(out)
}

// trimSlash drops the trailing slash of a directory member name.
func trimSlash(name string) string {
	return strings.TrimSuffix(name, "/")
}

// schedule places one gathered entry in place of the member it replaces, or
// keeps the member when the file didn't change.
func (a *archiver) schedule(e *entry, file *zip.File) {
	a.scheduled[e.name] = true
	if a.opts.mode == modeAdd || e.fi.ModTime().After(file.Modified) {
		verb := "updating"
		if a.opts.mode == modeFreshen {
			verb = "freshening"
		}
		a.replacing[e.name] = true
		a.writes = append(a.writes, write{entry: e, verb: verb})
	}
}

// resolve finds the archive an operand names: the ".zip" suffix Info-ZIP
// appends to a name without an extension, and the file it names when one
// exists.
func (a *archiver) resolve(operand string) (string, bool) {
	for _, name := range []string{operand, archiveName(operand)} {
		if _, err := a.rt.FS.Lstat(a.rt.FsPath(name)); err == nil {
			return name, true
		}
	}
	return archiveName(operand), false
}

// open starts the archive being written: the sibling temporary file which
// replaces the name when the archive already exists, or the name itself
// otherwise.
func (a *archiver) open(ctx context.Context, name string) (io.WriteCloser, *zip.Writer, error) {
	out := name
	if _, err := a.rt.FS.Lstat(a.rt.FsPath(name)); err == nil {
		out, a.tmp = name+".tmp", name+".tmp"
	}
	file, err := output.Create(a.rt, ctx, out)
	if err != nil {
		return nil, nil, a.writeError(out, err, 15)
	}
	return file, zipcore.NewWriter(file, a.opts.level), nil
}

// abort gives up on the archive being written, removing its temporary file.
func (a *archiver) abort(out io.WriteCloser, err error) error {
	out.Close()
	if a.tmp != "" {
		a.rt.FS.Remove(a.rt.FsPath(a.tmp))
	}
	return err
}

// gather collects the entries the input operands add to the archive, walking
// the directories of a recursive run.
func (a *archiver) gather(ctx context.Context, inputs []string) []*entry {
	var entries []*entry
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			break
		}
		a.gatherInput(ctx, input, &entries)
	}
	return entries
}

// gatherInput collects the entries of one operand, appending them to
// entries.
func (a *archiver) gatherInput(ctx context.Context, input string, entries *[]*entry) {
	name := zipcore.MemberName(input)
	fsName := a.rt.FsPath(input)
	if fsName == a.rt.FsPath(a.archive) {
		// An archive is never packed into itself.
		return
	}
	fi, err := a.rt.FS.Lstat(fsName)
	if err != nil {
		a.warn("name not matched: %s", input)
		return
	}

	symlink := fi.Mode()&fs.ModeSymlink != 0
	if symlink && !a.opts.symlinks {
		// The file the link points to is archived instead.
		if fi, err = fs.Stat(a.rt.FS, fsName); err != nil {
			a.warn("name not matched: %s", input)
			return
		}
		symlink = false
	}
	if name == "" {
		// The working directory itself: only its children are archived,
		// under their own names.
		if fi.IsDir() && a.opts.recurse {
			a.descend(ctx, "", fsName, entries)
		}
		return
	}
	a.appendEntry(name, fsName, fi, symlink, entries)
	if !fi.IsDir() || !a.opts.recurse {
		return
	}
	a.descend(ctx, name, fsName, entries)
}

// descend gathers the entries below one directory of a recursive run.
func (a *archiver) descend(ctx context.Context, name, fsName string, entries *[]*entry) {
	dirs, err := fs.ReadDir(a.rt.FS, fsName)
	if err != nil {
		a.failed = true
		stdopts.Errorf(a.rt, "zip", "%s: %v", name, err)
		return
	}
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return
		}
		child, childName := path.Join(fsName, dir.Name()), joinName(name, dir.Name())
		if child == a.rt.FsPath(a.archive) {
			// An archive is never packed into itself.
			continue
		}
		fi, err := a.rt.FS.Lstat(child)
		if err != nil {
			a.warn("name not matched: %s", childName)
			continue
		}
		symlink := fi.Mode()&fs.ModeSymlink != 0
		if symlink && !a.opts.symlinks {
			if fi, err = fs.Stat(a.rt.FS, child); err != nil {
				a.warn("name not matched: %s", childName)
				continue
			}
			symlink = false
		}
		a.appendEntry(childName, child, fi, symlink, entries)
		if fi.IsDir() {
			a.descend(ctx, childName, child, entries)
		}
	}
}

// appendEntry adds one entry to the list, unless a pattern leaves it out.
func (a *archiver) appendEntry(name, fsName string, fi fs.FileInfo, symlink bool, entries *[]*entry) {
	if fi.IsDir() && (a.opts.noDirs || a.opts.junk) {
		return
	}
	if a.opts.junk {
		name = path.Base(strings.TrimSuffix(name, "/"))
	}
	if !a.selected(name) {
		return
	}
	*entries = append(*entries, &entry{name: name, fsName: fsName, fi: fi, symlink: symlink})
}

// selected reports whether a member name passes the -x and -i patterns.
func (a *archiver) selected(name string) bool {
	for _, pattern := range a.opts.exclude {
		if pattern.Matches(name) {
			return false
		}
	}
	if len(a.opts.include) > 0 {
		kept := false
		for _, pattern := range a.opts.include {
			if pattern.Matches(name) {
				kept = true
			}
		}
		if !kept {
			return false
		}
	}
	return true
}

// entry is one file gathered for the archive.
type entry struct {
	name    string      // the member name
	fsName  string      // the path the file system knows it by
	fi      fs.FileInfo // how the file looks
	symlink bool
}

// addEntry writes one file to the archive, reporting it with verb.
func (a *archiver) addEntry(w *zip.Writer, e *entry, verb string, data []byte) error {
	if e.fi.IsDir() {
		a.changed = true
		if a.opts.move {
			a.moved = append(a.moved, e)
		}
		hdr := &zip.FileHeader{Name: e.name + "/", Method: zip.Store, Modified: e.fi.ModTime()}
		hdr.SetMode(e.fi.Mode())
		if _, err := w.CreateHeader(hdr); err != nil {
			return a.writeError(a.archive, err, 14)
		}
		a.report(verb, e.name+"/", 0, 0)
		return nil
	}

	method, payload := uint16(zip.Store), data
	if !e.symlink && a.opts.level > 0 && len(data) > 0 {
		deflated, err := deflate(data, a.opts.level)
		if err != nil {
			return a.writeError(a.archive, err, 14)
		}
		if len(deflated) < len(data) {
			method, payload = zip.Deflate, deflated
		}
	}
	hdr := &zip.FileHeader{
		Name:               e.name,
		Method:             method,
		Modified:           e.fi.ModTime(),
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(len(payload)),
		UncompressedSize64: uint64(len(data)),
	}
	hdr.SetMode(e.fi.Mode())
	zipcore.DOSTime(hdr, e.fi.ModTime())
	fw, err := w.CreateRaw(hdr)
	if err != nil {
		return a.writeError(a.archive, err, 14)
	}
	if _, err := fw.Write(payload); err != nil {
		return a.writeError(a.archive, err, 14)
	}
	a.changed = true
	if a.opts.move {
		a.moved = append(a.moved, e)
	}
	a.size += uint64(len(data))
	a.compressed += uint64(len(payload))
	a.report(verb, e.name, len(data), len(payload))
	return nil
}

// deflate compresses data at level.
func deflate(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, level)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// read returns the contents of one gathered file, reporting whether it
// could be read. The failure is warned about here: the entry is only
// skipped.
func (a *archiver) read(ctx context.Context, e *entry) ([]byte, bool) {
	if e.symlink {
		target, err := a.rt.FS.ReadLink(e.fsName)
		if err != nil {
			a.failed = true
			a.warn("could not read the link %s", e.name)
			return nil, false
		}
		return []byte(target), true
	}
	file, err := a.rt.FS.Open(e.fsName)
	if err != nil {
		a.failed = true
		a.warn("could not open for reading: %s", e.name)
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(ctxio.NewReader(ctx, file))
	if err != nil {
		if ctx.Err() != nil {
			return nil, false
		}
		a.failed = true
		a.warn("could not read: %s", e.name)
		return nil, false
	}
	return data, true
}

// setComment asks for the archive comment on the standard input, ending at
// a line holding only a period or at the end of the input. The old comment
// is shown first.
func (a *archiver) setComment(ctx context.Context, old string) (string, error) {
	if !a.opts.quiet {
		if old != "" {
			fmt.Fprintf(a.rt.Stdout, "current zip file comment is:\n%s\n", old)
		}
		fmt.Fprint(a.rt.Stdout, "enter new zip file comment (end with .):\n")
	}
	var lines []string
	for {
		line, err := a.readLine(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			break
		}
		if line == "." {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// readLine reads one line of the standard input, without its line ending,
// reporting the context error when the run is cancelled.
func (a *archiver) readLine(ctx context.Context) (string, error) {
	if a.stdin == nil {
		if a.rt.Stdin == nil {
			return "", io.EOF
		}
		a.stdin = zipcore.NewLineReader(a.rt.Stdin)
	}
	return a.stdin.ReadLine(ctx)
}

// finish closes the archive, moves its temporary file over the original,
// and deletes the input files of a move.
func (a *archiver) finish(out io.WriteCloser) error {
	if err := out.Close(); err != nil {
		return a.writeError(a.archive, err, 14)
	}
	if a.tmp != "" {
		if err := a.rt.FS.Rename(a.rt.FsPath(a.tmp), a.rt.FsPath(a.archive)); err != nil {
			return a.writeError(a.archive, err, 18)
		}
	}
	if a.opts.move {
		a.removeMoved()
	}
	return nil
}

// removeMoved deletes the files a move archived, and the directories the
// removals left empty.
func (a *archiver) removeMoved() {
	for _, e := range a.moved {
		if !e.fi.IsDir() {
			if err := a.rt.FS.Remove(e.fsName); err != nil && !errors.Is(err, fs.ErrNotExist) {
				a.failed = true
				a.warn("could not remove: %s", e.name)
			}
		}
	}
	for _, v := range slices.Backward(a.moved) {
		if v.fi.IsDir() {
			a.rt.FS.Remove(v.fsName)
		}
	}
}

// joinName joins a member name prefix and a child name.
func joinName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

// archiveName returns the name of an archive operand, with the ".zip" suffix
// Info-ZIP appends when the name has no extension at all.
func archiveName(operand string) string {
	if filepath.Ext(filepath.Base(operand)) == "" {
		return operand + ".zip"
	}
	return operand
}
