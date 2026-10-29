// Package mktemp implements the GNU mktemp utility.
package mktemp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
)

// Prog is the mktemp core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run creates a temporary file or directory from a template, printing its
// name. The template must hold at least three consecutive X's in its last
// component, which are replaced by random characters, and without one,
// tmp.XXXXXXXXXX is used in the temporary directory.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 1 {
		p.errorf("extra operand '%s'", operands[1])
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	name, err := p.create(ctx, opts, operands)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if !opts.quiet {
			p.errorf("%v", err)
		}
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	fmt.Fprintln(p.Stdout, name)
	return nil
}

// create generates random names from the template until one can be created,
// and returns it. With -u, a name that doesn't exist yet is returned instead,
// and nothing is created.
func (p *Prog) create(ctx context.Context, opts options, operands []string) (string, error) {
	prefix, count, suffix, err := p.template(opts, operands)
	if err != nil {
		return "", err
	}

	what := "file"
	if opts.directory {
		what = "directory"
	}
	umask := p.Umask.Perm()
	for range maxTries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		random, err := randomString(count)
		if err != nil {
			return "", err
		}
		name := prefix + random + suffix
		fspath := p.ResolvePath(name)
		if opts.dryRun {
			_, err := fs.Stat(p.FS, fspath)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return name, nil
			case err != nil:
				return "", err
			}
			continue
		}
		if opts.directory {
			if err := p.FS.Mkdir(fspath, fs.FileMode(0o700)&^umask); err != nil {
				if errors.Is(err, fs.ErrExist) {
					continue
				}
				return "", fmt.Errorf("failed to create %s via template '%s': %v", what, name, patherr.Unwrap(err))
			}
			return name, nil
		}
		f, err := p.FS.OpenFile(fspath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fs.FileMode(0o600)&^umask)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return "", fmt.Errorf("failed to create %s via template '%s': %v", what, name, patherr.Unwrap(err))
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("failed to create %s via template '%s': too many tries", what, prefix+strings.Repeat("X", count)+suffix)
}

// template resolves the name to generate from the operands and options. It
// returns the prefix before the X's of the template, how many X's it holds,
// and the suffix after them. The --suffix value is appended to that suffix
// after the X's are located, so X's it starts with stay literal.
func (p *Prog) template(opts options, operands []string) (string, int, string, error) {
	given := ""
	if len(operands) > 0 {
		given = operands[0]
	}
	tmpl := filepath.ToSlash(given)
	implied := len(operands) == 0
	if implied {
		tmpl = "tmp.XXXXXXXXXX"
	}

	suffix := ""
	if opts.suffixSet {
		if strings.Contains(opts.suffix, "/") {
			return "", 0, "", fmt.Errorf("invalid suffix '%s', contains directory separator", opts.suffix)
		}
		if !strings.HasSuffix(tmpl, "X") {
			return "", 0, "", fmt.Errorf("with --suffix, template '%s' must end in X", given)
		}
		suffix = opts.suffix
	}

	first, end := xRun(tmpl)
	if end-first < minXs || strings.Contains(tmpl[end:], "/") {
		return "", 0, "", fmt.Errorf("too few X's in template '%s'", given)
	}

	dir := ""
	switch {
	case opts.deprecated:
		if strings.Contains(tmpl, "/") {
			return "", 0, "", fmt.Errorf("invalid template '%s', contains directory separator", given)
		}
		dir = p.Getenv("TMPDIR")
		if dir == "" {
			dir = opts.tmpDir
		}
		if dir == "" {
			dir = p.tempDir()
		}
	case opts.tmpDirSet || implied:
		if path.IsAbs(tmpl) || filepath.IsAbs(given) {
			return "", 0, "", fmt.Errorf("template '%s' must not be absolute", given)
		}
		dir = opts.tmpDir
		if dir == "" {
			dir = p.tempDir()
		}
	}
	name := path.Join(dir, tmpl)
	first, end = xRun(name)
	return name[:first], end - first, name[end:] + suffix, nil
}

// tempDir returns the directory for --tmpdir when none is given: TMPDIR, or
// TEMP or TMP, for systems without TMPDIR like Windows, or /tmp.
func (p *Prog) tempDir() string {
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		if dir := p.Getenv(key); dir != "" {
			return dir
		}
	}
	return "/tmp"
}

// xRun returns the bounds of the final run of X's in template, as the index of
// its first X and the index after its last X.
func xRun(tmpl string) (int, int) {
	last := strings.LastIndexByte(tmpl, 'X')
	if last < 0 {
		return 0, 0
	}
	first := last
	for first > 0 && tmpl[first-1] == 'X' {
		first--
	}
	return first, last + 1
}

// randomChars are the characters the X's of a template are replaced with.
const randomChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomString returns n random characters from randomChars.
func randomString(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = randomChars[int(b)%len(randomChars)]
	}
	return string(out), nil
}

// minXs is the number of X's a template must at least hold, and maxTries
// bounds how many random names are tried before giving up.
const (
	minXs    = 3
	maxTries = 1 << 12
)

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "mktemp: "+format+"\n", args...)
}

// options holds the flags of mktemp: -d and -u select what to create, -q
// suppresses diagnostics, -p gives the directory the template is relative to,
// --suffix appends to the template, and -t is the deprecated form of -p with
// worse defaults.
type options struct {
	directory  bool
	dryRun     bool
	quiet      bool
	tmpDirSet  bool
	tmpDir     string
	suffixSet  bool
	suffix     string
	deprecated bool
}

// optSuffix is the rune the long-only --suffix option maps to. Having no
// short equivalent, it's kept above the bytes a group of short options can
// hold.
const optSuffix rune = 0x100

// longOptions maps the long options of mktemp to the short ones they stand
// for.
var longOptions = map[string]rune{
	"directory": 'd',
	"dry-run":   'u',
	"quiet":     'q',
	"tmpdir":    'p',
	"suffix":    optSuffix,
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The -p and --suffix options take their
// option-argument glued to the option or in the next argument, and the last
// one given is the one used. The --tmpdir form of -p may also be given without
// an argument, meaning the default directory.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{
		Valued:   string(optSuffix),
		Optional: "p",
		Long:     longOptions,
	}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'd':
			opts.directory = true
		case 'q':
			opts.quiet = true
		case 'u':
			opts.dryRun = true
		case 'p':
			opts.tmpDirSet, opts.tmpDir = true, value
		case 't':
			opts.deprecated = true
		case optSuffix:
			opts.suffixSet, opts.suffix = true, value
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
