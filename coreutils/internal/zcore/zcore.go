// Package zcore implements the behavior shared by the compression utilities,
// gzip and compress.
//
// The utilities only differ in the options they accept and the defaults they
// use: this package takes care of walking the operands, naming the output
// files, preserving their attributes, and reporting the exit status.
package zcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// Algorithm is a compression algorithm the utilities know about.
type Algorithm int

const (
	// Gzip is the DEFLATE-based format of RFC 1952, with the ".gz" suffix.
	Gzip Algorithm = iota

	// LZW is the format of the original compress, with the ".Z" suffix. It
	// isn't implemented yet, so operands of this format are rejected.
	LZW
)

// Suffix returns the file name suffix associated with the algorithm.
func (a Algorithm) Suffix() string {
	if a == LZW {
		return ".Z"
	}
	return ".gz"
}

// Options are the settings shared by the compression utilities.
type Options struct {
	// Prog names the utility in diagnostic messages.
	Prog string

	// Algorithm is the algorithm used when compressing.
	Algorithm Algorithm

	// Level is the compression level, from 0 to 9. A negative value means the
	// default level and zero means no compression.
	Level int

	// Decompress restores data instead of compressing it.
	Decompress bool

	// Stdout writes to the standard output, leaving files untouched.
	Stdout bool

	// Keep doesn't remove input files after (de)compression.
	Keep bool

	// Force overwrites existing output files, and lets data which isn't
	// compressed pass through when writing to the standard output.
	Force bool

	// RefuseGrowth leaves a file alone when compressing it wouldn't reduce its
	// size, as POSIX compress does.
	RefuseGrowth bool

	// Verbose reports the compression ratio of each file.
	Verbose bool

	// Quiet suppresses diagnostics, keeping the exit status.
	Quiet bool

	// Test checks compressed files for integrity instead of writing anything.
	Test bool

	// Recursive descends into directory operands.
	Recursive bool

	// Suffix overrides the suffix of the algorithm.
	Suffix string

	// SkipSuffixed leaves an operand which already has a compression suffix
	// alone, as the gzip implementations do.
	SkipSuffixed bool

	// StoreName uses the name stored in a compressed file for its output.
	StoreName bool

	// NoName keeps the name and modification time out of compressed files.
	NoName bool
}

// suffix returns the suffix used to name compressed files.
func (o Options) suffix() string {
	if o.Suffix != "" {
		return o.Suffix
	}
	return o.Algorithm.Suffix()
}

// Errors reported for operands which can't be processed.
var (
	errIsDir         = errors.New("is a directory")
	errNoAlgorithm   = errors.New("compression algorithm not supported")
	errNoFS          = errors.New("no file system available")
	errNoStdin       = errors.New("no standard input available")
	errNotCompressed = errors.New("format not recognized")
	errUnknownSuffix = errors.New("unknown suffix -- ignored")
	errUnsupported   = errors.New("unsupported compression format")
)

// run holds the state of a single utility execution.
type run struct {
	rt   *coreutils.Runtime
	opts Options

	// failed and warned tell whether any operand errored or was skipped, which
	// decides the exit status of the utility.
	failed bool
	warned bool
}

// Run processes operands following opts, reading and writing through rt.
//
// The standard input and output are used when no operand is given. It returns
// a [*coreutils.ExitError] when an operand failed or was skipped, and the
// context error when ctx is done.
func Run(ctx context.Context, rt *coreutils.Runtime, opts Options, operands []string) error {
	r := &run{rt: rt, opts: opts}
	if !opts.Decompress && opts.Algorithm == LZW {
		r.fail("", errNoAlgorithm)
		return &coreutils.ExitError{Code: 3, Err: errNoAlgorithm}
	}
	if len(operands) == 0 {
		if err := r.stdio(ctx); err != nil {
			return err
		}
		return r.status()
	}
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.operand(ctx, operand); err != nil {
			return err
		}
	}
	return r.status()
}

// status returns the error matching the problems found so far.
func (r *run) status() error {
	switch {
	case r.failed:
		return &coreutils.ExitError{Code: 1}
	case r.warned:
		return &coreutils.ExitError{Code: 2}
	}
	return nil
}

// operand processes one file operand, or recurses into it when -r is set.
func (r *run) operand(ctx context.Context, operand string) error {
	if operand == "-" {
		return r.stdio(ctx)
	}
	if r.rt.FS == nil {
		r.fail(operand, errNoFS)
		return nil
	}

	name := r.slash(operand)
	fi, err := r.rt.FS.Lstat(name)
	if err != nil && r.opts.Decompress {
		in, _, _ := r.decompressNames(operand)
		name = r.slash(in)
		fi, err = r.rt.FS.Lstat(name)
	}
	if err != nil {
		r.fail(operand, err)
		return nil
	}
	if fi.IsDir() {
		if !r.opts.Recursive {
			r.fail(operand, errIsDir)
			return nil
		}
		return r.walk(ctx, operand, name)
	}
	if !fi.Mode().IsRegular() {
		if r.opts.Decompress && r.opts.Stdout && r.opts.Force {
			return r.decompressToStdout(ctx, operand)
		}
		if !r.opts.Force {
			r.fail(operand, errors.New("not a regular file"))
			return nil
		}
	}

	if r.opts.Decompress {
		if r.opts.Test || r.opts.Stdout {
			return r.decompressToStdout(ctx, operand)
		}
		r.decompressFile(ctx, operand)
		return nil
	}
	if r.opts.Test {
		return r.testFile(ctx, operand)
	}
	if r.opts.Stdout {
		return r.fileToStdout(ctx, operand)
	}
	if r.opts.SkipSuffixed && !r.opts.Force {
		if _, hasSuffix := stripSuffix(operand, r.opts); hasSuffix {
			r.warn(operand, fmt.Errorf("already has %s suffix -- unchanged", path.Ext(operand)))
			return nil
		}
	}
	r.compressFile(ctx, operand)
	return nil
}

// decompressToStdout restores or tests a file without changing it.
func (r *run) decompressToStdout(ctx context.Context, operand string) error {
	input := operand
	if !r.exists(input) && r.exists(input+r.opts.suffix()) {
		input += r.opts.suffix()
	}
	file, err := r.rt.FS.Open(r.slash(input))
	if err != nil {
		r.fail(operand, err)
		return nil
	}
	defer file.Close()
	if r.opts.Test {
		return r.testFileOpen(ctx, operand, file)
	}
	_, err = openSource(file).decode(ctx, r.stdout(), r.opts.Force)
	return r.streamError(ctx, operand, err)
}

// walk processes the entries of a directory operand, as -r asks for. Symbolic
// links are skipped, like the recursive implementations do.
func (r *run) walk(ctx context.Context, operand, name string) error {
	entries, err := fs.ReadDir(r.rt.FS, name)
	if err != nil {
		r.fail(operand, err)
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			continue
		}
		if err := r.operand(ctx, path.Join(operand, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// stdio compresses the standard input to the standard output, or restores it
// when decompressing.
func (r *run) stdio(ctx context.Context) error {
	if r.rt.Stdin == nil {
		r.fail("", errNoStdin)
		return nil
	}

	if !r.opts.Decompress && !r.opts.Test {
		n, err := r.compress(ctx, r.stdout(), r.rt.Stdin, "", time.Time{})
		if err != nil {
			return r.streamError(ctx, "", err)
		}
		if r.opts.Verbose {
			r.reportf("standard input -> standard output (%d bytes)\n", n)
		}
		return nil
	}

	dst := io.Discard
	if !r.opts.Test {
		dst = r.stdout()
	}
	if _, err := openSource(r.rt.Stdin).decode(ctx, dst, r.opts.Force); err != nil {
		return r.streamError(ctx, "", err)
	}
	return nil
}

// fileToStdout writes a file operand to the standard output, compressing it.
func (r *run) fileToStdout(ctx context.Context, operand string) error {
	file, err := r.rt.FS.Open(r.slash(operand))
	if err != nil {
		r.fail(operand, err)
		return nil
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		r.fail(operand, err)
		return nil
	}
	n, err := r.compress(ctx, r.stdout(), file, path.Base(operand), fi.ModTime())
	if err != nil {
		return r.streamError(ctx, operand, err)
	}
	if r.opts.Verbose {
		r.reportf("%s -> standard output (%d bytes)\n", operand, n)
	}
	return nil
}

// testFile decompresses an operand discarding the result, reporting whether it
// is intact.
func (r *run) testFile(ctx context.Context, operand string) error {
	file, err := r.rt.FS.Open(r.slash(operand))
	if err != nil {
		r.fail(operand, err)
		return nil
	}
	defer file.Close()
	return r.testFileOpen(ctx, operand, file)
}

// testFileOpen verifies compressed input already opened by the caller.
func (r *run) testFileOpen(ctx context.Context, operand string, file fs.File) error {
	if _, err := openSource(file).decode(ctx, io.Discard, false); err != nil {
		if stopErr := r.stop(ctx, err); stopErr != nil {
			return stopErr
		}
		r.fail(operand, decodeError(err))
		return nil
	}
	if r.opts.Verbose {
		r.reportf("%s: OK\n", operand)
	}
	return nil
}

// compressFile replaces a file with its compressed form.
func (r *run) compressFile(ctx context.Context, operand string) {
	name := r.slash(operand)
	file, err := r.rt.FS.Open(name)
	if err != nil {
		r.fail(operand, err)
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		r.fail(operand, err)
		return
	}
	out := operand + r.opts.suffix()
	if err := checkName(out); err != nil {
		r.fail(operand, err)
		return
	}
	if r.exists(out) && !r.opts.Force {
		r.fail(operand, fmt.Errorf("cannot overwrite %s: file exists", out))
		return
	}

	dst, err := r.rt.FS.OpenFile(r.slash(out), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		r.fail(operand, err)
		return
	}
	n, err := r.compress(ctx, dst, file, path.Base(operand), fi.ModTime())
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		r.remove(out)
		if stopErr := r.stop(ctx, err); stopErr != nil {
			return
		}
		r.fail(operand, err)
		return
	}

	if r.opts.RefuseGrowth && !r.opts.Force && n >= fi.Size() {
		r.remove(out)
		r.warn(operand, errors.New("not compressed: larger than original"))
		return
	}

	r.copyAttrs(out, fi)
	if !r.opts.Keep {
		if err := r.rt.FS.Remove(name); err != nil {
			if !r.opts.Force {
				r.remove(out)
				r.fail(operand, err)
				return
			}
			r.warn(operand, err)
		}
	}
	if r.opts.Verbose {
		r.reportf("%s: %s -- replaced with %s\n", operand, ratio(n, fi.Size()), out)
	}
}

// decompressFile restores a compressed file, replacing it with its original
// name.
func (r *run) decompressFile(ctx context.Context, operand string) {
	in, out, err := r.decompressNames(operand)
	if err != nil {
		r.fail(operand, err)
		return
	}

	file, err := r.rt.FS.Open(r.slash(in))
	if err != nil {
		r.fail(operand, err)
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		r.fail(operand, err)
		return
	}
	src := openSource(file)
	if src.algorithm != Gzip {
		if src.algorithm == LZW {
			r.fail(operand, errUnsupported)
		} else {
			r.fail(operand, errNotCompressed)
		}
		return
	}
	if r.opts.StoreName {
		if err := src.open(); err != nil {
			r.fail(operand, decodeError(err))
			return
		}
		if src.storedName != "" {
			if !validStoredName(src.storedName) {
				r.fail(operand, errors.New("unsafe stored file name"))
				return
			}
			out = path.Join(path.Dir(in), src.storedName)
		}
	}
	if err := checkName(out); err != nil {
		r.fail(operand, err)
		return
	}
	if out == in {
		r.fail(operand, errors.New("cannot overwrite compressed file"))
		return
	}
	if r.exists(out) && !r.opts.Force {
		r.fail(operand, fmt.Errorf("cannot overwrite %s: file exists", out))
		return
	}

	dst, err := r.rt.FS.OpenFile(r.slash(out), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		r.fail(operand, err)
		return
	}
	n, err := src.decode(ctx, dst, false)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		r.remove(out)
		if stopErr := r.stop(ctx, err); stopErr != nil {
			return
		}
		r.fail(operand, decodeError(err))
		return
	}

	r.copyAttrs(out, fi)
	if !r.opts.Keep {
		if err := r.rt.FS.Remove(r.slash(in)); err != nil {
			if !r.opts.Force {
				r.remove(out)
				r.fail(operand, err)
				return
			}
			r.warn(operand, err)
		}
	}
	if r.opts.Verbose {
		r.reportf("%s -> %s (%d bytes)\n", operand, out, n)
	}
}

// decompressNames returns the file to read and the restored name. An operand
// with a recognized compression suffix is treated as an input; otherwise its
// algorithm suffix is appended to find the input.
func (r *run) decompressNames(operand string) (in, out string, err error) {
	if stripped, ok := stripSuffix(operand, r.opts); ok && r.exists(operand) {
		return operand, stripped, nil
	}
	if suffixed := operand + r.opts.suffix(); r.exists(suffixed) {
		return suffixed, operand, nil
	}
	if r.exists(operand) {
		return "", "", errUnknownSuffix
	}
	return operand, operand, nil
}

// compress writes the compressed form of src into dst, returning how many
// compressed bytes were written. The name and modification time are stored
// unless -n is given, and when there is no file to name.
func (r *run) compress(ctx context.Context, dst io.Writer, src io.Reader, name string, mtime time.Time) (int64, error) {
	counter := &countingWriter{w: dst}
	writer, err := gzipWriter(counter, r.opts.Level)
	if err != nil {
		return 0, err
	}
	if !r.opts.NoName && storeName(name) {
		writer.Name = name
		writer.ModTime = mtime
	}
	_, err = copyCtx(ctx, writer, src)
	if cerr := writer.Close(); err == nil {
		err = cerr
	}
	return counter.n, err
}

// copyAttrs gives the file at operand the mode, times and owner of fi, which
// the specifications ask for when the process has enough privileges to do it.
// Failures are ignored: they usually mean the file system doesn't support the
// operation.
func (r *run) copyAttrs(operand string, fi fs.FileInfo) {
	name := r.slash(operand)
	_ = r.rt.FS.Chmod(name, fi.Mode())
	atime, ok := fsx.Atime(fi)
	if !ok {
		atime = fi.ModTime()
	}
	_ = r.rt.FS.Chtimes(name, atime, fi.ModTime())
	if uid, gid, ok := fsx.Owner(fi); ok {
		if chownFS, ok := r.rt.FS.(fsx.ChownFS); ok {
			_ = chownFS.Chown(name, uid, gid)
		}
	}
}

// exists reports whether a file can be statted at operand.
func (r *run) exists(operand string) bool {
	_, err := r.rt.FS.Lstat(r.slash(operand))
	return err == nil
}

// remove drops a file created by an operation which then failed, ignoring
// errors: the caller already reported the failure.
func (r *run) remove(operand string) {
	_ = r.rt.FS.Remove(r.slash(operand))
}

// stdout writes the standard output, marking failures to write it.
func (r *run) stdout() io.Writer {
	return stdoutWriter{w: r.rt.Stdout}
}

// stop returns an error which aborts the whole run, or nil when the error only
// concerns the operand being processed.
func (r *run) stop(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if writeErr, ok := errors.AsType[*writeError](err); ok {
		r.failed = true
		return &coreutils.ExitError{Code: 1, Err: writeErr.err}
	}
	return nil
}

// streamError reports a stream error against operand, stopping only when the
// context is done or a write to stdout failed.
func (r *run) streamError(ctx context.Context, operand string, err error) error {
	if stopErr := r.stop(ctx, err); stopErr != nil {
		return stopErr
	}
	if err != nil {
		r.fail(operand, decodeError(err))
	}
	return nil
}

// writeError marks a failure to write the standard output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}

// stdoutWriter writes to the standard output, reporting failures as
// [writeError] so they stop the run.
type stdoutWriter struct {
	w io.Writer
}

// Write implements io.Writer.
func (o stdoutWriter) Write(p []byte) (int, error) {
	n, err := o.w.Write(p)
	if err != nil {
		return n, &writeError{err: err}
	}
	return n, nil
}

// fail reports an error for operand, marking the run as failed.
func (r *run) fail(operand string, err error) {
	r.failed = true
	if r.opts.Quiet {
		return
	}
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		err = pathErr.Err
	}
	fmt.Fprintf(r.rt.Stderr, "%s: %s\n", r.opts.Prog, label(operand, err))
}

// warn reports a skipped operand or a problem which isn't fatal, marking the
// run as warned.
func (r *run) warn(operand string, err error) {
	r.warned = true
	if r.opts.Quiet {
		return
	}
	fmt.Fprintf(r.rt.Stderr, "%s: %s\n", r.opts.Prog, label(operand, err))
}

// reportf writes a message to the standard error for the -v option.
func (r *run) reportf(format string, args ...any) {
	if r.opts.Quiet {
		return
	}
	fmt.Fprintf(r.rt.Stderr, format, args...)
}

// label formats a diagnostic message, prefixing it with the operand it's about.
func label(operand string, err error) string {
	if operand == "" {
		return err.Error()
	}
	return operand + ": " + err.Error()
}

// slash resolves an operand into a name usable with the file system.
func (r *run) slash(operand string) string {
	return filepath.ToSlash(r.rt.ResolvePath(operand))
}
