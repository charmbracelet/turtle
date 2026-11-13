package ls

import (
	"strings"
	"time"

	"charm.land/turtle/coreutils/internal/strftime"
)

// timeStyle is how the long format writes a timestamp: the strftime format
// of a recent one, and the one of an old or future timestamp.
type timeStyle struct {
	recent string
	old    string
}

// localeStyle is the default time style, the abbreviated form of the POSIX
// locale, the only one this implementation knows.
var localeStyle = timeStyle{
	recent: "%b %e %H:%M",
	old:    "%b %e  %Y",
}

// isoStyles maps the time styles GNU names to the formats they stand for.
// The "locale" and "posix-STYLE" ones are resolved with the LC_TIME
// variable, the only locale category honored here.
var isoStyles = map[string]timeStyle{
	"full-iso": {
		recent: "%Y-%m-%d %H:%M:%S.%N %z",
		old:    "%Y-%m-%d %H:%M:%S.%N %z",
	},
	"long-iso": {
		recent: "%Y-%m-%d %H:%M",
		old:    "%Y-%m-%d %H:%M",
	},
	"iso": {
		recent: "%m-%d %H:%M",
		old:    "%Y-%m-%d ",
	},
}

// resolveTimeStyle returns the time style the given word stands for, or an
// error when it stands for none. A word starting with "+" is taken as a
// strftime format, holding the format of the recent timestamps and, after a
// newline, the one of the others. A "posix-" prefix picks the locale style
// when LC_TIME is the POSIX locale, and the word it prefixes otherwise.
func resolveTimeStyle(word, lcTime string) (timeStyle, error) {
	if strings.HasPrefix(word, "+") {
		old, recent, found := strings.Cut(word[1:], "\n")
		if !found {
			recent = old
		}
		return timeStyle{recent: recent, old: old}, nil
	}
	style, lcTimeGiven := strings.CutPrefix(word, "posix-")
	if !lcTimeGiven {
		return namedTimeStyle(word)
	}
	if lcTime == "" || lcTime == "C" || lcTime == "POSIX" {
		return localeStyle, nil
	}
	return namedTimeStyle(style)
}

// namedTimeStyle returns the time style a word without a "posix-" prefix
// stands for.
func namedTimeStyle(word string) (timeStyle, error) {
	if word == "locale" {
		return localeStyle, nil
	}
	if style, ok := isoStyles[word]; ok {
		return style, nil
	}
	return timeStyle{}, &invalidStyleError{word}
}

// format writes the date and time field of the long format for t: its
// recent form when t is less than six months old and not in the future, and
// its other form otherwise.
func (s timeStyle) format(t, now time.Time) string {
	if t.After(now) || t.Before(now.AddDate(0, -6, 0)) {
		return strftime.Format(s.old, t)
	}
	return strftime.Format(s.recent, t)
}

// invalidStyleError reports a word which stands for no time style.
type invalidStyleError struct{ word string }

func (e *invalidStyleError) Error() string {
	return "invalid argument '" + e.word + "' for '--time-style'"
}
