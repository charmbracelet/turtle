package ls

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// quotingStyle is how a name is quoted.
type quotingStyle int

const (
	quoteLiteral           quotingStyle = iota // -N: as it is
	quoteShell                                 // '...' only when a name needs it
	quoteShellAlways                           // '...' always
	quoteShellEscape                           // '...' when it suffices, $'...' for the rest
	quoteShellEscapeAlways                     // '...' or $'...' always
	quoteC                                     // -Q: "..." with the C escapes
	quoteCMaybe                                // "..." only when a name needs it
	quoteEscape                                // -b: the C escapes without the quotes
	quoteLocale                                // \u2018...\u2019 with the C escapes
	quoteCLocale                               // like locale, the quotes GNU takes for it
)

// quotingStyles maps the words of --quoting-style to the styles they stand
// for.
var quotingStyles = map[string]quotingStyle{
	"literal":             quoteLiteral,
	"shell":               quoteShell,
	"shell-always":        quoteShellAlways,
	"shell-escape":        quoteShellEscape,
	"shell-escape-always": quoteShellEscapeAlways,
	"c":                   quoteC,
	"c-maybe":             quoteCMaybe,
	"escape":              quoteEscape,
	"locale":              quoteLocale,
	"clocale":             quoteCLocale,
}

// quoteName writes name as the given style asks: the nongraphic characters
// it would leave as they are are written as question marks when hide asks
// for them to be hidden, and the characters of extra are quoted in
// addition to the ones the style quotes on its own. It reports whether the
// result differs from the name, the way the GNU tools tell a name they
// quoted from one they left alone.
func quoteName(name string, style quotingStyle, hide bool, extra string) (string, bool) {
	elide := style == quoteShell || style == quoteShellEscape || style == quoteCMaybe
	out, forced := quoteRestyled(name, style, elide, extra)
	if forced {
		out, _ = quoteRestyled(name, style, false, extra)
	}
	if hide && !escapesStyle(style) {
		out = hideControl(out)
	}
	return out, out != name
}

// escapesStyle reports whether a style escapes the nongraphic characters
// by itself, the way the C-like ones do.
func escapesStyle(style quotingStyle) bool {
	switch style {
	case quoteEscape, quoteC, quoteCMaybe, quoteShellEscape, quoteShellEscapeAlways,
		quoteLocale, quoteCLocale:
		return true
	}
	return false
}

// hideControl replaces the nongraphic characters of a string with question
// marks: one for each one that is part of a valid sequence, and one for
// each byte that isn't.
func hideControl(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteByte('?')
			i++
			continue
		}
		if !isGraphic(r) {
			b.WriteByte('?')
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// isGraphic reports whether a character is printable, as the wide
// character classification of the C language counts them.
func isGraphic(r rune) bool {
	return r == ' ' || (r != utf8.RuneError && unicode.IsGraphic(r))
}

// quoter holds the state of a name being quoted in one style.
type quoter struct {
	b strings.Builder

	backslash bool   // the C escapes are written
	shell     bool   // one of the four shell styles
	elide     bool   // the quotes of the style are left out
	quote     string // the closing quote the C-like styles escape
	extra     string // the characters quoted in addition to the rest

	force         bool // the name needs the quotes that would be left out
	pending       bool // a $'...' splice is open, waiting for its close
	sawApostrophe bool
	allCompat     bool // every character is quoted the same by C and the shell
}

// quoteRestyled quotes name in the given style, following the way the GNU
// quotearg does: the four shell styles are written as the always-quoted
// one, and report with forced that the name turned out to need the quotes
// they would have left out. A name holding an apostrophe and nothing the
// C style would escape is, for the readability of it, written that way.
func quoteRestyled(name string, style quotingStyle, elide bool, extra string) (string, bool) {
	q := &quoter{elide: elide, extra: extra, allCompat: true}
	switch style {
	case quoteCMaybe:
		q.backslash, q.quote = true, `"`
		q.open(`"`)
	case quoteC:
		q.backslash, q.quote = true, `"`
		q.open(`"`)
	case quoteEscape:
		q.backslash = true
	case quoteLocale:
		q.backslash, q.quote = true, "\u2019"
		q.open("\u2018")
	case quoteCLocale:
		q.backslash, q.quote = true, "\u2019"
		q.open("\u2018")
	case quoteShellEscape, quoteShell, quoteShellEscapeAlways, quoteShellAlways:
		q.shell = true
		q.backslash = style == quoteShellEscape || style == quoteShellEscapeAlways
		q.open("'")
	}
	for i := 0; i < len(name) && !q.force; {
		i += q.char(name, i)
	}
	if q.force || (len(name) == 0 && q.shell && elide) {
		return "", true
	}
	if q.shell && !elide && q.sawApostrophe && q.allCompat {
		// The name reads better in the C style than under the
		// apostrophes of the shell one.
		return quoteRestyled(name, quoteC, false, extra)
	}
	if !elide {
		switch {
		case q.shell:
			q.b.WriteString("'")
		default:
			q.b.WriteString(q.quote)
		}
	}
	return q.b.String(), false
}

// open writes the quote a style closes around a name.
func (q *quoter) open(quote string) {
	if !q.elide {
		q.b.WriteString(quote)
	}
}

// char quotes the character at i of name, returning how many bytes of it
// were taken.
func (q *quoter) char(name string, i int) int {
	c := name[i]
	if q.backslash && !q.shell && q.quote != "" && strings.HasPrefix(name[i:], q.quote) {
		// The character closes the quotes of the style, and is
		// escaped as one of the C ones.
		if q.elide {
			q.force = true
			return 1
		}
		q.escape()
		q.b.WriteString(q.quote)
		q.allCompat = false
		return len(q.quote)
	}
	compat := false
	switch c {
	case '\a':
		q.named(c, 'a')
		return 1
	case '\b':
		q.named(c, 'b')
		return 1
	case '\f':
		q.named(c, 'f')
		return 1
	case '\n', '\r', '\t':
		// These three also make the shell styles quote the name.
		if q.shell && q.elide {
			q.force = true
			return 1
		}
		q.named(c, "nrt"[strings.IndexByte("\n\r\t", c)])
		return 1
	case '\v':
		q.named(c, 'v')
		return 1
	case '\\':
		if q.shell {
			// The shell never escapes a backslash, but one that
			// would need quoting makes the quotes stay.
			if q.elide {
				q.force = true
				return 1
			}
			q.store(c, false)
			return 1
		}
		if q.elide && q.quote != "" {
			// One that would be the only need for quotes is left
			// alone.
			q.store(c, false)
			return 1
		}
		q.named(c, '\\')
		return 1
	case '\'':
		q.sawApostrophe = true
		compat = true
		if q.shell {
			// An apostrophe closes the quote, is escaped, and
			// opens a new one.
			if q.elide {
				q.force = true
				return 1
			}
			q.b.WriteString(`'\''`)
			q.pending = false
			q.noteCompat(compat)
			return 1
		}
	case '?':
		// A question mark would be taken for the hiding of a
		// nongraphic character, so the shell styles quote it.
		if q.shell && q.elide {
			q.force = true
			return 1
		}
	case '{', '}':
		if len(name) != 1 {
			break
		}
		fallthrough
	case '#', '~':
		if i != 0 {
			break
		}
		fallthrough
	case ' ':
		compat = true
		fallthrough
	case '!', '"', '$', '&', '(', ')', '*', ';', '<', '=', '>', '[', '^', '`', '|':
		// A character special to the shell: the quotes it asks for
		// can no longer be left out.
		if q.shell && q.elide {
			q.force = true
			return 1
		}
	default:
		return q.sequence(name, i)
	}
	if q.quotesExtra(c) {
		q.escape()
		q.store(c, compat)
		return 1
	}
	q.store(c, compat)
	return 1
}

// named writes the named escape of a control character, or the character
// itself when the style writes no escapes.
func (q *quoter) named(c byte, letter byte) {
	if !q.backslash {
		q.store(c, false)
		return
	}
	q.escape()
	if q.force {
		return
	}
	q.b.WriteByte(letter)
	q.allCompat = false
}

// sequence quotes the multibyte sequence at i of name: one that isn't
// printable is escaped byte by byte, and one that is is written as it is.
func (q *quoter) sequence(name string, i int) int {
	r, size := utf8.DecodeRuneInString(name[i:])
	printable := r != utf8.RuneError && isGraphic(r)
	if r == utf8.RuneError {
		size = 1
	}
	q.noteCompat(printable)
	if size > 1 || (q.backslash && !printable) {
		for j := 0; j < size; j++ {
			c := name[i+j]
			if q.backslash && !printable {
				q.escape()
				q.b.WriteByte('0' + c>>6)
				q.b.WriteByte('0' + (c>>3)&7)
				q.b.WriteByte('0' + c&7)
				continue
			}
			q.store(c, printable)
		}
		return size
	}
	if q.quotesExtra(name[i]) {
		q.escape()
	}
	q.store(name[i], printable)
	return 1
}

// escape writes the backslash an escaped character follows: one that has
// to open a $'...' splice first closes the quotes and marks where it ends.
func (q *quoter) escape() {
	if q.elide {
		q.force = true
		return
	}
	if q.shell && !q.pending {
		q.b.WriteString("'$'")
		q.pending = true
	}
	q.b.WriteByte('\\')
}

// store writes one character as it is, closing the $'...' splice an
// earlier escape opened before it.
func (q *quoter) store(c byte, compat bool) {
	if q.pending {
		q.b.WriteString("''")
		q.pending = false
	}
	q.b.WriteByte(c)
	q.noteCompat(compat)
}

// quotesExtra reports whether a character is one the caller asked to quote
// in addition to the ones the style does, in a place the style honors it.
func (q *quoter) quotesExtra(c byte) bool {
	if q.shell && !q.elide {
		return false
	}
	return strings.IndexByte(q.extra, c) >= 0
}

// noteCompat records whether a character is one the C and shell styles
// quote the same way.
func (q *quoter) noteCompat(compat bool) {
	if !compat {
		q.allCompat = false
	}
}
