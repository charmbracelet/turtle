// Package output handles the output operands shared by the core utilities:
// creating or appending the files they write, or using the standard output.
package output

import (
	"context"
	"errors"
	"io"
	"os"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/ctxio"
)

// Create opens name for writing, truncating it when it exists, with the
// permission bits of 0666 masked by the umask of rt.
func Create(rt *coreutils.Runtime, ctx context.Context, name string) (io.WriteCloser, error) {
	return open(rt, ctx, name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
}

// Append opens name for appending, creating it when missing, with the
// permission bits of 0666 masked by the umask of rt.
func Append(rt *coreutils.Runtime, ctx context.Context, name string) (io.WriteCloser, error) {
	return open(rt, ctx, name, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
}

// open creates the file of an output operand, wrapping its writes with the
// context, so a cancelled one stops them.
func open(rt *coreutils.Runtime, ctx context.Context, name string, flag int) (io.WriteCloser, error) {
	if rt.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := rt.FS.OpenFile(rt.FsName(name), flag, 0o666&^rt.Umask.Perm())
	if err != nil {
		return nil, err
	}
	return ctxio.WriteCloser{Ctx: ctx, WC: f}, nil
}
