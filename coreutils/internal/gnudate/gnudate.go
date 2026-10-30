// Package gnudate parses GNU-style date strings, like the ones the -d options
// of date and touch take.
package gnudate

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse parses the GNU date string s relative to base, interpreting zoneless
// times in loc. Items are applied left to right: an absolute date or time
// item replaces the corresponding fields of the running result (a date
// resets the time of day to zero unless a time was already given), relative
// items shift it, and a time zone item only changes the zone. When loc is
// nil, UTC is used.
func Parse(s string, base time.Time, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	cur := base.In(loc)
	toks := tokenize(s)
	if len(toks) == 0 {
		return time.Time{}, fmt.Errorf("invalid date format %q", s)
	}
	var err error
	timeSeen := false
	monthSeen, daySeen, yearSeen := false, false, false

	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		next := ""
		if i+1 < len(toks) {
			next = toks[i+1]
		}

		if t, ok := parseKeyword(tok, cur, timeSeen); ok {
			cur = t
			continue
		}
		if t, ok := parseEpoch(tok); ok {
			cur = t
			continue
		}
		if strings.HasPrefix(tok, "@") {
			return time.Time{}, fmt.Errorf("invalid date format %q", s)
		}
		if m, ok := monthNumber(tok); ok {
			day, year := 1, 0
			j := i
			if d, ok := dayOf(next); ok {
				day, j = d, i+1
			} else if y, err := strconv.Atoi(next); err == nil {
				if y < 1000 || y > 9999 {
					return time.Time{}, fmt.Errorf("invalid date format %q", s)
				}
				year, j = y, i+1
			}
			if j+1 < len(toks) {
				if y, err := strconv.Atoi(toks[j+1]); err == nil && y >= 1000 && y <= 9999 {
					year, j = y, j+1
				}
			}
			cur, err = setDate(cur, year, int(m), day, timeSeen)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid date format %q", s)
			}
			yearSeen, monthSeen, daySeen = year != 0 || yearSeen, true, true
			i = j
			continue
		}
		if wd, ok := weekdayNumber(tok); ok {
			cur = addWeekday(cur, wd, 0)
			continue
		}
		if tok == "next" || tok == "last" {
			if wd, ok := weekdayNumber(next); ok {
				cur = addWeekday(cur, wd, map[bool]int{true: 1, false: -1}[tok == "next"])
				i++
				continue
			}
			if u, ok := unitOf(next); ok {
				n := 1
				if tok == "last" {
					n = -1
				}
				cur = addUnit(cur, u, n)
				i++
				continue
			}
			return time.Time{}, fmt.Errorf("invalid date format %q", s)
		}
		if t, isTime, ok := parseClock(tok, cur, timeSeen); ok {
			cur = t
			timeSeen = timeSeen || isTime
			continue
		}
		if z, ok := zoneOf(tok); ok {
			if timeSeen {
				// The zone re-interprets the time of day already given.
				_, o1 := cur.Zone()
				_, o2 := cur.In(z).Zone()
				cur = cur.Add(time.Duration(o1-o2) * time.Second)
			}
			cur = cur.In(z)
			continue
		}
		if n, ok := parseNumber(tok); ok {
			switch {
			case isMonth(next) && len(tok) <= 2 && n >= 1 && n <= 31:
				m, _ := monthNumber(next)
				cur, err = setDate(cur, 0, int(m), n, timeSeen)
				if err != nil {
					return time.Time{}, fmt.Errorf("invalid date format %q", s)
				}
				monthSeen, daySeen = true, true
				i++
				continue
			case isMonth(next) && n >= 1000 && n <= 9999:
				cur, err = setDate(cur, n, int(monthNumberOr(next)), 1, timeSeen)
				if err != nil {
					return time.Time{}, fmt.Errorf("invalid date format %q", s)
				}
				monthSeen, yearSeen = true, true
				i++
				continue
			case monthSeen && !daySeen && n >= 1 && n <= 31:
				cur, err = setDate(cur, cur.Year(), int(cur.Month()), n, timeSeen)
				if err != nil {
					return time.Time{}, fmt.Errorf("invalid date format %q", s)
				}
				daySeen = true
				continue
			case isUnit(next):
				u, _ := unitOf(next)
				j := i + 1
				if j+1 < len(toks) && toks[j+1] == "ago" {
					n = -n
					j++
				}
				cur = addUnit(cur, u, n)
				i = j
				continue
			case !yearSeen && n >= 1000 && n <= 9999:
				month, day := 1, 1
				if monthSeen {
					month = int(cur.Month())
					if daySeen {
						day = cur.Day()
					}
				}
				cur, err = setDate(cur, n, month, day, timeSeen)
				if err != nil {
					return time.Time{}, fmt.Errorf("invalid date format %q", s)
				}
				yearSeen = true
				continue
			}
			return time.Time{}, fmt.Errorf("invalid date format %q", s)
		}
		return time.Time{}, fmt.Errorf("invalid date format %q", s)
	}
	return cur, nil
}

// tokenize splits a date string into items, dropping spaces and the commas
// which follow month names.
func tokenize(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ','
	})
}

// parseKeyword handles now, today, tomorrow, yesterday, noon and midnight.
// The day keywords keep a time of day already given.
func parseKeyword(tok string, cur time.Time, timeSeen bool) (time.Time, bool) {
	switch strings.ToLower(tok) {
	case "now":
		return cur, true
	case "today":
		return shiftDay(cur, 0, timeSeen), true
	case "tomorrow":
		return shiftDay(cur, 1, timeSeen), true
	case "yesterday":
		return shiftDay(cur, -1, timeSeen), true
	case "noon":
		return time.Date(cur.Year(), cur.Month(), cur.Day(), 12, 0, 0, 0, cur.Location()), true
	case "midnight":
		return shiftDay(cur, 0, false), true
	}
	return time.Time{}, false
}

// shiftDay moves cur by n days, zeroing the time of day unless one was
// given.
func shiftDay(cur time.Time, n int, timeSeen bool) time.Time {
	h, m, s, ns := 0, 0, 0, 0
	if timeSeen {
		h, m, s, ns = cur.Hour(), cur.Minute(), cur.Second(), cur.Nanosecond()
	}
	day := time.Date(cur.Year(), cur.Month(), cur.Day(), h, m, s, ns, cur.Location())
	return day.AddDate(0, 0, n)
}

// parseEpoch handles @SECONDS[.FRACTION] times, which may carry a leading
// minus.
func parseEpoch(tok string) (time.Time, bool) {
	if !strings.HasPrefix(tok, "@") || len(tok) < 2 {
		return time.Time{}, false
	}
	sec, frac, found := strings.Cut(tok[1:], ".")
	if !found {
		frac = "0"
	}
	if len(frac) > 9 {
		return time.Time{}, false
	}
	s, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	for len(frac) < 9 {
		frac += "0"
	}
	n, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(s, n), true
}

// parseClock handles ISO dates ("2006-01-02"), date-times ("2006-01-02T15:04",
// with an optional zone) and times of day ("15:04[:05]", also in the 12-hour
// form with an am/pm suffix and an attached zone). isTime tells whether a
// time of day was given, as opposed to a bare date.
func parseClock(tok string, cur time.Time, timeSeen bool) (t time.Time, isTime bool, ok bool) {
	loc := cur.Location()
	hasDate := strings.Count(tok, "-") >= 2 || len(tok) >= 8 && tok[4] == '-'
	isTime = strings.Contains(tok, ":") && !hasDate
	if !hasDate && !isTime {
		return time.Time{}, false, false
	}

	layouts := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04Z0700",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
		"15:04:05Z07:00",
		"15:04:05Z0700",
		"15:04Z07:00",
		"15:04Z0700",
		"15:04:05",
		"15:04",
		"3:04:05PM",
		"3:04PM",
		"3PM",
		"3:04:05pm",
		"3:04pm",
		"3pm",
	}
	for _, layout := range layouts {
		p, err := time.ParseInLocation(layout, tok, loc)
		if err != nil {
			continue
		}
		switch {
		case isTime:
			return time.Date(
				cur.Year(), cur.Month(), cur.Day(),
				p.Hour(), p.Minute(), p.Second(), p.Nanosecond(),
				p.Location(),
			).In(cur.Location()), true, true
		case len(tok) > 10:
			return p.In(cur.Location()), true, true
		default:
			h, m, s, ns := 0, 0, 0, 0
			if timeSeen {
				h, m, s, ns = cur.Hour(), cur.Minute(), cur.Second(), cur.Nanosecond()
			}
			return time.Date(p.Year(), p.Month(), p.Day(), h, m, s, ns, cur.Location()), false, true
		}
	}
	return time.Time{}, false, false
}

// setDate replaces the date of cur, zeroing the time of day unless one was
// given. A year of zero keeps the current one. Dates which don't exist on
// the calendar, like February 30, are rejected.
func setDate(cur time.Time, year, month, day int, timeSeen bool) (time.Time, error) {
	if year == 0 {
		year = cur.Year()
	}
	h, m, s, ns := 0, 0, 0, 0
	if timeSeen {
		h, m, s, ns = cur.Hour(), cur.Minute(), cur.Second(), cur.Nanosecond()
	}
	t := time.Date(year, time.Month(month), day, h, m, s, ns, cur.Location())
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return time.Time{}, fmt.Errorf("invalid date format")
	}
	return t, nil
}

// addUnit shifts cur by n units.
func addUnit(cur time.Time, u string, n int) time.Time {
	switch u {
	case "second":
		return cur.Add(time.Duration(n) * time.Second)
	case "minute":
		return cur.Add(time.Duration(n) * time.Minute)
	case "hour":
		return cur.Add(time.Duration(n) * time.Hour)
	case "day":
		return cur.AddDate(0, 0, n)
	case "week":
		return cur.AddDate(0, 0, 7*n)
	case "month":
		return cur.AddDate(0, n, 0)
	case "year":
		return cur.AddDate(n, 0, 0)
	}
	return cur
}

// addWeekday moves cur to a weekday. With ord zero it lands on the closest
// coming one, today included; with +1 it skips to the next week's and with
// -1 to the previous week's.
func addWeekday(cur time.Time, wd time.Weekday, ord int) time.Time {
	diff := int(wd) - int(cur.Weekday())
	switch {
	case ord > 0:
		diff = ((diff-1)%7+7)%7 + 1
	case ord < 0:
		diff = -1 - (((-diff-1)%7 + 7) % 7)
	default:
		diff = ((diff % 7) + 7) % 7
	}
	return cur.AddDate(0, 0, diff)
}

// parseNumber parses a decimal count, which may carry a sign.
func parseNumber(tok string) (int, bool) {
	s := tok
	if s != "" && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if s == "" || !allDigits(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	if strings.HasPrefix(tok, "-") {
		return -n, true
	}
	return n, true
}

// allDigits reports whether s is only made of decimal digits.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// months and weekdays hold the names matched by their three-letter
// prefixes.
var months = []string{
	"january", "february", "march", "april", "may", "june",
	"july", "august", "september", "october", "november", "december",
}

var weekdays = []string{
	"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday",
}

// nameIndex reports whether the lowercase token is a prefix of at least
// three letters of one of the names.
func nameIndex(names []string, tok string) (int, bool) {
	tok = strings.ToLower(tok)
	if len(tok) < 3 {
		return 0, false
	}
	for i, name := range names {
		if len(tok) <= len(name) && strings.HasPrefix(name, tok) {
			return i, true
		}
	}
	return 0, false
}

// monthNumber tells the month the token names.
func monthNumber(tok string) (time.Month, bool) {
	i, ok := nameIndex(months, tok)
	if !ok {
		return 0, false
	}
	return time.Month(i + 1), true
}

func monthNumberOr(tok string) time.Month {
	if m, ok := monthNumber(tok); ok {
		return m
	}
	return time.January
}

// dayOf parses a day of the month.
func dayOf(tok string) (int, bool) {
	if len(tok) > 2 {
		return 0, false
	}
	n, err := strconv.Atoi(tok)
	if err != nil || n < 1 || n > 31 {
		return 0, false
	}
	return n, true
}

// weekdayNumber tells the weekday the token names.
func weekdayNumber(tok string) (time.Weekday, bool) {
	i, ok := nameIndex(weekdays, tok)
	if !ok {
		return 0, false
	}
	return time.Weekday(i), true
}

// units maps the words a relative count may be followed by.
var units = map[string]string{
	"s": "second", "sec": "second", "secs": "second", "second": "second", "seconds": "second",
	"m": "minute", "min": "minute", "mins": "minute", "minute": "minute", "minutes": "minute",
	"h": "hour", "hr": "hour", "hrs": "hour", "hour": "hour", "hours": "hour",
	"d": "day", "day": "day", "days": "day",
	"w": "week", "week": "week", "weeks": "week",
	"month": "month", "months": "month",
	"y": "year", "yr": "year", "yrs": "year", "year": "year", "years": "year",
}

// unitOf normalizes a relative unit word.
func unitOf(tok string) (string, bool) {
	u, ok := units[strings.ToLower(tok)]
	return u, ok
}

// isUnit tells whether the token is a relative unit word.
func isUnit(tok string) bool {
	_, ok := unitOf(tok)
	return ok
}

// isMonth tells whether the token is a month name.
func isMonth(tok string) bool {
	_, ok := monthNumber(tok)
	return ok
}

// zoneOf parses a time zone item: Z, UTC, GMT, or a numeric offset like
// -0530, +05:30 or +05.
func zoneOf(tok string) (*time.Location, bool) {
	switch strings.ToUpper(tok) {
	case "Z", "UTC", "GMT":
		return time.UTC, true
	}
	if len(tok) < 3 || tok[0] != '+' && tok[0] != '-' {
		return nil, false
	}
	sign := 1
	if tok[0] == '-' {
		sign = -1
	}
	t := tok[1:]
	hh, mm := t, "0"
	if h, m, found := strings.Cut(t, ":"); found {
		hh, mm = h, m
	} else if len(t) == 4 {
		hh, mm = t[:2], t[2:]
	} else if len(t) != 2 {
		return nil, false
	}
	if len(hh) != 2 || len(mm) != 2 || !allDigits(hh) || !allDigits(mm) {
		return nil, false
	}
	h, err1 := strconv.Atoi(hh)
	m, err2 := strconv.Atoi(mm)
	if err1 != nil || err2 != nil || h > 23 || m > 59 {
		return nil, false
	}
	return time.FixedZone(tok, sign*(h*3600+m*60)), true
}
