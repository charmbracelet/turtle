package strftime

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// posixTimeFmt is the date and time format of the POSIX locale, the one the
// %c conversion stands for.
const posixTimeFmt = "%a %b %e %H:%M:%S %Y"

// Format formats t as the strftime function of the C standard would in the
// POSIX locale, with the conversions of the +format operand of date.
//
// The E and O modifiers are accepted and ignored, like in any locale without
// alternative representations, and an unknown conversion is copied as is.
func Format(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		j := i + 1
		if j < len(format) && (format[j] == 'E' || format[j] == 'O') {
			j++
		}
		// The colons of the %:z, %::z and %:::z zone forms.
		cols := 0
		for j < len(format) && format[j] == ':' && cols < 3 {
			j++
			cols++
		}
		if j >= len(format) {
			// A trailing '%' or modifier: copy it as is.
			b.WriteString(format[i:])
			break
		}
		b.WriteString(convert(format[j], cols, t))
		i = j
	}
	return b.String()
}

// convert returns the string the conversion specifier c stands for, as it
// would be in the POSIX locale, with the number of colons of the zone forms.
func convert(c byte, cols int, t time.Time) string {
	switch c {
	case 'a':
		return t.Format("Mon")
	case 'A':
		return t.Format("Monday")
	case 'b', 'h':
		return t.Format("Jan")
	case 'B':
		return t.Format("January")
	case 'c':
		return Format(posixTimeFmt, t)
	case 'C':
		return pad2(t.Year() / 100)
	case 'd':
		return pad2(t.Day())
	case 'D':
		return Format("%m/%d/%y", t)
	case 'e':
		return fmt.Sprintf("%2d", t.Day())
	case 'F':
		return Format("%Y-%m-%d", t)
	case 'g':
		year, _ := t.ISOWeek()
		return pad2(year % 100)
	case 'G':
		year, _ := t.ISOWeek()
		return fmt.Sprintf("%04d", year)
	case 'H':
		return pad2(t.Hour())
	case 'I':
		return pad2(hour12(t))
	case 'j':
		return fmt.Sprintf("%03d", t.YearDay())
	case 'm':
		return pad2(int(t.Month()))
	case 'M':
		return pad2(t.Minute())
	case 'n':
		return "\n"
	case 'N':
		return fmt.Sprintf("%09d", t.Nanosecond())
	case 'p':
		return t.Format("PM")
	case 'r':
		return Format("%I:%M:%S %p", t)
	case 'R':
		return Format("%H:%M", t)
	case 'S':
		return pad2(t.Second())
	case 's':
		return strconv.FormatInt(t.Unix(), 10)
	case 't':
		return "\t"
	case 'T':
		return Format("%H:%M:%S", t)
	case 'u':
		return strconv.Itoa(isoWeekday(t))
	case 'U':
		return week(t, time.Sunday)
	case 'V':
		_, w := t.ISOWeek()
		return pad2(w)
	case 'w':
		return strconv.Itoa(int(t.Weekday()))
	case 'W':
		return week(t, time.Monday)
	case 'x':
		return Format("%m/%d/%y", t)
	case 'X':
		return Format("%H:%M:%S", t)
	case 'y':
		return pad2(t.Year() % 100)
	case 'Y':
		return strconv.Itoa(t.Year())
	case 'z':
		return zoneOffset(t, cols)
	case 'Z':
		return t.Format("MST")
	case '%':
		return "%"
	default:
		return "%" + strings.Repeat(":", cols) + string(c)
	}
}

// zoneOffset returns the numeric time zone offset of t, with the number of
// colons the GNU %:z, %::z and %:::z forms carry: "-0700", "-07:00",
// "-07:00:00", and with three of them the minimum precision, dropping the
// parts which are zero.
func zoneOffset(t time.Time, cols int) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	h, m, s := off/3600, off%3600/60, off%60
	switch {
	case cols == 1:
		return fmt.Sprintf("%s%02d:%02d", sign, h, m)
	case cols == 2:
		return fmt.Sprintf("%s%02d:%02d:%02d", sign, h, m, s)
	case cols == 3:
		switch {
		case m == 0:
			return fmt.Sprintf("%s%02d", sign, h)
		case s == 0:
			return fmt.Sprintf("%s%02d:%02d", sign, h, m)
		default:
			return fmt.Sprintf("%s%02d:%02d:%02d", sign, h, m, s)
		}
	default:
		return fmt.Sprintf("%s%02d%02d", sign, h, m)
	}
}

// week returns the week number of the year of t, with the weeks starting on
// firstDay and the days before the first one of them counted as week 00.
func week(t time.Time, firstDay time.Weekday) string {
	wday := (int(t.Weekday()) - int(firstDay) + 7) % 7
	return pad2((t.YearDay() - 1 + 7 - wday) / 7)
}

func hour12(t time.Time) int {
	h := t.Hour() % 12
	if h == 0 {
		return 12
	}
	return h
}

func isoWeekday(t time.Time) int {
	return (int(t.Weekday())+6)%7 + 1
}

func pad2(n int) string {
	return fmt.Sprintf("%02d", n)
}
