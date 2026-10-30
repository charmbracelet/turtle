package strings

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    io.Reader
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "default minimum string length",
			stdin: bytes.NewBufferString("abc\nabcd\x00hello"),
			want:  "abcd\nhello\n",
		},
		{
			name:  "scan entire input with offsets",
			args:  []string{"-a", "-t", "x", "-n2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01abc\x00xy")}},
			want:  "1 abc\n5 xy\n",
		},
		{
			name:  "decimal offsets",
			args:  []string{"-td", "-n", "2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\ncd")}},
			want:  "1 ab\n4 cd\n",
		},
		{
			name:  "octal offsets",
			args:  []string{"-to", "-n", "2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\x00cd")}},
			want:  "1 ab\n4 cd\n",
		},
		{
			name:  "print-file-name prefix",
			args:  []string{"-f", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01abcd\x00hello")}},
			want:  "file: abcd\nfile: hello\n",
		},
		{
			name:  "print-file-name long option",
			args:  []string{"--print-file-name", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01abcd\x00hello")}},
			want:  "file: abcd\nfile: hello\n",
		},
		{
			name:  "print-file-name with offsets",
			args:  []string{"-f", "-t", "x", "-n2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\x00cd")}},
			want:  "file: 1 ab\nfile: 4 cd\n",
		},
		{
			name:  "print-file-name for each file",
			args:  []string{"-f", "-n2", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("ab")}, "b": {Data: []byte("cd")}},
			want:  "a: ab\nb: cd\n",
		},
		{
			name:  "no file name prefix without f with multiple files",
			args:  []string{"-n2", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("ab")}, "b": {Data: []byte("cd")}},
			want:  "ab\ncd\n",
		},
		{
			name:  "print-file-name on the standard input",
			args:  []string{"-f", "-n2"},
			stdin: bytes.NewBufferString("ab\x00cd"),
			want:  "-: ab\n-: cd\n",
		},
		{
			name:  "o option is an alias for t o",
			args:  []string{"-o", "-n2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\x00\x00\x00\x00\x00\x00cd")}},
			want:  "1 ab\n11 cd\n",
		},
		{
			name:  "t after o overrides it",
			args:  []string{"-o", "-td", "-n2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\x00\x00\x00\x00\x00\x00cd")}},
			want:  "1 ab\n9 cd\n",
		},
		{
			name:  "o after t overrides it",
			args:  []string{"-td", "-o", "-n2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\x00\x00\x00\x00\x00\x00cd")}},
			want:  "1 ab\n11 cd\n",
		},
		{
			name:  "grouped f and o",
			args:  []string{"-fo", "-n2", "file"},
			files: fsx.MemFS{"file": {Data: []byte("\x01ab\x00\x00\x00\x00\x00\x00cd")}},
			want:  "file: 1 ab\nfile: 11 cd\n",
		},
		{
			name:     "invalid minimum length",
			args:     []string{"-n0"},
			wantErr:  "strings: invalid minimum string length\nTry 'strings --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "invalid offset format",
			args:     []string{"-tq"},
			wantErr:  "strings: invalid format type\nTry 'strings --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "missing file",
			args:     []string{"missing"},
			files:    fsx.MemFS{},
			wantErr:  "strings: missing: file does not exist\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			runtime := &coreutils.Runtime{Stdin: test.stdin, Stdout: stdout, Stderr: stderr, FS: test.files}
			err := New(runtime).Run(t.Context(), test.args...)
			code := 0
			if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
				code = exitErr.Code
			} else if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if got := stdout.String(); got != test.want {
				t.Errorf("Run() stdout = %q, want %q", got, test.want)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	prog := New(&coreutils.Runtime{
		Stdin:  bytes.NewBufferString("input"),
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err := prog.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "strings (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
