package grep

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"

	"charm.land/turtle/coreutils/internal/glob"
)

// options holds the options of a search.
type options struct {
	mode         byte // 'B', 'E' or 'F'
	output       byte // 'o' lines, 'c' count, 'l' names, 'L' names without match or 'q' quiet
	ignoreCase   bool
	lineNum      bool
	suppress     bool
	invert       bool
	whole        bool
	words        bool
	onlyMatching bool
	byteOffset   bool

	// maxCount stops the search of an input after that many selected
	// lines; a negative one never does.
	maxCount int64

	// filename tells whether the name of an input prefixes the output
	// lines: 0 by default, 1 with -H and -1 with -h.
	filename int8

	label      string
	initialTab bool
	nullName   bool // -Z
	nullData   bool // -z

	after      int64
	before     int64
	groupSep   string
	groupSepNo bool // --no-group-separator

	// binary tells how a binary input is handled: 0, the default,
	// suppresses its output; 'I' assumes no match; 'a' reads it as text.
	binary byte

	// dirs tells how a directory operand is handled: 0, the default, reads
	// it as a file; 's' skips it; 'R' recurses into it.
	dirs byte

	// devices tells how devices are handled: 0, the default, reads the
	// command-line ones and skips the ones met while recursing; 'r' reads
	// them all; 's' skips them all.
	devices byte

	// follow tells how symbolic links are followed while recursing: 0 and
	// 'H' follow the command-line ones only, 'L' follows every one.
	follow byte

	selectors    []selector       // --include and --exclude, in the order given
	excludeFiles []string         // --exclude-from
	excludeDirs  []*regexp.Regexp // --exclude-dir globs

	// color tells when the output is colored: 0 and 'n' never, 'a' when
	// the standard output is a terminal, 'y' always.
	color byte

	patternLists []string
	patternFiles []string
}

// context reports whether any context option is in effect.
func (o options) context() bool {
	return o.output == 'o' && !o.onlyMatching && (o.after > 0 || o.before > 0)
}

// keep reports whether a file is searched, honoring the --include and
// --exclude options. When some of them match, the last one given wins;
// when none does, the file is searched unless the first one includes.
func (o options) keep(name string, depth int) bool {
	if len(o.selectors) == 0 {
		return true
	}
	base := path.Base(name)
	keep := true
	matched := false
	for _, sel := range o.selectors {
		if sel.matches(name, base, depth) {
			keep = sel.include
			matched = true
		}
	}
	if matched {
		return keep
	}
	return !o.selectors[0].include
}

// skipDir reports whether a directory is skipped, honoring --exclude-dir.
func (o options) skipDir(name string, depth int) bool {
	if len(o.excludeDirs) == 0 {
		return false
	}
	return matchesAny(name, depth, o.excludeDirs)
}

// skipDevice reports whether a device, FIFO or socket is skipped, honoring
// -D and the defaults of -r and -R.
func (o options) skipDevice(fi fs.FileInfo, depth int) bool {
	if fi.Mode()&(fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket) == 0 {
		return false
	}
	switch o.devices {
	case 's':
		return true
	case 'r':
		return false
	}
	// The default reads the devices of the command line and the ones met
	// while following every symbolic link, and skips the ones met while
	// recursing with -r.
	return depth > 0 && o.follow != 'L'
}

// matchesAny reports whether a name matches any of the given regular
// expressions, at its base name below the starting points and at any of
// its trailing parts there.
func matchesAny(name string, depth int, res []*regexp.Regexp) bool {
	for _, re := range res {
		if depth > 0 {
			if re.MatchString(path.Base(name)) {
				return true
			}
			continue
		}
		if matchSuffix(re, name) {
			return true
		}
	}
	return false
}

// selector is one --include or --exclude option, in the order given.
type selector struct {
	re      *regexp.Regexp
	include bool
}

// matches reports whether the selector matches a file: at its base name
// below the starting points, and at any of its trailing parts there.
func (s selector) matches(name, base string, depth int) bool {
	if depth > 0 {
		return s.re.MatchString(base)
	}
	return matchSuffix(s.re, name)
}

// matchSuffix reports whether re matches a name or any trailing part of it
// that starts after a slash.
func matchSuffix(re *regexp.Regexp, name string) bool {
	if re.MatchString(name) {
		return true
	}
	for i := 0; i+1 < len(name); i++ {
		if name[i] == '/' && re.MatchString(name[i+1:]) {
			return true
		}
	}
	return false
}

// addSelector compiles and appends one --include or --exclude pattern.
func (o *options) addSelector(pattern string, include bool) error {
	re, err := glob.Compile(pattern, false)
	if err != nil {
		return err
	}
	o.selectors = append(o.selectors, selector{re: re, include: include})
	return nil
}

// count parses the argument of a context or max count option. A max count
// of -1 means infinity.
func count(value string, max bool) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	min := int64(0)
	if max {
		min = -1
	}
	if err != nil || n < min {
		if max {
			return 0, fmt.Errorf("invalid max count '%s'", value)
		}
		return 0, fmt.Errorf("invalid context length argument '%s'", value)
	}
	return n, nil
}

// expandContextArgs rewrites the old-style -NUM context options, which
// getopt can't describe, into their --context=NUM form. It walks the option
// syntax until the first operand, so the argument of a valued option is
// never taken for one.
func expandContextArgs(args []string) []string {
	valuedShort := "efmABCdD"
	valuedLong := map[string]bool{
		"regexp": true, "file": true, "max-count": true,
		"after-context": true, "before-context": true, "context": true,
		"devices": true, "directories": true, "exclude": true,
		"exclude-from": true, "exclude-dir": true, "label": true,
		"binary-files": true, "group-separator": true,
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return append(out, args[i:]...)
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			return append(out, args[i:]...)
		case strings.HasPrefix(arg, "--"):
			out = append(out, arg)
			name, _, given := strings.Cut(arg[2:], "=")
			if !given && valuedLong[name] && i+1 < len(args) {
				i++
				out = append(out, args[i])
			}
		case isContextNum(arg):
			out = append(out, "--context="+arg[1:])
		default:
			// A group of short options ends at a valued one, whose
			// argument may come in the next argument.
			out = append(out, arg)
			for j := 1; j < len(arg); j++ {
				if strings.IndexByte(valuedShort, arg[j]) >= 0 {
					if j == len(arg)-1 && i+1 < len(args) {
						i++
						out = append(out, args[i])
					}
					break
				}
			}
		}
	}
	return out
}

// isContextNum reports whether an argument is an old-style -NUM context
// option.
func isContextNum(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for _, c := range arg[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
