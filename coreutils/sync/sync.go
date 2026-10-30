// Package sync implements the GNU sync utility.
//
// Where the runtime or the platform can't honor a sync method, the closest
// behavior is used instead, down to a successful no-op.
package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the sync core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run flushes the data buffered by the whole file system, or only the files
// named by the operands, each one opened through the runtime file system.
// The specification gives "-" no special meaning, so it names a regular
// file, never the standard input.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "sync", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "sync")
		return &coreutils.ExitError{Code: 2}
	}
	if opts.data && opts.fileSystem {
		p.errorf("cannot specify both --data and --file-system")
		stdopts.TryHelp(p.Runtime, "sync")
		return &coreutils.ExitError{Code: 1}
	}
	if opts.data && len(operands) == 0 {
		p.errorf("--data needs at least one argument")
		stdopts.TryHelp(p.Runtime, "sync")
		return &coreutils.ExitError{Code: 1}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// With -f, everything is flushed with a single global sync: fsx has no
	// syncfs(2) equivalent, and the utility falls back the same way when
	// the system doesn't have the syscall.
	if len(operands) == 0 || opts.fileSystem {
		return p.syncAll()
	}
	ok := true
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !p.syncFile(operand) {
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// syncAll flushes the data buffered by the whole file system, doing nothing
// when the runtime file system doesn't implement fsx.SyncFS, like on
// Windows.
func (p *Prog) syncAll() error {
	fsys, ok := p.FS.(fsx.SyncFS)
	if !ok {
		return nil
	}
	if err := fsys.Sync(); err != nil {
		p.errorf("error syncing: %v", patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// syncFile opens the operand and flushes the data it buffers, reporting
// whether every step succeeded. With -d, the metadata that a plain file
// sync also flushes is kept, as the distinction can't be honored.
func (p *Prog) syncFile(operand string) bool {
	f, err := p.FS.OpenFile(p.resolve(operand), os.O_RDONLY, 0)
	if err != nil {
		p.errorf("error opening '%s': %v", operand, patherr.Unwrap(err))
		return false
	}

	ok := true
	if syncer, canSync := f.(syncer); canSync {
		if err := syncer.Sync(); err != nil {
			p.errorf("error syncing '%s': %v", operand, patherr.Unwrap(err))
			ok = false
		}
	}
	if err := f.Close(); err != nil {
		p.errorf("error closing '%s': %v", operand, patherr.Unwrap(err))
		ok = false
	}
	return ok
}

// syncer is a file whose buffered data can be flushed, like fsync(2) on an
// open file.
type syncer interface {
	Sync() error
}

// resolve converts an operand into the slash-separated path used with the
// file system, relative to the working directory. The path isn't resolved
// lexically: components like "." and ".." and a trailing slash are kept,
// so only the file system resolves them, following symbolic links on the
// way.
func (p *Prog) resolve(operand string) string {
	operand = filepath.ToSlash(operand)
	if p.WorkingDir == "" || filepath.IsAbs(operand) {
		return operand
	}
	wd := filepath.ToSlash(p.WorkingDir)
	if strings.HasSuffix(wd, "/") {
		return wd + operand
	}
	return wd + "/" + operand
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "sync: "+format+"\n", args...)
}

// options holds the flags of sync: -d asks for a data-only sync and -f for
// the sync of the file systems holding the files.
type options struct {
	data       bool
	fileSystem bool
}

// longOptions maps the long options of sync to the short ones they stand
// for.
var longOptions = map[string]rune{
	"data":        'd',
	"file-system": 'f',
}

// optSet describes the options of sync.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of sync.
const usage = `Usage: sync [OPTION] [FILE]...
Synchronize cached writes to persistent storage

  -d, --data
         sync only file data, no unneeded metadata
  -f, --file-system
         sync the file systems that contain the files
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--", and the last state of each flag is the one used.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'd':
			opts.data = true
		case 'f':
			opts.fileSystem = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
