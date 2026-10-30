// Package truncate implements the GNU truncate utility.
package truncate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the truncate core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run sets or adjusts the size of each file, as the -s or -r options say.
// Files that don't exist are created, unless -c is given. A file that fails
// doesn't keep the ones after it from being processed, and the first
// failure yields an exit status of 1.
//
// Run is interrupted when ctx is done between files, and the context error
// is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "truncate", usage, optSet, args); done {
		return err
	}
	opts, files, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "truncate")
		return &coreutils.ExitError{Code: 2}
	}
	if len(files) == 0 {
		p.errorf("missing file operand")
		stdopts.TryHelp(p.Runtime, "truncate")
		return &coreutils.ExitError{Code: 2}
	}
	if !opts.sizeGiven && !opts.referenceGiven {
		p.errorf("you must specify either --size or --reference")
		stdopts.TryHelp(p.Runtime, "truncate")
		return &coreutils.ExitError{Code: 2}
	}
	if opts.referenceGiven && opts.sizeGiven && opts.size.adjust == adjustSet {
		p.errorf("you must specify a relative --size with --reference")
		stdopts.TryHelp(p.Runtime, "truncate")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	spec, refSize, err := p.resolveSize(opts)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}

	ok := true
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.truncateFile(file, opts.noCreate, spec, refSize); err != nil {
			p.errorf("%v", err)
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// resolveSize resolves the size spec and the reference size of the options,
// statting the -r file when one was given. Without -s, the reference size
// is the size files are set to; with a relative one, it's their base.
func (p *Prog) resolveSize(opts options) (sizeSpec, int64, error) {
	if !opts.referenceGiven {
		return opts.size, -1, nil
	}
	fi, err := fs.Stat(p.FS, p.path(opts.reference))
	if err != nil {
		return sizeSpec{}, 0, fmt.Errorf("cannot stat %s: %v", opts.reference, patherr.Unwrap(err))
	}
	if !opts.sizeGiven {
		return sizeSpec{size: fi.Size()}, 0, nil
	}
	return opts.size, fi.Size(), nil
}

// truncateFile sets the size of a single file, computed by applying the
// spec to the -r reference size when refSize isn't negative, and to the
// current size of the file otherwise. A missing file is created, unless
// noCreate says it must be skipped instead.
func (p *Prog) truncateFile(name string, noCreate bool, spec sizeSpec, refSize int64) error {
	path := p.path(name)
	fi, err := fs.Stat(p.FS, path)
	if errors.Is(err, fs.ErrNotExist) {
		if noCreate {
			return nil
		}
		fi = nil // the file is created from an empty start
	} else if err != nil {
		return fmt.Errorf("cannot stat %s: %v", name, patherr.Unwrap(err))
	}

	base := refSize
	if base < 0 {
		base = 0
		if fi != nil {
			base = fi.Size()
		}
	}
	size := int64(0)
	if fi != nil {
		size = fi.Size()
	}
	want := spec.apply(base)
	if want == size && fi != nil {
		return nil
	}
	if want > size {
		return p.extend(name, path, want)
	}
	return p.rewrite(name, path, fi != nil, want)
}

// extend grows the file at path to want bytes, creating it when it doesn't
// exist. The bytes between the old end and the new one read as zeroes.
func (p *Prog) extend(name, path string, want int64) error {
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE, createPerm(p.Umask))
	if err != nil {
		return openErr(name, err)
	}
	if sk, ok := f.(fsx.SeekFile); ok {
		defer f.Close()
		// A single zero is written at the last byte, so the space in
		// between is left as a hole when the file system supports it.
		if _, err := sk.Seek(want-1, io.SeekStart); err != nil {
			return truncateErr(name, want, err)
		}
		if _, err := f.Write([]byte{0}); err != nil {
			return truncateErr(name, want, err)
		}
		return nil
	}
	f.Close()
	return p.extendAppend(name, path, want)
}

// extendAppend grows the file at path to want bytes by appending zeroes to
// it, for a file whose offset can't be sought.
func (p *Prog) extendAppend(name, path string, want int64) error {
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, createPerm(p.Umask))
	if err != nil {
		return openErr(name, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat %s: %v", name, patherr.Unwrap(err))
	}
	if _, err := io.CopyN(f, zeroReader{}, want-fi.Size()); err != nil {
		return truncateErr(name, want, err)
	}
	return nil
}

// rewrite shrinks the file at path to want bytes, or creates it with that
// size when it doesn't exist. Its kept bytes are first read back, then
// written over the file through a truncating open, since fsx has no
// truncate operation of its own.
func (p *Prog) rewrite(name, path string, exists bool, want int64) error {
	kept := []byte{}
	if exists {
		var err error
		if kept, err = p.readPrefix(path, want); err != nil {
			return fmt.Errorf("cannot read %s: %v", name, patherr.Unwrap(err))
		}
	}
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, createPerm(p.Umask))
	if err != nil {
		return openErr(name, err)
	}
	defer f.Close()
	if _, err := f.Write(kept); err != nil {
		return truncateErr(name, want, err)
	}
	return nil
}

// readPrefix reads at most n bytes of the file at path.
func (p *Prog) readPrefix(path string, n int64) ([]byte, error) {
	f, err := p.FS.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, n))
}

// zeroReader is an endless source of zeroes.
type zeroReader struct{}

// Read implements the io.Reader interface.
func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// createPerm returns the permissions new files are created with.
func createPerm(umask fs.FileMode) fs.FileMode {
	return 0o666 &^ umask.Perm()
}

// openErr wraps an error from opening a file for writing.
func openErr(name string, err error) error {
	return fmt.Errorf("cannot open %s for writing: %v", name, patherr.Unwrap(err))
}

// truncateErr wraps an error from setting the size of a file.
func truncateErr(name string, want int64, err error) error {
	return fmt.Errorf("failed to truncate %s at %d bytes: %v", name, want, patherr.Unwrap(err))
}

// path resolves a path operand for the runtime file system.
func (p *Prog) path(name string) string {
	return filepath.ToSlash(p.ResolvePath(name))
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "truncate: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	noCreate  bool
	reference string
	// referenceGiven tells whether -r was given, so an empty name is
	// distinct from an omitted option.
	referenceGiven bool
	size           sizeSpec
	sizeGiven      bool
}

// longOptions maps the long options of truncate to the short ones they
// stand for.
var longOptions = map[string]rune{
	"no-create": 'c',
	"reference": 'r',
	"size":      's',
}

// optSet describes the options of truncate.
var optSet = getopt.Set{Valued: "rs", Long: longOptions}

// usage describes the options of truncate.
const usage = `Usage: truncate OPTION... FILE...
Shrink or extend the size of each FILE to the specified size

Any FILE that does not exist is created. If a FILE is larger than the
specified size, the extra data is lost; if it is shorter, it is extended
and the extended part reads as zero bytes.

  -c, --no-create        do not create any files
  -r, --reference=FILE   use FILE as a reference size
  -s, --size=SIZE        set or adjust the file size by SIZE; SIZE may be
                        relative to the size of each file, with a leading
                        '+', '-', '<', '>', '/' or '%', and may hold a
                        multiplier suffix
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--". The last -s or -r option given is the one used.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	files, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c':
			opts.noCreate = true
		case 'r':
			opts.reference, opts.referenceGiven = value, true
		case 's':
			spec, err := parseSize(value)
			if err != nil {
				return err
			}
			opts.size, opts.sizeGiven = spec, true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, files, err
}

// adjust is the way a -s size adjusts the size of a file.
type adjust int

const (
	adjustSet       adjust = iota // set the size as it is
	adjustExtend                  // extend by
	adjustReduce                  // reduce by
	adjustAtMost                  // at most
	adjustAtLeast                 // at least
	adjustRoundDown               // round down to a multiple of
	adjustRoundUp                 // round up to a multiple of
)

// sizeSpec is a parsed -s option-argument: a size and its adjustment.
type sizeSpec struct {
	adjust adjust
	size   int64
}

// apply returns the size a file becomes when the spec adjusts base.
func (s sizeSpec) apply(base int64) int64 {
	var want int64
	switch s.adjust {
	case adjustSet:
		want = s.size
	case adjustExtend:
		want = add(base, s.size)
	case adjustReduce:
		want = base - s.size
	case adjustAtMost:
		want = min(base, s.size)
	case adjustAtLeast:
		want = max(base, s.size)
	case adjustRoundDown:
		want = base / s.size * s.size
	case adjustRoundUp:
		want = add(base, s.size-1) / s.size * s.size
	}
	if want < 0 {
		want = 0 // the size of a file can't be negative
	}
	return want
}

// add returns a+b, saturated at the maximum int64.
func add(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

// errMultipleModifiers reports a -s size which combines two of its
// relative prefixes, like "+/5".
var errMultipleModifiers = errors.New("multiple relative modifiers specified")

// parseSize parses the -s option-argument: a decimal integer with an
// optional multiplier suffix, and optionally relative to the size of each
// file through a leading adjustment prefix.
func parseSize(value string) (sizeSpec, error) {
	spec := sizeSpec{adjust: adjustSet}
	s := strings.TrimLeft(value, " \t")
	switch {
	case strings.HasPrefix(s, "<"):
		spec.adjust, s = adjustAtMost, s[1:]
	case strings.HasPrefix(s, ">"):
		spec.adjust, s = adjustAtLeast, s[1:]
	case strings.HasPrefix(s, "/"):
		spec.adjust, s = adjustRoundDown, s[1:]
	case strings.HasPrefix(s, "%"):
		spec.adjust, s = adjustRoundUp, s[1:]
	}
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		if spec.adjust != adjustSet {
			return spec, errMultipleModifiers
		}
		spec.adjust = adjustExtend
		if s[0] == '-' {
			spec.adjust = adjustReduce
		}
		s = strings.TrimLeft(s[1:], " \t")
	}

	digits := s
	for digits != "" && digits[0] >= '0' && digits[0] <= '9' {
		digits = digits[1:]
	}
	n, err := strconv.ParseInt(s[:len(s)-len(digits)], 10, 64)
	if err != nil {
		return spec, invalidNumber(value)
	}
	mult, err := multiplier(digits)
	if err != nil || mult > 1 && n > 0 && n > math.MaxInt64/mult {
		return spec, invalidNumber(value)
	}
	spec.size = n * mult
	if (spec.adjust == adjustRoundDown || spec.adjust == adjustRoundUp) && spec.size == 0 {
		return spec, errors.New("division by zero")
	}
	return spec, nil
}

// invalidNumber reports a -s size which doesn't hold a valid number.
func invalidNumber(value string) error {
	return fmt.Errorf("invalid number: '%s'", value)
}

// suffixPowers maps the multiplier suffix letters to the power they stand
// for, so that 'M' is the second power of 1024 and 'MB' the second of 1000.
var suffixPowers = map[byte]int{
	'k': 1, 'm': 2, 'g': 3, 't': 4, 'p': 5,
	'e': 6, 'z': 7, 'y': 8, 'r': 9, 'q': 10,
}

// multiplier returns the multiplier a size suffix stands for, as "K" for
// 1024, "KB" for 1000 and "KiB" for 1024 again. The letters may be given
// in any case.
func multiplier(suffix string) (int64, error) {
	if suffix == "" {
		return 1, nil
	}
	s := strings.ToLower(suffix)
	base, power := int64(1), 0
	switch {
	case len(s) == 1:
		base, power = 1024, suffixPowers[s[0]]
	case len(s) == 2 && s[1] == 'b':
		base, power = 1000, suffixPowers[s[0]]
	case len(s) == 3 && s[1] == 'i' && s[2] == 'b':
		base, power = 1024, suffixPowers[s[0]]
	default:
		return 0, invalidNumber(suffix)
	}
	if power == 0 {
		return 0, invalidNumber(suffix)
	}
	return intPow(base, power)
}

// intPow returns base**exp, or an error when it overflows.
func intPow(base int64, exp int) (int64, error) {
	result := int64(1)
	for range exp {
		if result > math.MaxInt64/base {
			return 0, errors.New("overflow")
		}
		result *= base
	}
	return result, nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
