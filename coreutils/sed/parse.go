package sed

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils/internal/bre"
)

// address is the address of an editing command: the last line of the input,
// a line number, or a regular expression matching the pattern space. GNU
// extensions add the first~step form, the +N and ~N second addresses, and
// the 0 first address, whose range ends on its own first line when the
// second address matches it.
type address struct {
	last bool
	line int
	step int
	re   *regexp.Regexp
	// rel is the line count of a +N second address, mod the divisor of a
	// ~N one, and both are zero for the other kinds of address.
	rel int
	mod int
	// zero tells the 0 first address of a 0,/re/ range apart from a line
	// number, which would never match.
	zero bool
}

// command is a single editing command of the script.
type command struct {
	a1, a2 *address
	neg    bool
	verb   byte

	// inRange holds the state of a two-address range, rangeStart and
	// rangeEnd tell whether the current cycle opens or closes it, and
	// endLine holds the closing line of a +N or ~N second address.
	inRange    bool
	rangeStart bool
	rangeEnd   bool
	endLine    int

	label string
	jump  int // resolved target of a b, t or T command

	text  string // a, i and c text
	file  string // r, R, w and W file
	sub   *subst
	width int // the wrap width of an l command, -1 when it takes none

	translit map[rune]rune
	// exit is the status a q or Q command asks for.
	exit int

	// end is the index of the command following the matching `}` of a `{`
	// command, used to skip the block when it isn't selected.
	end int
}

// subst is the substitution of an s command.
type subst struct {
	re     *regexp.Regexp
	tmpl   []replItem
	global bool
	print  bool
	nth    int
	file   string
}

// replItem is a part of the replacement of an s command: a literal string,
// the whole match (group -1), a group \n of the pattern, or one of the case
// conversion escapes of GNU.
type replItem struct {
	literal string
	group   int
	caseOp  byte // 0, or one of u l U L E
}

// literalItem builds the replItem of a literal string.
func literalItem(s string) replItem {
	return replItem{literal: s, group: -2}
}

// parser reads the script, producing its list of commands.
type parser struct {
	s        string
	i        int
	extended bool
	posix    bool
	nullData bool
	lastRE   *regexp.Regexp
}

func parseScript(script string, o *options) ([]*command, error) {
	p := &parser{s: script, extended: o.extended, posix: o.posix, nullData: o.nullData}
	cmds, err := p.commands()
	if err != nil {
		return nil, err
	}
	if err := resolve(cmds); err != nil {
		return nil, err
	}
	return cmds, nil
}

// gnuErr reports a GNU extension the script isn't allowed to use with
// --posix.
func gnuErr(what string) error {
	return fmt.Errorf("%s is a GNU extension, not allowed with --posix", what)
}

// commands parses commands until the end of the script, tracking the brace
// nesting to give each `{` command the index of its matching `}`.
func (p *parser) commands() ([]*command, error) {
	var cmds []*command
	var stack []*command
	// fresh tells whether the parser sits at the start of a command, that is,
	// right after a <newline> or a <semicolon> (or at the very beginning).
	// POSIX requires a `}` to be preceded by one of them.
	fresh := true
	for {
		p.blanks()
		if p.i >= len(p.s) {
			if len(stack) > 0 {
				return nil, errors.New("unmatched {")
			}
			return cmds, nil
		}
		switch p.s[p.i] {
		case '\n', ';':
			p.i++ // empty command
			fresh = true
			continue
		case '}':
			if len(stack) == 0 || !fresh {
				return nil, errors.New("unexpected }")
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			open.end = len(cmds)
			p.i++
			fresh = false
			continue
		}
		cmd, err := p.command()
		if err != nil {
			return nil, err
		}
		fresh = false
		if cmd.verb == '{' {
			cmd.end = len(cmds) // grows past the block when it's closed
			stack = append(stack, cmd)
		}
		cmds = append(cmds, cmd)
	}
}

// command parses a single editing command, starting at its first address.
func (p *parser) command() (*command, error) {
	cmd := &command{verb: 0, width: -1}
	var err error
	if cmd.a1, err = p.address(); err != nil {
		return nil, err
	}
	p.blanks()
	if p.peek() == ',' {
		p.i++
		p.blanks()
		if cmd.a2, err = p.rangeEnd(); err != nil {
			return nil, err
		}
		if cmd.a2 == nil {
			return nil, errors.New("missing address after ,")
		}
		if cmd.a2.step > 0 {
			return nil, errors.New("a step address can't close a range")
		}
	}
	if cmd.a1 != nil && cmd.a1.zero && cmd.a2 == nil {
		return nil, errors.New("0 address can only be the first of a range")
	}
	p.blanks()
	for p.peek() == '!' {
		cmd.neg = !cmd.neg
		p.i++
	}

	c := p.peek()
	if c == 0 {
		return nil, errors.New("missing command")
	}
	p.i++
	cmd.verb = c

	addresses := 0
	if cmd.a1 != nil {
		addresses++
	}
	if cmd.a2 != nil {
		addresses++
	}
	if limit := p.maxAddresses(c); addresses > limit {
		return nil, fmt.Errorf("too many addresses for command: %c", c)
	}

	switch c {
	case 'T', 'Q', 'z', 'F', 'W', 'R':
		if p.posix {
			return nil, gnuErr(fmt.Sprintf("the %c command", c))
		}
	}
	switch c {
	case ':', 'b', 't', 'T':
		cmd.label = p.labelArg()
	case 'l':
		if p.peek() >= '0' && p.peek() <= '9' {
			if p.posix {
				return nil, gnuErr("the width of the l command")
			}
			if cmd.width, err = p.number(); err != nil {
				return nil, err
			}
		}
	case 'q', 'Q':
		p.blanks()
		if p.peek() >= '0' && p.peek() <= '9' {
			if p.posix {
				return nil, gnuErr("the exit status of the " + string(c) + " command")
			}
			if cmd.exit, err = p.number(); err != nil {
				return nil, err
			}
			if cmd.exit > 255 {
				return nil, errors.New("exit status out of range")
			}
		}
	case '{', '=', 'd', 'D', 'g', 'G', 'h', 'H', 'n', 'N', 'p', 'P', 'x', 'z', 'F':
		// No argument.
	case 'a', 'i', 'c':
		if cmd.text, err = p.textArg(); err != nil {
			return nil, err
		}
	case 'r', 'R', 'w', 'W':
		cmd.file = p.fileArg()
		if cmd.file == "" {
			return nil, fmt.Errorf("filename expected for %c command", c)
		}
	case 's':
		if cmd.sub, err = p.substArg(); err != nil {
			return nil, err
		}
	case 'y':
		if err := p.translitArg(cmd); err != nil {
			return nil, err
		}
	case '#':
		p.skipLine()
	default:
		return nil, fmt.Errorf("unknown command: %c", c)
	}
	return cmd, nil
}

// maxAddresses returns the maximum number of addresses an editing command
// accepts: only # : } take none, q and Q take at most one, and every other
// command takes up to two. The GNU extensions letting a, i, r and = take a
// range are rejected with --posix.
func (p *parser) maxAddresses(verb byte) int {
	switch verb {
	case '#', ':', '}':
		return 0
	case 'q', 'Q':
		return 1
	}
	if p.posix {
		switch verb {
		case '=', 'a', 'i', 'r':
			return 1
		}
	}
	return 2
}

// address parses an address, returning nil when the next character doesn't
// start one.
func (p *parser) address() (*address, error) {
	switch c := p.peek(); {
	case c == '$':
		p.i++
		return &address{last: true}, nil
	case c >= '0' && c <= '9':
		n, err := p.number()
		if err != nil {
			return nil, err
		}
		if p.peek() == '~' {
			if p.posix {
				return nil, gnuErr("the first~step address")
			}
			p.i++
			step, err := p.number()
			if err != nil {
				return nil, errors.New("missing step after ~")
			}
			if step <= 0 {
				return nil, errors.New("step can't be 0")
			}
			return &address{line: n, step: step}, nil
		}
		if n == 0 {
			if p.posix {
				return nil, gnuErr("the 0 address")
			}
			return &address{zero: true}, nil
		}
		return &address{line: n}, nil
	case c == '/':
		return p.regexAddress('/')
	case c == '\\':
		p.i++
		d := p.peek()
		if d == 0 || d == '\\' || d == '\n' {
			return nil, errors.New("invalid regular expression delimiter")
		}
		return p.regexAddress(d)
	}
	return nil, nil
}

// rangeEnd parses the second address of a range, which the GNU extensions
// also let be a count of lines from the first one (+N) or the next multiple
// of a divisor (~N).
func (p *parser) rangeEnd() (*address, error) {
	switch p.peek() {
	case '+':
		if p.posix {
			return nil, gnuErr("the addr1,+N address")
		}
		p.i++
		n, err := p.number()
		if err != nil {
			return nil, errors.New("missing number after +")
		}
		return &address{line: n, rel: n}, nil
	case '~':
		if p.posix {
			return nil, gnuErr("the addr1,~N address")
		}
		p.i++
		n, err := p.number()
		if err != nil {
			return nil, errors.New("missing number after ~")
		}
		if n <= 0 {
			return nil, errors.New("the number after ~ must be positive")
		}
		return &address{line: n, mod: n}, nil
	}
	return p.address()
}

// regexAddress parses a context address, whose regular expression is
// delimited by the given character. The opening delimiter was consumed. The
// I and M modifiers of GNU, uppercase only since a command letter may
// follow, select case-insensitive and multi-line matching.
func (p *parser) regexAddress(delim byte) (*address, error) {
	p.i++
	re, err := p.scanDelimited(delim, scanRE)
	if err != nil {
		return nil, errors.New("unterminated regular expression")
	}
	fold, multi := false, false
	for {
		switch p.peek() {
		case 'I':
			if p.posix {
				return nil, gnuErr("the I modifier of an address")
			}
			fold = true
			p.i++
		case 'M':
			if p.posix {
				return nil, gnuErr("the M modifier of an address")
			}
			multi = true
			p.i++
		default:
			compiled, err := p.compileRE(re, fold, multi)
			if err != nil {
				return nil, err
			}
			return &address{re: compiled}, nil
		}
	}
}

// compileRE compiles a regular expression, extended or not, matching case
// insensitively or not and across the embedded newlines of the pattern space
// or not, and keeps it as the last one for the empty REs of later commands.
func (p *parser) compileRE(re string, fold, multi bool) (*regexp.Regexp, error) {
	if re == "" {
		if p.lastRE == nil {
			return nil, errors.New("no previous regular expression")
		}
		return p.lastRE, nil
	}
	// The period doesn't match the newlines of the pattern space, as POSIX
	// requires; a pattern matches embedded newlines only explicitly, through
	// an escape like \n or the joined lines of an explicit newline. The M
	// modifier makes ^ and $ match them, and with -z the newline is an
	// ordinary character of the NUL-separated record.
	flags := ""
	if fold {
		flags += "(?i)"
	}
	if multi {
		flags += "(?m)"
	}
	if p.nullData {
		flags += "(?s)"
	}
	var compiled *regexp.Regexp
	var err error
	switch {
	case p.extended:
		compiled, err = regexp.Compile(flags + re)
	default:
		// The BRE engine has no flags, but the pattern it produces is a Go
		// expression which can be compiled again with them.
		var plain *regexp.Regexp
		if plain, err = bre.Compile(re); err == nil {
			compiled, err = regexp.Compile(flags + plain.String())
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %v", err)
	}
	p.lastRE = compiled
	return compiled, nil
}

// scanKind tells scanDelimited what it is scanning, since the backslash of
// an escaped delimiter is dropped or kept depending on it.
type scanKind int

const (
	scanRE   scanKind = iota // the regular expression of an address or s command
	scanRepl                 // the replacement of an s command
)

// scanDelimited scans the text up to the next unescaped delimiter, which is
// consumed. An escaped delimiter stands for the delimiter itself, so its
// backslash is dropped in a regular expression when the delimiter is not a
// special character of the RE, as POSIX requires; when it is one, the
// backslash is kept, since the meaning of the escape is then unspecified. In
// the replacement of an s command the backslash of an alphanumeric delimiter
// is dropped, and other escaped delimiters keep it for compileRepl to
// discard. All other escapes are kept in the text as scanned. The delimiter
// inside a bracket expression of a regular expression doesn't terminate it.
// A backslash followed by a newline is allowed only in the replacement of an
// s command.
func (p *parser) scanDelimited(delim byte, kind scanKind) (string, error) {
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == delim:
			p.i++
			return b.String(), nil
		case c == '[' && kind == scanRE:
			end := bracketEnd(p.s, p.i)
			if end < 0 {
				return "", errors.New("unmatched [")
			}
			b.WriteString(p.s[p.i : end+1])
			p.i = end + 1
		case c == '\n':
			return "", errors.New("newline in text")
		case c == '\\':
			if p.i+1 >= len(p.s) {
				return "", errors.New("trailing backslash")
			}
			d := p.s[p.i+1]
			if d == delim {
				switch {
				case kind == scanRepl && isAlnum(delim),
					kind == scanRE && !p.isDelimSpecial(delim):
					b.WriteByte(delim)
				default:
					b.WriteByte('\\')
					b.WriteByte(delim)
				}
			} else {
				if d == '\n' && kind == scanRE {
					return "", errors.New("newline in regular expression")
				}
				b.WriteByte('\\')
				b.WriteByte(d)
			}
			p.i += 2
		default:
			b.WriteByte(c)
			p.i++
		}
	}
	return "", errors.New("unterminated text")
}

// labelArg reads the label of a :, b, t or T command. The label ends at the
// first <semicolon>, as in GNU sed, or at the end of the line, and an
// escaped <semicolon> belongs to the name. The leading blanks are skipped
// and the trailing ones dropped.
func (p *parser) labelArg() string {
	p.blanks()
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\n' || c == ';' {
			break
		}
		if c == '\\' && p.i+1 < len(p.s) && p.s[p.i+1] == ';' {
			b.WriteByte(';')
			p.i += 2
			continue
		}
		b.WriteByte(c)
		p.i++
	}
	return strings.TrimRight(b.String(), " \t")
}

// fileArg reads the file argument of an r, R, w or W command, which extends
// to the end of the line and terminates the command.
func (p *parser) fileArg() string {
	p.blanks()
	start := p.i
	for c := p.peek(); c != 0 && c != '\n'; c = p.peek() {
		p.i++
	}
	return strings.TrimRight(p.s[start:p.i], " \t")
}

// textArg reads the text of an a, i or c command. The text starts right
// after the backslash of the command, or on the next line, and continues on
// the following lines while they end with an escaped newline. The GNU
// one-line forms also let the text follow the command without a backslash,
// on the same line or on the one after it.
func (p *parser) textArg() (string, error) {
	p.blanks()
	switch c := p.peek(); {
	case c == '\\':
		p.i++
		if p.peek() == '\n' {
			p.i++ // the text starts on the next line
		}
	case c == '\n', c == 0:
		if p.posix {
			return "", errors.New(`text must start with \`)
		}
		if c == '\n' {
			p.i++ // the text starts on the next line
		}
	default:
		if p.posix {
			return "", errors.New(`text must start with \`)
		}
		// the text starts on the same line
	}

	var b strings.Builder
	for {
		if b.Len() == 0 {
			p.blanks() // the first text line loses its leading blanks
		}
		continued := false
		for p.i < len(p.s) {
			c := p.s[p.i]
			if c == '\n' {
				p.i++
				break
			}
			if c == '\\' {
				if p.i+1 < len(p.s) {
					switch d := p.s[p.i+1]; d {
					case '\n':
						p.i += 2
						continued = true
					case '\\':
						p.i += 2
						b.WriteByte('\\')
					default:
						p.i++
					}
					if continued {
						break
					}
					continue
				}
				p.i++ // a trailing backslash at the end of the script is literal
			}
			b.WriteByte(c)
			p.i++
		}
		if !continued {
			return b.String(), nil
		}
		b.WriteByte('\n')
	}
}

// substArg parses the regular expression, replacement and flags of an s
// command, whose delimiter was already peeked.
func (p *parser) substArg() (*subst, error) {
	if p.i >= len(p.s) {
		return nil, errors.New("missing delimiter for s command")
	}
	delim := p.s[p.i]
	if delim == '\\' || delim == '\n' {
		return nil, errors.New("invalid delimiter for s command")
	}
	p.i++
	reText, err := p.scanDelimited(delim, scanRE)
	if err != nil {
		return nil, fmt.Errorf("unterminated regular expression in s command: %v", err)
	}
	replText, err := p.scanDelimited(delim, scanRepl)
	if err != nil {
		return nil, fmt.Errorf("unterminated replacement in s command: %v", err)
	}

	sub := &subst{}
	fold, multi := false, false
flags:
	for p.i < len(p.s) {
		switch c := p.s[p.i]; {
		case c == '\n' || c == ';' || c == '}' || c == ' ' || c == '\t':
			break flags
		case c == 'g':
			sub.global = true
			p.i++
		case c == 'p':
			sub.print = true
			p.i++
		case c == 'i', c == 'I':
			// The lowercase i is in the POSIX specification; only the
			// uppercase I is GNU's alias for it.
			if c == 'I' && p.posix {
				return nil, gnuErr("the I flag of the s command")
			}
			fold = true
			p.i++
		case c == 'm', c == 'M':
			if p.posix {
				return nil, gnuErr("the M flag of the s command")
			}
			multi = true
			p.i++
		case c == 'w':
			p.i++
			sub.file = p.fileArg()
			if sub.file == "" {
				return nil, errors.New("filename expected for the w flag of the s command")
			}
			break flags
		case c >= '0' && c <= '9':
			n := 0
			for d := p.peek(); d >= '0' && d <= '9'; d = p.peek() {
				n = n*10 + int(d-'0')
				p.i++
			}
			sub.nth = n
		default:
			return nil, fmt.Errorf("unknown flag in s command: %c", c)
		}
	}
	if p.posix && sub.nth > 0 && sub.global {
		return nil, gnuErr("a number with the g flag of the s command")
	}

	re, err := p.compileRE(reText, fold, multi)
	if err != nil {
		return nil, err
	}
	sub.re = re
	if sub.tmpl, err = p.compileRepl(replText); err != nil {
		return nil, err
	}
	return sub, nil
}

// translitArg parses the two strings of a y command.
func (p *parser) translitArg(cmd *command) error {
	if p.i >= len(p.s) {
		return errors.New("missing delimiter for y command")
	}
	delim := p.s[p.i]
	if delim == '\\' || delim == '\n' {
		return errors.New("invalid delimiter for y command")
	}
	p.i++
	from, err := p.scanTranslit(delim)
	if err != nil {
		return err
	}
	to, err := p.scanTranslit(delim)
	if err != nil {
		return err
	}
	if len(from) != len(to) {
		return errors.New("y command strings have different lengths")
	}
	m := make(map[rune]rune, len(from))
	for i, r := range from {
		if _, dup := m[r]; dup {
			return errors.New("repeated character in y command")
		}
		m[r] = to[i]
	}
	cmd.translit = m
	return nil
}

// scanTranslit scans the escaped runes of a y command string, up to the next
// unescaped delimiter, which is consumed.
func (p *parser) scanTranslit(delim byte) ([]rune, error) {
	var runes []rune
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == delim:
			p.i++
			return runes, nil
		case c == '\n':
			return nil, errors.New("unterminated string in y command")
		case c == '\\':
			p.i++
			if p.i >= len(p.s) {
				return nil, errors.New("unterminated string in y command")
			}
			d := p.s[p.i]
			switch d {
			case 'n':
				runes = append(runes, '\n')
			case '\\', delim:
				runes = append(runes, rune(d))
			default:
				return nil, fmt.Errorf("invalid escape in y command: \\%c", d)
			}
			p.i++
		default:
			r, size := utf8.DecodeRuneInString(p.s[p.i:])
			runes = append(runes, r)
			p.i += size
		}
	}
	return nil, errors.New("unterminated string in y command")
}

// resolve matches the braces and labels of the script, giving the b, t and
// T commands their jump targets.
func resolve(cmds []*command) error {
	labels := map[string]int{}
	for i, cmd := range cmds {
		if cmd.verb == ':' {
			if _, dup := labels[cmd.label]; !dup {
				labels[cmd.label] = i
			}
		}
	}
	for _, cmd := range cmds {
		switch cmd.verb {
		case 'b', 't', 'T':
			if cmd.label == "" {
				cmd.jump = len(cmds) // branch to the end of the script
				continue
			}
			idx, ok := labels[cmd.label]
			if !ok {
				return fmt.Errorf("label not found: %s", cmd.label)
			}
			cmd.jump = idx + 1
		case '{':
			if cmd.end == 0 || cmd.end > len(cmds) {
				cmd.end = len(cmds)
			}
		}
	}
	return nil
}

func (p *parser) peek() byte {
	if p.i >= len(p.s) {
		return 0
	}
	return p.s[p.i]
}

// number parses the digits at the cursor, failing when there are none.
func (p *parser) number() (int, error) {
	start := p.i
	for c := p.peek(); c >= '0' && c <= '9'; c = p.peek() {
		p.i++
	}
	if start == p.i {
		return 0, errors.New("missing number")
	}
	n, err := strconv.Atoi(p.s[start:p.i])
	if err != nil {
		return 0, errors.New("number out of range")
	}
	return n, nil
}

func (p *parser) blanks() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

func (p *parser) skipLine() {
	for p.i < len(p.s) && p.s[p.i] != '\n' {
		p.i++
	}
}

// compileRepl compiles the replacement of an s command, keeping the raw text
// as the parser scanned it. A backslash escapes the next character: a digit
// refers to a group of the pattern, an ampersand, a newline or the delimiter
// loses its special meaning, and the case conversion escapes of GNU change
// how the text which follows them is written. An unescaped ampersand stands
// for the whole match.
func (p *parser) compileRepl(raw string) ([]replItem, error) {
	var items []replItem
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			items = append(items, literalItem(lit.String()))
			lit.Reset()
		}
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch c {
		case '\\':
			i++
			if i >= len(raw) {
				return nil, errors.New("trailing backslash in replacement")
			}
			switch d := raw[i]; {
			case d >= '0' && d <= '9':
				flush()
				items = append(items, replItem{group: int(d - '0')})
			case d == 'u' || d == 'l' || d == 'U' || d == 'L' || d == 'E':
				if p.posix {
					return nil, gnuErr(`the \` + string(d) + ` escape of a replacement`)
				}
				flush()
				items = append(items, replItem{caseOp: d})
			default:
				lit.WriteByte(d)
			}
		case '&':
			flush()
			items = append(items, replItem{group: -1})
		default:
			lit.WriteByte(c)
		}
	}
	flush()
	return items, nil
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// isDelimSpecial reports whether c is a character that XBD 9.3.3 lists as
// special in BREs, or that XBD 9.4.3 additionally lists as special in EREs
// when the -E option selects them. An escaped delimiter that is special
// keeps its backslash in the scanned regular expression, since POSIX leaves
// whether the escape matches the literal or the special character
// unspecified, while a non-special one is required to match itself.
func (p *parser) isDelimSpecial(c byte) bool {
	switch c {
	case '.', '*', '[', '\\', '^', '$': // special in BREs and EREs
		return true
	case '(', ')', '+', '?', '{', '|': // special in EREs only
		return p.extended
	default:
		return false
	}
}

// bracketEnd returns the index of the ] closing the bracket expression
// starting at i, or -1 when there's none. A ] right after the opening [ or a
// leading ^ is a member of the expression, and the ] closing a [:...:],
// [.x.] or [=x=] subexpression doesn't close the expression.
func bracketEnd(pattern string, i int) int {
	j := i + 1
	if j < len(pattern) && pattern[j] == '^' {
		j++
	}
	if j < len(pattern) && pattern[j] == ']' {
		j++
	}
	for ; j < len(pattern); j++ {
		if pattern[j] == ']' {
			return j
		}
		if end := subEnd(pattern, j); end >= 0 {
			j = end
		}
	}
	return -1
}

// subEnd returns the index of the ] closing the [:...:], [.x.] or [=x=]
// subexpression starting at j, or -1 when the bracket at j doesn't open one.
func subEnd(pattern string, j int) int {
	if j+1 >= len(pattern) {
		return -1
	}
	var marker byte
	switch pattern[j+1] {
	case ':', '.', '=':
		marker = pattern[j+1]
	default:
		return -1
	}
	for k := j + 2; k+1 < len(pattern); k++ {
		if pattern[k] == marker && pattern[k+1] == ']' {
			return k + 1
		}
	}
	return -1
}
