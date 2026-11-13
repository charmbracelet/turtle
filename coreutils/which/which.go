// Package which implements the GNU which utility.
//
// It searches each directory of the PATH for executables using the host
// file system, like the lookup of the coreutils package runs them. The
// aliases and shell functions the GNU utility also reports are not
// modeled: the options asking for them are accepted and ignored.
package which

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// defaultPath is the search path used when PATH is unset, matching the
// unexported default of the coreutils package.
const defaultPath = "/bin:/usr/bin"

// Prog is the which core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run prints the path of each command operand, searching the directories
// of the PATH. It exits with the number of commands it couldn't find, or
// with status 255 when no command is given.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "which", usage, optSet, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opts, operands, err := parseFlags(p.Terminal(1), args)
	if err != nil {
		stdopts.Errorf(p.Runtime, "which", "%v", err)
		stdopts.TryHelp(p.Runtime, "which")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if opts.version {
		return p.printVersion()
	}
	if len(operands) == 0 {
		// Without a command, GNU which prints the usage text to the
		// standard error and exits with the status -1, which the shell
		// reports as 255.
		if _, err := io.WriteString(p.Stderr, usage); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
		return &coreutils.ExitError{Code: 255}
	}
	return p.locate(ctx, opts, operands)
}

// locate prints the matches for every command operand, exiting with the
// number of commands which had none.
func (p *Prog) locate(ctx context.Context, opts options, operands []string) error {
	path, ok := p.LookupEnv("PATH")
	if !ok {
		path = defaultPath
	}
	wd, err := filepath.Abs(p.WorkingDir)
	if err != nil {
		wd = p.WorkingDir
	}
	wd = withTrailingSep(wd)
	home := ""
	if opts.skipTilde || opts.showTilde {
		home = withTrailingSep(p.Getenv("HOME"))
	}
	fails := 0
	for _, arg := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		found, err := p.locateOne(opts, filepath.ToSlash(arg), path, wd, home)
		if err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
		if !found {
			fails++
		}
	}
	if fails > 0 {
		return &coreutils.ExitError{Code: fails}
	}
	return nil
}

// locateOne prints the matches for a single command operand, reporting
// whether any was found. Names holding a path separator are checked as
// they are, relative to the working directory; the rest are searched in
// every directory of the path, in order.
func (p *Prog) locateOne(opts options, name, path, wd, home string) (bool, error) {
	if strings.ContainsRune(name, '/') {
		return p.locatePath(opts, name, wd, home)
	}
	return p.searchPath(opts, name, path, wd, home)
}

// locatePath checks a name holding a path separator against the working
// directory, the way the lookup of the coreutils package runs it, and
// prints the file it points to when it can be run. Like the GNU utility,
// a plain relative name is searched as "./name", and the diagnostic for a
// missing one names the directory as it was typed.
func (p *Prog) locatePath(opts options, name, wd, home string) (bool, error) {
	typed := name
	if !filepath.IsAbs(name) && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "~") {
		typed = "./" + name
	}
	i := strings.LastIndexByte(typed, '/')
	dir, base := ".", typed
	if i >= 0 {
		dir, base = typed[:i], typed[i+1:]
	}
	searchDir := dir
	if !filepath.IsAbs(searchDir) {
		searchDir = filepath.Join(wd, searchDir)
	}
	for _, candidate := range candidates(searchDir, base) {
		if !statExec(candidate) {
			continue
		}
		if opts.skipTilde && under(candidate, home) {
			continue
		}
		return true, p.println(show(opts, candidate, wd, home, strings.HasPrefix(dir, ".")))
	}
	p.fail(base, dir)
	return false, nil
}

// searchPath prints the matches for name in the directories of the path,
// in order, reporting whether any was found.
func (p *Prog) searchPath(opts options, name, path, wd, home string) (bool, error) {
	dirs := filepath.SplitList(path)
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	found := false
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		if opts.skipTilde && strings.HasPrefix(dir, "~") {
			continue
		}
		if opts.skipDot && !filepath.IsAbs(dir) {
			continue
		}
		dot := strings.HasPrefix(dir, ".")
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(wd, dir)
		}
		for _, candidate := range candidates(dir, name) {
			if !statExec(candidate) {
				continue
			}
			if opts.skipTilde && under(candidate, home) {
				continue
			}
			if err := p.println(show(opts, candidate, wd, home, dot)); err != nil {
				return true, err
			}
			found = true
			if !opts.all {
				return true, nil
			}
		}
	}
	if !found {
		p.fail(name, path)
	}
	return found, nil
}

// show renders a found path for printing: a match reached through a dot
// entry of the PATH and lying under the working directory is abbreviated
// to a leading dot with --show-dot, and one under the HOME directory to a
// leading tilde with --show-tilde.
func show(opts options, path, wd, home string, dot bool) string {
	if opts.showDot && dot && under(path, wd) {
		return "." + string(filepath.Separator) + path[len(wd):]
	}
	if opts.showTilde && under(path, home) {
		return "~" + string(filepath.Separator) + path[len(home):]
	}
	return path
}

// candidates returns the paths a command may be found as in a directory.
// On Windows, where any file found on the PATH is accepted, the .exe
// extension is tried first, like the lookup of the coreutils package.
func candidates(dir, name string) []string {
	if runtime.GOOS == "windows" {
		return []string{filepath.Join(dir, name+".exe"), filepath.Join(dir, name)}
	}
	return []string{filepath.Join(dir, name)}
}

// statExec reports whether the file at path can be run, as far as the
// host can tell: it exists, isn't a directory, and on Unix carries an
// executable permission bit.
func statExec(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o0111 != 0
}

// under reports whether path lies in dir, which is normalized with a
// trailing separator.
func under(path, dir string) bool {
	return dir != "" && len(path) > len(dir) && strings.HasPrefix(path, dir)
}

// withTrailingSep normalizes a directory with a trailing separator, so
// paths under it can be matched with a prefix.
func withTrailingSep(dir string) string {
	if dir == "" || strings.HasSuffix(dir, string(filepath.Separator)) {
		return dir
	}
	return dir + string(filepath.Separator)
}

// println writes a found path to the standard output.
func (p *Prog) println(path string) error {
	_, err := fmt.Fprintln(p.Stdout, path)
	return err
}

// fail writes the diagnostic for a command which wasn't found.
func (p *Prog) fail(name, path string) {
	stdopts.Errorf(p.Runtime, "which", "no %s in (%s)", name, path)
}

// printVersion writes the version text, like the shared standard options.
func (p *Prog) printVersion() error {
	_, err := fmt.Fprint(p.Stdout, stdopts.VersionText("which"))
	if err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// options holds the flags of which.
type options struct {
	all       bool
	skipDot   bool
	skipTilde bool
	showDot   bool
	showTilde bool
	// ttyOnly holds whether the cosmetic options are disabled, because
	// the standard output is not a terminal and --tty-only was given.
	ttyOnly bool
	version bool
}

// parseFlags parses the options of which, given whether the standard
// output is a terminal for the --tty-only option.
func parseFlags(tty bool, args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'a':
			opts.all = true
		case 'v', 'V':
			opts.version = true
		case optReadAlias, 'i', optSkipAlias, optReadFunctions, optSkipFunctions:
			// Aliases and shell functions are not modeled, so the options
			// reading them have nothing to do.
		case optTTYOnly:
			opts.ttyOnly = !tty
		case optSkipDot:
			opts.skipDot = !opts.ttyOnly
		case optSkipTilde:
			opts.skipTilde = !opts.ttyOnly
		case optShowDot:
			opts.showDot = !opts.ttyOnly
		case optShowTilde:
			opts.showTilde = !opts.ttyOnly
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// The long options of which map to runes above the range a single-byte
// short option can reach, so they stay long-only, as in the GNU utility.
const (
	optReadAlias rune = 0x100 + iota
	optSkipAlias
	optReadFunctions
	optSkipFunctions
	optSkipDot
	optSkipTilde
	optShowDot
	optShowTilde
	optTTYOnly
)

// longOptions maps the long options of which to the runes they stand for.
var longOptions = map[string]rune{
	"all":            'a',
	"read-alias":     optReadAlias,
	"skip-alias":     optSkipAlias,
	"read-functions": optReadFunctions,
	"skip-functions": optSkipFunctions,
	"skip-dot":       optSkipDot,
	"skip-tilde":     optSkipTilde,
	"show-dot":       optShowDot,
	"show-tilde":     optShowTilde,
	"tty-only":       optTTYOnly,
}

// optSet describes the options of which.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of which.
const usage = `Usage: which [OPTION]... [COMMAND]...
Search each directory of the PATH for an executable file named COMMAND,
printing the path of the first match, or of every match with -a.

  -a, --all
         print all matching executables in PATH, not just the first
  -i, --read-alias
         accepted but ignored: aliases are not modeled
      --skip-alias
         accepted but ignored: aliases are not modeled
      --read-functions
         accepted but ignored: shell functions are not modeled
      --skip-functions
         accepted but ignored: shell functions are not modeled
      --skip-dot
         skip the PATH directories which are not absolute
      --skip-tilde
         skip PATH directories starting with a tilde, and executables
         which reside in the HOME directory
      --show-dot
         print a dot instead of the current directory in the output
      --show-tilde
         print a tilde instead of the HOME directory in the output
      --tty-only
         stop processing options on the right if not on a tty
` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
