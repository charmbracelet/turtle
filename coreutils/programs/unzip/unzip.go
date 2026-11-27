// Package unzip implements the unzip utility.
//
// It extracts, lists and tests the members of ZIP archives through the
// standard archive/zip package: deflated and stored entries, the archive
// comment, and the 2-second timestamp granularity the format carries.
//
// The -c, -d, -f, -j, -l, -n, -o, -p, -q, -t, -u, -v, -x and -z options and
// the member operands, which are patterns where '*' and '?' also match the
// directory separator, follow the Info-ZIP unzip interface, including its
// overwrite prompt and its exit statuses. Encryption, the -a text conversion
// and the zipinfo -Z mode are out of scope.
package unzip

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/ctxio"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
	"charm.land/turtle/coreutils/internal/zipcore"
)

// Prog is the unzip core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

var _ coreutils.NewFunc = New

// mode is what a run of the utility does with the archive.
type mode int

// The operations of the utility.
const (
	modeExtract mode = iota
	modeList
	modeTest
	modeComment
)

// options holds the parsed flags of a run.
type options struct {
	mode      mode
	pipe      bool // -p
	names     bool // -c
	dir       string
	junk      bool
	never     bool
	overwrite bool
	freshen   bool
	update    bool
	quiet     int
	verbose   bool
	exclude   []string
}

// optSet describes the options of unzip.
var optSet = getopt.Set{Valued: "d", Collect: "x", Permute: true}

// usage describes the options of unzip.
const usage = `Usage: unzip [OPTION]... FILE[.zip] [MEMBER]... [-x PATTERN]... [-d DIR]
Extract, list or test the members of ZIP archives.

  -c
         extract the members to the standard output, printing their names
  -d DIR
         extract the members into DIR (the default is the working directory)
  -f
         freshen: extract only the members newer than the disk copies
  -j
         junk paths: extract every member into the extraction directory
  -l
         list the members of the archive
  -n
         never overwrite existing files
  -o
         overwrite existing files without prompting
  -p
         extract the members to the standard output, without their names
  -q
         suppress the informational messages (twice: the cautions too)
  -t
         test the integrity of the members of the archive
  -u
         update: freshen the newer members and extract the missing ones
  -v
         list the members verbosely, or print version information alone
  -x PATTERN
         exclude the members matching PATTERN (repeatable list)
  -z
         print only the archive comment
` + stdopts.Tail

// Run operates on the archive named by the first operand following the
// options given in args.
//
// It returns a [*coreutils.ExitError] holding 2 for usage errors and the
// Info-ZIP statuses otherwise: 1 for member failures, 2 for corrupted data,
// 9 for an archive which can't be opened, 11 when no member operand matches
// and 81 for an unsupported compression or encryption method. The context
// error is returned as is when ctx is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "unzip", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "unzip", err)
	}
	if len(operands) == 0 {
		if opts.verbose && opts.mode == modeExtract && !opts.pipe && !opts.names {
			fmt.Fprint(p.Stdout, stdopts.VersionText("unzip"))
			return nil
		}
		// Like Info-ZIP, a bare invocation prints the usage screen.
		fmt.Fprint(p.Stdout, usage)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	name, err := p.resolveArchive(operands[0])
	if err != nil {
		return p.archiveError(operands[0], err)
	}
	archive, err := zipcore.Read(ctx, p.Runtime, name)
	if err != nil {
		return p.archiveError(name, err)
	}
	defer archive.Close()

	r := &runner{rt: p.Runtime, opts: opts, name: name, zr: archive}
	members := operands[1:]
	defer r.closeStdin()

	// A -v without any extraction setting is the verbose listing.
	if opts.verbose && opts.mode == modeExtract && opts.idle() {
		opts.mode = modeList
		r.opts = opts
	}

	var runErr error
	switch opts.mode {
	case modeComment:
		runErr = r.comment()
	case modeList:
		runErr = r.list(members)
	case modeTest:
		runErr = r.test(ctx, members)
	default:
		runErr = r.extract(ctx, members)
	}
	if runErr != nil {
		return runErr
	}
	return r.status()
}

// resolveArchive finds the file of an archive operand: a name which isn't one
// is retried with a ".zip" suffix, as the Info-ZIP unzip does.
func (p *Prog) resolveArchive(operand string) (string, error) {
	if p.FS == nil {
		return operand, errors.New("no file system available")
	}
	for _, name := range []string{operand, operand + ".zip", operand + ".ZIP"} {
		if _, err := fs.Stat(p.FS, p.FsPath(name)); err == nil {
			return name, nil
		}
	}
	return operand, fs.ErrNotExist
}

// archiveError reports the failure of opening an archive the way Info-ZIP
// does, with the exit status 9.
func (p *Prog) archiveError(name string, err error) error {
	out := p.Stderr
	if out == nil {
		return &coreutils.ExitError{Code: 9}
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintf(out, "unzip:  cannot find or open %s, %s.zip or %s.ZIP.\n", name, name, name)
	case errors.Is(err, zip.ErrFormat), errors.Is(err, io.ErrUnexpectedEOF):
		fmt.Fprintf(out, "Archive:  %s\n", name)
		fmt.Fprint(out, eocdMessage)
		fmt.Fprintf(out, "unzip:  cannot find zipfile directory in one of %s or\n        %s.zip, and cannot find %s.ZIP, period.\n", name, name, name)
	default:
		fmt.Fprintf(out, "unzip:  %v\n", err)
	}
	return &coreutils.ExitError{Code: 9}
}

// eocdMessage is the report of a file whose end is missing the signature of
// the ZIP end-of-central-directory record.
const eocdMessage = `  End-of-central-directory signature not found.  Either this file is not
  a zipfile, or it constitutes one disk of a multi-part archive.  In the
  latter case the central directory and zipfile comment will be found on
  the last disk(s) of this archive.
`

// parseFlags parses the options and operands of a run.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c':
			opts.names = true
		case 'd':
			opts.dir = value
		case 'f':
			opts.freshen = true
		case 'j':
			opts.junk = true
		case 'l':
			opts.mode = modeList
		case 'n':
			opts.never = true
		case 'o':
			opts.overwrite = true
		case 'p':
			opts.pipe = true
		case 'q':
			opts.quiet++
		case 't':
			opts.mode = modeTest
		case 'u':
			opts.update = true
		case 'v':
			opts.verbose = true
		case 'x':
			opts.exclude = append(opts.exclude, value)
		case 'z':
			opts.mode = modeComment
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// idle reports whether the options don't ask anything but the listing.
func (o options) idle() bool {
	return !o.pipe && !o.names && !o.freshen && !o.update &&
		!o.never && !o.overwrite && !o.junk && o.dir == ""
}

// run holds the state of a single utility execution.
type runner struct {
	rt   *coreutils.Runtime
	opts options
	name string

	// zr is the open archive the run works on, when it reads one.
	zr    *zipcore.Archive
	stdin *zipcore.LineReader

	// failed marks a member which couldn't be extracted or tested, format a
	// corrupted one, unsupported an unusable compression or encryption, and
	// noMatch a member operand which matched nothing.
	failed      bool
	format      bool
	unsupported bool
	noMatch     bool

	// answer is the prompt answer which applies to the members below the one
	// which answered it: 'a' overwrites them all, 'n' skips them all.
	answer byte

	// dirs holds the mode and modification time of the extracted directory
	// entries, restored once every member is written.
	dirs []dirFix
}

// dirFix is the mode and modification time of an extracted directory,
// applied only after the whole archive is read so the permissions of a
// directory can't keep its own children out.
type dirFix struct {
	name  string
	mode  fs.FileMode
	mtime time.Time
}

// status returns the exit error matching the problems found so far, following
// the Info-ZIP statuses.
func (r *runner) status() error {
	code := 0
	switch {
	case r.failed:
		code = 1
	case r.format:
		code = 2
	case r.unsupported:
		code = 81
	case r.noMatch:
		code = 11
	}
	if code == 0 {
		return nil
	}
	return &coreutils.ExitError{Code: code}
}

// fail reports the failure of one member, marking the exit status.
func (r *runner) fail(name string, err error) {
	r.failed = true
	stdopts.Errorf(r.rt, "unzip", "%s: %v", name, patherr.Unwrap(err))
}

// usable reports whether a member's method can be processed, marking the
// unsupported ones.
func (r *runner) usable(file *zip.File) error {
	if file.Flags&0x1 != 0 {
		r.unsupported = true
		return errors.New("unsupported encrypted member")
	}
	switch file.Method {
	case zip.Store, zip.Deflate:
		return nil
	default:
		r.unsupported = true
		return fmt.Errorf("unknown compression method %d", file.Method)
	}
}

// selectMembers returns the members of the archive the operands select. An
// empty operand list selects everything. A pattern matches the whole member
// name, with the trailing slash of a directory entry optional, and '*' and
// '?' matching the directory separator too. The unmatched operands are
// reported, marking the exit status.
func (r *runner) selectMembers(operands []string) []*zip.File {
	patterns := make([]*zipcore.Pattern, len(operands))
	for i, operand := range operands {
		patterns[i] = zipcore.CompilePattern(operand)
	}
	excluded := make([]*zipcore.Pattern, len(r.opts.exclude))
	for i, pattern := range r.opts.exclude {
		excluded[i] = zipcore.CompilePattern(pattern)
	}

	var files []*zip.File
	matched := make([]bool, len(patterns))
	for _, file := range r.zr.File {
		selected := len(patterns) == 0
		for i, pattern := range patterns {
			if pattern.Matches(file.Name) {
				selected = true
				matched[i] = true
			}
		}
		for _, pattern := range excluded {
			if pattern.Matches(file.Name) {
				selected = false
				break
			}
		}
		if selected {
			files = append(files, file)
		}
	}
	for i, ok := range matched {
		if ok {
			continue
		}
		r.noMatch = true
		if r.opts.mode == modeExtract && r.opts.quiet < 2 {
			fmt.Fprintf(r.rt.Stderr, "caution: filename not matched:  %s\n", operands[i])
		}
	}
	return files
}

// header prints the archive header, unless the run is quiet or a pipe
// without names.
func (r *runner) header() {
	if r.opts.quiet > 0 || r.opts.pipe && !r.opts.names {
		return
	}
	fmt.Fprintf(r.rt.Stdout, "Archive:  %s\n", r.name)
}

// chatter prints the line of a processed member, unless the run is quiet.
func (r *runner) chatter(format string, args ...any) {
	if r.opts.quiet > 0 {
		return
	}
	fmt.Fprintf(r.rt.Stdout, format, args...)
}

// comment prints the archive comment, the -z mode.
func (r *runner) comment() error {
	r.header()
	if comment := r.zr.Comment; comment != "" {
		fmt.Fprintf(r.rt.Stdout, "%s\n", comment)
	}
	return nil
}

// list prints the members of the archive selected by the operands, in the
// short -l form or in the verbose -v form.
func (r *runner) list(operands []string) error {
	files := r.selectMembers(operands)
	r.header()
	if comment := r.zr.Comment; comment != "" && r.opts.quiet == 0 {
		fmt.Fprintf(r.rt.Stdout, "%s\n", comment)
	}
	if r.opts.verbose {
		return r.listVerbose(files)
	}
	return r.listShort(files)
}

// listShort prints the short listing of unzip -l.
func (r *runner) listShort(files []*zip.File) error {
	if r.opts.quiet == 0 {
		fmt.Fprint(r.rt.Stdout, "  Length      Date    Time    Name\n")
		fmt.Fprint(r.rt.Stdout, "---------  ---------- -----   ----\n")
	}
	var total uint64
	for _, file := range files {
		total += file.UncompressedSize64
		r.listRow(file)
	}
	if r.opts.quiet == 0 {
		fmt.Fprintf(r.rt.Stdout, "---------                     -------\n")
		fmt.Fprintf(r.rt.Stdout, "%9d%22d file%s\n", total, len(files), plural(len(files)))
	}
	return nil
}

// listRow prints one member of the short listing.
func (r *runner) listRow(file *zip.File) {
	mod := file.Modified
	fmt.Fprintf(r.rt.Stdout, "%9d  %s %s   %s\n",
		file.UncompressedSize64,
		mod.Format("01-02-2006"),
		mod.Format("15:04"),
		file.Name)
}

// listVerbose prints the verbose listing of unzip -v.
func (r *runner) listVerbose(files []*zip.File) error {
	if r.opts.quiet == 0 {
		fmt.Fprint(r.rt.Stdout, " Length   Method    Size  Cmpr    Date    Time   CRC-32   Name\n")
		fmt.Fprint(r.rt.Stdout, "--------  ------  ------- ---- ---------- ----- --------  ----\n")
	}
	var total, totalComp uint64
	for _, file := range files {
		total += file.UncompressedSize64
		totalComp += file.CompressedSize64
		r.listVerboseRow(file)
	}
	if r.opts.quiet == 0 {
		fmt.Fprint(r.rt.Stdout, "--------          -------  ---                            -------\n")
		pad := strings.Repeat(" ", 28)
		fmt.Fprintf(r.rt.Stdout, "%8d%17d %3d%%%s%d file%s\n",
			total, totalComp, percent(total, totalComp), pad, len(files), plural(len(files)))
	}
	return nil
}

// listVerboseRow prints one member of the verbose listing.
func (r *runner) listVerboseRow(file *zip.File) {
	mod := file.Modified
	fmt.Fprintf(r.rt.Stdout, "%8d  %-6s%9d %3d%% %s %s %08x  %s\n",
		file.UncompressedSize64,
		zipcore.MethodName(file.Method),
		file.CompressedSize64,
		percent(file.UncompressedSize64, file.CompressedSize64),
		mod.Format("01-02-2006"),
		mod.Format("15:04"),
		file.CRC32,
		file.Name)
}

// percent returns the compression percentage Info-ZIP reports: the share of
// the original size the compressed one takes, subtracted from 100.
func percent(size, compressed uint64) int {
	if size == 0 {
		return 0
	}
	return 100 - int(100*compressed/size)
}

// plural returns the plural suffix of a count.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// test checks the integrity of the selected members, the -t mode.
func (r *runner) test(ctx context.Context, operands []string) error {
	files := r.selectMembers(operands)
	r.header()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.testMember(ctx, file); err != nil {
			return err
		}
	}
	if r.opts.quiet == 0 {
		if r.failed || r.format || r.unsupported {
			fmt.Fprintf(r.rt.Stdout, "At least one error was detected in %s.\n", r.name)
		} else {
			fmt.Fprintf(r.rt.Stdout, "No errors detected in compressed data of %s.\n", r.name)
		}
	}
	return nil
}

// testMember decompresses one member, checking its CRC.
func (r *runner) testMember(ctx context.Context, file *zip.File) error {
	if err := r.usable(file); err != nil {
		fmt.Fprintf(r.rt.Stdout, "    testing: %-25s %s\n", file.Name, err)
		return nil
	}
	rc, err := file.Open()
	if err != nil {
		r.fail(file.Name, err)
		fmt.Fprintf(r.rt.Stdout, "    testing: %-25s error\n", file.Name)
		return nil
	}
	defer rc.Close()
	sum, err := copyHash(ctx, rc)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && !errors.Is(err, zip.ErrChecksum) {
		r.fail(file.Name, err)
		fmt.Fprintf(r.rt.Stdout, "    testing: %-25s %v\n", file.Name, err)
		return nil
	}
	if err != nil || sum != file.CRC32 {
		r.format = true
		fmt.Fprintf(r.rt.Stdout, "    testing: %-25s bad CRC %08x  (should be %08x)\n", file.Name, sum, file.CRC32)
		return nil
	}
	fmt.Fprintf(r.rt.Stdout, "    testing: %-25s OK\n", file.Name)
	return nil
}

// copyHash reads all of rc, returning its CRC-32.
func copyHash(ctx context.Context, rc io.Reader) (uint32, error) {
	sum := crc32.NewIEEE()
	_, err := io.Copy(sum, ctxio.NewReader(ctx, rc))
	return sum.Sum32(), err
}

// extract restores the selected members into the file system, or writes them
// to the standard output with -p or -c.
func (r *runner) extract(ctx context.Context, operands []string) error {
	files := r.selectMembers(operands)
	r.header()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.opts.pipe || r.opts.names {
			if err := r.toStdout(ctx, file); err != nil {
				return err
			}
			continue
		}
		if err := r.extractMember(ctx, file); err != nil {
			return err
		}
	}
	for _, v := range slices.Backward(r.dirs) {
		r.fixDir(v)
	}
	return nil
}

// toStdout writes one member to the standard output, the -p and -c modes.
func (r *runner) toStdout(ctx context.Context, file *zip.File) error {
	if isDirEntry(file) {
		return nil
	}
	if err := r.usable(file); err != nil {
		r.fail(file.Name, err)
		return nil
	}
	if r.opts.names && r.opts.quiet == 0 {
		fmt.Fprintf(r.rt.Stdout, " extracting: %-25s\n", file.Name)
	}
	rc, err := file.Open()
	if err != nil {
		r.fail(file.Name, err)
		return nil
	}
	defer rc.Close()
	sum := crc32.NewIEEE()
	if _, err := io.Copy(io.MultiWriter(r.rt.Stdout, sum), ctxio.NewReader(ctx, rc)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, zip.ErrChecksum) {
			r.format = true
			stdopts.Errorf(r.rt, "unzip", "error:  bad CRC %08x  (should be %08x)", sum.Sum32(), file.CRC32)
			return nil
		}
		r.fail(file.Name, err)
	}
	return nil
}

// extractMember restores one member into the file system.
func (r *runner) extractMember(ctx context.Context, file *zip.File) error {
	name, err := zipcore.SafeName(file.Name)
	if err != nil {
		r.fail(file.Name, err)
		return nil
	}
	if name == "" {
		return nil
	}
	if r.opts.junk {
		name = path.Base(strings.TrimSuffix(name, "/"))
	}
	if err := r.usable(file); err != nil {
		r.fail(file.Name, err)
		return nil
	}
	target := r.target(name)
	resolved := r.rt.FsPath(target)
	if isDirEntry(file) {
		if r.opts.junk {
			return nil
		}
		return r.extractDir(target, resolved, file)
	}
	if file.Mode()&fs.ModeSymlink != 0 {
		return r.extractSymlink(ctx, target, resolved, file)
	}
	return r.extractFile(ctx, target, resolved, file)
}

// target joins a member name with the extraction directory, as the messages
// show it.
func (r *runner) target(name string) string {
	dir := filepath.ToSlash(r.opts.dir)
	if dir == "" {
		return name
	}
	return strings.TrimSuffix(dir, "/") + "/" + name
}

// isDirEntry reports whether a member is a directory entry.
func isDirEntry(file *zip.File) bool {
	return file.Mode().IsDir() || strings.HasSuffix(file.Name, "/")
}

// extractDir creates one directory entry, deferring its exact attributes.
func (r *runner) extractDir(target, resolved string, file *zip.File) error {
	if r.rt.FS == nil {
		return errors.New("no file system available")
	}
	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}
	if err := zipcore.MkdirAll(r.rt.FS, resolved, mode|0o700); err != nil {
		r.fail(target, err)
		return nil
	}
	r.chatter("   creating: %s\n", strings.TrimSuffix(target, "/"))
	r.dirs = append(r.dirs, dirFix{name: resolved, mode: mode, mtime: file.Modified})
	return nil
}

// fixDir restores the mode and modification time of an extracted directory.
func (r *runner) fixDir(dir dirFix) {
	if err := r.rt.FS.Chmod(dir.name, dir.mode); err != nil {
		r.fail(dir.name, err)
		return
	}
	if err := r.rt.FS.Chtimes(dir.name, dir.mtime, dir.mtime); err != nil {
		r.fail(dir.name, err)
	}
}

// removeExisting takes an existing path out of the way of a link or a
// renamed member, keeping directories which still hold entries.
func (r *runner) removeExisting(resolved string) error {
	if _, err := r.rt.FS.Lstat(resolved); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return r.rt.FS.Remove(resolved)
}

// extractSymlink restores one symbolic link entry.
func (r *runner) extractSymlink(ctx context.Context, target, resolved string, file *zip.File) error {
	if r.rt.FS == nil {
		return errors.New("no file system available")
	}
	rc, err := file.Open()
	if err != nil {
		r.fail(file.Name, err)
		return nil
	}
	defer rc.Close()
	data, err := io.ReadAll(ctxio.NewReader(ctx, rc))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.fail(file.Name, err)
		return nil
	}
	if err := zipcore.MkdirAll(r.rt.FS, path.Dir(resolved), 0o777&^r.rt.Umask.Perm()); err != nil {
		r.fail(target, err)
		return nil
	}
	existing, err := r.rt.FS.Lstat(resolved)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.fail(target, err)
		return nil
	}
	if !exists && r.opts.freshen {
		return nil
	}
	if exists {
		// The -n, -o, -f, -u and prompt rules apply to the links too.
		proceed, renamed, err := r.decide(ctx, target, file, existing)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
		if renamed != "" {
			name, err := zipcore.SafeName(renamed)
			if err != nil {
				r.fail(file.Name, err)
				return nil
			}
			target = r.target(name)
			resolved = r.rt.FsPath(target)
		}
	}
	if err := r.removeExisting(resolved); err != nil {
		r.fail(target, err)
		return nil
	}
	if err := r.rt.FS.Symlink(string(data), resolved); err != nil {
		r.fail(target, err)
		return nil
	}
	r.chatter(" extracting: %-25s\n", target)
	return nil
}

// extractFile restores one regular member.
func (r *runner) extractFile(ctx context.Context, target, resolved string, file *zip.File) error {
	if r.rt.FS == nil {
		return errors.New("no file system available")
	}
	existing, err := r.rt.FS.Lstat(resolved)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.fail(target, err)
		return nil
	}
	if !exists && r.opts.freshen {
		return nil
	}
	if exists {
		proceed, renamed, err := r.decide(ctx, target, file, existing)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
		if renamed != "" {
			name, err := zipcore.SafeName(renamed)
			if err != nil {
				r.fail(file.Name, err)
				return nil
			}
			target = r.target(name)
			resolved = r.rt.FsPath(target)
		}
	}

	action := " extracting"
	if file.Method == zip.Deflate {
		action = "  inflating"
	}
	r.chatter("%s: %-25s\n", action, target)

	if err := zipcore.MkdirAll(r.rt.FS, path.Dir(resolved), 0o777&^r.rt.Umask.Perm()); err != nil {
		r.fail(target, err)
		return nil
	}
	// A symbolic link in the way is replaced, never followed: writing
	// through it would clobber its target.
	if exists && existing.Mode()&fs.ModeSymlink != 0 {
		if err := r.rt.FS.Remove(resolved); err != nil {
			r.fail(target, err)
			return nil
		}
	}
	rc, err := file.Open()
	if err != nil {
		r.fail(file.Name, err)
		return nil
	}
	defer rc.Close()

	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}
	out, err := r.rt.FS.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
	if err != nil {
		r.fail(target, err)
		return nil
	}
	sum := crc32.NewIEEE()
	_, copyErr := io.Copy(io.MultiWriter(out, sum), ctxio.NewReader(ctx, rc))
	closeErr := out.Close()
	if copyErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(copyErr, zip.ErrChecksum) {
			r.format = true
			stdopts.Errorf(r.rt, "unzip", "error:  bad CRC %08x  (should be %08x)", sum.Sum32(), file.CRC32)
			return nil
		}
		r.fail(target, copyErr)
		return nil
	}
	if closeErr != nil {
		r.fail(target, closeErr)
		return nil
	}
	if sum.Sum32() != file.CRC32 {
		r.format = true
		stdopts.Errorf(r.rt, "unzip", "error:  bad CRC %08x  (should be %08x)", sum.Sum32(), file.CRC32)
		return nil
	}
	if err := r.rt.FS.Chmod(resolved, mode); err != nil {
		r.fail(target, err)
		return nil
	}
	if err := r.rt.FS.Chtimes(resolved, file.Modified, file.Modified); err != nil {
		r.fail(target, err)
	}
	return nil
}

// decide reports whether an existing file is replaced, and the new name it
// is extracted as when the answer renames it, following the -f, -n, -o and
// -u options and the interactive prompt.
func (r *runner) decide(ctx context.Context, target string, file *zip.File, existing fs.FileInfo) (bool, string, error) {
	if r.opts.never {
		return false, "", nil
	}
	if r.opts.freshen || r.opts.update {
		if !file.Modified.After(existing.ModTime()) {
			return false, "", nil
		}
		if r.opts.overwrite {
			return true, "", nil
		}
	} else if r.opts.overwrite {
		return true, "", nil
	}
	switch r.answer {
	case 'a':
		return true, "", nil
	case 'n':
		return false, "", nil
	}
	return r.replace(ctx, target)
}

// replace asks whether an existing file is overwritten, returning the
// decision and, when the answer renames it, the new name. The 'A' and 'N'
// answers apply to the members below too, an invalid answer is asked again,
// and an exhausted standard input answers 'N'.
func (r *runner) replace(ctx context.Context, target string) (bool, string, error) {
	for {
		fmt.Fprintf(r.rt.Stderr, "replace %s? [y]es, [n]o, [A]ll, [N]one, [r]ename: ", target)
		line, err := r.readLine(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, "", ctxErr
			}
			fmt.Fprintf(r.rt.Stderr, " NULL\n(EOF or read error, treating as \"[N]one\" ...)\n")
			r.answer = 'n'
			return false, "", nil
		}
		if line == "" {
			fmt.Fprintf(r.rt.Stderr, "error:  invalid response []\n")
			continue
		}
		switch strings.ToLower(line[:1]) {
		case "y":
			return true, "", nil
		case "n":
			return false, "", nil
		case "a":
			r.answer = 'a'
			return true, "", nil
		case "r":
			fmt.Fprintf(r.rt.Stderr, "new name: ")
			name, err := r.readLine(ctx)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return false, "", ctxErr
				}
				name = ""
			}
			if name == "" {
				fmt.Fprintf(r.rt.Stderr, "error:  invalid response [%s]\n", name)
				continue
			}
			return true, name, nil
		default:
			fmt.Fprintf(r.rt.Stderr, "error:  invalid response [%s]\n", line)
		}
	}
}

// closeStdin stops the reader of the standard input, when one was started.
func (r *runner) closeStdin() {
	if r.stdin != nil {
		r.stdin.Close()
	}
}

// readLine reads one line of the standard input, without its line ending,
// reporting the context error when the run is cancelled.
func (r *runner) readLine(ctx context.Context) (string, error) {
	if r.stdin == nil {
		if r.rt.Stdin == nil {
			return "", io.EOF
		}
		r.stdin = zipcore.NewLineReader(r.rt.Stdin)
	}
	return r.stdin.ReadLine(ctx)
}
