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

	"charm.land/snail/coreutils"

	"charm.land/snail/fsx"
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

// cancelReader cancels its context on the first read, standing for an
// interrupted copy.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
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

func exitCode(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code
}

type ddCase struct {
	name      string
	stdin     io.Reader
	args      []string
	files     map[string]string
	want      string
	wantErr   string
	wantExit  int
	wantFiles map[string]string
}

func runCase(t *testing.T, test ddCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	fsys := fsx.MemFS{}
	for name, data := range test.files {
		fsys[name] = &fsx.MemFile{Data: []byte(data), Mode: 0o666}
	}
	stdin := test.stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	for name, want := range test.wantFiles {
		f, err := fsys.Open(name)
		if err != nil {
			t.Errorf("output file %s: %v", name, err)
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Errorf("reading %s: %v", name, err)
			continue
		}
		if got := string(data); got != want {
			t.Errorf("output file %s = %q, want %q", name, got, want)
		}
	}
}

func TestRunCopy(t *testing.T) {
	tests := []ddCase{
		{
			name:    "the standard input is copied whole",
			stdin:   strings.NewReader("hello"),
			want:    "hello",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "an empty input copies nothing",
			stdin:   strings.NewReader(""),
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:    "short input blocks are aggregated into output ones",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=3", "obs=7"},
			want:    "0123456789",
			wantErr: "3+1 records in\n1+1 records out\n",
		},
		{
			name:    "bs makes every input block a single output one",
			stdin:   strings.NewReader("abcde"),
			args:    []string{"bs=2"},
			want:    "abcde",
			wantErr: "2+1 records in\n2+1 records out\n",
		},
		{
			name:    "bs supersedes ibs and obs",
			stdin:   strings.NewReader("abcdef"),
			args:    []string{"ibs=1", "obs=1", "bs=4"},
			want:    "abcdef",
			wantErr: "1+1 records in\n1+1 records out\n",
		},
		{
			name:    "count copies only the first blocks",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=3", "count=2"},
			want:    "012345",
			wantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			name:    "count of zero copies nothing",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"count=0"},
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:    "count in bytes, as a trailing B asks",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=4", "count=7B"},
			want:    "0123456",
			wantErr: "1+1 records in\n0+1 records out\n",
		},
		{
			name:    "skip drops the first input blocks",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=3", "skip=2"},
			want:    "6789",
			wantErr: "1+1 records in\n0+1 records out\n",
		},
		{
			name:    "skip in bytes, as a trailing B asks",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=4", "skip=2B"},
			want:    "23456789",
			wantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			name:    "iseek is another name for skip",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=3", "iseek=1"},
			want:    "3456789",
			wantErr: "2+1 records in\n0+1 records out\n",
		},
		{
			name:    "skip is read and discarded on a non-seekable input",
			stdin:   plainReader{strings.NewReader("0123456789")},
			args:    []string{"ibs=3", "skip=2"},
			want:    "6789",
			wantErr: "1+1 records in\n0+1 records out\n",
		},
		{
			name:  "skipping past the end of the input warns",
			stdin: plainReader{strings.NewReader("abc")},
			args:  []string{"ibs=2", "skip=5"},
			wantErr: "dd: 'standard input': cannot skip to specified offset\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			name:  "status of none prints nothing",
			stdin: strings.NewReader("hello"),
			args:  []string{"status=none"},
			want:  "hello",
		},
		{
			name:    "status of noxfer keeps the records lines",
			stdin:   strings.NewReader("hello"),
			args:    []string{"status=noxfer"},
			want:    "hello",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "the last status level wins",
			stdin:   strings.NewReader("hello"),
			args:    []string{"status=none", "status=progress"},
			want:    "hello",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "a seek on a non-seekable output is filled with nulls",
			stdin:   strings.NewReader("ab"),
			args:    []string{"seek=2", "obs=2"},
			want:    "\x00\x00\x00\x00ab",
			wantErr: "0+1 records in\n1+0 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunConversions(t *testing.T) {
	tests := []ddCase{
		{
			name:    "ucase",
			stdin:   strings.NewReader("Hello, World!"),
			args:    []string{"conv=ucase"},
			want:    "HELLO, WORLD!",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "lcase",
			stdin:   strings.NewReader("Hello, World!"),
			args:    []string{"conv=lcase"},
			want:    "hello, world!",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "swab pairs every two bytes",
			stdin:   strings.NewReader("abcde"),
			args:    []string{"ibs=2", "obs=16", "conv=swab"},
			want:    "badce",
			wantErr: "2+1 records in\n0+1 records out\n",
		},
		{
			name:    "swab keeps the last byte of an even output whole",
			stdin:   strings.NewReader("abcd"),
			args:    []string{"ibs=2", "obs=16", "conv=swab"},
			want:    "badc",
			wantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			name:    "swab carries the odd byte across input blocks",
			stdin:   strings.NewReader("abcdefg"),
			args:    []string{"ibs=3", "obs=16", "conv=swab"},
			want:    "badcfeg",
			wantErr: "2+1 records in\n0+1 records out\n",
		},
		{
			name:    "sync pads the input block with nulls",
			stdin:   strings.NewReader("ab"),
			args:    []string{"ibs=4", "obs=8", "conv=sync"},
			want:    "ab\x00\x00",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "block pads short records with spaces",
			stdin:   strings.NewReader("ab\ncd\n"),
			args:    []string{"cbs=3", "obs=8", "conv=block"},
			want:    "ab cd ",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "block truncates long records and counts one",
			stdin:   strings.NewReader("abcd\n"),
			args:    []string{"cbs=3", "obs=8", "conv=block"},
			want:    "abc",
			wantErr: "0+1 records in\n0+1 records out\n1 truncated record\n",
		},
		{
			name:    "block truncates long records and counts many",
			stdin:   strings.NewReader("abcd\nabcd\n"),
			args:    []string{"cbs=3", "obs=8", "conv=block"},
			want:    "abcabc",
			wantErr: "0+1 records in\n0+1 records out\n2 truncated records\n",
		},
		{
			name:    "block counts one truncated record however long",
			stdin:   strings.NewReader("abcdef\n"),
			args:    []string{"cbs=3", "obs=8", "conv=block"},
			want:    "abc",
			wantErr: "0+1 records in\n0+1 records out\n1 truncated record\n",
		},
		{
			name:    "block counts a record truncated across input blocks",
			stdin:   strings.NewReader("abcd\n"),
			args:    []string{"ibs=2", "cbs=3", "obs=8", "conv=block"},
			want:    "abc",
			wantErr: "2+1 records in\n0+1 records out\n1 truncated record\n",
		},
		{
			name:    "block pads the record cut by the end of the input",
			stdin:   strings.NewReader("ab"),
			args:    []string{"cbs=3", "obs=8", "conv=block"},
			want:    "ab ",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "block spans input block boundaries",
			stdin:   strings.NewReader("ab\ncd\n"),
			args:    []string{"ibs=3", "cbs=4", "obs=8", "conv=block"},
			want:    "ab  cd  ",
			wantErr: "2+0 records in\n1+0 records out\n",
		},
		{
			name:    "unblock replaces the trailing spaces with newlines",
			stdin:   strings.NewReader("ab cd x"),
			args:    []string{"cbs=3", "obs=8", "conv=unblock"},
			want:    "ab\ncd\nx\n",
			wantErr: "0+1 records in\n1+0 records out\n",
		},
		{
			name:    "unblock keeps the spaces which are not trailing",
			stdin:   strings.NewReader("a b c d "),
			args:    []string{"cbs=2", "obs=8", "conv=unblock"},
			want:    "a\nb\nc\nd\n",
			wantErr: "0+1 records in\n1+0 records out\n",
		},
		{
			name:    "unblock drops the spaces held back at the end",
			stdin:   strings.NewReader("a  b"),
			args:    []string{"cbs=3", "obs=8", "conv=unblock"},
			want:    "a\nb\n",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "ascii converts and unblocks",
			stdin:   strings.NewReader("\xc1\xc2\x40"),
			args:    []string{"cbs=3", "obs=8", "conv=ascii"},
			want:    "AB\n",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "ebcdic converts and blocks",
			stdin:   strings.NewReader("AB\n"),
			args:    []string{"cbs=3", "obs=8", "conv=ebcdic"},
			want:    "\xc1\xc2\x40",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "ascii converts the newlines the tables define",
			stdin:   strings.NewReader("\x25\x25"),
			args:    []string{"obs=8", "conv=ascii"},
			want:    "\n\n",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "ibm converts to the alternate table",
			stdin:   strings.NewReader("~"),
			args:    []string{"obs=8", "conv=ibm"},
			want:    "\xa1",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "ebcdic and ascii are the inverse of each other",
			stdin:   strings.NewReader("~[a\n"),
			args:    []string{"obs=8", "conv=ebcdic"},
			want:    "\x5f\xad\x81\x25",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "ascii and ucase compose",
			stdin:   strings.NewReader("\x81\x82\x40"),
			args:    []string{"cbs=3", "obs=8", "conv=ascii,ucase"},
			want:    "AB\n",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "sync with block pads with spaces",
			stdin:   strings.NewReader("ab"),
			args:    []string{"ibs=4", "cbs=4", "obs=8", "conv=block,sync"},
			want:    "ab  ",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "bs with a data conversion still aggregates",
			stdin:   strings.NewReader("abcde"),
			args:    []string{"bs=4", "conv=swab"},
			want:    "badce",
			wantErr: "1+1 records in\n1+1 records out\n",
		},
		{
			name:    "ascii and swab compose",
			stdin:   strings.NewReader("\xc1\x82\xc3"),
			args:    []string{"obs=8", "conv=ascii,swab"},
			want:    "bAC",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []ddCase{
		{
			name:  "if and of copy between files",
			stdin: strings.NewReader("ignored"),
			args:  []string{"if=in.txt", "of=out.txt"},
			files: map[string]string{"in.txt": "hello"},
			wantFiles: map[string]string{
				"out.txt": "hello",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:  "of creates the output file",
			stdin: strings.NewReader("hi"),
			args:  []string{"of=out.txt"},
			wantFiles: map[string]string{
				"out.txt": "hi",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:  "of truncates the output file",
			stdin: strings.NewReader("hi"),
			args:  []string{"of=out.txt"},
			files: map[string]string{"out.txt": "OLD CONTENT"},
			wantFiles: map[string]string{
				"out.txt": "hi",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:  "notrunc keeps the rest of the output file",
			stdin: strings.NewReader("hi"),
			args:  []string{"of=out.txt", "conv=notrunc"},
			files: map[string]string{"out.txt": "OLDOLDOLD"},
			wantFiles: map[string]string{
				"out.txt": "hiDOLDOLD",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:  "seek writes past the preserved blocks",
			stdin: strings.NewReader("XY"),
			args:  []string{"of=out.txt", "seek=1", "obs=4"},
			files: map[string]string{"out.txt": "0123456789"},
			wantFiles: map[string]string{
				"out.txt": "0123XY",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:  "seek with notrunc keeps the tail of the output file",
			stdin: strings.NewReader("XY"),
			args:  []string{"of=out.txt", "seek=1", "obs=4", "conv=notrunc"},
			files: map[string]string{"out.txt": "0123456789"},
			wantFiles: map[string]string{
				"out.txt": "0123XY6789",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:  "seek past the end of the output file keeps its size",
			stdin: strings.NewReader(""),
			args:  []string{"of=out.txt", "seek=2", "obs=4"},
			files: map[string]string{"out.txt": "01234567"},
			wantFiles: map[string]string{
				"out.txt": "01234567",
			},
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:  "seek with count of zero still shortens the file",
			stdin: strings.NewReader("ignored"),
			args:  []string{"of=out.txt", "seek=1", "obs=4", "count=0"},
			files: map[string]string{"out.txt": "0123456789"},
			wantFiles: map[string]string{
				"out.txt": "0123",
			},
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:  "a seek with an empty input extends a new output file",
			stdin: strings.NewReader(""),
			args:  []string{"of=out.txt", "seek=2", "obs=4"},
			wantFiles: map[string]string{
				"out.txt": "\x00\x00\x00\x00\x00\x00\x00\x00",
			},
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:  "a seek with an empty input extends a shorter output file",
			stdin: strings.NewReader(""),
			args:  []string{"of=out.txt", "seek=2", "obs=4"},
			files: map[string]string{"out.txt": "abcd"},
			wantFiles: map[string]string{
				"out.txt": "abcd\x00\x00\x00\x00",
			},
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:  "a seek past the end keeps the output size with notrunc",
			stdin: strings.NewReader(""),
			args:  []string{"of=out.txt", "seek=2", "obs=4", "conv=notrunc"},
			files: map[string]string{"out.txt": "ab"},
			wantFiles: map[string]string{
				"out.txt": "ab",
			},
			wantErr: "0+0 records in\n0+0 records out\n",
		},
		{
			name:  "oseek is another name for seek",
			stdin: strings.NewReader("AB"),
			args:  []string{"of=out.txt", "oseek=2", "obs=1"},
			wantFiles: map[string]string{
				"out.txt": "\x00\x00AB",
			},
			wantErr: "0+1 records in\n2+0 records out\n",
		},
		{
			name:    "skip seeks the input file",
			stdin:   strings.NewReader("ignored"),
			args:    []string{"if=in.txt", "ibs=2", "skip=2"},
			files:   map[string]string{"in.txt": "0123456789"},
			want:    "456789",
			wantErr: "3+0 records in\n0+1 records out\n",
		},
		{
			name:     "a missing input file fails",
			args:     []string{"if=no.txt"},
			wantExit: 1,
			wantErr:  "dd: failed to open 'no.txt': file does not exist\n",
		},
		{
			name:     "excl fails when the output file exists",
			stdin:    strings.NewReader("hi"),
			args:     []string{"of=out.txt", "conv=excl"},
			files:    map[string]string{"out.txt": "old"},
			wantExit: 1,
			wantErr:  "dd: failed to open 'out.txt': file already exists\n",
		},
		{
			name:     "nocreat fails when the output file is missing",
			stdin:    strings.NewReader("hi"),
			args:     []string{"of=out.txt", "conv=nocreat"},
			wantExit: 1,
			wantErr:  "dd: failed to open 'out.txt': file does not exist\n",
		},
		{
			name:  "fsync flushes the output file",
			stdin: strings.NewReader("hi"),
			args:  []string{"of=out.txt", "conv=fsync"},
			wantFiles: map[string]string{
				"out.txt": "hi",
			},
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "operands are taken in any order",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"count=2", "ibs=3", "if=in.txt", "skip=1"},
			files:   map[string]string{"in.txt": "0123456789"},
			want:    "345678",
			wantErr: "2+0 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSizes(t *testing.T) {
	tests := []ddCase{
		{
			name:    "a bare number counts bytes",
			stdin:   strings.NewReader("01234567"),
			args:    []string{"ibs=4"},
			want:    "01234567",
			wantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			name:    "b multiplies by 512",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=1b", "count=1"},
			want:    "0123456789",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "k multiplies by 1024",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=1k", "obs=1k", "count=1"},
			want:    "0123456789",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "x multiplies the numbers it separates",
			stdin:   strings.NewReader("0123456"),
			args:    []string{"bs=2x2"},
			want:    "0123456",
			wantErr: "1+1 records in\n1+1 records out\n",
		},
		{
			name:    "KiB is the binary k",
			stdin:   strings.NewReader("0123456789"),
			args:    []string{"ibs=1KiB", "count=1"},
			want:    "0123456789",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "KB is the decimal k",
			stdin:   strings.NewReader(strings.Repeat("a", 999)),
			args:    []string{"ibs=1KB", "count=1"},
			want:    strings.Repeat("a", 999),
			wantErr: "0+1 records in\n1+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFullblock(t *testing.T) {
	tests := []ddCase{
		{
			name:    "short reads count as one block",
			stdin:   shortReader{strings.NewReader("01234567")},
			args:    []string{"ibs=4", "count=1"},
			want:    "01",
			wantErr: "0+1 records in\n0+1 records out\n",
		},
		{
			name:    "fullblock accumulates the whole block",
			stdin:   shortReader{strings.NewReader("01234567")},
			args:    []string{"ibs=4", "count=1", "iflag=fullblock"},
			want:    "0123",
			wantErr: "1+0 records in\n0+1 records out\n",
		},
		{
			name:    "fullblock without count copies everything",
			stdin:   shortReader{strings.NewReader("01234567")},
			args:    []string{"ibs=4", "iflag=fullblock"},
			want:    "01234567",
			wantErr: "2+0 records in\n0+1 records out\n",
		},
		{
			name:    "fullblock reads the count in bytes tail",
			stdin:   shortReader{strings.NewReader("01234567")},
			args:    []string{"ibs=4", "count=7B", "iflag=fullblock"},
			want:    "0123456",
			wantErr: "1+1 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name     string
		stdin    io.Reader
		args     []string
		want     string
		wantErr  string
		wantExit int
	}{
		{
			name:     "a read error stops the copy",
			stdin:    &errorReader{data: "ab", err: errors.New("read failure")},
			want:     "ab",
			wantErr:  "dd: error reading 'standard input': read failure\n0+1 records in\n0+1 records out\n",
			wantExit: 1,
		},
		{
			name:  "noerror stops on an input it can't reposition",
			stdin: &errorReader{data: "ab", err: errors.New("read failure")},
			args:  []string{"conv=noerror"},
			want:  "ab",
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+1 records in\n0+0 records out\n" +
				"0+1 records in\n0+1 records out\n",
			wantExit: 1,
		},
		{
			name:  "noerror and sync stop without the zeroed block",
			stdin: &errorReader{err: errors.New("read failure")},
			args:  []string{"ibs=4", "conv=noerror,sync"},
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"0+0 records in\n0+0 records out\n",
			wantExit: 1,
		},
		{
			name:     "noerror stops silently with status of none",
			stdin:    &errorReader{data: "ab", err: errors.New("read failure")},
			args:     []string{"conv=noerror", "status=none"},
			want:     "ab",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, ddCase{
				name:     test.name,
				stdin:    test.stdin,
				args:     test.args,
				want:     test.want,
				wantErr:  test.wantErr,
				wantExit: test.wantExit,
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
			name:  "noerror skips the rest of the failed block only",
			stdin: &errAtReader{data: "ABCDEFGHIJKLMNOPQRST", errAt: 11, partial: 1, err: readFailure},
			args:  []string{"ibs=8", "obs=16", "conv=noerror"},
			// The failing read starts at 11 and intends 8 bytes, so the copy
			// resumes at 19 and not at 16.
			want: "ABCDEFGHIJKT",
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"1+1 records in\n0+0 records out\n" +
				"1+2 records in\n0+1 records out\n",
		},
		{
			name:  "noerror and sync zero the failed block after a short one",
			stdin: &errAtReader{data: "ABCDEFGHIJK", errAt: 11, partial: 0, err: readFailure},
			args:  []string{"ibs=8", "obs=24", "conv=noerror,sync"},
			want:  "ABCDEFGH" + "IJK\x00\x00\x00\x00\x00" + "\x00\x00\x00\x00\x00\x00\x00\x00",
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"1+1 records in\n0+0 records out\n" +
				"1+2 records in\n1+0 records out\n",
		},
		{
			name:  "noerror and sync keep the data a failed read returned",
			stdin: &errAtReader{data: "ABCDEF", errAt: 0, partial: 2, err: readFailure},
			args:  []string{"ibs=4", "obs=8", "conv=noerror,sync"},
			want:  "AB\x00\x00EF\x00\x00",
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"0+2 records in\n1+0 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStuckInput(t *testing.T) {
	readFailure := errors.New("read failure")
	tests := []ddCase{
		{
			name:     "a rejecting input stops the copy under noerror",
			stdin:    &stuckReader{err: readFailure},
			args:     []string{"ibs=4", "obs=8", "conv=noerror"},
			wantExit: 1,
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot seek\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			name:     "a rejecting input stops the copy under noerror and sync",
			stdin:    &stuckReader{err: readFailure},
			args:     []string{"ibs=4", "obs=8", "conv=noerror,sync"},
			wantExit: 1,
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot seek\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			name:     "the data copied before the input went bad is kept",
			stdin:    &dyingReader{data: "ab", err: readFailure},
			args:     []string{"ibs=4", "obs=8", "conv=noerror"},
			want:     "ab",
			wantExit: 1,
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+1 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot seek\n" +
				"0+1 records in\n0+1 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSkipError(t *testing.T) {
	readFailure := errors.New("read failure")
	tests := []ddCase{
		{
			name:     "a read error while skipping stops the copy",
			stdin:    plainReader{&errAtReader{data: "ABCD", errAt: 0, partial: 0, err: readFailure}},
			args:     []string{"skip=2", "ibs=2", "obs=2"},
			wantExit: 1,
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n",
		},
		{
			name:  "noerror keeps skipping after a read error",
			stdin: plainReader{&errAtReader{data: "ABCD", errAt: 0, partial: 0, err: readFailure}},
			args:  []string{"skip=2", "ibs=2", "obs=2", "conv=noerror"},
			want:  "ABCD",
			wantErr: "dd: error reading 'standard input': read failure\n" +
				"0+0 records in\n0+0 records out\n" +
				"dd: 'standard input': cannot skip to specified offset\n" +
				"2+0 records in\n2+0 records out\n",
		},
		{
			name:    "a seekable input is skipped without reading it",
			stdin:   &errAtReader{data: "ABCD", errAt: 0, partial: 0, err: readFailure},
			args:    []string{"skip=2", "ibs=2", "obs=2", "conv=noerror"},
			want:    "",
			wantErr: "0+0 records in\n0+0 records out\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInvalidOperands(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "an operand without a value",
			args:    []string{"foo"},
			wantErr: "dd: unrecognized operand 'foo'\n",
		},
		{
			name:    "an unknown operand name",
			args:    []string{"foo=bar"},
			wantErr: "dd: unrecognized operand 'foo=bar'\n",
		},
		{
			name:    "an invalid number",
			args:    []string{"ibs=abc"},
			wantErr: "dd: invalid number 'abc'\n",
		},
		{
			name:    "a zero block size",
			args:    []string{"ibs=0"},
			wantErr: "dd: invalid number '0'\n",
		},
		{
			name:    "an invalid count",
			args:    []string{"count=x2"},
			wantErr: "dd: invalid number 'x2'\n",
		},
		{
			name:    "an invalid conversion",
			args:    []string{"conv=ascii,foo"},
			wantErr: "dd: invalid conversion 'foo'\n",
		},
		{
			name:    "an invalid status level",
			args:    []string{"status=quiet"},
			wantErr: "dd: invalid status level 'quiet'\n",
		},
		{
			name:    "an invalid input flag",
			args:    []string{"iflag=direct"},
			wantErr: "dd: invalid input flag 'direct'\n",
		},
		{
			name:    "an invalid output flag",
			args:    []string{"oflag=fullblock"},
			wantErr: "dd: invalid output flag 'fullblock'\n",
		},
		{
			name:    "two character set conversions",
			args:    []string{"conv=ascii,ebcdic", "cbs=1"},
			wantErr: "dd: cannot combine any two of ascii, ebcdic and ibm\n",
		},
		{
			name:    "block and unblock",
			args:    []string{"conv=block,unblock", "cbs=1"},
			wantErr: "dd: cannot combine block and unblock\n",
		},
		{
			name:    "lcase and ucase",
			args:    []string{"conv=lcase,ucase"},
			wantErr: "dd: cannot combine lcase and ucase\n",
		},
		{
			name:    "excl and nocreat",
			args:    []string{"conv=excl,nocreat"},
			wantErr: "dd: cannot combine excl and nocreat\n",
		},
		{
			name:    "an unknown option",
			args:    []string{"--foo"},
			wantErr: "dd: unrecognized option '--foo'\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, ddCase{
				name:     test.name,
				args:     test.args,
				wantExit: 1,
				wantErr:  test.wantErr + "Try 'dd --help' for more information.\n",
			})
		})
	}
}

func TestRunStandardOptions(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantExit int
		wantErr  string
	}{
		{
			name:    "help",
			args:    []string{"--help"},
			wantOut: usage,
		},
		{
			name: "version",
			args: []string{"--version"},
			wantOut: fmt.Sprintf("dd (charm.land/snail/coreutils) %s\nhttps://charm.land/snail\n",
				coreutils.Version),
		},
		{
			name:     "options are only taken before the operands",
			args:     []string{"if=x", "--help"},
			wantExit: 1,
			wantErr:  "dd: unrecognized operand '--help'\nTry 'dd --help' for more information.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
			err := New(rt).Run(t.Context(), test.args...)
			if code := exitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
			}
			if got := stdout.String(); got != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  &cancelReader{cancel: cancel},
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
	if code := exitCode(t, err); code != 0 {
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
