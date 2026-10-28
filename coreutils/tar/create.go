package tar

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/fsx"
)

// blockSize is the size of every block of an archive.
const blockSize = 512

// create writes a new archive holding the operands. An empty operand list is
// an error, like GNU's "cowardly" refusal.
func (a *archiver) create(ctx context.Context, operands []string) error {
	if len(operands) == 0 {
		a.errorf("Cowardly refusing to create an empty archive")
		return &coreutils.ExitError{Code: 2}
	}
	out, err := a.openWriter()
	if err != nil {
		a.fail(a.archiveName(), err)
		return a.exit(1)
	}
	cw := &countWriter{w: out}
	tw := tar.NewWriter(cw)
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			out.Close()
			return err
		}
		a.archiveEntry(ctx, tw, operand, a.resolve(operand), nil)
	}
	if err := ctx.Err(); err != nil {
		out.Close()
		return err
	}
	err = tw.Close()
	if padErr := a.padToRecord(cw); err == nil {
		err = padErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		a.fail(a.archiveName(), err)
	}
	return a.exit(1)
}

// append adds the operands to an existing archive, or with update set, only
// those of them newer than the copy already stored. The archive is rebuilt
// through a sibling temporary file, which is renamed over it at the end.
func (a *archiver) append(ctx context.Context, operands []string, update bool) error {
	if len(operands) == 0 {
		return nil
	}
	name := a.archiveName()
	if name == "-" {
		a.fail("", errors.New("cannot update an archive on a standard stream"))
		return a.exit(1)
	}
	if a.rt.FS == nil {
		a.fail("", errors.New("no file system available"))
		return a.exit(1)
	}

	var existing map[string]time.Time
	if update {
		var err error
		existing, err = a.scanEntries(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			a.fail(name, err)
			return a.exit(1)
		}
	}

	src, err := a.rt.FS.Open(a.resolve(name))
	if err != nil {
		a.fail(name, err)
		return a.exit(1)
	}
	tmpName := name + ".tmp"
	tmp, err := a.rt.FS.OpenFile(a.resolve(tmpName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^a.rt.Umask.Perm())
	if err != nil {
		src.Close()
		a.fail(tmpName, err)
		return a.exit(1)
	}

	cw := &countWriter{w: tmp}
	err = copyEntries(cw, src)
	src.Close()
	if err == nil {
		tw := tar.NewWriter(cw)
		for _, operand := range operands {
			if ctx.Err() != nil {
				break
			}
			a.archiveEntry(ctx, tw, operand, a.resolve(operand), existing)
		}
		err = tw.Close()
	}
	if padErr := a.padToRecord(cw); err == nil {
		err = padErr
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		a.rt.FS.Remove(a.resolve(tmpName))
		return ctxErr
	}
	if err != nil {
		a.rt.FS.Remove(a.resolve(tmpName))
		a.fail(name, err)
		return a.exit(1)
	}
	if err := a.rt.FS.Rename(a.resolve(tmpName), a.resolve(name)); err != nil {
		a.fail(name, err)
	}
	return a.exit(1)
}

// scanEntries maps the entry names of the archive to their modification
// times, for -u to compare against.
func (a *archiver) scanEntries(ctx context.Context) (map[string]time.Time, error) {
	in, err := a.openReader()
	if err != nil {
		return nil, err
	}
	defer in.Close()
	tr, err := newTarReader(in)
	if err != nil {
		return nil, err
	}
	entries := map[string]time.Time{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, errNotTar
		}
		entries[trimDir(hdr.Name)] = hdr.ModTime
	}
}

// failCreate reports an input failure while archiving: with -e the run is
// still a success, like GNU's --ignore-failed-read.
func (a *archiver) failCreate(name string, err error) {
	if a.opts.ignoreErrors {
		a.errorf("%s: %v", name, patherr.Unwrap(err))
		return
	}
	a.fail(name, err)
}

// archiveEntry writes a single operand to tw under name, recursing into
// directories. With existing set (update), entries which aren't newer than
// the stored copy are left out.
func (a *archiver) archiveEntry(ctx context.Context, tw *tar.Writer, name, resolved string, existing map[string]time.Time) {
	if err := ctx.Err(); err != nil {
		return
	}
	if a.rt.FS == nil {
		a.fail(name, errors.New("no file system available"))
		return
	}
	fi, err := a.rt.FS.Lstat(resolved)
	if a.opts.dereference {
		fi, err = fs.Stat(a.rt.FS, resolved)
	}
	if err != nil {
		a.failCreate(name, err)
		return
	}
	name = a.relativeName(cleanName(name))
	if fi.IsDir() {
		a.writeEntry(ctx, tw, name, resolved, fi, existing)
		children, err := fs.ReadDir(a.rt.FS, resolved)
		if err != nil {
			a.failCreate(name, err)
			return
		}
		for _, child := range children {
			a.archiveEntry(ctx, tw, name+"/"+child.Name(), resolved+"/"+child.Name(), existing)
		}
		return
	}
	a.writeEntry(ctx, tw, name, resolved, fi, existing)
}

// writeEntry writes the header of a single entry, and its contents for a
// regular file.
func (a *archiver) writeEntry(ctx context.Context, tw *tar.Writer, name, resolved string, fi fs.FileInfo, existing map[string]time.Time) {
	hdr, err := a.header(name, resolved, fi)
	if err != nil {
		a.failCreate(name, err)
		return
	}
	if existing != nil {
		if stored, ok := existing[trimDir(hdr.Name)]; ok && !fi.ModTime().After(stored) {
			return
		}
	}
	if a.opts.interactive && !a.confirm("add", hdr.Name) {
		return
	}
	if a.opts.verbose {
		a.errorf("%s", hdr.Name)
	}
	if err := tw.WriteHeader(hdr); err != nil {
		a.fail(name, err)
		return
	}
	if hdr.Typeflag != tar.TypeReg {
		return
	}
	in, err := a.rt.FS.Open(resolved)
	if err != nil {
		a.failCreate(name, err)
		return
	}
	_, copyErr := io.Copy(tw, ctxio.NewReader(ctx, in))
	closeErr := in.Close()
	if copyErr != nil {
		a.failCreate(name, copyErr)
	} else if closeErr != nil {
		a.failCreate(name, closeErr)
	}
}

// header builds the archive header of a file system entry. Entry types which
// an archive can't portably hold are rejected.
func (a *archiver) header(name, resolved string, fi fs.FileInfo) (*tar.Header, error) {
	hdr := &tar.Header{
		Name:    name,
		Mode:    headerMode(fi.Mode()),
		ModTime: fi.ModTime(),
	}
	if uid, gid, ok := fsx.Owner(fi); ok {
		hdr.Uid = uid
		hdr.Gid = gid
	}
	switch {
	case fi.IsDir():
		hdr.Typeflag = tar.TypeDir
		hdr.Name += "/"
	case fi.Mode()&fs.ModeSymlink != 0:
		target, err := a.rt.FS.ReadLink(resolved)
		if err != nil {
			return nil, err
		}
		hdr.Typeflag = tar.TypeSymlink
		hdr.Linkname = target
	case fi.Mode().IsRegular():
		hdr.Typeflag = tar.TypeReg
		hdr.Size = fi.Size()
	case fi.Mode()&fs.ModeNamedPipe != 0:
		hdr.Typeflag = tar.TypeFifo
	case fi.Mode()&fs.ModeSocket != 0:
		return nil, errors.New("socket ignored")
	default:
		return nil, errors.New("unsupported file type ignored")
	}
	return hdr, nil
}

// headerMode converts the permission and special bits of a file mode into
// the numeric mode of an archive header.
func headerMode(mode fs.FileMode) int64 {
	m := int64(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		m |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		m |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		m |= 0o1000
	}
	return m
}

// padToRecord writes zero blocks so the archive length becomes a multiple of
// the blocking factor.
func (a *archiver) padToRecord(cw *countWriter) error {
	record := int64(a.opts.blocking) * blockSize
	rem := cw.n % record
	if rem == 0 {
		return nil
	}
	zeros := make([]byte, blockSize)
	for left := record - rem; left > 0; left -= blockSize {
		n := min(left, int64(blockSize))
		if _, err := cw.Write(zeros[:n]); err != nil {
			return err
		}
	}
	return nil
}

// copyEntries copies the entries of an existing archive to dst as raw
// blocks, stopping at the end-of-archive marker so more entries can be
// appended.
func copyEntries(dst io.Writer, src io.Reader) error {
	block := make([]byte, blockSize)
	for {
		if _, err := io.ReadFull(src, block); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if isZeroBlock(block) {
			return nil
		}
		size, err := parseSize(block[124:136])
		if err != nil {
			return err
		}
		if _, err := dst.Write(block); err != nil {
			return err
		}
		if size == 0 {
			continue
		}
		data := (size + blockSize - 1) / blockSize * blockSize
		if _, err := io.CopyN(dst, src, data); err != nil {
			return err
		}
	}
}

// isZeroBlock reports whether a block is part of the end-of-archive marker.
func isZeroBlock(block []byte) bool {
	for _, b := range block {
		if b != 0 {
			return false
		}
	}
	return true
}

// parseSize parses the size field of a header block, held in octal or, for
// big files, in base-256 with the first bit set.
func parseSize(field []byte) (int64, error) {
	if field[0]&0o200 != 0 {
		var n int64
		for i, b := range field {
			if i == 0 {
				b &= 0o177
			}
			n = n<<8 | int64(b)
		}
		return n, nil
	}
	s := strings.Trim(string(field), "\x00 ")
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 8, 63)
	if err != nil {
		return 0, errNotTar
	}
	return n, nil
}

// trimDir drops the trailing slash of a directory entry name.
func trimDir(name string) string {
	return strings.TrimSuffix(name, "/")
}
