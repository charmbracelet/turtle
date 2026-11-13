package input

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		oper string
		want Input
	}{
		{name: "pathname", oper: "file.txt", want: Input{Name: "file.txt"}},
		{name: "dash is the standard input", oper: "-", want: Input{Name: "-", Stdin: true}},
		{name: "empty name stays empty", oper: "", want: Input{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.oper); got != tt.want {
				t.Errorf("New(%q) = %+v, want %+v", tt.oper, got, tt.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name     string
		operands []string
		want     []Input
	}{
		{
			name: "no operands default to the standard input",
			want: []Input{{Stdin: true}},
		},
		{
			name:     "operands keep their names",
			operands: []string{"a.txt", "-"},
			want:     []Input{{Name: "a.txt"}, {Name: "-", Stdin: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := List(tt.operands)
			if len(got) != len(tt.want) {
				t.Fatalf("List = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("List[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestReadAll(t *testing.T) {
	tests := []struct {
		name    string
		rt      *coreutils.Runtime
		operand string
		want    string
		wantErr string
	}{
		{
			name:    "dash is the standard input",
			rt:      &coreutils.Runtime{Stdin: strings.NewReader("hello")},
			operand: "-",
			want:    "hello",
		},
		{
			name:    "file in the file system",
			rt:      &coreutils.Runtime{FS: fsx.MemFS{"file.txt": {Data: []byte("contents")}}},
			operand: "file.txt",
			want:    "contents",
		},
		{
			name:    "no standard input available",
			rt:      &coreutils.Runtime{},
			operand: "-",
			wantErr: "no standard input available",
		},
		{
			name:    "no file system available",
			rt:      &coreutils.Runtime{},
			operand: "file.txt",
			wantErr: "no file system available",
		},
		{
			name:    "missing file",
			rt:      &coreutils.Runtime{FS: fsx.MemFS{}},
			operand: "missing.txt",
			wantErr: "open missing.txt: file does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := ReadAll(tt.rt, t.Context(), tt.operand)
			if err != nil {
				if err.Error() != tt.wantErr {
					t.Fatalf("ReadAll error = %q, want %q", err, tt.wantErr)
				}
				return
			}
			if tt.wantErr != "" {
				t.Fatalf("ReadAll error = nil, want %q", tt.wantErr)
			}
			if string(data) != tt.want {
				t.Errorf("ReadAll = %q, want %q", data, tt.want)
			}
		})
	}
}

func TestReadAllContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := ReadAll(&coreutils.Runtime{Stdin: strings.NewReader("hello")}, ctx, "-")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ReadAll error = %v, want %v", err, context.Canceled)
	}
}

func TestReadError(t *testing.T) {
	inner := errors.New("boom")
	err := &ReadError{Label: "file.txt", Err: inner}
	if err.Error() != "boom" {
		t.Errorf("Error() = %q, want %q", err.Error(), "boom")
	}
	if err.Label != "file.txt" {
		t.Errorf("Label = %q, want %q", err.Label, "file.txt")
	}
	if !errors.Is(err, inner) {
		t.Error("ReadError doesn't unwrap to the wrapped error")
	}
}

func TestLabel(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want string
	}{
		{name: "default standard input", in: Input{Stdin: true}, want: "standard input"},
		{name: "dash operand keeps its name", in: Input{Name: "-", Stdin: true}, want: "-"},
		{name: "pathname", in: Input{Name: "file.txt"}, want: "file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	tests := []struct {
		name    string
		rt      *coreutils.Runtime
		in      Input
		want    string
		wantErr string
	}{
		{
			name: "standard input",
			rt:   &coreutils.Runtime{Stdin: strings.NewReader("hello")},
			in:   Input{Stdin: true},
			want: "hello",
		},
		{
			name:    "no standard input available",
			rt:      &coreutils.Runtime{},
			in:      Input{Stdin: true},
			wantErr: "no standard input available",
		},
		{
			name: "file in the file system",
			rt: &coreutils.Runtime{FS: fsx.MemFS{
				"file.txt": {Data: []byte("hello")},
			}},
			in:   Input{Name: "file.txt"},
			want: "hello",
		},
		{
			name: "file resolved from the working directory",
			rt: &coreutils.Runtime{FS: fsx.MemFS{
				"dir/file.txt": {Data: []byte("hello")},
			}, WorkingDir: "dir"},
			in:   Input{Name: "file.txt"},
			want: "hello",
		},
		{
			name:    "no file system available",
			rt:      &coreutils.Runtime{},
			in:      Input{Name: "file.txt"},
			wantErr: "no file system available",
		},
		{
			name: "missing file",
			rt: &coreutils.Runtime{FS: fsx.MemFS{
				"file.txt": {Data: []byte("hello")},
			}},
			in:      Input{Name: "missing.txt"},
			wantErr: "open missing.txt: file does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Open(tt.rt, t.Context(), tt.in)
			if err != nil {
				if err.Error() != tt.wantErr {
					t.Fatalf("Open error = %q, want %q", err, tt.wantErr)
				}
				return
			}
			if tt.wantErr != "" {
				t.Fatalf("Open error = nil, want %q", tt.wantErr)
			}
			data, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll error = %v", err)
			}
			if err := r.Close(); err != nil {
				t.Fatalf("Close error = %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("ReadAll = %q, want %q", data, tt.want)
			}
		})
	}
}
