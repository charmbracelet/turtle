package dd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"

	"charm.land/turtle/fsx"
)

// plainReader hides the Seek method of the reader it wraps, standing for a
// pipe or another non-seekable input.
type plainReader struct {
	r io.Reader
}

func (p plainReader) Read(b []byte) (int, error) { return p.r.Read(b) }

// shortReader returns at most two bytes per read, like a pipe that keeps
// giving short reads.
type shortReader struct {
	r io.Reader
}

func (s shortReader) Read(b []byte) (int, error) {
	if len(b) > 2 {
		b = b[:2]
	}
	return s.r.Read(b)
}

// errorReader yields its data, then one read error, then the end of file.
type errorReader struct {
	data   string
	failed bool
	err    error
}

func (r *errorReader) Read(b []byte) (int, error) {
	if r.data != "" {
		n := copy(b, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	if !r.failed {
		r.failed = true
		return 0, r.err
	}
	return 0, io.EOF
}

// errAtReader reads its data, failing once when a read starts at a given
// offset: the failing read returns partial bytes along with the error, like
// device reads can. Its offset can be seeked.
type errAtReader struct {
	data    string
	errAt   int
	partial int
	err     error
	failed  bool
	pos     int
}

func (r *errAtReader) Read(p []byte) (int, error) {
	if !r.failed && r.pos == r.errAt {
		r.failed = true
		n := copy(p, r.data[r.pos:r.pos+r.partial])
		r.pos += n
		return n, r.err
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	// Reads stop at the offset the next one fails on.
	end := len(r.data)
	if !r.failed && r.pos < r.errAt && end > r.errAt {
		end = r.errAt
	}
	n := copy(p, r.data[r.pos:end])
	r.pos += n
	return n, nil
}

// Seek implements io.Seeker, so error recovery can skip past a bad block.
func (r *errAtReader) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart {
		return 0, errors.New("unsupported whence")
	}
	r.pos = int(offset)
	return offset, nil
}

// ddCase is the fields of a testutil.Case plus the input files as the plain
// texts they hold: dd opens them through the file system, so they can't be
// the standard input.
type ddCase struct {
	Name      string
	Stdin     io.Reader
	Args      []string
	Want      string
	WantErr   string
	WantExit  int
	WantFiles map[string]string

	Files map[string]string
}

func runCase(t *testing.T, test ddCase) {
	t.Helper()

	fsys := fsx.MemoryFS{}
	for name, data := range test.Files {
		fsys[name] = &fsx.MemoryFile{Data: []byte(data), Mode: 0o666}
	}
	stdin := test.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	testutil.Once(t, New, testutil.Case{
		Name:      test.Name,
		Stdin:     stdin,
		FS:        fsys,
		Args:      test.Args,
		Want:      test.Want,
		WantErr:   test.WantErr,
		WantExit:  test.WantExit,
		WantFiles: test.WantFiles,
	})
}

func TestRunCopy(t *testing.T) {
	tests := []ddCase{
		{
			Name:    "the standard input is copied whole",
			Stdin:   strings.NewReader("hello"),
			Want:    "hello",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "an empty input copies nothing",
			Stdin:   strings.NewReader(""),
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:    "short input blocks are aggregated into output ones",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=3", "obs=7"},
			Want:    "0123456789",
			WantErr: "3+1 records in\n1+1 records out\n",
		},
		{
			Name:    "bs makes every input block a single output one",
			Stdin:   strings.NewReader("abcde"),
			Args:    []string{"bs=2"},
			Want:    "abcde",
			WantErr: "2+1 records in\n2+1 records out\n",
		},
		{
			Name:    "bs supersedes ibs and obs",
			Stdin:   strings.NewReader("abcdef"),
			Args:    []string{"ibs=1", "obs=1", "bs=4"},
			Want:    "abcdef",
			WantErr: "1+1 records in\n1+1 records out\n",
		},
		{
			Name:    "count copies only the first blocks",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=3", "count=2"},
			Want:    "012345",
			WantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			Name:    "count of zero copies nothing",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"count=0"},
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:    "count in bytes, as a trailing B asks",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=4", "count=7B"},
			Want:    "0123456",
			WantErr: "1+1 records in\n0+1 records out\n",
		},
		{
			Name:    "skip drops the first input blocks",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=3", "skip=2"},
			Want:    "6789",
			WantErr: "1+1 records in\n0+1 records out\n",
		},
		{
			Name:    "skip in bytes, as a trailing B asks",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=4", "skip=2B"},
			Want:    "23456789",
			WantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			Name:    "iseek is another name for skip",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=3", "iseek=1"},
			Want:    "3456789",
			WantErr: "2+1 records in\n0+1 records out\n",
		},
		{
			Name:    "skip is read and discarded on a non-seekable input",
			Stdin:   plainReader{strings.NewReader("0123456789")},
			Args:    []string{"ibs=3", "skip=2"},
			Want:    "6789",
			WantErr: "1+1 records in\n0+1 records out\n",
		},
		{
			Name:  "skipping past the end of the input warns",
			Stdin: plainReader{strings.NewReader("abc")},
			Args:  []string{"ibs=2", "skip=5"},
			WantErr: "dd: 'standard input': cannot skip to specified offset\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "status of none prints nothing",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"status=none"},
			Want:  "hello",
		},
		{
			Name:    "status of noxfer keeps the records lines",
			Stdin:   strings.NewReader("hello"),
			Args:    []string{"status=noxfer"},
			Want:    "hello",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "the last status level wins",
			Stdin:   strings.NewReader("hello"),
			Args:    []string{"status=none", "status=progress"},
			Want:    "hello",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "a seek on a non-seekable output is filled with nulls",
			Stdin:   strings.NewReader("ab"),
			Args:    []string{"seek=2", "obs=2"},
			Want:    "\x00\x00\x00\x00ab",
			WantErr: "0+1 records in\n1+0 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunConversions(t *testing.T) {
	tests := []ddCase{
		{
			Name:    "ucase",
			Stdin:   strings.NewReader("Hello, World!"),
			Args:    []string{"conv=ucase"},
			Want:    "HELLO, WORLD!",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "lcase",
			Stdin:   strings.NewReader("Hello, World!"),
			Args:    []string{"conv=lcase"},
			Want:    "hello, world!",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "swab pairs every two bytes",
			Stdin:   strings.NewReader("abcde"),
			Args:    []string{"ibs=2", "obs=16", "conv=swab"},
			Want:    "badce",
			WantErr: "2+1 records in\n0+1 records out\n",
		},
		{
			Name:    "swab keeps the last byte of an even output whole",
			Stdin:   strings.NewReader("abcd"),
			Args:    []string{"ibs=2", "obs=16", "conv=swab"},
			Want:    "badc",
			WantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			Name:    "swab carries the odd byte across input blocks",
			Stdin:   strings.NewReader("abcdefg"),
			Args:    []string{"ibs=3", "obs=16", "conv=swab"},
			Want:    "badcfeg",
			WantErr: "2+1 records in\n0+1 records out\n",
		},
		{
			Name:    "sync pads the input block with nulls",
			Stdin:   strings.NewReader("ab"),
			Args:    []string{"ibs=4", "obs=8", "conv=sync"},
			Want:    "ab\x00\x00",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "block pads short records with spaces",
			Stdin:   strings.NewReader("ab\ncd\n"),
			Args:    []string{"cbs=3", "obs=8", "conv=block"},
			Want:    "ab cd ",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "block truncates long records and counts one",
			Stdin:   strings.NewReader("abcd\n"),
			Args:    []string{"cbs=3", "obs=8", "conv=block"},
			Want:    "abc",
			WantErr: "0+1 records in\n0+1 records out\n1 truncated record\n",
		},
		{
			Name:    "block truncates long records and counts many",
			Stdin:   strings.NewReader("abcd\nabcd\n"),
			Args:    []string{"cbs=3", "obs=8", "conv=block"},
			Want:    "abcabc",
			WantErr: "0+1 records in\n0+1 records out\n2 truncated records\n",
		},
		{
			Name:    "block counts one truncated record however long",
			Stdin:   strings.NewReader("abcdef\n"),
			Args:    []string{"cbs=3", "obs=8", "conv=block"},
			Want:    "abc",
			WantErr: "0+1 records in\n0+1 records out\n1 truncated record\n",
		},
		{
			Name:    "block counts a record truncated across input blocks",
			Stdin:   strings.NewReader("abcd\n"),
			Args:    []string{"ibs=2", "cbs=3", "obs=8", "conv=block"},
			Want:    "abc",
			WantErr: "2+1 records in\n0+1 records out\n1 truncated record\n",
		},
		{
			Name:    "block pads the record cut by the end of the input",
			Stdin:   strings.NewReader("ab"),
			Args:    []string{"cbs=3", "obs=8", "conv=block"},
			Want:    "ab ",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "block spans input block boundaries",
			Stdin:   strings.NewReader("ab\ncd\n"),
			Args:    []string{"ibs=3", "cbs=4", "obs=8", "conv=block"},
			Want:    "ab  cd  ",
			WantErr: "2+0 records in\n1+0 records out\n",
		},
		{
			Name:    "unblock replaces the trailing spaces with newlines",
			Stdin:   strings.NewReader("ab cd x"),
			Args:    []string{"cbs=3", "obs=8", "conv=unblock"},
			Want:    "ab\ncd\nx\n",
			WantErr: "0+1 records in\n1+0 records out\n",
		},
		{
			Name:    "unblock keeps the spaces which are not trailing",
			Stdin:   strings.NewReader("a b c d "),
			Args:    []string{"cbs=2", "obs=8", "conv=unblock"},
			Want:    "a\nb\nc\nd\n",
			WantErr: "0+1 records in\n1+0 records out\n",
		},
		{
			Name:    "unblock drops the spaces held back at the end",
			Stdin:   strings.NewReader("a  b"),
			Args:    []string{"cbs=3", "obs=8", "conv=unblock"},
			Want:    "a\nb\n",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "ascii converts and unblocks",
			Stdin:   strings.NewReader("\xc1\xc2\x40"),
			Args:    []string{"cbs=3", "obs=8", "conv=ascii"},
			Want:    "AB\n",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "ebcdic converts and blocks",
			Stdin:   strings.NewReader("AB\n"),
			Args:    []string{"cbs=3", "obs=8", "conv=ebcdic"},
			Want:    "\xc1\xc2\x40",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "ascii converts the newlines the tables define",
			Stdin:   strings.NewReader("\x25\x25"),
			Args:    []string{"obs=8", "conv=ascii"},
			Want:    "\n\n",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "ibm converts to the alternate table",
			Stdin:   strings.NewReader("~"),
			Args:    []string{"obs=8", "conv=ibm"},
			Want:    "\xa1",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "ebcdic and ascii are the inverse of each other",
			Stdin:   strings.NewReader("~[a\n"),
			Args:    []string{"obs=8", "conv=ebcdic"},
			Want:    "\x5f\xad\x81\x25",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "ascii and ucase compose",
			Stdin:   strings.NewReader("\x81\x82\x40"),
			Args:    []string{"cbs=3", "obs=8", "conv=ascii,ucase"},
			Want:    "AB\n",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "sync with block pads with spaces",
			Stdin:   strings.NewReader("ab"),
			Args:    []string{"ibs=4", "cbs=4", "obs=8", "conv=block,sync"},
			Want:    "ab  ",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "bs with a data conversion still aggregates",
			Stdin:   strings.NewReader("abcde"),
			Args:    []string{"bs=4", "conv=swab"},
			Want:    "badce",
			WantErr: "1+1 records in\n1+1 records out\n",
		},
		{
			Name:    "ascii and swab compose",
			Stdin:   strings.NewReader("\xc1\x82\xc3"),
			Args:    []string{"obs=8", "conv=ascii,swab"},
			Want:    "bAC",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []ddCase{
		{
			Name:  "if and of copy between files",
			Stdin: strings.NewReader("ignored"),
			Args:  []string{"if=in.txt", "of=out.txt"},
			Files: map[string]string{"in.txt": "hello"},
			WantFiles: map[string]string{
				"out.txt": "hello",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:  "of creates the output file",
			Stdin: strings.NewReader("hi"),
			Args:  []string{"of=out.txt"},
			WantFiles: map[string]string{
				"out.txt": "hi",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:  "of truncates the output file",
			Stdin: strings.NewReader("hi"),
			Args:  []string{"of=out.txt"},
			Files: map[string]string{"out.txt": "OLD CONTENT"},
			WantFiles: map[string]string{
				"out.txt": "hi",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:  "notrunc keeps the rest of the output file",
			Stdin: strings.NewReader("hi"),
			Args:  []string{"of=out.txt", "conv=notrunc"},
			Files: map[string]string{"out.txt": "OLDOLDOLD"},
			WantFiles: map[string]string{
				"out.txt": "hiDOLDOLD",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:  "seek writes past the preserved blocks",
			Stdin: strings.NewReader("XY"),
			Args:  []string{"of=out.txt", "seek=1", "obs=4"},
			Files: map[string]string{"out.txt": "0123456789"},
			WantFiles: map[string]string{
				"out.txt": "0123XY",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:  "seek with notrunc keeps the tail of the output file",
			Stdin: strings.NewReader("XY"),
			Args:  []string{"of=out.txt", "seek=1", "obs=4", "conv=notrunc"},
			Files: map[string]string{"out.txt": "0123456789"},
			WantFiles: map[string]string{
				"out.txt": "0123XY6789",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:  "seek past the end of the output file keeps its size",
			Stdin: strings.NewReader(""),
			Args:  []string{"of=out.txt", "seek=2", "obs=4"},
			Files: map[string]string{"out.txt": "01234567"},
			WantFiles: map[string]string{
				"out.txt": "01234567",
			},
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "seek with count of zero still shortens the file",
			Stdin: strings.NewReader("ignored"),
			Args:  []string{"of=out.txt", "seek=1", "obs=4", "count=0"},
			Files: map[string]string{"out.txt": "0123456789"},
			WantFiles: map[string]string{
				"out.txt": "0123",
			},
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "a seek with an empty input extends a new output file",
			Stdin: strings.NewReader(""),
			Args:  []string{"of=out.txt", "seek=2", "obs=4"},
			WantFiles: map[string]string{
				"out.txt": "\x00\x00\x00\x00\x00\x00\x00\x00",
			},
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "a seek with an empty input extends a shorter output file",
			Stdin: strings.NewReader(""),
			Args:  []string{"of=out.txt", "seek=2", "obs=4"},
			Files: map[string]string{"out.txt": "abcd"},
			WantFiles: map[string]string{
				"out.txt": "abcd\x00\x00\x00\x00",
			},
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "a seek past the end keeps the output size with notrunc",
			Stdin: strings.NewReader(""),
			Args:  []string{"of=out.txt", "seek=2", "obs=4", "conv=notrunc"},
			Files: map[string]string{"out.txt": "ab"},
			WantFiles: map[string]string{
				"out.txt": "ab",
			},
			WantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "oseek is another name for seek",
			Stdin: strings.NewReader("AB"),
			Args:  []string{"of=out.txt", "oseek=2", "obs=1"},
			WantFiles: map[string]string{
				"out.txt": "\x00\x00AB",
			},
			WantErr: "0+1 records in\n2+0 records out\n",
		},
		{
			Name:    "skip seeks the input file",
			Stdin:   strings.NewReader("ignored"),
			Args:    []string{"if=in.txt", "ibs=2", "skip=2"},
			Files:   map[string]string{"in.txt": "0123456789"},
			Want:    "456789",
			WantErr: "3+0 records in\n0+1 records out\n",
		},
		{
			Name:     "a missing input file fails",
			Args:     []string{"if=no.txt"},
			WantExit: 1,
			WantErr:  "dd: failed to open 'no.txt': file does not exist\n",
		},
		{
			Name:     "excl fails when the output file exists",
			Stdin:    strings.NewReader("hi"),
			Args:     []string{"of=out.txt", "conv=excl"},
			Files:    map[string]string{"out.txt": "old"},
			WantExit: 1,
			WantErr:  "dd: failed to open 'out.txt': file already exists\n",
		},
		{
			Name:     "nocreat fails when the output file is missing",
			Stdin:    strings.NewReader("hi"),
			Args:     []string{"of=out.txt", "conv=nocreat"},
			WantExit: 1,
			WantErr:  "dd: failed to open 'out.txt': file does not exist\n",
		},
		{
			Name:  "fsync flushes the output file",
			Stdin: strings.NewReader("hi"),
			Args:  []string{"of=out.txt", "conv=fsync"},
			WantFiles: map[string]string{
				"out.txt": "hi",
			},
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "operands are taken in any order",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"count=2", "ibs=3", "if=in.txt", "skip=1"},
			Files:   map[string]string{"in.txt": "0123456789"},
			Want:    "345678",
			WantErr: "2+0 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSizes(t *testing.T) {
	tests := []ddCase{
		{
			Name:    "a bare number counts bytes",
			Stdin:   strings.NewReader("01234567"),
			Args:    []string{"ibs=4"},
			Want:    "01234567",
			WantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			Name:    "b multiplies by 512",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=1b", "count=1"},
			Want:    "0123456789",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "k multiplies by 1024",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=1k", "obs=1k", "count=1"},
			Want:    "0123456789",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "x multiplies the numbers it separates",
			Stdin:   strings.NewReader("0123456"),
			Args:    []string{"bs=2x2"},
			Want:    "0123456",
			WantErr: "1+1 records in\n1+1 records out\n",
		},
		{
			Name:    "KiB is the binary k",
			Stdin:   strings.NewReader("0123456789"),
			Args:    []string{"ibs=1KiB", "count=1"},
			Want:    "0123456789",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "KB is the decimal k",
			Stdin:   strings.NewReader(strings.Repeat("a", 999)),
			Args:    []string{"ibs=1KB", "count=1"},
			Want:    strings.Repeat("a", 999),
			WantErr: "0+1 records in\n1+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFullblock(t *testing.T) {
	tests := []ddCase{
		{
			Name:    "short reads count as one block",
			Stdin:   shortReader{strings.NewReader("01234567")},
			Args:    []string{"ibs=4", "count=1"},
			Want:    "01",
			WantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			Name:    "fullblock accumulates the whole block",
			Stdin:   shortReader{strings.NewReader("01234567")},
			Args:    []string{"ibs=4", "count=1", "iflag=fullblock"},
			Want:    "0123",
			WantErr: "1+0 records in\n0+1 records out\n",
		},
		{
			Name:    "fullblock without count copies everything",
			Stdin:   shortReader{strings.NewReader("01234567")},
			Args:    []string{"ibs=4", "iflag=fullblock"},
			Want:    "01234567",
			WantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			Name:    "fullblock reads the count in bytes tail",
			Stdin:   shortReader{strings.NewReader("01234567")},
			Args:    []string{"ibs=4", "count=7B", "iflag=fullblock"},
			Want:    "0123456",
			WantErr: "1+1 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		Name     string
		Stdin    io.Reader
		Args     []string
		Want     string
		WantErr  string
		WantExit int
	}{
		{
			Name:     "a read error stops the copy",
			Stdin:    &errorReader{data: "ab", err: errors.New("read failure")},
			Want:     "ab",
			WantErr:  "dd: error reading 'standard input': read failure\n0+1 records in\n0+1 records out\n",
			WantExit: 1,
		},
		{
			Name:  "noerror stops on an input it can't reposition",
			Stdin: &errorReader{data: "ab", err: errors.New("read failure")},
			Args:  []string{"conv=noerror"},
			Want:  "ab",
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+1 records in\n0+0 records out\n" +
				"0+1 records in\n0+1 records out\n",
			WantExit: 1,
		},
		{
			Name:  "noerror and sync stop without the zeroed block",
			Stdin: &errorReader{err: errors.New("read failure")},
			Args:  []string{"ibs=4", "conv=noerror,sync"},
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"0+0 records in\n0+0 records out\n",
			WantExit: 1,
		},
		{
			Name:     "noerror stops silently with status of none",
			Stdin:    &errorReader{data: "ab", err: errors.New("read failure")},
			Args:     []string{"conv=noerror", "status=none"},
			Want:     "ab",
			WantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, ddCase{
				Name:     test.Name,
				Stdin:    test.Stdin,
				Args:     test.Args,
				Want:     test.Want,
				WantErr:  test.WantErr,
				WantExit: test.WantExit,
			})
		})
	}
}

// stuckReader fails every read and rejects every seek, like a device that
// keeps erroring and can't be repositioned.
type stuckReader struct {
	err error
}

func (r *stuckReader) Read([]byte) (int, error) { return 0, r.err }

func (r *stuckReader) Seek(int64, int) (int64, error) {
	return 0, errors.New("cannot seek")
}

// dyingReader yields its data, then fails every read and rejects every seek,
// like a device that goes bad mid-copy and can't be repositioned.
type dyingReader struct {
	data string
	err  error
}

func (r *dyingReader) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *dyingReader) Seek(int64, int) (int64, error) {
	return 0, errors.New("cannot seek")
}

func TestRunReadErrorRecovery(t *testing.T) {
	readFailure := errors.New("read failure")
	tests := []ddCase{
		{
			Name:  "noerror skips the rest of the failed block only",
			Stdin: &errAtReader{data: "ABCDEFGHIJKLMNOPQRST", errAt: 11, partial: 1, err: readFailure},
			Args:  []string{"ibs=8", "obs=16", "conv=noerror"},
			// The failing read starts at 11 and intends 8 bytes, so the copy
			// resumes at 19 and not at 16.
			Want: "ABCDEFGHIJKT",
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"1+1 records in\n0+0 records out\n" +
				"1+2 records in\n0+1 records out\n",
		},
		{
			Name:  "noerror and sync zero the failed block after a short one",
			Stdin: &errAtReader{data: "ABCDEFGHIJK", errAt: 11, partial: 0, err: readFailure},
			Args:  []string{"ibs=8", "obs=24", "conv=noerror,sync"},
			Want:  "ABCDEFGH" + "IJK\x00\x00\x00\x00\x00" + "\x00\x00\x00\x00\x00\x00\x00\x00",
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"1+1 records in\n0+0 records out\n" +
				"1+2 records in\n1+0 records out\n",
		},
		{
			Name:  "noerror and sync keep the data a failed read returned",
			Stdin: &errAtReader{data: "ABCDEF", errAt: 0, partial: 2, err: readFailure},
			Args:  []string{"ibs=4", "obs=8", "conv=noerror,sync"},
			Want:  "AB\x00\x00EF\x00\x00",
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"0+2 records in\n1+0 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStuckInput(t *testing.T) {
	readFailure := errors.New("read failure")
	tests := []ddCase{
		{
			Name:     "a rejecting input stops the copy under noerror",
			Stdin:    &stuckReader{err: readFailure},
			Args:     []string{"ibs=4", "obs=8", "conv=noerror"},
			WantExit: 1,
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot seek\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			Name:     "a rejecting input stops the copy under noerror and sync",
			Stdin:    &stuckReader{err: readFailure},
			Args:     []string{"ibs=4", "obs=8", "conv=noerror,sync"},
			WantExit: 1,
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot seek\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			Name:     "the data copied before the input went bad is kept",
			Stdin:    &dyingReader{data: "ab", err: readFailure},
			Args:     []string{"ibs=4", "obs=8", "conv=noerror"},
			Want:     "ab",
			WantExit: 1,
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+1 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot seek\n" +
				"0+1 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSkipError(t *testing.T) {
	readFailure := errors.New("read failure")
	tests := []ddCase{
		{
			Name:     "a read error while skipping stops the copy",
			Stdin:    plainReader{&errAtReader{data: "ABCD", errAt: 0, partial: 0, err: readFailure}},
			Args:     []string{"skip=2", "ibs=2", "obs=2"},
			WantExit: 1,
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			Name:  "noerror keeps skipping after a read error",
			Stdin: plainReader{&errAtReader{data: "ABCD", errAt: 0, partial: 0, err: readFailure}},
			Args:  []string{"skip=2", "ibs=2", "obs=2", "conv=noerror"},
			Want:  "ABCD",
			WantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot skip to specified offset\n" +
				"2+0 records in\n2+0 records out\n",
		},
		{
			Name:    "a seekable input is skipped without reading it",
			Stdin:   &errAtReader{data: "ABCD", errAt: 0, partial: 0, err: readFailure},
			Args:    []string{"skip=2", "ibs=2", "obs=2", "conv=noerror"},
			Want:    "",
			WantErr: "0+0 records in\n0+0 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInvalidOperands(t *testing.T) {
	tests := []struct {
		Name    string
		Args    []string
		WantErr string
	}{
		{
			Name:    "an operand without a value",
			Args:    []string{"foo"},
			WantErr: "dd: unrecognized operand 'foo'\n",
		},
		{
			Name:    "an unknown operand name",
			Args:    []string{"foo=bar"},
			WantErr: "dd: unrecognized operand 'foo=bar'\n",
		},
		{
			Name:    "an invalid number",
			Args:    []string{"ibs=abc"},
			WantErr: "dd: invalid number 'abc'\n",
		},
		{
			Name:    "a zero block size",
			Args:    []string{"ibs=0"},
			WantErr: "dd: invalid number '0'\n",
		},
		{
			Name:    "an invalid count",
			Args:    []string{"count=x2"},
			WantErr: "dd: invalid number 'x2'\n",
		},
		{
			Name:    "an invalid conversion",
			Args:    []string{"conv=ascii,foo"},
			WantErr: "dd: invalid conversion 'foo'\n",
		},
		{
			Name:    "an invalid status level",
			Args:    []string{"status=quiet"},
			WantErr: "dd: invalid status level 'quiet'\n",
		},
		{
			Name:    "an invalid input flag",
			Args:    []string{"iflag=direct"},
			WantErr: "dd: invalid input flag 'direct'\n",
		},
		{
			Name:    "an invalid output flag",
			Args:    []string{"oflag=fullblock"},
			WantErr: "dd: invalid output flag 'fullblock'\n",
		},
		{
			Name:    "two character set conversions",
			Args:    []string{"conv=ascii,ebcdic", "cbs=1"},
			WantErr: "dd: cannot combine any two of ascii, ebcdic and ibm\n",
		},
		{
			Name:    "block and unblock",
			Args:    []string{"conv=block,unblock", "cbs=1"},
			WantErr: "dd: cannot combine block and unblock\n",
		},
		{
			Name:    "lcase and ucase",
			Args:    []string{"conv=lcase,ucase"},
			WantErr: "dd: cannot combine lcase and ucase\n",
		},
		{
			Name:    "excl and nocreat",
			Args:    []string{"conv=excl,nocreat"},
			WantErr: "dd: cannot combine excl and nocreat\n",
		},
		{
			Name:    "an unknown option",
			Args:    []string{"--foo"},
			WantErr: "dd: unrecognized option '--foo'\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, ddCase{
				Name:     test.Name,
				Args:     test.Args,
				WantExit: 1,
				WantErr:  test.WantErr + "Try 'dd --help' for more information.\n",
			})
		})
	}
}

func TestRunStandardOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "help",
			Args: []string{"--help"},
			Want: usage,
		},
		{
			Name: "version",
			Args: []string{"--version"},
			Want: fmt.Sprintf("dd (charm.land/turtle/coreutils) %s\nhttps://charm.land/turtle\n",
				coreutils.Version),
		},
		{
			Name:     "options are only taken before the operands",
			Args:     []string{"if=x", "--help"},
			WantExit: 1,
			WantErr:  "dd: unrecognized operand '--help'\nTry 'dd --help' for more information.\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  &testutil.CancelReader{Cancel: cancel},
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(ctx, "bs=1")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunProgress(t *testing.T) {
	base := time.Unix(0, 0)
	times := []time.Time{base, base, base.Add(2 * time.Second)}
	var i int
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("abc"),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		Clock: func() time.Time {
			if i < len(times) {
				t := times[i]
				i++
				return t
			}
			return times[len(times)-1]
		},
	}
	err := New(rt).Run(t.Context(), "ibs=1", "status=progress")
	if code := testutil.ExitCode(t, err); code != 0 {
		t.Fatalf("Run() exit code = %d, want 0 (error = %v)", code, err)
	}
	want := "1+0 records in\n0+0 records out\n" +
		"3+0 records in\n0+1 records out\n"
	if got := rt.Stderr.(*bytes.Buffer).String(); got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestParseExpr(t *testing.T) {
	tests := []struct {
		expr    string
		want    int64
		inBytes bool
		wantErr bool
	}{
		{expr: "512", want: 512},
		{expr: "1", want: 1},
		{expr: "0", want: 0},
		{expr: "1b", want: 512},
		{expr: "1c", want: 1},
		{expr: "2w", want: 4},
		{expr: "1k", want: 1024},
		{expr: "1K", want: 1024},
		{expr: "1kB", want: 1000, inBytes: true},
		{expr: "1KB", want: 1000, inBytes: true},
		{expr: "1KiB", want: 1024, inBytes: true},
		{expr: "1M", want: 1024 * 1024},
		{expr: "1G", want: 1024 * 1024 * 1024},
		{expr: "2x3", want: 6},
		{expr: "1x1k", want: 1024},
		{expr: "2x2x2", want: 8},
		{expr: "1B", want: 1, inBytes: true},
		{expr: "10B", want: 10, inBytes: true},
		{expr: "", wantErr: true},
		{expr: "x", wantErr: true},
		{expr: "1x", wantErr: true},
		{expr: "x2", wantErr: true},
		{expr: "abc", wantErr: true},
		{expr: "-1", wantErr: true},
		{expr: "1z", wantErr: true},
		{expr: "1q", wantErr: true},
		{expr: "1X2", wantErr: true},
		{expr: "1kbb", wantErr: true},
		{expr: "1Ki", wantErr: true},
		{expr: "999999999999999999999999", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			n, inBytes, err := parseExpr(test.expr)
			if test.wantErr {
				if err == nil {
					t.Errorf("parseExpr(%q) = %d, want an error", test.expr, n)
				}
				return
			}
			if err != nil {
				t.Errorf("parseExpr(%q) error = %v", test.expr, err)
				return
			}
			if n != test.want || inBytes != test.inBytes {
				t.Errorf("parseExpr(%q) = %d, %v, want %d, %v",
					test.expr, n, inBytes, test.want, test.inBytes)
			}
		})
	}
}

func TestConversionTables(t *testing.T) {
	for b := range 256 {
		if got := ebcdicToASCII[asciiToEbcdic[b]]; got != byte(b) {
			t.Fatalf("ascii %d converts to EBCDIC %#x, which converts back to %#x", b, asciiToEbcdic[b], got)
		}
	}
	// The values the POSIX tables call out: LF, '~' and '[' in both EBCDIC
	// sets, and the EBCDIC letters.
	if asciiToEbcdic['\n'] != 0x25 || ebcdicToASCII[0x25] != '\n' {
		t.Errorf("LF converts through %#x, want 0x25", asciiToEbcdic['\n'])
	}
	if asciiToEbcdic['~'] != 0x5f || asciiToIBM['~'] != 0xa1 {
		t.Errorf("'~' converts to %#x and %#x, want 0x5f and 0xa1",
			asciiToEbcdic['~'], asciiToIBM['~'])
	}
	if asciiToEbcdic['['] != asciiToIBM['['] {
		t.Errorf("'[' differs between the EBCDIC tables")
	}
	if asciiToEbcdic['A'] != 0xc1 || ebcdicToASCII[0xc1] != 'A' {
		t.Errorf("'A' converts through %#x, want 0xc1", asciiToEbcdic['A'])
	}
}

func TestUnsetCount(t *testing.T) {
	// An unset count means every block, which no input reaches.
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("hello"),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
	}
	if err := New(rt).Run(t.Context(), "ibs=1"); err != nil {
		t.Errorf("Run() error = %v", err)
	}
	if got := rt.Stderr.(*bytes.Buffer).String(); got != "5+0 records in\n0+1 records out\n" {
		t.Errorf("Run() stderr = %q", got)
	}
}
