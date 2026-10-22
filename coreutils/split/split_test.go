package split

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

// failWriteFS wraps a MemFS so writes to created files fail.
type failWriteFS struct {
	fsx.MemFS
	err error
}

// OpenFile implements fsx.WriteFS.
func (m failWriteFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.MemFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return failWriteFile{File: f, err: m.err}, nil
}

type failWriteFile struct {
	fsx.File
	err error
}

func (f failWriteFile) Write([]byte) (int, error) {
	return 0, f.err
}

type testCase struct {
	name      string
	stdin     io.Reader
	fsys      fsx.FS
	args      []string
	want      map[string]string
	wantErr   string
	wantExit  int
	wantFiles []string
}

func lines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	fsys := test.fsys
	if fsys == nil {
		fsys = fsx.MemFS{}
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  test.stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), test.args...)

	code := 0
	if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
		code = exitErr.Code
	} else if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}

	if mem, ok := fsys.(fsx.MemFS); ok {
		var got []string
		for name := range mem {
			got = append(got, name)
		}
		sort.Strings(got)
		want := test.wantFiles
		if want == nil {
			want = make([]string, 0, len(test.want))
			for name := range test.want {
				want = append(want, name)
			}
			sort.Strings(want)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("Run() files = %v, want %v", got, want)
		}
		for name, content := range test.want {
			if got := string(mem[name].Data); got != content {
				t.Errorf("Run() file %s = %q, want %q", name, got, content)
			}
		}
	}
}

func TestRunLines(t *testing.T) {
	tests := []testCase{
		{
			name:      "default is 1000 lines per piece",
			fsys:      fsx.MemFS{"in.txt": {Data: []byte(lines(1, 2500))}},
			args:      []string{"in.txt"},
			wantFiles: []string{"in.txt", "xaa", "xab", "xac"},
			want: map[string]string{
				"xaa": lines(1, 1000),
				"xab": lines(1001, 2000),
				"xac": lines(2001, 2500),
			},
		},
		{
			name:  "pieces of two lines",
			stdin: strings.NewReader("a\nb\nc\nd\ne\n"),
			args:  []string{"-l", "2"},
			want: map[string]string{
				"xaa": "a\nb\n",
				"xab": "c\nd\n",
				"xac": "e\n",
			},
		},
		{
			name:  "an input ending at a boundary leaves no empty piece",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			args:  []string{"-l2"},
			want: map[string]string{
				"xaa": "a\nb\n",
				"xab": "c\nd\n",
			},
		},
		{
			name:  "the last line may miss its newline",
			stdin: strings.NewReader("a\nb\nc"),
			args:  []string{"-l", "2"},
			want: map[string]string{
				"xaa": "a\nb\n",
				"xab": "c",
			},
		},
		{
			name:  "custom prefix",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-l", "2", "-", "part_"},
			want: map[string]string{
				"part_aa": "a\nb\n",
				"part_ab": "c\n",
			},
		},
		{
			name:  "longer suffixes",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-l", "1", "-a3"},
			want: map[string]string{
				"xaaa": "a\n",
				"xaab": "b\n",
				"xaac": "c\n",
			},
		},
		{
			name:      "empty input creates no files",
			fsys:      fsx.MemFS{"empty.txt": {Data: []byte("")}},
			args:      []string{"empty.txt"},
			wantFiles: []string{"empty.txt"},
		},
		{
			name:      "empty input creates no files even with zero suffix length",
			fsys:      fsx.MemFS{"empty.txt": {Data: []byte("")}},
			args:      []string{"-a", "0", "empty.txt"},
			wantFiles: []string{"empty.txt"},
		},
		{
			name:  "zero suffix length allows a single piece",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-a", "0", "-l", "9"},
			want:  map[string]string{"x": "a\nb\n"},
		},
		{
			name:      "pieces in a subdirectory prefix",
			fsys:      fsx.MemFS{"in.txt": {Data: []byte("a\nb\nc\n")}, "out/keep.txt": {Data: []byte("k")}},
			args:      []string{"-l", "1", "in.txt", "out/p_"},
			wantFiles: []string{"in.txt", "out/keep.txt", "out/p_aa", "out/p_ab", "out/p_ac"},
			want: map[string]string{
				"out/p_aa": "a\n",
				"out/p_ab": "b\n",
				"out/p_ac": "c\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBytes(t *testing.T) {
	tests := []testCase{
		{
			name:  "pieces of two bytes",
			stdin: strings.NewReader("abcde"),
			args:  []string{"-b", "2"},
			want: map[string]string{
				"xaa": "ab",
				"xab": "cd",
				"xac": "e",
			},
		},
		{
			name:  "a trailing k multiplies by 1024",
			stdin: strings.NewReader(strings.Repeat("z", 3000)),
			args:  []string{"-b", "1k"},
			want: map[string]string{
				"xaa": strings.Repeat("z", 1024),
				"xab": strings.Repeat("z", 1024),
				"xac": strings.Repeat("z", 952),
			},
		},
		{
			name:  "a trailing m multiplies by 1048576",
			stdin: strings.NewReader(strings.Repeat("z", 1048576+10)),
			args:  []string{"-b1m"},
			want: map[string]string{
				"xaa": strings.Repeat("z", 1048576),
				"xab": strings.Repeat("z", 10),
			},
		},
		{
			name:  "byte pieces may split lines",
			stdin: strings.NewReader("ab\ncd\nef"),
			args:  []string{"-b", "4"},
			want: map[string]string{
				"xaa": "ab\nc",
				"xab": "d\nef",
			},
		},
		{
			name:  "an input ending at a boundary leaves no empty piece",
			stdin: strings.NewReader("abcd"),
			args:  []string{"-b", "4"},
			want:  map[string]string{"xaa": "abcd"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSuffixExhaustion(t *testing.T) {
	files := fsx.MemFS{"in.txt": {Data: []byte(lines(1, 30))}}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     files,
	}
	err := New(rt).Run(t.Context(), "-a", "1", "-l", "1", "in.txt")
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok || exitErr.Code != 1 {
		t.Fatalf("Run() error = %v, want exit status 1", err)
	}
	if got, want := stderr.String(), "split: output file suffixes exhausted\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
	var names []string
	for name := range files {
		if strings.HasPrefix(name, "x") {
			names = append(names, name)
		}
	}
	if len(names) != 26 {
		t.Errorf("Run() created %d pieces, want 26", len(names))
	}
	if got := string(files["xz"].Data); got != "l26\n" {
		t.Errorf("Run() xz = %q, want %q", got, "l26\n")
	}
}

func TestRunInvalidUsage(t *testing.T) {
	long := strings.Repeat("n", 254)
	tests := []testCase{
		{
			name:     "unknown option",
			args:     []string{"-q"},
			wantErr:  "split: invalid option -- 'q'\n",
			wantExit: 2,
		},
		{
			name:     "byte and line modes conflict",
			args:     []string{"-b", "1", "-l", "1"},
			wantErr:  "split: cannot split in more than one way\n",
			wantExit: 2,
		},
		{
			name:     "zero lines is invalid",
			args:     []string{"-l", "0"},
			wantErr:  "split: invalid number of lines: '0'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric line count is invalid",
			args:     []string{"-l", "1x"},
			wantErr:  "split: invalid number of lines: '1x'\n",
			wantExit: 2,
		},
		{
			name:     "zero bytes is invalid",
			args:     []string{"-b", "0"},
			wantErr:  "split: invalid number of bytes: '0'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown multiplier is invalid",
			args:     []string{"-b", "1g"},
			wantErr:  "split: invalid number of bytes: '1g'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric suffix length is invalid",
			args:     []string{"-a", "x"},
			wantErr:  "split: invalid suffix length: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a negative suffix length is invalid",
			args:     []string{"-a", "-1"},
			wantErr:  "split: invalid suffix length: '-1'\n",
			wantExit: 2,
		},
		{
			name:     "extra operand",
			args:     []string{"a", "b", "c"},
			wantErr:  "split: extra operand 'c'\n",
			wantExit: 2,
		},
		{
			name:     "a prefix that would exceed NAME_MAX fails before any file",
			args:     []string{"-l", "1", "-", long},
			stdin:    strings.NewReader("a\n"),
			wantErr:  "split: " + long + "aa: file name too long\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing input file",
			fsys:     fsx.MemFS{},
			args:     []string{"missing.txt"},
			wantErr:  "split: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "read error",
			stdin:    errorReader{err: errors.New("boom")},
			args:     []string{"-l", "1"},
			wantErr:  "split: -: boom\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}

	t.Run("write error", func(t *testing.T) {
		runCase(t, testCase{
			fsys:     failWriteFS{MemFS: fsx.MemFS{}, err: errors.New("disk full")},
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-l", "1"},
			wantErr:  "split: xaa: disk full\n",
			wantExit: 1,
		})
	})
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	prog := New(&coreutils.Runtime{
		Stdin:  &cancelReader{cancel: cancel},
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fsx.MemFS{},
	})
	if err := prog.Run(ctx, "-l", "1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}
