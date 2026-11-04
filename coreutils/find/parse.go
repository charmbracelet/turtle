package find

import (
	"errors"
	"fmt"
	"io/fs"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/bracket"
	"charm.land/snail/coreutils/internal/filemode"
)

// parseExpr parses the tokens of the expression of find.
func parseExpr(rt *coreutils.Runtime, tokens []string) (expr, error) {
	p := &parser{rt: rt, tokens: tokens}
	e, err := p.parseOr()
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
	tokens []string
	pos    int
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

// parseOr parses expressions separated by the -o operator.
func (p *parser) parseOr() (expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "-o" {
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
		case "", ")", "-o":
			return left, nil
		case "-a":
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
	if p.peek() == "!" {
		p.next()
		if p.peek() == "" {
			return nil, errors.New("missing expression after '!'")
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
	case "(":
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, errors.New("missing ')'")
		}
		return inner, nil
	case "-a", "-o":
		return nil, fmt.Errorf("invalid expression; '%s' with nothing before it", tok)
	case "-name", "-iname":
		pattern, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		re, err := compilePattern(pattern, tok == "-iname")
		if err != nil {
			return nil, err
		}
		return nameExpr{re}, nil
	case "-path":
		pattern, err := p.arg(tok)
		if err != nil {
			return nil, err
		}
		re, err := compilePattern(pattern, false)
		if err != nil {
			return nil, err
		}
		return pathMatch{re}, nil
	case "-nouser":
		return nouserExpr{}, nil
	case "-nogroup":
		return nogroupExpr{}, nil
	case "-mount":
		return mountExpr{}, nil
	case "-xdev":
		return xdevExpr{}, nil
	case "-prune":
		return pruneExpr{}, nil
	case "-depth":
		return depthExpr{}, nil
	case "-print":
		return printExpr{}, nil
	case "-print0":
		return printExpr{zero: true}, nil
	case "-perm":
		return p.parsePerm()
	case "-type":
		return p.parseType()
	case "-links":
		return p.parseLinks()
	case "-user":
		return p.parseUser()
	case "-group":
		return p.parseGroup()
	case "-size":
		return p.parseSize()
	case "-atime", "-ctime", "-mtime":
		return p.parseTime(tok)
	case "-newer":
		return p.parseNewer()
	case "-exec":
		return p.parseExec(false)
	case "-ok":
		return p.parseExec(true)
	default:
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

// parsePerm parses the -perm primary, whose mode may be prefixed by a '-'.
func (p *parser) parsePerm() (expr, error) {
	mode, err := p.arg("-perm")
	if err != nil {
		return nil, err
	}
	all := false
	if strings.HasPrefix(mode, "-") {
		all = true
		mode = mode[1:]
	}
	spec, err := filemode.Parse(mode)
	if err != nil {
		return nil, fmt.Errorf("invalid mode '%s'", mode)
	}
	return permExpr{spec: spec, all: all}, nil
}

// parseType parses the -type primary.
func (p *parser) parseType() (expr, error) {
	arg, err := p.arg("-type")
	if err != nil {
		return nil, err
	}
	var typ fs.FileMode
	switch arg {
	case "b":
		typ = fs.ModeDevice
	case "c":
		typ = fs.ModeDevice | fs.ModeCharDevice
	case "d":
		typ = fs.ModeDir
	case "l":
		typ = fs.ModeSymlink
	case "p":
		typ = fs.ModeNamedPipe
	case "f":
		typ = 0
	case "s":
		typ = fs.ModeSocket
	default:
		return nil, fmt.Errorf("unknown type '%s'", arg)
	}
	return typeExpr{typ: typ}, nil
}

// parseLinks parses the -links primary.
func (p *parser) parseLinks() (expr, error) {
	arg, err := p.arg("-links")
	if err != nil {
		return nil, err
	}
	n, err := parseNum(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid argument '%s' to -links", arg)
	}
	return linksExpr{n: n}, nil
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

// parseGroup parses the -group primary, which takes a name or an identifier.
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

// parseSize parses the -size primary, which counts 512-byte blocks or, with
// a c, bytes.
func (p *parser) parseSize() (expr, error) {
	arg, err := p.arg("-size")
	if err != nil {
		return nil, err
	}
	bytes := false
	if strings.HasSuffix(arg, "c") {
		bytes = true
		arg = arg[:len(arg)-1]
	}
	n, err := parseNum(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid argument '%s' to -size", arg)
	}
	return sizeExpr{n: n, bytes: bytes}, nil
}

// parseTime parses the -atime, -ctime and -mtime primaries.
func (p *parser) parseTime(name string) (expr, error) {
	arg, err := p.arg(name)
	if err != nil {
		return nil, err
	}
	n, err := parseNum(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid argument '%s' to %s", arg, name)
	}
	return timeExpr{n: n, which: name[1]}, nil
}

// parseNewer parses the -newer primary, reading the modification time of
// its file once.
func (p *parser) parseNewer() (expr, error) {
	arg, err := p.arg("-newer")
	if err != nil {
		return nil, err
	}
	name := path.Clean(filepath.ToSlash(p.rt.ResolvePath(arg)))
	fi, err := fs.Stat(p.rt.FS, name)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		// A link whose target doesn't exist is described as itself; the
		// reference time of -newer is then the one of the link.
		if lfi, lerr := fs.Lstat(p.rt.FS, name); lerr == nil {
			fi, err = lfi, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot stat '%s': %v", arg, statReason(err))
	}
	return newerExpr{mtime: fi.ModTime()}, nil
}

// parseExec parses the -exec and -ok primaries, which hold a utility and
// its arguments, ended by a ';' or, for -exec, by a '{}' '+'.
func (p *parser) parseExec(ok bool) (expr, error) {
	name := "-exec"
	if ok {
		name = "-ok"
	}
	e := &execExpr{ok: ok}
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
// with the aggregating -exec primaries, and reports whether the expression
// holds an action: a -exec, -ok, -print or -print0 primary.
func scanExpr(e expr, opts *options) (action bool, execs []*execExpr) {
	var visit func(expr)
	visit = func(e expr) {
		switch t := e.(type) {
		case orExpr:
			visit(t.left)
			visit(t.right)
		case andExpr:
			visit(t.left)
			visit(t.right)
		case notExpr:
			visit(t.inner)
		case depthExpr:
			opts.depth = true
		case mountExpr:
			opts.mount = true
		case xdevExpr:
			opts.xdev = true
		case printExpr:
			action = true
		case *execExpr:
			action = true
			if t.plus {
				execs = append(execs, t)
			}
		}
	}
	visit(e)
	return action, execs
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

// compilePattern compiles a pattern of the pattern matching notation, which
// the -name, -iname and -path primaries use. Unlike a filename expansion
// pattern, a '*' or '?' matches a slash, and a leading dot is not special.
// The pattern matches the whole string.
func compilePattern(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	var out strings.Builder
	if ignoreCase {
		out.WriteString("(?i)")
	}
	out.WriteString(`\A(?:`)
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; c {
		case '*':
			out.WriteString(".*")
			i++
		case '?':
			out.WriteByte('.')
			i++
		case '[':
			end := bracket.End(pattern, i)
			if end < 0 {
				out.WriteString(`\[`)
				i++
				continue
			}
			t, err := bracket.Translate(pattern[i : end+1])
			if err != nil {
				return nil, fmt.Errorf("invalid pattern '%s'", pattern)
			}
			out.WriteString(t)
			i = end + 1
		case '\\':
			if i+1 < len(pattern) {
				out.WriteString(regexp.QuoteMeta(pattern[i+1 : i+2]))
				i += 2
			} else {
				out.WriteString(`\\`)
				i++
			}
		default:
			out.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
		}
	}
	out.WriteString(`)\z`)
	re, err := regexp.Compile(out.String())
	if err != nil {
		return nil, fmt.Errorf("invalid pattern '%s'", pattern)
	}
	return re, nil
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
