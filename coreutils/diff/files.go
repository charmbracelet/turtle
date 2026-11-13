package diff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"charm.land/turtle/coreutils/internal/diff"
)

// pair holds the two files of one comparison: their names as written to the
// output, their paths in the file system, empty for the standard input,
// whether one of them is missing and compared against an empty file, and
// the header line written before their changes when compared under
// directories.
type pair struct {
	name1, path1 string
	missing1     bool
	name2, path2 string
	missing2     bool
	header       string
}

// compareFiles reads both files of the pair and writes their differences,
// reporting whether they differ.
func (p *Prog) compareFiles(ctx context.Context, pr pair, opts options) (bool, error) {
	data1, err := p.readFile(ctx, pr.path1, pr.missing1)
	if err != nil {
		return false, &pathError{label: pr.name1, err: err}
	}
	data2, err := p.readFile(ctx, pr.path2, pr.missing2)
	if err != nil {
		return false, &pathError{label: pr.name2, err: err}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if opts.stripCR {
		data1, data2 = stripCR(data1), stripCR(data2)
	}

	if !opts.text && (isBinary(data1) || isBinary(data2)) {
		if bytes.Equal(data1, data2) {
			return false, p.reportIdentical(pr, opts)
		}
		if opts.brief {
			if err := p.write(fmt.Sprintf("Files %s and %s differ\n", pr.label1(opts), pr.label2(opts))); err != nil {
				return false, err
			}
			return true, nil
		}
		if err := p.writeHeader(pr, opts); err != nil {
			return false, err
		}
		if err := p.write(fmt.Sprintf("Binary files %s and %s differ\n", pr.label1(opts), pr.label2(opts))); err != nil {
			return false, err
		}
		return true, nil
	}

	a, b := diff.SplitLines(data1), diff.SplitLines(data2)
	key := opts.keyOpts().Key()
	changes := diff.Lines(a, b, key)
	changes = opts.dropIgnored(changes, a, b, key)
	// The side-by-side format writes the common lines too, so even equal
	// files come out, as long as the common lines aren't suppressed.
	sided := opts.format == sideFormat && !opts.suppressCommon
	if len(changes) == 0 && !sided {
		return false, p.reportIdentical(pr, opts)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if opts.brief {
		if err := p.write(fmt.Sprintf("Files %s and %s differ\n", pr.label1(opts), pr.label2(opts))); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := p.writeHeader(pr, opts); err != nil {
		return false, err
	}
	if err := p.writeChanges(pr, a, b, changes, opts); err != nil {
		return false, err
	}
	return len(changes) > 0, nil
}

// reportIdentical writes the message of the -s option about files which
// compare equal, when it's asked for.
func (p *Prog) reportIdentical(pr pair, opts options) error {
	if !opts.reportIdentical {
		return nil
	}
	return p.write(fmt.Sprintf("Files %s and %s are identical\n", pr.label1(opts), pr.label2(opts)))
}

// label1 returns the name of the first file of the pair as the messages
// write it: its -L label when one was given.
func (pr pair) label1(opts options) string {
	if opts.labels[0] != "" {
		return opts.labels[0]
	}
	return pr.name1
}

// label2 returns the name of the second file of the pair as the messages
// write it: its -L label when one was given.
func (pr pair) label2(opts options) string {
	if opts.labels[1] != "" {
		return opts.labels[1]
	}
	return pr.name2
}

// writeChanges writes the differences in the output format the options
// picked.
func (p *Prog) writeChanges(pr pair, a, b []string, changes []diff.Change, opts options) error {
	lo := diff.LineOpts{
		ExpandTabs: opts.expandTabs,
		InitialTab: opts.initialTab,
		TabSize:    opts.tabSize,
	}
	var err error
	switch opts.format {
	case contextFormat, unifiedFormat:
		var f1, f2 diff.File
		f1, err = p.metaFile(pr, 1, opts)
		if err != nil {
			return err
		}
		f2, err = p.metaFile(pr, 2, opts)
		if err != nil {
			return err
		}
		if opts.format == contextFormat {
			err = diff.Context(p.Stdout, f1, f2, a, b, changes, opts.lines, lo)
		} else {
			err = diff.Unified(p.Stdout, f1, f2, a, b, changes, opts.lines, lo)
		}
	case edFormat:
		err = diff.Ed(p.Stdout, b, changes, lo)
	case forwardEdFormat:
		err = diff.ForwardEd(p.Stdout, b, changes, lo)
	case rcsFormat:
		err = diff.RCS(p.Stdout, b, changes, lo)
	case ifdefFormat:
		err = diff.Ifdef(p.Stdout, opts.ifdef, a, b, changes, lo)
	case sideFormat:
		err = diff.Side(p.Stdout, a, b, changes, diff.SideOpts{
			Width:          opts.width,
			TabSize:        opts.tabSize,
			ExpandTabs:     opts.expandTabs,
			LeftColumn:     opts.leftColumn,
			SuppressCommon: opts.suppressCommon,
		})
	default:
		err = diff.Normal(p.Stdout, a, b, changes, lo)
	}
	if err != nil {
		return &writeError{err: err}
	}
	return nil
}

// metaFile describes one side of a pair for the headers of the context and
// unified formats: its -L label when one was given, otherwise its name and
// the modification time of the file, which is the current time for the
// standard input and the epoch for a missing one.
func (p *Prog) metaFile(pr pair, side int, opts options) (diff.File, error) {
	name, path, label := pr.name1, pr.path1, opts.labels[0]
	if side == 2 {
		name, path, label = pr.name2, pr.path2, opts.labels[1]
	}
	f := diff.File{Name: name, Label: label, Time: time.Now()}
	if (side == 1 && pr.missing1) || (side == 2 && pr.missing2) {
		f.Time = time.Unix(0, 0)
		return f, nil
	}
	if path != "" {
		fi, err := fs.Stat(p.FS, path)
		if err != nil {
			return diff.File{}, err
		}
		f.Time = fi.ModTime()
	}
	return f, nil
}

// writeHeader writes the header of a pair compared under directories, when
// it has one and its files differ.
func (p *Prog) writeHeader(pr pair, opts options) error {
	if pr.header == "" {
		return nil
	}
	return p.write(pr.header + "\n")
}

// keyOpts returns the line comparison which the white space and case
// options ask for.
func (o options) keyOpts() diff.KeyOpts {
	return diff.KeyOpts{
		IgnoreCase:          o.ignoreCase,
		IgnoreAllSpace:      o.ignoreAllSpace,
		IgnoreSpaceChange:   o.blank,
		IgnoreTabExpansion:  o.ignoreTabExpansion,
		IgnoreTrailingSpace: o.ignoreTrailing,
		TabSize:             o.tabSize,
	}
}

// dropIgnored drops the changes whose lines are all blank or all matching
// the -I patterns, which the -B and -I options ask to ignore. The lines
// around a dropped change still come out, as context or common lines.
func (o options) dropIgnored(changes []diff.Change, a, b []string, key func(string) string) []diff.Change {
	if !o.blankLines && len(o.ignoreRes) == 0 {
		return changes
	}
	out := changes[:0]
	for _, c := range changes {
		trivial := true
		for i := c.A0; i < c.A1 && trivial; i++ {
			trivial = o.trivialLine(a[i])
		}
		for i := c.B0; i < c.B1 && trivial; i++ {
			trivial = o.trivialLine(b[i])
		}
		if !trivial {
			out = append(out, c)
		}
	}
	return out
}

// trivialLine reports whether a line counts as nothing for the -B and -I
// options: blank, or matching one of the patterns. A line counts as blank
// in whatever way the comparison of the lines already ignores: as an
// empty line when it ignores nothing.
func (o options) trivialLine(line string) bool {
	if len(o.ignoreRes) > 0 {
		content := strings.TrimSuffix(line, "\n")
		for _, re := range o.ignoreRes {
			if re.MatchString(content) {
				return true
			}
		}
	}
	return o.blankLines && o.keyOpts().Blank(line)
}

// loadExcludes reads the pattern files of the -X options and gathers all
// the exclude patterns, one pattern per line of each file.
func (p *Prog) loadExcludes(ctx context.Context, opts *options) error {
	opts.patterns = opts.excludes
	for _, name := range opts.excludeFrom {
		if p.FS == nil {
			return errNoFS
		}
		f, err := p.FS.Open(p.FsName(name))
		if err != nil {
			return &pathError{label: name, err: err}
		}
		data, err := readAll(ctx, f)
		f.Close()
		if err != nil {
			return &pathError{label: name, err: err}
		}
		for pattern := range strings.SplitSeq(string(data), "\n") {
			if pattern != "" {
				opts.patterns = append(opts.patterns, pattern)
			}
		}
	}
	return nil
}

// excluded reports whether a directory entry name matches any of the
// exclude patterns.
func (o options) excluded(name string) bool {
	for _, pattern := range o.patterns {
		if ok, err := path.Match(pattern, name); err == nil && ok {
			return true
		}
	}
	return false
}

// readFile reads the whole file at the path, or the standard input when the
// path is empty, and returns nothing for a missing file, which is compared
// as an empty one. Reads stop early when ctx is done.
func (p *Prog) readFile(ctx context.Context, path string, missing bool) ([]byte, error) {
	if missing {
		return nil, nil
	}
	var r io.Reader
	if path == "" {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		r = p.Stdin
	} else {
		f, err := p.FS.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	return readAll(ctx, r)
}

// readAll reads all of r, stopping with the context error once ctx is done,
// so long inputs don't delay returning.
func readAll(ctx context.Context, r io.Reader) ([]byte, error) {
	var out []byte
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// stripCR removes the carriage returns which end lines with a newline,
// as the --strip-trailing-cr option asks.
func stripCR(data []byte) []byte {
	if !bytes.Contains(data, []byte("\r\n")) {
		return data
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// isBinary reports whether data holds a NUL byte, in which case the line
// formats of diff can't represent it.
func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

// write writes s to the standard output, flagging write errors.
func (p *Prog) write(s string) error {
	if _, err := io.WriteString(p.Stdout, s); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// pathError marks an error as coming from reaching a file.
type pathError struct {
	label string
	err   error
}

// Error implements the error interface.
func (e *pathError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make pathError unwrapable.
func (e *pathError) Unwrap() error {
	return e.err
}

// writeError marks an error as coming from writing to the output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}
