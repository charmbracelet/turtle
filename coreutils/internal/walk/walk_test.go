package walk

import (
	"errors"
	"io/fs"
	"testing"
)

func TestDirError(t *testing.T) {
	inner := fs.ErrPermission
	tests := []struct {
		name     string
		err      error
		wantMsg  string
		wantIsOK bool
	}{
		{
			name:     "a readdirent failure is a directory read failure",
			err:      &fs.PathError{Op: "readdirent", Path: "dir", Err: inner},
			wantMsg:  "cannot read directory 'dir': permission denied",
			wantIsOK: true,
		},
		{
			name:     "a readdir failure is a directory read failure",
			err:      &fs.PathError{Op: "readdir", Path: "dir", Err: inner},
			wantMsg:  "cannot read directory 'dir': permission denied",
			wantIsOK: true,
		},
		{
			name:    "a path error of another operation is not one",
			err:     &fs.PathError{Op: "open", Path: "dir", Err: inner},
			wantMsg: "",
		},
		{
			name:    "any other error is not one",
			err:     fs.ErrNotExist,
			wantMsg: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, isDirErr := DirError("dir", tt.err)
			if isDirErr != tt.wantIsOK {
				t.Errorf("DirError() isDirErr = %v, want %v", isDirErr, tt.wantIsOK)
			}
			if msg != tt.wantMsg {
				t.Errorf("DirError() msg = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
	if _, isDirErr := DirError("dir", nil); isDirErr {
		t.Error("DirError() reported a nil error as a directory read failure")
	}
	if _, isDirErr := DirError("dir", errors.Join(fs.ErrNotExist)); isDirErr {
		t.Error("DirError() reported a joined error as a directory read failure")
	}
}
