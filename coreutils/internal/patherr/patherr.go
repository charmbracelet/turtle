// Package patherr unwraps the path errors of the file system, so diagnostics
// can name the files themselves.
package patherr

import (
	"errors"
	"io/fs"
)

// Unwrap unwraps *fs.PathError so diagnostics don't repeat the file name.
func Unwrap(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}
