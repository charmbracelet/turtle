package stat

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// quotingStyle is how a name is quoted by %N and %Qn, as the QUOTING_STYLE
// variable names.
type quotingStyle int

const (
	quoteLiteral           quotingStyle = iota
	quoteShell                          // '...' only when a name needs it
	quoteShellAlways                    // '...' always
	quoteShellEscape                    // '...' when it suffices, $'...' for the rest
	quoteShellEscapeAlways              // '...' or $'...' always
	quoteC                              // "..." with the C escapes
	quoteCMaybe                         // "..." only when a name needs it
	quoteEscape                         // the C escapes without the quotes
	quoteLocale                         // ‘...’ with the C escapes
	quoteCLocale                        // like locale, the quotes GNU takes for it
)

// quotingStyles maps the words QUOTING_STYLE stands for to the styles they
// name.
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

// quoteName writes name the way a quoting style asks: the shell styles
// quote the names a shell would read back, the C-like ones escape the
// nongraphic characters, and the literal one writes the name as it is.
func quoteName(name string, style quotingStyle) string {
	switch style {
	case quoteLiteral:
		return name
	case quoteShell, quoteShellAlways, quoteShellEscape, quoteShellEscapeAlways:
		return quoteShellStyle(name, style)
	default:
		return quoteCStyle(name, style)
	}
}

// shellSafe reports whether a character can stand in a shell word without
// the word being quoted.
func shellSafe(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	return strings.ContainsRune("-_=.,/:@+", r)
}

// quoteShellStyle writes a name the shell styles ask for: single quotes
// around the ones a shell would otherwise split or expand, and the $'...'
// form of the ones holding characters a single-quoted word can't carry.
func quoteShellStyle(name string, style quotingStyle) string {
	always := style == quoteShellAlways || style == quoteShellEscapeAlways
	escape := style == quoteShellEscape || style == quoteShellEscapeAlways
	needsQuote, needsDollar := false, false
	if name == "" {
		needsQuote = true
	}
	for _, r := range name {
		switch {
		case !shellSafe(r):
			needsQuote = true
			if r < 0o40 || r == 0o177 {
				needsDollar = true
			}
		case r == '\'':
			needsQuote = true
		}
	}
	if !always && !needsQuote {
		return name
	}
	if escape && needsDollar {
		return "$'" + quoteDollar(name) + "'"
	}
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}

// quoteDollar writes name inside a $'...' word, escaping the characters a
// dollar-quoted word can't carry as themselves.
func quoteDollar(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(`\` + octal3(name[i]))
			i++
			continue
		}
		switch r {
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case 0x1b:
			b.WriteString(`\e`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\v':
			b.WriteString(`\v`)
		case '\\', '\'':
			b.WriteString(`\` + string(r))
		default:
			if r < 0o40 || r == 0o177 {
				b.WriteString(`\` + octal3(byte(r)))
			} else {
				b.WriteRune(r)
			}
		}
		i += size
	}
	return b.String()
}

// quoteCStyle writes a name the C-like styles ask for: the escapes of a C
// character string literal around the nongraphic characters it holds, with
// the quotes of the style, or none for the escape style.
func quoteCStyle(name string, style quotingStyle) string {
	open, close := "\"", "\""
	switch style {
	case quoteEscape:
		open, close = "", ""
	case quoteLocale:
		open, close = "‘", "’"
	case quoteCLocale:
		open, close = "“", "”"
	}
	var b strings.Builder
	b.WriteString(open)
	escaped := false
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(`\` + octal3(name[i]))
			escaped = true
			i++
			continue
		}
		switch r {
		case '\a':
			b.WriteString(`\a`)
			escaped = true
		case '\b':
			b.WriteString(`\b`)
			escaped = true
		case '\f':
			b.WriteString(`\f`)
			escaped = true
		case '\n':
			b.WriteString(`\n`)
			escaped = true
		case '\r':
			b.WriteString(`\r`)
			escaped = true
		case '\t':
			b.WriteString(`\t`)
			escaped = true
		case '\v':
			b.WriteString(`\v`)
			escaped = true
		case '\\':
			b.WriteString(`\\`)
			escaped = true
		case '"':
			if open == "\"" {
				b.WriteString(`\"`)
				escaped = true
			} else {
				b.WriteByte('"')
			}
		default:
			if r < 0o40 || r == 0o177 {
				b.WriteString(`\` + octal3(byte(r)))
				escaped = true
			} else {
				b.WriteRune(r)
			}
		}
		i += size
	}
	b.WriteString(close)
	if style == quoteCMaybe && !escaped {
		return name
	}
	return b.String()
}

// octal3 writes a byte as a three-digit octal escape.
func octal3(c byte) string {
	return fmt.Sprintf("%03o", c)
}
