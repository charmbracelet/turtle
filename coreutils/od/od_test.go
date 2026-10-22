package od

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
			name:  "default octal words and addresses",
			stdin: bytes.NewReader([]byte{1, 2, 3}),
			want:  "0000000 001001 000003\n0000003\n",
		},
		{
			name:  "byte octal type and no addresses",
			args:  []string{"-A", "n", "-t", "o1", "file"},
			files: fsx.MemFS{"file": {Data: []byte{1, 2, 255}}},
			want:  "001 002 377\n",
		},
		{
			name:  "character escapes",
			args:  []string{"-A", "n", "-tc", "file"},
			files: fsx.MemFS{"file": {Data: []byte{'A', 0, '\n', 0x7f}}},
			want:  "A \\0 \\n 177\n",
		},
		{
			name:  "named characters",
			args:  []string{"-A", "n", "-ta", "file"},
			files: fsx.MemFS{"file": {Data: []byte{0, 32, 65}}},
			want:  "nul sp A\n",
		},
		{
			name:  "hexadecimal unsigned values",
			args:  []string{"-A", "x", "-t", "x2", "file"},
			files: fsx.MemFS{"file": {Data: []byte{0x34, 0x12, 0xff, 0xff}}},
			want:  "0000000 1234 ffff\n0000004\n",
		},
		{
			name:  "skip and count parse prefixes",
			args:  []string{"-A", "d", "-j", "0x1", "-N", "02", "-t", "o1", "file"},
			files: fsx.MemFS{"file": {Data: []byte{1, 2, 3, 4, 5}}},
			want:  "0000001 002 003\n0000003\n",
		},
		{
			name:     "skip past end is an error",
			args:     []string{"-j", "5", "file"},
			files:    fsx.MemFS{"file": {Data: []byte{1, 2}}},
			wantErr:  "od: cannot skip past end of input\n",
			wantCode: 1,
		},
		{
			name:     "invalid type is rejected",
			args:     []string{"-t", "q"},
			wantErr:  "od: invalid type string\n",
			wantCode: 1,
		},
		{
			name:     "unknown option is rejected",
			args:     []string{"-q"},
			wantErr:  "od: invalid option -- 'q'\n",
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
	prog := New(&coreutils.Runtime{Stdin: bytes.NewReader([]byte{1}), Stdout: io.Discard, Stderr: io.Discard})
	if err := prog.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}
