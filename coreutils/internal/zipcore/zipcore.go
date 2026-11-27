// Package zipcore implements the ZIP plumbing shared by the zip and unzip
// utilities: opening archives through the runtime file system, the member
// naming both directions of the utilities need, the member patterns, the
// lines read from the standard input, and the deflate writer they share.
package zipcore

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/ctxio"
	"charm.land/turtle/fsx"
)

// DefaultLevel is the deflate level used when the -0..-9 options aren't
// given, matching the -6 default of the Info-ZIP zip utility.
const DefaultLevel = 6

// Archive is an open ZIP archive.
type Archive struct {
	*zip.Reader

	file io.Closer
}

// Close releases the file the archive reads from.
func (a *Archive) Close() error {
	return a.file.Close()
}

// Read opens the archive of the operand name through the file system of rt,
// reading it whole in memory when the file doesn't allow random access. The
// name is taken as given: the ".zip" fallback of the unzip operands belongs
// to the utility.
//
// The returned archive is closed by the caller: its ZIP reader keeps reading
// the file lazily, so it can't be closed here. A missing file reports
// fs.ErrNotExist, and anything the ZIP reader can't recognize reports
// zip.ErrFormat.
func Read(ctx context.Context, rt *coreutils.Runtime, name string) (*Archive, error) {
	if rt.FS == nil {
		return nil, errors.New("no file system available")
	}
	file, err := rt.FS.Open(rt.FsPath(name))
	if err != nil {
		return nil, err
	}
	if random, ok := file.(io.ReaderAt); ok {
		if fi, err := file.Stat(); err == nil {
			zr, err := zip.NewReader(random, fi.Size())
			if err != nil {
				file.Close()
				return nil, err
			}
			return &Archive{Reader: zr, file: file}, nil
		}
	}
	data, err := io.ReadAll(ctxio.NewReader(ctx, file))
	file.Close()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	return &Archive{Reader: zr, file: io.NopCloser(file)}, nil
}

// NewWriter returns a ZIP writer which deflates its entries at level, as the
// -0..-9 options ask. The entries themselves choose their method.
func NewWriter(w io.Writer, level int) *zip.Writer {
	writer := zip.NewWriter(w)
	writer.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, level)
	})
	return writer
}

// MkdirAll creates a directory and its missing parents, like mkdir -p.
func MkdirAll(fsys fsx.FS, name string, perm fs.FileMode) error {
	if name == "" || name == "." {
		return nil
	}
	fi, err := fsys.Lstat(name)
	if err == nil {
		if !fi.IsDir() {
			return errors.New("not a directory")
		}
		return nil
	}
	if err := MkdirAll(fsys, path.Dir(name), perm); err != nil {
		return err
	}
	if err := fsys.Mkdir(name, perm); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// MethodName names a compression method the way the verbose listing of unzip
// does.
func MethodName(method uint16) string {
	switch method {
	case zip.Store:
		return "Stored"
	case zip.Deflate:
		return "Defl:N"
	default:
		return "Unkn-"
	}
}

// DOSTime sets the MS-DOS time fields of a header from t, which the raw
// creation of an entry doesn't take from Modified.
func DOSTime(fh *zip.FileHeader, t time.Time) {
	year := max(t.Year(), 1980)
	fh.ModifiedDate = uint16(year-1980)<<9 | uint16(t.Month())<<5 | uint16(t.Day())
	fh.ModifiedTime = uint16(t.Hour())<<11 | uint16(t.Minute())<<5 | uint16(t.Second()/2)
}
