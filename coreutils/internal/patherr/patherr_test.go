package patherr

import (
	"errors"
	"io/fs"
	"testing"
)

func TestUnwrap(t *testing.T) {
	inner := fs.ErrNotExist
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "a path error loses its operation and path",
			err:  &fs.PathError{Op: "open", Path: "file.txt", Err: inner},
			want: inner,
		},
		{
			name: "any other error stays as it is",
			err:  inner,
			want: inner,
		},
		{
			name: "a nil error stays nil",
			err:  nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Unwrap(tt.err); !errors.Is(got, tt.want) {
				t.Errorf("Unwrap() = %v, want %v", got, tt.want)
			}
		})
	}
}
