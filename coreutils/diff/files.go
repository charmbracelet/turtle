package diff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"charm.land/snail/coreutils/internal/diff"
)

// pair holds the two files of one comparison: their names as written to the
// output, their paths in the file system, empty for the standard input, and
// the header line written before their changes when compared under
// directories.
type pair struct {
	name1, name2 string
	path1, path2 string
	header       string
}

// compareFiles reads both files of the pair and writes their differences,
// reporting whether they differ.
func (p *Prog) compareFiles(ctx context.Context, pr pair, opts options) (bool, error) {
	data1, err := p.readFile(ctx, pr.path1)
	if err != nil {
		return false, &pathError{label: pr.name1, err: err}
	}
	data2, err := p.readFile(ctx, pr.path2)
	if err != nil {
		return false, &pathError{label: pr.name2, err: err}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if isBinary(data1) || isBinary(data2) {
		if bytes.Equal(data1, data2) {
			return false, nil
		}
		if err := p.writeHeader(pr); err != nil {
			return false, err
		}
		if err := p.write(fmt.Sprintf("Binary files %s and %s differ\n", pr.name1, pr.name2)); err != nil {
			return false, err
		}
		return true, nil
	}

	if bytes.Equal(data1, data2) {
		return false, nil
	}
	a, b := diff.SplitLines(data1), diff.SplitLines(data2)
	var key func(string) string
	if opts.blank {
		key = diff.BlankKey
	}
	changes := diff.Lines(a, b, key)
	if len(changes) == 0 {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := p.writeHeader(pr); err != nil {
		return false, err
	}
	if err := p.writeChanges(pr, a, b, changes, opts); err != nil {
		return false, err
	}
	return true, nil
}

// writeChanges writes the differences in the output format the options
// picked.
func (p *Prog) writeChanges(pr pair, a, b []string, changes []diff.Change, opts options) error {
	var err error
	switch opts.format {
	case contextFormat, unifiedFormat:
		var f1, f2 diff.File
		f1, err = p.metaFile(pr.name1, pr.path1)
		if err != nil {
			return &pathError{label: pr.name1, err: err}
		}
		f2, err = p.metaFile(pr.name2, pr.path2)
		if err != nil {
			return &pathError{label: pr.name2, err: err}
		}
		if opts.format == contextFormat {
			err = diff.Context(p.Stdout, f1, f2, a, b, changes, opts.lines)
		} else {
			err = diff.Unified(p.Stdout, f1, f2, a, b, changes, opts.lines)
		}
	case edFormat:
		err = diff.Ed(p.Stdout, b, changes)
	case forwardEdFormat:
		err = diff.ForwardEd(p.Stdout, b, changes)
	default:
		err = diff.Normal(p.Stdout, a, b, changes)
	}
	if err != nil {
		return &writeError{err: err}
	}
	return nil
}

// metaFile describes one side of a pair for the headers of the context and
// unified formats, holding the modification time of the file, or the
// current time for the standard input.
func (p *Prog) metaFile(name, path string) (diff.File, error) {
	f := diff.File{Name: name, Time: time.Now()}
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
func (p *Prog) writeHeader(pr pair) error {
	if pr.header == "" {
		return nil
	}
	return p.write(pr.header + "\n")
}

// readFile reads the whole file at the path, or the standard input when the
// path is empty. Reads stop early when ctx is done.
func (p *Prog) readFile(ctx context.Context, path string) ([]byte, error) {
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
