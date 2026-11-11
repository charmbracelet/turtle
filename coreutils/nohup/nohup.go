// Package nohup implements the POSIX nohup utility.
//
// The runtime cannot control the signals of the utility it runs, so the
// hangup immunity nohup stands for is a no-op: the utility runs exactly as
// it would outside nohup, with only its terminal-bound streams redirected
// as the specification requires.
package nohup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the nohup core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// optSet describes the options of nohup, which takes none of its own: only
// the standard --help and --version, before the utility operand.
var optSet = getopt.Set{}

// usage describes the options of nohup.
const usage = `Usage: nohup COMMAND [ARG]...
  or:  nohup OPTION
Run COMMAND, ignoring hangup signals.

If standard output is a terminal, append output to 'nohup.out' if
possible, '$HOME/nohup.out' otherwise.
If standard error is a terminal, redirect it to standard output.
To save output to FILE, use 'nohup COMMAND > FILE'.

` + stdopts.Tail

// Run redirects the terminal-bound standard streams as the specification
// requires, then invokes the utility operand with the argument operands.
// The exit status of the utility becomes the exit status of nohup, and the
// 127, 126 and 125 statuses of a failed execution are passed along with a
// diagnostic. The context error is returned as-is when the caller's
// context is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "nohup", usage, optSet, args); done {
		return err
	}
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		return getopt.InvalidOption(opt)
	})
	if err != nil {
		return p.usageError(err)
	}
	if len(operands) == 0 {
		return p.usageError(errors.New("missing operand"))
	}
	return p.invoke(ctx, operands[0], operands[1:])
}

// usageError reports a usage error, followed by the standard hint, exiting
// with 125, the status GNU nohup fails with.
func (p *Prog) usageError(err error) error {
	stdopts.Errorf(p.Runtime, "nohup", "%v", err)
	stdopts.TryHelp(p.Runtime, "nohup")
	return &coreutils.ExitError{Code: 125, Err: err}
}

// invoke wires the streams the utility runs with and runs it, closing
// the output file the redirects opened once the run is over. A nil writer
// keeps the runtime stream, as the execution contract defines.
func (p *Prog) invoke(ctx context.Context, utility string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stdout, stderr, closeOut, err := p.redirect()
	if err != nil {
		return err
	}
	defer closeOut()
	if p.Exec == nil {
		stdopts.Errorf(p.Runtime, "nohup", "no command execution available")
		return &coreutils.ExitError{Code: 125}
	}
	status, err := p.Exec(ctx, coreutils.ExecOpts{
		Name:   utility,
		Args:   args,
		Env:    p.EnvList(),
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		stdopts.Errorf(p.Runtime, "nohup", "%v", err)
		return &coreutils.ExitError{Code: status, Err: err}
	}
	if status != 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

// redirect wires the standard streams the utility runs with, as POSIX and
// GNU define: a terminal standard output is appended to nohup.out, a
// terminal standard error goes to the same file, or to the standard output
// when that one is open but not a terminal. It returns the writers to give
// the utility in place of its standard output and error, with a function
// closing the output file it opened; a nil writer keeps the runtime
// stream, and the function is a no-op when no file was opened.
func (p *Prog) redirect() (stdout, stderr io.Writer, closeOut func(), err error) {
	ignoringInput := p.Terminal(0)
	redirectOut, redirectErr := p.Terminal(1), p.Terminal(2)
	stdoutClosed := p.Stdout == nil

	closeOut = func() {}
	var out io.Writer
	if redirectOut || redirectErr && stdoutClosed {
		file, name, err := p.openNohupOut()
		if err != nil {
			return nil, nil, nil, err
		}
		out = file
		closeOut = func() { file.Close() }
		if ignoringInput {
			stdopts.Errorf(p.Runtime, "nohup", "ignoring input and appending output to '%s'", name)
		} else {
			stdopts.Errorf(p.Runtime, "nohup", "appending output to '%s'", name)
		}
	} else if ignoringInput && !redirectErr {
		stdopts.Errorf(p.Runtime, "nohup", "ignoring input")
	}

	if redirectOut {
		stdout = out
	}
	if redirectErr {
		if !redirectOut {
			if ignoringInput {
				stdopts.Errorf(p.Runtime, "nohup", "ignoring input and redirecting standard error to standard output")
			} else {
				stdopts.Errorf(p.Runtime, "nohup", "redirecting standard error to standard output")
			}
		}
		stderr = out
		if stderr == nil {
			stderr = p.Stdout
		}
	}
	return stdout, stderr, closeOut, nil
}

// openNohupOut opens nohup.out in the working directory for appending,
// falling back to $HOME/nohup.out when that fails, as POSIX and GNU
// define. It returns the file and the name the diagnostics report, or an
// error which makes nohup itself fail when neither file can be opened. A
// file is created with the read and write permissions of its owner,
// regardless of the umask.
func (p *Prog) openNohupOut() (fsx.File, string, error) {
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "nohup", "failed to open 'nohup.out': no file system available")
		return nil, "", &coreutils.ExitError{Code: 125}
	}
	const name = "nohup.out"
	f, localErr := p.openAppend(p.FsName(name))
	homeErr := error(nil)
	homeName := ""
	if localErr != nil {
		if home, ok := p.LookupEnv("HOME"); ok {
			homeName = filepath.ToSlash(filepath.Join(home, name))
			f, homeErr = p.openAppend(homeName)
		}
		if f == nil {
			stdopts.Errorf(p.Runtime, "nohup", "failed to open '%s': %v", name, patherr.Unwrap(localErr))
			if homeErr != nil {
				stdopts.Errorf(p.Runtime, "nohup", "failed to open '%s': %v", homeName, patherr.Unwrap(homeErr))
			}
			return nil, "", &coreutils.ExitError{Code: 125}
		}
		return f, homeName, nil
	}
	return f, name, nil
}

// openAppend opens the named file for writing, appending to it and
// creating it with the read and write permissions of its owner. The name
// is used as given, already resolved by the caller.
func (p *Prog) openAppend(name string) (fsx.File, error) {
	return p.FS.OpenFile(
		name,
		os.O_WRONLY|os.O_CREATE|os.O_APPEND,
		0o600,
	)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
