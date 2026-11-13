package find

import (
	"errors"
	"fmt"
	"io/fs"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/bre"
	"charm.land/turtle/coreutils/internal/ere"
	"charm.land/turtle/coreutils/internal/filemode"
	"charm.land/turtle/coreutils/internal/glob"
	"charm.land/turtle/coreutils/internal/gnudate"
	"charm.land/turtle/coreutils/internal/walk"
)

// parseExpr parses the tokens of the expression of find.
func parseExpr(rt *coreutils.Runtime, now time.Time, tokens []string, follow byte) (expr, error) {
	p := &parser{rt: rt, now: now, tokens: tokens, follow: follow}
	e, err := p.parseList()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected predicate '%s'", p.tokens[p.pos])
	}
	return e, nil
}

// parser reads the tokens of an expression, one at a time.
type parser struct {
	rt     *coreutils.Runtime
	now    time.Time
	tokens []string
	pos    int
	bre    bool // the regular expressions are basic, from -regextype
	follow byte // how the reference files are followed, from -H, -L or -P
}

// peek returns the current token without consuming it.
func (p *parser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

// next consumes and returns the current token.
func (p *parser) next() string {
	tok := p.peek()
	if tok != "" {
		p.pos++
	}
	return tok
}

// parseList parses expressions separated by the comma operator, which both
// sides of are always evaluated.
func (p *parser) parseList() (expr, error) {
	left, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	for p.peek() == "," {
		p.next()
		right, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		left = listExpr{left, right}
	}
	return left, nil
}

// parseOr parses expressions separated by the -o operator.
func (p *parser) parseOr() (expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "-o" || p.peek() == "-or" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orExpr{left, right}
	}
	return left, nil
}

// parseAnd parses expressions separated by the -a operator or by
// juxtaposition.
func (p *parser) parseAnd() (expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek() {
		case "", ")", "-o", "-or", ",":
			return left, nil
		case "-a", "-and":
			p.next()
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = andExpr{left, right}
	}
}

// parseNot parses a negated expression, which may be negated again.
func (p *parser) parseNot() (expr, error) {
	switch p.peek() {
	case "!", "-not":
		name := "!"
		if p.peek() == "-not" {
			name = "-not"
		}
		p.next()
		if p.peek() == "" {
			return nil, fmt.Errorf("missing expression after '%s'", name)
		}
		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return notExpr{inner}, nil
	}
	return p.parsePrimary()
}

// parsePrimary parses one primary, or a parenthesized expression.
func (p *parser) parsePrimary() (expr, error) {
	switch tok := p.next(); tok {
	case "":
		return nil, errors.New("missing expression")
	case ")":
		return nil, errors.New("unexpected ')'")
	case ",":
		return nil, errors.New("unexpected ','")
	case "(":
		inner, err := p.parseList()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, errors.New("missing ')'")
		}
		return inner, nil
	case "-a", "-and":
		return nil, fmt.Errorf("invalid expression; '%s' with nothing before it", tok)
	case "-o", "-or":
		return nil, fmt.Errorf("invalid expression; '%s' with nothing before it", tok)
	case "-true":
		return trueExpr{}, nil
	case "-false":
		return falseExpr{}, nil
	case "-name", "-iname":
		pattern, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		re, err := glob.Compile(pattern, tok == "-iname")
		if err != nil {
			return nil, err
		}
		return nameExpr{re}, nil
	case "-path", "-wholename", "-ipath", "-iwholename":
		pattern, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		re, err := glob.Compile(pattern, tok == "-ipath" || tok == "-iwholename")
		if err != nil {
			return nil, err
		}
		return pathMatch{re}, nil
	case "-lname", "-ilname":
		pattern, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		re, err := glob.Compile(pattern, tok == "-ilname")
		if err != nil {
			return nil, err
		}
		return linkMatch{re}, nil
	case "-regex", "-iregex":
		pattern, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		re, err := p.compileRegex(pattern, tok == "-iregex")
		if err != nil {
			return nil, err
		}
		return regexMatch{re}, nil
	case "-regextype":
		return p.parseRegexType()
	case "-nouser":
		return nouserExpr{}, nil
	case "-nogroup":
		return nogroupExpr{}, nil
	case "-mount":
		return mountExpr{}, nil
	case "-xdev":
		return xdevExpr{}, nil
	case "-noleaf":
		// No optimization to turn off is done.
		return trueExpr{}, nil
	case "-prune":
		return pruneExpr{}, nil
	case "-quit":
		return quitExpr{}, nil
	case "-depth", "-d":
		return depthExpr{}, nil
	case "-daystart":
		return daystartExpr{}, nil
	case "-follow":
		return followExpr{}, nil
	case "-ignore_readdir_race":
		return ignoreRaceExpr{}, nil
	case "-noignore_readdir_race":
		return noIgnoreRaceExpr{}, nil
	case "-maxdepth":
		return p.parseDepth(tok)
	case "-mindepth":
		return p.parseDepth(tok)
	case "-empty":
		return emptyExpr{}, nil
	case "-delete":
		return deleteExpr{}, nil
	case "-print":
		return printExpr{}, nil
	case "-print0":
		return printExpr{zero: true}, nil
	case "-fprint", "-fprint0":
		name, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		return printExpr{zero: tok == "-fprint0", file: name}, nil
	case "-ls", "-fls":
		if tok == "-ls" {
			return lsExpr{}, nil
		}
		name, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		return lsExpr{file: name}, nil
	case "-printf", "-fprintf":
		return p.parsePrintf(tok)
	case "-perm":
		return p.parsePerm()
	case "-type":
		return p.parseType(false)
	case "-xtype":
		return p.parseType(true)
	case "-links":
		return p.parseNumPrimary(tok, func(n num) expr { return linksExpr{n: n} })
	case "-inum":
		return p.parseNumPrimary(tok, func(n num) expr { return inumExpr{n: n} })
	case "-uid":
		return p.parseNumPrimary(tok, func(n num) expr { return uidExpr{n: n} })
	case "-gid":
		return p.parseNumPrimary(tok, func(n num) expr { return gidExpr{n: n} })
	case "-user":
		return p.parseUser()
	case "-group":
		return p.parseGroup()
	case "-size":
		return p.parseSize()
	case "-atime", "-ctime", "-mtime":
		return p.parseTime(tok)
	case "-amin", "-cmin", "-mmin":
		return p.parseTime(tok)
	case "-newer":
		return p.parseNewerXY('m', 'm')
	case "-anewer":
		return p.parseNewerXY('a', 'm')
	case "-cnewer":
		return p.parseNewerXY('c', 'm')
	case "-readable":
		return accessExpr{bit: 0o400}, nil
	case "-writable":
		return accessExpr{bit: 0o200}, nil
	case "-executable":
		return accessExpr{bit: 0o100}, nil
	case "-samefile":
		return p.parseSamefile()
	case "-used":
		return p.parseNumPrimary(tok, func(n num) expr { return usedExpr{n: n} })
	case "-exec":
		return p.parseExec(false, false)
	case "-execdir":
		return p.parseExec(false, true)
	case "-ok":
		return p.parseExec(true, false)
	case "-okdir":
		return p.parseExec(true, true)
	case "-fstype", "-context":
		return nil, fmt.Errorf("-%s is not supported", tok[1:])
	default:
		if strings.HasPrefix(tok, "-newer") && len(tok) == 8 {
			return p.parseNewerXY(tok[6], tok[7])
		}
		return nil, fmt.Errorf("unknown predicate '%s'", tok)
	}
}

// arg returns the argument of a primary.
func (p *parser) arg(name string) (string, error) {
	if p.pos >= len(p.tokens) {
		return "", fmt.Errorf("missing argument to '%s'", name)
	}
	return p.next(), nil
}

// compileRegex compiles the pattern of a -regex primary, which matches the
// whole pathname.
func (p *parser) compileRegex(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	var translated string
	var err error
	if p.bre {
		translated, err = bre.Translate(pattern)
	} else {
		translated, err = ere.Translate(pattern)
	}
	if err != nil {
		return nil, err
	}
	if ignoreCase {
		translated = "(?i)" + translated
	}
	re, err := regexp.Compile(`\A(?:` + translated + `)\z`)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression '%s'", pattern)
	}
	return re, nil
}

// parseRegexType parses the -regextype option, which selects the syntax
// the regular expressions after it are written in.
func (p *parser) parseRegexType() (expr, error) {
	name, err := p.arg("-regextype")
	if err != nil {
		return nil, err
	}
	switch name {
	case "posix-basic", "grep", "emacs":
		p.bre = true
	case "findutils-default", "posix-extended", "posix-egrep", "egrep",
		"posix-awk", "awk", "gnu-awk":
		p.bre = false
	default:
		return nil, fmt.Errorf("unknown regular expression type '%s'", name)
	}
	return trueExpr{}, nil
}

// parseDepth parses the -maxdepth and -mindepth options.
func (p *parser) parseDepth(name string) (expr, error) {
	arg, err := p.arg(name)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid argument '%s' to %s", arg, name)
	}
	if name == "-maxdepth" {
		return maxDepthExpr{n}, nil
	}
	return minDepthExpr{n}, nil
}

// parsePerm parses the -perm primary, whose mode may be prefixed by a '-'
// or a '/'.
func (p *parser) parsePerm() (expr, error) {
	mode, err := p.arg("-perm")
	if err != nil {
		return nil, err
	}
	var all, any bool
	switch {
	case strings.HasPrefix(mode, "-"):
		all, mode = true, mode[1:]
	case strings.HasPrefix(mode, "/"):
		any, mode = true, mode[1:]
	case strings.HasPrefix(mode, "+"):
		if len(mode) > 1 && mode[1] >= '0' && mode[1] <= '7' {
			return nil, fmt.Errorf("invalid mode '%s': mode must not start with '+' followed by an octal digit", mode)
		}
		// The deprecated form of '/', for symbolic modes.
		any, mode = true, mode[1:]
	}
	spec, err := filemode.Parse(mode)
	if err != nil {
		return nil, fmt.Errorf("invalid mode '%s'", mode)
	}
	return permExpr{spec: spec, all: all, any: any}, nil
}

// parseType parses the -type and -xtype primaries, which take one type
// letter or a list of them separated by commas.
func (p *parser) parseType(x bool) (expr, error) {
	name := "-type"
	if x {
		name = "-xtype"
	}
	arg, err := p.arg(name)
	if err != nil {
		return nil, err
	}
	var typs []fs.FileMode
	for c := range strings.SplitSeq(arg, ",") {
		typ, err := typeOf(c)
		if err != nil {
			return nil, err
		}
		typs = append(typs, typ)
	}
	if x {
		return xtypeExpr{typs: typs}, nil
	}
	return typeExpr{typs: typs}, nil
}

// typeOf returns the file mode type a type letter stands for.
func typeOf(c string) (fs.FileMode, error) {
	switch c {
	case "b":
		return fs.ModeDevice, nil
	case "c":
		return fs.ModeDevice | fs.ModeCharDevice, nil
	case "d":
		return fs.ModeDir, nil
	case "l":
		return fs.ModeSymlink, nil
	case "p":
		return fs.ModeNamedPipe, nil
	case "f":
		return 0, nil
	case "s":
		return fs.ModeSocket, nil
	}
	return 0, fmt.Errorf("unknown type '%s'", c)
}

// parseUser parses the -user primary, which takes a name or an identifier.
func (p *parser) parseUser() (expr, error) {
	arg, err := p.arg("-user")
	if err != nil {
		return nil, err
	}
	uid, err := lookupUser(arg)
	if err != nil {
		return nil, err
	}
	return userExpr{uid: uid}, nil
}

// parseGroup parses the -group primary, which takes a name or an
// identifier.
func (p *parser) parseGroup() (expr, error) {
	arg, err := p.arg("-group")
	if err != nil {
		return nil, err
	}
	gid, err := lookupGroup(arg)
	if err != nil {
		return nil, err
	}
	return groupExpr{gid: gid}, nil
}

// parseNumPrimary parses a primary whose argument is a number, as a
// callback builds the expression of.
func (p *parser) parseNumPrimary(name string, build func(num) expr) (expr, error) {
	arg, err := p.arg(name)
	if err != nil {
		return nil, err
	}
	n, err := parseNum(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid argument '%s' to %s", arg, name)
	}
	return build(n), nil
}

// parseSize parses the -size primary, which counts 512-byte blocks by
// default and takes a unit suffix.
func (p *parser) parseSize() (expr, error) {
	arg, err := p.arg("-size")
	if err != nil {
		return nil, err
	}
	if arg == "" {
		return nil, fmt.Errorf("invalid argument '' to -size")
	}
	unit := int64(512)
	switch arg[len(arg)-1] {
	case 'b':
		arg, unit = arg[:len(arg)-1], 512
	case 'c':
		arg, unit = arg[:len(arg)-1], 1
	case 'w':
		arg, unit = arg[:len(arg)-1], 2
	case 'k':
		arg, unit = arg[:len(arg)-1], 1024
	case 'M':
		arg, unit = arg[:len(arg)-1], 1024*1024
	case 'G':
		arg, unit = arg[:len(arg)-1], 1024*1024*1024
	}
	n, err := parseNum(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid argument '%s' to -size", arg)
	}
	return sizeExpr{n: n, unit: unit}, nil
}

// parseTime parses the -atime, -ctime, -mtime, -amin, -cmin and -mmin
// primaries.
func (p *parser) parseTime(name string) (expr, error) {
	arg, err := p.arg(name)
	if err != nil {
		return nil, err
	}
	n, err := parseNum(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid argument '%s' to %s", arg, name)
	}
	seconds := int64(86400)
	if strings.HasSuffix(name, "min") {
		seconds = 60
	}
	return timeExpr{n: n, which: name[1], seconds: seconds}, nil
}

// parseNewerXY parses the -newerXY primary, where X selects the time of the
// file and Y the one of the reference, a file or, with t, a date string.
func (p *parser) parseNewerXY(x, y byte) (expr, error) {
	if x != 'a' && x != 'B' && x != 'c' && x != 'm' ||
		y != 'a' && y != 'B' && y != 'c' && y != 'm' && y != 't' {
		return nil, fmt.Errorf("invalid argument -newer%c%c", x, y)
	}
	arg, err := p.arg(fmt.Sprintf("-newer%c%c", x, y))
	if err != nil {
		return nil, err
	}
	if y == 'B' {
		return nil, fmt.Errorf("cannot use the birth time of '%s': it is not available", arg)
	}
	e := newerExpr{which: lower(x)}
	if x == 'B' {
		e.never = true
		return e, nil
	}
	if y == 't' {
		t, err := gnudate.Parse(arg, p.now, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid date '%s'", arg)
		}
		e.ref = t
		return e, nil
	}
	fi, err := p.statRef(arg)
	if err != nil {
		return nil, err
	}
	e.ref = fileTime(fi, lower(y))
	return e, nil
}

// parseSamefile parses the -samefile primary, which reads the identity of
// its file once.
func (p *parser) parseSamefile() (expr, error) {
	arg, err := p.arg("-samefile")
	if err != nil {
		return nil, err
	}
	fi, err := p.statRef(arg)
	if err != nil {
		return nil, err
	}
	st, ok := fileInfoStat(fi)
	if !ok {
		return nil, fmt.Errorf("cannot use '%s' as a reference: it reports no inode", arg)
	}
	return samefileExpr{dev: st.Dev, ino: st.Ino}, nil
}

// statRef describes the reference file of the primaries which take one,
// following symbolic links as the options ask for.
func (p *parser) statRef(arg string) (fs.FileInfo, error) {
	name := p.rt.FsPath(arg)
	fi, err := walk.Stat(p.rt.FS, p.follow, name, true)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		// A link whose target doesn't exist is described as itself; the
		// reference times are then the ones of the link.
		if lfi, lerr := fs.Lstat(p.rt.FS, name); lerr == nil {
			fi, err = lfi, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot stat '%s': %v", arg, statReason(err))
	}
	return fi, nil
}

// parseExec parses the -exec, -execdir, -ok and -okdir primaries, which
// hold a utility and its arguments, ended by a ';' or, for the exec ones,
// by a '{}' '+'.
func (p *parser) parseExec(ok, dir bool) (expr, error) {
	name := "-exec"
	switch {
	case ok && dir:
		name = "-okdir"
	case ok:
		name = "-ok"
	case dir:
		name = "-execdir"
	}
	e := &execExpr{ok: ok, dir: dir}
	for {
		if p.pos >= len(p.tokens) {
			return nil, fmt.Errorf("missing argument to '%s'", name)
		}
		tok := p.tokens[p.pos]
		if tok == ";" {
			p.pos++
			break
		}
		if tok == "+" && len(e.args) > 0 && e.args[len(e.args)-1] == "{}" {
			if ok {
				return nil, fmt.Errorf("'%s' doesn't support the '+' terminator", name)
			}
			p.pos++
			e.plus = true
			break
		}
		e.args = append(e.args, tok)
		p.pos++
	}
	if len(e.args) == 0 {
		return nil, fmt.Errorf("missing argument to '%s'", name)
	}
	return e, nil
}

// scanExpr collects the primaries that apply to the whole traversal, along
// with the aggregating -exec primaries and the files the -fprint family
// writes to, and reports whether the expression holds an action.
func scanExpr(e expr, opts *options) (action bool, execs []*execExpr, outFiles []string) {
	var visit func(expr)
	visit = func(e expr) {
		switch t := e.(type) {
		case orExpr:
			visit(t.left)
			visit(t.right)
		case andExpr:
			visit(t.left)
			visit(t.right)
		case listExpr:
			visit(t.left)
			visit(t.right)
		case notExpr:
			visit(t.inner)
		case depthExpr, deleteExpr:
			// -delete acts after the entries of a directory, and so does
			// the traversal.
			opts.depth = true
			if _, ok := e.(deleteExpr); ok {
				action = true
			}
		case maxDepthExpr:
			opts.maxDepth = t.n
		case minDepthExpr:
			opts.minDepth = t.n
		case daystartExpr:
			opts.daystart = true
		case followExpr:
			opts.follow = 'L'
		case ignoreRaceExpr:
			opts.ignoreRace = true
		case noIgnoreRaceExpr:
			opts.ignoreRace = false
		case mountExpr:
			opts.mount = true
		case xdevExpr:
			opts.xdev = true
		case printExpr:
			action = true
			if t.file != "" {
				outFiles = append(outFiles, t.file)
			}
		case printfExpr:
			action = true
			if t.file != "" {
				outFiles = append(outFiles, t.file)
			}
		case lsExpr:
			action = true
			if t.file != "" {
				outFiles = append(outFiles, t.file)
			}
		case *execExpr:
			action = true
			if t.plus {
				execs = append(execs, t)
			}
		}
	}
	visit(e)
	return action, execs, outFiles
}

// num is the numeric argument of a primary: exactly n, more than n with a
// '+', or less than n with a '-'.
type num struct {
	plus  bool
	minus bool
	n     int64
}

// match reports whether v satisfies the argument.
func (m num) match(v int64) bool {
	switch {
	case m.plus:
		return v > m.n
	case m.minus:
		return v < m.n
	default:
		return v == m.n
	}
}

// parseNum parses the numeric argument of a primary.
func parseNum(s string) (num, error) {
	var n num
	if strings.HasPrefix(s, "+") {
		n.plus = true
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		n.minus = true
		s = s[1:]
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return num{}, err
	}
	n.n = v
	return n, nil
}

// lookupUser returns the identifier a -user primary matches, resolving a
// name or taking a decimal integer that names no user as the identifier.
func lookupUser(arg string) (int, error) {
	if u, err := user.Lookup(arg); err == nil {
		return strconv.Atoi(u.Uid)
	}
	if v, err := strconv.Atoi(arg); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("'%s' is not the name of an existing user", arg)
}

// lookupGroup returns the identifier a -group primary matches, resolving a
// name or taking a decimal integer that names no group as the identifier.
func lookupGroup(arg string) (int, error) {
	if g, err := user.LookupGroup(arg); err == nil {
		return strconv.Atoi(g.Gid)
	}
	if v, err := strconv.Atoi(arg); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("'%s' is not the name of an existing group", arg)
}

// statReason returns the reason of a stat failure, without the operation
// and the path it names.
func statReason(err error) error {
	if perr, ok := errors.AsType[*fs.PathError](err); ok {
		return perr.Err
	}
	return err
}

// lower returns the lowercase form of a time selector letter.
func lower(c byte) byte {
	if c == 'B' {
		return 'b'
	}
	return c
}
