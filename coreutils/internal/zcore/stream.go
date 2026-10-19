package zcore

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// none is the format of input that isn't compressed, or that this package
// doesn't recognize.
const none Algorithm = -1

// source is an input whose format was sniffed from its leading bytes.
type source struct {
	br        *bufio.Reader
	algorithm Algorithm

	reader     *gzip.Reader
	storedName string
	storedMod  time.Time
	written    int64
}

// openSource buffers src, which must not be a buffered reader, and sniffs the
// format of its contents.
func openSource(src io.Reader) *source {
	s := &source{br: bufio.NewReader(src)}
	s.algorithm = sniff(s.br)
	return s
}

// open decodes the gzip contents of s, reading what its header stores.
func (s *source) open() error {
	if s.reader != nil {
		return nil
	}
	reader, err := gzip.NewReader(s.br)
	if err != nil {
		return err
	}
	s.reader = reader
	s.storedName, s.storedMod = reader.Name, reader.ModTime
	return nil
}

// decode writes the contents of s to dst, returning how many bytes were
// written. Input which isn't compressed is copied through only when allowed.
func (s *source) decode(ctx context.Context, dst io.Writer, passThrough bool) (int64, error) {
	switch s.algorithm {
	case none:
		if !passThrough {
			return 0, errNotCompressed
		}
		n, err := copyCtx(ctx, dst, s.br)
		s.written += n
		return n, err
	case Gzip:
	default:
		return 0, errUnsupported
	}
	if err := s.open(); err != nil {
		return 0, err
	}
	n, err := copyCtx(ctx, dst, s.reader)
	if cerr := s.reader.Close(); err == nil {
		err = cerr
	}
	s.written += n
	return n, err
}

// sniff reports the algorithm of the leading bytes of r. An input which is
// empty or doesn't start with a known magic number reports none.
func sniff(r *bufio.Reader) Algorithm {
	magic, err := r.Peek(2)
	if err != nil && !errors.Is(err, io.EOF) {
		return none
	}
	switch {
	case len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		return Gzip
	case len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x9d:
		return LZW
	}
	return none
}

// gzipWriter returns a writer for the given compression level, where a
// negative value means the default one.
func gzipWriter(dst io.Writer, level int) (*gzip.Writer, error) {
	if level < 0 {
		level = gzip.DefaultCompression
	}
	return gzip.NewWriterLevel(dst, level)
}

// decodeError turns a decoding failure into a concise message about the input.
func decodeError(err error) error {
	switch {
	case errors.Is(err, errNotCompressed),
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, gzip.ErrHeader),
		errors.Is(err, gzip.ErrChecksum),
		isCorrupt(err):
		return errNotCompressed
	}
	return err
}

// isCorrupt reports whether err comes from data which doesn't decompress.
func isCorrupt(err error) bool {
	var corrupt *flate.CorruptInputError
	return errors.As(err, &corrupt)
}

// storeName reports whether a name can be kept in the header of a gzip file,
// which stores it as a single NUL-terminated component.
func storeName(name string) bool {
	return name != "" && !strings.ContainsRune(name, 0) && len(name) <= nameMax
}

// copyCtx copies src to dst with a buffer, checking ctx before every block so
// a long transfer can be interrupted.
func copyCtx(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			total += int64(written)
			if werr != nil {
				return total, werr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

// countingWriter counts the bytes written through it.
type countingWriter struct {
	w io.Writer
	n int64
}

// Write implements io.Writer.
func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
